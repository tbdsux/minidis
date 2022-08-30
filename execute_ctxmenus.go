package minidis

import "github.com/bwmarrin/discordgo"

func (m *Minidis) executeMessage(s *discordgo.Session, i *discordgo.Interaction) (error, bool) {
	command := i.ApplicationCommandData().Name

	if handler, ok := m.messageCommands[command]; ok {
		return handler.Execute(&MessageCommandContext{
			Session:     s,
			Interaction: i,
			Message:     i.ApplicationCommandData().Resolved.Messages[i.ApplicationCommandData().TargetID],
		}), true
	}

	return nil, false
}

func (m *Minidis) executeUser(s *discordgo.Session, i *discordgo.Interaction) (error, bool) {
	command := i.ApplicationCommandData().Name

	if handler, ok := m.userCommands[command]; ok {
		return handler.Execute(&UserCommandContext{
			Session:     s,
			Interaction: i,
			Member:      i.ApplicationCommandData().Resolved.Members[i.ApplicationCommandData().TargetID],
			User:        i.ApplicationCommandData().Resolved.Users[i.ApplicationCommandData().TargetID],
		}), true
	}

	return nil, false
}
