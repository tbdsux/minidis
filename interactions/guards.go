package interactions

import "github.com/tbdsux/minidis/discord"

// RequirePermissions is middleware that runs the handler only for members who
// have all of perms in the channel, such as discord.PermissionBanMembers, and
// answers others with an ephemeral message. Administrators have every
// permission. Outside a guild, where there are no permissions, it answers
// that the command only works in a server.
//
// Command.DefaultMemberPermissions hides a command from members without the
// permissions, but server admins can change who sees it; this check holds
// regardless. With Use it covers every interaction. To guard one command, wrap
// its Execute:
//
//	Execute: interactions.RequirePermissions(discord.PermissionBanMembers)(ban),
func RequirePermissions(perms int64) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			if c.Member == nil {
				return c.ReplyWith(Message{Content: "This only works in a server.", Ephemeral: true})
			}
			have := c.Member.Permissions
			if have&discord.PermissionAdministrator == 0 && have&perms != perms {
				return c.ReplyWith(Message{Content: "You don't have the permissions to use this.", Ephemeral: true})
			}
			return next(c)
		}
	}
}
