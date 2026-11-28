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

if err := bot.Start(); err != nil {
	log.Fatal(err)
}
```

- Slash commands, context menus, buttons, select menus, modals and autocomplete, with responses that keep working for Discord's full 15 minutes.
- Every gateway event, messages and text commands, with no intents to set: the bot asks for the ones its handlers need.
- Handlers that return errors and get a `context.Context`, with middleware and error handlers.
- Interactions over the gateway or over HTTP.
- A complete REST and gateway client with rate limiting, sharding and an optional state cache, usable on its own.

## Install

```sh
go get -u github.com/tbdsux/minidis
```

Or generate a project with the [CLI](./cli/minidis/README.md):

```sh
go install github.com/tbdsux/minidis/cli/minidis@latest
minidis init mybot
```

## Documentation

- [Getting started](./docs/getting-started.md)
- [Slash commands](./docs/commands.md)
- [Responding to interactions](./docs/responding.md)
- [Components and modals](./docs/components-and-modals.md)
- [Autocomplete](./docs/autocomplete.md)
- [Messages and events](./docs/messages-and-events.md)
- [Middleware and errors](./docs/middleware-and-errors.md)
- [Receiving interactions over HTTP](./docs/http.md)
- [The Discord client](./docs/client.md)
- [State cache](./docs/state.md)
- [Voice and music](./docs/voice.md)

The API reference is on [pkg.go.dev](https://pkg.go.dev/github.com/tbdsux/minidis), and runnable programs are in [`example/`](./example/).

## Packages

Each layer works without the ones above it:

| Package                  | What it is                                                                                                                                                         |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `minidis`                | The framework: one `Bot` for commands, components, messages and every gateway event.                                                                               |
| `minidis/interactions`   | The framework for interactions alone: slash commands, context menus, components, modals and autocomplete. Also for bots that only receive interactions over HTTP.  |
| `minidis/discord/client` | The REST and Gateway client.                                                                                                                                       |
| `minidis/discord`        | The Discord API types.                                                                                                                                             |
| `minidis/lavalink`       | A client for audio nodes (the minidis `node` module, or Lavalink) that play music in voice channels. See [Voice and music](./docs/voice.md).                       |

##

**&copy; 2022 | tbdsux**
