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
	guilds := strings.Fields(strings.ReplaceAll(os.Getenv("GUILD"), ",", " ")) // none means global
	bot := minidis.New(os.Getenv("TOKEN"))

	bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	// {user} is captured from the custom ID into c.Params
	bot.HandleModal("modals_survey:{user}", func(c *minidis.ModalSubmitContext) error {
		if err := c.Reply("Thank you for taking your time to fill the survey"); err != nil {
			return err
		}

		// Read the submitted values by the custom IDs of the components.
		_, err := c.Session.ChannelMessageSend(c, ResultsChannel, fmt.Sprintf(
			"Feedback received. From <@%s>\n\n**Opinion**:\n%s\n\n**Suggestions**:\n%s\n\n**Rating**: %s\n**Liked**: %s\n**May contact**: %t",
			c.Params["user"],
			c.Text("opinion"),
			c.Text("suggestions"),
			c.Text("rating"),
			strings.Join(c.Selected("liked"), ", "),
			c.Checked("contact"),
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
					discord.Label{
						Label: "How would you rate them?",
						Component: discord.RadioGroup{
							CustomID: "rating",
							Options: []discord.GroupOption{
								{Value: "good", Label: "Good"},
								{Value: "okay", Label: "Okay"},
								{Value: "bad", Label: "Bad"},
							},
						},
					},
					discord.Label{
						Label: "What did you like?",
						Component: discord.CheckboxGroup{
							CustomID: "liked",
							Required: discord.Bool(false),
							Options: []discord.GroupOption{
								{Value: "speed", Label: "Speed"},
								{Value: "looks", Label: "Looks"},
							},
						},
					},
					discord.Label{
						Label:     "May we contact you about your feedback?",
						Component: discord.Checkbox{CustomID: "contact"},
					},
				},
			})
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
