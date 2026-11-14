package minidis

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// signedRequest builds an interaction request as Discord would send it.
func signedRequest(t *testing.T, key ed25519.PrivateKey, body string) *http.Request {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "/interactions", strings.NewReader(body))
	timestamp := fmt.Sprint(time.Now().Unix())
	r.Header.Set("X-Signature-Timestamp", timestamp)
	r.Header.Set("X-Signature-Ed25519", hex.EncodeToString(ed25519.Sign(key, []byte(timestamp+body))))
	return r
}

func newHTTPHandler(t *testing.T, bot *Minidis) (http.Handler, ed25519.PrivateKey) {
	t.Helper()

	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	h, err := bot.HTTPHandler(t.Context(), hex.EncodeToString(public))
	if err != nil {
		t.Fatal(err)
	}
	return h, private
}

func TestHTTPHandlerRejectsBadKeys(t *testing.T) {
	bot, _ := newTestBot(t)

	for _, key := range []string{"", "zz", strings.Repeat("ab", 31)} {
		if _, err := bot.HTTPHandler(t.Context(), key); err == nil {
			t.Errorf("key %q was accepted", key)
		}
	}
}

func TestHTTPHandlerVerifiesRequests(t *testing.T) {
	bot, _ := newTestBot(t)
	h, key := newHTTPHandler(t, bot)
	_, otherKey, _ := ed25519.GenerateKey(nil)

	ping := `{"type":1,"id":"p","application_id":"app","token":"t"}`

	tampered := signedRequest(t, key, ping)
	tampered.Header.Set("X-Signature-Timestamp", "1")

	unsigned := signedRequest(t, key, ping)
	unsigned.Header.Del("X-Signature-Ed25519")

	get := signedRequest(t, key, ping)
	get.Method = http.MethodGet

	tests := []struct {
		name string
		req  *http.Request
		want int
	}{
		{"valid ping", signedRequest(t, key, ping), http.StatusOK},
		{"wrong key", signedRequest(t, otherKey, ping), http.StatusUnauthorized},
		{"tampered timestamp", tampered, http.StatusUnauthorized},
		{"no signature", unsigned, http.StatusUnauthorized},
		{"not a POST", get, http.StatusMethodNotAllowed},
		{"invalid JSON", signedRequest(t, key, "{"), http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, tt.req)

			if w.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.want, w.Body)
			}
			if tt.want == http.StatusOK {
				if w.Header().Get("Content-Type") != "application/json" || strings.TrimSpace(w.Body.String()) != `{"type":1}` {
					t.Errorf("pong = %q %q", w.Header().Get("Content-Type"), w.Body)
				}
			}
		})
	}
}

func TestHTTPHandlerRunsHandlers(t *testing.T) {
	bot, requests := newTestBot(t)
	h, key := newHTTPHandler(t, bot)

	release, finished := make(chan struct{}), make(chan struct{})
	bot.AddCommand(&Command{Name: "ping", Execute: func(c *Context) error {
		defer close(finished)
		if err := c.Reply("pong"); err != nil {
			return err
		}

		// The request is answered once the reply is sent, before this returns.
		<-release
		_, err := c.Followup("more")
		return err
	}})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedRequest(t, key, slashPayload("ping", "")))

	if w.Code != http.StatusAccepted || w.Body.Len() != 0 {
		t.Fatalf("status = %d body = %q", w.Code, w.Body)
	}

	close(release)
	<-finished

	// The first request is HTTPHandler looking up the bot user.
	reqs := requests()
	if len(reqs) != 3 || reqs[0].Path != "/users/@me" || reqs[1].Path != "/interactions/i1/tok/callback" || reqs[2].Path != "/webhooks/app/tok" {
		t.Errorf("requests = %+v", reqs)
	}
}

// memoryHTTPBot is an HTTP-interactions bot whose API calls are answered in
// memory, so that its handlers can run inside a synctest bubble.
func memoryHTTPBot(t *testing.T) (*Minidis, http.Handler, ed25519.PrivateKey) {
	t.Helper()

	bot := New("token")
	bot.Session.HTTPClient = &http.Client{Transport: inMemoryAPI{}}
	h, key := newHTTPHandler(t, bot)
	return bot, h, key
}

func TestHTTPAnswerWaitsForTheResponse(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    time.Duration
	}{
		{"nothing responds: answered at the cutoff", `{"type":3,` + guildFields + `,"data":{"custom_id":"nobody","component_type":2}}`, responseWindow},
		{"a goroutine responds after the handler returned", slashPayload("later", ""), time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A bot per bubble: synctest keeps its WaitGroups to one bubble.
			bot, h, key := memoryHTTPBot(t)
			bot.HandleComponentFallback(func(*ComponentContext) error { return nil })
			bot.AddCommand(&Command{Name: "later", Execute: func(c *Context) error {
				go func() {
					time.Sleep(time.Second)
					_ = c.Reply("from a goroutine")
				}()
				return nil
			}})

			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				w := httptest.NewRecorder()
				h.ServeHTTP(w, signedRequest(t, key, tt.payload))

				if took := time.Since(start); w.Code != http.StatusAccepted || took != tt.want {
					t.Errorf("status %d after %v, want 202 after %v", w.Code, took, tt.want)
				}
			})
		})
	}
}

func TestShutdownLetsHTTPHandlersFinish(t *testing.T) {
	bot, _ := newTestBot(t)
	h, key := newHTTPHandler(t, bot)

	deferred, release := make(chan struct{}), make(chan struct{})
	var ctxErr, editErr error
	bot.AddCommand(&Command{Name: "slow", Execute: func(c *Context) error {
		if err := c.DeferReply(false); err != nil {
			return err
		}
		close(deferred)
		<-release
		ctxErr, editErr = c.Err(), c.Edit("done")
		return nil
	}})

	go h.ServeHTTP(httptest.NewRecorder(), signedRequest(t, key, slashPayload("slow", "")))
	<-deferred

	shutdown := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdown <- bot.Shutdown(ctx)
	}()

	select {
	case err := <-shutdown:
		t.Fatalf("Shutdown returned before the handler finished: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	// While draining, new interactions are refused.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedRequest(t, key, strings.Replace(slashPayload("slow", ""), `"id":"i1"`, `"id":"i2"`, 1)))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status while shutting down = %d", w.Code)
	}

	close(release)
	if err := <-shutdown; err != nil {
		t.Fatal(err)
	}
	if ctxErr != nil || editErr != nil {
		t.Errorf("the handler was cut short: ctx err = %v, edit err = %v", ctxErr, editErr)
	}
}

func TestShutdownDeadlineCancelsHTTPHandlers(t *testing.T) {
	bot, _ := newTestBot(t)
	h, key := newHTTPHandler(t, bot)

	deferred, done := make(chan struct{}), make(chan error, 1)
	bot.AddCommand(&Command{Name: "stuck", Execute: func(c *Context) error {
		if err := c.DeferReply(false); err != nil {
			return err
		}
		close(deferred)
		<-c.Done()
		done <- c.Err()
		return nil
	}})

	go h.ServeHTTP(httptest.NewRecorder(), signedRequest(t, key, slashPayload("stuck", "")))
	<-deferred

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := bot.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Shutdown = %v", err)
	}

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("handler ctx err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the handler was not canceled at the deadline")
	}
}

func TestHTTPHandlerRejectsReplays(t *testing.T) {
	bot, _ := newTestBot(t)
	h, key := newHTTPHandler(t, bot)

	runs := 0
	bot.AddCommand(&Command{Name: "once", Execute: func(c *Context) error {
		runs++
		return c.Reply("done")
	}})

	payload := slashPayload("once", "")
	first := signedRequest(t, key, payload)
	replay := first.Clone(t.Context())
	replay.Body = io.NopCloser(strings.NewReader(payload))

	for i, want := range []int{http.StatusAccepted, http.StatusConflict} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, []*http.Request{first, replay}[i])
		if w.Code != want {
			t.Errorf("request %d: status = %d, want %d", i, w.Code, want)
		}
	}

	// A correctly signed request with an old timestamp is refused too.
	stale := httptest.NewRequest(http.MethodPost, "/interactions", strings.NewReader(payload))
	old := fmt.Sprint(time.Now().Add(-time.Hour).Unix())
	stale.Header.Set("X-Signature-Timestamp", old)
	stale.Header.Set("X-Signature-Ed25519", hex.EncodeToString(ed25519.Sign(key, []byte(old+payload))))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, stale)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("stale request: status = %d", w.Code)
	}

	if runs != 1 {
		t.Errorf("handler ran %d times", runs)
	}
}

func TestHTTPErrorHandlerPanicIsContained(t *testing.T) {
	bot, h, key := memoryHTTPBot(t)

	calls := 0
	bot.HandleError(func(*Context, error) {
		calls++
		panic("error handler bug")
	})
	bot.AddCommand(&Command{Name: "fail", Execute: func(*Context) error { return errors.New("nope") }})

	// Without containment the panic would end the test binary, as the handler
	// runs in a goroutine of its own.
	synctest.Test(t, func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, signedRequest(t, key, slashPayload("fail", "")))

		if w.Code != http.StatusAccepted || calls != 1 {
			t.Errorf("status = %d, error handler calls = %d", w.Code, calls)
		}
	})
}

func TestHTTPHandlerFillsInTheBotUser(t *testing.T) {
	bot, _ := newTestBot(t)
	h, key := newHTTPHandler(t, bot)

	var botID string
	bot.AddCommand(&Command{Name: "who", Execute: func(c *Context) error {
		if c.Bot != nil {
			botID = c.Bot.ID
		}
		return c.Reply("me")
	}})

	h.ServeHTTP(httptest.NewRecorder(), signedRequest(t, key, slashPayload("who", "")))

	if botID != "sent" { // the fake API answers every GET with {"id":"sent"}
		t.Errorf("c.Bot.ID = %q", botID)
	}
}

func TestSignatureFreshness(t *testing.T) {
	bot, _ := newTestBot(t)
	h, key := newHTTPHandler(t, bot)

	sign := func(body string, at time.Time) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/interactions", strings.NewReader(body))
		ts := fmt.Sprint(at.Unix())
		r.Header.Set("X-Signature-Timestamp", ts)
		r.Header.Set("X-Signature-Ed25519", hex.EncodeToString(ed25519.Sign(key, []byte(ts+body))))
		return r
	}

	ping := `{"type":1,"id":"p","application_id":"app","token":"t"}`
	tests := []struct {
		name string
		req  *http.Request
		want int
	}{
		{"future-dated", sign(slashPayload("x", ""), time.Now().Add(time.Hour)), http.StatusUnauthorized},
		{"stale", sign(slashPayload("x", ""), time.Now().Add(-time.Hour)), http.StatusUnauthorized},
		{"a stale ping is still answered", sign(ping, time.Now().Add(-time.Hour)), http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, tt.req)
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}
