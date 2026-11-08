package minidis

import "github.com/tbdsux/minidis/discord"

// SetIntents sets the required or used intents by the bot.
func (m *Minidis) SetIntents(intents discord.Intent) {
	m.Session.Intents = intents
}
