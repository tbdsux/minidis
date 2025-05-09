package minidis

import (
	"github.com/bwmarrin/discordgo"
)

type Minidis struct {
	session *discordgo.Session

	commands        map[string]*SlashCommandProps
	messageCommands map[string]*MessageCommandProps
	userCommands    map[string]*UserCommandProps

	componentHandlers      map[string]*ComponentInteractionProps
	customComponentHandler func(*SlashContext, *ComponentContext) error

	modalSubmitHandlers      map[string]*ModalInteractionProps
	customModalSubmitHandler func(*SlashContext, *ModalSubmitContext) error

	guilds         []string // guilds to sync the app commands
	Token          string
	AppID          string
	customHandlers *CustomHandlers

	messageCreateHandler   func(*discordgo.Session, *discordgo.MessageCreate)
	channelMessageHandlers map[string]func(*ChannelMessageContext) error
}

type CustomHandlers struct {
	onClose       func(*discordgo.Session)
	onBeforeStart func(*discordgo.Session)
}

// Create a new Minidis instance.
func New(token string) *Minidis {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		// FIXME: do not use panic
		panic(err)
	}

	return &Minidis{
		session:                  s,
		commands:                 map[string]*SlashCommandProps{},
		messageCommands:          map[string]*MessageCommandProps{},
		userCommands:             map[string]*UserCommandProps{},
		componentHandlers:        map[string]*ComponentInteractionProps{},
		modalSubmitHandlers:      map[string]*ModalInteractionProps{},
		customComponentHandler:   nil,
		customModalSubmitHandler: nil,
		Token:                    token,
		customHandlers: &CustomHandlers{
			onClose:       nil,
			onBeforeStart: nil,
		},
	}
}
