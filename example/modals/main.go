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

var (
	ResultsChannel = os.Getenv("CHANNEL")
)

func main() {
	guilds := strings.Split(os.Getenv("GUILD"), ",")
	bot := minidis.New(os.Getenv("TOKEN"))

	bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	// {user} is captured from the custom ID into c.Params
	bot.HandleModal("modals_survey:{user}", func(c *minidis.ModalSubmitContext) error {
		if err := c.Reply("Thank you for taking your time to fill the survey"); err != nil {
			return err
		}

		_, err := c.Session.ChannelMessageSend(ResultsChannel, fmt.Sprintf(
			"Feedback received. From <@%s>\n\n**Opinion**:\n%s\n\n**Suggestions**:\n%s",
			c.Params["user"],
			c.Data.Components[0].(*discord.Label).Component.(*discord.TextInput).Value,
			c.Data.Components[1].(*discord.Label).Component.(*discord.TextInput).Value,
		))

		return err
	})

	// simple command
	bot.AddCommand(&minidis.Command{
		Name:        "modals",
		Description: "Show a modal component example",
		Execute: func(c *minidis.Context) error {
			return c.ReplyModal(minidis.Modal{
				Title:    "Modals survey",
				CustomID: "modals_survey:" + c.Author.ID,
				Components: []discord.MessageComponent{
					discord.Label{
						Label: "What is your opinion on them?",
						Component: discord.TextInput{
							CustomID:    "opinion",
							Style:       discord.TextInputShort,
							Placeholder: "Don't be shy, share your opinion with us",
							Required:    discord.Bool(true),
							MaxLength:   300,
							MinLength:   10,
						},
					},
					discord.Label{
						Label: "What would you suggest to improve them?",
						Component: discord.TextInput{
							CustomID:  "suggestions",
							Style:     discord.TextInputParagraph,
							Required:  discord.Bool(false),
							MaxLength: 2000,
						},
					},
				},
			})
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
