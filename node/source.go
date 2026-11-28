package node

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Source resolves identifiers into tracks and streams them as Opus frames.
// Open must validate info.Identifier itself: encoded tracks come from clients.
type Source interface {
	Load(ctx context.Context, identifier string) (TrackInfo, error)
	Open(ctx context.Context, info TrackInfo, position time.Duration, volume int) (FrameReader, error)
}

// FrameReader yields 20ms Opus frames until io.EOF.
type FrameReader interface {
	ReadFrame() ([]byte, error)
	Close() error
}

// ErrUnsupported is returned by Load for identifiers no source handles.
var ErrUnsupported = errors.New("unsupported identifier: want an http(s) URL or a file under the local directory")

// FFmpeg plays http(s) URLs and local files through ffmpeg and ffprobe, and
// YouTube links through yt-dlp piped into ffmpeg.
type FFmpeg struct {
	// LocalDir is the only directory local files may be played from.
	// Empty disables local files.
	LocalDir string
	// FFmpeg, FFprobe and YTDLP are the binaries; empty means "ffmpeg",
	// "ffprobe" and "yt-dlp". yt-dlp is only needed for YouTube links.
	FFmpeg, FFprobe, YTDLP string
}

// youTubeHosts are the hosts whose links play through yt-dlp.
var youTubeHosts = map[string]bool{"youtube.com": true, "www.youtube.com": true, "m.youtube.com": true,
	"music.youtube.com": true, "youtu.be": true}

// whitelist limits the protocols ffmpeg may open for each source, so an
// HTTP playlist can't reach local files and a local file can't reach the network.
func whitelist(sourceName string) string {
	switch sourceName {
	case "local":
		return "file"
	case "youtube":
		return "pipe" // yt-dlp does the network part
	}
	return "http,https,tcp,tls,crypto"
}

// input maps an identifier to an ffmpeg input, or rejects it. Local paths
// are pinned with the "file:" protocol so ffmpeg never reads them as
// another protocol or option.
func (f *FFmpeg) input(id string) (in, sourceName string, err error) {
	if u, err := url.Parse(id); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
		if youTubeHosts[strings.ToLower(u.Hostname())] {
			return id, "youtube", nil
		}
		return id, "http", nil
	}
	if f.LocalDir == "" || strings.Contains(id, "://") {
		return "", "", ErrUnsupported
	}
	root, err := filepath.EvalSymlinks(f.LocalDir)
	if err != nil {
		return "", "", fmt.Errorf("local directory: %w", err)
	}
	if root, err = filepath.Abs(root); err != nil {
		return "", "", err
	}
	p := id
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	// shortcut: a symlink swapped in after this check is followed; fine while LocalDir is trusted.
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", "", fmt.Errorf("file not found: %s", id)
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", ErrUnsupported
	}
	return "file:" + real, "local", nil
}

func (f *FFmpeg) bin(name, override string) string {
	if override != "" {
		return override
	}
	return name
}

func (f *FFmpeg) Load(ctx context.Context, id string) (TrackInfo, error) {
	in, sourceName, err := f.input(id)
	if err != nil {
		return TrackInfo{}, err
	}
	if sourceName == "youtube" {
		return f.loadYouTube(ctx, id)
	}
	out, err := exec.CommandContext(ctx, f.bin("ffprobe", f.FFprobe), "-v", "error",
		"-protocol_whitelist", whitelist(sourceName),
		"-show_entries", "format=duration:format_tags=title,artist,TITLE,ARTIST",
		"-of", "json", in).Output()
	if err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			return TrackInfo{}, fmt.Errorf("ffprobe: %s", bytes.TrimSpace(ee.Stderr))
		}
		return TrackInfo{}, err
	}
	var probe struct {
		Format struct {
			Duration string            `json:"duration"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return TrackInfo{}, fmt.Errorf("ffprobe: %w", err)
	}
	tags := map[string]string{}
	for k, v := range probe.Format.Tags {
		tags[strings.ToLower(k)] = v
	}
	info := TrackInfo{
		Identifier: id,
		Author:     tags["artist"],
		Title:      tags["title"],
		URI:        &id,
		SourceName: sourceName,
	}
	if info.Author == "" {
		info.Author = "Unknown artist"
	}
	if info.Title == "" {
		info.Title = path.Base(strings.SplitN(id, "?", 2)[0])
	}
	if secs, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil && secs > 0 {
		info.Length = int64(secs * 1000)
		info.IsSeekable = true
	} else {
		info.IsStream = true
	}
	return info, nil
}

// ytdlpArgs are the options every yt-dlp run gets: no user config, one
// video even from a playlist link, and the URL after "--" so it is never
// read as an option.
func ytdlpArgs(id string, args ...string) []string {
	return append(append([]string{"--ignore-config", "--no-playlist", "--no-warnings"}, args...), "--", id)
}

func (f *FFmpeg) loadYouTube(ctx context.Context, id string) (TrackInfo, error) {
	cmd := exec.CommandContext(ctx, f.bin("yt-dlp", f.YTDLP), ytdlpArgs(id, "--dump-single-json")...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			return TrackInfo{}, fmt.Errorf("yt-dlp: %s", bytes.TrimSpace(ee.Stderr))
		}
		return TrackInfo{}, fmt.Errorf("yt-dlp: %w", err)
	}
	var v struct {
		Type       string  `json:"_type"`
		Title      string  `json:"title"`
		Uploader   string  `json:"uploader"`
		Channel    string  `json:"channel"`
		Duration   float64 `json:"duration"`
		IsLive     bool    `json:"is_live"`
		WebpageURL string  `json:"webpage_url"`
		Thumbnail  string  `json:"thumbnail"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return TrackInfo{}, fmt.Errorf("yt-dlp: %w", err)
	}
	if v.Type == "playlist" {
		return TrackInfo{}, errors.New("playlists are not supported; link one video")
	}
	info := TrackInfo{Identifier: id, Title: v.Title, Author: cmp.Or(v.Channel, v.Uploader, "Unknown artist"),
		URI: &id, SourceName: "youtube"}
	if v.WebpageURL != "" {
		info.URI = &v.WebpageURL
	}
	if v.Thumbnail != "" {
		info.ArtworkURL = &v.Thumbnail
	}
	if v.IsLive || v.Duration <= 0 {
		info.IsStream = true
	} else {
		info.Length = int64(v.Duration * 1000)
		info.IsSeekable = true
	}
	return info, nil
}

// ffmpegArgs builds the command line that turns in into 20ms Ogg/Opus pages.
func ffmpegArgs(in, sourceName string, position time.Duration, volume int) []string {
	args := []string{"-nostdin", "-v", "error", "-protocol_whitelist", whitelist(sourceName)}
	if position > 0 {
		args = append(args, "-ss", strconv.FormatFloat(position.Seconds(), 'f', 3, 64))
	}
	args = append(args, "-i", in, "-vn", "-map", "0:a:0")
	if volume != 100 {
		args = append(args, "-af", "volume="+strconv.FormatFloat(float64(volume)/100, 'f', 2, 64))
	}
	return append(args, "-c:a", "libopus", "-b:a", "128k", "-ar", "48000", "-ac", "2",
		"-frame_duration", "20", "-application", "audio", "-f", "ogg", "-page_duration", "20000", "pipe:1")
}

func (f *FFmpeg) Open(ctx context.Context, info TrackInfo, position time.Duration, volume int) (FrameReader, error) {
	in, sourceName, err := f.input(info.Identifier)
	if err != nil {
		return nil, err
	}
	if sourceName == "youtube" {
		in = "pipe:0"
	}
	cmd := exec.CommandContext(ctx, f.bin("ffmpeg", f.FFmpeg), ffmpegArgs(in, sourceName, position, volume)...)
	s := &ffStream{cmd: cmd}
	cmd.Stderr = &s.stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if sourceName == "youtube" {
		// shortcut: seeking decodes from the start of the piped audio; fine for songs, slow deep into long videos.
		s.dl = exec.CommandContext(ctx, f.bin("yt-dlp", f.YTDLP),
			ytdlpArgs(info.Identifier, "--quiet", "-f", "bestaudio/best", "-o", "-")...)
		s.dl.Stderr = &s.dlStderr
		r, w, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		s.dl.Stdout, cmd.Stdin = w, r
		err = s.dl.Start()
		w.Close()
		if err != nil {
			r.Close()
			return nil, fmt.Errorf("start yt-dlp: %w", err)
		}
		defer r.Close() // ffmpeg has its own copy once started
	}

	if err := cmd.Start(); err != nil {
		s.Close()
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	s.ogg = newOggReader(out)
	return s, nil
}

// ffStream is a running ffmpeg, fed by yt-dlp (dl) for YouTube.
type ffStream struct {
	cmd      *exec.Cmd
	dl       *exec.Cmd
	ogg      *oggReader
	stderr   bytes.Buffer
	dlStderr bytes.Buffer
	once     sync.Once
	waitErr  error
}

// wait reaps both processes. yt-dlp's error comes first: when the download
// fails, ffmpeg only sees its input end.
func (s *ffStream) wait() error {
	s.once.Do(func() {
		var ffErr error
		if s.cmd.Process != nil {
			ffErr = s.cmd.Wait()
		}
		if s.dl != nil {
			if err := s.dl.Wait(); err != nil && s.dlStderr.Len() > 0 {
				s.waitErr = fmt.Errorf("yt-dlp: %v: %s", err, bytes.TrimSpace(s.dlStderr.Bytes()))
				return
			}
		}
		if ffErr != nil {
			s.waitErr = fmt.Errorf("ffmpeg: %v: %s", ffErr, bytes.TrimSpace(s.stderr.Bytes()))
		}
	})
	return s.waitErr
}

func (s *ffStream) ReadFrame() ([]byte, error) {
	pkt, err := s.ogg.Packet()
	if err == nil {
		return pkt, nil
	}
	if werr := s.wait(); werr != nil {
		return nil, werr
	}
	if err == io.EOF {
		return nil, io.EOF
	}
	return nil, err
}

func (s *ffStream) Close() error {
	for _, c := range []*exec.Cmd{s.cmd, s.dl} {
		if c != nil && c.Process != nil {
			c.Process.Kill()
		}
	}
	s.wait()
	return nil
}
