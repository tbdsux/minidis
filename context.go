package minidis

import "github.com/bwmarrin/discordgo"

type SlashContext struct {
	event   *discordgo.Interaction
	session *discordgo.Session
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
func (s *SlashContext) ReplyString(content string) (*InteractionContext, error) {
	return s.ReplyC(ReplyProps{
		Content: content,
	})
}

// Reply sends a string content with embeds if there is.
func (s *SlashContext) Reply(content string, embeds ...*discordgo.MessageEmbed) (*InteractionContext, error) {
	return s.ReplyC(ReplyProps{
		Content: content,
		Embeds:  embeds,
	})
}

// Reply sends a string content with embeds if there is. `Ephemeral` - the response message will only be seen
// by the user who called it.
func (s *SlashContext) ReplyEphemeral(content string, embeds ...*discordgo.MessageEmbed) (*InteractionContext, error) {
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
func (s *SlashContext) ReplyC(reply ReplyProps) (*InteractionContext, error) {
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
	if err := s.session.InteractionRespond(s.event, res); err != nil {
		return nil, err
	}

	// return new context
	return s.NewInteractionContext(), nil
}
