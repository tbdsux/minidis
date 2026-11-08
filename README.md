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
	bot.AddCommand(&minidis.SlashCommandProps{
		Name:        "ping",
		Description: "Simple ping command.",
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyString(fmt.Sprintf("Hello **%s**, pong?", c.Author.Username))
		},
	})

	// Open session
	if err := bot.OpenSession(); err != nil {
		log.Fatalln("Failed to open session:", err)
		return
	}

	// Register the commands. With no guilds they are registered globally.
	if err := bot.SyncCommands(guilds...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
		return
	}

	// Wait for CTRL+C to exit
	fmt.Println("Bot is running. Press CTRL+C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Close the session, giving running handlers time to finish
	bot.CloseSession()
}

```

More [examples...](./example/)

## Features

### Permissions

A command is usable by everyone unless `DefaultMemberPermissions` is set.

```go
DefaultMemberPermissions: discord.Int64(0),     // admins only
DefaultMemberPermissions: discord.Int64(1 << 5), // members who can manage the server
```

For user-installable commands, set `IntegrationTypes` and `Contexts` on a command and sync it globally.

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
	return func(c *minidis.SlashContext) error {
		// runs around every command, component and modal handler
		return next(c)
	}
})

bot.OnError(func(c *minidis.SlashContext, err error) {
	// err is a *minidis.PanicError if a handler panicked, and wraps
	// minidis.ErrNoHandler for a command that is no longer registered
})
```

Without `OnError`, errors are logged. Channel message handlers have `OnChannelMessageError`.

### Components

Handler IDs can capture `:`-separated segments, and modal IDs work the same way.

```go
bot.AddComponentHandler(&minidis.ComponentInteractionProps{
	ID: "vote:{poll}",
	Execute: func(s *minidis.SlashContext, c *minidis.ComponentContext) error {
		return c.Update("Voted in " + c.Params["poll"]) // edits the message with the button
	},
})
```

Use `c.UpdateC` for embeds and components, passing an empty non-nil slice to clear them, and `c.DeferUpdate` for slow work.

### Presence

```go
bot.SetPresence(discord.StatusIdle, &discord.Activity{Name: "the logs", Type: discord.ActivityWatching})
```

### Client

`bot.Session` has typed methods for messages, pins, reactions, channels, guilds, members, roles and bans, and handlers such as `OnMessageUpdate`, `OnMessageDelete`, `OnMessageReactionAdd`, `OnGuildCreate`, `OnGuildMemberAdd` and `OnGuildMemberRemove`. The member events need the privileged `discord.IntentsGuildMembers` intent.

##

**&copy; 2022 | tbdsux**
