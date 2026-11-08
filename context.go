package minidis

import (
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

type SlashContext struct {
	Interaction *discord.Interaction
	Session     *client.Session

	AppID  string
	Author *discord.User
	Member *discord.Member // only filled when called in a guild
	IsDM   bool
	Bot    *discord.User // this is the bot user

	Id          string
	GuildId     string
	GuildLocale *discord.Locale
	Local       discord.Locale
	ChannelId   string

	// NOTE: this is empty if component is called.
	// When a subcommand is called, these are the options of that subcommand.
	Options map[string]*discord.ApplicationCommandInteractionDataOption
}

// Creates a new slash context for slash command interaction. This is called internally.
func (m *Minidis) NewSlashContext(session *client.Session, inter *discord.Interaction, isSlash bool) *SlashContext {
	context := &SlashContext{
		Interaction: inter,
		Session:     session,

		AppID:   inter.AppID,
		Options: map[string]*discord.ApplicationCommandInteractionDataOption{},
		Bot:     session.User(),

		Id:          inter.ID,
		GuildId:     inter.GuildID,
		GuildLocale: inter.GuildLocale,
		Local:       inter.Locale,
		ChannelId:   inter.ChannelID,
	}

	if data, ok := inter.Data.(discord.ApplicationCommandInteractionData); ok && isSlash {
		context.setOptions(data.Options)
	}

	context.IsDM = inter.GuildID == ""
	context.Member = inter.Member

	if inter.Member != nil {
		context.Author = inter.Member.User
	} else {
		context.Author = inter.User
	}

	return context
}

// setOptions indexes options by name for better accessibility.
func (s *SlashContext) setOptions(options []*discord.ApplicationCommandInteractionDataOption) {
	s.Options = make(map[string]*discord.ApplicationCommandInteractionDataOption, len(options))
	for _, v := range options {
		s.Options[v.Name] = v
	}
}

// SendText sends a string text as interaction response.
func (s *SlashContext) ReplyString(content string) error {
	// return errors.New("sample")

	return s.ReplyC(ReplyProps{
		Content: content,
	})
}

// Reply sends a string content with embeds if there is.
func (s *SlashContext) Reply(content string, embeds ...*discord.MessageEmbed) error {
	return s.ReplyC(ReplyProps{
		Content: content,
		Embeds:  embeds,
	})
}

// Reply sends a string content with embeds if there is. `Ephemeral` - the response message will only be seen
// by the user who called it.
func (s *SlashContext) ReplyEphemeral(content string, embeds ...*discord.MessageEmbed) error {
	return s.ReplyC(ReplyProps{
		Content:     content,
		Embeds:      embeds,
		IsEphemeral: true,
	})
}

type ReplyProps struct {
	Content         string
	Embeds          []*discord.MessageEmbed
	Components      []discord.MessageComponent
	IsEphemeral     bool
	Attachments     []*discord.File
	AllowedMentions *discord.MessageAllowedMentions
}

// ReplyC is the full reply component structure.
func (s *SlashContext) ReplyC(reply ReplyProps) error {
	return replyFunc(s.Session, s.Interaction, reply)
}

// DeferReply sends an interaction response where the user sees a loading state.
// After sending, 15 minutes is given to complete your command's tasks.
// This is considered as an interaction response, so you should not use the `Reply*` functions after.
// - `ephemeral` -> only the user sees the loading state
func (s *SlashContext) DeferReply(ephemeral bool) error {
	res := &discord.InteractionResponse{
		Type: discord.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discord.InteractionResponseData{},
	}

	if ephemeral {
		res.Data.Flags = discord.MessageFlagsEphemeral
	}

	return s.Session.InteractionRespond(s.Interaction, res)
}

// Edit edis the interaction response.
func (s *SlashContext) Edit(content string) error {
	return s.EditC(EditProps{
		Content: content,
	})
}

// EditProps changes a message. Nil Embeds or Components are left unchanged; an
// empty non-nil slice clears them.
type EditProps struct {
	Content         string
	Embeds          []*discord.MessageEmbed
	Components      []discord.MessageComponent
	Attachments     []*discord.File
	AllowedMentions *discord.MessageAllowedMentions
}

func (p EditProps) webhookEdit() *discord.WebhookEdit {
	res := &discord.WebhookEdit{
		Content:         &p.Content,
		Files:           p.Attachments,
		AllowedMentions: p.AllowedMentions,
	}

	if p.Embeds != nil {
		res.Embeds = &p.Embeds
	}

	if p.Components != nil {
		res.Components = &p.Components
	}

	return res
}

// EditC is the full edit interaction component structure.
func (s *SlashContext) EditC(reply EditProps) error {
	_, err := s.Session.InteractionResponseEdit(s.Interaction, reply.webhookEdit())

	return err
}

// Delete deletes the interaction response.
func (s *SlashContext) Delete() error {
	return s.Session.InteractionResponseDelete(s.Interaction)
}

// Followup creates a followup message to the interaction response.
func (s *SlashContext) Followup(content string) (*FollowupContext, error) {
	return s.FollowupC(FollowupProps{
		Content: content,
	})
}

type FollowupProps ReplyProps

// FollowupC is the full followup component structure.
func (s *SlashContext) FollowupC(reply FollowupProps) (*FollowupContext, error) {
	res := &discord.WebhookParams{
		Content: reply.Content,
	}

	if len(reply.Embeds) > 0 {
		res.Embeds = reply.Embeds
	}

	if len(reply.Components) > 0 {
		res.Components = reply.Components
	}

	if len(reply.Attachments) > 0 {
		res.Files = reply.Attachments
	}

	if reply.IsEphemeral {
		res.Flags = discord.MessageFlagsEphemeral
	}

	if reply.AllowedMentions != nil {
		res.AllowedMentions = reply.AllowedMentions
	}

	// send follup
	message, err := s.Session.FollowupMessageCreate(s.Interaction, res)
	if err != nil {
		return nil, err
	}

	// return new context
	return &FollowupContext{
		message:     message,
		Session:     s.Session,
		Interaction: s.Interaction,
		AppID:       s.AppID,
	}, nil
}

// general function for sending replies back
func replyFunc(session *client.Session, interaction *discord.Interaction, reply ReplyProps) error {
	res := &discord.InteractionResponse{
		Type: discord.InteractionResponseChannelMessageWithSource,
		Data: &discord.InteractionResponseData{
			Content: reply.Content,
		},
	}

	if len(reply.Embeds) > 0 {
		res.Data.Embeds = reply.Embeds
	}

	if len(reply.Components) > 0 {
		res.Data.Components = reply.Components
	}

	if len(reply.Attachments) > 0 {
		res.Data.Files = reply.Attachments
	}

	if reply.IsEphemeral {
		res.Data.Flags = discord.MessageFlagsEphemeral
	}

	if reply.AllowedMentions != nil {
		res.Data.AllowedMentions = reply.AllowedMentions
	}

	// send response
	return session.InteractionRespond(interaction, res)
}

type ReplyModalProps struct {
	Title      string
	CustomID   string
	Components []discord.MessageComponent
}

// ReplyModal returns a modal component as InteractionResponse
func (s *SlashContext) ReplyModal(reply ReplyModalProps) error {
	res := &discord.InteractionResponse{
		Type: discord.InteractionResponseModal,
		Data: &discord.InteractionResponseData{
			Title:      reply.Title,
			CustomID:   reply.CustomID,
			Components: reply.Components,
		},
	}

	return s.Session.InteractionRespond(s.Interaction, res)
}
