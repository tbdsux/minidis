package interactions

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
		mc := &MessageCommandContext{Context: c}
		if data.Resolved != nil {
			mc.Message = data.Resolved.Messages[data.TargetID]
		}
		return cmd.Execute(mc)
	}
}

func (m *Minidis) routeUser(data discord.ApplicationCommandInteractionData) HandlerFunc {
	cmd, ok := m.userCommands[data.Name]
	if !ok || cmd.Execute == nil {
		return failWith(fmt.Errorf("%w: user command %q", ErrNoHandler, data.Name))
	}

	return func(c *Context) error {
		uc := &UserCommandContext{Context: c}
		if r := data.Resolved; r != nil {
			uc.TargetUser = r.Users[data.TargetID]
			// Discord leaves the user out of a resolved member.
			if m := r.Members[data.TargetID]; m != nil {
				member := *m
				member.User = uc.TargetUser
				uc.TargetMember = &member
			}
		}
		return cmd.Execute(uc)
	}
}

func (m *Minidis) routeEntryPoint(data discord.ApplicationCommandInteractionData) HandlerFunc {
	if m.entryPoint == nil || m.entryPoint.Execute == nil {
		return failWith(fmt.Errorf("%w: entry point command %q", ErrNoHandler, data.Name))
	}
	return m.entryPoint.Execute
}
