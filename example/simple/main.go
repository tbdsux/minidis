package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tbdsux/minidis"
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

func main() {
	guilds := strings.Split(os.Getenv("GUILD"), ",")
	bot := minidis.New(os.Getenv("TOKEN"))

	// Set Intents
	bot.SetIntents(discord.IntentsGuilds | discord.IntentsGuildMessages)

	bot.OnReady(func(s *client.Session, i *discord.Ready) {
		log.Println("Bot is ready!")
	})

	// Simple Ping command
	bot.AddCommand(&minidis.SlashCommandProps{
		Name:                     "ping",
		Description:              "Simple ping command.",
		DefaultMemberPermissions: discord.Int64(1 << 31),
		Options:                  []*discord.ApplicationCommandOption{},
		Execute: func(c *minidis.SlashContext) error {
			return c.ReplyString(fmt.Sprintf("Hello **%s**, pong? Guild: %s", c.Author.Username, c.GuildId))
		},
	})

	// Deferred replies
	bot.AddCommand(&minidis.SlashCommandProps{
		Name:                     "defer",
		Description:              "Deferred reply.",
		DefaultMemberPermissions: discord.Int64(1 << 31),
		Execute: func(c *minidis.SlashContext) error {
			c.DeferReply(true)

			time.Sleep(time.Second * 5)

			return c.Edit("This is a deffered reply edit message!")
		},
	})

	// Followup messages
	bot.AddCommand(&minidis.SlashCommandProps{
		Name:        "responses",
		Description: "Responses management.",
		Options:     []*discord.ApplicationCommandOption{},
		Execute: func(s *minidis.SlashContext) error {
			if err := s.ReplyString("Hello! this message will be modified after 5 seconds..."); err != nil {
				_, err = s.Followup("A problem has occured")
				return err
			}

			time.Sleep(time.Second * 5)

			s.Edit("new message here!")

			time.Sleep(time.Second * 5)

			_, err := s.Followup("another message in here!")
			return err
		},
	})

	// Admin only commands by role
	bot.AddCommand(&minidis.SlashCommandProps{
		Name:                     "admin-only",
		Description:              "Admin only command",
		DefaultMemberPermissions: discord.Int64(0), // 0 = admin only command
		Execute: func(s *minidis.SlashContext) error {
			return s.ReplyString("You should only see this reply if you are an admin!")
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
