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

	bot.AddCommand(&minidis.Command{
		Name:        "hello",
		Description: "Say hello to the bot.",
		Execute: func(c *minidis.Context) error {
			return c.Reply("Hi!")
		},
	})

	// A command with subcommands only runs them, so it has no Execute itself.
	bot.AddCommand(&minidis.Command{
		Name:        "group",
		Description: "Simple group command.",
		Subcommands: []*minidis.Subcommand{
			{
				Name:        "subcommand",
				Description: "A simple subcommand.",
				Execute: func(c *minidis.Context) error {
					return c.Reply("This is a subcommand!")
				},
			},
		},
		Groups: []*minidis.SubcommandGroup{
			{
				Name:        "subgroup",
				Description: "A sub command group.",
				Subcommands: []*minidis.Subcommand{
					{
						Name:        "sg",
						Description: "Nested subcommand inside sub command group.",
						Execute: func(c *minidis.Context) error {
							return c.Reply("This a sub command under a sub command group.")
						},
					},
				},
			},
		},
	})

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
