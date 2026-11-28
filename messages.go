package minidis

import (
	"context"
	"runtime/debug"
	"slices"
	"strings"
	"unicode"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
	"github.com/tbdsux/minidis/locale"
)

// messageIntents deliver the messages of guilds and of DMs.
const messageIntents = discord.IntentsGuildMessages | discord.IntentsDirectMessages

const messageCreate = "MESSAGE_CREATE"

// MessageContext is passed to message handlers and text commands. It embeds
// the EventContext of the message's event, and so is a context.Context too.
//
// Its methods that send, such as Reply, keep working after the handler
// returns, until the bot closes.
type MessageContext struct {
	*EventContext

	Message *discord.Message
	Content string // the text of the message

	Author    *discord.User
	Member    *discord.Member // Author as a guild member, with its User set, or nil in a DM
	GuildID   string
	ChannelID string
	IsDM      bool

	// The fields below are set for text commands. For "!echo  hello there",
	// Command is "echo", Args is ["hello", "there"] and ArgString is
	// "hello there".
	Command   string   // the name the command was called by, in lower case
	Args      []string // the words after it
	ArgString string   // the text after it, as written
}

func newMessageContext(c *EventContext, m *discord.Message) *MessageContext {
	mc := &MessageContext{
		EventContext: c,
		Message:      m,
		Content:      m.Content,
		Author:       m.Author,
		GuildID:      m.GuildID,
		ChannelID:    m.ChannelID,
		IsDM:         m.GuildID == "",
	}

	// The member of a message comes without its user.
	if m.Member != nil {
		member := *m.Member
		if member.User == nil {
			member.User = m.Author
		}
		mc.Member = &member
	}
	return mc
}

// fromPerson reports whether a message was written by a user, rather than by
// a bot, this one included, a webhook or Discord itself. Discord's own
// messages, such as "ann joined the server" and "ann pinned a message", name
// the user they are about as their author, and are told apart by their type.
func fromPerson(m *discord.Message) bool {
	if m.Author == nil || m.Author.Bot || m.Author.System || m.WebhookID != "" {
		return false
	}
	return m.Type == discord.MessageTypeDefault || m.Type == discord.MessageTypeReply
}

// OnMessage adds a handler for the messages people send, in every channel the
// bot can see and in its DMs. Messages from bots, the bot itself included,
// and from webhooks are left out, so that bots do not answer each other
// forever, and so are Discord's own, such as the notice that someone joined;
// On with a discord.MessageCreate sees those too.
//
// It asks for the privileged discord.IntentsMessageContent, which must also
// be enabled for the bot in the developer portal. Without it, the content of
// messages is empty unless they mention the bot or are sent to it in a DM.
//
// An error or a panic goes to the handler set with HandleMessageError.
func (b *Bot) OnMessage(h func(c *MessageContext) error) {
	client.Handle(b.Session, func(s *client.Session, e *discord.MessageCreate) {
		if !fromPerson(e.Message) {
			return
		}

		c, done := b.newEvent(s, messageCreate)
		defer done()

		b.runMessage(newMessageContext(c, e.Message), h)
	})

	b.need(messageIntents | discord.IntentsMessageContent)
}

// WaitForMessage waits for the next message that filter accepts, or any if
// filter is nil, among those people send, as OnMessage sees them, and returns
// it. A handler uses it to ask for an answer:
//
//	ctx, cancel := context.WithTimeout(c, time.Minute)
//	defer cancel()
//
//	answer, err := c.Bot.WaitForMessage(ctx, func(m *minidis.MessageContext) bool {
//		return m.ChannelID == c.ChannelID && m.Author.ID == c.Author.ID
//	})
//
// Handlers of OnMessage and text commands see the message as well. The first
// waiter to begin gets it if several accept it, and filter may run for
// several messages at once.
//
// Like OnMessage, it needs gateway intents, among them the privileged
// discord.IntentsMessageContent. As it is called while the bot runs, a bot
// that has no handler of messages asks for them with AddIntents before it
// runs; otherwise Discord does not send the messages, which the bot logs.
//
// It returns ctx's error if ctx ends first, and context.Canceled once the bot
// begins to close, so that the caller can still clean up before Shutdown
// ends. If filter panics, the panic goes to the handler of HandleEventError,
// and WaitForMessage returns an error. The MessageContext it returns is
// canceled when ctx ends.
func (b *Bot) WaitForMessage(ctx context.Context, filter func(c *MessageContext) bool) (*MessageContext, error) {
	b.waitingMessages.Do(func() {
		b.need(messageIntents | discord.IntentsMessageContent)
		client.Handle(b.Session, b.offerMessage)
	})

	return b.messageWaiters.Wait(ctx, b.Stopping(), filter)
}

func (b *Bot) offerMessage(s *client.Session, e *discord.MessageCreate) {
	if !fromPerson(e.Message) || b.messageWaiters.Len() == 0 {
		return
	}

	c, done := b.newEvent(s, messageCreate)
	var waiting context.Context
	b.runEvent(c, func() error {
		waiting, _ = b.messageWaiters.Offer(newMessageContext(c, e.Message))
		return nil
	})
	if waiting != nil {
		context.AfterFunc(waiting, done)
		return
	}
	done()
}

// runMessage runs a message handler and reports its error or panic.
func (b *Bot) runMessage(c *MessageContext, h func(*MessageContext) error) {
	defer func() {
		if r := recover(); r != nil {
			b.messageError(c, &PanicError{Value: r, Stack: debug.Stack()})
		}
	}()

	if err := h(c); err != nil {
		b.messageError(c, err)
	}
}

// HandleMessageError sets the function that receives the errors returned by
// message handlers and text commands, replacing the default of passing them
// to the handler of HandleEventError, which logs them. A panic arrives as a
// *PanicError. Use it to tell the user that something failed, with c.Reply.
// A panic in it is logged.
func (b *Bot) HandleMessageError(h func(c *MessageContext, err error)) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.onMessageError = h
}

func (b *Bot) messageError(c *MessageContext, err error) {
	b.mu.RLock()
	h := b.onMessageError
	b.mu.RUnlock()

	if h == nil {
		b.eventError(c.EventContext, err)
		return
	}

	defer func() {
		if r := recover(); r != nil {
			b.logError("error handler of a message", &PanicError{Value: r, Stack: debug.Stack()})
		}
	}()
	h(c, err)
}

// reference points at the message being handled. It does not fail if the
// message was deleted in the meantime.
func (c *MessageContext) reference() *discord.MessageReference {
	return &discord.MessageReference{
		MessageID:       c.Message.ID,
		ChannelID:       c.ChannelID,
		GuildID:         c.GuildID,
		FailIfNotExists: discord.Bool(false),
	}
}

// T returns the message for key from the bot's catalog (see SetCatalog) in
// the guild's language, when Session.State has the guild, else in the
// catalog's default, else key itself, formatted with args like fmt.Sprintf.
// Messages do not say the language of their author.
func (c *MessageContext) T(key string, args ...any) string {
	var l discord.Locale
	if s := c.Session; s != nil && s.State != nil && c.GuildID != "" {
		if g := s.State.Guild(c.GuildID); g != nil {
			l = g.PreferredLocale
		}
	}
	var cat *locale.Catalog
	if c.Bot != nil {
		cat = c.Bot.Catalog()
	}
	return cat.T(l, key, args...)
}

// Send sends a text message to the channel of the message.
func (c *MessageContext) Send(content string) (*discord.Message, error) {
	return c.SendWith(Message{Content: content})
}

// SendWith sends a message to the channel of the message. Use
// AllowedMentions when the content repeats what users wrote, so that it
// cannot ping @everyone. Ephemeral does not apply to these messages.
func (c *MessageContext) SendWith(msg Message) (*discord.Message, error) {
	return c.Session.ChannelMessageSendComplex(c.background(), c.ChannelID, msg.MessageSend())
}

// Reply sends a text message as a reply to the message.
func (c *MessageContext) Reply(content string) (*discord.Message, error) {
	return c.ReplyWith(Message{Content: content})
}

// ReplyWith sends a message as a reply to the message; see SendWith.
func (c *MessageContext) ReplyWith(msg Message) (*discord.Message, error) {
	send := msg.MessageSend()
	send.Reference = c.reference()
	return c.Session.ChannelMessageSendComplex(c.background(), c.ChannelID, send)
}

// React adds the bot's reaction to the message. emoji is a Unicode emoji, or
// "name:id" for a custom one, which Emoji.APIName returns.
func (c *MessageContext) React(emoji string) error {
	return c.Session.MessageReactionAdd(c.background(), c.ChannelID, c.Message.ID, emoji)
}

// Delete deletes the message, which needs the Manage Messages permission.
func (c *MessageContext) Delete() error {
	return c.Session.ChannelMessageDelete(c.background(), c.ChannelID, c.Message.ID)
}

// Typing shows the bot as typing in the channel for 10 seconds, or until it
// sends a message. Use it before work that takes a while.
func (c *MessageContext) Typing() error {
	return c.Session.ChannelTyping(c.background(), c.ChannelID)
}

// TextCommand is a command people run by writing a message, such as "!ping".
// Slash commands are the better choice for most bots: Discord shows them to
// users, checks their options, and they need no privileged intent.
type TextCommand struct {
	Name        string
	Aliases     []string // other names it answers to
	Description string   // for a help command of your own; see Bot.TextCommands
	Execute     func(c *MessageContext) error
}

// SetPrefix sets what a message starts with to run a text command, such as
// "!" for "!ping". Without one, text commands are run by mentioning the bot
// first, as in "@bot ping", which always works as well. It replaces a
// function set with SetPrefixFunc.
//
// A prefix asks for the privileged discord.IntentsMessageContent, which must
// also be enabled for the bot in the developer portal; messages that mention
// the bot do not need it.
func (b *Bot) SetPrefix(prefix string) {
	if prefix == "" {
		b.SetPrefixFunc(nil)
		return
	}
	b.SetPrefixFunc(func(*discord.Message) string { return prefix })
}

// SetPrefixFunc sets a function that returns the prefix of text commands for
// a message, such as one stored for its guild:
//
//	bot.SetPrefixFunc(func(m *discord.Message) string {
//		if p, ok := prefixes.Load(m.GuildID); ok { // GuildID is empty in a DM
//			return p.(string)
//		}
//		return "!"
//	})
//
// It runs for every message people send, so it should be quick, such as a
// lookup in memory. An empty prefix means the message runs a command only by
// mentioning the bot, which always works as well. It replaces a prefix set
// with SetPrefix, and nil removes it.
//
// Like SetPrefix, it asks for the privileged discord.IntentsMessageContent.
func (b *Bot) SetPrefixFunc(f func(m *discord.Message) string) {
	b.mu.Lock()
	b.prefix = f
	b.mu.Unlock()

	if f != nil {
		b.need(discord.IntentsMessageContent)
	}
}

// AddTextCommand adds text commands. Names and aliases are matched without
// regard to case, and one added later wins: it replaces a command of the
// same name, and one whose name is among its aliases, and takes over an alias
// that another command had. They are run for messages from people only, like
// the handlers of OnMessage, which see those messages too.
func (b *Bot) AddTextCommand(cmds ...*TextCommand) {
	b.mu.Lock()
	for _, cmd := range cmds {
		names := append([]string{cmd.Name}, cmd.Aliases...)

		// A command that loses its own name could no longer be run, so it
		// goes altogether, with what else it answered to.
		for _, name := range names {
			if old := b.textCommands[strings.ToLower(name)]; old != nil && old != cmd && strings.EqualFold(old.Name, name) {
				b.forget(old)
			}
		}

		b.forget(cmd) // added before, perhaps with other aliases
		b.textCommandList = append(b.textCommandList, cmd)
		for _, name := range names {
			b.textCommands[strings.ToLower(name)] = cmd
		}
	}

	listen := !b.listening
	b.listening = true
	b.mu.Unlock()

	if listen {
		client.Handle(b.Session, b.handleTextCommand)
		b.need(messageIntents)
	}
}

// forget removes a text command. b.mu must be held.
func (b *Bot) forget(cmd *TextCommand) {
	for name, c := range b.textCommands {
		if c == cmd {
			delete(b.textCommands, name)
		}
	}
	b.textCommandList = slices.DeleteFunc(b.textCommandList, func(c *TextCommand) bool { return c == cmd })
}

// TextCommands returns the text commands, sorted by name, such as to list
// them in a help command.
func (b *Bot) TextCommands() []*TextCommand {
	b.mu.RLock()
	cmds := slices.Clone(b.textCommandList)
	b.mu.RUnlock()

	slices.SortFunc(cmds, func(x, y *TextCommand) int { return strings.Compare(x.Name, y.Name) })
	return cmds
}

func (b *Bot) handleTextCommand(s *client.Session, e *discord.MessageCreate) {
	if !fromPerson(e.Message) {
		return
	}

	b.mu.RLock()
	prefixOf := b.prefix
	b.mu.RUnlock()

	prefix := ""
	if prefixOf != nil {
		prefix = prefixOf(e.Message)
	}

	name, rest, ok := splitCommand(e.Content, prefix, s.User())
	if !ok {
		return
	}

	b.mu.RLock()
	cmd := b.textCommands[name]
	b.mu.RUnlock()

	if cmd == nil || cmd.Execute == nil {
		return
	}

	c, done := b.newEvent(s, messageCreate)
	defer done()

	mc := newMessageContext(c, e.Message)
	mc.Command, mc.Args, mc.ArgString = name, strings.Fields(rest), rest

	b.runMessage(mc, cmd.Execute)
}

// splitCommand takes the name of a text command, in lower case, and the text
// after it from a message that starts with the prefix or a mention of the bot.
func splitCommand(content, prefix string, bot *discord.User) (name, rest string, ok bool) {
	body, found := "", false
	if prefix != "" {
		body, found = strings.CutPrefix(content, prefix)
	}
	if !found && bot != nil {
		// Discord wrote mentions of users with nicknames as <@!id>.
		if body, found = strings.CutPrefix(content, "<@"+bot.ID+">"); !found {
			body, found = strings.CutPrefix(content, "<@!"+bot.ID+">")
		}
	}
	if !found {
		return "", "", false
	}

	body = strings.TrimSpace(body)
	if i := strings.IndexFunc(body, unicode.IsSpace); i >= 0 {
		name, rest = body[:i], strings.TrimSpace(body[i:])
	} else {
		name = body
	}

	return strings.ToLower(name), rest, name != ""
}
