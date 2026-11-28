package interactions

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

var testPages = []Page{{Content: "one"}, {Content: "two"}, {Embeds: []*discord.MessageEmbed{{Title: "three"}}}}

// startPaginate runs a command that paginates with opts, returning a channel
// closed once its handler returns.
func startPaginate(t *testing.T, bot *Minidis, pages []Page, opts PaginateOptions) <-chan struct{} {
	t.Helper()

	bot.AddCommand(&Command{Name: "list", Execute: func(c *Context) error {
		return bot.Paginate(c, pages, opts)
	}})
	done := make(chan struct{})
	go func() {
		defer close(done)
		bot.handleInteraction(bot.Session, interaction(t, memberSlash("list", "u1", "0")))
	}()
	return done
}

func click(t *testing.T, bot *Minidis, userID, action string) {
	t.Helper()
	payload := strings.Replace(componentPayload("minidis:page:i1:"+action), `"id":"u1"`, `"id":"`+userID+`"`, 1)
	bot.handleInteraction(bot.Session, interaction(t, payload))
}

type testButton struct {
	label    string
	disabled bool
}

// buttons returns the buttons of a message body by the action in their custom ID.
func buttons(t *testing.T, msg map[string]any) map[string]testButton {
	t.Helper()

	got := map[string]testButton{}
	rows, _ := msg["components"].([]any)
	for _, row := range rows {
		for _, b := range row.(map[string]any)["components"].([]any) {
			b := b.(map[string]any)
			disabled, _ := b["disabled"].(bool)
			label, _ := b["label"].(string)
			got[strings.TrimPrefix(b["custom_id"].(string), "minidis:page:i1:")] = testButton{label, disabled}
		}
	}
	return got
}

func lastRequest(t *testing.T, requests func() []recordedRequest) recordedRequest {
	t.Helper()
	reqs := requests()
	if len(reqs) == 0 {
		t.Fatal("nothing was sent")
	}
	return reqs[len(reqs)-1]
}

func TestPaginateTurnsPages(t *testing.T) {
	bot, requests := newTestBot(t)
	done := startPaginate(t, bot, testPages, PaginateOptions{FirstLast: true, Counter: true, Stop: true})
	untilWaiting(t, bot, 1)

	reply := lastRequest(t, requests)
	data, _ := reply.Body["data"].(map[string]any)
	b := buttons(t, data)
	if reply.Body["type"] != float64(4) || data["content"] != "one" || len(b) != 6 ||
		!b["first"].disabled || !b["prev"].disabled || b["next"].disabled || b["counter"] != (testButton{"1/3", true}) {
		t.Fatalf("reply = %s", reply.Raw)
	}
	if rows := data["components"].([]any); len(rows) != 2 {
		t.Errorf("rows = %d, want Stop on a row of its own", len(rows))
	}

	click(t, bot, "u1", "last")
	untilWaiting(t, bot, 1)
	update := lastRequest(t, requests)
	data, _ = update.Body["data"].(map[string]any)
	b = buttons(t, data)
	embeds, _ := data["embeds"].([]any)
	if update.Body["type"] != float64(7) || data["content"] != "" || len(embeds) != 1 ||
		b["prev"].disabled || !b["next"].disabled || b["counter"].label != "3/3" {
		t.Fatalf("update = %s", update.Raw)
	}

	click(t, bot, "u2", "prev")
	untilWaiting(t, bot, 1)
	denied := lastRequest(t, requests)
	data, _ = denied.Body["data"].(map[string]any)
	if denied.Body["type"] != float64(4) || data["flags"] != float64(64) || !strings.Contains(data["content"].(string), "<@u1>") {
		t.Fatalf("another user's click = %s", denied.Raw)
	}

	click(t, bot, "u1", "prev")
	untilWaiting(t, bot, 1)
	if data, _ := lastRequest(t, requests).Body["data"].(map[string]any); data["content"] != "two" {
		t.Fatalf("prev = %s", lastRequest(t, requests).Raw)
	}

	click(t, bot, "u1", "stop")
	<-done
	stopped := lastRequest(t, requests)
	data, _ = stopped.Body["data"].(map[string]any)
	for action, b := range buttons(t, data) {
		if !b.disabled {
			t.Errorf("%s is still enabled after Stop: %s", action, stopped.Raw)
		}
	}
	if _, ok := data["content"]; ok {
		t.Errorf("Stop changed the page: %s", stopped.Raw)
	}
}

func TestPaginateLetsAnyoneTurnPages(t *testing.T) {
	bot, requests := newTestBot(t)
	done := startPaginate(t, bot, testPages, PaginateOptions{Anyone: true})
	untilWaiting(t, bot, 1)

	click(t, bot, "u2", "next")
	untilWaiting(t, bot, 1)
	if data, _ := lastRequest(t, requests).Body["data"].(map[string]any); data["content"] != "two" {
		t.Fatalf("got %s", lastRequest(t, requests).Raw)
	}
	if err := bot.Close(); err != nil {
		t.Fatal(err)
	}
	<-done
}

// wantDisabled fails unless the last request disabled the buttons, without
// changing the page.
func wantDisabled(t *testing.T, requests func() []recordedRequest) {
	t.Helper()

	edit := lastRequest(t, requests)
	b := buttons(t, edit.Body)
	if edit.Method != "PATCH" || edit.Path != "/webhooks/app/tok/messages/@original" || len(b) != 2 || !b["prev"].disabled || !b["next"].disabled {
		t.Fatalf("edit = %s %s %s", edit.Method, edit.Path, edit.Raw)
	}
	if _, ok := edit.Body["content"]; ok {
		t.Errorf("the page changed: %s", edit.Raw)
	}
}

func TestPaginateTimesOut(t *testing.T) {
	bot, requests := newTestBot(t)
	<-startPaginate(t, bot, testPages, PaginateOptions{Timeout: 10 * time.Millisecond})

	wantDisabled(t, requests)
	if n := bot.componentWaiters.Len(); n != 0 {
		t.Errorf("waiters = %d", n)
	}
}

func TestPaginateEndsWithShutdown(t *testing.T) {
	bot, requests := newTestBot(t)
	h, key := newHTTPHandler(t, bot)
	bot.AddCommand(&Command{Name: "list", Execute: func(c *Context) error {
		return bot.Paginate(c, testPages, PaginateOptions{Timeout: time.Hour})
	}})

	h.ServeHTTP(httptest.NewRecorder(), signedRequest(t, key, memberSlash("list", "u1", "0")))
	untilWaiting(t, bot, 1)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := bot.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown waited out the paginator: %v", err)
	}
	wantDisabled(t, requests)
}

func TestPaginateOnePage(t *testing.T) {
	bot, requests := newTestBot(t)
	<-startPaginate(t, bot, testPages[:1], PaginateOptions{})

	reqs := requests()
	if len(reqs) != 1 || reqs[0].Body["data"].(map[string]any)["components"] != nil {
		t.Errorf("requests = %+v", reqs)
	}
}

// A reply that fails to reach another user leaves the pages working, and a
// click that fails to turn the page leaves its token unused for the cleanup.
func TestPaginateFailedClicks(t *testing.T) {
	bot, _ := newTestBot(t)
	done := startPaginate(t, bot, testPages, PaginateOptions{})
	untilWaiting(t, bot, 1)

	var mu sync.Mutex
	var paths []string
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"message":"nope","code":0}`)
	}))
	t.Cleanup(failing.Close)
	bot.Session.BaseURL = failing.URL
	clickAs := func(userID, token, action string) {
		payload := strings.NewReplacer(`"id":"u1"`, `"id":"`+userID+`"`, `"token":"tok"`, `"token":"`+token+`"`).
			Replace(componentPayload("minidis:page:i1:" + action))
		bot.handleInteraction(bot.Session, interaction(t, payload))
	}

	clickAs("u2", "tok2", "next")
	untilWaiting(t, bot, 1) // still waiting for the author

	clickAs("u1", "tok3", "next")
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("paginator kept going after its update failed")
	}
	mu.Lock()
	defer mu.Unlock()
	if last := paths[len(paths)-1]; last != "PATCH /webhooks/app/tok/messages/@original" {
		t.Fatalf("cleanup = %s, want the edit with the slash command's token; all: %v", last, paths)
	}
}
