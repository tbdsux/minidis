package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tbdsux/minidis/discord"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeGateway serves /gateway/bot, and runs script[n] for the nth websocket connection.
type fakeGateway struct {
	t      *testing.T
	server *httptest.Server
	script []func(c *fakeConn)
	conns  atomic.Int32
	starts atomic.Int32
	remain atomic.Int32
}

type fakeConn struct {
	t    *testing.T
	ws   *websocket.Conn
	path string
	host string
}

func (c *fakeConn) send(op int, d any, seq int, event string) {
	c.t.Helper()
	payload := map[string]any{"op": op, "d": d}
	if event != "" {
		payload["s"], payload["t"] = seq, event
	}
	if err := c.ws.WriteJSON(payload); err != nil {
		c.t.Errorf("fake gateway write: %v", err)
	}
}

func (c *fakeConn) hello(intervalMS int) {
	c.send(opHello, map[string]int{"heartbeat_interval": intervalMS}, 0, "")
}

func (c *fakeConn) ready(seq int) {
	c.send(opDispatch, map[string]any{
		"v": 10, "session_id": "sess-1", "resume_gateway_url": "ws://" + c.host + "/resume",
		"user": map[string]any{"id": "42", "username": "bot"}, "guilds": []any{},
		"application": map[string]any{"id": "99", "flags": 0},
	}, seq, "READY")
}

// read returns the next payload with the given op, skipping heartbeats.
func (c *fakeConn) read(op int) gatewayPayload {
	c.t.Helper()
	_ = c.ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var p gatewayPayload
		if err := c.ws.ReadJSON(&p); err != nil {
			c.t.Errorf("fake gateway read waiting for op %d: %v", op, err)
			return p
		}
		if p.Op == op {
			return p
		}
		if p.Op != opHeartbeat {
			c.t.Errorf("got op %d, want %d", p.Op, op)
			return p
		}
	}
}

func (c *fakeConn) waitClosed() {
	_ = c.ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := c.ws.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *fakeConn) closeWith(code int) {
	msg := websocket.FormatCloseMessage(code, "test")
	_ = c.ws.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
}

func newFakeGateway(t *testing.T, script ...func(c *fakeConn)) (*Session, *fakeGateway) {
	t.Helper()

	identifyInterval, minReconnectDelay = 10*time.Millisecond, 5*time.Millisecond

	g := &fakeGateway{t: t, script: script}
	g.remain.Store(1000)

	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/gateway/bot", func(w http.ResponseWriter, r *http.Request) {
		g.starts.Add(1)
		fmt.Fprintf(w, `{"url":"ws://%s/gw","shards":1,"session_start_limit":{"total":1000,"remaining":%d,"reset_after":1000,"max_concurrency":1}}`,
			r.Host, g.remain.Load())
	})
	serveWS := func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer ws.Close()

		if got := r.URL.Query().Get("v"); got != "10" {
			t.Errorf("gateway version = %q", got)
		}
		if got := r.URL.Query().Get("encoding"); got != "json" {
			t.Errorf("gateway encoding = %q", got)
		}

		n := int(g.conns.Add(1)) - 1
		if n >= len(g.script) {
			(&fakeConn{t: t, ws: ws}).waitClosed()
			return
		}
		g.script[n](&fakeConn{t: t, ws: ws, path: r.URL.Path, host: r.Host})
	}
	mux.HandleFunc("/gw", serveWS)
	mux.HandleFunc("/resume", serveWS)

	g.server = httptest.NewServer(mux)
	t.Cleanup(g.server.Close)

	s := New("Bot token")
	s.BaseURL = g.server.URL
	return s, g
}

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestOpenIdentifiesAndDispatches(t *testing.T) {
	closed := make(chan struct{})

	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)

		var identify struct {
			Token      string `json:"token"`
			Intents    int    `json:"intents"`
			Properties struct {
				OS, Browser, Device string
			} `json:"properties"`
		}
		if err := json.Unmarshal(c.read(opIdentify).D, &identify); err != nil {
			t.Error(err)
		}
		if identify.Token != "Bot token" || identify.Intents != int(discord.IntentsGuilds) || identify.Properties.Browser != "minidis" {
			t.Errorf("identify = %+v", identify)
		}

		c.ready(1)
		c.send(opDispatch, map[string]any{"id": "m1", "channel_id": "c1", "content": "hi", "author": map[string]string{"id": "7"}}, 2, "MESSAGE_CREATE")
		c.send(opDispatch, map[string]any{"id": "i1", "application_id": "99", "type": 3, "token": "tok", "data": map[string]any{"custom_id": "b", "component_type": 2}}, 3, "INTERACTION_CREATE")
		c.send(opDispatch, map[string]any{"x": 1}, 4, "SOME_NEW_EVENT")

		// A clean Close must send 1000 so that the session ends.
		_ = c.ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		for {
			if _, _, err := c.ws.ReadMessage(); err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					t.Errorf("close: %v", err)
				}
				close(closed)
				return
			}
		}
	})
	s.Intents = discord.IntentsGuilds

	ready, message, interaction, raw := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})

	s.OnReady(func(_ *Session, r *discord.Ready) {
		if r.User.ID != "42" || r.Application.ID != "99" {
			t.Errorf("ready = %+v", r)
		}
		close(ready)
	})
	s.OnMessageCreate(func(_ *Session, m *discord.MessageCreate) {
		if m.Content != "hi" || m.Author.ID != "7" {
			t.Errorf("message = %+v", m.Message)
		}
		close(message)
	})
	s.OnInteractionCreate(func(_ *Session, i *discord.InteractionCreate) {
		if i.MessageComponentData().CustomID != "b" {
			t.Errorf("interaction = %+v", i.Interaction)
		}
		close(interaction)
	})
	s.OnRawEvent(func(_ *Session, event string, data json.RawMessage) {
		if event == "SOME_NEW_EVENT" {
			close(raw)
		}
	})

	if err := s.Open(); err != nil {
		t.Fatal(err)
	}

	if s.User().ID != "42" || s.ApplicationID() != "99" {
		t.Errorf("after Open: user=%+v app=%q", s.User(), s.ApplicationID())
	}
	if err := s.Open(); !errors.Is(err, ErrAlreadyOpen) {
		t.Errorf("second Open = %v", err)
	}

	waitFor(t, ready, "ready")
	waitFor(t, message, "message")
	waitFor(t, interaction, "interaction")
	waitFor(t, raw, "raw event")

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, closed, "close frame")
}

func TestOpenFailsOnFatalCloseCode(t *testing.T) {
	s, g := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.closeWith(4014)
		c.waitClosed()
	})

	var closeErr *GatewayCloseError
	if err := s.Open(); !errors.As(err, &closeErr) || closeErr.Code != 4014 {
		t.Fatalf("Open = %v, want close code 4014", err)
	}
	if n := g.conns.Load(); n != 1 {
		t.Errorf("connections = %d, want no retry", n)
	}
}

func TestOpenRefusesWithoutSessionStarts(t *testing.T) {
	s, g := newFakeGateway(t)
	g.remain.Store(0)

	if err := s.Open(); err == nil || !strings.Contains(err.Error(), "no session starts") {
		t.Fatalf("Open = %v", err)
	}
	if n := g.conns.Load(); n != 0 {
		t.Errorf("connections = %d, want 0", n)
	}
}

// resumeScript returns a connection script that expects a Resume with seq, on the resume URL.
func resumeScript(t *testing.T, seq int64, done chan struct{}) func(c *fakeConn) {
	return func(c *fakeConn) {
		if c.path != "/resume" {
			t.Errorf("resumed on %q", c.path)
		}
		c.hello(60000)

		var resume struct {
			Token     string `json:"token"`
			SessionID string `json:"session_id"`
			Seq       int64  `json:"seq"`
		}
		if err := json.Unmarshal(c.read(opResume).D, &resume); err != nil {
			t.Error(err)
		}
		if resume.SessionID != "sess-1" || resume.Seq != seq || resume.Token != "Bot token" {
			t.Errorf("resume = %+v", resume)
		}

		c.send(opDispatch, map[string]any{}, int(seq)+1, "RESUMED")
		close(done)
		c.waitClosed()
	}
}

func TestReconnectOpResumes(t *testing.T) {
	done := make(chan struct{})

	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(5)
		c.send(opReconnect, nil, 0, "")
		c.waitClosed()
	}, resumeScript(t, 5, done))

	if err := s.Open(); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	waitFor(t, done, "resume")
}

func TestResumableCloseCodeResumes(t *testing.T) {
	done := make(chan struct{})

	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(9)
		c.closeWith(4000)
		c.waitClosed()
	}, resumeScript(t, 9, done))

	if err := s.Open(); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	waitFor(t, done, "resume")
}

func TestMissedHeartbeatACKResumes(t *testing.T) {
	done := make(chan struct{})

	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(20)
		c.read(opIdentify)
		c.ready(3)
		c.waitClosed() // never ACKs
	}, resumeScript(t, 3, done))

	if err := s.Open(); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	waitFor(t, done, "resume")
}

func TestInvalidSessionIdentifiesAgain(t *testing.T) {
	cases := map[string]func(c *fakeConn){
		"op 9 false":      func(c *fakeConn) { c.send(opInvalidSession, false, 0, ""); c.waitClosed() },
		"close code 4009": func(c *fakeConn) { c.closeWith(4009); c.waitClosed() },
	}

	for name, end := range cases {
		t.Run(name, func(t *testing.T) {
			done := make(chan struct{})

			s, g := newFakeGateway(t, func(c *fakeConn) {
				c.hello(60000)
				c.read(opIdentify)
				c.ready(5)
				end(c)
			}, func(c *fakeConn) {
				if c.path != "/gw" {
					t.Errorf("new session on %q", c.path)
				}
				c.hello(60000)
				c.read(opIdentify)
				c.ready(1)
				close(done)
				c.waitClosed()
			})

			if err := s.Open(); err != nil {
				t.Fatal(err)
			}
			defer s.Close()

			waitFor(t, done, "second identify")

			// Open and the re-identify must each check the session start limit.
			if n := g.starts.Load(); n != 2 {
				t.Errorf("GET /gateway/bot calls = %d, want 2", n)
			}
		})
	}
}

func TestHeartbeatSendsLastSequenceAndAnswersServerRequest(t *testing.T) {
	done := make(chan struct{})

	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(17)

		c.send(opHeartbeat, nil, 0, "") // Discord asks for an immediate heartbeat

		_ = c.ws.SetReadDeadline(time.Now().Add(3 * time.Second))
		var p gatewayPayload
		if err := c.ws.ReadJSON(&p); err != nil {
			t.Error(err)
		}
		if p.Op != opHeartbeat || string(p.D) != "17" {
			t.Errorf("heartbeat = op %d d %s", p.Op, p.D)
		}

		close(done)
		c.waitClosed()
	})

	if err := s.Open(); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	waitFor(t, done, "heartbeat")
}

func TestCloseAllowsReopen(t *testing.T) {
	script := func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(1)
		c.waitClosed()
	}

	s, g := newFakeGateway(t, script, script)

	for range 2 {
		if err := s.Open(); err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if n := g.conns.Load(); n != 2 {
		t.Errorf("connections = %d", n)
	}
}
