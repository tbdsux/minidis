package minidis

import (
	"github.com/tbdsux/minidis/discord"
)

// MessageCommandContext is passed to message context menu commands. The embedded
// SlashContext describes the user who ran the command and provides the reply
// methods.
type MessageCommandContext struct {
	*SlashContext
	Message *discord.Message // the message the command was used on
}

// UserCommandContext is passed to user context menu commands. Member and User
// are the target of the command; the embedded SlashContext describes the user
// who ran it (c.Author, c.SlashContext.Member) and provides the reply methods.
type UserCommandContext struct {
	*SlashContext
	Member *discord.Member
	User   *discord.User
}
