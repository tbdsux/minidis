package minidis

import (
	"log"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
)

type Minidis struct {
	session  *discordgo.Session
	commands map[string]SlashCommandProps
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
		session: s,
		Token:   token,
		AppID:   s.State.User.ID,
	}
}

func (m *Minidis) Run() {
	m.session.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			m.executeSlash(s, i.Interaction)
		case discordgo.InteractionMessageComponent:
			return
		}
	})

	// try to open websocker
	if err := m.session.Open(); err != nil {
		log.Fatalf("Cannot open session: %v\n", err)
	}

	// always close websocket.
	defer m.session.Close()

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, os.Interrupt)
	<-sc

	log.Println("Closing...")
}
