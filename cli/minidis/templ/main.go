package templ

var (
	// primary main.go file
	MainGoTemplate = `
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"{{ .PkgName }}/commands"
	"{{ .PkgName }}/lib"
)

func main() {
	// Register the commands. This replaces any registered before.
	if err := commands.Bot.SyncCommands(lib.GUILDS...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
	}

	// Run until CTRL+C
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("Bot is running. Press CTRL+C to exit.")
	if err := commands.Bot.Run(ctx); err != nil {
		log.Fatalln("Bot stopped with an error:", err)
	}
}

`

	// commands/root.go - base file for adding new commands and other stuff
	RootGoTemplate = `
package commands

import (
	"log"

	"github.com/tbdsux/minidis"
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"

	"{{ .PkgName }}/lib"
)

var Bot *minidis.Minidis

func init() {
	Bot = minidis.New(lib.TOKEN)

	Bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	Bot.AddCommand(helloCommand)
}
`

	// commands/hello.go - initial command starter
	HelloCmdGoTemplate = `
package commands

import (
	"github.com/tbdsux/minidis"
)

var helloCommand = &minidis.Command{
	Name:        "hello",
	Description: "Say hi to the bot",
	Execute: func(c *minidis.Context) error {
		return c.Reply("Hi!")
	},
}
`

	// lib/env.go - environment variables
	LibEnvGoTemplate = `
package lib

import (
	"os"
	"strings"
)

var (
	GUILDS = strings.Split(os.Getenv("GUILD"), ",")
	TOKEN = os.Getenv("TOKEN")
)
`
)
