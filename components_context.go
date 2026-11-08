package minidis

import "github.com/tbdsux/minidis/discord"

type ComponentContext struct {
	Data discord.MessageComponentInteractionData

	// Params holds the captures of a handler ID such as "vote:{id}".
	Params map[string]string
}

func (m *Minidis) NewComponentContext(event *discord.Interaction) *ComponentContext {
	return &ComponentContext{
		Data: event.MessageComponentData(),
	}
}
