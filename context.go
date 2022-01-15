package minidis

import "github.com/bwmarrin/discordgo"

type SlashContext struct {
	event   *discordgo.Interaction
	session *discordgo.Session
	AppID   string
	Author  *discordgo.User
	Member  *discordgo.Member // only filled when called in a guild
	IsDM    bool
	Options []*discordgo.ApplicationCommandInteractionDataOption
}

// Creates a new slash context for slash command interaction. This is called internally.
func (m *Minidis) NewSlashContext(session *discordgo.Session, event *discordgo.Interaction) *SlashContext {
	context := &SlashContext{
		event:   event,
		session: session,
		AppID:   session.State.User.ID,
		Options: event.ApplicationCommandData().Options,
	}

	if event.GuildID == "" {
		// if dm
		context.IsDM = true
		context.Author = event.User
	} else {
		context.IsDM = false
		context.Author = event.Member.User
		context.Member = event.Member
	}

	return context
}

// SendText sends a string text as interaction response.
func (s *SlashContext) ReplyString(content string) error {
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
	return s.session.InteractionRespond(s.event, res)
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

	if reply.AllowedMentions != nil {
		res.AllowedMentions = reply.AllowedMentions
	}

	// edit interaction response
	_, err := s.session.InteractionResponseEdit(s.AppID, s.event, res)

	return err
}

// Delete deletes the interaction response.
func (s *SlashContext) Delete() error {
	return s.session.InteractionResponseDelete(s.AppID, s.event)
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
	message, err := s.session.FollowupMessageCreate(s.AppID, s.event, true, res)
	if err != nil {
		return nil, err
	}

	// return new context
	return &FollowupContext{
		message: message,
		session: s.session,
		event:   s.event,
		AppID:   s.AppID,
	}, nil
}
