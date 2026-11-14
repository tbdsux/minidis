package minidis

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

// ErrNoResponse is the cause of a handler context canceled because the
// interaction was not responded to within 3 seconds, after which Discord no
// longer accepts a response. Check for it with context.Cause.
var ErrNoResponse = errors.New("minidis: the interaction was not responded to within 3 seconds")

// Context is passed to every interaction handler: to slash commands directly,
// and to the others inside a type that embeds it, such as ComponentContext. It
// describes the interaction and the user who caused it, and responds to it.
//
// Context is also a context.Context, which the response methods use for their
// requests. It is canceled when the handler returns, when the bot closes, 15
// minutes after the interaction arrived (when Discord stops accepting
// responses), and after 3 seconds if nothing has responded by then, with
// ErrNoResponse as its cause. Pass it to the calls a handler makes, and use
// WithContext in middleware to add values to it.
type Context struct {
	ctx   context.Context
	reply *replyState // shared by the copies WithContext makes

	Interaction *discord.Interaction
	Session     *client.Session

	AppID  string
	Author *discord.User   // the user who caused the interaction
	Member *discord.Member // Author as a guild member, or nil in a DM
	IsDM   bool

	// Bot is the bot's own user. A bot that receives interactions through
	// HTTPHandler without opening the gateway has it once HTTPHandler looked
	// it up.
	Bot *discord.User

	// ID is the ID of the interaction. A component's custom ID is in
	// ComponentContext.Data.CustomID.
	ID          string
	GuildID     string
	ChannelID   string
	Locale      discord.Locale  // the language of the user
	GuildLocale *discord.Locale // the language of the guild, or nil in a DM

	// Options are the values given to the slash command or subcommand that was
	// run. The getters, such as String and User, are easier to use.
	Options map[string]*discord.ApplicationCommandInteractionDataOption
}

// replyState tracks the initial response of an interaction.
type replyState struct {
	responded atomic.Bool
	ack       func() // called after the initial response; see runInteraction
}

func newContext(ctx context.Context, s *client.Session, i *discord.Interaction, bot *discord.User) *Context {
	c := &Context{
		ctx:         ctx,
		reply:       &replyState{},
		Interaction: i,
		Session:     s,

		AppID:  i.AppID,
		Member: i.Member,
		Author: i.User,
		IsDM:   i.GuildID == "",
		Bot:    bot,

		ID:          i.ID,
		GuildID:     i.GuildID,
		ChannelID:   i.ChannelID,
		Locale:      i.Locale,
		GuildLocale: i.GuildLocale,

		Options: map[string]*discord.ApplicationCommandInteractionDataOption{},
	}

	if i.Member != nil {
		c.Author = i.Member.User
	}

	return c
}

// setOptions indexes options by name.
func (c *Context) setOptions(options []*discord.ApplicationCommandInteractionDataOption) {
	c.Options = make(map[string]*discord.ApplicationCommandInteractionDataOption, len(options))
	for _, v := range options {
		c.Options[v.Name] = v
	}
}

// orBackground guards against a zero handler context, which has no ctx.
func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// Deadline implements context.Context.
func (c *Context) Deadline() (time.Time, bool) { return orBackground(c.ctx).Deadline() }

// Done implements context.Context.
func (c *Context) Done() <-chan struct{} { return orBackground(c.ctx).Done() }

// Err implements context.Context.
func (c *Context) Err() error { return orBackground(c.ctx).Err() }

// Value implements context.Context.
func (c *Context) Value(key any) any { return orBackground(c.ctx).Value(key) }

// WithContext returns a copy of c that uses ctx as its context.Context, which
// should be derived from c. Middleware uses it to add values:
//
//	return next(c.WithContext(context.WithValue(c, key, value)))
//
// The copy shares the interaction's response state with c.
func (c *Context) WithContext(ctx context.Context) *Context {
	if ctx == nil {
		panic("minidis: nil context")
	}
	copied := *c
	copied.ctx = ctx
	return &copied
}

// Responded reports whether the interaction has been responded to, with a
// reply, a deferral, a modal, an update or autocomplete choices. After that,
// a message goes through Followup or Edit instead of Reply. An error handler
// uses it to tell the user that something failed.
func (c *Context) Responded() bool {
	return c.reply != nil && c.reply.responded.Load()
}

// Message is a new message: a reply or a followup.
type Message struct {
	Content         string
	Embeds          []*discord.MessageEmbed
	Components      []discord.MessageComponent
	Files           []*discord.File
	AllowedMentions *discord.MessageAllowedMentions
	Poll            *discord.PollCreateRequest
	TTS             bool

	// Ephemeral messages are only shown to the user who caused the interaction.
	// It has no effect on channel messages.
	Ephemeral bool

	// Flags are further message flags: discord.MessageFlagsIsComponentsV2 to
	// lay the message out with Components only (Content, Embeds and Poll must
	// then be empty), discord.MessageFlagsSuppressEmbeds, or
	// discord.MessageFlagsSuppressNotifications.
	Flags discord.MessageFlags
}

func (msg Message) flags() discord.MessageFlags {
	if msg.Ephemeral {
		return msg.Flags | discord.MessageFlagsEphemeral
	}
	return msg.Flags
}

func (msg Message) responseData() *discord.InteractionResponseData {
	data := &discord.InteractionResponseData{
		Embeds:          msg.Embeds,
		Components:      msg.Components,
		Files:           msg.Files,
		AllowedMentions: msg.AllowedMentions,
		Poll:            msg.Poll,
		TTS:             msg.TTS,
		Flags:           msg.flags(),
	}
	if msg.Content != "" {
		data.Content = &msg.Content
	}
	return data
}

func (msg Message) webhookParams() *discord.WebhookParams {
	return &discord.WebhookParams{
		Content:         msg.Content,
		Embeds:          msg.Embeds,
		Components:      msg.Components,
		Files:           msg.Files,
		AllowedMentions: msg.AllowedMentions,
		Poll:            msg.Poll,
		TTS:             msg.TTS,
		Flags:           msg.flags(),
	}
}

func (msg Message) channelMessage() *discord.MessageSend {
	return &discord.MessageSend{
		Content:         msg.Content,
		Embeds:          msg.Embeds,
		Components:      msg.Components,
		Files:           msg.Files,
		AllowedMentions: msg.AllowedMentions,
		Poll:            msg.Poll,
		TTS:             msg.TTS,
		Flags:           msg.Flags,
	}
}

// MessageEdit changes a message. A nil field is left unchanged; an empty
// value clears it, such as an empty non-nil Components to remove the buttons.
type MessageEdit struct {
	Content         *string // use discord.String to set it
	Embeds          []*discord.MessageEmbed
	Components      []discord.MessageComponent
	AllowedMentions *discord.MessageAllowedMentions

	// Files are added to the message. Attachments, when not nil, are the
	// existing files to keep; an empty non-nil slice removes them all.
	Files       []*discord.File
	Attachments []*discord.MessageAttachment

	// Poll can only be added when editing a deferred response.
	Poll *discord.PollCreateRequest

	// Flags adds discord.MessageFlagsSuppressEmbeds or
	// discord.MessageFlagsIsComponentsV2; zero leaves the flags unchanged.
	// Turning Components V2 on cannot be undone, and is how a deferred reply
	// becomes a Components V2 message: the text and poll are then cleared.
	Flags discord.MessageFlags
}

func (e MessageEdit) webhookEdit() *discord.WebhookEdit {
	res := &discord.WebhookEdit{
		Content:         e.Content,
		Files:           e.Files,
		AllowedMentions: e.AllowedMentions,
		Poll:            e.Poll,
	}
	if e.Embeds != nil {
		res.Embeds = &e.Embeds
	}
	if e.Components != nil {
		res.Components = &e.Components
	}
	if e.Attachments != nil {
		res.Attachments = &e.Attachments
	}
	if e.Flags != 0 {
		res.Flags = &e.Flags
	}
	return res
}

func (e MessageEdit) responseData() *discord.InteractionResponseData {
	data := &discord.InteractionResponseData{
		Content:         e.Content,
		Embeds:          e.Embeds,
		Components:      e.Components,
		Files:           e.Files,
		AllowedMentions: e.AllowedMentions,
		Poll:            e.Poll,
		Flags:           e.Flags,
	}
	if e.Attachments != nil {
		data.Attachments = &e.Attachments
	}
	return data
}

// Modal is a form shown to the user. Its submission goes to the handler set
// with HandleModal for its CustomID.
type Modal struct {
	CustomID   string
	Title      string
	Components []discord.MessageComponent
}

// respond sends the initial response to the interaction.
func (c *Context) respond(res *discord.InteractionResponse) error {
	// Marked first, so that the 3-second cutoff cannot cancel a response
	// that is being sent.
	if c.reply != nil {
		c.reply.responded.Store(true)
	}

	err := c.Session.InteractionRespond(c, c.Interaction, res)

	if c.reply != nil && c.reply.ack != nil {
		c.reply.ack()
	}
	return err
}

// Reply responds to the interaction with a message.
func (c *Context) Reply(content string) error {
	return c.ReplyWith(Message{Content: content})
}

// ReplyWith responds to the interaction with a message.
func (c *Context) ReplyWith(msg Message) error {
	return c.respond(&discord.InteractionResponse{
		Type: discord.InteractionResponseChannelMessageWithSource,
		Data: msg.responseData(),
	})
}

// DeferReply responds to the interaction with a loading state, to be replaced
// within 15 minutes using Edit. It counts as the response, so Reply cannot be
// used afterwards. If ephemeral, only the user sees the loading state and the
// message that replaces it.
func (c *Context) DeferReply(ephemeral bool) error {
	return c.respond(&discord.InteractionResponse{
		Type: discord.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discord.InteractionResponseData{Flags: Message{Ephemeral: ephemeral}.flags()},
	})
}

// ReplyModal responds to the interaction by showing a modal.
func (c *Context) ReplyModal(modal Modal) error {
	return c.respond(&discord.InteractionResponse{
		Type: discord.InteractionResponseModal,
		Data: &discord.InteractionResponseData{
			Title:      modal.Title,
			CustomID:   modal.CustomID,
			Components: modal.Components,
		},
	})
}

// LaunchActivity responds to the interaction by launching the app's Activity.
func (c *Context) LaunchActivity() error {
	return c.respond(&discord.InteractionResponse{Type: discord.InteractionResponseLaunchActivity})
}

// update responds by changing the message that has the component the
// interaction came from.
func (c *Context) update(edit MessageEdit) error {
	return c.respond(&discord.InteractionResponse{
		Type: discord.InteractionResponseUpdateMessage,
		Data: edit.responseData(),
	})
}

// deferUpdate acknowledges the interaction without changing the message yet.
func (c *Context) deferUpdate() error {
	return c.respond(&discord.InteractionResponse{
		Type: discord.InteractionResponseDeferredMessageUpdate,
	})
}

// Response fetches the message sent as the response, such as to learn its ID.
func (c *Context) Response() (*discord.Message, error) {
	return c.Session.InteractionResponse(c, c.Interaction)
}

// Edit replaces the text of the response.
func (c *Context) Edit(content string) error {
	return c.EditWith(MessageEdit{Content: &content})
}

// EditWith changes the response.
func (c *Context) EditWith(edit MessageEdit) error {
	_, err := c.Session.InteractionResponseEdit(c, c.Interaction, edit.webhookEdit())
	return err
}

// Delete deletes the response.
func (c *Context) Delete() error {
	return c.Session.InteractionResponseDelete(c, c.Interaction)
}

// Followup sends another message after the response.
func (c *Context) Followup(content string) (*FollowupMessage, error) {
	return c.FollowupWith(Message{Content: content})
}

// FollowupWith sends another message after the response.
func (c *Context) FollowupWith(msg Message) (*FollowupMessage, error) {
	sent, err := c.Session.FollowupMessageCreate(c, c.Interaction, msg.webhookParams())
	if err != nil {
		return nil, err
	}

	return &FollowupMessage{Message: sent, ctx: c, session: c.Session, interaction: c.Interaction}, nil
}
