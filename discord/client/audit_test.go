package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

func TestPathParametersStayOneSegment(t *testing.T) {
	var mu sync.Mutex
	var got []string
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Method+" "+r.RequestURI)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	if err := s.GuildMemberRemove(t.Context(), "111111111111111111", "222/../../../channels/333"); err != nil {
		t.Fatal(err)
	}
	if err := s.GuildBanCreate(t.Context(), "111111111111111111", "222?x=1#frag", 0); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"DELETE /guilds/111111111111111111/members/222%2F..%2F..%2F..%2Fchannels%2F333",
		"PUT /guilds/111111111111111111/bans/222%3Fx=1%23frag",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request %d = %q, want %q", i, got[i], want[i])
		}
	}

	if route, _ := routeKey(http.MethodDelete, memberPath("111111111111111111", "222/../../../channels/333")); route != "DELETE /guilds/111111111111111111/members/222%2F..%2F..%2F..%2Fchannels%2F333" {
		t.Errorf("route = %q", route)
	}
}

func TestOpenRetriesFailuresBeforeReady(t *testing.T) {
	s, g := newFakeGateway(t,
		func(c *fakeConn) {
			c.hello(60000)
			c.read(opIdentify)
			c.send(opInvalidSession, false, 0, "") // Identify contention
			c.waitClosed()
		},
		func(c *fakeConn) {
			c.hello(60000)
			c.read(opIdentify)
			c.ready(1)
			c.waitClosed()
		},
	)

	if err := s.Open(t.Context()); err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer s.Close()

	if n := g.conns.Load(); n != 2 {
		t.Errorf("connections = %d, want 2", n)
	}
}

func TestIdentifyIsSpacedAcrossShards(t *testing.T) {
	for _, tc := range []struct {
		mc    int32
		split bool // a Session for each shard
	}{{1, false}, {2, false}, {1, true}, {2, true}} {
		mc, split := tc.mc, tc.split
		t.Run(fmt.Sprintf("max_concurrency %d, split %t", mc, split), func(t *testing.T) {
			var mu sync.Mutex
			var identified []time.Time
			shard := func(c *fakeConn) {
				c.hello(60000)
				c.read(opIdentify)
				mu.Lock()
				identified = append(identified, time.Now())
				mu.Unlock()
				c.ready(1)
				c.waitClosed()
			}

			first, g := newFakeGateway(t, shard, shard)
			g.maxConcurrency.Store(mc)
			identifyInterval = 300 * time.Millisecond

			// Sessions in one process share the Identify buckets of a token,
			// whether one connects every shard or they split them.
			token := "Bot shards-" + t.Name() // a fresh Identify bucket
			sessions := []*Session{first}
			first.Token, first.TotalShards = token, 2
			if split {
				second := New(token)
				second.BaseURL = g.server.URL
				first.Shards, second.Shards = []int{0}, []int{1}
				second.TotalShards = 2
				sessions = append(sessions, second)
			}

			var wg sync.WaitGroup
			for _, s := range sessions {
				wg.Go(func() {
					if err := s.Open(t.Context()); err != nil {
						t.Error(err)
					}
				})
			}
			wg.Wait()
			for _, s := range sessions {
				defer s.Close()
			}

			gap := identified[1].Sub(identified[0])
			switch {
			case mc == 1 && gap < 250*time.Millisecond:
				t.Errorf("shards identified %s apart, want at least the interval", gap)
			case mc == 2 && gap > 150*time.Millisecond:
				t.Errorf("shards in different buckets identified %s apart", gap)
			}
		})
	}
}

func TestCloudflare429IsGlobal(t *testing.T) {
	cases := []struct {
		name   string
		header http.Header
		body   string
		global bool
	}{
		{"Cloudflare HTML", http.Header{"Retry-After": {"2"}}, "<html>banned</html>", true},
		{"Discord route limit", http.Header{"X-Ratelimit-Scope": {"user"}}, `{"retry_after":1,"global":false}`, false},
		{"Discord global", http.Header{"X-Ratelimit-Scope": {"global"}, "X-Ratelimit-Global": {"true"}}, `{"retry_after":1,"global":true}`, true},
	}
	for _, tt := range cases {
		if _, global := parseRateLimit(tt.header, []byte(tt.body)); global != tt.global {
			t.Errorf("%s: global = %v", tt.name, global)
		}
	}

	// After a Cloudflare 429, no other route sends until it passes.
	var first atomic.Bool
	var at429 time.Time
	arrivals := make(chan time.Time, 4)
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/channels/1/messages/1" && first.CompareAndSwap(false, true) {
			at429 = time.Now()
			w.Header().Set("Retry-After", "0.3")
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, "<html>slow down</html>")
			return
		}
		if r.URL.Path == "/channels/2/messages/2" {
			arrivals <- time.Now()
		}
		io.WriteString(w, `{"id":"1"}`)
	})

	var wg sync.WaitGroup
	defer wg.Wait() // do not leave the retrying request running into the next test
	wg.Go(func() { _, _ = s.ChannelMessage(context.Background(), "1", "1") })
	time.Sleep(50 * time.Millisecond) // the 429 has arrived
	if _, err := s.ChannelMessage(t.Context(), "2", "2"); err != nil {
		t.Fatal(err)
	}
	if gap := (<-arrivals).Sub(at429); gap < 250*time.Millisecond {
		t.Errorf("another route sent %s after a Cloudflare 429", gap)
	}
}

func TestGlobalLimitIsKeptAheadOfTime(t *testing.T) {
	var callbacks atomic.Int32
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		callbacks.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})
	s.limiter.globalRate = 100 // a burst of 100, then one per 10ms

	start := time.Now()
	for i := range 120 {
		if err := s.ChannelMessageDelete(t.Context(), fmt.Sprint(i), "1"); err != nil {
			t.Fatal(err)
		}
	}
	if took := time.Since(start); took < 180*time.Millisecond {
		t.Errorf("120 requests took %s, faster than the global limit allows", took)
	}

	// Interaction callbacks are exempt from the global limit.
	start = time.Now()
	i := &discord.Interaction{ID: "1", Token: "t"}
	for range 50 {
		if err := s.InteractionRespond(t.Context(), i, &discord.InteractionResponse{Type: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if took := time.Since(start); took > 300*time.Millisecond {
		t.Errorf("interaction callbacks waited for the global limit: %s", took)
	}
}

func TestGatewayEventsStayWithinBudget(t *testing.T) {
	defer func(limit int, window time.Duration) { gatewaySendLimit, gatewaySendWindow = limit, window }(gatewaySendLimit, gatewaySendWindow)
	gatewaySendLimit, gatewaySendWindow = 3, 300*time.Millisecond

	done := make(chan struct{})
	var presences int
	var last string
	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify) // 1 of 3
		c.ready(1)

		deadline := time.Now().Add(2 * time.Second)
		for last != "p9" && time.Now().Before(deadline) {
			p := c.read(opPresenceUpdate)
			presences++
			if got := decodePresence(t, p); len(got.Activities) > 0 {
				last = got.Activities[0].Name
			}
		}
		close(done)
		c.waitClosed()
	})

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for i := range 10 {
		if err := s.UpdatePresence(&discord.PresenceUpdate{Activities: []*discord.Activity{{Name: fmt.Sprintf("p%d", i)}}}); err != nil {
			t.Fatal(err)
		}
	}

	waitFor(t, done, "the latest presence")
	if last != "p9" || presences > 3 {
		t.Errorf("got %d presence updates ending with %q; want the latest, with the rest combined", presences, last)
	}
}

func TestAuditLogReason(t *testing.T) {
	var got string
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Audit-Log-Reason")
		w.WriteHeader(http.StatusNoContent)
	})

	reason := "spam & eggs/ü"
	if err := s.GuildBanCreate(WithAuditLogReason(t.Context(), reason), "1", "2", 0); err != nil {
		t.Fatal(err)
	}
	if want := url.PathEscape(reason); got != want {
		t.Errorf("header = %q, want %q", got, want)
	}
}

func TestRejectedTokenFailsFast(t *testing.T) {
	var calls atomic.Int32
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"message":"401: Unauthorized","code":0}`)
	})

	// A webhook token is not the bot token: its rejection proves nothing.
	i := &discord.Interaction{AppID: "1", Token: "stale"}
	if _, err := s.FollowupMessageCreate(t.Context(), i, &discord.WebhookParams{Content: "x"}); err == nil {
		t.Fatal("followup succeeded")
	}

	for range 3 {
		var restErr *RESTError
		if _, err := s.Channel(t.Context(), "1"); !errors.As(err, &restErr) || restErr.StatusCode != http.StatusUnauthorized {
			t.Fatalf("err = %v", err)
		}
	}

	if n := calls.Load(); n != 2 {
		t.Errorf("requests sent = %d, want the followup and one more", n)
	}
}

func TestFailedOpenForgetsThePreviousRun(t *testing.T) {
	s, _ := newFakeGateway(t,
		func(c *fakeConn) {
			c.hello(60000)
			c.read(opIdentify)
			c.ready(1)
			c.closeWith(4014)
			c.waitClosed()
		},
		func(c *fakeConn) {
			c.hello(60000)
			c.read(opIdentify)
			c.closeWith(4013)
			c.waitClosed()
		},
	)

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, s.Done(), "the fatal close")

	if err := s.Open(t.Context()); err == nil {
		t.Fatal("second Open succeeded")
	}
	if s.Done() != nil || s.Err() != nil {
		t.Errorf("after a failed Open, Done = %v and Err = %v, want both nil", s.Done(), s.Err())
	}
}

func TestReopenFetchesTheGatewayOnce(t *testing.T) {
	session := func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(1)
		c.waitClosed()
	}
	s, g := newFakeGateway(t, session, session)

	for range 2 {
		if err := s.Open(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if n := g.starts.Load(); n != 2 {
		t.Errorf("GET /gateway/bot calls = %d, want one per Open", n)
	}
}

func TestIdentifyPayloadHasRawToken(t *testing.T) {
	got := make(chan string, 1)
	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		var identify struct {
			Token string `json:"token"`
		}
		_ = json.Unmarshal(c.read(opIdentify).D, &identify)
		got <- identify.Token
		c.ready(1)
		c.waitClosed()
	})

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if token := <-got; token != "token" {
		t.Errorf("Identify token = %q, want it without the Bot prefix", token)
	}
}
