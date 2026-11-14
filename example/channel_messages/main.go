package main

import (
	"context"
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

	channels := strings.Split(os.Getenv("CHANNELS"), ",")

	bot.OnChannelMessageCreate(func(c *minidis.ChannelMessageContext) error {
		message, err := c.Reply("Hello! This is a message from the bot.")
		if err != nil {
			return err
		}

		fmt.Println(message.Content)

		return nil
	}, channels...)

	// Register the commands. This replaces any registered before.
	if err := bot.SyncCommands(guilds...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
	}

	// Run until CTRL+C
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("Bot is running. Press CTRL+C to exit.")
	if err := bot.Run(ctx); err != nil {
		log.Fatalln("Bot stopped with an error:", err)
	}
}
