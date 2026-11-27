package interactions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func componentPayload(id string) string {
	return `{"type":3,` + guildFields + `,"data":{"custom_id":"` + id + `","component_type":2}}`
}

// untilWaiting blocks until n component waiters are registered.
func untilWaiting(t *testing.T, bot *Minidis, n int) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for bot.componentWaiters.Len() != n {
		if time.Now().After(deadline) {
			t.Fatalf("waiters = %d, want %d", bot.componentWaiters.Len(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

type waited struct {
	c   *ComponentContext
	err error
}

func waitAsync(bot *Minidis, ctx context.Context, filter func(*ComponentContext) bool) <-chan waited {
	got := make(chan waited, 1)
	go func() {
		c, err := bot.WaitForComponent(ctx, filter)
		got <- waited{c, err}
	}()
	return got
}

func byID(id string) func(*ComponentContext) bool {
	return func(c *ComponentContext) bool { return c.Data.CustomID == id }
}

func TestWaitForComponentTakesTheMatch(t *testing.T) {
	bot, requests := newTestBot(t)

	handled := false
	bot.HandleComponent("ok", func(*ComponentContext) error { handled = true; return nil })

	ctx, cancel := context.WithCancel(t.Context())
	got := waitAsync(bot, ctx, byID("ok"))
	untilWaiting(t, bot, 1)

	dispatched := make(chan struct{})
	go func() {
		defer close(dispatched)
		bot.handleInteraction(bot.Session, interaction(t, componentPayload("ok")))
	}()

	w := <-got
	if w.err != nil || w.c.Data.CustomID != "ok" || w.c.Author.ID != "u1" {
		t.Fatalf("got %+v", w)
	}
	if w.c.Err() != nil {
		t.Fatal("the returned context is already canceled")
	}
	if err := w.c.Update("done"); err != nil {
		t.Fatal(err)
	}

	// The interaction's handler holds its context open until ctx ends.
	select {
	case <-dispatched:
		t.Fatal("the handler returned before the waiter was done")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	<-dispatched

	if w.c.Err() == nil {
		t.Error("the returned context outlived ctx")
	}
	if handled {
		t.Error("the component's own handler ran")
	}
	if reqs := requests(); len(reqs) != 1 || reqs[0].Path != "/interactions/i1/tok/callback" || reqs[0].Body["type"] != float64(7) {
		t.Errorf("requests = %+v", reqs)
	}
	if n := bot.componentWaiters.Len(); n != 0 {
		t.Errorf("waiters = %d", n)
	}
}

func TestWaitForComponentPassesOtherComponentsOn(t *testing.T) {
	bot, _ := newTestBot(t)

	var handled string
	bot.HandleComponent("sel", func(c *ComponentContext) error { handled = c.Data.CustomID; return nil })

	var unhandled error
	bot.HandleError(func(c *Context, err error) { unhandled = err })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	got := waitAsync(bot, ctx, byID("ok"))
	untilWaiting(t, bot, 1)

	bot.handleInteraction(bot.Session, interaction(t, componentPayload("sel")))
	bot.handleInteraction(bot.Session, interaction(t, componentPayload("nobody")))

	if handled != "sel" {
		t.Errorf("handled = %q", handled)
	}
	if !errors.Is(unhandled, ErrNoHandler) {
		t.Errorf("unhandled = %v", unhandled)
	}
	if n := bot.componentWaiters.Len(); n != 1 {
		t.Errorf("waiters = %d, want the waiter still waiting", n)
	}

	cancel()
	if w := <-got; !errors.Is(w.err, context.Canceled) || w.c != nil {
		t.Errorf("got %+v", w)
	}
}

func TestWaitForComponentEnds(t *testing.T) {
	bot, _ := newTestBot(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if c, err := bot.WaitForComponent(ctx, nil); c != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("timeout: c = %v, err = %v", c, err)
	}

	got := waitAsync(bot, t.Context(), nil)
	untilWaiting(t, bot, 1)
	if err := bot.Close(); err != nil {
		t.Fatal(err)
	}
	if w := <-got; !errors.Is(w.err, context.Canceled) {
		t.Errorf("close: err = %v", w.err)
	}

	if n := bot.componentWaiters.Len(); n != 0 {
		t.Errorf("waiters = %d", n)
	}
}

func TestWaitForComponentConcurrently(t *testing.T) {
	bot, _ := newTestBot(t)

	const n = 20
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var waits []<-chan waited
	for i := range n {
		waits = append(waits, waitAsync(bot, ctx, byID(fmt.Sprint("b", i))))
	}
	// Two waiters with the same filter take one click each.
	twins := []<-chan waited{waitAsync(bot, ctx, byID("same")), waitAsync(bot, ctx, byID("same"))}
	untilWaiting(t, bot, n+2)

	var dispatched sync.WaitGroup
	for i := range n {
		dispatched.Go(func() { bot.handleInteraction(bot.Session, interaction(t, componentPayload(fmt.Sprint("b", i)))) })
	}
	dispatched.Go(func() { bot.handleInteraction(bot.Session, interaction(t, componentPayload("same"))) })
	dispatched.Go(func() { bot.handleInteraction(bot.Session, interaction(t, componentPayload("same"))) })

	for i, got := range waits {
		if w := <-got; w.err != nil || w.c.Data.CustomID != fmt.Sprint("b", i) {
			t.Errorf("waiter %d got %+v", i, w)
		}
	}
	for _, got := range twins {
		if w := <-got; w.err != nil || w.c.Data.CustomID != "same" {
			t.Errorf("twin got %+v", w)
		}
	}

	cancel()
	dispatched.Wait()
	if n := bot.componentWaiters.Len(); n != 0 {
		t.Errorf("waiters = %d", n)
	}
}

func TestWaitForComponentOverHTTP(t *testing.T) {
	bot, h, key := memoryHTTPBot(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	got := waitAsync(bot, ctx, byID("ok"))
	untilWaiting(t, bot, 1)

	responded := make(chan *httptest.ResponseRecorder)
	go func() {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, signedRequest(t, key, componentPayload("ok")))
		responded <- w
	}()

	w := <-got
	if w.err != nil || w.c.Data.CustomID != "ok" {
		t.Fatalf("got %+v", w)
	}
	if err := w.c.Update("done"); err != nil {
		t.Fatal(err)
	}

	// The request is answered once the waiter responds, not when it is done.
	if rec := <-responded; rec.Code != http.StatusAccepted {
		t.Errorf("status = %d: %s", rec.Code, rec.Body)
	}
}

func TestShutdownEndsAHandlerWaitingForAComponent(t *testing.T) {
	bot, h, key := memoryHTTPBot(t)

	type result struct{ err, ctxErr error }
	woke := make(chan result, 1)
	bot.AddCommand(&Command{Name: "ask", Execute: func(c *Context) error {
		if err := c.DeferReply(false); err != nil {
			return err
		}
		_, err := bot.WaitForComponent(c, nil)
		woke <- result{err, c.Err()}
		return nil
	}})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, signedRequest(t, key, memberSlash("ask", "u1", "0")))
	untilWaiting(t, bot, 1)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := bot.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown waited for the handler to time out: %v", err)
	}
	if r := <-woke; !errors.Is(r.err, context.Canceled) || r.ctxErr != nil {
		t.Errorf("err = %v, handler context err = %v; want Canceled with the context still open", r.err, r.ctxErr)
	}
}
