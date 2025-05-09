package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/tbdsux/minidis"
)

func main() {
	guilds := strings.Split(os.Getenv("GUILD"), ",")
	bot := minidis.New(os.Getenv("TOKEN"))

	bot.SetIntents(discordgo.IntentsGuilds | discordgo.IntentsGuildMessages)

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

	// Open session
	if err := bot.OpenSession(); err != nil {
		log.Fatalln("Failed to open session:", err)
		return
	}

	// Re-sync commands
	if err := bot.ClearCommands(guilds...); err != nil {
		log.Fatalln("Failed to clear commands:", err)
		return
	}
	if err := bot.SyncCommands(guilds...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
		return
	}

	// Run the bot
	bot.Run()

	// Wait for CTRL+C to exit
	fmt.Println("Bot is running. Press CTRL+C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Close the session
	bot.CloseSession()
}
