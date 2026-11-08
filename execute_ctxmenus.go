package minidis

import (
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

func (m *Minidis) executeMessage(s *client.Session, i *discord.Interaction) (error, bool) {
	command := i.ApplicationCommandData().Name

	if handler, ok := m.MessageCommands[command]; ok {
		return handler.Execute(&MessageCommandContext{
			Session:     s,
			Interaction: i,
			Message:     i.ApplicationCommandData().Resolved.Messages[i.ApplicationCommandData().TargetID],
		}), true
	}

	return nil, false
}

func (m *Minidis) executeUser(s *client.Session, i *discord.Interaction) (error, bool) {
	command := i.ApplicationCommandData().Name

	if handler, ok := m.UserCommands[command]; ok {
		return handler.Execute(&UserCommandContext{
			Session:     s,
			Interaction: i,
			Member:      i.ApplicationCommandData().Resolved.Members[i.ApplicationCommandData().TargetID],
			User:        i.ApplicationCommandData().Resolved.Users[i.ApplicationCommandData().TargetID],
		}), true
	}

	return nil, false
}
