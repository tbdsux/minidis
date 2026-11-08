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
// The id can contain captures, as described on AddComponentHandler.
func (m *Minidis) AddModalSubmitHandler(mi *ModalInteractionProps) {
	m.ModalSubmitHandlers[mi.ID] = mi
	m.modalRoutes.add(mi.ID)
}

// AddCustomModalSubmitHandler is a fallback handler for AddModalSubmitHandler.
// This is useful if you have custom mod ids that are changing or unique.
// This is called if the component's id has no handler set.
func (m *Minidis) AddCustomModalSubmitHandler(handler func(s *SlashContext, c *ModalSubmitContext) error) {
	m.CustomModalSubmitHandler = handler
}

type ModalSubmitContext struct {
	Data discord.ModalSubmitInteractionData

	// Params holds the captures of a handler ID such as "feedback:{id}".
	Params map[string]string
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
	} else if id, params, ok := m.modalRoutes.match(customID); ok {
		execute = m.ModalSubmitHandlers[id].Execute
		modalContext.Params = params
	}

	if execute == nil {
		return failWith(fmt.Errorf("%w: modal %q", ErrNoHandler, customID))
	}

	return func(c *SlashContext) error {
		return execute(c, modalContext)
	}
}
