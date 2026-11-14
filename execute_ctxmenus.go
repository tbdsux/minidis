package minidis

import (
	"fmt"

	"github.com/tbdsux/minidis/discord"
)

func (m *Minidis) routeMessage(data discord.ApplicationCommandInteractionData) HandlerFunc {
	cmd, ok := m.messageCommands[data.Name]
	if !ok || cmd.Execute == nil {
		return failWith(fmt.Errorf("%w: message command %q", ErrNoHandler, data.Name))
	}

	return func(c *Context) error {
		return cmd.Execute(&MessageCommandContext{
			Context: c,
			Message: data.Resolved.Messages[data.TargetID],
		})
	}
}

func (m *Minidis) routeUser(data discord.ApplicationCommandInteractionData) HandlerFunc {
	cmd, ok := m.userCommands[data.Name]
	if !ok || cmd.Execute == nil {
		return failWith(fmt.Errorf("%w: user command %q", ErrNoHandler, data.Name))
	}

	return func(c *Context) error {
		return cmd.Execute(&UserCommandContext{
			Context:      c,
			TargetMember: data.Resolved.Members[data.TargetID],
			TargetUser:   data.Resolved.Users[data.TargetID],
		})
	}
}
