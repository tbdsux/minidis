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
func (s *SlashContext) SendText(content string) error {
	res := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
		},
	}

	return s.session.InteractionRespond(s.event, res)
}
