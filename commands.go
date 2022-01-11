package minidis

import "github.com/bwmarrin/discordgo"

type SlashCommandProps struct {
	Command     string
	Description string
	Options     []*discordgo.ApplicationCommandOption
}

func (m *Minidis) AddCommand(cmd SlashCommandProps) {
	m.commands = append(m.commands, cmd)
}
