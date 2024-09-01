package templ

var (
	// primary main.go file
	MainGoTemplate = `
package main

import (
	"log"

	"github.com/tbdsux/minidis"

	"{{ .PkgName }}/commands"
)

func main() {
	if err := minidis.Execute(commands.Bot); err != nil {
		log.Fatal(err)
	}
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

	// sync to server
	Bot.SyncToGuilds(lib.GUILD...)

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
	GUILD = strings.Split(os.Getenv("GUILD"), ",")
	TOKEN = os.Getenv("TOKEN")
)
`
)
