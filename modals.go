package minidis

import (
	"fmt"

	"github.com/tbdsux/minidis/discord"
)

// ModalSubmitContext is passed to the handler of a submitted modal.
type ModalSubmitContext struct {
	*Context
	Data discord.ModalSubmitInteractionData

	// Params holds the captures of a handler ID such as "feedback:{id}".
	Params map[string]string
}

// HandleModal sets the handler for submissions of the modals with the custom ID
// id. The id can contain captures, as described on HandleComponent.
func (m *Minidis) HandleModal(id string, h func(c *ModalSubmitContext) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.modals.add(id, h)
}

// HandleModalFallback sets the handler for modal submissions that no
// HandleModal id matches. Without one, they reach the error handler as
// ErrNoHandler.
func (m *Minidis) HandleModalFallback(h func(c *ModalSubmitContext) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.modalFallback = h
}

func (m *Minidis) routeModalSubmit(c *Context) HandlerFunc {
	data := c.Interaction.ModalSubmitData()

	h, params, ok := m.modals.find(data.CustomID)
	if !ok {
		h = m.modalFallback
	}
	if h == nil {
		return failWith(fmt.Errorf("%w: modal %q", ErrNoHandler, data.CustomID))
	}

	return func(c *Context) error {
		return h(&ModalSubmitContext{Context: c, Data: data, Params: params})
	}
}
