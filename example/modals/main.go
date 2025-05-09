package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/tbdsux/minidis"
)

var (
	ResultsChannel = os.Getenv("CHANNEL")
)

func main() {
	guilds := strings.Split(os.Getenv("GUILD"), ",")
	bot := minidis.New(os.Getenv("TOKEN"))

	bot.OnReady(func(s *discordgo.Session, i *discordgo.Ready) {
		log.Println("Bot is ready!")
	})

	bot.AddCustomModalSubmitHandler(func(s *minidis.SlashContext, c *minidis.ModalSubmitContext) error {
		s.ReplyString("Thank you for taking your time to fill the survey")

		if !strings.HasPrefix(c.Data.CustomID, "modals_survey") {
			return nil
		}

		userid := strings.Split(c.Data.CustomID, "_")[2]
		_, err := s.Session.ChannelMessageSend(ResultsChannel, fmt.Sprintf(
			"Feedback received. From <@%s>\n\n**Opinion**:\n%s\n\n**Suggestions**:\n%s",
			userid,
			c.Data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value,
			c.Data.Components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value,
		))

		return err
	})

	// simple command
	bot.AddCommand(&minidis.SlashCommandProps{
		Name:        "modals",
		Description: "Show a modal component example",
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyModal(minidis.ReplyModalProps{
				Title:    "Modals survey",
				CustomID: "modals_survey_" + c.Interaction.Member.User.ID,
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "opinion",
								Label:       "What is your opinion on them?",
								Style:       discordgo.TextInputShort,
								Placeholder: "Don't be shy, share your opinion with us",
								Required:    true,
								MaxLength:   300,
								MinLength:   10,
							},
						},
					},
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:  "suggestions",
								Label:     "What would you suggest to improve them?",
								Style:     discordgo.TextInputParagraph,
								Required:  false,
								MaxLength: 2000,
							},
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

	// Re-sync commands
	if err := bot.ClearCommands(guilds...); err != nil {
		log.Fatalln("Failed to clear commands:", err)
		return
	}
	if err := bot.SyncCommands(guilds...); err != nil {
		log.Fatalln("Failed to sync commands:", err)
		return
	}

	// Run the bot
	bot.Run()

	// Wait for CTRL+C to exit
	fmt.Println("Bot is running. Press CTRL+C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Close the session
	bot.CloseSession()
}
