package minidis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
	"github.com/tbdsux/minidis/interactions"
)

// Bot is a Discord bot. Add commands and handlers to it, then call Start or
// Run.
//
// It embeds the bot of the interactions package, so its methods are
// available here: AddCommand, AddMessageCommand, AddUserCommand,
// HandleComponent, HandleModal, Use and HandleError for interactions,
// SetPresence, HTTPHandler, and Session for the Discord client.
//
// Its methods are safe for concurrent use, except that the settings (SetIntents,
// AddIntents, SetGuilds, DisableSync and SetPrefix) must be made before the
// bot runs.
type Bot struct {
	*interactions.Minidis

	mu sync.RWMutex

	intents  discord.Intent // what the handlers need
	explicit bool           // SetIntents was called, so intents are not inferred

	guilds []string
	noSync bool

	prefix          string
	textCommands    map[string]*TextCommand // by name and by alias, in lower case
	textCommandList []*TextCommand
	listening       bool // the handler of text commands is registered

	onEventError   func(*EventContext, error)
	onMessageError func(*MessageContext, error)
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

	if !b.explicit {
		b.intents |= intents
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
// handlers and AddIntents, or those given to SetIntents.
func (b *Bot) Intents() discord.Intent {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.intents
}

// SetGuilds makes the bot register its commands in the given guilds rather
// than globally. Use it while developing: commands of a guild show up at
// once, and global ones can take a while. Empty IDs are ignored, so that
// SetGuilds(os.Getenv("GUILD")) registers globally when the variable is unset.
func (b *Bot) SetGuilds(guildIDs ...string) {
	var guilds []string
	for _, id := range guildIDs {
		if id = strings.TrimSpace(id); id != "" {
			guilds = append(guilds, id)
		}
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
	b.Session.Intents = b.Intents()
	return b.explain(b.Minidis.Open(ctx))
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

	b.Session.Intents = b.Intents()
	return b.explain(b.Minidis.Run(ctx))
}

// Start runs the bot until the program is interrupted with Ctrl+C or asked
// to terminate; see Run.
//
//	log.Fatal(bot.Start())
//
// It returns nil when the bot was stopped that way and closed cleanly.
func (b *Bot) Start() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
