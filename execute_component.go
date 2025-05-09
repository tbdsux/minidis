package minidis

import "github.com/bwmarrin/discordgo"

func (m *Minidis) executeComponentHandler(session *discordgo.Session, event *discordgo.Interaction) error {
	data := event.MessageComponentData()

	slashContext := m.NewSlashContext(session, event, false)
	componentContext := m.NewComponentContext(event)

	if handler, ok := m.ComponentHandlers[data.CustomID]; ok {
		return handler.Execute(
			slashContext,
			componentContext,
		)
	}

	// nil means it is not set
	if m.CustomComponentHandler != nil {
		return m.CustomComponentHandler(
			slashContext,
			componentContext,
		)
	}

	return nil
}
