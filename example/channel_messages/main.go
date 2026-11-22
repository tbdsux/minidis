package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
	"github.com/tbdsux/minidis/interactions"
)

func main() {
	bot := interactions.New(os.Getenv("TOKEN"))

	bot.SetIntents(discord.IntentsGuilds | discord.IntentsGuildMessages)

	bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	greet := func(c *interactions.ChannelMessageContext) error {
		message, err := c.Reply("Hello! This is a message from the bot.")
		if err != nil {
			return err
		}

		fmt.Println(message.Content)

		return nil
	}

	// CHANNELS is a comma-separated list of the channels to answer in.
	for _, channel := range strings.Split(os.Getenv("CHANNELS"), ",") {
		if channel != "" {
			bot.HandleChannelMessage(channel, greet)
		}
	}

	// Stop on CTRL+C
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// This bot has no commands, so it does not call SyncCommands: syncing
	// would replace the app's registered commands with none.

	fmt.Println("Bot is running. Press CTRL+C to exit.")
	if err := bot.Run(ctx); err != nil {
		log.Fatalln("Bot stopped with an error:", err)
	}
}
