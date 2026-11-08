package minidis

import (
	"fmt"

	"github.com/tbdsux/minidis/discord"
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
	Data discord.ModalSubmitInteractionData
}

func (m *Minidis) NewModalContext(data discord.ModalSubmitInteractionData) *ModalSubmitContext {
	return &ModalSubmitContext{
		Data: data,
	}
}

func (m *Minidis) routeModalSubmit(c *SlashContext) HandlerFunc {
	modalContext := m.NewModalContext(c.Interaction.ModalSubmitData())
	customID := modalContext.Data.CustomID

	execute := m.CustomModalSubmitHandler

	if handler, ok := m.ModalSubmitHandlers[customID]; ok {
		execute = handler.Execute
	}

	if execute == nil {
		return failWith(fmt.Errorf("%w: modal %q", ErrNoHandler, customID))
	}

	return func(c *SlashContext) error {
		return execute(c, modalContext)
	}
}
