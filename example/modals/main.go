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
	bot.AddModalSubmitHandler(&minidis.ModalInteractionProps{
		ID: "modals_survey:{user}",
		Execute: func(s *minidis.SlashContext, c *minidis.ModalSubmitContext) error {
			if err := s.ReplyString("Thank you for taking your time to fill the survey"); err != nil {
				return err
			}

			_, err := s.Session.ChannelMessageSend(ResultsChannel, fmt.Sprintf(
				"Feedback received. From <@%s>\n\n**Opinion**:\n%s\n\n**Suggestions**:\n%s",
				c.Params["user"],
				c.Data.Components[0].(*discord.Label).Component.(*discord.TextInput).Value,
				c.Data.Components[1].(*discord.Label).Component.(*discord.TextInput).Value,
			))

			return err
		},
	})

	// simple command
	bot.AddCommand(&minidis.SlashCommandProps{
		Name:        "modals",
		Description: "Show a modal component example",
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyModal(minidis.ReplyModalProps{
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
