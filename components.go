package minidis

type ComponentInteractionProps struct {
	ID      string
	Execute func(s *SlashContext, c *ComponentContext) error
}

// AddComponentHandler adds a function handler once the `id` is called.
// The id can capture parts of a custom ID: segments are separated by ":", and
// one written as "{name}" matches anything, so "vote:{poll}" handles "vote:42"
// with c.Params["poll"] set to "42". An exact id takes priority over a pattern.
func (m *Minidis) AddComponentHandler(cpi *ComponentInteractionProps) {
	m.ComponentHandlers[cpi.ID] = cpi
	m.componentRoutes.add(cpi.ID)
}

// AddCustomComponentHandler is a fallback handler for the AddComponentHandler.
// This is usefull if you have custom component ids that are changing or unique.
// This is called if the component's id has no handler set.
func (m *Minidis) AddCustomComponentHandler(handler func(s *SlashContext, c *ComponentContext) error) {
	m.CustomComponentHandler = handler
}
