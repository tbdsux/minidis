package minidis

import (
	"errors"

	"github.com/bwmarrin/discordgo"
)

type ModalInteractionProps struct {
	ID      string
	Execute func(s *SlashContext, c *ModalSubmitContext) error
}

// AddModalSubmitHandler adds a new modal submit interaction handler for a specific custom id.
func (m *Minidis) AddModalSubmitHandler(mi *ModalInteractionProps) {
	m.ModalSubmitHandlers[mi.ID] = mi
}

// AddCustomModalSubmitHandler is a fallback handler for AddModalSubmitHandler.
// This is useful if you have custom mod ids that are changing or unique.
// This is called if the component's id has no handler set.
func (m *Minidis) AddCustomModalSubmitHandler(handler func(s *SlashContext, c *ModalSubmitContext) error) {
	m.CustomModalSubmitHandler = handler
}

type ModalSubmitContext struct {
	Data discordgo.ModalSubmitInteractionData
}

func (m *Minidis) NewModalContext(data discordgo.ModalSubmitInteractionData) *ModalSubmitContext {
	return &ModalSubmitContext{
		Data: data,
	}
}

func (m *Minidis) executeModalSubmit(session *discordgo.Session, event *discordgo.Interaction) error {
	data := event.ModalSubmitData()

	slashContext := m.NewSlashContext(session, event, false)
	modalContext := m.NewModalContext(data)

	if handler, ok := m.ModalSubmitHandlers[data.CustomID]; ok {
		return handler.Execute(slashContext, modalContext)
	}

	if m.CustomModalSubmitHandler != nil {
		return m.CustomModalSubmitHandler(slashContext, modalContext)
	}

	return errors.New("no modal submit handler defined for custom id: " + data.CustomID)
}
