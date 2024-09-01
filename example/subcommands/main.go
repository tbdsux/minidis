package main

import (
	"log"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/tbdsux/minidis"
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

	bot.AddCommand(&minidis.SlashCommandProps{
		Name:        "hello",
		Description: "Say hello to the bot.",
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyString("Hi!")
		},
	})

	// group command
	cmd := bot.AddCommand(&minidis.SlashCommandProps{
		Name:        "group",
		Description: "Simple group command.",
	})
	cmd.AddSubcommand(&minidis.SlashSubcommandProps{
		Name:        "subcommand",
		Description: "A simple subcommand.",
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyString("This is a subcommand!")
		},
	})

	group := cmd.AddSubcommandGroup(&minidis.SlashSubcommandGroupProps{
		Name:        "subgroup",
		Description: "A sub command group.",
	})
	group.AddSubcommand(&minidis.SlashSubcommandProps{
		Name:        "sg",
		Description: "Nested subcommand inside sub command group.",
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyString("This a sub command under a sub command group.")
		},
	})

	if err := bot.Run(); err != nil {
		log.Fatalln(err)
	}
}
