package minidis

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
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
// Context is also a context.Context for the handler's own work. It is
// canceled when the handler returns, 15 minutes after the interaction arrived
// (when Discord stops accepting responses), after 3 seconds if nothing has
// responded by then (with ErrNoResponse as its cause), and when a closing bot
// stops waiting for its handlers. Pass it to the calls a handler makes, and use
// WithContext in middleware to add values to it.
//
// The response methods, such as Reply, Edit and Followup, keep working after
// the handler returns, until Discord's 15 minutes are up or the bot closes.
// Respond through them rather than through Session directly: a response sent
// another way is not seen, so the context is canceled after 3 seconds.
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
	state atomic.Int32 // pending, responded or tooLate
	ack   func()       // called after the initial response; see runInteraction

	// api is the context of the requests the response methods make. It
	// outlives the handler, so that a response can be edited afterwards.
	api context.Context

	// latest is the copy last made with WithContext, which the error handler
	// receives so that it sees the values middleware added.
	latest atomic.Pointer[Context]
}

const (
	pending int32 = iota
	responded
	tooLate // no response within the 3 seconds
)

func newContext(ctx, api context.Context, s *client.Session, i *discord.Interaction, bot *discord.User) *Context {
	c := &Context{
		ctx:         ctx,
		reply:       &replyState{api: api},
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
// The copy shares the interaction's response state with c, and is what the
// error handler receives if the handler fails.
func (c *Context) WithContext(ctx context.Context) *Context {
	if ctx == nil {
		panic("minidis: nil context")
	}
	copied := *c
	copied.ctx = ctx
	c.reply.latest.Store(&copied)
	return &copied
}

// forErrors is the Context the error handler receives.
func (c *Context) forErrors() *Context {
	if latest := c.reply.latest.Load(); latest != nil {
		return latest
	}
	return c
}

// Responded reports whether the interaction has been responded to, with a
// reply, a deferral, a modal, an update or autocomplete choices. A response
// that Discord refused does not count. After a response, a message goes
// through Followup or Edit instead of Reply; an error handler uses it to pick.
func (c *Context) Responded() bool {
	return c.reply != nil && c.reply.state.Load() == responded
}

// errTooLate is returned by the response methods once the 3 seconds passed
// without a response.
var errTooLate = fmt.Errorf("minidis: cannot respond: %w", ErrNoResponse)

// api returns the context for a request about the interaction.
func (c *Context) api() (context.Context, error) {
	if c.reply.state.Load() == tooLate {
		return nil, errTooLate
	}
	return c.reply.api, nil
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
//
// Mind slices built with append: one that ends up with no elements is still
// nil, which leaves the field unchanged. Start from an empty slice, such as
// []discord.MessageComponent{}, when the result may be empty and should clear.
type MessageEdit struct {
	Content         *string // use discord.String to set it
	Embeds          []*discord.MessageEmbed
	Components      []discord.MessageComponent
	AllowedMentions *discord.MessageAllowedMentions

	// Files are added to the message. Attachments, when not nil, are the
	// existing files to keep, and an empty non-nil slice removes them all;
	// the new Files are kept too.
	Files       []*discord.File
	Attachments []*discord.MessageAttachment

	// Poll can only be added when editing a deferred response.
	Poll *discord.PollCreateRequest

	// Flags replaces the message's flags, so include those to keep; zero
	// leaves them unchanged. Only discord.MessageFlagsSuppressEmbeds and
	// discord.MessageFlagsIsComponentsV2 can be set. Turning Components V2 on
	// cannot be undone, and is how a deferred reply becomes a Components V2
	// message: its text, embeds and poll are then cleared.
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
		kept := e.attachments()
		res.Attachments = &kept
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
		kept := e.attachments()
		data.Attachments = &kept
	}
	return data
}

// attachments lists the files the message keeps: Discord drops any not
// listed, including new ones, which are referred to by their index in Files.
func (e MessageEdit) attachments() []*discord.MessageAttachment {
	kept := slices.Clone(e.Attachments)
	for i, f := range e.Files {
		kept = append(kept, &discord.MessageAttachment{ID: strconv.Itoa(i), Filename: f.Name})
	}
	return kept
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
	return c.sendResponse(func(ctx context.Context) error {
		return c.Session.InteractionRespond(ctx, c.Interaction, res)
	})
}

// SendResponse sends an initial response built by hand, such as a type this
// package has no method for, and returns what Discord reports about it, such
// as the message it created or the Activity it launched. Use it rather than
// Session, so that the response counts for Responded and the 3-second cutoff.
func (c *Context) SendResponse(res *discord.InteractionResponse) (*discord.InteractionCallbackResponse, error) {
	var out *discord.InteractionCallbackResponse
	err := c.sendResponse(func(ctx context.Context) error {
		var err error
		out, err = c.Session.InteractionRespondWithResponse(ctx, c.Interaction, res)
		return err
	})
	return out, err
}

// sendResponse tracks an initial response that send makes.
func (c *Context) sendResponse(send func(context.Context) error) error {
	r := c.reply

	// Claimed before sending, so that the 3-second cutoff cannot cancel a
	// response being sent. A second response is left for Discord to refuse.
	claimed := r.state.CompareAndSwap(pending, responded)
	if !claimed && r.state.Load() == tooLate {
		return errTooLate
	}

	err := send(r.api)
	if err != nil && claimed && notDelivered(err) {
		r.state.Store(pending)
	}

	if r.ack != nil {
		r.ack()
	}
	return err
}

// notDelivered reports whether a response certainly did not reach Discord:
// Discord refused it, other than for an earlier response, or it was never sent.
func notDelivered(err error) bool {
	var rest *client.RESTError
	if errors.As(err, &rest) {
		const alreadyAcknowledged = 40060
		return rest.StatusCode >= http.StatusBadRequest && rest.StatusCode < http.StatusInternalServerError &&
			rest.Code != alreadyAcknowledged
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
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
	ctx, err := c.api()
	if err != nil {
		return nil, err
	}
	return c.Session.InteractionResponse(ctx, c.Interaction)
}

// Edit replaces the text of the response.
func (c *Context) Edit(content string) error {
	return c.EditWith(MessageEdit{Content: &content})
}

// EditWith changes the response.
func (c *Context) EditWith(edit MessageEdit) error {
	ctx, err := c.api()
	if err != nil {
		return err
	}
	_, err = c.Session.InteractionResponseEdit(ctx, c.Interaction, edit.webhookEdit())
	return err
}

// Delete deletes the response.
func (c *Context) Delete() error {
	ctx, err := c.api()
	if err != nil {
		return err
	}
	return c.Session.InteractionResponseDelete(ctx, c.Interaction)
}

// Followup sends another message after the response.
func (c *Context) Followup(content string) (*FollowupMessage, error) {
	return c.FollowupWith(Message{Content: content})
}

// FollowupWith sends another message after the response.
func (c *Context) FollowupWith(msg Message) (*FollowupMessage, error) {
	ctx, err := c.api()
	if err != nil {
		return nil, err
	}

	sent, err := c.Session.FollowupMessageCreate(ctx, c.Interaction, msg.webhookParams())
	if err != nil {
		return nil, err
	}

	return &FollowupMessage{Message: sent, ctx: ctx, session: c.Session, interaction: c.Interaction}, nil
}
