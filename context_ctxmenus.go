package minidis

import (
	"github.com/tbdsux/minidis/discord"
)

// MessageCommandContext is passed to message context menu commands. The
// embedded Context describes the user who ran the command and responds to it.
type MessageCommandContext struct {
	*Context
	Message *discord.Message // the message the command was used on
}

// UserCommandContext is passed to user context menu commands. The embedded
// Context describes the user who ran the command and responds to it.
type UserCommandContext struct {
	*Context
	TargetUser   *discord.User   // the user the command was used on
	TargetMember *discord.Member // TargetUser as a guild member, or nil
}
