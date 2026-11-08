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

	// Sample command with options
	bot.AddCommand(&minidis.SlashCommandProps{
		Name:        "options",
		Description: "Simple command with options.",
		Options: []*discord.ApplicationCommandOption{
			{
				Type:        discord.ApplicationCommandOptionString,
				Name:        "name",
				Description: "Your name,",
				Required:    true,
			},
		},
		Execute: func(c *minidis.SlashContext) error {
			name := c.String("name")

			return c.ReplyString(fmt.Sprintf("Your name is **%s**?", name))
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
