package minidis

import "github.com/bwmarrin/discordgo"

// SetIntents sets the required or used intents by the bot.
func (m *Minidis) SetIntents(intents discordgo.Intent) {
	m.session.Identify.Intents = intents
}
