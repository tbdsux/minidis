package minidis

import "github.com/bwmarrin/discordgo"

func (m *Minidis) executeMessage(s *discordgo.Session, i *discordgo.Interaction) error {
	command := i.ApplicationCommandData().Name

	if handler, ok := m.messageCommands[command]; ok {
		return handler.Execute(&MessageCommandContext{
			session: s,
			event:   i,
			Message: i.ApplicationCommandData().Resolved.Messages[i.ApplicationCommandData().TargetID],
		})
	}

	return nil

}

func (m *Minidis) executeUser(s *discordgo.Session, i *discordgo.Interaction) error {
	command := i.ApplicationCommandData().Name

	if handler, ok := m.userCommands[command]; ok {
		return handler.Execute(&UserCommandContext{
			session: s,
			event:   i,
			Member:  i.ApplicationCommandData().Resolved.Members[i.ApplicationCommandData().TargetID],
			User:    i.ApplicationCommandData().Resolved.Users[i.ApplicationCommandData().TargetID],
		})
	}

	return nil
}
