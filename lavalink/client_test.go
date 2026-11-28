package lavalink

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type request struct{ method, path, body string }

type fakeNode struct {
	t    *testing.T
	reqs chan request
	ws   chan *websocket.Conn
}

func newFakeNode(t *testing.T) (*fakeNode, *Client) {
	f := &fakeNode{t: t, reqs: make(chan request, 20), ws: make(chan *websocket.Conn, 1)}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	c := New(srv.URL+"/", "pw")
	t.Cleanup(func() { c.Close() })
	return f, c
}

var loadResults = map[string]string{
	"one":      `{"loadType":"track","data":{"encoded":"E1","info":{"title":"One"}}}`,
	"search":   `{"loadType":"search","data":[{"encoded":"E2"},{"encoded":"E3"}]}`,
	"playlist": `{"loadType":"playlist","data":{"info":{"name":"P"},"tracks":[{"encoded":"E4"}]}}`,
	"none":     `{"loadType":"empty","data":{}}`,
	"broken":   `{"loadType":"error","data":{"message":"boom","severity":"fault","cause":"x"}}`,
}

func (f *fakeNode) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "pw" {
		w.WriteHeader(401)
		return
	}
	switch {
	case r.URL.Path == "/v4/websocket":
		if r.Header.Get("User-Id") != "555" || !strings.HasPrefix(r.Header.Get("Client-Name"), "minidis/") {
			f.t.Errorf("websocket headers = %v", r.Header)
		}
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ws.WriteJSON(map[string]any{"op": "ready", "resumed": false, "sessionId": "S"})
		f.ws <- ws
	case r.URL.Path == "/v4/loadtracks":
		io.WriteString(w, loadResults[r.URL.Query().Get("identifier")])
	case r.URL.Path == "/v4/sessions/S/players/404":
		w.WriteHeader(404)
		io.WriteString(w, `{"status":404,"error":"Not Found","message":"player not found"}`)
	default:
		body, _ := io.ReadAll(r.Body)
		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		f.reqs <- request{r.Method, path, strings.TrimSpace(string(body))}
		if r.Method == "DELETE" {
			w.WriteHeader(204)
			return
		}
		io.WriteString(w, `{"guildId":"1","volume":100}`)
	}
}

func (f *fakeNode) want(method, path, body string) {
	f.t.Helper()
	select {
	case r := <-f.reqs:
		if r.method != method || r.path != path || r.body != body {
			f.t.Fatalf("request = %s %s %s\nwant      %s %s %s", r.method, r.path, r.body, method, path, body)
		}
	case <-time.After(5 * time.Second):
		f.t.Fatalf("no request, want %s %s", method, path)
	}
}

func (f *fakeNode) none() {
	f.t.Helper()
	select {
	case r := <-f.reqs:
		f.t.Fatalf("unexpected request %s %s %s", r.method, r.path, r.body)
	case <-time.After(50 * time.Millisecond):
	}
}

func connect(t *testing.T, f *fakeNode, c *Client) *websocket.Conn {
	t.Helper()
	if err := c.Connect(context.Background(), "555"); err != nil {
		t.Fatal(err)
	}
	if c.SessionID() != "S" {
		t.Fatalf("session = %q", c.SessionID())
	}
	return <-f.ws
}

func TestEvents(t *testing.T) {
	f, c := newFakeNode(t)
	events := make(chan *Event, 2)
	c.OnEvent(func(e *Event) { events <- e })
	ws := connect(t, f, c)

	ws.WriteJSON(map[string]any{"op": "playerUpdate", "guildId": "1", "state": map[string]any{}})
	ws.WriteJSON(map[string]any{"op": "event", "type": TrackEndEvent, "guildId": "1",
		"track": map[string]any{"encoded": "E1"}, "reason": EndFinished})
	ws.WriteJSON(map[string]any{"op": "event", "type": WebSocketClosedEvent, "guildId": "1",
		"code": 4014, "reason": "gone", "byRemote": true})

	e := <-events
	if e.Type != TrackEndEvent || e.GuildID != "1" || e.Track.Encoded != "E1" || !e.MayStartNext() {
		t.Fatalf("end event = %+v", e)
	}
	if e = <-events; e.Code != 4014 || e.Reason != "gone" || !e.ByRemote || e.MayStartNext() {
		t.Fatalf("closed event = %+v", e)
	}
}

func TestLoadTrack(t *testing.T) {
	_, c := newFakeNode(t)
	ctx := context.Background()
	for id, want := range map[string]string{"one": "E1", "search": "E2", "playlist": "E4"} {
		if tr, err := c.LoadTrack(ctx, id); err != nil || tr.Encoded != want {
			t.Errorf("LoadTrack(%s) = %+v, %v; want %s", id, tr, err, want)
		}
	}
	if _, err := c.LoadTrack(ctx, "none"); !errors.Is(err, ErrNoMatches) {
		t.Errorf("empty = %v", err)
	}
	if _, err := c.LoadTrack(ctx, "broken"); err == nil || err.(*Exception).Message != "boom" {
		t.Errorf("error = %v", err)
	}
	c.Password = "wrong"
	if _, err := c.LoadTrack(ctx, "one"); err == nil || err.(*Error).Status != 401 {
		t.Errorf("bad password = %v", err)
	}
}

func TestPlayerRequests(t *testing.T) {
	f, c := newFakeNode(t)
	ctx := context.Background()
	if err := c.Stop(ctx, "1"); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("before connect = %v", err)
	}
	connect(t, f, c)
	const p = "/v4/sessions/S/players/1"

	c.Play(ctx, "1", &Track{Encoded: "E1"})
	f.want("PATCH", p, `{"track":{"encoded":"E1"}}`)
	c.Stop(ctx, "1")
	f.want("PATCH", p, `{"track":{"encoded":null}}`)
	c.Pause(ctx, "1", true)
	f.want("PATCH", p, `{"paused":true}`)
	c.Seek(ctx, "1", 1500*time.Millisecond)
	f.want("PATCH", p, `{"position":1500}`)
	c.SetVolume(ctx, "1", 0)
	f.want("PATCH", p, `{"volume":0}`)
	c.UpdatePlayer(ctx, "1", &UpdatePlayer{Track: &UpdateTrack{Identifier: "x"}}, true)
	f.want("PATCH", p+"?noReplace=true", `{"track":{"identifier":"x"}}`)
	if err := c.Destroy(ctx, "1"); err != nil {
		t.Fatal(err)
	}
	f.want("DELETE", p, "")

	_, err := c.Player(ctx, "404")
	var e *Error
	if !errors.As(err, &e) || e.Status != 404 || e.Message != "player not found" {
		t.Fatalf("missing player = %v", err)
	}
}

func TestVoiceForwarding(t *testing.T) {
	f, c := newFakeNode(t)
	connect(t, f, c)
	const p = "/v4/sessions/S/players/1"

	c.voiceStateUpdate("1", "999", "other", "50") // someone else
	c.voiceStateUpdate("1", "555", "vs", "50")
	f.none() // no server yet
	c.voiceServerUpdate("1", "tok", "")
	f.none() // Discord is still allocating a server
	c.voiceServerUpdate("1", "tok", "ep:443")
	f.want("PATCH", p, `{"voice":{"token":"tok","endpoint":"ep:443","sessionId":"vs","channelId":"50"}}`)

	c.voiceStateUpdate("1", "555", "vs", "51") // moved channel
	f.want("PATCH", p, `{"voice":{"token":"tok","endpoint":"ep:443","sessionId":"vs","channelId":"51"}}`)

	c.voiceStateUpdate("1", "555", "vs", "") // left
	c.voiceServerUpdate("1", "tok2", "ep2")
	f.none()
}
