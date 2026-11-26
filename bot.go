package minidis

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"unicode"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
	"github.com/tbdsux/minidis/interactions"
	"github.com/tbdsux/minidis/internal/wait"
)

// Bot is a Discord bot. Add commands and handlers to it, then call Start or
// Run.
//
// It embeds the bot of the interactions package, so its methods are
// available here: AddCommand, AddMessageCommand, AddUserCommand,
// HandleComponent, HandleModal, Use and HandleError for interactions,
// SetPresence, HTTPHandler, and Session for the Discord client.
//
// Add everything before the bot runs. Its methods are safe for concurrent
// use, and commands, components and modals can still be added while it runs,
// but the settings (SetIntents, AddIntents, SetGuilds, DisableSync and
// SetPrefix) cannot be changed then, and neither can a handler be added that
// needs a gateway intent the bot did not connect with: Discord would not send
// its events, which the bot logs.
//
// Set intents with SetIntents or AddIntents rather than on Session, though
// the bot connects with those of Session.Intents as well.
type Bot struct {
	*interactions.Minidis

	mu sync.RWMutex

	intents  discord.Intent // what the handlers need
	explicit bool           // SetIntents was called, so intents are not inferred

	// connected is whether the bot is running, and with which intents.
	connected  bool
	identified discord.Intent

	// sessionIntents is Session.Intents as it was before connect replaced
	// it, put back on disconnect so that it does not keep the bot's intents.
	sessionIntents discord.Intent

	guilds []string
	noSync bool

	prefix          string
	textCommands    map[string]*TextCommand // by name and by alias, in lower case
	textCommandList []*TextCommand
	listening       bool // the handler of text commands is registered

	onEventError   func(*EventContext, error)
	onMessageError func(*MessageContext, error)

	messageWaiters  wait.Set[*MessageContext]
	waitingMessages sync.Once // the handler that offers messages to them is registered
}

// New creates a bot that authenticates with the given bot token.
func New(token string) *Bot {
	b := &Bot{
		Minidis:      interactions.New(token),
		textCommands: map[string]*TextCommand{},
	}

	// The session starts out asking for every intent that is not privileged.
	// A Bot asks for what its handlers need instead; see SetIntents.
	b.Session.Intents = discord.IntentsNone
	return b
}

// need records intents that a handler depends on.
func (b *Bot) need(intents discord.Intent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.explicit {
		return
	}
	b.intents |= intents

	// Intents are sent once, when the bot connects.
	if missing := intents &^ b.identified; b.connected && missing != 0 {
		log.Printf("minidis: a handler was added while the bot is running that needs gateway intents "+
			"the bot did not connect with (%#b), so Discord will not send its events; add handlers before the bot runs", missing)
	}
}

// SetIntents sets the gateway intents of the bot itself. From then on the
// bot no longer works them out from its handlers, and only AddIntents adds
// to them. It is rarely needed.
func (b *Bot) SetIntents(intents discord.Intent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.intents, b.explicit = intents, true
}

// AddIntents asks for intents on top of those the bot's handlers need. Use it
// for handlers added to Session directly, which the bot does not see, and for
// discord.IntentsMessageContent when a handler added with On reads the
// content of messages.
func (b *Bot) AddIntents(intents discord.Intent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.intents |= intents
}

// Intents returns the gateway intents the bot connects with: those of its
// handlers and AddIntents, or those given to SetIntents, and in either case
// those set on Session.Intents and those that Session.State needs.
func (b *Bot) Intents() discord.Intent {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.intentsWith(b.Session.Intents)
}

// intentsWith is Intents with session in place of Session.Intents. b.mu must be held.
func (b *Bot) intentsWith(session discord.Intent) discord.Intent {
	intents := b.intents | session
	if st := b.Session.State; st != nil {
		intents |= st.Intents()
	}
	return intents
}

// connect sets the intents the session identifies with.
func (b *Bot) connect() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Still connected means the last connection stopped on its own and Open
	// is called again without Close: Session.Intents then holds its intents.
	if !b.connected {
		b.sessionIntents = b.Session.Intents
	}
	intents := b.intentsWith(b.sessionIntents)
	b.Session.Intents = intents
	b.connected, b.identified = true, intents
}

func (b *Bot) disconnected() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.connected {
		b.Session.Intents = b.sessionIntents
	}
	b.connected = false
}

// SetGuilds makes the bot register its commands in the given guilds rather
// than globally. Use it while developing: commands of a guild show up at
// once, and global ones can take a while.
//
// Each argument may hold several IDs, separated by commas or spaces, and
// empty ones are ignored, so that SetGuilds(os.Getenv("GUILD")) takes one
// guild or a list, and registers globally when the variable is unset.
func (b *Bot) SetGuilds(guildIDs ...string) {
	var guilds []string
	for _, ids := range guildIDs {
		guilds = append(guilds, strings.FieldsFunc(ids, func(r rune) bool {
			return r == ',' || unicode.IsSpace(r)
		})...)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.guilds = guilds
}

// DisableSync stops Run from registering the commands with Discord, for a
// bot whose commands are registered elsewhere, such as by a deploy step that
// calls SyncCommands.
func (b *Bot) DisableSync() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.noSync = true
}

// HandleChannelMessage sets the handler for messages in a channel; see the
// interactions package. OnMessage handles the messages of every channel.
func (b *Bot) HandleChannelMessage(channelID string, h func(c *ChannelMessageContext) error) {
	b.need(messageIntents | discord.IntentsMessageContent)
	b.Minidis.HandleChannelMessage(channelID, h)
}

// OnMessageCreate adds a handler for every message the bot can see, its own
// included, without the context and the error handling of OnMessage.
func (b *Bot) OnMessageCreate(h func(*client.Session, *discord.MessageCreate)) {
	b.need(messageIntents)
	b.Minidis.OnMessageCreate(h)
}

// Open connects to the Discord gateway and returns once the bot is ready,
// without registering commands. Most bots call Run or Start instead.
func (b *Bot) Open(ctx context.Context) error {
	b.connect()

	err := b.Minidis.Open(ctx)
	if err != nil && !errors.Is(err, client.ErrAlreadyOpen) {
		b.disconnected()
	}
	return b.explain(err)
}

// Close stops the bot and gives running handlers up to 10 seconds to finish;
// see the interactions package.
func (b *Bot) Close() error {
	defer b.disconnected()
	return b.Minidis.Close()
}

// Shutdown stops the bot, waiting for running handlers until ctx ends; see
// the interactions package.
func (b *Bot) Shutdown(ctx context.Context) error {
	defer b.disconnected()
	return b.Minidis.Shutdown(ctx)
}

// Run registers the bot's commands with Discord, connects, and keeps the bot
// running until ctx ends, then closes it, giving running handlers up to 10
// seconds to finish. It also returns, with the reason, if Discord ends the
// connection for good, such as for an invalid token.
//
// The commands are registered globally, or in the guilds given to SetGuilds,
// replacing the ones Discord has. A bot without commands leaves those alone;
// use ClearCommands to remove them.
func (b *Bot) Run(ctx context.Context) error {
	b.mu.RLock()
	register, guilds := !b.noSync, b.guilds
	b.mu.RUnlock()

	if register && b.HasCommands() {
		if err := b.SyncCommands(ctx, guilds...); err != nil {
			return fmt.Errorf("minidis: registering the commands: %w", err)
		}
	}

	b.connect()
	defer b.disconnected()

	return b.explain(b.Minidis.Run(ctx))
}

// Start runs the bot until the program is interrupted with Ctrl+C or asked
// to terminate; see Run.
//
//	if err := bot.Start(); err != nil {
//		log.Fatal(err)
//	}
//
// It returns nil when the bot was stopped that way and closed cleanly. While
// the bot is closing, a second Ctrl+C ends the program at once.
func (b *Bot) Start() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Once the first signal has arrived, the next one is no longer caught.
	context.AfterFunc(ctx, stop)

	return b.Run(ctx)
}

// disallowedIntents is the code Discord closes the gateway with when a bot
// asks for a privileged intent that is not enabled for it.
const disallowedIntents = 4014

// privilegedIntents names the privileged intents as the developer portal does.
var privilegedIntents = []struct {
	intent discord.Intent
	name   string
}{
	{discord.IntentsGuildPresences, "Presence Intent"},
	{discord.IntentsGuildMembers, "Server Members Intent"},
	{discord.IntentsMessageContent, "Message Content Intent"},
}

// explain says which switches to flip when Discord refuses the bot's
// intents, which its own error does not.
func (b *Bot) explain(err error) error {
	var closed *client.GatewayCloseError
	if !errors.As(err, &closed) || closed.Code != disallowedIntents {
		return err
	}

	var names []string
	intents := b.Intents()
	for _, p := range privilegedIntents {
		if intents&p.intent != 0 {
			names = append(names, p.name)
		}
	}
	if len(names) == 0 {
		return err
	}

	return fmt.Errorf("minidis: the bot's handlers need the %s, which must be enabled for the bot "+
		"in the Discord developer portal, under Bot > Privileged Gateway Intents: %w", strings.Join(names, " and the "), err)
}
