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
	guilds := strings.Fields(strings.ReplaceAll(os.Getenv("GUILD"), ",", " ")) // none means global
	bot := interactions.New(os.Getenv("TOKEN"))

	bot.SetIntents(discord.IntentsGuilds | discord.IntentsGuildMessages)

	bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	// Message command
	bot.AddMessageCommand(&interactions.MessageCommand{
		Name: "re-reply",
		Execute: func(c *interactions.MessageCommandContext) error {
			// A message with only embeds or attachments has no text, and Discord
			// refuses an empty reply.
			content := c.Message.Content
			if content == "" {
				content = "(this message has no text)"
			}
			return c.ReplyWith(interactions.Message{Content: content, Ephemeral: true})
		},
	})

	// User command
	bot.AddUserCommand(&interactions.UserCommand{
		Name: "get-user",
		Execute: func(c *interactions.UserCommandContext) error {

			fmt.Println(c.TargetMember)
			fmt.Println(c.TargetUser)

			return c.Reply("hello " + c.TargetUser.Username)
		},
	})

	// Stop on CTRL+C
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Register the commands. This replaces any registered before.
	if err := bot.SyncCommands(ctx, guilds...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
	}

	fmt.Println("Bot is running. Press CTRL+C to exit.")
	if err := bot.Run(ctx); err != nil {
		log.Fatalln("Bot stopped with an error:", err)
	}
}
