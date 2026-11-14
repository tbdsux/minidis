package minidis

import (
	"context"
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

	running atomic.Pointer[runState] // nil unless open
}

// runState is the context handlers derive from while the bot is open.
type runState struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a bot that authenticates with the given bot token.
func New(token string) *Minidis {
	m := &Minidis{
		Session:         client.New("Bot " + token),
		commands:        map[string]*Command{},
		messageCommands: map[string]*MessageCommand{},
		userCommands:    map[string]*UserCommand{},
		channelHandlers: map[string]func(*ChannelMessageContext) error{},
	}

	m.Session.OnInteractionCreate(m.handleInteraction)
	m.Session.OnMessageCreate(m.handleChannelMessage)

	return m
}

// Open connects to Discord and returns once the bot is ready. The connection is
// kept in the background until Close.
func (m *Minidis) Open() error {
	ctx, cancel := context.WithCancel(context.Background())

	if !m.running.CompareAndSwap(nil, &runState{ctx: ctx, cancel: cancel}) {
		cancel()
		return client.ErrAlreadyOpen
	}

	if err := m.Session.Open(); err != nil {
		m.running.Store(nil)
		cancel()
		return err
	}

	return nil
}

// Close disconnects from Discord, cancels the context of running handlers, and
// gives them up to 10 seconds to return. Use Shutdown to choose how long.
func (m *Minidis) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()

	return m.Shutdown(ctx)
}

// Shutdown is Close, waiting for running handlers until ctx ends instead.
func (m *Minidis) Shutdown(ctx context.Context) error {
	if r := m.running.Swap(nil); r != nil {
		r.cancel()
	}

	return m.Session.Shutdown(ctx)
}

// Run opens the bot, waits for ctx to end, and closes it. Pair it with
// signal.NotifyContext to stop on Ctrl+C.
func (m *Minidis) Run(ctx context.Context) error {
	if err := m.Open(); err != nil {
		return err
	}

	<-ctx.Done()

	return m.Close()
}

// baseContext is the parent of handler contexts.
func (m *Minidis) baseContext() context.Context {
	if r := m.running.Load(); r != nil {
		return r.ctx
	}
	return context.Background()
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
