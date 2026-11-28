# minidis documentation

Guides for building Discord bots with minidis. The API reference is on [pkg.go.dev](https://pkg.go.dev/github.com/tbdsux/minidis), and runnable programs are in [`example/`](../example/).

## Guides

1. [Getting started](./getting-started.md): install, create a bot, run it, intents and shutting down.
2. [Slash commands](./commands.md): options, subcommands, permissions, context menus, localization and registering commands.
3. [Responding to interactions](./responding.md): replies, deferring, edits, followups, message layout and the handler context.
4. [Components and modals](./components-and-modals.md): buttons, select menus, handler IDs and forms.
5. [Autocomplete](./autocomplete.md): suggesting option values as the user types.
6. [Messages and events](./messages-and-events.md): gateway events, messages, text commands and presence.
7. [Middleware and errors](./middleware-and-errors.md): wrapping handlers and dealing with what goes wrong.
8. [Receiving interactions over HTTP](./http.md): an Interactions Endpoint URL instead of the gateway.
9. [The Discord client](./client.md): REST, the gateway, sharding and errors, with or without the framework.
10. [State cache](./state.md): keeping guilds, channels, members and messages in memory.
11. [Testing handlers](./testing.md): running handlers in unit tests, without connecting to Discord.

## Packages

minidis is made of layers, and each works without the ones above it:

| Package                  | What it is                                                                                                                                                         |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `minidis`                | The framework: one `Bot` for commands, components, messages and every gateway event.                                                                               |
| `minidis/interactions`   | The framework for interactions alone: slash commands, context menus, components, modals and autocomplete. Also for bots that only receive interactions over HTTP.  |
| `minidis/discord/client` | The REST and Gateway client.                                                                                                                                       |
| `minidis/discord`        | The Discord API types.                                                                                                                                             |
| `minidis/minidistest`    | Fakes Discord for unit tests of handlers.                                                                                                                          |

The `minidis.Bot` embeds the bot of `interactions`, and has its types under the same names (`minidis.Command` is `interactions.Command`), so everything in these guides about interactions works with either. The examples use `minidis`; a bot that only handles interactions can swap in `interactions`, as described in [Getting started](./getting-started.md#interactions-only-bots).

## Examples

| Example                                               | Shows                                                         |
| ----------------------------------------------------- | ------------------------------------------------------------- |
| [`bot`](../example/bot/main.go)                       | The `minidis` framework: commands, events, messages and text commands |
| [`simple`](../example/simple/main.go)                 | The smallest `interactions` bot                               |
| [`options`](../example/options/main.go)               | A command with options                                        |
| [`subcommands`](../example/subcommands/main.go)       | Subcommands and groups                                        |
| [`autocomplete`](../example/autocomplete/main.go)     | Autocomplete                                                  |
| [`components`](../example/components/main.go)         | Buttons and select menus                                      |
| [`paginator`](../example/paginator/main.go)           | Pages of embeds turned with buttons                           |
| [`modals`](../example/modals/main.go)                 | A modal with text inputs, radio groups and checkboxes         |
| [`context_menus`](../example/context_menus/main.go)   | Message and user commands                                     |
| [`channel_messages`](../example/channel_messages/main.go) | Messages of one channel                                   |
| [`http`](../example/http/main.go)                     | Interactions over HTTP                                        |
| [`client`](../example/client/main.go)                 | The client on its own                                         |

Run one with `TOKEN=<bot token> GUILD=<test server ID> go run ./example/bot`.
