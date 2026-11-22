package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tbdsux/minidis/discord"
)

const (
	opDispatch                = 0
	opHeartbeat               = 1
	opIdentify                = 2
	opPresenceUpdate          = 3
	opVoiceStateUpdate        = 4
	opResume                  = 6
	opReconnect               = 7
	opRequestGuildMembers     = 8
	opInvalidSession          = 9
	opHello                   = 10
	opHeartbeatACK            = 11
	opRequestSoundboardSounds = 31
	opRequestChannelInfo      = 43
)

const gatewayWriteTimeout = 10 * time.Second

// Variables so tests can shorten them.
var (
	gatewayOpenTimeout = 30 * time.Second
	identifyInterval   = 5 * time.Second // between Identifies in one max_concurrency bucket
	minReconnectDelay  = time.Second
	maxReconnectDelay  = time.Minute
	closeDrainTimeout  = 10 * time.Second

	// After an Invalid Session, Discord asks for a random wait of 1 to 5
	// seconds before identifying again.
	invalidSessionMinDelay = time.Second
	invalidSessionMaxDelay = 5 * time.Second

	// Discord allows 120 gateway events per connection per minute and
	// disconnects beyond that. Heartbeats are kept out of this budget, which
	// leaves them room.
	gatewaySendLimit  = 110
	gatewaySendWindow = time.Minute
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

// identifySlots spaces Identifies across every Session in the process. Discord
// allows one per identifyInterval in each bucket, and a bot's shard s uses
// bucket s % max_concurrency, so Sessions for shards of one bot share these.
var identifySlots = struct {
	mu   sync.Mutex
	next map[string]time.Time // bucket key → when its next Identify may go
}{next: map[string]time.Time{}}

// waitIdentify waits for this Session's turn to Identify. The turn is taken
// even if ctx ends first.
func (s *Session) waitIdentify(ctx context.Context) error {
	key := s.Token + "|" + strconv.Itoa(s.ShardID%max(s.maxConcurrency, 1))

	identifySlots.mu.Lock()
	at := time.Now()
	if next := identifySlots.next[key]; next.After(at) {
		at = next
	}
	identifySlots.next[key] = at.Add(identifyInterval)
	identifySlots.mu.Unlock()

	return sleep(ctx, time.Until(at))
}

// GatewayBot fetches the gateway URL, the number of shards Discord recommends
// for the bot, and how many sessions it may still start today.
func (s *Session) GatewayBot(ctx context.Context) (*discord.GatewayBot, error) {
	return fetch[*discord.GatewayBot](ctx, s, "/gateway/bot")
}

// ErrShuttingDown is returned by an Open that races a Shutdown: one that was
// canceled by it, or that was waiting while it ran. The session stays closed.
var ErrShuttingDown = errors.New("discord: session is shutting down")

// gatewayRun is one Open: the gateway goroutine and the handlers it started.
type gatewayRun struct {
	cancel  context.CancelFunc // stops the gateway goroutine
	done    chan struct{}      // closed when the gateway goroutine has stopped
	err     error              // why it stopped on its own; written before done closes
	running *sync.WaitGroup    // the handlers it started

	shutDown atomic.Bool // stopped by Shutdown rather than by Discord

	lastErr atomic.Pointer[error] // why the last attempt before READY failed
}

func (r *gatewayRun) stopped() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// lock takes the lifecycle lock, or returns ctx's cause if ctx ends first.
func (s *Session) lock(ctx context.Context) error {
	// A free lock is taken even with ctx done: select would otherwise pick
	// at random between the two.
	select {
	case s.lifecycle <- struct{}{}:
		return nil
	default:
	}

	select {
	case s.lifecycle <- struct{}{}:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (s *Session) unlock() { <-s.lifecycle }

// Open connects to the Gateway and returns once the session is READY. After
// that, the connection is maintained in the background until Shutdown, Close,
// or a close code from Discord that rules out reconnecting (see Done and Err).
//
// Canceling ctx aborts the connection attempt, including waiting for another
// Open or Shutdown to finish, and returns ctx's error; it has no effect once
// Open has returned. Open gives up after 30 seconds without READY. A Shutdown
// or Close called meanwhile cancels it, and an Open that races a Shutdown
// fails with ErrShuttingDown rather than reopening the session after it.
func (s *Session) Open(ctx context.Context) error {
	s.openingMu.Lock()
	shutdowns := s.shutdowns
	s.openingMu.Unlock()

	if err := s.lock(ctx); err != nil {
		return err
	}
	defer s.unlock()

	if r := s.run.Load(); r != nil && !r.stopped() {
		return ErrAlreadyOpen
	}

	openCtx, cancelOpen := context.WithTimeout(ctx, gatewayOpenTimeout)
	defer cancelOpen()

	if !s.startOpening(cancelOpen, shutdowns) {
		return ErrShuttingDown
	}
	defer s.startOpening(nil, 0)

	// From here on Done and Err describe this Open, not the previous one.
	s.run.Store(nil)

	// A previous connection that Discord ended starts over with a new session.
	s.resetSession()

	info, err := s.GatewayBot(openCtx)
	if err != nil {
		return openError(ctx, openCtx, err)
	}

	// Discord resets the token of bots that exceed the daily Identify limit.
	if info.SessionStartLimit.Remaining <= 0 {
		return fmt.Errorf("discord: no session starts remaining, resets in %s",
			time.Duration(info.SessionStartLimit.ResetAfter)*time.Millisecond)
	}

	s.gatewayURL = info.URL
	s.maxConcurrency = info.SessionStartLimit.MaxConcurrency
	s.identified = false // this fetch covers the first Identify

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
		if last := run.lastErr.Load(); last != nil && errors.Is(openCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			err = fmt.Errorf("%w; last attempt: %w", err, *last)
		}
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
		return context.Cause(ctx)
	case errors.Is(openCtx.Err(), context.DeadlineExceeded):
		return errors.New("discord: timed out waiting for READY")
	case openCtx.Err() != nil:
		return ErrShuttingDown
	default:
		return err
	}
}

// startOpening records the cancel func of an Open in progress, or clears it
// when cancel is nil. It reports false if a Shutdown is waiting or ran since
// the Open began (shutdowns is the count it saw then), in which case the Open
// should not start.
func (s *Session) startOpening(cancel context.CancelFunc, shutdowns uint64) bool {
	s.openingMu.Lock()
	defer s.openingMu.Unlock()

	if cancel != nil && (s.pendingShutdowns > 0 || s.shutdowns != shutdowns) {
		return false
	}
	s.opening = cancel
	return true
}

// Done returns a channel that is closed when the connection made by the last
// Open stops for good: after Shutdown or Close, or when Discord closes it with
// a code that rules out reconnecting, such as an invalid token or disallowed
// intents. Err then says why. Done returns nil before the first Open and
// after an Open that failed, and after Shutdown the closed channel of the last
// Open.
func (s *Session) Done() <-chan struct{} {
	if r := s.run.Load(); r != nil {
		return r.done
	}
	return nil
}

// Err returns why the connection made by the last Open stopped on its own,
// such as a *GatewayCloseError. It is nil while the connection is up, after
// Shutdown, before the first Open and after an Open that failed. Open can be
// called again afterwards.
func (s *Session) Err() error {
	r := s.run.Load()
	if r == nil || !r.stopped() || r.shutDown.Load() {
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
// case. An Open in progress is canceled first. Waiting for handlers does not
// block other calls: a later Open can start meanwhile, and a later Shutdown
// waits for the same handlers within its own ctx. Calling Shutdown from a
// handler waits for that handler too, so it would use up the whole of ctx.
func (s *Session) Shutdown(ctx context.Context) error {
	s.openingMu.Lock()
	s.pendingShutdowns++
	s.shutdowns++
	if s.opening != nil {
		s.opening()
	}
	s.openingMu.Unlock()

	waiting := s.stop()

	s.openingMu.Lock()
	s.pendingShutdowns--
	s.openingMu.Unlock()

	return s.wait(ctx, waiting)
}

// stop ends the current run under the lifecycle lock and returns the handlers
// to wait for.
//
// It takes the lock whatever the context, so that a Shutdown with no time left
// still stops the gateway. That cannot block for long: Shutdown has canceled
// any Open in progress, and nothing else holds the lock across a wait.
func (s *Session) stop() []*sync.WaitGroup {
	s.lifecycle <- struct{}{}
	defer s.unlock()

	if r := s.run.Load(); r != nil && !r.shutDown.Load() {
		r.shutDown.Store(true)
		r.cancel()
		<-r.done
		s.resetSession()
	}

	// No new handlers can start in these groups now that their gateway
	// goroutines have stopped. Groups stay listed until a wait sees them
	// finish, so that every Shutdown waits for the handlers still running.
	return slices.Clone(s.drain)
}

// wait waits for the handlers in groups, or for ctx to end.
func (s *Session) wait(ctx context.Context, groups []*sync.WaitGroup) error {
	if len(groups) == 0 {
		return nil
	}

	// yagni: this goroutine outlives ctx until a stuck handler returns.
	drained := make(chan struct{})
	go func() {
		for _, running := range groups {
			running.Wait()
		}
		close(drained)
	}()

	select {
	case <-drained:
	case <-ctx.Done():
		return fmt.Errorf("discord: handlers still running: %w", context.Cause(ctx))
	}

	// Forget the groups that are done, unless another call holds the lock.
	select {
	case s.lifecycle <- struct{}{}:
		s.drain = slices.DeleteFunc(s.drain, func(g *sync.WaitGroup) bool { return slices.Contains(groups, g) })
		s.unlock()
	default:
	}
	return nil
}

// UpdatePresence sets the bot's status and activities. It may be called at any
// time; before the connection is ready, and after a reconnect, the presence is
// applied as soon as the session is ready. Updates beyond Discord's limit on
// gateway events are combined: the latest is sent once the limit allows.
func (s *Session) UpdatePresence(p *discord.PresenceUpdate) error {
	update := *p
	if update.Status == "" {
		update.Status = discord.StatusOnline
	}
	if update.Activities == nil {
		update.Activities = []*discord.Activity{} // Discord requires an array
	}

	// Storing and sending under one lock keeps Discord from ending up with an
	// older presence than the stored one.
	s.presenceMu.Lock()
	defer s.presenceMu.Unlock()

	s.presence.Store(&update)

	if c := s.conn.Load(); c != nil {
		return s.sendPresence(c)
	}
	return nil
}

// sendPresence sends the stored presence on c, or, if c is out of gateway
// events, schedules it for when there is room. s.presenceMu must be held.
func (s *Session) sendPresence(c *gatewayConn) error {
	p := s.presence.Load()
	err := c.send(opPresenceUpdate, p)

	var over *overBudgetError
	if !errors.As(err, &over) {
		if err == nil {
			c.presenceSent = p
		}
		return err
	}

	if c.presenceTimer == nil {
		c.presenceTimer = time.AfterFunc(over.wait, func() {
			s.presenceMu.Lock()
			defer s.presenceMu.Unlock()

			c.presenceTimer = nil
			if s.conn.Load() == c && s.presence.Load() != c.presenceSent {
				_ = s.sendPresence(c)
			}
		})
	}
	return nil
}

// markConnected makes c available to UpdatePresence and applies the presence,
// unless it went with the Identify, which covers any update made while there
// was no connection.
func (s *Session) markConnected(c *gatewayConn) error {
	s.presenceMu.Lock()
	defer s.presenceMu.Unlock()

	s.conn.Store(c)

	if p := s.presence.Load(); p != nil && p != c.presenceSent {
		return s.sendPresence(c)
	}
	return nil
}

// disconnected forgets c, which has closed.
func (s *Session) disconnected(c *gatewayConn) {
	s.presenceMu.Lock()
	defer s.presenceMu.Unlock()

	s.conn.CompareAndSwap(c, nil)
	if c.presenceTimer != nil {
		c.presenceTimer.Stop()
		c.presenceTimer = nil
	}
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
	invalidSession                    // like reconnectFresh, after Discord's 1–5s wait
	stopGateway                       // Discord says retrying cannot succeed
)

// gatewayLoop runs connections until the context ends or Discord rules out
// reconnecting. It reports the first READY, or a failure before it that rules
// out retrying, on opened, and closes run.done when it returns. Other failures
// before READY are retried until Open gives up and cancels ctx.
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

		if outcome == stopGateway {
			if !isOpen {
				opened <- err
				return
			}
			log.Printf("discord: gateway stopped: %v", err)
			run.err = err
			return
		}

		if outcome == reconnectFresh || outcome == invalidSession {
			s.resetSession()
		}

		switch {
		case !isOpen:
			if err == nil {
				err = errors.New("discord: gateway disconnected before READY")
			}
			run.lastErr.Store(&err)
		case err != nil:
			log.Printf("discord: gateway disconnected, reconnecting: %v", err)
		}

		wait := delay/2 + rand.N(delay/2+1)
		if outcome == invalidSession {
			wait = invalidSessionMinDelay + rand.N(invalidSessionMaxDelay-invalidSessionMinDelay+1)
		}
		if sleep(ctx, wait) != nil {
			return
		}
		delay = min(delay*2, maxReconnectDelay)
	}
}

// prepareIdentify applies Discord's limits before a new session is started.
// The session start limit of the first Identify after Open is covered by the
// check in Open.
func (s *Session) prepareIdentify(ctx context.Context) error {
	if s.identified {
		info, err := s.GatewayBot(ctx)
		if err != nil {
			return err
		}
		s.gatewayURL = info.URL
		s.maxConcurrency = info.SessionStartLimit.MaxConcurrency

		if info.SessionStartLimit.Remaining <= 0 {
			wait := time.Duration(info.SessionStartLimit.ResetAfter) * time.Millisecond
			log.Printf("discord: no session starts remaining, waiting %s", wait)
			if err := sleep(ctx, wait); err != nil {
				return err
			}
		}
	}

	return s.waitIdentify(ctx)
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

	sent []time.Time // recent sends other than heartbeats, oldest first; guarded by writeMu

	// Guarded by Session.presenceMu.
	presenceSent  *discord.PresenceUpdate // the last presence sent on this connection
	presenceTimer *time.Timer             // sends a presence that was over budget
}

// overBudgetError means a send would exceed Discord's limit on gateway events.
type overBudgetError struct{ wait time.Duration }

func (e *overBudgetError) Error() string {
	return fmt.Sprintf("discord: gateway event limit reached, room again in %s", e.wait)
}

// take records a send against the budget, or reports how long until there is
// room. c.writeMu must be held.
func (c *gatewayConn) take(now time.Time) (time.Duration, bool) {
	cutoff := now.Add(-gatewaySendWindow)
	c.sent = slices.DeleteFunc(c.sent, func(t time.Time) bool { return !t.After(cutoff) })

	if len(c.sent) >= gatewaySendLimit {
		return c.sent[0].Sub(cutoff), false
	}
	c.sent = append(c.sent, now)
	return 0, true
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

	if op != opHeartbeat {
		if wait, ok := c.take(time.Now()); !ok {
			return &overBudgetError{wait: wait}
		}
	}

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
		Token          string                  `json:"token"`
		Properties     properties              `json:"properties"`
		Intents        discord.Intent          `json:"intents"`
		LargeThreshold int                     `json:"large_threshold"`
		Shard          *[2]int                 `json:"shard,omitempty"`
		Presence       *discord.PresenceUpdate `json:"presence,omitempty"`
	}{
		Token:          s.gatewayToken(),
		Properties:     properties{OS: runtime.GOOS, Browser: libraryName, Device: libraryName},
		Intents:        s.Intents,
		LargeThreshold: 250,
	}

	if s.ShardCount > 1 {
		identify.Shard = &[2]int{s.ShardID, s.ShardCount}
	}

	// The presence goes with the Identify, so that the bot does not show as
	// plainly online before it is applied.
	s.presenceMu.Lock()
	identify.Presence = s.presence.Load()
	c.presenceSent = identify.Presence
	s.presenceMu.Unlock()

	s.identified = true

	return c.send(opIdentify, identify)
}

func (s *Session) sendResume(c *gatewayConn) error {
	return c.send(opResume, struct {
		Token     string `json:"token"`
		SessionID string `json:"session_id"`
		Seq       int64  `json:"seq"`
	}{s.gatewayToken(), s.sessionID, s.sequence.Load()})
}

// gatewayToken is the token for Identify and Resume, which take it without
// the "Bot " prefix of the Authorization header.
func (s *Session) gatewayToken() string {
	return strings.TrimPrefix(s.Token, "Bot ")
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
	defer s.disconnected(conn)

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
			return invalidSession, nil

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

	e := s.events[event]
	if e == nil {
		return
	}

	// The event is decoded once, and its handlers share the result.
	v, err := e.decode(data)
	if err != nil {
		log.Printf("discord: invalid %s event: %v", event, err)
		return
	}
	for _, h := range e.handlers {
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
