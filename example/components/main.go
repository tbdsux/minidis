package main

import (
	"log"
	"os"

	"github.com/World-of-Cryptopups/minidis"
	"github.com/bwmarrin/discordgo"
)

func main() {
	bot := minidis.New(os.Getenv("TOKEN"))

	// set intents
	bot.SetIntents(discordgo.IntentsGuilds | discordgo.IntentsGuildMessages)

	//
	bot.SyncToGuilds(os.Getenv("GUILD"))

	bot.OnReady(func(s *discordgo.Session, i *discordgo.Ready) {
		log.Println("Bot is ready!")
	})

	// simple command
	bot.AddCommand(&minidis.SlashCommandProps{
		Command:     "buttons",
		Description: "Show buttons component.",
		Options:     []*discordgo.ApplicationCommandOption{},
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyC(
				minidis.ReplyProps{
					Content: "Are you cool?",
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								discordgo.Button{
									// Label is what the user will see on the button.
									Label: "Yes",
									// Style provides coloring of the button. There are not so many styles tho.
									Style: discordgo.SuccessButton,
									// Disabled allows bot to disable some buttons for users.
									Disabled: false,
									// CustomID is a thing telling Discord which data to send when this button will be pressed.
									CustomID: "fd_yes",
								},
								discordgo.Button{
									Label:    "No",
									Style:    discordgo.DangerButton,
									Disabled: false,
									CustomID: "fd_no",
								},
								discordgo.Button{
									Label:    "I don't know",
									Style:    discordgo.LinkButton,
									Disabled: false,
									// Link buttons don't require CustomID and do not trigger the gateway/HTTP event
									URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
									Emoji: discordgo.ComponentEmoji{
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
		Command:     "selects",
		Description: "Example for select options.",
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyC(
				minidis.ReplyProps{
					Content: "What is your favourite programming language?",
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								discordgo.SelectMenu{
									// Select menu, as other components, must have a customID, so we set it to this value.
									CustomID:    "select",
									Placeholder: "Choose your favorite programming language 👇",
									Options: []discordgo.SelectMenuOption{
										{
											Label: "Go",
											// As with components, this things must have their own unique "id" to identify which is which.
											// In this case such id is Value field.
											Value: "go",
											Emoji: discordgo.ComponentEmoji{
												Name: "🦦",
											},
											// You can also make it a default option, but in this case we won't.
											Default:     false,
											Description: "Go programming language",
										},
										{
											Label: "JS",
											Value: "js",
											Emoji: discordgo.ComponentEmoji{
												Name: "🟨",
											},
											Description: "JavaScript programming language",
										},
										{
											Label: "Python",
											Value: "py",
											Emoji: discordgo.ComponentEmoji{
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
					return s.ReplyString("This is the way.")
				}
			default:
				{
					return s.ReplyString("It is not the way to go.")
				}

			}
		},
	})

	bot.AddComponentHandler(&minidis.ComponentInteractionProps{
		ID: "fd_yes",
		Execute: func(s *minidis.SlashContext, c *minidis.ComponentContext) error {
			return s.ReplyString("Wow Nice!")
		},
	})

	bot.AddComponentHandler(&minidis.ComponentInteractionProps{
		ID: "fd_no",
		Execute: func(s *minidis.SlashContext, c *minidis.ComponentContext) error {
			return s.Reply("Ohhh why???")
		},
	})

	if err := bot.Run(); err != nil {
		log.Fatalln(err)
	}
}
