package main

import (
	"fmt"
	"log"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/tbdsux/minidis"
)

func main() {
	bot := minidis.New(os.Getenv("TOKEN"))

	// set intents
	bot.SetIntents(discordgo.IntentsGuilds | discordgo.IntentsGuildMessages)

	//
	bot.SyncToGuilds(os.Getenv("GUILD"))

	bot.OnReady(func(s *discordgo.Session, i *discordgo.Ready) {
		log.Println("Bot is ready!")
	})

	// message command
	bot.AddMessageCommand(&minidis.MessageCommandProps{
		Name: "re-reply",
		Execute: func(c *minidis.MessageCommandContext) error {
			return c.ReplyC(minidis.ReplyProps{
				Content:     c.Message.Content,
				IsEphemeral: true,
			})
		},
	})

	// user command
	bot.AddUserCommand(&minidis.UserCommandProps{
		Command: "get-user",
		Execute: func(c *minidis.UserCommandContext) error {

			fmt.Println(c.Member)
			fmt.Println(c.User)

			return c.ReplyC(minidis.ReplyProps{
				Content: "hello",
			})
		},
	})

	bot.OnBeforeStart(func(s *discordgo.Session) {
		// try to remove old commands first
		if err := bot.ClearCommands(); err != nil {
			log.Fatal(err)
		}
	})

	bot.OnClose(func(s *discordgo.Session) {
		log.Println("Closing...")
	})

	if err := bot.Run(); err != nil {
		log.Fatalln(err)
	}
}
