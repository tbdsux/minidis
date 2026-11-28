package node

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tbdsux/minidis/voice"
)

// fakeSource: "short" plays 3 frames, "long" never ends, "stuck" never
// yields, "fail" can't open, "bad" can't load, "nope" is unsupported.
type fakeSource struct{ opens chan string }

func (f *fakeSource) Load(_ context.Context, id string) (TrackInfo, error) {
	switch id {
	case "bad":
		return TrackInfo{}, errors.New("probe failed")
	case "nope":
		return TrackInfo{}, ErrUnsupported
	}
	return TrackInfo{Identifier: id, Title: id, Length: 60000, IsSeekable: true, SourceName: "fake"}, nil
}

// Search answers "find:q" with q-1 and q-2, and "find:none" with nothing.
func (f *fakeSource) Search(_ context.Context, id string) ([]TrackInfo, error) {
	q, ok := strings.CutPrefix(id, "find:")
	switch {
	case !ok:
		return nil, ErrUnsupported
	case q == "none":
		return nil, nil
	case q == "broken":
		return nil, errors.New("search failed")
	}
	return []TrackInfo{{Identifier: q + "-1", Title: q + "-1"}, {Identifier: q + "-2", Title: q + "-2"}}, nil
}

func (f *fakeSource) Open(ctx context.Context, info TrackInfo, pos time.Duration, volume int) (FrameReader, error) {
	f.opens <- fmt.Sprintf("%s@%d v%d", info.Identifier, pos.Milliseconds(), volume)
	switch info.Identifier {
	case "fail":
		return nil, errors.New("cannot open")
	case "short":
		return &fakeReader{left: 3}, nil
	case "stuck":
		return &fakeReader{block: make(chan struct{})}, nil
	}
	return &fakeReader{left: -1}, nil
}

type fakeReader struct {
	left  int
	block chan struct{}
	once  sync.Once
}

func (r *fakeReader) ReadFrame() ([]byte, error) {
	if r.block != nil {
		<-r.block
		return nil, io.EOF
	}
	if r.left == 0 {
		return nil, io.EOF
	}
	r.left--
	return []byte("frame"), nil
}

func (r *fakeReader) Close() error {
	if r.block != nil {
		r.once.Do(func() { close(r.block) })
	}
	return nil
}

type fakeConn struct {
	frames   chan []byte
	speaking chan bool
	done     chan struct{}
	once     sync.Once
	err      error
}

func newFakeConn() *fakeConn {
	return &fakeConn{frames: make(chan []byte, 1000), speaking: make(chan bool, 100), done: make(chan struct{})}
}

func (c *fakeConn) WriteOpus(f []byte) error { c.frames <- f; return nil }
func (c *fakeConn) Speaking(on bool) error   { c.speaking <- on; return nil }
func (c *fakeConn) Ping() int64              { return 7 }
func (c *fakeConn) Done() <-chan struct{}    { return c.done }
func (c *fakeConn) Err() error               { return c.err }
func (c *fakeConn) Close() error             { c.closeWith(nil); return nil }
func (c *fakeConn) closeWith(err error)      { c.once.Do(func() { c.err = err; close(c.done) }) }

type harness struct {
	t      *testing.T
	url    string
	ws     *websocket.Conn
	sid    string
	src    *fakeSource
	dials  chan voice.Params
	conns  chan *fakeConn
	dialFn func() (VoiceConn, error)
}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, src: &fakeSource{opens: make(chan string, 100)},
		dials: make(chan voice.Params, 10), conns: make(chan *fakeConn, 10)}
	h.dialFn = func() (VoiceConn, error) {
		c := newFakeConn()
		h.conns <- c
		return c, nil
	}
	srv := httptest.NewServer(New(Config{
		Password: "pw", Source: h.src,
		Dial: func(_ context.Context, p voice.Params) (VoiceConn, error) {
			h.dials <- p
			return h.dialFn()
		},
		StuckThreshold: 100 * time.Millisecond, PlayerUpdateInterval: 30 * time.Millisecond, StatsInterval: time.Hour,
		Logger: slog.New(slog.DiscardHandler),
	}))
	t.Cleanup(srv.Close)
	h.url = srv.URL

	hdr := http.Header{"Authorization": {"pw"}, "User-Id": {"555"}, "Client-Name": {"test/1"}}
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/v4/websocket", hdr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	h.ws = ws
	ready := h.next("ready")
	if ready["resumed"] != false {
		t.Fatalf("ready = %v", ready)
	}
	h.sid = ready["sessionId"].(string)
	return h
}

// next reads WebSocket messages until one with op (or event type) want.
func (h *harness) next(want string) map[string]any {
	h.t.Helper()
	for {
		h.ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		var m map[string]any
		if err := h.ws.ReadJSON(&m); err != nil {
			h.t.Fatalf("waiting for %s: %v", want, err)
		}
		if m["op"] == want || m["type"] == want {
			return m
		}
		if m["op"] == "event" {
			h.t.Fatalf("got event %v while waiting for %s", m["type"], want)
		}
	}
}

func (h *harness) do(method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.url+path, r)
	req.Header.Set("Authorization", "pw")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (h *harness) patch(body any) map[string]any {
	h.t.Helper()
	code, out := h.do("PATCH", "/v4/sessions/"+h.sid+"/players/42", body)
	if code != 200 {
		h.t.Fatalf("PATCH = %d %v", code, out)
	}
	return out
}

func (h *harness) wantOpen(want string) {
	h.t.Helper()
	select {
	case got := <-h.src.opens:
		if got != want {
			h.t.Fatalf("open = %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		h.t.Fatalf("no open, want %q", want)
	}
}

func encoded(id string) string { return newTrack(TrackInfo{Identifier: id}).Encoded }

var testVoice = map[string]any{"token": "tok", "endpoint": "ep.discord.media", "sessionId": "vs", "channelId": "99"}

// join connects voice and returns the fake connection.
func (h *harness) join() *fakeConn {
	h.t.Helper()
	h.patch(map[string]any{"voice": testVoice})
	p := <-h.dials
	if p.UserID != "555" || p.GuildID != "42" || p.ChannelID != "99" || p.SessionID != "vs" || p.Token != "tok" || p.Endpoint != "ep.discord.media" {
		h.t.Fatalf("dial params = %+v", p)
	}
	c := <-h.conns
	for h.next("playerUpdate")["state"].(map[string]any)["connected"] != true {
	}
	return c
}

func TestAuth(t *testing.T) {
	h := newHarness(t)
	resp, _ := http.Get(h.url + "/v4/info")
	if resp.StatusCode != 401 {
		t.Fatalf("no password = %d", resp.StatusCode)
	}
	if _, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(h.url, "http")+"/v4/websocket",
		http.Header{"Authorization": {"wrong"}, "User-Id": {"1"}}); err == nil {
		t.Fatal("websocket with wrong password connected")
	}
	if code, info := h.do("GET", "/v4/info", nil); code != 200 || info["version"].(map[string]any)["major"] != 4.0 {
		t.Fatalf("info = %d %v", code, info)
	}
	if code, st := h.do("GET", "/v4/stats", nil); code != 200 || st["players"] != 0.0 {
		t.Fatalf("stats = %d %v", code, st)
	}
	if code, _ := h.do("GET", "/v4/sessions/nope/players", nil); code != 404 {
		t.Fatalf("unknown session = %d", code)
	}
}

func TestLoadAndDecode(t *testing.T) {
	h := newHarness(t)
	_, res := h.do("GET", "/v4/loadtracks?identifier=song", nil)
	if res["loadType"] != "track" {
		t.Fatalf("load = %v", res)
	}
	track := res["data"].(map[string]any)
	if track["info"].(map[string]any)["title"] != "song" {
		t.Fatalf("track = %v", track)
	}
	_, dec := h.do("GET", "/v4/decodetrack?encodedTrack="+track["encoded"].(string), nil)
	if dec["info"].(map[string]any)["identifier"] != "song" {
		t.Fatalf("decode = %v", dec)
	}
	if _, res := h.do("GET", "/v4/loadtracks?identifier=nope", nil); res["loadType"] != "empty" {
		t.Fatalf("unsupported = %v", res)
	}
	if _, res := h.do("GET", "/v4/loadtracks?identifier=bad", nil); res["loadType"] != "error" ||
		res["data"].(map[string]any)["message"] != "probe failed" {
		t.Fatalf("error = %v", res)
	}
	if code, _ := h.do("GET", "/v4/decodetrack?encodedTrack=!!!", nil); code != 400 {
		t.Fatalf("bad decode = %d", code)
	}
}

func TestPlayToEnd(t *testing.T) {
	h := newHarness(t)
	c := h.join()
	h.patch(map[string]any{"track": map[string]any{"encoded": encoded("short"), "userData": map[string]any{"by": "me"}}})
	h.wantOpen("short@0 v100")
	start := h.next("TrackStartEvent")
	if start["guildId"] != "42" || start["track"].(map[string]any)["userData"].(map[string]any)["by"] != "me" {
		t.Fatalf("start = %v", start)
	}
	end := h.next("TrackEndEvent")
	if end["reason"] != "finished" {
		t.Fatalf("end = %v", end)
	}
	if <-c.speaking != true || <-c.speaking != false {
		t.Fatal("speaking not toggled on then off")
	}
	var frames []string
	for len(c.frames) > 0 {
		frames = append(frames, string(<-c.frames))
	}
	if len(frames) != 8 || frames[0] != "frame" || frames[3] != string(voice.SilenceFrame) {
		t.Fatalf("frames = %q, want 3 frames then 5 silence", frames)
	}
	if _, p := h.do("GET", "/v4/sessions/"+h.sid+"/players/42", nil); p["track"] != nil {
		t.Fatalf("player after end = %v", p)
	}
}

func TestPauseReplaceStop(t *testing.T) {
	h := newHarness(t)
	c := h.join()
	h.patch(map[string]any{"track": map[string]any{"encoded": encoded("long")}})
	h.wantOpen("long@0 v100")
	h.next("TrackStartEvent")
	<-c.frames

	if p := h.patch(map[string]any{"paused": true}); p["paused"] != true {
		t.Fatalf("pause = %v", p)
	}
	for on := range c.speaking {
		if !on {
			break
		}
	}
	time.Sleep(60 * time.Millisecond)
	for len(c.frames) > 0 {
		<-c.frames
	}
	time.Sleep(60 * time.Millisecond)
	if n := len(c.frames); n != 0 {
		t.Fatalf("%d frames sent while paused", n)
	}
	h.patch(map[string]any{"paused": false})
	<-c.frames

	h.patch(map[string]any{"encodedTrack": encoded("other")}) // deprecated field still works
	if end := h.next("TrackEndEvent"); end["reason"] != "replaced" || end["track"].(map[string]any)["info"].(map[string]any)["identifier"] != "long" {
		t.Fatalf("replace end = %v", end)
	}
	h.wantOpen("other@0 v100")
	h.next("TrackStartEvent")

	h.patch(map[string]any{"track": map[string]any{"encoded": encoded("x")}}) // noReplace keeps "other"
	code, _ := h.do("PATCH", "/v4/sessions/"+h.sid+"/players/42?noReplace=true", map[string]any{"track": map[string]any{"encoded": encoded("y")}})
	if code != 200 {
		t.Fatal(code)
	}
	h.next("TrackEndEvent")
	h.wantOpen("x@0 v100")
	h.next("TrackStartEvent")

	h.patch(map[string]any{"track": map[string]any{"encoded": nil}})
	if end := h.next("TrackEndEvent"); end["reason"] != "stopped" {
		t.Fatalf("stop = %v", end)
	}
	if len(h.src.opens) != 0 {
		t.Fatalf("unexpected open %q", <-h.src.opens)
	}
}

func TestSeekAndVolumeRestart(t *testing.T) {
	h := newHarness(t)
	h.join()
	h.patch(map[string]any{"track": map[string]any{"encoded": encoded("long")}, "position": 1000})
	h.wantOpen("long@1000 v100")
	h.next("TrackStartEvent")

	h.patch(map[string]any{"position": 5000})
	h.wantOpen("long@5000 v100")
	time.Sleep(100 * time.Millisecond)
	h.patch(map[string]any{"volume": 50})
	select {
	case got := <-h.src.opens:
		var pos int
		if _, err := fmt.Sscanf(got, "long@%d v50", &pos); err != nil || pos <= 5000 {
			t.Fatalf("volume restart = %q, want position past 5000 and volume 50", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("volume change did not restart the stream")
	}
	_, p := h.do("GET", "/v4/sessions/"+h.sid+"/players/42", nil)
	if p["volume"] != 50.0 || p["state"].(map[string]any)["position"].(float64) <= 5000 {
		t.Fatalf("player = %v", p)
	}
	h.patch(map[string]any{"track": map[string]any{"encoded": nil}})
	h.next("TrackEndEvent") // and no TrackStart/End came from the restarts
}

func TestLoadFailedAndStuck(t *testing.T) {
	h := newHarness(t)
	h.join()
	h.patch(map[string]any{"track": map[string]any{"encoded": encoded("fail")}})
	if ex := h.next("TrackExceptionEvent"); ex["exception"].(map[string]any)["message"] != "cannot open" {
		t.Fatalf("exception = %v", ex)
	}
	if end := h.next("TrackEndEvent"); end["reason"] != "loadFailed" {
		t.Fatalf("end = %v", end)
	}

	h.patch(map[string]any{"track": map[string]any{"encoded": encoded("stuck")}})
	h.next("TrackStartEvent")
	if st := h.next("TrackStuckEvent"); st["thresholdMs"] != 100.0 {
		t.Fatalf("stuck = %v", st)
	}

	if code, _ := h.do("PATCH", "/v4/sessions/"+h.sid+"/players/42", map[string]any{"track": map[string]any{"identifier": "bad"}}); code != 400 {
		t.Fatalf("bad identifier = %d", code)
	}
}

func TestSearch(t *testing.T) {
	h := newHarness(t)
	_, res := h.do("GET", "/v4/loadtracks?identifier=find:song", nil)
	tracks, _ := res["data"].([]any)
	if res["loadType"] != "search" || len(tracks) != 2 || tracks[1].(map[string]any)["info"].(map[string]any)["title"] != "song-2" {
		t.Fatalf("search = %v", res)
	}
	if _, res := h.do("GET", "/v4/loadtracks?identifier=find:none", nil); res["loadType"] != "empty" {
		t.Fatalf("no results = %v", res)
	}
	if _, res := h.do("GET", "/v4/loadtracks?identifier=find:broken", nil); res["loadType"] != "error" {
		t.Fatalf("failed search = %v", res)
	}

	// A search given to a player plays its first result.
	h.patch(map[string]any{"track": map[string]any{"identifier": "find:song"}})
	h.wantOpen("song-1@0 v100")
	if code, _ := h.do("PATCH", "/v4/sessions/"+h.sid+"/players/42", map[string]any{"track": map[string]any{"identifier": "find:none"}}); code != 400 {
		t.Fatalf("search without results = %d", code)
	}
}

func TestVoiceClosedAndDestroy(t *testing.T) {
	h := newHarness(t)
	c := h.join()
	c.closeWith(&voice.CloseError{Code: 4014, Reason: "disconnected"})
	if ev := h.next("WebSocketClosedEvent"); ev["code"] != 4014.0 || ev["byRemote"] != true {
		t.Fatalf("closed = %v", ev)
	}

	h.dialFn = func() (VoiceConn, error) { return nil, errors.New("dial failed") }
	h.patch(map[string]any{"voice": map[string]any{"token": "t2", "endpoint": "e2", "sessionId": "s2", "channelId": "98"}})
	<-h.dials
	if ev := h.next("WebSocketClosedEvent"); ev["reason"] != "dial failed" || ev["byRemote"] != false {
		t.Fatalf("dial failure = %v", ev)
	}

	if code, _ := h.do("PATCH", "/v4/sessions/"+h.sid+"/players/42", map[string]any{"voice": map[string]any{"token": "t"}}); code != 400 {
		t.Fatalf("partial voice = %d", code)
	}

	h.dialFn = func() (VoiceConn, error) { c := newFakeConn(); h.conns <- c; return c, nil }
	c = h.join()
	if code, _ := h.do("DELETE", "/v4/sessions/"+h.sid+"/players/42", nil); code != 204 {
		t.Fatalf("delete = %d", code)
	}
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
		t.Fatal("voice not closed on destroy")
	}
	if code, _ := h.do("GET", "/v4/sessions/"+h.sid+"/players/42", nil); code != 404 {
		t.Fatalf("player after delete = %d", code)
	}
}
