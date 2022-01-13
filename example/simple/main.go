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

	bot.OnReady(func(s *discordgo.Session, i *discordgo.Ready) {
		log.Println("Bot is ready!")
	})

	// simple command
	bot.AddCommand(minidis.SlashCommandProps{
		Command:     "ping",
		Description: "Simple ping command.",
		Options:     []*discordgo.ApplicationCommandOption{},
		Execute: func(c *minidis.SlashContext) error {
			_, err := c.ReplyString(fmt.Sprintf("Hello **%s**, pong?", c.Author.Username))

			return err
		},
	})

	// if err := bot.SyncCommands("751230186594500689"); err != nil {
	// 	log.Fatalf("Failed to sync commands: %v\n", err)
	// }

	bot.Run()
}
