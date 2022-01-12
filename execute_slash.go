package minidis

import "github.com/bwmarrin/discordgo"

func (m *Minidis) executeSlash(s *discordgo.Session, i *discordgo.Interaction) error {
	if cmd, ok := m.commands[i.ApplicationCommandData().Name]; ok {
		context := m.NewSlashContext(s, i)

		return cmd.Execute(context)
	}

	return nil
}
