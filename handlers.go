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
