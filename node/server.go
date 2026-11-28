// Package node is a standalone audio node for Discord bots. It speaks a
// subset of the Lavalink v4 REST and WebSocket protocol, so bots (and
// existing Lavalink clients) control per-guild players over HTTP while the
// node does the voice connection, DAVE encryption and ffmpeg decoding.
//
// Implemented: /version, /v4/info, /v4/stats, /v4/loadtracks (http(s) URLs,
// YouTube links and "ytsearch:" searches through yt-dlp, and files under
// Config.Source's local directory), /v4/decodetrack(s),
// players (GET/PATCH/DELETE: track, position, volume, paused, voice),
// PATCH /v4/sessions/{id} (resuming and its timeout), and the WebSocket ops
// ready, playerUpdate, stats and event. A session with resuming on keeps its
// players playing for the timeout after its WebSocket drops, queues their
// events, and is taken back by a WebSocket that sends its Session-Id header.
package node

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tbdsux/minidis/voice"
)

// Version is the Lavalink protocol version the node reports.
const Version = "4.2.0"

type Config struct {
	// Password must match each request's Authorization header.
	Password string
	Source   Source
	// Dial opens voice connections; nil means voice.Dial.
	Dial func(ctx context.Context, p voice.Params) (VoiceConn, error)

	StuckThreshold       time.Duration // default 10s
	PlayerUpdateInterval time.Duration // default 5s
	StatsInterval        time.Duration // default 60s
	Logger               *slog.Logger
}

// Server is the node's http.Handler.
type Server struct {
	cfg     Config
	log     *slog.Logger
	mux     *http.ServeMux
	started time.Time

	// pingPeriod and pongWait are how often the node pings clients and how
	// long it waits for an answer; tests shorten them.
	pingPeriod, pongWait time.Duration

	mu       sync.Mutex
	sessions map[string]*session
}

func New(cfg Config) *Server {
	if cfg.Dial == nil {
		cfg.Dial = func(ctx context.Context, p voice.Params) (VoiceConn, error) {
			c, err := voice.Dial(ctx, p)
			if err != nil {
				return nil, err
			}
			return c, nil
		}
	}
	if cfg.StuckThreshold <= 0 {
		cfg.StuckThreshold = 10 * time.Second
	}
	if cfg.PlayerUpdateInterval <= 0 {
		cfg.PlayerUpdateInterval = 5 * time.Second
	}
	if cfg.StatsInterval <= 0 {
		cfg.StatsInterval = 60 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &Server{cfg: cfg, log: cfg.Logger, mux: http.NewServeMux(), started: time.Now(), sessions: map[string]*session{},
		pingPeriod: 15 * time.Second, pongWait: 45 * time.Second}

	s.mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(Version)) })
	s.mux.HandleFunc("GET /v4/info", s.handleInfo)
	s.mux.HandleFunc("GET /v4/stats", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.stats()) })
	s.mux.HandleFunc("GET /v4/loadtracks", s.handleLoadTracks)
	s.mux.HandleFunc("GET /v4/decodetrack", s.handleDecodeTrack)
	s.mux.HandleFunc("POST /v4/decodetracks", s.handleDecodeTracks)
	s.mux.HandleFunc("PATCH /v4/sessions/{sid}", s.handleUpdateSession)
	s.mux.HandleFunc("GET /v4/sessions/{sid}/players", s.handlePlayers)
	s.mux.HandleFunc("GET /v4/sessions/{sid}/players/{gid}", s.handlePlayer)
	s.mux.HandleFunc("PATCH /v4/sessions/{sid}/players/{gid}", s.handleUpdatePlayer)
	s.mux.HandleFunc("DELETE /v4/sessions/{sid}/players/{gid}", s.handleDestroyPlayer)
	s.mux.HandleFunc("GET /v4/websocket", s.handleWebSocket)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(s.cfg.Password)) != 1 {
		writeError(w, r, http.StatusUnauthorized, "invalid authorization")
		return
	}
	s.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"timestamp": time.Now().UnixMilli(), "status": status,
		"error": http.StatusText(status), "message": msg, "path": r.URL.Path})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"version":        map[string]any{"semver": Version, "major": 4, "minor": 2, "patch": 0, "preRelease": nil, "build": "minidis"},
		"buildTime":      s.started.UnixMilli(),
		"git":            map[string]any{"branch": "", "commit": "", "commitTime": 0},
		"jvm":            runtime.Version(),
		"lavaplayer":     "none (ffmpeg)",
		"sourceManagers": []string{"http", "youtube", "local"},
		"filters":        []string{},
		"plugins":        []any{},
	})
}

func (s *Server) stats() Stats {
	var st Stats
	s.mu.Lock()
	sessions := slices.Collect(maps.Values(s.sessions))
	s.mu.Unlock()
	for _, sess := range sessions {
		for _, p := range sess.playerList() {
			p.mu.Lock()
			st.Players++
			if p.track != nil && !p.paused {
				st.PlayingPlayers++
			}
			p.mu.Unlock()
		}
	}
	st.Uptime = time.Since(s.started).Milliseconds()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	st.Memory.Used = m.HeapAlloc
	st.Memory.Allocated = m.Sys
	st.Memory.Free = m.Sys - m.HeapAlloc
	st.Memory.Reservable = m.Sys
	st.CPU.Cores = runtime.NumCPU()
	return st
}

func (s *Server) handleLoadTracks(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("identifier")
	if id == "" {
		writeError(w, r, http.StatusBadRequest, "missing identifier")
		return
	}
	if infos, err := s.search(r.Context(), id); !errors.Is(err, ErrUnsupported) {
		switch {
		case err != nil:
			writeJSON(w, map[string]any{"loadType": "error",
				"data": Exception{Message: err.Error(), Severity: "common", Cause: err.Error()}})
		case len(infos) == 0:
			writeJSON(w, map[string]any{"loadType": "empty", "data": map[string]any{}})
		default:
			tracks := make([]*Track, len(infos))
			for i, info := range infos {
				tracks[i] = newTrack(info)
			}
			writeJSON(w, map[string]any{"loadType": "search", "data": tracks})
		}
		return
	}
	info, err := s.cfg.Source.Load(r.Context(), id)
	switch {
	case errors.Is(err, ErrUnsupported):
		writeJSON(w, map[string]any{"loadType": "empty", "data": map[string]any{}})
	case err != nil:
		writeJSON(w, map[string]any{"loadType": "error",
			"data": Exception{Message: err.Error(), Severity: "common", Cause: err.Error()}})
	default:
		writeJSON(w, map[string]any{"loadType": "track", "data": newTrack(info)})
	}
}

// search runs id as a search if the source has them, or returns ErrUnsupported.
func (s *Server) search(ctx context.Context, id string) ([]TrackInfo, error) {
	if sr, ok := s.cfg.Source.(Searcher); ok {
		return sr.Search(ctx, id)
	}
	return nil, ErrUnsupported
}

// loadOne resolves id to one track: the first result of a search.
func (s *Server) loadOne(ctx context.Context, id string) (TrackInfo, error) {
	infos, err := s.search(ctx, id)
	switch {
	case errors.Is(err, ErrUnsupported):
		return s.cfg.Source.Load(ctx, id)
	case err != nil:
		return TrackInfo{}, err
	case len(infos) == 0:
		return TrackInfo{}, errors.New("no matches")
	}
	return infos[0], nil
}

func (s *Server) handleDecodeTrack(w http.ResponseWriter, r *http.Request) {
	t, err := decodeTrack(r.URL.Query().Get("encodedTrack"))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, t)
}

func (s *Server) handleDecodeTracks(w http.ResponseWriter, r *http.Request) {
	var encoded []string
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&encoded); err != nil {
		writeError(w, r, http.StatusBadRequest, "body must be an array of encoded tracks")
		return
	}
	tracks := make([]*Track, 0, len(encoded))
	for _, e := range encoded {
		t, err := decodeTrack(e)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		tracks = append(tracks, t)
	}
	writeJSON(w, tracks)
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) *session {
	s.mu.Lock()
	sess := s.sessions[r.PathValue("sid")]
	s.mu.Unlock()
	if sess == nil {
		writeError(w, r, http.StatusNotFound, "session not found")
	}
	return sess
}

func (s *Server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	sess := s.session(w, r)
	if sess == nil {
		return
	}
	var u struct {
		Resuming *bool  `json:"resuming"`
		Timeout  *int64 `json:"timeout"` // seconds
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&u); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if u.Timeout != nil && *u.Timeout <= 0 {
		writeError(w, r, http.StatusBadRequest, "timeout must be positive")
		return
	}
	sess.mu.Lock()
	if u.Resuming != nil {
		sess.resuming = *u.Resuming
	}
	if u.Timeout != nil {
		sess.timeout = time.Duration(*u.Timeout) * time.Second
	}
	res := map[string]any{"resuming": sess.resuming, "timeout": int64(sess.timeout / time.Second)}
	sess.mu.Unlock()
	writeJSON(w, res)
}

func (s *Server) handlePlayers(w http.ResponseWriter, r *http.Request) {
	sess := s.session(w, r)
	if sess == nil {
		return
	}
	infos := []PlayerInfo{}
	for _, p := range sess.playerList() {
		infos = append(infos, p.info())
	}
	writeJSON(w, infos)
}

func (s *Server) handlePlayer(w http.ResponseWriter, r *http.Request) {
	sess := s.session(w, r)
	if sess == nil {
		return
	}
	p := sess.player(r.PathValue("gid"), false)
	if p == nil {
		writeError(w, r, http.StatusNotFound, "player not found")
		return
	}
	writeJSON(w, p.info())
}

func (s *Server) handleUpdatePlayer(w http.ResponseWriter, r *http.Request) {
	sess := s.session(w, r)
	if sess == nil {
		return
	}
	var u UpdatePlayer
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&u); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if u.Position != nil && *u.Position < 0 {
		writeError(w, r, http.StatusBadRequest, "position must not be negative")
		return
	}
	if u.Voice != nil && (u.Voice.Token == "" || u.Voice.Endpoint == "" || u.Voice.SessionID == "" || u.Voice.ChannelID == "") {
		writeError(w, r, http.StatusBadRequest, "voice needs token, endpoint, sessionId and channelId")
		return
	}

	encoded, identifier := u.EncodedTrack, u.Identifier
	var userData json.RawMessage
	if u.Track != nil {
		encoded, identifier, userData = u.Track.Encoded, u.Track.Identifier, u.Track.UserData
	}
	var track *Track
	stop := false
	switch {
	case encoded.Set && encoded.Value == nil:
		stop = true
	case encoded.Set:
		t, err := decodeTrack(*encoded.Value)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		track = t
	case identifier != nil:
		info, err := s.loadOne(r.Context(), *identifier)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "load track: "+err.Error())
			return
		}
		track = newTrack(info)
	}
	if track != nil && len(userData) > 0 {
		track.UserData = userData
	}

	p := sess.player(r.PathValue("gid"), true)
	p.update(&u, track, stop, r.URL.Query().Get("noReplace") == "true")
	writeJSON(w, p.info())
}

func (s *Server) handleDestroyPlayer(w http.ResponseWriter, r *http.Request) {
	sess := s.session(w, r)
	if sess == nil {
		return
	}
	if p := sess.removePlayer(r.PathValue("gid")); p != nil {
		p.destroy()
	}
	w.WriteHeader(http.StatusNoContent)
}

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }} // bots aren't browsers; auth is the password

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("User-Id")
	if userID == "" {
		writeError(w, r, http.StatusBadRequest, "missing User-Id header")
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	sess := s.resume(r.Header.Get("Session-Id"), userID, ws)
	if sess != nil {
		s.log.Info("node: session resumed", "session", sess.id)
	} else {
		sess = newSession(s, userID)
		s.mu.Lock()
		s.sessions[sess.id] = sess
		s.mu.Unlock()
		sess.attach(ws, false)
		s.log.Info("node: session opened", "session", sess.id, "client", r.Header.Get("Client-Name"))
		go sess.tick()
	}
	// The node pings, and drops a client that stops answering, so a WebSocket
	// that died without failing, as after a network change, still ends. Set
	// before reading starts: the pong handler runs inside ReadMessage.
	ws.SetReadDeadline(time.Now().Add(s.pongWait))
	ws.SetPongHandler(func(string) error { return ws.SetReadDeadline(time.Now().Add(s.pongWait)) })
	stop := make(chan struct{})
	go keepalive(ws, s.pingPeriod, stop)
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			break
		}
	}
	close(stop)
	s.detach(sess, ws)
}

// keepalive pings ws every period until stop is closed.
func keepalive(ws *websocket.Conn, period time.Duration, stop chan struct{}) {
	t := time.NewTicker(period)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
				return
			}
		}
	}
}

// resume gives ws the session id when it is the user's and resuming is on,
// or returns nil.
func (s *Server) resume(id, userID string, ws *websocket.Conn) *session {
	if id == "" {
		return nil
	}
	s.mu.Lock()
	sess := s.sessions[id]
	s.mu.Unlock()
	if sess == nil || sess.userID != userID {
		return nil
	}
	sess.mu.Lock()
	resuming := sess.resuming
	sess.mu.Unlock()
	if !resuming || !sess.attach(ws, true) {
		return nil
	}
	return sess
}

// detach handles ws dropping: the session ends, or with resuming on waits
// for its timeout to be resumed.
func (s *Server) detach(sess *session, ws *websocket.Conn) {
	ws.Close()
	sess.wsMu.Lock()
	if sess.ws != ws { // resumed on a new WebSocket, or ended
		sess.wsMu.Unlock()
		return
	}
	sess.ws = nil
	sess.mu.Lock()
	resuming, timeout := sess.resuming, sess.timeout
	sess.mu.Unlock()
	if resuming {
		sess.expiry = time.AfterFunc(timeout, func() {
			sess.wsMu.Lock()
			expired := sess.ws == nil && !sess.ended
			if expired {
				sess.ended = true // so no resume slips in before end
			}
			sess.wsMu.Unlock()
			if expired {
				s.end(sess)
			}
		})
	}
	sess.wsMu.Unlock()
	if resuming {
		s.log.Info("node: session waiting to be resumed", "session", sess.id, "timeout", timeout)
		return
	}
	s.end(sess)
}

// end removes the session and destroys its players.
func (s *Server) end(sess *session) {
	s.mu.Lock()
	delete(s.sessions, sess.id)
	s.mu.Unlock()
	sess.close()
	s.log.Info("node: session closed", "session", sess.id)
}

// maxQueued is how many events a session waiting to be resumed keeps.
const maxQueued = 1000

// session is one client and the players it owns. Its WebSocket is nil while
// it waits to be resumed.
type session struct {
	node   *Server
	id     string
	userID string
	ctx    context.Context
	cancel context.CancelFunc

	wsMu   sync.Mutex
	ws     *websocket.Conn
	queue  []any       // events sent while ws is nil
	expiry *time.Timer // ends the session unless it is resumed first
	ended  bool

	mu       sync.Mutex
	players  map[string]*player
	resuming bool
	timeout  time.Duration
}

func newSession(n *Server, userID string) *session {
	ctx, cancel := context.WithCancel(context.Background())
	return &session{node: n, id: rand.Text()[:16], userID: userID, ctx: ctx, cancel: cancel,
		players: map[string]*player{}, timeout: 60 * time.Second}
}

// attach makes ws the session's WebSocket, sends it the ready message and
// the queued events, and closes a WebSocket it replaces. It fails once the
// session ended.
func (s *session) attach(ws *websocket.Conn, resumed bool) bool {
	s.wsMu.Lock()
	if s.ended {
		s.wsMu.Unlock()
		return false
	}
	old := s.ws
	s.ws = ws
	if s.expiry != nil {
		s.expiry.Stop()
		s.expiry = nil
	}
	s.writeLocked(map[string]any{"op": "ready", "resumed": resumed, "sessionId": s.id})
	for _, v := range s.queue {
		s.writeLocked(v)
	}
	s.queue = nil
	s.wsMu.Unlock()
	if old != nil {
		old.Close() // a client resuming before the node saw its old WebSocket drop
	}
	return true
}

// send writes v to the WebSocket, or queues it if it is an event and the
// session waits to be resumed. playerUpdate and stats are not queued.
func (s *session) send(v any) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	if s.ws != nil {
		s.writeLocked(v)
		return
	}
	if m, ok := v.(map[string]any); ok && m["op"] == "event" && !s.ended && len(s.queue) < maxQueued {
		s.queue = append(s.queue, v)
	}
}

func (s *session) writeLocked(v any) {
	s.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := s.ws.WriteJSON(v); err != nil {
		s.node.log.Debug("node: websocket write", "session", s.id, "error", err)
	}
}

func (s *session) player(guildID string, create bool) *player {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.players[guildID]
	if p == nil && create {
		p = newPlayer(s, guildID)
		s.players[guildID] = p
	}
	return p
}

func (s *session) removePlayer(guildID string) *player {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.players[guildID]
	delete(s.players, guildID)
	return p
}

func (s *session) playerList() []*player {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Collect(maps.Values(s.players))
}

// tick sends stats and playerUpdate messages until the session ends.
func (s *session) tick() {
	stats := time.NewTicker(s.node.cfg.StatsInterval)
	updates := time.NewTicker(s.node.cfg.PlayerUpdateInterval)
	defer stats.Stop()
	defer updates.Stop()
	s.sendStats()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-stats.C:
			s.sendStats()
		case <-updates.C:
			for _, p := range s.playerList() {
				p.mu.Lock()
				active := p.track != nil || p.conn != nil
				state := p.stateLocked()
				p.mu.Unlock()
				if active {
					s.send(map[string]any{"op": "playerUpdate", "guildId": p.guildID, "state": state})
				}
			}
		}
	}
}

func (s *session) sendStats() {
	st := s.node.stats()
	b, _ := json.Marshal(st)
	m := map[string]any{"op": "stats"}
	json.Unmarshal(b, &m)
	s.send(m)
}

func (s *session) close() {
	s.cancel()
	s.wsMu.Lock()
	s.ended = true
	if s.ws != nil {
		s.ws.Close()
	}
	if s.expiry != nil {
		s.expiry.Stop()
	}
	s.queue = nil
	s.wsMu.Unlock()
	for _, p := range s.playerList() {
		p.destroy()
	}
}
