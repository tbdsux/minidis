# Messages and events

These need the gateway, so they work with `minidis.Bot`, but not with a bot that [receives interactions over HTTP](./http.md).

## Gateway events

`minidis.On` handles any gateway event, chosen by the type of the handler's second parameter:

```go
minidis.On(bot, func(c *minidis.EventContext, e *discord.GuildMemberAdd) error {
	_, err := c.Session.ChannelMessageSend(c, welcomeChannelID, "Welcome, "+e.Mention()+"!")
	return err
})

minidis.On(bot, func(c *minidis.EventContext, e *discord.Ready) error {
	log.Println("Logged in as", e.User.Username)
	return nil
})
```

Every event type of the `discord` package works, such as `discord.GuildCreate`, `discord.MessageReactionAdd`, `discord.ChannelCreate`, `discord.ThreadCreate`, `discord.VoiceStateUpdate` and `discord.PresenceUpdateEvent`. `On` panics for a type that is not an event. Several handlers can be added for the same event.

The bot asks Discord for the intents each event needs. Some are privileged and must be enabled in the Developer Portal; see [Intents](./getting-started.md#intents). A handler of message events that reads what messages say also needs `bot.AddIntents(discord.IntentsMessageContent)`.

The `EventContext` has the `Bot`, the `Session` ([the client](./client.md)) and the `Event` name, such as `"GUILD_MEMBER_ADD"`. It is a `context.Context` for the handler's work, canceled when the handler returns or a closing bot stops waiting. Each event runs its handlers in goroutines of their own, so a slow handler does not hold up the others.

Errors and panics are logged, or go to `bot.HandleEventError`; see [Middleware and errors](./middleware-and-errors.md#events-and-messages).

## Messages

`bot.OnMessage` handles the messages people send, in every channel the bot can see and in its DMs:

```go
bot.OnMessage(func(c *minidis.MessageContext) error {
	if c.IsDM {
		_, err := c.Reply("I only work in servers.")
		return err
	}
	return c.React("👀")
})
```

Messages from bots, the bot itself included, from webhooks, and Discord's own notices, such as that someone joined, are left out, so that bots do not answer each other forever. Use `minidis.On` with a `discord.MessageCreate` to see those too.

A `MessageContext` has:

| Field or method                            | Is                                                                            |
| ------------------------------------------ | ----------------------------------------------------------------------------- |
| `Message`, `Content`                       | The message, and its text                                                     |
| `Author`, `Member`                         | Who sent it, and as a guild member (nil in a DM)                              |
| `GuildID`, `ChannelID`, `IsDM`             | Where it was sent                                                             |
| `Send`, `SendWith`                         | Send a message to the same channel                                            |
| `Reply`, `ReplyWith`                       | Send a message as a reply to it                                               |
| `React(emoji)`                             | Add a reaction: a Unicode emoji, or `"name:id"` for a custom one               |
| `Delete()`                                 | Delete it, which needs the Manage Messages permission                         |
| `Typing()`                                 | Show the bot as typing for 10 seconds, or until it sends                      |

`SendWith` and `ReplyWith` take a `minidis.Message`, like interaction responses do. When the content repeats what users wrote, set `AllowedMentions: &discord.MessageAllowedMentions{}` so that it cannot ping `@everyone`.

Reading the content of messages needs the privileged Message Content intent, which `OnMessage` asks for. Without it enabled in the Developer Portal, `Content` is empty unless the message mentions the bot or is a DM.

## Text commands

Slash commands are the better choice for most bots: Discord shows them to users, checks their options, and they need no privileged intent. Commands written as messages are there too:

```go
bot.SetPrefix("!")

bot.AddTextCommand(&minidis.TextCommand{
	Name:        "say",
	Aliases:     []string{"echo"},
	Description: "Repeats what you wrote.",
	Execute: func(c *minidis.MessageContext) error {
		// For "!say hello there": c.Command is "say", c.Args is ["hello", "there"]
		// and c.ArgString is "hello there"
		_, err := c.SendWith(minidis.Message{Content: c.ArgString, AllowedMentions: &discord.MessageAllowedMentions{}})
		return err
	},
})
```

- Names and aliases are matched regardless of case. A command added later replaces one of the same name, and takes over its aliases.
- Mentioning the bot always works, as in `@bot say hi`, and needs no privileged intent. Without a prefix, it is the only way. A prefix needs the Message Content intent.
- `bot.TextCommands()` lists the commands sorted by name, with their `Description`, for a help command of your own.
- Text commands run for messages from people only. `OnMessage` handlers see those messages too.

Their errors, and those of `OnMessage` handlers, go to `bot.HandleMessageError`, which can tell the user with `c.Reply`.

## Messages of one channel

`bot.HandleChannelMessage` sets the handler for the messages of a single channel, replacing any set before:

```go
bot.HandleChannelMessage(channelID, func(c *minidis.ChannelMessageContext) error {
	_, err := c.Reply("hello") // or c.Send, which does not reply to the message
	return err
})
```

The bot's own messages are ignored. Errors go to `bot.HandleChannelMessageError`. It is also available on the `interactions` bot, which has no `OnMessage`.

## Presence

```go
bot.SetPresence(discord.StatusIdle, &discord.Activity{Name: "the logs", Type: discord.ActivityWatching})
```

The status is one of `StatusOnline`, `StatusIdle`, `StatusDND` and `StatusInvisible`. `SetPresence` can be called before the bot connects, and is kept across reconnects.

## Lower level

`bot.OnMessageCreate` and the `On…` methods of `bot.Session` take handlers without a context or error handling, and see everything, the bot's own messages included. Intents for handlers added on `bot.Session` are not worked out, so ask for them with `bot.AddIntents`. See [The Discord client](./client.md#gateway).

Next: [Middleware and errors](./middleware-and-errors.md).
