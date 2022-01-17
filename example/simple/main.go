package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/World-of-Cryptopups/minidis"
	"github.com/bwmarrin/discordgo"
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

	// simple command
	bot.AddCommand(&minidis.SlashCommandProps{
		Command:     "ping",
		Description: "Simple ping command.",
		Options:     []*discordgo.ApplicationCommandOption{},
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyString(fmt.Sprintf("Hello **%s**, pong?", c.Author.Username))

		},
	})

	bot.AddCommand(&minidis.SlashCommandProps{
		Command:     "response",
		Description: "Responses management.",
		Options:     []*discordgo.ApplicationCommandOption{},
		Execute: func(s *minidis.SlashContext) error {
			if err := s.ReplyString("Hello! this message will be modified after 5 seconds..."); err != nil {
				_, err = s.Followup("A problem has occured")
				return err
			}

			time.Sleep(time.Second * 5)

			return s.Edit("new message here!")
		},
	})

	if err := bot.Run(); err != nil {
		log.Fatalln(err)
	}
}
