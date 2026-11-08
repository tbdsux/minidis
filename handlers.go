package minidis

import (
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

// OnMessageCreate adds a wrapper handler that executes when a message is created.
func (m *Minidis) OnMessageCreate(handler func(s *client.Session, i *discord.MessageCreate)) {
	m.MessageCreateHandler = handler
}

// OnReady adds a wrapper handler that executes once the bot is ready.
func (m *Minidis) OnReady(v func(s *client.Session, i *discord.Ready)) {
	m.Session.OnReady(v)
}

// SetPresence sets the bot's status and activities. It can be called before
// OpenSession, and is kept across reconnects.
func (m *Minidis) SetPresence(status discord.Status, activities ...*discord.Activity) error {
	return m.Session.UpdatePresence(&discord.PresenceUpdate{Status: status, Activities: activities})
}
