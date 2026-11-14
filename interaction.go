package minidis

import (
	"context"
	"runtime/debug"
	"time"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

const (
	// interactionLifetime is how long Discord accepts responses to an
	// interaction.
	interactionLifetime = 15 * time.Minute

	// responseWindow is how long Discord waits for the initial response.
	responseWindow = 3 * time.Second
)

func (m *Minidis) handleInteraction(s *client.Session, i *discord.InteractionCreate) {
	m.runInteraction(m.current().ctx, s, i.Interaction, nil)
}

// runInteraction handles an interaction however it was received, within the
// context of the bot's run. ack, if not nil, is called once the interaction
// has been responded to.
func (m *Minidis) runInteraction(run context.Context, s *client.Session, i *discord.Interaction, ack func()) {
	ctx, cancel := context.WithTimeout(run, interactionLifetime)
	defer cancel()

	ctx, cancelCause := context.WithCancelCause(ctx)
	defer cancelCause(nil)

	c := newContext(ctx, run, s, i, m.botUser(s))
	c.reply.ack = ack

	// Discord drops an interaction that is not responded to in time, so stop
	// the handler's work rather than let it respond in vain.
	cutoff := time.AfterFunc(responseWindow, func() {
		if c.reply.state.CompareAndSwap(pending, tooLate) {
			cancelCause(ErrNoResponse)
		}
	})
	defer cutoff.Stop()

	defer func() {
		if r := recover(); r != nil {
			m.handleError(c.forErrors(), &PanicError{Value: r, Stack: debug.Stack()})
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
		m.handleError(c.forErrors(), err)
	}
}

// botUser returns the bot's user: from the gateway session when it is open,
// otherwise as looked up by HTTPHandler.
func (m *Minidis) botUser(s *client.Session) *discord.User {
	if u := s.User(); u != nil {
		return u
	}
	return m.httpBot.Load()
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
