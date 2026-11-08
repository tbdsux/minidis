package minidis

import (
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

type Minidis struct {
	Session *client.Session

	Commands        map[string]*SlashCommandProps
	MessageCommands map[string]*MessageCommandProps
	UserCommands    map[string]*UserCommandProps

	ComponentHandlers      map[string]*ComponentInteractionProps
	CustomComponentHandler func(*SlashContext, *ComponentContext) error

	ModalSubmitHandlers      map[string]*ModalInteractionProps
	CustomModalSubmitHandler func(*SlashContext, *ModalSubmitContext) error

	Token string
	AppID string

	MessageCreateHandler   func(*client.Session, *discord.MessageCreate)
	ChannelMessageHandlers map[string]func(*ChannelMessageContext) error
}

// Create a new Minidis instance.
func New(token string) *Minidis {
	return &Minidis{
		Session:                  client.New("Bot " + token),
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
