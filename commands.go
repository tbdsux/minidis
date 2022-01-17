package minidis

import (
	"github.com/bwmarrin/discordgo"
)

type SlashCommandProps struct {
	Command     string
	Description string
	Options     []*discordgo.ApplicationCommandOption
	Execute     func(c *SlashContext) error
}

// AddCommand adds a new slash command.
func (m *Minidis) AddCommand(cmd *SlashCommandProps) *SlashCommandProps {
	m.commands[cmd.Command] = cmd

	return cmd
}
