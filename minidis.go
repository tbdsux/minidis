package minidis

import (
	"github.com/bwmarrin/discordgo"
)

type Minidis struct {
	Session *discordgo.Session

	Commands        map[string]*SlashCommandProps
	MessageCommands map[string]*MessageCommandProps
	UserCommands    map[string]*UserCommandProps

	ComponentHandlers      map[string]*ComponentInteractionProps
	CustomComponentHandler func(*SlashContext, *ComponentContext) error

	ModalSubmitHandlers      map[string]*ModalInteractionProps
	CustomModalSubmitHandler func(*SlashContext, *ModalSubmitContext) error

	Token string
	AppID string

	MessageCreateHandler   func(*discordgo.Session, *discordgo.MessageCreate)
	ChannelMessageHandlers map[string]func(*ChannelMessageContext) error
}

// Create a new Minidis instance.
func New(token string) *Minidis {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		// FIXME: do not use panic
		panic(err)
	}

	return &Minidis{
		Session:                  s,
		Commands:                 map[string]*SlashCommandProps{},
		MessageCommands:          map[string]*MessageCommandProps{},
		UserCommands:             map[string]*UserCommandProps{},
		ComponentHandlers:        map[string]*ComponentInteractionProps{},
		ModalSubmitHandlers:      map[string]*ModalInteractionProps{},
		CustomComponentHandler:   nil,
		CustomModalSubmitHandler: nil,
		Token:                    token,
	}
}
