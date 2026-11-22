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

In PowerShell, set the variables with `$env:TOKEN="<bot token>"; $env:GUILD="<test server ID>"; go run .`.

`init` works out where the project goes, then:

1. creates the directory and, unless it is already inside a module, a `go.mod` declaring the same Go version as minidis;
2. adds minidis at the version the CLI was installed from, so the generated code matches its API. A module that already requires an older minidis is upgraded to it, never downgraded, and one that replaces minidis is left alone;
3. writes the project, then runs `go mod tidy` (and `go mod vendor` in a vendored module) and builds it to check that it works.

The `go get` runs before any file is written, so if it fails, for example offline, fix the problem and run the same `init` command again, as the error shows.

Inside an existing module, a directory without `-module` becomes a package of that module, for example `minidis init cmd/bot` gives `example.com/yours/cmd/bot`. With `-module` it becomes a module of its own. A module inside a workspace that does not use it, new or existing, is set up and checked on its own, and `init` shows the `go work use` command that adds it to the workspace.

Existing Go files are never overwritten unless given `-force`, and neither is a directory that already holds a package other than `main`, or a `commands` or `events` directory with a package of another name. `README.md` and `.gitignore` are never overwritten: an existing README is kept, and `.gitignore` only gets the lines it is missing. Everything, including that the module path is one the go command accepts and does not clash with the standard library, is checked before anything is written.

| Flag | |
|---|---|
| `-module path` | Module path of a new `go.mod`. Defaults to the directory name, which must then be a valid module path. An existing `go.mod` is kept. |
| `-http` | Receive interactions over HTTP, at an Interactions Endpoint URL, instead of the gateway. Such a bot gets no gateway events, so the project has no `events` and no text command. |
| `-force` | Overwrite Go files that already exist. |
| `-skip-deps` | Do not run `go get`, `go mod tidy` and the build check. |

Flags may come before or after the directory.

A CLI built from a checkout, rather than installed with `go install …@version`, adds the latest release of minidis, or its `main` branch while there is none, which may not match the checkout's templates. The build check then explains how to point the project at the checkout instead.

## What you get

A bot on the minidis framework, with one of each kind of handler to copy from:

```
mybot/
├── main.go               reads TOKEN and GUILD, adds the handlers and runs the bot
├── commands/
│   ├── commands.go       Register, where commands and component handlers are added, and the error handler
│   ├── hello.go          a /hello slash command with a button
│   └── ping.go           a text command, run by writing "@mybot ping"
├── events/
│   ├── events.go         Register, where event handlers are added, and one for when the bot is ready
│   └── threads.go        a handler of a gateway event: the bot joins new threads
├── README.md
└── .gitignore            ignores the binary, named after the last element of the module path
```

The bot runs as generated, with no privileged intent to enable: it asks Discord only for the intents its handlers need, which the comments in `events/events.go` explain.

With `-http`, `main.go` also reads `PUBLIC_KEY` and `ADDR` and serves the interactions endpoint with an `http.Server`. It checks the key, the token and the address before it syncs the commands, so a mistake changes nothing on Discord. There is no `events` directory and no `commands/ping.go`, as those need the gateway.
