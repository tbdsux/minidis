package node

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
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

// FFmpeg plays http(s) URLs and local files through ffmpeg and ffprobe.
type FFmpeg struct {
	// LocalDir is the only directory local files may be played from.
	// Empty disables local files.
	LocalDir string
	// FFmpeg and FFprobe are the binaries; empty means "ffmpeg" / "ffprobe".
	FFmpeg, FFprobe string
}

// whitelist limits the protocols ffmpeg may open for each source, so an
// HTTP playlist can't reach local files and a local file can't reach the network.
func whitelist(sourceName string) string {
	if sourceName == "local" {
		return "file"
	}
	return "http,https,tcp,tls,crypto"
}

// input maps an identifier to an ffmpeg input, or rejects it. Local paths
// are pinned with the "file:" protocol so ffmpeg never reads them as
// another protocol or option.
func (f *FFmpeg) input(id string) (in, sourceName string, err error) {
	if u, err := url.Parse(id); err == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") {
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
	cmd := exec.CommandContext(ctx, f.bin("ffmpeg", f.FFmpeg), ffmpegArgs(in, sourceName, position, volume)...)
	s := &ffStream{cmd: cmd}
	cmd.Stderr = &s.stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start ffmpeg: %w", err)
	}
	s.ogg = newOggReader(out)
	return s, nil
}

type ffStream struct {
	cmd     *exec.Cmd
	ogg     *oggReader
	stderr  bytes.Buffer
	once    sync.Once
	waitErr error
}

func (s *ffStream) wait() error {
	s.once.Do(func() { s.waitErr = s.cmd.Wait() })
	return s.waitErr
}

func (s *ffStream) ReadFrame() ([]byte, error) {
	pkt, err := s.ogg.Packet()
	if err == nil {
		return pkt, nil
	}
	if werr := s.wait(); werr != nil {
		return nil, fmt.Errorf("ffmpeg: %v: %s", werr, bytes.TrimSpace(s.stderr.Bytes()))
	}
	if err == io.EOF {
		return nil, io.EOF
	}
	return nil, err
}

func (s *ffStream) Close() error {
	s.cmd.Process.Kill()
	s.wait()
	return nil
}
