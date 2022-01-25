package minidis

import (
	"github.com/bwmarrin/discordgo"
)

type Minidis struct {
	session                *discordgo.Session
	commands               map[string]*SlashCommandProps
	componentHandlers      map[string]*ComponentInteractionProps
	customComponentHandler func(*SlashContext, *ComponentContext) error
	guilds                 []string // guilds to sync the app commands
	Token                  string
	AppID                  string
}

func New(token string) *Minidis {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		// FIXME: do not use panic
		panic(err)
	}

	return &Minidis{
		session:                s,
		commands:               map[string]*SlashCommandProps{},
		componentHandlers:      map[string]*ComponentInteractionProps{},
		customComponentHandler: nil,
		Token:                  token,
	}
}
