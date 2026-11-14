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
2. adds minidis at the version the CLI was installed from, so the generated code matches its API. A module that already requires minidis keeps its version;
3. writes the project, then runs `go mod tidy` and builds it to check that it works.

The `go get` runs before any file is written, so if it fails, for example offline, run `init` again or finish with the commands it prints.

Inside an existing module, a directory without `-module` becomes a package of that module, for example `minidis init cmd/bot` gives `example.com/yours/cmd/bot`. With `-module` it becomes a module of its own; in a workspace, `init` reminds you to `go work use` it.

Existing Go files are never overwritten unless given `-force`. `README.md` and `.gitignore` are never overwritten: an existing README is kept, and `.gitignore` only gets the lines it is missing. Everything is checked before anything is written.

| Flag | |
|---|---|
| `-module path` | Module path of a new `go.mod`. Defaults to the directory name, which must then be a valid module path. An existing `go.mod` is kept. |
| `-http` | Receive interactions over HTTP, at an Interactions Endpoint URL, instead of the gateway. |
| `-force` | Overwrite Go files that already exist. |
| `-skip-deps` | Do not run `go get`, `go mod tidy` and the build check. |

Flags may come before or after the directory.

A CLI built from a checkout, rather than installed from a version, adds the latest release of minidis, which may not match its templates. The build check then explains how to point the project at the checkout instead.

## What you get

```
mybot/
├── main.go               reads TOKEN and GUILD, registers the commands, syncs them and runs the bot
├── commands/
│   ├── commands.go       Register, where commands and component handlers are added
│   └── hello.go          a /hello command with a button
├── README.md
└── .gitignore            ignores the binary, named after the last element of the module path
```

With `-http`, `main.go` also serves the interactions endpoint with an `http.Server` and reads `PUBLIC_KEY` and `ADDR`.
