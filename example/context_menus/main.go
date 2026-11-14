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

	// Message command
	bot.AddMessageCommand(&minidis.MessageCommand{
		Name: "re-reply",
		Execute: func(c *minidis.MessageCommandContext) error {
			return c.ReplyWith(minidis.Message{Content: c.Message.Content, Ephemeral: true})
		},
	})

	// User command
	bot.AddUserCommand(&minidis.UserCommand{
		Name: "get-user",
		Execute: func(c *minidis.UserCommandContext) error {

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
