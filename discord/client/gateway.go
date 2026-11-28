package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
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

// waitIdentify waits for sh's turn to Identify. The turn is taken even if
// ctx ends first.
func (s *Session) waitIdentify(ctx context.Context, sh *shard) error {
	key := s.Token + "|" + strconv.Itoa(sh.id%max(sh.maxConcurrency, 1))

	identifySlots.mu.Lock()
	now := time.Now()
	at := now
	if next := identifySlots.next[key]; next.After(at) {
		at = next
	}
	// Past slots no longer delay anyone; drop them so old tokens do not pile up.
	maps.DeleteFunc(identifySlots.next, func(_ string, next time.Time) bool { return next.Before(now) })
	identifySlots.next[key] = at.Add(identifyInterval)
	identifySlots.mu.Unlock()

	return sleep(ctx, time.Until(at))
}

// GatewayBot fetches the gateway URL, the number of shards Discord recommends
// for the bot, and how many sessions it may still start today.
func (s *Session) GatewayBot(ctx context.Context) (*discord.GatewayBot, error) {
	info, err := fetch[*discord.GatewayBot](ctx, s, "/gateway/bot")
	if err != nil {
		return nil, err
	}
	// An empty or null answer decodes into nothing, without an error.
	if info == nil {
		return nil, errors.New("discord: GET /gateway/bot: the response has no gateway information")
	}
	return info, nil
}

// ErrShuttingDown is returned by an Open that races a Shutdown: one that was
// canceled by it, or that was waiting while it ran. The session stays closed.
var ErrShuttingDown = errors.New("discord: session is shutting down")

// gatewayRun is one Open: the connections of its shards and the handlers
// they started.
type gatewayRun struct {
	cancel  context.CancelFunc // stops every shard
	done    chan struct{}      // closed when every shard has stopped
	running *sync.WaitGroup    // the handlers it started

	// set is the shards it connects. It is replaced, with more shards,
	// when Discord refuses the number and auto allows choosing another.
	set  atomic.Pointer[shardSet]
	auto bool // the number of shards is Discord's, not TotalShards

	// A fetch of GET /gateway/bot that shards identifying again share.
	infoMu sync.Mutex
	info   *discord.GatewayBot
	infoAt time.Time

	errOnce sync.Once
	err     error // why it stopped on its own; written before done closes

	opened   atomic.Bool // Open succeeded with this run
	shutDown atomic.Bool // stopped by Shutdown rather than by Discord

	lastErr atomic.Pointer[error] // why the last attempt before a READY failed
}

// fail stops every shard of r, because Discord ruled out reconnecting one.
func (r *gatewayRun) fail(err error) {
	r.errOnce.Do(func() { r.err = err })
	r.cancel()
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

// Open connects each shard to the Gateway (see TotalShards and Shards) and
// returns once every one is READY. After that, the connections are maintained
// in the background until Shutdown, Close, or a close code from Discord that
// rules out reconnecting one of them, which stops them all (see Done and Err).
//
// When the number of shards is Discord's, and Discord later closes a shard
// for having too many guilds (close code 4011), every shard reconnects with
// as many shards as Discord then recommends, at least twice as many.
//
// Canceling ctx aborts the connection attempt, including waiting for another
// Open or Shutdown to finish, and returns ctx's error; it has no effect once
// Open has returned. Open gives up after 30 seconds without READY, plus 5
// seconds for each shard ahead of the last one in its Identify bucket, as
// Discord lets the shards of a bucket (shard ID modulo max_concurrency)
// identify only one at a time. A Shutdown or Close called meanwhile cancels it, and an
// Open that races a Shutdown fails with ErrShuttingDown rather than reopening
// the session after it.
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

	if err := s.checkShards(); err != nil {
		return err
	}

	start := time.Now()
	openCtx, cancelOpen := context.WithCancel(ctx)
	defer cancelOpen()

	if !s.startOpening(cancelOpen, shutdowns) {
		return ErrShuttingDown
	}
	defer s.startOpening(nil, 0)

	// From here on Done and Err describe this Open, not the previous one.
	s.run.Store(nil)

	fetchCtx, cancelFetch := context.WithTimeout(openCtx, gatewayOpenTimeout)
	defer cancelFetch()

	info, err := s.GatewayBot(fetchCtx)
	if err != nil {
		return openError(ctx, fetchCtx, err)
	}

	// Each Open starts new gateway sessions: those of a previous connection
	// were ended by Close or by Discord. This fetch covers their first
	// Identify.
	count, ids := s.shardIDs(info)
	if err := checkSessionStarts(info, len(ids)); err != nil {
		return err
	}
	set := newShardSet(info, count, ids)
	run := &gatewayRun{auto: s.TotalShards == 0}
	run.set.Store(set)

	waitCtx, cancelWait := context.WithDeadline(openCtx, start.Add(set.openTimeout()))
	defer cancelWait()

	// The connections outlive ctx, so they get a context of their own.
	runCtx, cancel := context.WithCancel(context.Background())
	run.cancel, run.done, run.running = cancel, make(chan struct{}), &sync.WaitGroup{}
	s.running = run.running
	s.active.Store(run)

	// Handlers started by any run are drained by the next Shutdown, including
	// those of a run that failed below after READY arrived too late.
	s.drain = append(s.drain, run.running)

	ready := make(chan struct{}, len(set.shards))
	go s.gatewayLoop(runCtx, run, ready)

	for n := 0; n < len(set.shards) && err == nil; {
		select {
		case <-ready:
			n++
		case <-run.done:
			// A shard failed in a way that rules out retrying.
			err = run.err
			if err == nil {
				err = errors.New("discord: gateway stopped before READY")
			}
		case <-waitCtx.Done():
			err = openError(ctx, waitCtx, waitCtx.Err())
			if last := run.lastErr.Load(); last != nil && errors.Is(waitCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
				err = fmt.Errorf("%w; last attempt: %w", err, *last)
			}
		}
	}

	if err != nil {
		cancel()
		<-run.done
		// ShardOf goes back to TotalShards.
		s.active.CompareAndSwap(run, nil)
		return err
	}

	run.opened.Store(true)
	s.run.Store(run)
	return nil
}

// openError reports why an Open failed: the caller canceled ctx, the timeout
// of timeoutCtx passed, a Shutdown canceled it, or err.
func openError(ctx, timeoutCtx context.Context, err error) error {
	switch {
	case ctx.Err() != nil:
		return context.Cause(ctx)
	case errors.Is(timeoutCtx.Err(), context.DeadlineExceeded):
		return errors.New("discord: timed out waiting for READY")
	case timeoutCtx.Err() != nil:
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

// Done returns a channel that is closed when the connections made by the last
// Open stop for good: after Shutdown or Close, or when Discord closes one of
// them with a code that rules out reconnecting, such as an invalid token or
// disallowed intents, which stops the others too. Err then says why. Done returns nil before the first Open and
// after an Open that failed, and after Shutdown the closed channel of the last
// Open.
func (s *Session) Done() <-chan struct{} {
	if r := s.run.Load(); r != nil {
		return r.done
	}
	return nil
}

// Err returns why the connections made by the last Open stopped on their own,
// such as a *GatewayCloseError. It is nil while they are up, after
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

	s.presence.Store(&update)

	r := s.active.Load()
	if r == nil {
		return nil
	}

	// Each shard sends the latest presence after it was stored, so that
	// Discord does not end up with an older one, even with concurrent calls.
	var errs []error
	for _, sh := range r.set.Load().shards {
		if err := s.applyPresence(sh); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", sh.name(), err))
		}
	}
	return errors.Join(errs...)
}

// applyPresence sends the stored presence on the connection of sh, if it has
// one that has not sent it.
func (s *Session) applyPresence(sh *shard) error {
	sh.presenceMu.Lock()
	defer sh.presenceMu.Unlock()

	if c := sh.conn.Load(); c != nil {
		return s.sendPresence(sh, c)
	}
	return nil
}

// sendPresence sends the stored presence on c, the connection of sh, unless
// c has sent it already, or, if c is out of gateway events, schedules it for
// when there is room. sh.presenceMu must be held.
func (s *Session) sendPresence(sh *shard, c *gatewayConn) error {
	p := s.presence.Load()
	if p == nil || p == c.presenceSent {
		return nil
	}
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
			sh.presenceMu.Lock()
			defer sh.presenceMu.Unlock()

			c.presenceTimer = nil
			if sh.conn.Load() == c {
				_ = s.sendPresence(sh, c)
			}
		})
	}
	return nil
}

// markConnected makes c, the connection of sh, available to gateway commands
// and UpdatePresence, and applies the presence, unless it went with the
// Identify, which covers any update made while there was no connection.
func (s *Session) markConnected(sh *shard, c *gatewayConn) error {
	sh.presenceMu.Lock()
	defer sh.presenceMu.Unlock()

	sh.conn.Store(c)
	return s.sendPresence(sh, c)
}

// disconnected forgets c, the connection of sh, which has closed.
func (s *Session) disconnected(sh *shard, c *gatewayConn) {
	sh.presenceMu.Lock()
	defer sh.presenceMu.Unlock()

	sh.conn.CompareAndSwap(c, nil)
	if c.presenceTimer != nil {
		c.presenceTimer.Stop()
		c.presenceTimer = nil
	}
}

// connOutcome says what to do after a connection ends.
type connOutcome int

const (
	reconnect      connOutcome = iota // reconnect, resuming if there is a session
	reconnectFresh                    // the session is gone: reconnect and Identify
	invalidSession                    // like reconnectFresh, after Discord's 1–5s wait
	stopGateway                       // Discord says retrying cannot succeed
)

// gatewayLoop runs the shards of run until ctx ends or Discord rules out
// reconnecting one of them, and closes run.done once they have all stopped.
// When Discord asks for more shards than an open run has, and the number is
// Discord's to choose, it stops them all and starts a new set.
func (s *Session) gatewayLoop(ctx context.Context, run *gatewayRun, ready chan<- struct{}) {
	defer close(run.done)

	for {
		set := run.set.Load()
		setCtx, stopSet := context.WithCancel(ctx)

		var reshard atomic.Bool
		var wg sync.WaitGroup
		for _, sh := range set.shards {
			wg.Go(func() {
				err := s.shardLoop(setCtx, run, sh, ready)
				switch {
				case err == nil:
				case run.auto && run.opened.Load() && isShardingRequired(err):
					reshard.Store(true)
					stopSet()
				default:
					if run.opened.Load() {
						s.logger().Error("discord: shard stopped", "shard", sh.name(), "err", err)
					}
					run.fail(err)
				}
			})
		}
		wg.Wait()
		stopSet()

		if !reshard.Load() || ctx.Err() != nil {
			return
		}

		next, err := s.reshard(ctx, run)
		if err != nil {
			if ctx.Err() == nil {
				s.logger().Error("discord: gateway stopped", "err", err)
				run.fail(err)
			}
			return
		}
		run.set.Store(next)
	}
}

// shardLoop runs connections of sh until ctx ends, returning nil, or Discord
// rules out reconnecting, returning why. It reports the first READY on
// ready. Other failures before READY are retried until Open gives up and
// cancels ctx.
func (s *Session) shardLoop(ctx context.Context, run *gatewayRun, sh *shard, ready chan<- struct{}) error {
	isOpen := false
	delay := minReconnectDelay

	onReady := func() {
		delay = minReconnectDelay
		if !isOpen {
			isOpen = true
			// Only Open waits for this, and only for the first set.
			select {
			case ready <- struct{}{}:
			default:
			}
		}
	}

	for {
		outcome, err := s.runConnection(ctx, run, sh, onReady)

		if ctx.Err() != nil {
			return nil
		}

		if outcome == stopGateway {
			return err
		}

		if outcome == reconnectFresh || outcome == invalidSession {
			sh.resetSession()
		}

		switch {
		case !isOpen:
			if err == nil {
				err = fmt.Errorf("discord: %s disconnected before READY", sh.name())
			}
			run.lastErr.Store(&err)
		case err != nil:
			s.logger().Warn("discord: shard disconnected, reconnecting", "shard", sh.name(), "err", err)
		}

		wait := delay/2 + rand.N(delay/2+1)
		if outcome == invalidSession {
			wait = invalidSessionMinDelay + rand.N(invalidSessionMaxDelay-invalidSessionMinDelay+1)
		}
		if sleep(ctx, wait) != nil {
			return nil
		}
		delay = min(delay*2, maxReconnectDelay)
	}
}

// prepareIdentify applies Discord's limits before sh starts a new session.
// The session start limit of the first Identify after Open is covered by the
// check in Open.
func (s *Session) prepareIdentify(ctx context.Context, run *gatewayRun, sh *shard) error {
	if sh.identified {
		info, err := run.gatewayBot(ctx, s)
		if err != nil {
			return err
		}
		sh.gatewayURL = info.URL
		sh.maxConcurrency = info.SessionStartLimit.MaxConcurrency

		if info.SessionStartLimit.Remaining <= 0 {
			wait := time.Duration(info.SessionStartLimit.ResetAfter) * time.Millisecond
			s.logger().Warn("discord: no session starts remaining, waiting", "shard", sh.name(), "wait", wait)
			if err := sleep(ctx, wait); err != nil {
				return err
			}
		}
	}

	return s.waitIdentify(ctx, sh)
}

func gatewayEndpoint(base string, compress bool) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("discord: invalid gateway url: %w", err)
	}

	q := u.Query()
	q.Set("v", strconv.Itoa(APIVersion))
	q.Set("encoding", "json")
	if compress {
		q.Set("compress", "zlib-stream")
	}
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

	// Guarded by shard.presenceMu.
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

func sendHeartbeat(sh *shard, c *gatewayConn) error {
	if seq := sh.sequence.Load(); seq != 0 {
		return c.send(opHeartbeat, seq)
	}
	return c.send(opHeartbeat, nil)
}

// heartbeat runs until ctx ends. Closing the socket without a close frame keeps
// the session resumable and unblocks the read loop.
func heartbeat(ctx context.Context, logger *slog.Logger, sh *shard, c *gatewayConn, interval time.Duration) {
	timer := time.NewTimer(time.Duration(rand.Float64() * float64(interval)))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		if !c.acked.Swap(false) {
			logger.Warn("discord: no heartbeat ACK, reconnecting", "shard", sh.name())
			c.ws.Close()
			return
		}

		if err := sendHeartbeat(sh, c); err != nil {
			c.ws.Close()
			return
		}

		timer.Reset(interval)
	}
}

func (s *Session) sendIdentify(sh *shard, c *gatewayConn) error {
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
	if s.State != nil {
		identify.Intents |= s.State.Intents()
	}

	if sh.count > 1 {
		identify.Shard = &[2]int{sh.id, sh.count}
	}

	// The presence goes with the Identify, so that the bot does not show as
	// plainly online before it is applied.
	sh.presenceMu.Lock()
	identify.Presence = s.presence.Load()
	c.presenceSent = identify.Presence
	sh.presenceMu.Unlock()

	sh.identified = true

	return c.send(opIdentify, identify)
}

func (s *Session) sendResume(sh *shard, c *gatewayConn) error {
	return c.send(opResume, struct {
		Token     string `json:"token"`
		SessionID string `json:"session_id"`
		Seq       int64  `json:"seq"`
	}{s.gatewayToken(), sh.sessionID, sh.sequence.Load()})
}

// gatewayToken is the token for Identify and Resume, which take it without
// the "Bot " prefix of the Authorization header.
func (s *Session) gatewayToken() string {
	return strings.TrimPrefix(s.Token, "Bot ")
}

// runConnection opens one websocket for sh and reads from it until it ends.
func (s *Session) runConnection(ctx context.Context, run *gatewayRun, sh *shard, onReady func()) (connOutcome, error) {
	resuming := sh.sessionID != ""

	target := sh.resumeURL
	if !resuming {
		if err := s.prepareIdentify(ctx, run, sh); err != nil {
			var restErr *RESTError
			if errors.As(err, &restErr) && restErr.StatusCode == http.StatusUnauthorized {
				return stopGateway, err
			}
			return reconnect, err
		}
		target = sh.gatewayURL
	}

	endpoint, err := gatewayEndpoint(target, !s.DisableCompression)
	if err != nil {
		return reconnectFresh, err
	}

	ws, _, err := websocket.DefaultDialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return reconnect, fmt.Errorf("discord: dial gateway: %w", err)
	}
	defer ws.Close()

	// Heartbeats only start with Hello, so until then a stalled socket is
	// caught by a read deadline.
	if err := ws.SetReadDeadline(time.Now().Add(gatewayOpenTimeout)); err != nil {
		return reconnect, err
	}

	conn := &gatewayConn{ws: ws}
	conn.acked.Store(true)
	defer s.disconnected(sh, conn)

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

	// With compression, Discord sends binary messages; otherwise, text.
	var z inflater
	for {
		kind, data, err := ws.ReadMessage()
		if err != nil {
			return classifyDisconnect(err)
		}
		if kind == websocket.BinaryMessage {
			if data, err = z.inflate(data); err != nil {
				return reconnect, fmt.Errorf("discord: %s: inflate: %w", sh.name(), err)
			}
			if data == nil {
				continue
			}
		}

		var p gatewayPayload
		if err := json.Unmarshal(data, &p); err != nil {
			s.logger().Error("discord: invalid payload", "shard", sh.name(), "err", err)
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
			if hello.HeartbeatInterval <= 0 {
				return reconnect, fmt.Errorf("discord: invalid Hello: heartbeat interval %d", hello.HeartbeatInterval)
			}
			if err := ws.SetReadDeadline(time.Time{}); err != nil {
				return reconnect, err
			}

			go heartbeat(connCtx, s.logger(), sh, conn, time.Duration(hello.HeartbeatInterval)*time.Millisecond)

			if resuming {
				err = s.sendResume(sh, conn)
			} else {
				err = s.sendIdentify(sh, conn)
			}
			if err != nil {
				return reconnect, err
			}

		case opHeartbeat:
			if err := sendHeartbeat(sh, conn); err != nil {
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
				sh.sequence.Store(*p.S)
			}

			switch p.T {
			case "READY":
				var ready discord.Ready
				if err := json.Unmarshal(p.D, &ready); err != nil {
					return reconnectFresh, fmt.Errorf("discord: invalid READY: %w", err)
				}

				sh.sessionID = ready.SessionID
				sh.resumeURL = ready.ResumeGatewayURL
				if sh.resumeURL == "" {
					sh.resumeURL = sh.gatewayURL
				}

				s.user.Store(ready.User)
				if ready.Application != nil {
					id := ready.Application.ID
					s.appID.Store(&id)
				}

				if err := s.markConnected(sh, conn); err != nil {
					return reconnect, err
				}
				onReady()
			case "RESUMED":
				if err := s.markConnected(sh, conn); err != nil {
					return reconnect, err
				}
				onReady()
			case "USER_UPDATE":
				// The bot's own user changed, so that User stays current.
				var u discord.User
				if err := json.Unmarshal(p.D, &u); err == nil && u.ID != "" {
					s.user.Store(&u)
				}
			}

			s.dispatch(p.T, p.D, nil)
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

// Dispatch runs the handlers of a gateway event as if the gateway had sent
// it, and returns once they have returned. It is for tests, which feed a bot
// events without connecting it; package minidistest builds on it. Like the
// gateway, a READY or USER_UPDATE sets User, and READY ApplicationID.
func (s *Session) Dispatch(event string, data json.RawMessage) {
	switch event {
	case "READY":
		var ready discord.Ready
		if json.Unmarshal(data, &ready) == nil {
			s.user.Store(ready.User)
			if ready.Application != nil {
				id := ready.Application.ID
				s.appID.Store(&id)
			}
		}
	case "USER_UPDATE":
		var u discord.User
		if json.Unmarshal(data, &u) == nil && u.ID != "" {
			s.user.Store(&u)
		}
	}

	var done sync.WaitGroup
	s.dispatch(event, data, &done)
	done.Wait()
}

// dispatch updates the state with an event, then decodes it and starts its
// handlers, so that they see the state with the event applied. done, if not
// nil, also tracks the handlers.
func (s *Session) dispatch(event string, data json.RawMessage, done *sync.WaitGroup) {
	if s.State != nil {
		s.State.apply(s.logger(), event, data)
	}

	s.handlersMu.RLock()
	defer s.handlersMu.RUnlock()

	for _, h := range s.onRawEvents {
		s.spawn(event, done, func() { h(s, event, data) })
	}

	e := s.events[event]
	if e == nil {
		return
	}

	// The event is decoded once, and its handlers share the result.
	v, err := e.decode(data)
	if err != nil {
		s.logger().Error("discord: invalid event", "event", event, "err", err)
		return
	}
	for _, h := range e.handlers {
		s.spawn(event, done, func() { h(s, v) })
	}
}

// spawn runs a handler in its own goroutine, tracked for Shutdown. A panic stops
// only that handler. This is the single place handler goroutines start, so it
// covers handlers registered directly on the Session as well as the framework's.
func (s *Session) spawn(event string, done *sync.WaitGroup, h func()) {
	if done != nil {
		done.Add(1)
	}
	s.running.Go(func() {
		if done != nil {
			defer done.Done()
		}
		defer func() {
			if r := recover(); r != nil {
				s.logger().Error("discord: panic in handler", "event", event, "panic", r, "stack", string(debug.Stack()))
			}
		}()
		h()
	})
}
