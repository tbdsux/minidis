package minidis

import (
	"fmt"

	"github.com/tbdsux/minidis/discord"
)

// ComponentContext is passed to the handler of a button or select menu.
type ComponentContext struct {
	*Context
	Data discord.MessageComponentInteractionData

	// Params holds the captures of a handler ID such as "vote:{id}".
	Params map[string]string
}

// HandleComponent sets the handler for the components with the custom ID id.
//
// The id can capture parts of a custom ID: segments are separated by ":", and
// one written as "{name}" matches anything, so "vote:{poll}" handles "vote:42"
// with c.Params["poll"] set to "42". A capture is one segment, so the value
// put there must not contain ":". An exact id takes priority over a pattern,
// and a pattern with more literal segments over one with fewer.
func (m *Minidis) HandleComponent(id string, h func(c *ComponentContext) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.components.add(id, h)
}

// HandleComponentFallback sets the handler for components that no
// HandleComponent id matches. Without one, they reach the error handler as
// ErrNoHandler; when other code handles some components, such as a collector
// on Session.OnInteractionCreate, set one that returns nil.
func (m *Minidis) HandleComponentFallback(h func(c *ComponentContext) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.componentFallback = h
}

func (m *Minidis) routeComponent(c *Context) HandlerFunc {
	data := c.Interaction.MessageComponentData()

	h, params, ok := m.components.find(data.CustomID)
	if !ok {
		h = m.componentFallback
	}
	if h == nil {
		return failWith(fmt.Errorf("%w: component %q", ErrNoHandler, data.CustomID))
	}

	return func(c *Context) error {
		return h(&ComponentContext{Context: c, Data: data, Params: params})
	}
}

// Update responds by replacing the text of the message the component is on.
func (c *ComponentContext) Update(content string) error {
	return c.update(MessageEdit{Content: &content})
}

// UpdateWith responds by changing the message the component is on.
func (c *ComponentContext) UpdateWith(edit MessageEdit) error {
	return c.update(edit)
}

// DeferUpdate acknowledges the interaction without changing the message yet.
// Change it within 15 minutes using Edit.
func (c *ComponentContext) DeferUpdate() error {
	return c.deferUpdate()
}
