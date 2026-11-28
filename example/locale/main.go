package main

import (
	"embed"
	"log"
	"os"
	"time"

	"github.com/tbdsux/minidis"
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/locale"
)

// The translations, one JSON file per Discord locale.
//
//go:embed locales/*.json
var locales embed.FS

func main() {
	bot := minidis.New(os.Getenv("TOKEN"))
	bot.SetGuilds(os.Getenv("GUILD"))

	// Messages missing in the user's and the guild's language are taken from
	// en-US.json.
	cat := locale.New(discord.EnglishUS)
	if err := cat.LoadFS(locales, "locales/*.json"); err != nil {
		log.Fatal(err)
	}
	bot.SetCatalog(cat)

	// The name and description in other languages come from the catalog, under
	// command.greet.name and command.greet.description.
	bot.AddCommand(&minidis.Command{
		Name:        "greet",
		Description: "Says hello in your language.",
		// The cooldown reply is translated under minidis.cooldown.
		Execute: minidis.Cooldown(10*time.Second, minidis.PerUser)(func(c *minidis.Context) error {
			return c.Reply(c.T("greet.reply", c.Author.Username))
		}),
	})

	if err := bot.Start(); err != nil {
		log.Fatal(err)
	}
}
