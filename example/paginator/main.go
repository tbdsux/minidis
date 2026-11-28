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
	"github.com/tbdsux/minidis/interactions"
)

func main() {
	guilds := strings.Fields(strings.ReplaceAll(os.Getenv("GUILD"), ",", " ")) // none means global
	bot := interactions.New(os.Getenv("TOKEN"))

	bot.AddCommand(&interactions.Command{
		Name:        "planets",
		Description: "Browse the planets, one page each.",
		Execute: func(c *interactions.Context) error {
			var pages []interactions.Page
			for i, name := range []string{"Mercury", "Venus", "Earth", "Mars", "Jupiter", "Saturn", "Uranus", "Neptune"} {
				pages = append(pages, interactions.Page{Embeds: []*discord.MessageEmbed{{
					Title:       name,
					Description: fmt.Sprintf("Planet number %d from the Sun.", i+1),
				}}})
			}

			// Blocks until the buttons time out, Stop is clicked or the bot closes.
			return bot.Paginate(c, pages, interactions.PaginateOptions{
				FirstLast: true,
				Counter:   true,
				Stop:      true,
			})
		},
	})

	// Stop on CTRL+C
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := bot.SyncCommands(ctx, guilds...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
	}

	fmt.Println("Bot is running. Press CTRL+C to exit.")
	if err := bot.Run(ctx); err != nil {
		log.Fatalln("Bot stopped with an error:", err)
	}
}
