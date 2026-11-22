package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
	"github.com/tbdsux/minidis/interactions"
)

var languages = []string{"Go", "JavaScript", "Python", "Rust", "TypeScript", "Zig"}

func main() {
	guilds := strings.Fields(strings.ReplaceAll(os.Getenv("GUILD"), ",", " ")) // none means global
	bot := interactions.New(os.Getenv("TOKEN"))

	bot.SetIntents(discord.IntentsGuilds)

	bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	// Middleware runs around every command, component and modal handler
	bot.Use(func(next interactions.HandlerFunc) interactions.HandlerFunc {
		return func(c *interactions.Context) error {
			log.Printf("interaction from %s", c.Author.Username)
			return next(c)
		}
	})

	// Errors and panics from any interaction handler end up here
	bot.HandleError(func(c *interactions.Context, err error) {
		log.Printf("interaction failed: %v", err)

		// Autocomplete can only answer with choices.
		if c.Interaction.Type == discord.InteractionApplicationCommandAutocomplete {
			return
		}

		// After a reply or a deferral, the user is told in a followup instead.
		msg := interactions.Message{Content: "Something went wrong.", Ephemeral: true}
		var tellErr error
		if c.Responded() {
			_, tellErr = c.FollowupWith(msg)
		} else {
			tellErr = c.ReplyWith(msg)
		}
		if tellErr != nil {
			log.Printf("could not tell the user: %v", tellErr)
		}
	})

	bot.AddCommand(&interactions.Command{
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
		Autocomplete: func(c *interactions.AutocompleteContext) error {
			input := strings.ToLower(c.Input())

			var choices []*discord.ApplicationCommandOptionChoice
			for _, language := range languages {
				if strings.Contains(strings.ToLower(language), input) {
					choices = append(choices, interactions.Choice(language, language))
				}
			}

			return c.Respond(choices...)
		},
		Execute: func(c *interactions.Context) error {
			// Autocomplete only suggests: the user may still type anything. The
			// value goes into a custom ID below, which is limited to 100
			// characters and whose {language} capture cannot contain ":", so
			// accept only the known languages.
			name := c.String("name")
			if !slices.Contains(languages, name) {
				return c.ReplyWith(interactions.Message{Content: "Unknown language: pick one of the suggestions.", Ephemeral: true})
			}

			return c.ReplyWith(interactions.Message{
				Content: fmt.Sprintf("You picked **%s**. Keep it?", name),
				Components: []discord.MessageComponent{
					discord.ActionsRow{
						Components: []discord.MessageComponent{
							discord.Button{
								Label: "Keep",
								Style: discord.SuccessButton,
								// {language} is captured into c.Params by the handler below
								CustomID: "keep:" + name,
							},
						},
					},
				},
			})
		},
	})

	bot.HandleComponent("keep:{language}", func(c *interactions.ComponentContext) error {
		// An empty, non-nil slice removes the button from the message
		return c.UpdateWith(interactions.MessageEdit{
			Content:    discord.String("Kept **" + c.Params["language"] + "**."),
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
