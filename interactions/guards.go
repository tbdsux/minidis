package interactions

import (
	"cmp"
	"fmt"
	"sync"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// Cooldown is middleware that lets each command run once per d for each key,
// such as PerUser, and answers the uses in between with an ephemeral message
// saying when the command can be used again. A command's subcommands share its
// cooldown. Components and modal submits are not limited.
//
// With Use it covers every command. To limit one, wrap its Execute:
//
//	Execute: interactions.Cooldown(time.Hour, interactions.PerUser)(daily),
//
// A d of zero or less limits nothing.
func Cooldown(d time.Duration, key func(*Context) string) Middleware {
	if d <= 0 {
		return func(next HandlerFunc) HandlerFunc { return next }
	}

	var (
		mu    sync.Mutex
		until = map[string]time.Time{}
	)

	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			if c.Interaction.Type != discord.InteractionApplicationCommand {
				return next(c)
			}
			// A slash command and a context menu command may share a name.
			data := c.Interaction.ApplicationCommandData()
			k := fmt.Sprint(data.CommandType, "\x00", data.Name, "\x00", key(c))

			mu.Lock()
			end, ok := until[k]
			limited := ok && time.Now().Before(end)
			if !limited {
				end = time.Now().Add(d)
				until[k] = end
				// Forget the key once it expires, unless a later use replaced it.
				time.AfterFunc(d, func() {
					mu.Lock()
					defer mu.Unlock()
					if until[k].Equal(end) {
						delete(until, k)
					}
				})
			}
			mu.Unlock()

			if limited {
				// Rounded up, so that the time shown is never in the past.
				return c.ReplyWith(Message{
					Content:   fmt.Sprintf("You can use this command again <t:%d:R>.", end.Add(time.Second-1).Unix()),
					Ephemeral: true,
				})
			}
			return next(c)
		}
	}
}

// PerUser is a Cooldown key that limits each user on their own.
func PerUser(c *Context) string { return c.Author.ID }

// PerGuild is a Cooldown key that limits each guild as a whole, and each DM
// on its own.
func PerGuild(c *Context) string { return cmp.Or(c.GuildID, c.ChannelID) }

// PerChannel is a Cooldown key that limits each channel as a whole.
func PerChannel(c *Context) string { return c.ChannelID }

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
