// Package lavalink is a client for audio nodes that speak the Lavalink v4
// protocol: the minidis node (github.com/tbdsux/minidis/node) or Lavalink
// itself. The node joins voice channels and plays audio, so none of that
// runs in the bot.
//
//	ll := lavalink.New("http://localhost:2333", "password")
//	ll.Attach(bot.Session) // before the bot connects
//	...
//	ll.Join(guildID, channelID)
//	track, err := ll.LoadTrack(ctx, "https://example.com/song.mp3")
//	err = ll.Play(ctx, guildID, track)
//
// Attach forwards the bot's voice events to the node and opens the node's
// WebSocket when the bot is ready. When the WebSocket drops, the client
// reconnects and resumes its session, so the node's players play on. The
// client keeps no queue: play the next track from OnEvent when an Event says
// MayStartNext.
package lavalink

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

// ErrNoMatches is returned by LoadTrack when the node found nothing.
var ErrNoMatches = errors.New("lavalink: no matches")

// ErrNotConnected is returned before the node's WebSocket is open.
var ErrNotConnected = errors.New("lavalink: not connected to the node")

// Error is an error response from the node's REST API.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("lavalink: %d: %s", e.Status, e.Message) }

// resumeTimeout is how long the node keeps the players after the WebSocket
// drops, waiting for the client to reconnect.
const resumeTimeout = 60 * time.Second

// pingPeriod and pongWait catch a node that stopped answering without the
// WebSocket failing, as after a network change: the client pings it, and
// reconnects when neither a pong nor a message came for pongWait. Lavalink
// sends stats every minute, so pongWait stays longer than that even for a
// node that doesn't answer pings.
const (
	pingPeriod = 20 * time.Second
	pongWait   = 70 * time.Second
)

// Client talks to one node. Set its fields before using it.
type Client struct {
	URL        string // the node's base URL, such as http://localhost:2333
	Password   string
	HTTPClient *http.Client
	Logger     *slog.Logger

	connectMu sync.Mutex // serializes Connect

	// pingPeriod and pongWait override the constants when set; tests shorten them.
	pingPeriod, pongWait time.Duration

	mu        sync.Mutex
	session   *client.Session
	userID    string
	sessionID string // kept while reconnecting, to resume it
	ws        *websocket.Conn
	closed    bool                   // by Close, which stops reconnecting
	voice     map[string]*VoiceState // by guild, as Discord sent it so far
	playing   map[string]*Track      // by guild, from TrackStartEvent until its TrackEndEvent
	handlers  []func(*Event)
}

// New returns a client for the node at url.
func New(url, password string) *Client {
	return &Client{URL: strings.TrimRight(url, "/"), Password: password}
}

func (c *Client) log() *slog.Logger { return cmp.Or(c.Logger, slog.Default()) }

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// Attach wires the client to a Discord session: it asks for the voice
// states intent, connects to the node when the session is ready, and
// forwards the bot's voice state and server updates. Call it before the
// session (or the minidis.Bot) connects.
func (c *Client) Attach(s *client.Session) {
	c.mu.Lock()
	c.session = s
	c.mu.Unlock()
	s.Intents |= discord.IntentsGuildVoiceStates
	s.OnReady(func(_ *client.Session, r *discord.Ready) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := c.Connect(ctx, r.User.ID); err != nil && !errors.Is(err, ErrNotConnected) {
			// As when a node starts with the bot: keep trying, as after a drop.
			c.log().Error("lavalink: connect to node; retrying", "error", err)
			go c.reconnect(r.User.ID)
		}
	})
	s.OnVoiceStateUpdate(func(_ *client.Session, e *discord.VoiceStateUpdate) {
		if e.VoiceState != nil {
			c.voiceStateUpdate(e.GuildID, e.UserID, e.SessionID, e.ChannelID)
		}
	})
	s.OnVoiceServerUpdate(func(_ *client.Session, e *discord.VoiceServerUpdate) {
		c.voiceServerUpdate(e.GuildID, e.Token, e.Endpoint)
	})
}

// Connect opens the node's WebSocket as the bot userID and turns on
// resuming for its session. Attach calls it on READY; it does nothing when
// already connected.
func (c *Client) Connect(ctx context.Context, userID string) error {
	c.mu.Lock()
	c.closed = false
	c.mu.Unlock()
	return c.connect(ctx, userID)
}

// connect is Connect without undoing Close, for reconnecting.
func (c *Client) connect(ctx context.Context, userID string) error {
	c.connectMu.Lock()
	defer c.connectMu.Unlock()
	c.mu.Lock()
	connected, closed, old := c.ws != nil, c.closed, c.sessionID
	c.userID = userID // so the bot's voice states are kept while it dials
	c.mu.Unlock()
	if closed {
		return ErrNotConnected
	}
	if connected {
		return nil
	}
	ws, sessionID, resumed, err := c.dial(ctx, userID, old)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.closed { // Close ran while dialing
		c.mu.Unlock()
		ws.Close()
		return ErrNotConnected
	}
	c.ws, c.userID, c.sessionID = ws, userID, sessionID
	guilds := slices.Collect(maps.Keys(c.voice))
	var lost map[string]*Track
	if old != "" && !resumed {
		lost, c.playing = c.playing, nil
	}
	wait := cmp.Or(c.pongWait, pongWait)
	c.mu.Unlock()
	// Set before reading starts: the pong handler runs inside ReadMessage.
	ws.SetReadDeadline(time.Now().Add(wait))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(wait)) })
	go c.readLoop(ws, wait, lost)
	go c.keepalive(ws)

	if old != "" && !resumed {
		// The node ended the old session, and its players with it; readLoop
		// tells the handlers their tracks ended.
		c.log().Warn("lavalink: node session not resumed; tracks that were playing ended", "session", sessionID)
	}
	if !resumed {
		// A new session has no voice connections: send the voice states
		// known so far, including any that came while dialing.
		for _, g := range guilds {
			go c.sendVoice(g)
		}
	}
	if err := c.do(ctx, http.MethodPatch, "/v4/sessions/"+url.PathEscape(sessionID),
		map[string]any{"resuming": true, "timeout": int(resumeTimeout / time.Second)}, nil); err != nil {
		c.log().Warn("lavalink: turn on resuming", "error", err)
	}
	return nil
}

// dial opens the WebSocket, asking to resume sessionID if set, and reads the
// ready message.
func (c *Client) dial(ctx context.Context, userID, sessionID string) (*websocket.Conn, string, bool, error) {
	u, err := url.Parse(c.URL + "/v4/websocket")
	if err != nil {
		return nil, "", false, err
	}
	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)
	header := http.Header{
		"Authorization": {c.Password},
		"User-Id":       {userID},
		"Client-Name":   {"minidis/" + client.LibraryVersion},
	}
	if sessionID != "" {
		header.Set("Session-Id", sessionID)
	}
	ws, resp, err := websocket.DefaultDialer.DialContext(ctx, u.String(), header)
	if err != nil {
		if resp != nil {
			return nil, "", false, fmt.Errorf("lavalink: connect: %w (HTTP %d)", err, resp.StatusCode)
		}
		return nil, "", false, fmt.Errorf("lavalink: connect: %w", err)
	}
	var ready struct {
		Op        string `json:"op"`
		Resumed   bool   `json:"resumed"`
		SessionID string `json:"sessionId"`
	}
	if dl, ok := ctx.Deadline(); ok {
		ws.SetReadDeadline(dl)
	}
	if err := ws.ReadJSON(&ready); err != nil || ready.Op != "ready" {
		ws.Close()
		return nil, "", false, fmt.Errorf("lavalink: no ready message: %v", err)
	}
	ws.SetReadDeadline(time.Time{})
	return ws, ready.SessionID, ready.Resumed, nil
}

// reconnect connects again after the WebSocket dropped, waiting longer
// after each failure, until it succeeds or Close is called.
func (c *Client) reconnect(userID string) {
	for wait := time.Second; ; wait = min(wait*2, 30*time.Second) {
		time.Sleep(wait)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := c.connect(ctx, userID)
		cancel()
		if err == nil || errors.Is(err, ErrNotConnected) {
			return
		}
		c.log().Error("lavalink: reconnect to node", "error", err, "retry_in", min(wait*2, 30*time.Second))
	}
}

// keepalive pings ws until it is replaced or fails.
func (c *Client) keepalive(ws *websocket.Conn) {
	c.mu.Lock()
	period := cmp.Or(c.pingPeriod, pingPeriod)
	c.mu.Unlock()
	t := time.NewTicker(period)
	defer t.Stop()
	for range t.C {
		c.mu.Lock()
		current := c.ws == ws
		c.mu.Unlock()
		if !current || ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)) != nil {
			return
		}
	}
}

// readLoop dispatches ws's events. It first sends a TrackEndEvent with
// EndCleanup for each track of lost, which played on a session the node
// ended, so handlers see every event on this one goroutine.
func (c *Client) readLoop(ws *websocket.Conn, wait time.Duration, lost map[string]*Track) {
	if len(lost) > 0 {
		c.mu.Lock()
		handlers := c.handlers
		c.mu.Unlock()
		for g, t := range lost {
			e := &Event{Type: TrackEndEvent, GuildID: g, Track: t, Reason: EndCleanup}
			for _, h := range handlers {
				h(e)
			}
		}
	}
	for {
		_, data, err := ws.ReadMessage()
		if err == nil {
			ws.SetReadDeadline(time.Now().Add(wait))
		}
		if err != nil {
			c.mu.Lock()
			lost := c.ws == ws && !c.closed // else Close was called
			if lost {
				c.ws = nil
			}
			userID := c.userID
			c.mu.Unlock()
			ws.Close()
			if lost {
				c.log().Warn("lavalink: node connection lost; reconnecting", "error", err)
				go c.reconnect(userID)
			}
			return
		}
		var e struct {
			Op string `json:"op"`
			Event
		}
		if json.Unmarshal(data, &e) != nil || e.Op != "event" {
			continue // playerUpdate and stats
		}
		c.mu.Lock()
		switch e.Type {
		case TrackStartEvent:
			if c.playing == nil {
				c.playing = map[string]*Track{}
			}
			c.playing[e.GuildID] = e.Track
		case TrackEndEvent:
			delete(c.playing, e.GuildID)
		}
		handlers := c.handlers
		c.mu.Unlock()
		for _, h := range handlers {
			h(&e.Event)
		}
	}
}

// OnEvent adds a handler for the node's player events. Handlers run one at a
// time on the client's reading goroutine, so they should not block long.
func (c *Client) OnEvent(h func(*Event)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers = append(c.handlers, h)
}

// Close closes the node's WebSocket and stops reconnecting. It turns
// resuming off first, so the node ends the players at once.
func (c *Client) Close() error {
	c.mu.Lock()
	ws, sid := c.ws, c.sessionID
	c.ws, c.sessionID, c.closed = nil, "", true
	c.mu.Unlock()
	if sid != "" { // also while reconnecting, when the node keeps the session waiting
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.do(ctx, http.MethodPatch, "/v4/sessions/"+url.PathEscape(sid), map[string]any{"resuming": false}, nil)
	}
	if ws == nil {
		return nil
	}
	return ws.Close()
}

// SessionID is the node's session, or "" before the first connection and
// after Close. It stays set while the client reconnects to resume it.
func (c *Client) SessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

// Join asks Discord to move the bot into a voice channel. The node connects
// once Discord's voice events arrive. The bot joins deafened, as it only
// plays.
func (c *Client) Join(guildID, channelID string) error {
	c.mu.Lock()
	s := c.session
	c.mu.Unlock()
	if s == nil {
		return errors.New("lavalink: Join needs Attach")
	}
	return s.UpdateVoiceState(guildID, channelID, false, true)
}

// Leave disconnects the bot from voice in the guild and destroys its player.
func (c *Client) Leave(ctx context.Context, guildID string) error {
	c.mu.Lock()
	s := c.session
	delete(c.voice, guildID)
	c.mu.Unlock()
	if s != nil {
		if err := s.UpdateVoiceState(guildID, "", false, false); err != nil {
			return err
		}
	}
	return c.Destroy(ctx, guildID)
}

func (c *Client) voiceStateUpdate(guildID, userID, sessionID, channelID string) {
	c.mu.Lock()
	if userID != c.userID {
		c.mu.Unlock()
		return
	}
	if channelID == "" {
		delete(c.voice, guildID)
		c.mu.Unlock()
		return
	}
	v := c.voiceLocked(guildID)
	v.SessionID, v.ChannelID = sessionID, channelID
	c.mu.Unlock()
	c.sendVoice(guildID)
}

func (c *Client) voiceServerUpdate(guildID, token, endpoint string) {
	c.mu.Lock()
	v := c.voiceLocked(guildID)
	v.Token, v.Endpoint = token, endpoint
	c.mu.Unlock()
	c.sendVoice(guildID)
}

func (c *Client) voiceLocked(guildID string) *VoiceState {
	if c.voice == nil {
		c.voice = map[string]*VoiceState{}
	}
	v := c.voice[guildID]
	if v == nil {
		v = &VoiceState{}
		c.voice[guildID] = v
	}
	return v
}

// sendVoice gives the node the guild's voice state once all of it is known.
func (c *Client) sendVoice(guildID string) {
	c.mu.Lock()
	v, ok := c.voice[guildID]
	var state VoiceState
	if ok {
		state = *v
	}
	c.mu.Unlock()
	if state.Token == "" || state.Endpoint == "" || state.SessionID == "" || state.ChannelID == "" {
		return // Discord sends the state and the server separately; endpoint is empty while it moves servers
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.UpdatePlayer(ctx, guildID, &UpdatePlayer{Voice: &state}, false); err != nil {
		c.log().Error("lavalink: forward voice state", "guild_id", guildID, "error", err)
	}
}

// do sends a REST request and decodes the JSON response into out.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.Password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&e)
		return &Error{Status: resp.StatusCode, Message: cmp.Or(e.Message, http.StatusText(resp.StatusCode))}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) playerPath(guildID string) (string, error) {
	sid := c.SessionID()
	if sid == "" {
		return "", ErrNotConnected
	}
	return "/v4/sessions/" + url.PathEscape(sid) + "/players/" + url.PathEscape(guildID), nil
}

// LoadTrack resolves an identifier (a URL, a file on the node, or a search
// such as "ytsearch:..." on nodes that support it) to its first track.
func (c *Client) LoadTrack(ctx context.Context, identifier string) (*Track, error) {
	tracks, err := c.LoadTracks(ctx, identifier)
	if err != nil {
		return nil, err
	}
	return tracks[0], nil
}

// LoadTracks resolves an identifier like LoadTrack, but returns every track:
// all the results of a search, or all the tracks of a playlist, such as to
// let someone pick one. It returns ErrNoMatches rather than no tracks.
func (c *Client) LoadTracks(ctx context.Context, identifier string) ([]*Track, error) {
	var res struct {
		LoadType string          `json:"loadType"`
		Data     json.RawMessage `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/v4/loadtracks?identifier="+url.QueryEscape(identifier), nil, &res); err != nil {
		return nil, err
	}
	var tracks []*Track
	switch res.LoadType {
	case "track":
		var t Track
		if err := json.Unmarshal(res.Data, &t); err != nil {
			return nil, err
		}
		tracks = []*Track{&t}
	case "search":
		if err := json.Unmarshal(res.Data, &tracks); err != nil {
			return nil, err
		}
	case "playlist":
		var pl struct {
			Tracks []*Track `json:"tracks"`
		}
		if err := json.Unmarshal(res.Data, &pl); err != nil {
			return nil, err
		}
		tracks = pl.Tracks
	case "error":
		var e Exception
		json.Unmarshal(res.Data, &e)
		return nil, &e
	}
	if len(tracks) == 0 {
		return nil, ErrNoMatches
	}
	return tracks, nil
}

// UpdatePlayer changes the guild's player, creating it if needed. With
// noReplace, a new track is ignored while one is playing.
func (c *Client) UpdatePlayer(ctx context.Context, guildID string, u *UpdatePlayer, noReplace bool) (*PlayerInfo, error) {
	return c.patch(ctx, guildID, u, noReplace)
}

func (c *Client) patch(ctx context.Context, guildID string, body any, noReplace bool) (*PlayerInfo, error) {
	path, err := c.playerPath(guildID)
	if err != nil {
		return nil, err
	}
	if noReplace {
		path += "?noReplace=true"
	}
	var p PlayerInfo
	if err := c.do(ctx, http.MethodPatch, path, body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Play plays track in the guild, replacing what is playing.
func (c *Client) Play(ctx context.Context, guildID string, track *Track) error {
	_, err := c.patch(ctx, guildID, &UpdatePlayer{Track: &UpdateTrack{Encoded: track.Encoded}}, false)
	return err
}

// Stop stops the track. The player and its voice connection stay.
func (c *Client) Stop(ctx context.Context, guildID string) error {
	_, err := c.patch(ctx, guildID, map[string]any{"track": map[string]any{"encoded": nil}}, false)
	return err
}

// Pause pauses or resumes the player.
func (c *Client) Pause(ctx context.Context, guildID string, paused bool) error {
	_, err := c.patch(ctx, guildID, &UpdatePlayer{Paused: &paused}, false)
	return err
}

// Seek moves the track to position.
func (c *Client) Seek(ctx context.Context, guildID string, position time.Duration) error {
	ms := position.Milliseconds()
	_, err := c.patch(ctx, guildID, &UpdatePlayer{Position: &ms}, false)
	return err
}

// SetVolume sets the volume in percent, 0 to 1000.
func (c *Client) SetVolume(ctx context.Context, guildID string, volume int) error {
	_, err := c.patch(ctx, guildID, &UpdatePlayer{Volume: &volume}, false)
	return err
}

// Player returns the guild's player. It fails with a 404 *Error when there is none.
func (c *Client) Player(ctx context.Context, guildID string) (*PlayerInfo, error) {
	path, err := c.playerPath(guildID)
	if err != nil {
		return nil, err
	}
	var p PlayerInfo
	if err := c.do(ctx, http.MethodGet, path, nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Destroy destroys the guild's player, which leaves voice on the node's side.
func (c *Client) Destroy(ctx context.Context, guildID string) error {
	path, err := c.playerPath(guildID)
	if err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.playing, guildID) // the node sends no end event for it
	c.mu.Unlock()
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// Stats returns the node's stats.
func (c *Client) Stats(ctx context.Context) (*Stats, error) {
	var s Stats
	if err := c.do(ctx, http.MethodGet, "/v4/stats", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
