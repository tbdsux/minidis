# minidis CLI

Creates a Discord bot project that uses [minidis](../../README.md).

## Install

```sh
go install github.com/tbdsux/minidis/cli/minidis@latest
```

## Create a bot

```sh
minidis init mybot
cd mybot
TOKEN=<bot token> GUILD=<test server ID> go run .
```

`init` creates the directory and a `go.mod` if needed, writes the project, and adds minidis at the same version as the CLI, so the generated code matches its API. It never overwrites existing files unless given `-force`, and checks for them before writing anything.

| Flag | |
|---|---|
| `-module path` | Module path of a new `go.mod`. Defaults to the directory name. An existing `go.mod` is kept. |
| `-http` | Receive interactions over HTTP, at an Interactions Endpoint URL, instead of the gateway. |
| `-force` | Overwrite files that already exist. |
| `-skip-deps` | Do not run `go get` and `go mod tidy`. |

Flags may come before or after the directory.

## What you get

```
mybot/
├── main.go               reads TOKEN and GUILD, registers the commands, syncs them and runs the bot
├── commands/
│   ├── commands.go       Register, where commands and component handlers are added
│   └── hello.go          a /hello command with a button
├── README.md
└── .gitignore
```

With `-http`, `main.go` also serves the interactions endpoint with an `http.Server` and reads `PUBLIC_KEY` and `ADDR`.
