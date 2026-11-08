package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tbdsux/minidis"
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

func main() {
	guilds := strings.Split(os.Getenv("GUILD"), ",")
	bot := minidis.New(os.Getenv("TOKEN"))

	bot.SetIntents(discord.IntentsGuilds | discord.IntentsGuildMessages)

	bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	// Message command
	bot.AddMessageCommand(&minidis.MessageCommandProps{
		Name: "re-reply",
		Execute: func(c *minidis.MessageCommandContext) error {
			return c.ReplyEphemeral(c.Message.Content)
		},
	})

	// User command
	bot.AddUserCommand(&minidis.UserCommandProps{
		Name: "get-user",
		Execute: func(c *minidis.UserCommandContext) error {

			fmt.Println(c.Member)
			fmt.Println(c.User)

			return c.ReplyString("hello " + c.User.Username)
		},
	})

	// Open session
	if err := bot.OpenSession(); err != nil {
		log.Fatalln("Failed to open session:", err)
		return
	}

	// Register the commands. This replaces any registered before.
	if err := bot.SyncCommands(guilds...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
		return
	}

	// Wait for CTRL+C to exit
	fmt.Println("Bot is running. Press CTRL+C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Close the session
	bot.CloseSession()
}
