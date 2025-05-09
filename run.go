package minidis

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

// Run executes the command handler.
func (m *Minidis) Run() {
	run(m)
}

// Execute the bot.
// It is similar to `Run()` function of `Minidis` struct.
func Execute(bot *Minidis) {
	run(bot)
}

func (m *Minidis) OpenSession() error {
	return m.Session.Open()
}

func (m *Minidis) CloseSession() error {
	return m.Session.Close()
}

// main bot command handler
func run(m *Minidis) {
	m.AppID = m.Session.State.User.ID

	m.Session.AddHandler(m.handleOnMessageCreate)

	m.Session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			err, exists := m.executeSlash(s, i.Interaction)
			if err != nil {
				log.Printf("failed to execute slash command: %v\n", err)
			}
			if exists {
				break
			}

			err, exists = m.executeUser(s, i.Interaction)
			if err != nil {
				log.Printf("failed to execute user command: %v\n", err)
			}
			if exists {
				break
			}

			if err, _ := m.executeMessage(s, i.Interaction); err != nil {
				log.Printf("failed to execute message command: %v\n", err)
			}
		case discordgo.InteractionMessageComponent:
			if err := m.executeComponentHandler(s, i.Interaction); err != nil {
				log.Printf("failed to execute component handler: %v\n", err)
			}
		case discordgo.InteractionModalSubmit:
			if err := m.executeModalSubmit(s, i.Interaction); err != nil {
				log.Printf("failed to execute modal submit handler: %v\n", err)
			}
		default:
			// TODO:
		}
	})
}
