package minidis

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
)

// Run executes the command handler.
func (m *Minidis) Run() error {
	return run(m)
}

// Execute the bot.
// It is similar to `Run()` function of `Minidis` struct.
func Execute(bot *Minidis) error {
	return run(bot)
}

// main bot command handler
func run(m *Minidis) error {
	m.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
				log.Printf("failed to execute user command: %v", err)
			}
			if exists {
				break
			}

			if err, _ := m.executeMessage(s, i.Interaction); err != nil {
				log.Printf("failed to execute message command: %v", err)
			}
		case discordgo.InteractionMessageComponent:
			if err := m.executeComponentHandler(s, i.Interaction); err != nil {
				log.Printf("failed to execute component handler: %v\n", err)
			}
		default:

		}
	})

	// try to open websocket
	if err := m.session.Open(); err != nil {
		return fmt.Errorf("cannot open session: %v", err)
	}

	// set app id
	m.AppID = m.session.State.User.ID

	if m.customHandlers.onBeforeStart != nil {
		// call beforeStart wrapper if not nil
		m.customHandlers.onBeforeStart(m.session)
	}

	// sync commands internally
	if err := m.syncCommands(m.guilds); err != nil {
		return fmt.Errorf("failed to sync commands: %v", err)
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, os.Interrupt)
	<-sc

	if m.customHandlers.onClose != nil {
		// call onClose wrapper if not nil
		m.customHandlers.onClose(m.session)
	}

	// Close the websocket as final.
	return m.session.Close()
}
