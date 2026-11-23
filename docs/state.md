# State cache

A `client.State` keeps what the gateway sends, so that a handler can look up a guild, channel or member without a REST request. There is none by default. Set one before the bot or session opens:

```go
bot.Session.State = client.NewState(client.StateOptions{
	Members:       true, // all members; asks for the privileged Server Members intent
	ThreadMembers: true, // who is in each thread; the same intent
	VoiceStates:   true,
	Presences:     false, // the privileged Presence intent
	Messages:      100,   // the latest messages of each channel, with their reactions and poll votes
})

bot.OnMessage(func(c *minidis.MessageContext) error {
	perms, ok := c.Session.State.Permissions(c.ChannelID, c.Session.User().ID)
	if ok && perms&discord.PermissionAddReactions != 0 {
		return c.React("👀")
	}
	return nil
})
```

The session asks for the intents the state needs. Privileged ones must still be enabled in the Developer Portal; see [Intents](./getting-started.md#intents).

## What is kept

| Kept                                                         | Read with                                                                     |
| ------------------------------------------------------------ | ----------------------------------------------------------------------------- |
| Always: guilds with their roles, emojis and stickers         | `Guild`, `Guilds`, `Role`, `Emoji`, `Sticker`                                 |
| Always: channels, active threads and DM channels             | `Channel`, `GuildChannels`, `GuildThreads`                                    |
| Always: stage instances, scheduled events, soundboard sounds | `StageInstance(s)`, `ScheduledEvent(s)`, `SoundboardSound(s)`                 |
| Always: the bot's own member                                 | `Member`, `Permissions`                                                       |
| `Members` or `KeepMember`                                    | `Member`, `Members`, `Permissions`                                            |
| `ThreadMembers`                                              | `ThreadMember`, `ThreadMembers`                                               |
| `VoiceStates`                                                | `VoiceState`, `VoiceStates`                                                   |
| `Presences`                                                  | `Presence`, `Presences` (members who are not offline)                         |
| `Messages`                                                   | `Message`, `Messages` (oldest first), with reactions and poll votes counted as they come |

The readers return `nil`, or `ok == false` for `Permissions`, when something is not in the state.

- A `Guild` has its `Roles`, `Emojis` and `Stickers`, but not its channels, members and the other lists, which are the state's other methods.
- The bot's own member is enough for `Permissions` to say what the bot may do in a channel. A thread has the permissions of the channel it is in.
- Members are also picked up from messages, reactions, typing, interactions and voice states.
- Discord sends the members of large guilds only as they become active. `bot.Session.RequestGuildMembers` asks for the rest, and the chunks it brings are kept too.
- Discord reports who joins and leaves a thread rather than a list of its members, so a thread has those who joined while the bot was connected. `Session.ThreadMembers` lists them all.

Bans, invites, webhooks, integrations, auto moderation rules and entitlements are not kept: Discord never sends all of them on the gateway, so they could not be complete.

## Keeping only some members

`KeepMember` chooses which members to keep, to save memory in large guilds. A member is dropped as soon as it no longer passes, such as after losing a role, and the bot's own member is always kept:

```go
client.StateOptions{
	Members:    true,
	KeepMember: func(guildID string, m *discord.Member) bool { return slices.Contains(m.Roles, staffRoleID) },
}
```

Without `Members`, it chooses among the members Discord sends without the privileged intent: those of messages, reactions, typing, interactions, voice states and `RequestGuildMembers`.

`KeepMember` runs on the gateway's goroutine, after the event is applied and before its handlers start. It may call the state's methods, but must be quick, as events wait for it.

## Cached requests

The `…Cached` methods of the session take what the state has and ask Discord for the rest, so code works whether or not something is cached, and without a state at all:

| Method                            | Returns                                                  |
| --------------------------------- | -------------------------------------------------------- |
| `GuildCached`                     | A guild                                                  |
| `ChannelCached`                   | A channel or thread                                      |
| `GuildRoleCached`                 | A role                                                   |
| `GuildMemberCached`               | A member, then kept if the options keep it               |
| `ChannelMessageCached`            | A message                                                |
| `ChannelMemberPermissionsCached`  | A member's permissions in a channel, from the above      |

```go
member, err := c.Session.GuildMemberCached(c, guildID, userID)
perms, err := c.Session.ChannelMemberPermissionsCached(c, channelID, userID) // no request when all is cached
```

## Rules

- The state is updated before an event's handlers run, so a handler sees the state with its event applied.
- It starts over with each new gateway session, as Discord sends every guild again then.
- What it returns is shared: treat it as read-only. An update replaces a value instead of changing it, so a value already returned stays as it was.
- A guild Discord has announced but not sent yet, or that is in an outage, has only its `ID` and `Unavailable`.
- A bot that [receives interactions over HTTP](./http.md) has no gateway, so its state stays empty.
