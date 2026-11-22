# minidis

Simple, opinionated Discord bot framework for Go, built directly on the [Discord API](https://docs.discord.com/developers/intro).

```go
bot := minidis.New(os.Getenv("TOKEN"))

bot.AddCommand(&minidis.Command{
	Name:        "ping",
	Description: "Replies with pong.",
	Execute: func(c *minidis.Context) error {
		return c.Reply("pong")
	},
})

log.Fatal(bot.Start())
```

It is made of layers, and each works without the ones above it:

| Package                  | What it is                                                                                                                                                        |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `minidis`                | The framework: one `Bot` for commands, components, messages and every gateway event.                                                                              |
| `minidis/interactions`   | The framework for interactions alone: slash commands, context menus, components, modals and autocomplete. Also for bots that only receive interactions over HTTP. |
| `minidis/discord/client` | The REST and Gateway client.                                                                                                                                      |
| `minidis/discord`        | The Discord API types.                                                                                                                                            |

## Install

```sh
go get -u github.com/tbdsux/minidis
```

## [CLI](./cli/minidis/README.md)

You can also use a simple boilerplate generator to kickstart a new discord bot project

```sh
go install github.com/tbdsux/minidis/cli/minidis@latest
```

## Usage

```go
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/tbdsux/minidis"
	"github.com/tbdsux/minidis/discord"
)

func main() {
	bot := minidis.New(os.Getenv("TOKEN"))

	// Commands show up at once in a guild, so name one while developing.
	// With GUILD unset, they are registered globally.
	bot.SetGuilds(os.Getenv("GUILD"))

	// A slash command
	bot.AddCommand(&minidis.Command{
		Name:        "ping",
		Description: "Simple ping command.",
		Execute: func(c *minidis.Context) error {
			return c.Reply(fmt.Sprintf("Hello **%s**, pong?", c.Author.Username))
		},
	})

	// A gateway event, chosen by the type of the second parameter
	minidis.On(bot, func(c *minidis.EventContext, e *discord.GuildMemberAdd) error {
		log.Println(e.User.Username, "joined", e.GuildID)
		return nil
	})

	// Messages people send
	bot.OnMessage(func(c *minidis.MessageContext) error {
		if c.Content == "hello" {
			_, err := c.Reply("Hello, " + c.Author.Mention() + "!")
			return err
		}
		return nil
	})

	// Register the commands, connect, and run until CTRL+C. Then new events
	// stop, and running handlers get up to 10 seconds to finish before their
	// contexts are canceled
	if err := bot.Start(); err != nil {
		log.Fatalln("Bot stopped with an error:", err)
	}
}
```

`Start` is `Run` with a context that ends on CTRL+C; call `bot.Run(ctx)` to decide yourself when the bot stops. Both register the commands first, replacing the ones Discord has, unless the bot has none or `bot.DisableSync()` was called. `Run` also returns if Discord ends the connection for good, such as for an invalid token. Use `bot.Open(ctx)` and `bot.Close` to manage the connection yourself. A handler that closes the bot should call `Close` in a new goroutine, as `Close` waits for running handlers.

There are no intents to set: the bot asks Discord for the ones its handlers need. The privileged ones must also be enabled for the bot in the developer portal, under Bot > Privileged Gateway Intents, and `Run` says which when Discord refuses them. `bot.AddIntents` asks for more, and `bot.SetIntents` takes over.

More [examples...](./example/)

## Events and messages

### Events

`minidis.On` handles any gateway event, chosen by the type of the handler's second parameter: every event type of the `discord` package works, such as `discord.GuildMemberAdd`, `discord.MessageReactionAdd`, `discord.ChannelCreate` or `discord.VoiceStateUpdate`.

```go
minidis.On(bot, func(c *minidis.EventContext, e *discord.GuildMemberAdd) error {
	_, err := c.Session.ChannelMessageSend(c, welcomeChannel, "Welcome, "+e.Mention()+"!")
	return err
})
```

The `EventContext` is a `context.Context` for the handler's work, canceled when the handler returns or a closing bot stops waiting. `c.Session` is the [client](#client). Errors and panics are logged, or go to `bot.HandleEventError`.

### Messages

`bot.OnMessage` handles the messages people send, in every channel the bot can see and in its DMs. Messages from bots, the bot itself included, and from webhooks are left out; use `minidis.On` with a `discord.MessageCreate` to see those too.

```go
bot.OnMessage(func(c *minidis.MessageContext) error {
	if c.IsDM {
		_, err := c.Reply("I only work in servers.")
		return err
	}
	return c.React("👀")
})
```

A `MessageContext` has the `Message`, its `Content`, `Author` and `Member`, and `Send`, `Reply`, `React`, `Delete` and `Typing`. `SendWith` and `ReplyWith` take a `minidis.Message`, like interaction responses do. Reading the content of messages needs the privileged Message Content Intent.

### Text commands

Slash commands are the better choice for most bots, but commands written as messages are there too:

```go
bot.SetPrefix("!")

bot.AddTextCommand(&minidis.TextCommand{
	Name:    "say",
	Aliases: []string{"echo"},
	Execute: func(c *minidis.MessageContext) error {
		// For "!say hello there": c.Args is ["hello", "there"] and c.ArgString "hello there"
		_, err := c.SendWith(minidis.Message{Content: c.ArgString, AllowedMentions: &discord.MessageAllowedMentions{}})
		return err
	},
})
```

Without a prefix, a text command is run by mentioning the bot, as in `@bot say hi`, which needs no privileged intent. Their errors, and those of `OnMessage` handlers, go to `bot.HandleMessageError`, which can tell the user about it with `c.Reply`.

## Interactions

Everything below comes from the `interactions` package, whose types the framework has under the same names. A bot that only handles interactions can use that package on its own, with `interactions.New`, `SyncCommands` and `Run`, or receive them over HTTP.

### Permissions

A command is usable by everyone unless `DefaultMemberPermissions` is set.

```go
DefaultMemberPermissions: discord.Int64(0),                             // admins only
DefaultMemberPermissions: discord.Int64(discord.PermissionManageGuild), // members who can manage the server
```

Global commands are usable in guilds only, where member permissions apply, unless `Contexts` says otherwise. User-installable commands, whose `IntegrationTypes` include `discord.ApplicationIntegrationUserInstall`, and the entry point command keep Discord's default of everywhere instead. Sync user-installable commands globally.

```go
Contexts: []discord.InteractionContextType{discord.InteractionContextGuild, discord.InteractionContextBotDM},
```

### Subcommands

```go
bot.AddCommand(&minidis.Command{
	Name:        "role",
	Description: "Manage roles.",
	Subcommands: []*minidis.Subcommand{
		{Name: "list", Description: "List roles.", Execute: listRoles},
	},
	Groups: []*minidis.SubcommandGroup{
		{Name: "member", Description: "Roles of a member.", Subcommands: []*minidis.Subcommand{
			{Name: "add", Description: "Give a role.", Execute: addRole},
		}},
	},
})
```

A command with subcommands only runs them, so it cannot have `Options`, an `Execute` or an `Autocomplete` of its own. `SyncCommands` reports such a command, and also an option with `Autocomplete: true` that no handler answers.

### Responding

```go
c.Reply("pong")
c.ReplyWith(minidis.Message{Content: "only you can see this", Ephemeral: true})

c.DeferReply(false) // for work that takes more than 3 seconds, then:
c.Edit("done")
c.EditWith(minidis.MessageEdit{Content: discord.String("done"), Components: []discord.MessageComponent{}})

f, err := c.Followup("one more thing")
c.ReplyModal(minidis.Modal{CustomID: "feedback", Title: "Feedback", Components: ...})
```

In a `MessageEdit`, a nil field is left unchanged and an empty one clears it. A slice built with `append` that ends up empty is still nil, so start from an empty slice when the result should clear. `Flags` replaces the message's flags, so include those to keep.

`Message` also takes a `Poll`, `TTS` and further `Flags`. Use `discord.MessageFlagsIsComponentsV2` to lay a message out with components such as `discord.Container`, `discord.Section` and `discord.TextDisplay`. A deferred reply becomes one through `c.EditWith(minidis.MessageEdit{Flags: discord.MessageFlagsIsComponentsV2, Components: ...})`. `c.Response()` fetches the message that was sent, and `c.SendResponse` sends a response built by hand and returns what Discord reports about it.

Every handler context is also a `context.Context` for the handler's own work. It is canceled when the handler returns, 15 minutes after the interaction arrived (when Discord stops accepting responses), after 3 seconds if nothing responded by then (with `minidis.ErrNoResponse` as its cause), and when a closing bot stops waiting for its handlers. Pass it to the calls the handler makes, including those on `c.Session`:

```go
rows, err := db.QueryContext(c, "SELECT ...")
_, err = c.Session.ChannelMessageSend(c, logChannelID, "ran /ping")
```

The response methods (`Reply`, `Edit`, `Followup` and the rest) keep working after the handler returns, until Discord's 15 minutes are up or the bot closes, so a followup can be edited later. Respond through them rather than `c.Session`: a response sent another way is not seen, and the context is canceled after 3 seconds.

`c.Responded()` reports whether the interaction has been responded to, so that a message goes through `c.Followup` instead of `c.Reply` afterwards. A response Discord refused does not count.

### Reading options

These never panic. A missing option or one of another type gives the zero value or `nil`.

```go
name := c.String("name")
count, given := c.OptionalInt("count")
target := c.User("target")    // also c.OptionMember, c.Channel, c.Role, c.Attachment
```

### Autocomplete

Set `Autocomplete: true` on the option and an `Autocomplete` handler on the command or subcommand.

```go
Autocomplete: func(c *minidis.AutocompleteContext) error {
	return c.Respond(minidis.Choice("Go", "go"))  // c.Input() is what the user typed
},
```

### Middleware and errors

```go
bot.Use(func(next minidis.HandlerFunc) minidis.HandlerFunc {
	return func(c *minidis.Context) error {
		// runs around every command, component and modal handler;
		// c.WithContext passes values on to the handler
		return next(c.WithContext(context.WithValue(c, requestIDKey, newRequestID())))
	}
})

bot.HandleError(func(c *minidis.Context, err error) {
	// err is a *minidis.PanicError if a handler panicked (errors.Is and
	// errors.As see through it), and wraps minidis.ErrNoHandler for a
	// command or component that is no longer registered. c is the copy that
	// middleware passed on, with its values.
	if c.Interaction.Type == discord.InteractionApplicationCommandAutocomplete {
		return // autocomplete cannot show a message
	}
	if c.Responded() {
		c.FollowupWith(minidis.Message{Content: "Something went wrong.", Ephemeral: true})
	} else {
		c.ReplyWith(minidis.Message{Content: "Something went wrong.", Ephemeral: true})
	}
})
```

Without `HandleError`, errors are logged. A panic in the error handler is logged too. Channel message handlers have `HandleChannelMessageError`.

### Components and modals

Handler IDs can capture `:`-separated segments, and modal IDs work the same way. An exact ID wins over a pattern, and a pattern with more fixed segments over one with fewer, so `ticket:close:{id}` wins over `ticket:{action}:{id}`.

```go
bot.HandleComponent("vote:{poll}", func(c *minidis.ComponentContext) error {
	return c.Update("Voted in " + c.Params["poll"]) // edits the message with the button
})

bot.HandleModal("feedback", func(c *minidis.ModalSubmitContext) error {
	return c.Reply("Thanks!")
})
```

Use `c.UpdateWith` for embeds and components, passing an empty non-nil slice to clear them, and `c.DeferUpdate` for slow work. A modal opened from a component can update that component's message the same way. A capture is one segment, so a value put there must not contain `:`.

`HandleComponentFallback` and `HandleModalFallback` receive the IDs that no handler matches. Without them, such interactions reach the error handler as `minidis.ErrNoHandler`.

A submitted modal is read by the custom IDs of its components:

```go
c.Text("name")         // a text input, or the choice of a discord.RadioGroup
c.Selected("toppings") // a select menu, a discord.CheckboxGroup, or the IDs of a discord.FileUpload
c.Checked("agree")     // a discord.Checkbox
c.Attachments("files") // the files of a discord.FileUpload
```

### Receiving interactions over HTTP

Instead of the gateway, Discord can send interactions to the app's Interactions Endpoint URL. Discord uses one or the other, so pick either `Open`/`Run` or HTTP. `HTTPHandler` verifies requests with the app's public key, rejects replayed and stale ones, answers Discord's PING, and runs the same handlers. Each request is answered once Discord has the response, or when the 3 seconds pass without one. It looks up the bot's user for `c.Bot`, since there is no gateway session. `Shutdown` waits for running handlers; after it, an HTTP-only bot cannot be started again, as `Open` would connect the gateway. See the [http example](./example/http/main.go).

```go
handler, err := bot.HTTPHandler(ctx, os.Getenv("PUBLIC_KEY"))
mux.Handle("POST /interactions", handler)
```

### Activities

`bot.SetEntryPoint` sets the command that launches the app's Activity. With `discord.EntryPointHandlerApp`, its `Execute` runs and responds with `c.LaunchActivity()`. Without one, syncing global commands keeps the entry point Discord created, which it refuses to remove in a bulk update.

### Channel messages

For the messages of one channel; `bot.OnMessage` handles those of every channel.

```go
bot.HandleChannelMessage(channelID, func(c *minidis.ChannelMessageContext) error {
	_, err := c.Reply("hello") // or c.Send, which does not reply to the message
	return err
})
```

`SendWith` and `ReplyWith` take a `minidis.Message`. When the content repeats what users wrote, set `AllowedMentions` so that it cannot ping `@everyone`.

### Presence

```go
bot.SetPresence(discord.StatusIdle, &discord.Activity{Name: "the logs", Type: discord.ActivityWatching})
```

## Client

`discord/client` is the whole Discord API for bots, and works on its own, without `interactions` ([example](./example/client/main.go)). A bot made with `minidis.New` has the same client as `bot.Session`.

```go
s := client.New("Bot " + os.Getenv("TOKEN"))
s.Intents = discord.IntentsGuilds | discord.IntentsGuildMessages | discord.IntentsMessageContent

s.OnMessageCreate(func(s *client.Session, e *discord.MessageCreate) {
	if e.Content == "!ping" {
		s.ChannelMessageSend(context.Background(), e.ChannelID, "pong")
	}
})

if err := s.Open(ctx); err != nil {
	log.Fatal(err)
}
defer s.Close()
<-ctx.Done()
```

### REST

Every method takes a `context.Context` first. They cover:

- messages, reactions, pins, polls and crossposting;
- channels, permission overwrites, computed permissions, voice channel statuses, invites, typing and followed announcement channels;
- threads and forum posts, their members and the archived ones;
- guilds, members (nicknames, roles, timeouts, moving between voice channels), roles, bans, pruning, the audit log, integrations, the widget, the welcome screen, onboarding and templates;
- webhooks and the messages they send;
- emojis of guilds and of the app, stickers and soundboard sounds;
- auto moderation rules, scheduled events and stage instances;
- the bot's user and application, application commands, interaction responses, linked role metadata, SKUs, entitlements and subscriptions.

Fields that Discord clears with `null` are a `discord.Optional`: leave it unset to keep the value, or use `discord.Some(v)` and `discord.Null[T]()`.

```go
s.GuildMemberTimeout(ctx, guildID, userID, time.Now().Add(time.Hour))
s.ChannelEdit(ctx, channelID, &discord.ChannelParams{Name: "general", ParentID: discord.Null[string]()})
s.GuildBanCreate(client.WithAuditLogReason(ctx, "spam"), guildID, userID, 0)
```

`Session.Request` reaches any endpoint without a typed method.

### Gateway

There is an `On…` handler for every gateway event, such as `OnMessageCreate`, `OnGuildMemberAdd`, `OnChannelUpdate`, `OnThreadCreate`, `OnVoiceStateUpdate` and `OnPresenceUpdate`, and `OnRawEvent` receives them all undecoded. Each handler runs in its own goroutine. Some events need an intent in `Session.Intents`, and the member, presence and message content ones are privileged: enable them in the developer portal.

`RequestGuildMembers` asks for the members of a guild, answered in `OnGuildMembersChunk`. Discord limits how often all members can be requested, and reports a refused request in `OnRateLimited`. `UpdateVoiceState` joins and leaves voice channels, but sending and receiving audio is not part of this library: `OnVoiceStateUpdate` and `OnVoiceServerUpdate` give a voice library what it needs to connect.

### Helpers

`discord` has the small things a bot keeps needing: `User.Mention`, `User.AvatarURL`, `Member.DisplayName`, `Emoji.MessageFormat`, `Message.URL`, `discord.Timestamp`, `discord.SnowflakeTime`, `discord.ImageData` for uploading avatars and emojis, and `discord.MemberPermissions` to compute what a member may do in a channel.

##

**&copy; 2022 | tbdsux**
