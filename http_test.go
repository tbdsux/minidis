package minidis

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
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

	h, err := bot.HTTPHandler(hex.EncodeToString(public))
	if err != nil {
		t.Fatal(err)
	}
	return h, private
}

func TestHTTPHandlerRejectsBadKeys(t *testing.T) {
	bot, _ := newTestBot(t)

	for _, key := range []string{"", "zz", strings.Repeat("ab", 31)} {
		if _, err := bot.HTTPHandler(key); err == nil {
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

	reqs := requests()
	if len(reqs) != 2 || reqs[0].Path != "/interactions/i1/tok/callback" || reqs[1].Path != "/webhooks/app/tok" {
		t.Errorf("requests = %+v", reqs)
	}
}

func TestHTTPHandlerAnswersWhenNothingResponds(t *testing.T) {
	bot, _ := newTestBot(t)
	h, key := newHTTPHandler(t, bot)

	done := make(chan int)
	go func() {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, signedRequest(t, key, `{"type":3,`+guildFields+`,"data":{"custom_id":"nobody","component_type":2}}`))
		done <- w.Code
	}()

	select {
	case code := <-done:
		if code != http.StatusAccepted {
			t.Errorf("status = %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("waited for the response window although the handler had returned")
	}
}

func TestCloseWaitsForHTTPHandlersAndRefusesNewOnes(t *testing.T) {
	bot, _ := newTestBot(t)
	h, key := newHTTPHandler(t, bot)

	started := make(chan struct{})
	var mu sync.Mutex
	var sawCancel bool
	bot.AddCommand(&Command{Name: "slow", Execute: func(c *Context) error {
		close(started)
		<-c.Done()
		time.Sleep(50 * time.Millisecond) // still running after the cancel

		mu.Lock()
		sawCancel = true
		mu.Unlock()
		return nil
	}})

	go h.ServeHTTP(httptest.NewRecorder(), signedRequest(t, key, slashPayload("slow", "")))
	<-started

	if err := bot.Close(); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !sawCancel {
		t.Error("Close returned before the HTTP handler finished")
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, signedRequest(t, key, slashPayload("slow", "")))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status after Close = %d", w.Code)
	}
}
