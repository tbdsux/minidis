package minidis

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
)

type Minidis struct {
	session  *discordgo.Session
	commands map[string]*SlashCommandProps
	guilds   []string // guilds to sync the app commands
	Token    string
	AppID    string
}

func New(token string) *Minidis {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		// FIXME: do not use panic
		panic(err)
	}

	return &Minidis{
		session:  s,
		commands: map[string]*SlashCommandProps{},
		Token:    token,
	}
}

// Run executes the command handler.
func (m *Minidis) Run() error {
	m.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if err := m.executeSlash(s, i.Interaction); err != nil {
				log.Printf("failed to execute slash command: %v\n", err)
			}
		case discordgo.InteractionMessageComponent:
			return
		}
	})

	// try to open websocket
	if err := m.session.Open(); err != nil {
		return fmt.Errorf("cannot open session: %v", err)
	}

	// set app id
	m.AppID = m.session.State.User.ID

	// sync commands internally
	if err := m.syncCommands(m.guilds); err != nil {
		return fmt.Errorf("failed to sync commands: %v", err)
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, os.Interrupt)
	<-sc

	log.Println("Closing...")

	// Close the websocket as final.
	return m.session.Close()
}
