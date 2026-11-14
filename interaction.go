package minidis

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

// interactionLifetime is how long Discord accepts responses to an interaction.
const interactionLifetime = 15 * time.Minute

func (m *Minidis) handleInteraction(s *client.Session, i *discord.InteractionCreate) {
	ctx, cancel := context.WithTimeout(m.baseContext(), interactionLifetime)
	defer cancel()

	c := newContext(ctx, s, i.Interaction)

	defer func() {
		if r := recover(); r != nil {
			m.handleError(c, &PanicError{Value: r, Stack: debug.Stack()})
		}
	}()

	handler := m.route(c)
	if handler == nil {
		return
	}

	// Autocomplete fires on every keystroke, so middleware is skipped for it.
	if i.Type != discord.InteractionApplicationCommandAutocomplete {
		handler = m.chain(handler)
	}

	if err := handler(c); err != nil {
		m.handleError(c, err)
	}
}

// route finds the handler for an interaction, or nil if there is nothing to do.
func (m *Minidis) route(c *Context) HandlerFunc {
	m.mu.RLock()
	defer m.mu.RUnlock()

	switch c.Interaction.Type {
	case discord.InteractionApplicationCommand:
		data := c.Interaction.ApplicationCommandData()

		// A slash and a context menu command can share a name.
		switch data.CommandType {
		case discord.PrimaryEntryPointApplicationCommand:
			return m.routeEntryPoint(data)
		case discord.UserApplicationCommand:
			return m.routeUser(data)
		case discord.MessageApplicationCommand:
			return m.routeMessage(data)
		default:
			return m.routeSlash(c, data)
		}
	case discord.InteractionApplicationCommandAutocomplete:
		return m.routeAutocomplete(c)
	case discord.InteractionMessageComponent:
		return m.routeComponent(c)
	case discord.InteractionModalSubmit:
		return m.routeModalSubmit(c)
	default:
		return nil
	}
}

// failWith is a handler that reports err, so that it passes through middleware
// and reaches the error handler like any other failure.
func failWith(err error) HandlerFunc {
	return func(*Context) error { return err }
}
