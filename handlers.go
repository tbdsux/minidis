package minidis

import "github.com/bwmarrin/discordgo"

// OnReady adds a wrapper handler that executes once the bot is ready.
func (m *Minidis) OnReady(v func(s *discordgo.Session, i *discordgo.Ready)) {
	m.session.AddHandler(v)
}
