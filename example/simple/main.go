package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tbdsux/minidis"
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

func main() {
	guilds := strings.Split(os.Getenv("GUILD"), ",")
	bot := minidis.New(os.Getenv("TOKEN"))

	// Set Intents
	bot.SetIntents(discord.IntentsGuilds | discord.IntentsGuildMessages)

	bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	// Simple Ping command
	bot.AddCommand(&minidis.Command{
		Name:                     "ping",
		Description:              "Simple ping command.",
		DefaultMemberPermissions: discord.Int64(1 << 31),
		Options:                  []*discord.ApplicationCommandOption{},
		Execute: func(c *minidis.Context) error {
			return c.Reply(fmt.Sprintf("Hello **%s**, pong? Guild: %s", c.Author.Username, c.GuildID))
		},
	})

	// Deferred replies
	bot.AddCommand(&minidis.Command{
		Name:                     "defer",
		Description:              "Deferred reply.",
		DefaultMemberPermissions: discord.Int64(1 << 31),
		Execute: func(c *minidis.Context) error {
			if err := c.DeferReply(true); err != nil {
				return err
			}

			// c is a context.Context, so slow work can stop when the bot closes.
			select {
			case <-time.After(5 * time.Second):
			case <-c.Done():
				return c.Err()
			}

			return c.Edit("This is a deferred reply edit message!")
		},
	})

	// Followup messages
	bot.AddCommand(&minidis.Command{
		Name:        "responses",
		Description: "Responses management.",
		Options:     []*discord.ApplicationCommandOption{},
		Execute: func(c *minidis.Context) error {
			if err := c.Reply("Hello! this message will be modified after 5 seconds..."); err != nil {
				return err
			}

			time.Sleep(time.Second * 5)

			if err := c.Edit("new message here!"); err != nil {
				return err
			}

			time.Sleep(time.Second * 5)

			_, err := c.Followup("another message in here!")
			return err
		},
	})

	// Admin only commands by role
	bot.AddCommand(&minidis.Command{
		Name:                     "admin-only",
		Description:              "Admin only command",
		DefaultMemberPermissions: discord.Int64(0), // 0 = admin only command
		Execute: func(c *minidis.Context) error {
			return c.Reply("You should only see this reply if you are an admin!")
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
