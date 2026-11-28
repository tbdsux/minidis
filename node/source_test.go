package node

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFFmpegInput(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(dir, "song.mp3"), nil, 0o644)
	outside := filepath.Join(t.TempDir(), "secret.mp3")
	os.WriteFile(outside, nil, 0o644)
	os.Symlink(outside, filepath.Join(dir, "link.mp3"))
	f := &FFmpeg{LocalDir: dir}

	ok := map[string]string{
		"song.mp3":                      "file:" + filepath.Join(dir, "song.mp3"),
		filepath.Join(dir, "song.mp3"):  "file:" + filepath.Join(dir, "song.mp3"),
		"https://example.com/a.mp3?x=1": "https://example.com/a.mp3?x=1",
	}
	for id, want := range ok {
		if got, _, err := f.input(id); err != nil || got != want {
			t.Errorf("input(%q) = %q, %v; want %q", id, got, err, want)
		}
	}
	for _, id := range []string{"../" + filepath.Base(filepath.Dir(outside)) + "/secret.mp3", outside, "link.mp3",
		"ftp://example.com/a.mp3", "file:///etc/passwd", "concat:song.mp3", "-i", "http:///nohost"} {
		if got, _, err := f.input(id); err == nil {
			t.Errorf("input(%q) = %q, want rejected", id, got)
		}
	}
	if _, _, err := (&FFmpeg{}).input("song.mp3"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("local file with no LocalDir = %v", err)
	}
}

func TestFFmpegArgs(t *testing.T) {
	args := ffmpegArgs("file:/m/a.mp3", "local", 1500*time.Millisecond, 50)
	joined := strings.Join(args, " ")
	for _, want := range []string{"-protocol_whitelist file ", "-ss 1.500 -i file:/m/a.mp3", "-af volume=0.50", "-frame_duration 20", "-f ogg"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
	if got := ffmpegArgs("https://x/a", "http", 0, 100); slices.Contains(got, "-ss") || slices.Contains(got, "-af") ||
		!slices.Contains(got, "http,https,tcp,tls,crypto") {
		t.Errorf("http args = %v", got)
	}
}

// TestFFmpegIntegration plays a generated 1s tone from disk and over HTTP.
func TestFFmpegIntegration(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	tone := filepath.Join(dir, "tone.wav")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-metadata", "title=Tone", tone).CombinedOutput(); err != nil {
		t.Fatalf("make tone: %v %s", err, out)
	}
	web := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer web.Close()

	f := &FFmpeg{LocalDir: dir}
	ctx := context.Background()
	for _, id := range []string{"tone.wav", web.URL + "/tone.wav"} {
		t.Run(id, func(t *testing.T) {
			info, err := f.Load(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if info.Length < 900 || info.Length > 1100 || !info.IsSeekable || info.IsStream {
				t.Fatalf("info = %+v", info)
			}
			for _, tc := range []struct {
				pos      time.Duration
				min, max int
			}{{0, 48, 52}, {500 * time.Millisecond, 23, 27}} {
				fr, err := f.Open(ctx, info, tc.pos, 80)
				if err != nil {
					t.Fatal(err)
				}
				n := 0
				for {
					frame, err := fr.ReadFrame()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					if len(frame) == 0 || len(frame) > 1275 {
						t.Fatalf("frame size %d", len(frame))
					}
					n++
				}
				fr.Close()
				if n < tc.min || n > tc.max {
					t.Errorf("from %v: %d frames, want %d-%d", tc.pos, n, tc.min, tc.max)
				}
			}
		})
	}

	if _, err := f.Load(ctx, web.URL+"/missing.wav"); err == nil {
		t.Error("missing HTTP file loaded")
	}
}
