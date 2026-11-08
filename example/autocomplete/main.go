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

var languages = []string{"Go", "JavaScript", "Python", "Rust", "TypeScript", "Zig"}

func main() {
	guilds := strings.Split(os.Getenv("GUILD"), ",")
	bot := minidis.New(os.Getenv("TOKEN"))

	bot.SetIntents(discord.IntentsGuilds)

	bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	// Middleware runs around every command, component and modal handler
	bot.Use(func(next minidis.HandlerFunc) minidis.HandlerFunc {
		return func(c *minidis.SlashContext) error {
			log.Printf("interaction from %s", c.Author.Username)
			return next(c)
		}
	})

	// Errors and panics from any interaction handler end up here
	bot.OnError(func(c *minidis.SlashContext, err error) {
		log.Printf("interaction failed: %v", err)
		if replyErr := c.ReplyEphemeral("Something went wrong."); replyErr != nil {
			log.Printf("could not tell the user: %v", replyErr)
		}
	})

	bot.AddCommand(&minidis.SlashCommandProps{
		Name:        "language",
		Description: "Pick a programming language.",
		Options: []*discord.ApplicationCommandOption{
			{
				Type:         discord.ApplicationCommandOptionString,
				Name:         "name",
				Description:  "Start typing a language",
				Required:     true,
				Autocomplete: true,
			},
		},
		Autocomplete: func(c *minidis.AutocompleteContext) error {
			input := strings.ToLower(c.Input())

			var choices []*discord.ApplicationCommandOptionChoice
			for _, language := range languages {
				if strings.Contains(strings.ToLower(language), input) {
					choices = append(choices, minidis.Choice(language, language))
				}
			}

			return c.Respond(choices...)
		},
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyC(minidis.ReplyProps{
				Content: fmt.Sprintf("You picked **%s**. Keep it?", c.String("name")),
				Components: []discord.MessageComponent{
					discord.ActionsRow{
						Components: []discord.MessageComponent{
							discord.Button{
								Label: "Keep",
								Style: discord.SuccessButton,
								// {language} is captured into c.Params by the handler below
								CustomID: "keep:" + c.String("name"),
							},
						},
					},
				},
			})
		},
	})

	bot.AddComponentHandler(&minidis.ComponentInteractionProps{
		ID: "keep:{language}",
		Execute: func(s *minidis.SlashContext, c *minidis.ComponentContext) error {
			// An empty, non-nil slice removes the button from the message
			return c.UpdateC(minidis.EditProps{
				Content:    "Kept **" + c.Params["language"] + "**.",
				Components: []discord.MessageComponent{},
			})
		},
	})

	// Open session
	if err := bot.OpenSession(); err != nil {
		log.Fatalln("Failed to open session:", err)
		return
	}

	// Register the commands
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
