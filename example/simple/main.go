package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/TheBoringDude/minidis"
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
		Name:        "ping",
		Description: "Simple ping command.",
		Options:     []*discordgo.ApplicationCommandOption{},
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyString(fmt.Sprintf("Hello **%s**, pong? Guild: %s", c.Author.Username, c.GuildId))

		},
	})

	// deferred replies
	bot.AddCommand(&minidis.SlashCommandProps{
		Name:        "defer",
		Description: "Deferred reply.",
		Execute: func(c *minidis.SlashContext) error {
			c.DeferReply(true)

			time.Sleep(time.Second * 5)

			return c.Edit("This is a deffered reply edit message!")
		},
	})

	// responses
	bot.AddCommand(&minidis.SlashCommandProps{
		Name:        "response",
		Description: "Responses management.",
		Options:     []*discordgo.ApplicationCommandOption{},
		Execute: func(s *minidis.SlashContext) error {
			if err := s.ReplyString("Hello! this message will be modified after 5 seconds..."); err != nil {
				_, err = s.Followup("A problem has occured")
				return err
			}

			time.Sleep(time.Second * 5)

			s.Edit("new message here!")

			time.Sleep(time.Second * 5)

			_, err := s.Followup("another message in here!")
			return err
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
