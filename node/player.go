package node

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tbdsux/minidis/voice"
)

// VoiceConn is what a player needs from a voice connection; *voice.Conn
// implements it.
type VoiceConn interface {
	WriteOpus(frame []byte) error
	Speaking(on bool) error
	Ping() int64
	Done() <-chan struct{}
	Err() error
	Close() error
}

const frameDuration = 20 * time.Millisecond

// player plays one guild's audio. opMu serializes client updates; mu guards
// the state shared with the stream goroutine. Events are sent while holding
// mu so they come out in order.
type player struct {
	s       *session
	guildID string

	opMu sync.Mutex

	mu        sync.Mutex
	track     *Track
	volume    int
	paused    bool
	voice     VoiceState
	conn      VoiceConn
	stream    *stream
	destroyed bool
}

// stream is one ffmpeg run of the current track from start. Seeking or
// changing volume replaces it without track events.
type stream struct {
	cancel context.CancelFunc
	done   chan struct{}
	start  time.Duration
	sent   atomic.Int64 // frames written
}

func (st *stream) position() time.Duration {
	return st.start + time.Duration(st.sent.Load())*frameDuration
}

func newPlayer(s *session, guildID string) *player {
	return &player{s: s, guildID: guildID, volume: 100}
}

func (p *player) info() PlayerInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PlayerInfo{GuildID: p.guildID, Track: p.trackWithPositionLocked(), Volume: p.volume,
		Paused: p.paused, State: p.stateLocked(), Voice: p.voice, Filters: emptyObject}
}

func (p *player) trackWithPositionLocked() *Track {
	if p.track == nil {
		return nil
	}
	t := *p.track
	if p.stream != nil {
		t.Info.Position = p.stream.position().Milliseconds()
	}
	return &t
}

func (p *player) stateLocked() PlayerState {
	st := PlayerState{Time: time.Now().UnixMilli(), Connected: p.conn != nil, Ping: -1}
	if p.conn != nil {
		st.Ping = p.conn.Ping()
	}
	if p.stream != nil {
		st.Position = p.stream.position().Milliseconds()
	}
	return st
}

// update applies a PATCH. track is the resolved new track, or nil with
// stop true for an explicit null.
func (p *player) update(u *UpdatePlayer, track *Track, stop, noReplace bool) {
	p.opMu.Lock()
	defer p.opMu.Unlock()

	if u.Voice != nil {
		p.setVoice(*u.Voice)
	}
	p.mu.Lock()
	if u.Paused != nil {
		p.paused = *u.Paused
	}
	volumeChanged := u.Volume != nil && *u.Volume != p.volume
	if u.Volume != nil {
		p.volume = min(max(*u.Volume, 0), 1000)
	}
	playing := p.track != nil
	p.mu.Unlock()

	switch {
	case stop:
		p.replace(nil, 0, "stopped")
	case track != nil && !(noReplace && playing):
		var pos time.Duration
		if u.Position != nil {
			pos = time.Duration(*u.Position) * time.Millisecond
		}
		p.replace(track, pos, "replaced")
	case u.Position != nil:
		p.restart(time.Duration(*u.Position) * time.Millisecond)
	case volumeChanged:
		p.restart(-1)
	}
}

// detachStream stops the current stream and waits for it. Call without mu.
func (p *player) detachStream() *stream {
	p.mu.Lock()
	st := p.stream
	p.stream = nil
	p.mu.Unlock()
	if st != nil {
		st.cancel()
		<-st.done
	}
	return st
}

// replace ends the current track with reason and starts t (nil to stop).
func (p *player) replace(t *Track, pos time.Duration, reason string) {
	p.detachStream()
	p.mu.Lock()
	defer p.mu.Unlock()
	if old := p.track; old != nil {
		p.s.send(event("TrackEndEvent", p.guildID, map[string]any{"track": old, "reason": reason}))
	}
	p.track = t
	if t != nil && !p.destroyed {
		p.startLocked(pos, true)
	}
}

// restart reopens the current track at pos (-1 for the current position).
func (p *player) restart(pos time.Duration) {
	st := p.detachStream()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.track == nil || p.destroyed {
		return
	}
	if pos < 0 {
		pos = 0
		if st != nil {
			pos = st.position()
		}
	}
	p.startLocked(pos, false)
}

func (p *player) startLocked(pos time.Duration, announce bool) {
	ctx, cancel := context.WithCancel(p.s.ctx)
	st := &stream{cancel: cancel, done: make(chan struct{}), start: pos}
	p.stream = st
	go p.run(ctx, st, p.track, p.volume, announce)
}

// end finishes st's track if st is still current: err nil means finished.
func (p *player) end(st *stream, t *Track, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream != st {
		return
	}
	p.stream = nil
	p.track = nil
	reason := "finished"
	if err != nil {
		reason = "loadFailed"
		p.s.send(event("TrackExceptionEvent", p.guildID, map[string]any{"track": t,
			"exception": Exception{Message: err.Error(), Severity: "fault", Cause: err.Error()}}))
	}
	p.s.send(event("TrackEndEvent", p.guildID, map[string]any{"track": t, "reason": reason}))
}

// run streams frames to the voice connection every 20ms. It holds frames
// while there's no connection, the player is paused, or DAVE isn't ready.
func (p *player) run(ctx context.Context, st *stream, t *Track, volume int, announce bool) {
	defer close(st.done)
	fr, err := p.s.node.cfg.Source.Open(ctx, t.Info, st.start, volume)
	if err != nil {
		if ctx.Err() == nil {
			p.end(st, t, err)
		}
		return
	}
	defer fr.Close()

	frames := make(chan []byte, 50)
	var readErr error
	go func() {
		defer close(frames)
		for {
			f, err := fr.ReadFrame()
			if err != nil {
				if err != io.EOF {
					readErr = err
				}
				return
			}
			select {
			case frames <- f:
			case <-ctx.Done():
				return
			}
		}
	}()

	if announce {
		p.mu.Lock()
		if p.stream == st {
			p.s.send(event("TrackStartEvent", p.guildID, map[string]any{"track": t}))
		}
		p.mu.Unlock()
	}

	var (
		conn     VoiceConn
		speaking bool
		pending  []byte
		last     = time.Now()
		stuck    bool
	)
	quiet := func() {
		if speaking && conn != nil {
			for range 5 {
				conn.WriteOpus(voice.SilenceFrame)
			}
			conn.Speaking(false)
		}
		speaking = false
	}
	defer func() { quiet() }()

	threshold := p.s.node.cfg.StuckThreshold
	tick := time.NewTicker(frameDuration)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		p.mu.Lock()
		c, paused := p.conn, p.paused
		p.mu.Unlock()
		if c != conn {
			conn, speaking = c, false
		}
		if conn == nil || paused {
			quiet()
			last = time.Now()
			continue
		}
		if pending == nil {
			select {
			case f, ok := <-frames:
				if !ok {
					p.end(st, t, readErr)
					return
				}
				pending = f
			default:
				if !stuck && time.Since(last) > threshold {
					stuck = true
					p.mu.Lock()
					if p.stream == st {
						p.s.send(event("TrackStuckEvent", p.guildID, map[string]any{"track": t, "thresholdMs": threshold.Milliseconds()}))
					}
					p.mu.Unlock()
				}
				continue
			}
		}
		if !speaking {
			conn.Speaking(true)
			speaking = true
		}
		if err := conn.WriteOpus(pending); errors.Is(err, voice.ErrNotReady) {
			continue
		}
		pending = nil
		last = time.Now()
		stuck = false
		st.sent.Add(1)
	}
}

// setVoice connects to a new voice server when the state changed. The dial
// runs in the background; failures become WebSocketClosedEvents.
func (p *player) setVoice(v VoiceState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if v == p.voice && p.conn != nil {
		return
	}
	p.voice = v
	if old := p.conn; old != nil {
		p.conn = nil // cleared first so its watcher sends no close event
		old.Close()
	}
	params := voice.Params{GuildID: p.guildID, ChannelID: v.ChannelID, UserID: p.s.userID,
		SessionID: v.SessionID, Token: v.Token, Endpoint: v.Endpoint, Logger: p.s.node.log}
	go func() {
		ctx, cancel := context.WithTimeout(p.s.ctx, 20*time.Second)
		defer cancel()
		c, err := p.s.node.cfg.Dial(ctx, params)
		p.mu.Lock()
		defer p.mu.Unlock()
		if err != nil {
			if p.voice == v && !p.destroyed {
				p.closedLocked(err, false)
			}
			return
		}
		if p.voice != v || p.destroyed || p.conn != nil {
			c.Close()
			return
		}
		p.conn = c
		go func() {
			<-c.Done()
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.conn == c {
				p.conn = nil
				p.closedLocked(c.Err(), true)
			}
		}()
	}()
}

func (p *player) closedLocked(err error, byRemote bool) {
	code, reason := 4000, ""
	if err != nil {
		reason = err.Error()
	}
	if ce, ok := errors.AsType[*voice.CloseError](err); ok {
		code, reason = ce.Code, ce.Reason
	}
	p.s.send(event("WebSocketClosedEvent", p.guildID, map[string]any{"code": code, "reason": reason, "byRemote": byRemote}))
}

// destroy stops playback and leaves voice without sending events.
func (p *player) destroy() {
	p.opMu.Lock()
	defer p.opMu.Unlock()
	p.mu.Lock()
	p.destroyed = true
	conn := p.conn
	p.conn = nil
	p.mu.Unlock()
	p.detachStream()
	if conn != nil {
		conn.Close()
	}
}
