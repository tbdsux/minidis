package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/tbdsux/minidis"
)

func main() {
	bot := minidis.New(os.Getenv("TOKEN"))

	bot.SetIntents(discordgo.IntentsGuilds | discordgo.IntentsGuildMessages)

	bot.SyncToGuilds(os.Getenv("GUILD"))

	bot.OnReady(func(s *discordgo.Session, i *discordgo.Ready) {
		log.Println("Bot is ready!")
	})

	channels := strings.Split(os.Getenv("CHANNELS"), ",")

	bot.OnChannelMessageCreate(func(c *minidis.ChannelMessageContext) error {
		if c.Message.Author.ID == c.Session.State.User.ID {
			return nil
		}

		message, err := c.Reply("Hello! This is a message from the bot.", true)
		if err != nil {
			return err
		}

		fmt.Println(message.Content)

		return nil
	}, channels...)

	bot.OnClose(func(s *discordgo.Session) {
		if err := bot.ClearCommands(); err != nil {
			log.Fatal(err)
		}

		log.Println("Closing...")
	})

	if err := bot.Run(); err != nil {
		log.Fatalln(err)
	}
}
