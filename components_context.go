package minidis

import "github.com/tbdsux/minidis/discord"

type ComponentContext struct {
	Data discord.MessageComponentInteractionData
}

func (m *Minidis) NewComponentContext(event *discord.Interaction) *ComponentContext {
	return &ComponentContext{
		Data: event.MessageComponentData(),
	}
}
