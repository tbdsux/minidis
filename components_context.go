package minidis

import "github.com/bwmarrin/discordgo"

type ComponentContext struct {
	Data discordgo.MessageComponentInteractionData
}

func (m *Minidis) NewComponentContext(event *discordgo.Interaction) *ComponentContext {
	return &ComponentContext{
		Data: event.MessageComponentData(),
	}
}
