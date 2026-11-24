# Getting started

## Install

```sh
go get -u github.com/tbdsux/minidis
```

minidis needs Go 1.25 or later.

### Or start from a generated project

The [minidis CLI](../cli/minidis/README.md) creates a bot project with one of each kind of handler to copy from:

```sh
go install github.com/tbdsux/minidis/cli/minidis@latest
minidis init mybot
```

## Create the bot in Discord

1. In the [Developer Portal](https://discord.com/developers/applications), create an application.
2. Under **Bot**, reset and copy the token. This is the `TOKEN` below; keep it secret.
3. Under **OAuth2 > URL Generator**, tick the `bot` and `applications.commands` scopes and the permissions the bot needs, then open the generated URL to add the bot to a test server.
4. With Developer Mode on in Discord (**Settings > Advanced**), right-click the test server and copy its ID. This is the `GUILD` below.

## A first bot

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

	// Commands show up at once in a guild, so name one while developing, or
	// several separated by commas. With GUILD unset, they are registered globally.
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

	// Register the commands, connect, and run until CTRL+C
	if err := bot.Start(); err != nil {
		log.Fatalln("Bot stopped with an error:", err)
	}
}
```

```sh
TOKEN=<bot token> GUILD=<test server ID> go run .
```

The `GuildMemberAdd` handler and `OnMessage` need privileged intents, which must be enabled for the bot first; see [Intents](#intents).

## Running and stopping

`bot.Start()` runs the bot until the program gets CTRL+C or SIGTERM. Then new events stop, and running handlers get up to 10 seconds to finish before their contexts are canceled. It returns `nil` when the bot stopped that way. A second CTRL+C while it closes ends the program at once.

`Start` is `bot.Run(ctx)` with a context that ends on those signals. Call `Run` yourself to decide when the bot stops:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

if err := bot.Run(ctx); err != nil {
	log.Fatal(err)
}
```

`Run` also returns if Discord ends the connection for good, such as for an invalid token or intents the bot is not allowed. Other disconnects are reconnected, and resumed when Discord allows it.

To manage the connection yourself:

| Method               | Does                                                                                                     |
| -------------------- | -------------------------------------------------------------------------------------------------------- |
| `bot.Open(ctx)`      | Connects and returns once the bot is ready. It does not register commands.                               |
| `bot.Close()`        | Disconnects, and gives running handlers up to 10 seconds.                                                |
| `bot.Shutdown(ctx)`  | Disconnects, and waits for running handlers until `ctx` ends, then cancels their contexts.               |

A handler that stops the bot should call `Close` in a new goroutine, as `Close` waits for running handlers, the caller included.

## Registering commands

`Run` and `Start` register the bot's commands with Discord before connecting, replacing the ones Discord has: in the guilds given to `SetGuilds`, or globally without any. Guild commands show up at once; global ones can take a while, so use a test guild while developing.

A bot with no commands leaves Discord's alone. Call `bot.DisableSync()` when commands are registered some other way, such as by a deploy step. See [Registering commands](./commands.md#registering-commands) for `SyncCommands` and `ClearCommands`.

## Intents

Intents say which gateway events Discord sends. With `minidis.Bot` there are none to set: the bot asks for the ones its handlers need. Slash commands and other interactions need none at all.

Some intents are privileged and must also be enabled for the bot in the Developer Portal, under **Bot > Privileged Gateway Intents**:

| Intent           | Needed by                                                                                                      |
| ---------------- | -------------------------------------------------------------------------------------------------------------- |
| Server Members   | Member events such as `discord.GuildMemberAdd`, and a [state](./state.md) that keeps all members                |
| Presence         | `discord.PresenceUpdateEvent`, and a state that keeps presences                                                |
| Message Content  | `bot.OnMessage`, a prefix set with `bot.SetPrefix`, and any handler that reads what a message says              |

`Run` says which privileged intent Discord refused. Bots in 100 or more servers need Discord's approval for each.

Adjusting the intents:

- `bot.AddIntents(discord.IntentsMessageContent)` asks for more, such as for a `minidis.On` handler of `discord.MessageCreate` that reads the content, or for handlers added on `bot.Session` directly, which the bot does not see.
- `bot.SetIntents(...)` takes over, so the bot stops working them out. It is rarely needed.
- `bot.Intents()` reports what the bot will connect with.

Add handlers before the bot runs. Intents are sent when it connects, so a handler added later that needs a new one gets no events, which the bot logs. Commands, components and modals can still be added while it runs.

## Interactions-only bots

A bot that only handles interactions can use the `interactions` package without the framework. Its bot does not register commands by itself, and does not work out intents:

```go
bot := interactions.New(os.Getenv("TOKEN"))
bot.AddCommand(&interactions.Command{
	Name:        "ping",
	Description: "Replies with pong.",
	Execute: func(c *interactions.Context) error {
		return c.Reply("pong")
	},
})

ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

// No guild IDs registers the commands globally. An empty ID is an error
// instead, so that an unset variable cannot replace the global commands.
guilds := strings.Fields(strings.ReplaceAll(os.Getenv("GUILD"), ",", " "))
if err := bot.SyncCommands(ctx, guilds...); err != nil {
	log.Fatal(err)
}
if err := bot.Run(ctx); err != nil {
	log.Fatal(err)
}
```

`bot.OnReady` runs each time the bot starts a gateway session, once for each [shard](./client.md#sharding), and `bot.SetIntents` subscribes to gateway events for handlers added on `bot.Session`. Such a bot can also [receive interactions over HTTP](./http.md) without connecting to the gateway at all.

Next: [Slash commands](./commands.md).
