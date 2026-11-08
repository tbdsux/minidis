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

func main() {
	guilds := strings.Split(os.Getenv("GUILD"), ",")
	bot := minidis.New(os.Getenv("TOKEN"))

	bot.SetIntents(discord.IntentsGuilds | discord.IntentsGuildMessages)

	bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	// Set up a command to show component buttons
	bot.AddCommand(&minidis.SlashCommandProps{
		Name:        "buttons",
		Description: "Show buttons component.",
		Options:     []*discord.ApplicationCommandOption{},
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyC(
				minidis.ReplyProps{
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

	bot.AddCommand(&minidis.SlashCommandProps{
		Name:        "selects",
		Description: "Example for select options.",
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyC(
				minidis.ReplyProps{
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

	bot.AddComponentHandler(&minidis.ComponentInteractionProps{
		ID: "select",
		Execute: func(s *minidis.SlashContext, c *minidis.ComponentContext) error {
			switch c.Data.Values[0] {
			case "go":
				{
					return c.Update("This is the way.")
				}
			default:
				{
					return c.Update("It is not the way to go.")
				}

			}
		},
	})

	bot.AddComponentHandler(&minidis.ComponentInteractionProps{
		ID: "fd_yes",
		Execute: func(s *minidis.SlashContext, c *minidis.ComponentContext) error {
			// An empty, non-nil slice removes the buttons from the message
			return c.UpdateC(minidis.EditProps{
				Content:    "Wow Nice!",
				Components: []discord.MessageComponent{},
			})
		},
	})

	bot.AddComponentHandler(&minidis.ComponentInteractionProps{
		ID: "fd_no",
		Execute: func(s *minidis.SlashContext, c *minidis.ComponentContext) error {
			// An empty, non-nil slice removes the buttons from the message
			return c.UpdateC(minidis.EditProps{
				Content:    "Ohhh why???",
				Components: []discord.MessageComponent{},
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
