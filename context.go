package minidis

import (
	"github.com/bwmarrin/discordgo"
)

type SlashContext struct {
	Interaction *discordgo.Interaction
	Session     *discordgo.Session

	AppID  string
	Author *discordgo.User
	Member *discordgo.Member // only filled when called in a guild
	IsDM   bool
	Bot    *discordgo.User // this is the bot user

	Id          string
	GuildId     string
	GuildLocale *discordgo.Locale
	Local       discordgo.Locale
	ChannelId   string

	// NOTE: this is empty if component is called
	Options map[string]*discordgo.ApplicationCommandInteractionDataOption
}

// Creates a new slash context for slash command interaction. This is called internally.
func (m *Minidis) NewSlashContext(session *discordgo.Session, inter *discordgo.Interaction, isSlash bool) *SlashContext {
	context := &SlashContext{
		Interaction: inter,
		Session:     session,

		AppID:   session.State.User.ID,
		Options: map[string]*discordgo.ApplicationCommandInteractionDataOption{},
		Bot:     session.State.User,

		Id:          inter.ID,
		GuildId:     inter.GuildID,
		GuildLocale: inter.GuildLocale,
		Local:       inter.Locale,
		ChannelId:   inter.ChannelID,
	}

	if isSlash {
		// parse options into a map for better accessibility
		for _, v := range inter.ApplicationCommandData().Options {
			context.Options[v.Name] = v
		}
	}

	if inter.GuildID == "" {
		// if dm
		context.IsDM = true
		context.Author = inter.User
	} else {
		context.IsDM = false
		context.Author = inter.Member.User
		context.Member = inter.Member
	}

	return context
}

// SendText sends a string text as interaction response.
func (s *SlashContext) ReplyString(content string) error {
	// return errors.New("sample")

	return s.ReplyC(ReplyProps{
		Content: content,
	})
}

// Reply sends a string content with embeds if there is.
func (s *SlashContext) Reply(content string, embeds ...*discordgo.MessageEmbed) error {
	return s.ReplyC(ReplyProps{
		Content: content,
		Embeds:  embeds,
	})
}

// Reply sends a string content with embeds if there is. `Ephemeral` - the response message will only be seen
// by the user who called it.
func (s *SlashContext) ReplyEphemeral(content string, embeds ...*discordgo.MessageEmbed) error {
	return s.ReplyC(ReplyProps{
		Content:     content,
		Embeds:      embeds,
		IsEphemeral: true,
	})
}

type ReplyProps struct {
	Content         string
	Embeds          []*discordgo.MessageEmbed
	Components      []discordgo.MessageComponent
	IsEphemeral     bool
	Attachments     []*discordgo.File
	AllowedMentions *discordgo.MessageAllowedMentions
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
	res := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{},
	}

	if ephemeral {
		res.Data.Flags = 1 << 6
	}

	return s.Session.InteractionRespond(s.Interaction, res)
}

// Edit edis the interaction response.
func (s *SlashContext) Edit(content string) error {
	return s.EditC(EditProps{
		Content: content,
	})
}

type EditProps struct {
	Content         string
	Embeds          []*discordgo.MessageEmbed
	Components      []discordgo.MessageComponent
	Attachments     []*discordgo.File
	AllowedMentions *discordgo.MessageAllowedMentions
}

// EditC is the full edit interaction component structure.
func (s *SlashContext) EditC(reply EditProps) error {
	res := &discordgo.WebhookEdit{
		Content: &reply.Content,
	}

	if len(reply.Embeds) > 0 {
		res.Embeds = &reply.Embeds
	}

	if len(reply.Components) > 0 {
		res.Components = &reply.Components
	}

	if len(reply.Attachments) > 0 {
		res.Files = reply.Attachments
	}

	if reply.AllowedMentions != nil {
		res.AllowedMentions = reply.AllowedMentions
	}

	// edit interaction response
	_, err := s.Session.InteractionResponseEdit(s.Interaction, res)

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
	res := &discordgo.WebhookParams{
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
		res.Flags = 1 << 6
	}

	if reply.AllowedMentions != nil {
		res.AllowedMentions = reply.AllowedMentions
	}

	// send follup
	message, err := s.Session.FollowupMessageCreate(s.Interaction, true, res)
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
func replyFunc(session *discordgo.Session, interaction *discordgo.Interaction, reply ReplyProps) error {
	res := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
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
		res.Data.Flags = 1 << 6
	}

	if reply.AllowedMentions != nil {
		res.Data.AllowedMentions = reply.AllowedMentions
	}

	// send response
	return session.InteractionRespond(interaction, res)
}
