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

	// Message command
	bot.AddMessageCommand(&minidis.MessageCommandProps{
		Name: "re-reply",
		Execute: func(c *minidis.MessageCommandContext) error {
			return c.ReplyC(minidis.ReplyProps{
				Content:     c.Message.Content,
				IsEphemeral: true,
			})
		},
	})

	// User command
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
