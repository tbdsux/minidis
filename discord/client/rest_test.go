package client

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tbdsux/minidis/discord"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newRESTSession(t *testing.T, h http.HandlerFunc) *Session {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	s := New("Bot token")
	s.BaseURL = srv.URL
	return s
}

func TestRequestSendsHeadersAndDecodes(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/channels/1/messages" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bot token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("User-Agent"); !strings.HasPrefix(got, "DiscordBot (") {
			t.Errorf("User-Agent = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["content"] != "hello" {
			t.Errorf("body = %v", body)
		}

		io.WriteString(w, `{"id":"m1","content":"hello","edited_timestamp":null,"components":[]}`)
	})

	m, err := s.ChannelMessageSend("1", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "m1" {
		t.Errorf("message = %+v", m)
	}
}

func TestRequestErrorIsDecoded(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"message":"Missing Access","code":50001}`)
	})

	_, err := s.ChannelMessageSend("1", "x")

	var restErr *RESTError
	if !errors.As(err, &restErr) {
		t.Fatalf("err = %v", err)
	}
	if restErr.StatusCode != 403 || restErr.Code != 50001 || restErr.Message != "Missing Access" {
		t.Errorf("error = %+v", restErr)
	}
}

func TestTokensNeverAppearInErrors(t *testing.T) {
	i := &discord.Interaction{ID: "1", AppID: "2", Token: "SECRETTOKEN"}

	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"message":"Unknown Webhook","code":10015}`)
	})

	if err := s.InteractionRespond(i, &discord.InteractionResponse{Type: discord.InteractionResponsePong}); err == nil || strings.Contains(err.Error(), "SECRETTOKEN") {
		t.Errorf("respond err = %v", err)
	}
	if err := s.InteractionResponseDelete(i); err == nil || strings.Contains(err.Error(), "SECRETTOKEN") {
		t.Errorf("delete err = %v", err)
	}

	// Transport errors from net/http include the full URL unless it is stripped.
	s.BaseURL = "http://127.0.0.1:1"
	if err := s.InteractionResponseDelete(i); err == nil || strings.Contains(err.Error(), "SECRETTOKEN") {
		t.Errorf("transport err = %v", err)
	}
}

func TestRateLimited429IsRetried(t *testing.T) {
	var calls atomic.Int32

	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"message":"You are being rate limited.","retry_after":0.05,"global":false}`)
			return
		}
		io.WriteString(w, `[]`)
	})

	start := time.Now()
	if _, err := s.ApplicationCommands("1", ""); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d", calls.Load())
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Errorf("retried after %s, before retry_after", time.Since(start))
	}
}

func TestPersistent429ReturnsError(t *testing.T) {
	var calls atomic.Int32

	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"message":"limited","retry_after":0.001,"global":false}`)
	})

	var restErr *RESTError
	if _, err := s.ApplicationCommands("1", ""); !errors.As(err, &restErr) || restErr.StatusCode != 429 {
		t.Fatalf("err = %v", err)
	}
	if calls.Load() != maxRateLimitRetries+1 {
		t.Errorf("calls = %d", calls.Load())
	}
}

func TestBucketExhaustionDelaysNextRequest(t *testing.T) {
	var calls atomic.Int32

	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("X-RateLimit-Bucket", "abc")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset-After", "0.1")
		}
		io.WriteString(w, `[]`)
	})

	if _, err := s.ApplicationCommands("1", ""); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if _, err := s.ApplicationCommands("1", ""); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 80*time.Millisecond {
		t.Errorf("second request waited only %s", time.Since(start))
	}
}

func TestInteractionCallbacksSkipExhaustedBuckets(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	s.limiter.setGlobal(time.Hour)

	done := make(chan error, 1)
	go func() {
		done <- s.InteractionRespond(&discord.Interaction{ID: "1", Token: "t"}, &discord.InteractionResponse{Type: discord.InteractionResponsePong})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("interaction callback waited on the global limit")
	}
}

func TestBadGatewayIsRetried(t *testing.T) {
	serverRetryDelay = time.Millisecond

	var calls atomic.Int32
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		io.WriteString(w, `[]`)
	})

	if _, err := s.ApplicationCommands("1", ""); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d", calls.Load())
	}
}

func TestContextCancelStopsRateLimitWait(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"message":"limited","retry_after":60,"global":false}`)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	if err := s.Request(ctx, "GET", "/x", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestMultipartUpload(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}

		mr := multipart.NewReader(r.Body, params["boundary"])

		part, err := mr.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := io.ReadAll(part)
		if part.FormName() != "payload_json" || part.Header.Get("Content-Type") != "application/json" ||
			!strings.Contains(string(payload), `"content":"see file"`) {
			t.Errorf("payload part %q: %s", part.FormName(), payload)
		}

		part, err = mr.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(part)
		if part.FormName() != "files[0]" || part.FileName() != "a.txt" || string(data) != "file body" ||
			part.Header.Get("Content-Type") != "text/plain" {
			t.Errorf("file part %q %q: %s", part.FormName(), part.FileName(), data)
		}

		io.WriteString(w, `{"id":"m"}`)
	})

	_, err := s.ChannelMessageSendComplex("1", &discord.MessageSend{
		Content: "see file",
		Files:   []*discord.File{{Name: "a.txt", ContentType: "text/plain", Reader: strings.NewReader("file body")}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEndpointPaths(t *testing.T) {
	i := &discord.Interaction{ID: "IID", AppID: "APP", Token: "TOK"}

	tests := []struct {
		name string
		want string
		call func(s *Session) error
	}{
		{"callback", "POST /interactions/IID/TOK/callback", func(s *Session) error {
			return s.InteractionRespond(i, &discord.InteractionResponse{Type: discord.InteractionResponseChannelMessageWithSource})
		}},
		{"edit original", "PATCH /webhooks/APP/TOK/messages/@original", func(s *Session) error {
			_, err := s.InteractionResponseEdit(i, &discord.WebhookEdit{})
			return err
		}},
		{"delete original", "DELETE /webhooks/APP/TOK/messages/@original", func(s *Session) error { return s.InteractionResponseDelete(i) }},
		{"followup", "POST /webhooks/APP/TOK", func(s *Session) error {
			_, err := s.FollowupMessageCreate(i, &discord.WebhookParams{Content: "x"})
			return err
		}},
		{"edit followup", "PATCH /webhooks/APP/TOK/messages/M", func(s *Session) error {
			_, err := s.FollowupMessageEdit(i, "M", &discord.WebhookEdit{})
			return err
		}},
		{"delete followup", "DELETE /webhooks/APP/TOK/messages/M", func(s *Session) error { return s.FollowupMessageDelete(i, "M") }},
		{"global commands", "GET /applications/APP/commands", func(s *Session) error {
			_, err := s.ApplicationCommands("APP", "")
			return err
		}},
		{"guild commands", "PUT /applications/APP/guilds/G/commands", func(s *Session) error {
			_, err := s.ApplicationCommandBulkOverwrite("APP", "G", nil)
			return err
		}},
		{"delete command", "DELETE /applications/APP/commands/C", func(s *Session) error { return s.ApplicationCommandDelete("APP", "", "C") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
				got = r.Method + " " + r.URL.Path
				w.WriteHeader(http.StatusNoContent)
			})

			if err := tt.call(s); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRouteKey(t *testing.T) {
	tests := []struct{ path, route, major string }{
		{"/channels/123456789012345678/messages", "GET /channels/123456789012345678/messages", "123456789012345678"},
		{"/channels/123456789012345678/messages/223456789012345678", "GET /channels/123456789012345678/messages/:id", "123456789012345678"},
		{"/applications/123456789012345678/commands/223456789012345678?x=1", "GET /applications/:id/commands/:id", ""},
		{"/webhooks/123456789012345678/tok/messages/@original", "GET /webhooks/123456789012345678/tok/messages/@original", "123456789012345678/tok"},
	}

	for _, tt := range tests {
		route, major := routeKey("GET", tt.path)
		if route != tt.route || major != tt.major {
			t.Errorf("routeKey(%q) = %q, %q", tt.path, route, major)
		}
	}
}

func TestLimiterEvictsExpiredBuckets(t *testing.T) {
	l := newRateLimiter()
	for i := range maxTrackedBuckets * 3 {
		l.bucketFor("route" + string(rune(i)))
	}
	if n := len(l.buckets); n > maxTrackedBuckets {
		t.Errorf("tracked buckets = %d", n)
	}
}
