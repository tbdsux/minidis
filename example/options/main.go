package main

import (
	"fmt"
	"log"
	"os"

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

	// sample command with options
	bot.AddCommand(&minidis.SlashCommandProps{
		Command:     "options",
		Description: "Simple command with options.",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "name",
				Description: "Your name,",
				Required:    true,
			},
		},
		Execute: func(c *minidis.SlashContext) error {
			name := c.Options["name"].StringValue()

			return c.ReplyString(fmt.Sprintf("Your name is **%s**?", name))
		},
	})

	if err := bot.Run(); err != nil {
		log.Fatalln(err)
	}
}
