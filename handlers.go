package minidis

import "github.com/bwmarrin/discordgo"

// OnInteractionCreate adds a wrapper handler that executes when an interaction is created.
func (m *Minidis) OnMessageCreate(handler func(s *discordgo.Session, i *discordgo.MessageCreate)) {
	m.messageCreateHandler = handler
}

// OnReady adds a wrapper handler that executes once the bot is ready.
func (m *Minidis) OnReady(v func(s *discordgo.Session, i *discordgo.Ready)) {
	m.session.AddHandler(v)
}

// OnClose adds a custom function that will be called when exit is called.
func (m *Minidis) OnClose(v func(s *discordgo.Session)) {
	m.customHandlers.onClose = v
}

// OnBeforeStart adds a custom function that will be called before syncing the
// application commands and running the bot.
func (m *Minidis) OnBeforeStart(v func(s *discordgo.Session)) {
	m.customHandlers.onBeforeStart = v
}
