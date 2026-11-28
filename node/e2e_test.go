package node

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tbdsux/minidis/lavalink"
	"github.com/tbdsux/minidis/voice"
)

// TestClientEndToEnd drives the node with the minidis lavalink client and
// real ffmpeg; only the Discord voice connection is faked.
func TestClientEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=duration=0.5",
		filepath.Join(dir, "tone.mp3")).CombinedOutput(); err != nil {
		t.Fatalf("make tone: %v %s", err, out)
	}
	web := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer web.Close()

	conns := make(chan *fakeConn, 1)
	srv := httptest.NewServer(New(Config{
		Password: "pw", Source: &FFmpeg{LocalDir: dir}, Logger: slog.New(slog.DiscardHandler),
		Dial: func(_ context.Context, p voice.Params) (VoiceConn, error) {
			c := newFakeConn()
			conns <- c
			return c, nil
		},
	}))
	defer srv.Close()

	ll := lavalink.New(srv.URL, "pw")
	defer ll.Close()
	events := make(chan *lavalink.Event, 10)
	ll.OnEvent(func(e *lavalink.Event) { events <- e })
	ctx := context.Background()
	if err := ll.Connect(ctx, "555"); err != nil {
		t.Fatal(err)
	}
	// What Attach forwards once Discord's voice events arrive.
	if _, err := ll.UpdatePlayer(ctx, "42", &lavalink.UpdatePlayer{Voice: &lavalink.VoiceState{
		Token: "t", Endpoint: "e", SessionID: "s", ChannelID: "9"}}, false); err != nil {
		t.Fatal(err)
	}
	conn := <-conns

	for _, id := range []string{"tone.mp3", web.URL + "/tone.mp3"} {
		track, err := ll.LoadTrack(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if err := ll.Play(ctx, "42", track); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{lavalink.TrackStartEvent, lavalink.TrackEndEvent} {
			select {
			case e := <-events:
				if e.Type != want || e.Track.Info.Identifier != id {
					t.Fatalf("%s: event = %+v, want %s", id, e, want)
				}
				if want == lavalink.TrackEndEvent && !e.MayStartNext() {
					t.Fatalf("%s: end reason %q", id, e.Reason)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("%s: no %s", id, want)
			}
		}
		n := 0
		for len(conn.frames) > 0 {
			<-conn.frames
			n++
		}
		if n < 25 || n > 32 { // ~25 frames of audio, then 5 of silence
			t.Fatalf("%s: %d frames sent", id, n)
		}
	}

	if _, err := ll.LoadTrack(ctx, "/etc/hosts"); err == nil {
		t.Fatal("file outside the local directory loaded")
	}
}
