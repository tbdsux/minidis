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

func TestYouTubeInput(t *testing.T) {
	f := &FFmpeg{}
	for id, want := range map[string]string{
		"https://www.youtube.com/watch?v=abc": "youtube",
		"https://youtu.be/abc":                "youtube",
		"https://music.youtube.com/watch?v=x": "youtube",
		"https://youtube.com.example/v":       "http",
		"https://example.com/youtube.com":     "http",
	} {
		if _, got, err := f.input(id); err != nil || got != want {
			t.Errorf("input(%q) source = %q, %v; want %q", id, got, err, want)
		}
	}
	args := strings.Join(ffmpegArgs("pipe:0", "youtube", 0, 100), " ")
	if !strings.Contains(args, "-protocol_whitelist pipe -i pipe:0") {
		t.Errorf("youtube ffmpeg args = %s", args)
	}
	if got := ytdlpArgs("-x", "-J"); got[len(got)-2] != "--" || got[len(got)-1] != "-x" {
		t.Errorf("yt-dlp args = %v, want the URL after --", got)
	}
}

// TestYouTubePipeline runs real ffmpeg behind a fake yt-dlp that prints
// metadata for -J and streams a tone file for -o -.
func TestYouTubePipeline(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	tone := filepath.Join(dir, "tone.webm")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "sine=duration=1", "-c:a", "libopus", tone).CombinedOutput(); err != nil {
		t.Fatalf("make tone: %v %s", err, out)
	}
	script := filepath.Join(dir, "yt-dlp")
	os.WriteFile(script, []byte(`#!/bin/sh
for a; do url=$a; done
case "$url" in *gone*) echo "ERROR: Video unavailable" >&2; exit 1;; esac
case " $* " in
*" --dump-single-json "*)
  case "$url" in *list=*) echo '{"_type":"playlist","entries":[]}';;
  *) echo '{"title":"Tone","channel":"Test Channel","duration":1.0,"webpage_url":"https://www.youtube.com/watch?v=abc","thumbnail":"https://i.ytimg.com/a.jpg"}';; esac;;
*" -o - "*) cat "`+tone+`";;
esac
`), 0o755)
	f := &FFmpeg{YTDLP: script}
	ctx := context.Background()

	info, err := f.Load(ctx, "https://youtu.be/abc")
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "Tone" || info.Author != "Test Channel" || info.Length != 1000 || !info.IsSeekable ||
		info.SourceName != "youtube" || *info.URI != "https://www.youtube.com/watch?v=abc" || *info.ArtworkURL != "https://i.ytimg.com/a.jpg" {
		t.Fatalf("info = %+v", info)
	}
	fr, err := f.Open(ctx, info, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for {
		if _, err := fr.ReadFrame(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		n++
	}
	fr.Close()
	if n < 48 || n > 52 {
		t.Fatalf("%d frames, want about 50", n)
	}

	if _, err := f.Load(ctx, "https://www.youtube.com/playlist?list=PL1"); err == nil || !strings.Contains(err.Error(), "playlists") {
		t.Errorf("playlist = %v", err)
	}
	if _, err := f.Load(ctx, "https://youtu.be/gone"); err == nil || !strings.Contains(err.Error(), "Video unavailable") {
		t.Errorf("unavailable load = %v", err)
	}
	fr, err = f.Open(ctx, TrackInfo{Identifier: "https://youtu.be/gone"}, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer fr.Close()
	if _, err := fr.ReadFrame(); err == nil || !strings.Contains(err.Error(), "yt-dlp") || !strings.Contains(err.Error(), "Video unavailable") {
		t.Errorf("unavailable play = %v, want yt-dlp's error", err)
	}
}
