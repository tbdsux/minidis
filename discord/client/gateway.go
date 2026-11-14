package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tbdsux/minidis/discord"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	opDispatch       = 0
	opHeartbeat      = 1
	opIdentify       = 2
	opPresenceUpdate = 3
	opResume         = 6
	opReconnect      = 7
	opInvalidSession = 9
	opHello          = 10
	opHeartbeatACK   = 11
)

const gatewayWriteTimeout = 10 * time.Second

// Variables so tests can shorten them.
var (
	gatewayOpenTimeout = 30 * time.Second
	identifyInterval   = 5 * time.Second // one Identify per 5s at max_concurrency 1
	minReconnectDelay  = time.Second
	maxReconnectDelay  = time.Minute
	closeDrainTimeout  = 10 * time.Second
)

// ErrAlreadyOpen is returned by Open if the session has not been closed.
var ErrAlreadyOpen = errors.New("discord: session is already open")

// GatewayCloseError means Discord closed the gateway connection with an error code.
type GatewayCloseError struct {
	Code int
	Text string
}

func (e *GatewayCloseError) Error() string {
	return fmt.Sprintf("discord: gateway closed with code %d: %s", e.Code, e.Text)
}

// gatewayBot is the response of GET /gateway/bot.
type gatewayBot struct {
	URL               string `json:"url"`
	SessionStartLimit struct {
		Remaining  int `json:"remaining"`
		ResetAfter int `json:"reset_after"` // milliseconds
	} `json:"session_start_limit"`
}

func (s *Session) fetchGatewayBot(ctx context.Context) (*gatewayBot, error) {
	info := &gatewayBot{}
	if err := s.Request(ctx, http.MethodGet, "/gateway/bot", nil, info); err != nil {
		return nil, err
	}
	return info, nil
}

// errShuttingDown is returned by an Open that would race a Shutdown waiting
// for the session.
var errShuttingDown = errors.New("discord: session is shutting down")

// gatewayRun is one Open: the gateway goroutine and the handlers it started.
type gatewayRun struct {
	cancel  context.CancelFunc // stops the gateway goroutine
	done    chan struct{}      // closed when the gateway goroutine has stopped
	err     error              // why it stopped on its own; written before done closes
	running *sync.WaitGroup    // the handlers it started
}

func (r *gatewayRun) stopped() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// Open connects to the Gateway and returns once the session is READY. After
// that, the connection is maintained in the background until Shutdown, Close,
// or a close code from Discord that rules out reconnecting (see Done and Err).
//
// Canceling ctx aborts the connection attempt and returns ctx's error; it has
// no effect once Open has returned. Open gives up after 30 seconds without
// READY. A Shutdown or Close called meanwhile cancels it, and an Open that
// races a Shutdown in progress fails rather than reopening the session.
func (s *Session) Open(ctx context.Context) error {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()

	if r := s.run.Load(); r != nil && !r.stopped() {
		return ErrAlreadyOpen
	}

	openCtx, cancelOpen := context.WithTimeout(ctx, gatewayOpenTimeout)
	defer cancelOpen()

	if !s.startOpening(cancelOpen) {
		return errShuttingDown
	}
	defer s.startOpening(nil)

	// A previous connection that Discord ended starts over with a new session.
	s.resetSession()

	info, err := s.fetchGatewayBot(openCtx)
	if err != nil {
		return openError(ctx, openCtx, err)
	}

	// Discord resets the token of bots that exceed the daily Identify limit.
	if info.SessionStartLimit.Remaining <= 0 {
		return fmt.Errorf("discord: no session starts remaining, resets in %s",
			time.Duration(info.SessionStartLimit.ResetAfter)*time.Millisecond)
	}

	s.gatewayURL = info.URL

	// The connection outlives ctx, so it gets a context of its own.
	runCtx, cancel := context.WithCancel(context.Background())
	run := &gatewayRun{cancel: cancel, done: make(chan struct{}), running: &sync.WaitGroup{}}
	s.running = run.running

	// Handlers started by any run are drained by the next Shutdown, including
	// those of a run that failed below after READY arrived too late.
	s.drain = append(s.drain, run.running)

	opened := make(chan error, 1)
	go s.gatewayLoop(runCtx, run, opened)

	select {
	case err = <-opened:
	case <-openCtx.Done():
		err = openError(ctx, openCtx, openCtx.Err())
	}

	if err != nil {
		cancel()
		<-run.done
		s.resetSession()
		return err
	}

	s.run.Store(run)
	return nil
}

// openError reports why an Open failed: the caller canceled ctx, the open
// timeout passed, a Shutdown canceled it, or err.
func openError(ctx, openCtx context.Context, err error) error {
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(openCtx.Err(), context.DeadlineExceeded):
		return errors.New("discord: timed out waiting for READY")
	case openCtx.Err() != nil:
		return errShuttingDown
	default:
		return err
	}
}

// startOpening records the cancel func of an Open in progress, or clears it
// when cancel is nil. It reports false if a Shutdown is waiting, in which case
// the Open should not start.
func (s *Session) startOpening(cancel context.CancelFunc) bool {
	s.openingMu.Lock()
	defer s.openingMu.Unlock()

	if cancel != nil && s.pendingShutdowns > 0 {
		return false
	}
	s.opening = cancel
	return true
}

// Done returns a channel that is closed when the connection made by the last
// Open stops for good: after Shutdown or Close, or when Discord closes it with
// a code that rules out reconnecting, such as an invalid token or disallowed
// intents. Err then says why. Done returns nil before the first Open and after
// Shutdown.
func (s *Session) Done() <-chan struct{} {
	if r := s.run.Load(); r != nil {
		return r.done
	}
	return nil
}

// Err returns why the connection made by the last Open stopped on its own,
// such as a *GatewayCloseError. It is nil while the connection is up, after
// Shutdown, and before the first Open. Open can be called again afterwards.
func (s *Session) Err() error {
	r := s.run.Load()
	if r == nil || !r.stopped() {
		return nil
	}
	return r.err
}

// Close disconnects from the Gateway, ends the session, and gives running
// handlers up to 10 seconds to finish. Use Shutdown to choose how long to wait.
func (s *Session) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), closeDrainTimeout)
	defer cancel()

	return s.Shutdown(ctx)
}

// Shutdown disconnects from the Gateway, ends the session, and waits for
// running handlers to finish or for ctx to end, returning ctx's error in that
// case. An Open in progress is canceled first. Calling Shutdown from a handler
// waits for that handler too, so it would use up the whole of ctx.
func (s *Session) Shutdown(ctx context.Context) error {
	s.openingMu.Lock()
	s.pendingShutdowns++
	if s.opening != nil {
		s.opening()
	}
	s.openingMu.Unlock()

	defer func() {
		s.openingMu.Lock()
		s.pendingShutdowns--
		s.openingMu.Unlock()
	}()

	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()

	if r := s.run.Load(); r != nil {
		r.cancel()
		<-r.done
		s.run.Store(nil)
		s.resetSession()
	}

	// No new handlers can start now that the gateway goroutine has stopped.
	waiting := s.drain
	s.drain = nil
	if len(waiting) == 0 {
		return nil
	}

	// yagni: this goroutine outlives ctx until a stuck handler returns.
	drained := make(chan struct{})
	go func() {
		for _, running := range waiting {
			running.Wait()
		}
		close(drained)
	}()

	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("discord: handlers still running: %w", ctx.Err())
	}
}

// UpdatePresence sets the bot's status and activities. It may be called at any
// time; before the connection is ready, and after a reconnect, the presence is
// applied as soon as the session is ready.
func (s *Session) UpdatePresence(p *discord.PresenceUpdate) error {
	update := *p
	if update.Status == "" {
		update.Status = discord.StatusOnline
	}
	if update.Activities == nil {
		update.Activities = []*discord.Activity{} // Discord requires an array
	}

	s.presence.Store(&update)

	if c := s.conn.Load(); c != nil {
		return c.send(opPresenceUpdate, &update)
	}
	return nil
}

// markConnected makes c available to UpdatePresence and applies the presence,
// which covers any update made while there was no connection.
func (s *Session) markConnected(c *gatewayConn) error {
	s.conn.Store(c)

	if p := s.presence.Load(); p != nil {
		return c.send(opPresenceUpdate, p)
	}
	return nil
}

func (s *Session) resetSession() {
	s.sessionID, s.resumeURL = "", ""
	s.sequence.Store(0)
}

// connOutcome says what to do after a connection ends.
type connOutcome int

const (
	reconnect      connOutcome = iota // reconnect, resuming if there is a session
	reconnectFresh                    // the session is gone: reconnect and Identify
	stopGateway                       // Discord says retrying cannot succeed
)

// gatewayLoop runs connections until the context ends or Discord rules out
// reconnecting. It reports the first READY, or the failure before it, on
// opened, and closes run.done when it returns.
func (s *Session) gatewayLoop(ctx context.Context, run *gatewayRun, opened chan<- error) {
	defer close(run.done)

	isOpen := false
	delay := minReconnectDelay

	onReady := func() {
		delay = minReconnectDelay
		if !isOpen {
			isOpen = true
			opened <- nil
		}
	}

	for {
		outcome, err := s.runConnection(ctx, onReady)

		if ctx.Err() != nil {
			return
		}

		if !isOpen {
			if err == nil {
				err = errors.New("discord: gateway disconnected before READY")
			}
			opened <- err
			return
		}

		switch outcome {
		case stopGateway:
			log.Printf("discord: gateway stopped: %v", err)
			run.err = err
			return
		case reconnectFresh:
			s.resetSession()
		}

		if err != nil {
			log.Printf("discord: gateway disconnected, reconnecting: %v", err)
		}

		wait := delay/2 + rand.N(delay/2+1)
		if sleep(ctx, wait) != nil {
			return
		}
		delay = min(delay*2, maxReconnectDelay)
	}
}

// prepareIdentify applies Discord's limits before a new session is started.
// The first Identify is covered by the check in Open.
func (s *Session) prepareIdentify(ctx context.Context) error {
	if !s.identified {
		return nil
	}

	info, err := s.fetchGatewayBot(ctx)
	if err != nil {
		return err
	}
	s.gatewayURL = info.URL

	if info.SessionStartLimit.Remaining <= 0 {
		wait := time.Duration(info.SessionStartLimit.ResetAfter) * time.Millisecond
		log.Printf("discord: no session starts remaining, waiting %s", wait)
		if err := sleep(ctx, wait); err != nil {
			return err
		}
	}

	return sleep(ctx, time.Until(s.lastIdentify.Add(identifyInterval)))
}

func gatewayEndpoint(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("discord: invalid gateway url: %w", err)
	}

	q := u.Query()
	q.Set("v", strconv.Itoa(APIVersion))
	q.Set("encoding", "json")
	u.RawQuery = q.Encode()

	return u.String(), nil
}

type gatewayPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
	S  *int64          `json:"s"`
	T  string          `json:"t"`
}

// gatewayConn is one websocket connection.
type gatewayConn struct {
	ws      *websocket.Conn
	writeMu sync.Mutex
	acked   atomic.Bool
}

func (c *gatewayConn) send(op int, d any) error {
	payload, err := json.Marshal(struct {
		Op int `json:"op"`
		D  any `json:"d"`
	}{op, d})
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := c.ws.SetWriteDeadline(time.Now().Add(gatewayWriteTimeout)); err != nil {
		return err
	}
	return c.ws.WriteMessage(websocket.TextMessage, payload)
}

func (s *Session) sendHeartbeat(c *gatewayConn) error {
	if seq := s.sequence.Load(); seq != 0 {
		return c.send(opHeartbeat, seq)
	}
	return c.send(opHeartbeat, nil)
}

// heartbeat runs until ctx ends. Closing the socket without a close frame keeps
// the session resumable and unblocks the read loop.
func (s *Session) heartbeat(ctx context.Context, c *gatewayConn, interval time.Duration) {
	timer := time.NewTimer(time.Duration(rand.Float64() * float64(interval)))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if !c.acked.Swap(false) {
			log.Printf("discord: no heartbeat ACK, reconnecting")
			c.ws.Close()
			return
		}

		if err := s.sendHeartbeat(c); err != nil {
			c.ws.Close()
			return
		}

		timer.Reset(interval)
	}
}

func (s *Session) sendIdentify(c *gatewayConn) error {
	type properties struct {
		OS      string `json:"os"`
		Browser string `json:"browser"`
		Device  string `json:"device"`
	}

	identify := struct {
		Token          string         `json:"token"`
		Properties     properties     `json:"properties"`
		Intents        discord.Intent `json:"intents"`
		LargeThreshold int            `json:"large_threshold"`
		Shard          *[2]int        `json:"shard,omitempty"`
	}{
		Token:          s.Token,
		Properties:     properties{OS: runtime.GOOS, Browser: libraryName, Device: libraryName},
		Intents:        s.Intents,
		LargeThreshold: 250,
	}

	if s.ShardCount > 1 {
		identify.Shard = &[2]int{s.ShardID, s.ShardCount}
	}

	s.lastIdentify = time.Now()
	s.identified = true

	return c.send(opIdentify, identify)
}

func (s *Session) sendResume(c *gatewayConn) error {
	return c.send(opResume, struct {
		Token     string `json:"token"`
		SessionID string `json:"session_id"`
		Seq       int64  `json:"seq"`
	}{s.Token, s.sessionID, s.sequence.Load()})
}

// runConnection opens one websocket and reads from it until it ends.
func (s *Session) runConnection(ctx context.Context, onReady func()) (connOutcome, error) {
	resuming := s.sessionID != ""

	target := s.resumeURL
	if !resuming {
		if err := s.prepareIdentify(ctx); err != nil {
			var restErr *RESTError
			if errors.As(err, &restErr) && restErr.StatusCode == http.StatusUnauthorized {
				return stopGateway, err
			}
			return reconnect, err
		}
		target = s.gatewayURL
	}

	endpoint, err := gatewayEndpoint(target)
	if err != nil {
		return reconnectFresh, err
	}

	ws, _, err := websocket.DefaultDialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return reconnect, fmt.Errorf("discord: dial gateway: %w", err)
	}
	defer ws.Close()

	conn := &gatewayConn{ws: ws}
	conn.acked.Store(true)
	defer s.conn.Store(nil)

	connCtx, stop := context.WithCancel(ctx)
	defer stop()

	// On Close, say goodbye with 1000 so Discord ends the session immediately.
	go func() {
		<-connCtx.Done()
		if ctx.Err() != nil {
			msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
			_ = ws.WriteControl(websocket.CloseMessage, msg, time.Now().Add(gatewayWriteTimeout))
		}
		ws.Close()
	}()

	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			return classifyDisconnect(err)
		}

		var p gatewayPayload
		if err := json.Unmarshal(data, &p); err != nil {
			log.Printf("discord: invalid gateway payload: %v", err)
			continue
		}

		switch p.Op {
		case opHello:
			var hello struct {
				HeartbeatInterval int `json:"heartbeat_interval"`
			}
			if err := json.Unmarshal(p.D, &hello); err != nil {
				return reconnect, fmt.Errorf("discord: invalid Hello: %w", err)
			}

			go s.heartbeat(connCtx, conn, time.Duration(hello.HeartbeatInterval)*time.Millisecond)

			if resuming {
				err = s.sendResume(conn)
			} else {
				err = s.sendIdentify(conn)
			}
			if err != nil {
				return reconnect, err
			}

		case opHeartbeat:
			if err := s.sendHeartbeat(conn); err != nil {
				return reconnect, err
			}

		case opHeartbeatACK:
			conn.acked.Store(true)

		case opReconnect:
			return reconnect, nil

		case opInvalidSession:
			var resumable bool
			_ = json.Unmarshal(p.D, &resumable)
			if resumable {
				return reconnect, nil
			}
			return reconnectFresh, nil

		case opDispatch:
			if p.S != nil {
				s.sequence.Store(*p.S)
			}

			switch p.T {
			case "READY":
				var ready discord.Ready
				if err := json.Unmarshal(p.D, &ready); err != nil {
					return reconnectFresh, fmt.Errorf("discord: invalid READY: %w", err)
				}

				s.sessionID = ready.SessionID
				s.resumeURL = ready.ResumeGatewayURL
				if s.resumeURL == "" {
					s.resumeURL = s.gatewayURL
				}

				s.user.Store(ready.User)
				if ready.Application != nil {
					id := ready.Application.ID
					s.appID.Store(&id)
				}

				if err := s.markConnected(conn); err != nil {
					return reconnect, err
				}
				onReady()
			case "RESUMED":
				if err := s.markConnected(conn); err != nil {
					return reconnect, err
				}
				onReady()
			}

			s.dispatch(p.T, p.D)
		}
	}
}

// classifyDisconnect applies the "Reconnect" column of Discord's close code table.
func classifyDisconnect(err error) (connOutcome, error) {
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		return reconnect, err
	}

	wrapped := &GatewayCloseError{Code: closeErr.Code, Text: closeErr.Text}

	switch closeErr.Code {
	case 4004, 4010, 4011, 4012, 4013, 4014:
		return stopGateway, wrapped
	case 4007, 4009:
		return reconnectFresh, wrapped
	default:
		return reconnect, wrapped
	}
}

// dispatch decodes an event and starts its handlers.
func (s *Session) dispatch(event string, data json.RawMessage) {
	s.handlersMu.RLock()
	defer s.handlersMu.RUnlock()

	for _, h := range s.onRawEvents {
		s.spawn(event, func() { h(s, event, data) })
	}

	switch event {
	case "READY":
		deliver(s, event, data, s.onReady)
	case "MESSAGE_CREATE":
		deliver(s, event, data, s.onMessage)
	case "MESSAGE_UPDATE":
		deliver(s, event, data, s.onMessageUpdate)
	case "MESSAGE_DELETE":
		deliver(s, event, data, s.onMessageDelete)
	case "MESSAGE_REACTION_ADD":
		deliver(s, event, data, s.onReactionAdd)
	case "GUILD_CREATE":
		deliver(s, event, data, s.onGuildCreate)
	case "GUILD_MEMBER_ADD":
		deliver(s, event, data, s.onGuildMemberAdd)
	case "GUILD_MEMBER_REMOVE":
		deliver(s, event, data, s.onGuildMemberGone)

	case "INTERACTION_CREATE":
		// InteractionCreate would promote the UnmarshalJSON of its nil embedded
		// pointer, so the Interaction is decoded on its own.
		if len(s.onInteract) > 0 {
			interaction := &discord.Interaction{}
			if decodeEvent(event, data, interaction) {
				for _, h := range s.onInteract {
					s.spawn(event, func() { h(s, &discord.InteractionCreate{Interaction: interaction}) })
				}
			}
		}
	}
}

// deliver decodes an event into a T and starts its handlers.
func deliver[T any](s *Session, event string, data json.RawMessage, handlers []func(*Session, *T)) {
	if len(handlers) == 0 {
		return
	}

	v := new(T)
	if !decodeEvent(event, data, v) {
		return
	}

	for _, h := range handlers {
		s.spawn(event, func() { h(s, v) })
	}
}

// spawn runs a handler in its own goroutine, tracked for Shutdown. A panic stops
// only that handler. This is the single place handler goroutines start, so it
// covers handlers registered directly on the Session as well as the framework's.
func (s *Session) spawn(event string, h func()) {
	s.running.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("discord: panic in %s handler: %v\n%s", event, r, debug.Stack())
			}
		}()
		h()
	})
}

func decodeEvent(event string, data json.RawMessage, v any) bool {
	if err := json.Unmarshal(data, v); err != nil {
		log.Printf("discord: invalid %s event: %v", event, err)
		return false
	}
	return true
}
