package minidis

import "github.com/bwmarrin/discordgo"

// OnMessageCreate adds a wrapper handler that executes when a message is created.
func (m *Minidis) OnMessageCreate(handler func(s *discordgo.Session, i *discordgo.MessageCreate)) {
	m.MessageCreateHandler = handler
}

// OnReady adds a wrapper handler that executes once the bot is ready.
func (m *Minidis) OnReady(v func(s *discordgo.Session, i *discordgo.Ready)) {
	m.Session.AddHandler(v)
}
