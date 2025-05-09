package templ

var (
	// primary main.go file
	MainGoTemplate = `
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"{{ .PkgName }}/commands"
	"{{ .PkgName }}/lib"
)

func main() {
	// Open session
	if err := commands.Bot.OpenSession(); err != nil {
		log.Fatalln("Failed to open session:", err)
		return
	}

	// Re-sync commands
	if err := commands.Bot.ClearCommands(guilds...); err != nil {
		log.Fatalln("Failed to clear commands:", err)
		return
	}
	if err := commands.Bot.SyncCommands(guilds...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
		return
	}

	// Run the bot
	commands.Bot.Run()

	// Wait for CTRL+C to exit
	fmt.Println("Bot is running. Press CTRL+C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Close the session
	commands.Bot.CloseSession()
}

`

	// commands/root.go - base file for adding new commands and other stuff
	RootGoTemplate = `
package commands

import (
	"log"

	"github.com/tbdsux/minidis"
	"github.com/bwmarrin/discordgo"

	"{{ .PkgName }}/lib"
)

var Bot *minidis.Minidis

func init() {
	Bot = minidis.New(lib.TOKEN)

	Bot.OnReady(func(s *discordgo.Session, i *discordgo.Ready) {
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

var helloCommand = &minidis.SlashCommandProps{
	Name:        "hello",
	Description: "Say hi to the bot",
	Execute: func(c *minidis.SlashContext) error {
		return c.ReplyC(minidis.ReplyProps{
			Content: "Hi!",
		})
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
