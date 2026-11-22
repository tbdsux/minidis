// Command bot is a bot made with the minidis framework: a slash command with
// a button, a welcome for new members, a message handler and text commands.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/tbdsux/minidis"
	"github.com/tbdsux/minidis/discord"
)

func main() {
	bot := minidis.New(os.Getenv("TOKEN"))

	// Commands show up at once in a guild, so name one while developing. With
	// GUILD unset, they are registered globally.
	bot.SetGuilds(os.Getenv("GUILD"))

	// A slash command, answered with a button.
	bot.AddCommand(&minidis.Command{
		Name:        "hello",
		Description: "Says hello.",
		Execute: func(c *minidis.Context) error {
			return c.ReplyWith(minidis.Message{
				Content: "Hello, " + c.Author.Mention() + "!",
				Components: []discord.MessageComponent{discord.ActionsRow{Components: []discord.MessageComponent{
					discord.Button{Label: "Wave back", Style: discord.PrimaryButton, CustomID: "wave"},
				}}},
			})
		},
	})
	bot.HandleComponent("wave", func(c *minidis.ComponentContext) error {
		return c.ReplyWith(minidis.Message{Content: "👋", Ephemeral: true})
	})

	// Any gateway event, chosen by the type of the second parameter. The bot
	// asks Discord for the intents its handlers need; this one needs the
	// Server Members Intent, which is enabled in the developer portal.
	minidis.On(bot, func(c *minidis.EventContext, e *discord.GuildMemberAdd) error {
		guild, err := c.Session.Guild(c, e.GuildID)
		if err != nil || guild.SystemChannelID == "" {
			return err
		}
		_, err = c.Session.ChannelMessageSend(c, guild.SystemChannelID, "Welcome, "+e.Mention()+"!")
		return err
	})

	minidis.On(bot, func(c *minidis.EventContext, e *discord.Ready) error {
		log.Printf("Logged in as %s. Press Ctrl+C to stop.", e.User.Username)
		return nil
	})

	// Messages people send. Reading them needs the Message Content Intent.
	bot.OnMessage(func(c *minidis.MessageContext) error {
		if strings.Contains(strings.ToLower(c.Content), "good bot") {
			return c.React("❤️")
		}
		return nil
	})

	// Text commands: "!roll 20", or "@bot roll 20".
	bot.SetPrefix("!")
	bot.AddTextCommand(
		&minidis.TextCommand{
			Name:        "say",
			Aliases:     []string{"echo"},
			Description: "Repeats what you wrote.",
			Execute: func(c *minidis.MessageContext) error {
				if c.ArgString == "" {
					_, err := c.Reply("Say what?")
					return err
				}
				// What users wrote must not get to ping anyone.
				_, err := c.SendWith(minidis.Message{Content: c.ArgString, AllowedMentions: &discord.MessageAllowedMentions{}})
				return err
			},
		},
		&minidis.TextCommand{
			Name:        "help",
			Description: "Lists the commands.",
			Execute: func(c *minidis.MessageContext) error {
				var list strings.Builder
				for _, cmd := range c.Bot.TextCommands() {
					fmt.Fprintf(&list, "`!%s` %s\n", cmd.Name, cmd.Description)
				}
				_, err := c.Reply(list.String())
				return err
			},
		},
	)

	// Tell people when their command failed, rather than only logging it.
	bot.HandleMessageError(func(c *minidis.MessageContext, err error) {
		log.Printf("%s: %v", c.Content, err)
		c.Reply("Something went wrong.")
	})

	// Register the commands, connect, and run until Ctrl+C.
	if err := bot.Start(); err != nil {
		log.Fatalln("Bot stopped with an error:", err)
	}
}
