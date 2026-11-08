package minidis

import (
	"fmt"

	"github.com/tbdsux/minidis/discord"
)

func (m *Minidis) routeMessage(data discord.ApplicationCommandInteractionData) HandlerFunc {
	cmd, ok := m.MessageCommands[data.Name]
	if !ok || cmd.Execute == nil {
		return failWith(fmt.Errorf("%w: message command %q", ErrNoHandler, data.Name))
	}

	return func(c *SlashContext) error {
		return cmd.Execute(&MessageCommandContext{
			SlashContext: c,
			Message:      data.Resolved.Messages[data.TargetID],
		})
	}
}

func (m *Minidis) routeUser(data discord.ApplicationCommandInteractionData) HandlerFunc {
	cmd, ok := m.UserCommands[data.Name]
	if !ok || cmd.Execute == nil {
		return failWith(fmt.Errorf("%w: user command %q", ErrNoHandler, data.Name))
	}

	return func(c *SlashContext) error {
		return cmd.Execute(&UserCommandContext{
			SlashContext: c,
			Member:       data.Resolved.Members[data.TargetID],
			User:         data.Resolved.Users[data.TargetID],
		})
	}
}
