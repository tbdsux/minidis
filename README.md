# minidis

Simple Discord library for Go, built directly on the [Discord API](https://docs.discord.com/developers/intro).

| Package | What it holds |
| --- | --- |
| `discord` | The Discord API types. |
| `discord/client` | The REST and Gateway client. |
| `interactions` | The bot framework for slash commands, context menus, components, modals and autocomplete. |

The usage below is that of `interactions`. Use `Session.Request` for endpoints without a typed method and `Session.OnRawEvent` for gateway events without a typed handler.

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
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
	"github.com/tbdsux/minidis/interactions"
)

func main() {
	guilds := strings.Fields(strings.ReplaceAll(os.Getenv("GUILD"), ",", " ")) // none means global
	bot := interactions.New(os.Getenv("TOKEN"))

	// Set intents (not required when doing only slash commands)
	bot.SetIntents(discord.IntentsGuilds | discord.IntentsGuildMessages)

	bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	// simple command
	bot.AddCommand(&interactions.Command{
		Name:        "ping",
		Description: "Simple ping command.",
		Execute: func(c *interactions.Context) error {
			return c.Reply(fmt.Sprintf("Hello **%s**, pong?", c.Author.Username))
		},
	})

	// Stop on CTRL+C
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Register the commands. With no guilds (GUILD unset) they are registered
	// globally; SyncCommands refuses a list of only empty IDs, so an unset
	// variable never replaces the global commands by mistake.
	if err := bot.SyncCommands(ctx, guilds...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
	}

	// Run until CTRL+C, then close: new interactions stop, and running handlers
	// get up to 10 seconds to finish before their contexts are canceled
	if err := bot.Run(ctx); err != nil {
		log.Fatalln("Bot stopped with an error:", err)
	}
}
```

Use `bot.Open(ctx)` and `bot.Close` instead of `Run` to manage the connection yourself. `Run` also returns if Discord ends the connection for good, such as for an invalid token. A handler that closes the bot should call `Close` in a new goroutine, as `Close` waits for running handlers.

More [examples...](./example/)

## Features

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
bot.AddCommand(&interactions.Command{
	Name:        "role",
	Description: "Manage roles.",
	Subcommands: []*interactions.Subcommand{
		{Name: "list", Description: "List roles.", Execute: listRoles},
	},
	Groups: []*interactions.SubcommandGroup{
		{Name: "member", Description: "Roles of a member.", Subcommands: []*interactions.Subcommand{
			{Name: "add", Description: "Give a role.", Execute: addRole},
		}},
	},
})
```

A command with subcommands only runs them, so it cannot have `Options`, an `Execute` or an `Autocomplete` of its own. `SyncCommands` reports such a command, and also an option with `Autocomplete: true` that no handler answers.

### Responding

```go
c.Reply("pong")
c.ReplyWith(interactions.Message{Content: "only you can see this", Ephemeral: true})

c.DeferReply(false) // for work that takes more than 3 seconds, then:
c.Edit("done")
c.EditWith(interactions.MessageEdit{Content: discord.String("done"), Components: []discord.MessageComponent{}})

f, err := c.Followup("one more thing")
c.ReplyModal(interactions.Modal{CustomID: "feedback", Title: "Feedback", Components: ...})
```

In a `MessageEdit`, a nil field is left unchanged and an empty one clears it. A slice built with `append` that ends up empty is still nil, so start from an empty slice when the result should clear. `Flags` replaces the message's flags, so include those to keep.

`Message` also takes a `Poll`, `TTS` and further `Flags`. Use `discord.MessageFlagsIsComponentsV2` to lay a message out with components such as `discord.Container`, `discord.Section` and `discord.TextDisplay`. A deferred reply becomes one through `c.EditWith(interactions.MessageEdit{Flags: discord.MessageFlagsIsComponentsV2, Components: ...})`. `c.Response()` fetches the message that was sent, and `c.SendResponse` sends a response built by hand and returns what Discord reports about it.

Every handler context is also a `context.Context` for the handler's own work. It is canceled when the handler returns, 15 minutes after the interaction arrived (when Discord stops accepting responses), after 3 seconds if nothing responded by then (with `interactions.ErrNoResponse` as its cause), and when a closing bot stops waiting for its handlers. Pass it to the calls the handler makes, including those on `c.Session`:

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
Autocomplete: func(c *interactions.AutocompleteContext) error {
	return c.Respond(interactions.Choice("Go", "go"))  // c.Input() is what the user typed
},
```

### Middleware and errors

```go
bot.Use(func(next interactions.HandlerFunc) interactions.HandlerFunc {
	return func(c *interactions.Context) error {
		// runs around every command, component and modal handler;
		// c.WithContext passes values on to the handler
		return next(c.WithContext(context.WithValue(c, requestIDKey, newRequestID())))
	}
})

bot.HandleError(func(c *interactions.Context, err error) {
	// err is a *interactions.PanicError if a handler panicked (errors.Is and
	// errors.As see through it), and wraps interactions.ErrNoHandler for a
	// command or component that is no longer registered. c is the copy that
	// middleware passed on, with its values.
	if c.Interaction.Type == discord.InteractionApplicationCommandAutocomplete {
		return // autocomplete cannot show a message
	}
	if c.Responded() {
		c.FollowupWith(interactions.Message{Content: "Something went wrong.", Ephemeral: true})
	} else {
		c.ReplyWith(interactions.Message{Content: "Something went wrong.", Ephemeral: true})
	}
})
```

Without `HandleError`, errors are logged. A panic in the error handler is logged too. Channel message handlers have `HandleChannelMessageError`.

### Components and modals

Handler IDs can capture `:`-separated segments, and modal IDs work the same way. An exact ID wins over a pattern, and a pattern with more fixed segments over one with fewer, so `ticket:close:{id}` wins over `ticket:{action}:{id}`.

```go
bot.HandleComponent("vote:{poll}", func(c *interactions.ComponentContext) error {
	return c.Update("Voted in " + c.Params["poll"]) // edits the message with the button
})

bot.HandleModal("feedback", func(c *interactions.ModalSubmitContext) error {
	return c.Reply("Thanks!")
})
```

Use `c.UpdateWith` for embeds and components, passing an empty non-nil slice to clear them, and `c.DeferUpdate` for slow work. A modal opened from a component can update that component's message the same way. A capture is one segment, so a value put there must not contain `:`.

`HandleComponentFallback` and `HandleModalFallback` receive the IDs that no handler matches. Without them, such interactions reach the error handler as `interactions.ErrNoHandler`.

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

```go
bot.HandleChannelMessage(channelID, func(c *interactions.ChannelMessageContext) error {
	_, err := c.Reply("hello") // or c.Send, which does not reply to the message
	return err
})
```

`SendWith` and `ReplyWith` take a `interactions.Message`. When the content repeats what users wrote, set `AllowedMentions` so that it cannot ping `@everyone`.

### Presence

```go
bot.SetPresence(discord.StatusIdle, &discord.Activity{Name: "the logs", Type: discord.ActivityWatching})
```

## Client

`discord/client` is the whole Discord API for bots, and works on its own, without `interactions` ([example](./example/client/main.go)). A bot made with `interactions.New` has the same client as `bot.Session`.

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
