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

func main() {
	guilds := strings.Split(os.Getenv("GUILD"), ",")
	bot := minidis.New(os.Getenv("TOKEN"))

	bot.SetIntents(discordgo.IntentsGuilds | discordgo.IntentsGuildMessages)

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
