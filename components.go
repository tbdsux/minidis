package minidis

type ComponentInteractionProps struct {
	ID      string
	Execute func(s *SlashContext, c *ComponentContext) error
}

// AddComponentHandler adds a function handler once the `id` is called.
func (m *Minidis) AddComponentHandler(cpi *ComponentInteractionProps) {
	m.ComponentHandlers[cpi.ID] = cpi
}

// AddCustomComponentHandler is a fallback handler for the AddComponentHandler.
// This is usefull if you have custom component ids that are changing or unique.
// This is called if the component's id has no handler set.
func (m *Minidis) AddCustomComponentHandler(handler func(s *SlashContext, c *ComponentContext) error) {
	m.CustomComponentHandler = handler
}
