package minidis

import (
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

func (m *Minidis) executeComponentHandler(session *client.Session, event *discord.Interaction) error {
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
