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

	openMu sync.Mutex // serializes Open

	runMu sync.RWMutex
	run   *runState // replaced by Open after Shutdown
}

// runState is what handlers share between Open (or New) and Shutdown.
type runState struct {
	ctx    context.Context // the parent of handler contexts
	cancel context.CancelFunc

	// http tracks handlers of interactions received by HTTPHandler, which the
	// Session does not know about.
	http sync.WaitGroup
}

func newRunState() *runState {
	ctx, cancel := context.WithCancel(context.Background())
	return &runState{ctx: ctx, cancel: cancel}
}

// New creates a bot that authenticates with the given bot token.
func New(token string) *Minidis {
	m := &Minidis{
		Session:         client.New("Bot " + token),
		commands:        map[string]*Command{},
		messageCommands: map[string]*MessageCommand{},
		userCommands:    map[string]*UserCommand{},
		channelHandlers: map[string]func(*ChannelMessageContext) error{},
		run:             newRunState(),
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

	m.runMu.Lock()
	run := m.run
	reopening := run.ctx.Err() != nil
	if reopening {
		run = newRunState()
		m.run = run
	}
	m.runMu.Unlock()

	if err := m.Session.Open(ctx); err != nil {
		if reopening {
			run.cancel() // stay closed, as before this Open
		}
		return err
	}

	// A Shutdown that ran while the gateway was connecting found no session
	// to close, so close the one that just opened.
	if run.ctx.Err() != nil {
		return errors.Join(errors.New("minidis: closed while opening"), m.Session.Close())
	}
	return nil
}

// Close disconnects from Discord, cancels the context of running handlers, and
// gives them up to 10 seconds to return. Use Shutdown to choose how long.
//
// A handler that calls Close waits for itself, so call it in a new goroutine
// there. A bot that only receives interactions through HTTPHandler cannot be
// opened again: Open would connect the gateway.
func (m *Minidis) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()

	return m.Shutdown(ctx)
}

// Shutdown is Close, waiting for running handlers until ctx ends instead. It
// also stops an Open in progress.
func (m *Minidis) Shutdown(ctx context.Context) error {
	// Canceling under the lock means HTTPHandler cannot start a handler after
	// the wait below has begun.
	m.runMu.Lock()
	run := m.run
	run.cancel()
	m.runMu.Unlock()

	err := m.Session.Shutdown(ctx)

	drained := make(chan struct{})
	go func() {
		run.http.Wait()
		close(drained)
	}()

	select {
	case <-drained:
		return err
	case <-ctx.Done():
		return errors.Join(err, fmt.Errorf("minidis: HTTP interaction handlers still running: %w", ctx.Err()))
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
// interactions need none.
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
// its own and those also handled by OnChannelMessageCreate.
func (m *Minidis) OnMessageCreate(h func(*client.Session, *discord.MessageCreate)) {
	m.Session.OnMessageCreate(h)
}
