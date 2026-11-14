package minidis

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

// closeTimeout is how long Close gives running handlers to return.
const closeTimeout = 10 * time.Second

// Minidis is a Discord bot. Register commands and handlers, sync the commands
// with SyncCommands, then connect with Run or Open.
//
// Its methods are safe for concurrent use, so handlers may be registered while
// the bot is running. A registered command must not be changed afterwards.
type Minidis struct {
	// Session is the Discord client, for the REST endpoints and gateway events
	// that minidis does not wrap.
	Session *client.Session

	mu                sync.RWMutex
	commands          map[string]*Command
	messageCommands   map[string]*MessageCommand
	userCommands      map[string]*UserCommand
	components        idHandlers[func(*ComponentContext) error]
	componentFallback func(*ComponentContext) error
	modals            idHandlers[func(*ModalSubmitContext) error]
	modalFallback     func(*ModalSubmitContext) error
	channelHandlers   map[string]func(*ChannelMessageContext) error
	middleware        []Middleware
	onError           func(*Context, error)
	onChannelError    func(*ChannelMessageContext, error)
	entryPoint        *EntryPointCommand

	appIDMu sync.Mutex
	appID   string // looked up by SyncCommands when the session is not open

	httpBot atomic.Pointer[discord.User] // looked up by HTTPHandler

	// openMu serializes Open, and Shutdown's closing of the session against
	// it; opening cancels an Open in progress, guarded by runMu.
	openMu  sync.Mutex
	opening context.CancelFunc

	runMu sync.RWMutex
	run   *runState // replaced by Open after Shutdown
}

// runState is what handlers share between Open (or New) and Shutdown.
type runState struct {
	ctx    context.Context // the parent of handler contexts
	cancel context.CancelFunc

	// Guarded by Minidis.runMu. accepting is whether HTTPHandler takes new
	// interactions; shutdown is set once Shutdown has begun.
	accepting bool
	shutdown  bool

	// http tracks handlers of interactions received by HTTPHandler, which the
	// Session does not know about; httpRunning counts them for reporting.
	http        sync.WaitGroup
	httpRunning atomic.Int64
}

func newRunState(accepting bool) *runState {
	ctx, cancel := context.WithCancel(context.Background())
	return &runState{ctx: ctx, cancel: cancel, accepting: accepting}
}

// New creates a bot that authenticates with the given bot token.
func New(token string) *Minidis {
	m := &Minidis{
		Session:         client.New("Bot " + token),
		commands:        map[string]*Command{},
		messageCommands: map[string]*MessageCommand{},
		userCommands:    map[string]*UserCommand{},
		channelHandlers: map[string]func(*ChannelMessageContext) error{},
		run:             newRunState(true),
	}

	m.Session.OnInteractionCreate(m.handleInteraction)
	m.Session.OnMessageCreate(m.handleChannelMessage)

	return m
}

// Open connects to the Discord gateway and returns once the bot is ready, or
// with ctx's error if ctx ends first. The connection is kept in the background
// until Close. A bot that only receives interactions through HTTPHandler does
// not need it.
func (m *Minidis) Open(ctx context.Context) error {
	m.openMu.Lock()
	defer m.openMu.Unlock()

	// After a Shutdown, a new run takes over; it accepts HTTP interactions only
	// once the gateway is open, so a failed Open leaves the bot closed.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	m.runMu.Lock()
	run := m.run
	reopening := run.shutdown
	if reopening {
		run = newRunState(false)
		m.run = run
	}
	m.opening = cancel
	m.runMu.Unlock()

	defer func() {
		m.runMu.Lock()
		m.opening = nil
		m.runMu.Unlock()
	}()

	if err := m.Session.Open(ctx); err != nil {
		if reopening {
			m.runMu.Lock()
			run.shutdown = true
			m.runMu.Unlock()
			run.cancel()
		}
		return err
	}

	m.runMu.Lock()
	// A Shutdown that ran while the gateway was connecting found no session
	// to close, so close the one that just opened.
	closed := run.shutdown
	if !closed {
		run.accepting = true
	}
	m.runMu.Unlock()

	if closed {
		return errors.Join(errors.New("minidis: closed while opening"), m.Session.Close())
	}
	return nil
}

// Close stops the bot and gives running handlers up to 10 seconds to finish.
// Use Shutdown to choose how long.
//
// A handler that calls Close waits for itself, so call it in a new goroutine
// there. A bot that only receives interactions through HTTPHandler cannot be
// opened again: Open would connect the gateway.
func (m *Minidis) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()

	return m.Shutdown(ctx)
}

// Shutdown stops the bot like http.Server.Shutdown: it disconnects from the
// gateway and refuses new HTTP interactions, then waits for running handlers
// to finish. If ctx ends first, it cancels their contexts and returns ctx's
// error without waiting further. It also stops an Open in progress.
func (m *Minidis) Shutdown(ctx context.Context) error {
	// Refusing under the lock means HTTPHandler cannot start a handler after
	// the wait below has begun.
	m.runMu.Lock()
	run := m.run
	run.accepting = false
	run.shutdown = true
	if m.opening != nil {
		m.opening()
	}
	m.runMu.Unlock()

	// Handler contexts end with the shutdown, whether the handlers finished or
	// ctx ran out.
	defer run.cancel()

	// Waiting out the canceled Open means no Open can connect a new run
	// between this Shutdown deciding what to stop and stopping it.
	m.openMu.Lock()
	err := m.Session.Shutdown(ctx)
	m.openMu.Unlock()

	drained := make(chan struct{})
	go func() {
		run.http.Wait()
		close(drained)
	}()

	select {
	case <-drained:
		return err
	case <-ctx.Done():
		if n := run.httpRunning.Load(); n > 0 {
			err = errors.Join(err, fmt.Errorf("minidis: %d HTTP interaction handlers still running: %w", n, ctx.Err()))
		}
		return err
	}
}

// Run opens the bot and keeps it running until ctx ends, then closes it. Pair
// it with signal.NotifyContext to stop on Ctrl+C. It also returns, with the
// reason, if Discord ends the connection for good, such as for an invalid
// token or intents the bot is not allowed.
func (m *Minidis) Run(ctx context.Context) error {
	if err := m.Open(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return m.Close()
	case <-m.Session.Done():
		return errors.Join(m.Session.Err(), m.Close())
	}
}

// current returns the run that handlers starting now belong to.
func (m *Minidis) current() *runState {
	m.runMu.RLock()
	defer m.runMu.RUnlock()

	return m.run
}

// SetIntents sets the gateway events the bot receives. Slash commands and other
// interactions need none. Call it before Open: unlike the other methods, it is
// not safe while the bot is connected.
func (m *Minidis) SetIntents(intents discord.Intent) {
	m.Session.Intents = intents
}

// SetPresence sets the bot's status and activities. It can be called before
// Open, and is kept across reconnects.
func (m *Minidis) SetPresence(status discord.Status, activities ...*discord.Activity) error {
	return m.Session.UpdatePresence(&discord.PresenceUpdate{Status: status, Activities: activities})
}

// OnReady adds a handler that runs each time the bot connects.
func (m *Minidis) OnReady(h func(*client.Session, *discord.Ready)) {
	m.Session.OnReady(h)
}

// OnMessageCreate adds a handler for every message the bot can see, including
// its own and those also handled by HandleChannelMessage.
func (m *Minidis) OnMessageCreate(h func(*client.Session, *discord.MessageCreate)) {
	m.Session.OnMessageCreate(h)
}
