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

	// Set up a command to show component buttons
	bot.AddCommand(&minidis.Command{
		Name:        "buttons",
		Description: "Show buttons component.",
		Options:     []*discord.ApplicationCommandOption{},
		Execute: func(c *minidis.Context) error {
			return c.ReplyWith(
				minidis.Message{
					Content: "Are you cool?",
					Components: []discord.MessageComponent{
						discord.ActionsRow{
							Components: []discord.MessageComponent{
								discord.Button{
									// Label is what the user will see on the button.
									Label: "Yes",
									// Style provides coloring of the button. There are not so many styles tho.
									Style: discord.SuccessButton,
									// Disabled allows bot to disable some buttons for users.
									Disabled: false,
									// CustomID is a thing telling Discord which data to send when this button will be pressed.
									CustomID: "fd_yes",
								},
								discord.Button{
									Label:    "No",
									Style:    discord.DangerButton,
									Disabled: false,
									CustomID: "fd_no",
								},
								discord.Button{
									Label:    "I don't know",
									Style:    discord.LinkButton,
									Disabled: false,
									// Link buttons don't require CustomID and do not trigger the gateway/HTTP event
									URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
									Emoji: &discord.ComponentEmoji{
										Name: "🤷",
									},
								},
							},
						},
					},
				},
			)
		},
	})

	bot.AddCommand(&minidis.Command{
		Name:        "selects",
		Description: "Example for select options.",
		Execute: func(c *minidis.Context) error {
			return c.ReplyWith(
				minidis.Message{
					Content: "What is your favourite programming language?",
					Components: []discord.MessageComponent{
						discord.ActionsRow{
							Components: []discord.MessageComponent{
								discord.SelectMenu{
									// Select menu, as other components, must have a customID, so we set it to this value.
									CustomID:    "select",
									Placeholder: "Choose your favorite programming language 👇",
									Options: []discord.SelectMenuOption{
										{
											Label: "Go",
											// As with components, this things must have their own unique "id" to identify which is which.
											// In this case such id is Value field.
											Value: "go",
											Emoji: &discord.ComponentEmoji{
												Name: "🦦",
											},
											// You can also make it a default option, but in this case we won't.
											Default:     false,
											Description: "Go programming language",
										},
										{
											Label: "JS",
											Value: "js",
											Emoji: &discord.ComponentEmoji{
												Name: "🟨",
											},
											Description: "JavaScript programming language",
										},
										{
											Label: "Python",
											Value: "py",
											Emoji: &discord.ComponentEmoji{
												Name: "🐍",
											},
											Description: "Python programming language",
										},
									},
								},
							},
						},
					},
				},
			)
		},
	})

	bot.HandleComponent("select", func(c *minidis.ComponentContext) error {
		if c.Data.Values[0] == "go" {
			return c.Update("This is the way.")
		}
		return c.Update("It is not the way to go.")
	})

	bot.HandleComponent("fd_yes", func(c *minidis.ComponentContext) error {
		// An empty, non-nil slice removes the buttons from the message
		return c.UpdateWith(minidis.MessageEdit{
			Content:    discord.String("Wow Nice!"),
			Components: []discord.MessageComponent{},
		})
	})

	bot.HandleComponent("fd_no", func(c *minidis.ComponentContext) error {
		// An empty, non-nil slice removes the buttons from the message
		return c.UpdateWith(minidis.MessageEdit{
			Content:    discord.String("Ohhh why???"),
			Components: []discord.MessageComponent{},
		})
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
