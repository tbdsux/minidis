# minidis

Simple Discord bot framework for Go, built directly on the [Discord API](https://docs.discord.com/developers/intro).

The `discord/client` package is its REST and Gateway client and `discord` holds the API types. Use `Session.Request` for endpoints without a typed method and `Session.OnRawEvent` for gateway events without a typed handler.

## Install

This is usable for simple and basic commands but is still missing some of other features.

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

	"github.com/tbdsux/minidis"
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

func main() {
	guilds := strings.Split(os.Getenv("GUILD"), ",")
	bot := minidis.New(os.Getenv("TOKEN"))

	// Set intents (not required when doing only slash commands)
	bot.SetIntents(discord.IntentsGuilds | discord.IntentsGuildMessages)

	bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	// simple command
	bot.AddCommand(&minidis.Command{
		Name:        "ping",
		Description: "Simple ping command.",
		Execute: func(c *minidis.Context) error {
			return c.Reply(fmt.Sprintf("Hello **%s**, pong?", c.Author.Username))
		},
	})

	// Register the commands. With no guilds they are registered globally.
	if err := bot.SyncCommands(guilds...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
	}

	// Run until CTRL+C, then close, giving running handlers time to finish
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bot.Run(ctx); err != nil {
		log.Fatalln("Bot stopped with an error:", err)
	}
}
```

Use `bot.Open` and `bot.Close` instead of `Run` to manage the connection yourself.

More [examples...](./example/)

## Features

### Permissions

A command is usable by everyone unless `DefaultMemberPermissions` is set.

```go
DefaultMemberPermissions: discord.Int64(0),     // admins only
DefaultMemberPermissions: discord.Int64(1 << 5), // members who can manage the server
```

Global commands are usable in guilds only, unless `Contexts` says otherwise. For user-installable commands, also set `IntegrationTypes`, and sync the command globally.

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

In a `MessageEdit`, a nil field is left unchanged and an empty one clears it.

Every handler context is also a `context.Context`. It is canceled when the handler returns, when the bot closes, and 15 minutes after the interaction arrived, when Discord stops accepting responses. Pass it to the calls the handler makes:

```go
rows, err := db.QueryContext(c, "SELECT ...")
```

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
		// runs around every command, component and modal handler
		return next(c)
	}
})

bot.OnError(func(c *minidis.Context, err error) {
	// err is a *minidis.PanicError if a handler panicked, and wraps
	// minidis.ErrNoHandler for a command that is no longer registered
})
```

Without `OnError`, errors are logged. Channel message handlers have `OnChannelMessageError`.

### Components and modals

Handler IDs can capture `:`-separated segments, and modal IDs work the same way.

```go
bot.HandleComponent("vote:{poll}", func(c *minidis.ComponentContext) error {
	return c.Update("Voted in " + c.Params["poll"]) // edits the message with the button
})

bot.HandleModal("feedback", func(c *minidis.ModalSubmitContext) error {
	return c.Reply("Thanks!")
})
```

Use `c.UpdateWith` for embeds and components, passing an empty non-nil slice to clear them, and `c.DeferUpdate` for slow work. `HandleComponentFallback` and `HandleModalFallback` receive the IDs that no handler matches.

A submitted modal is read by the custom IDs of its components:

```go
c.Value("name")       // a text input, or the choice of a discord.RadioGroup
c.Values("toppings")  // a select menu, a discord.CheckboxGroup, or the IDs of a discord.FileUpload
c.Checked("agree")    // a discord.Checkbox
c.Attachments("files") // the files of a discord.FileUpload
```

### Channel messages

```go
bot.OnChannelMessageCreate(func(c *minidis.ChannelMessageContext) error {
	_, err := c.Reply("hello") // or c.Send, which does not reply to the message
	return err
}, channelIDs...)
```

### Presence

```go
bot.SetPresence(discord.StatusIdle, &discord.Activity{Name: "the logs", Type: discord.ActivityWatching})
```

### Client

`bot.Session` has typed methods for messages, pins, reactions, channels, guilds, members, roles, bans, application commands and their permissions, and interaction responses, and handlers such as `OnMessageUpdate`, `OnMessageDelete`, `OnMessageReactionAdd`, `OnGuildCreate`, `OnGuildMemberAdd` and `OnGuildMemberRemove`. The member events need the privileged `discord.IntentsGuildMembers` intent.

##

**&copy; 2022 | tbdsux**
