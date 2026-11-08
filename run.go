package minidis

import (
	"runtime/debug"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

// Run registers the command handler if OpenSession has not already done so. It
// does not block, and it is safe to call more than once.
func (m *Minidis) Run() {
	run(m)
}

// Execute the bot.
// It is similar to `Run()` function of `Minidis` struct.
func Execute(bot *Minidis) {
	run(bot)
}

// OpenSession registers the command handler and connects to Discord.
func (m *Minidis) OpenSession() error {
	run(m)

	if err := m.Session.Open(); err != nil {
		return err
	}

	m.AppID = m.Session.ApplicationID()

	return nil
}

// CloseSession disconnects from Discord and gives running handlers up to 10
// seconds to finish. Use Session.Shutdown to choose how long to wait.
func (m *Minidis) CloseSession() error {
	return m.Session.Close()
}

// main bot command handler
func run(m *Minidis) {
	m.registerOnce.Do(func() {
		m.Session.OnMessageCreate(m.handleOnMessageCreate)
		m.Session.OnInteractionCreate(m.handleInteraction)
	})
}

func (m *Minidis) handleInteraction(s *client.Session, i *discord.InteractionCreate) {
	isCommand := i.Type == discord.InteractionApplicationCommand ||
		i.Type == discord.InteractionApplicationCommandAutocomplete
	c := m.NewSlashContext(s, i.Interaction, isCommand)

	defer func() {
		if r := recover(); r != nil {
			m.handleError(c, &PanicError{Value: r, Stack: debug.Stack()})
		}
	}()

	if i.Type == discord.InteractionApplicationCommandAutocomplete {
		if err := m.executeAutocomplete(c); err != nil {
			m.handleError(c, err)
		}
		return
	}

	handler := m.route(c)
	if handler == nil {
		return
	}

	if err := m.chain(handler)(c); err != nil {
		m.handleError(c, err)
	}
}

// route finds the handler for an interaction, or nil if there is nothing to do.
func (m *Minidis) route(c *SlashContext) HandlerFunc {
	switch c.Interaction.Type {
	case discord.InteractionApplicationCommand:
		data := c.Interaction.ApplicationCommandData()

		// A slash and a context menu command can share a name.
		switch data.CommandType {
		case discord.UserApplicationCommand:
			return m.routeUser(data)
		case discord.MessageApplicationCommand:
			return m.routeMessage(data)
		default:
			return m.routeSlash(c, data)
		}
	case discord.InteractionMessageComponent:
		return m.routeComponent(c)
	case discord.InteractionModalSubmit:
		return m.routeModalSubmit(c)
	default:
		return nil
	}
}
