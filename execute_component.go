package minidis

// routeComponent returns the handler for a component interaction, or nil if
// none is set.
func (m *Minidis) routeComponent(c *SlashContext) HandlerFunc {
	componentContext := m.NewComponentContext(c.Session, c.Interaction)
	customID := componentContext.Data.CustomID

	execute := m.CustomComponentHandler

	if handler, ok := m.ComponentHandlers[customID]; ok {
		execute = handler.Execute
	} else if id, params, ok := m.componentRoutes.match(customID); ok {
		execute = m.ComponentHandlers[id].Execute
		componentContext.Params = params
	}

	if execute == nil {
		return nil
	}

	return func(c *SlashContext) error {
		return execute(c, componentContext)
	}
}
