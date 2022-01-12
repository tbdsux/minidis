package minidis

import "github.com/bwmarrin/discordgo"

// Sync the commands to these guilds.
func (m *Minidis) SyncToGuilds(guildIDs ...string) {
	m.guilds = guildIDs
}

func (m *Minidis) syncCommands(guildIDs []string) error {
	allCommands := []*discordgo.ApplicationCommand{}

	for _, v := range m.commands {
		allCommands = append(allCommands, &discordgo.ApplicationCommand{
			Name:        v.Command,
			Description: v.Description,
			Options:     v.Options,
		})
	}

	if len(guildIDs) == 0 {
		return m.setupCommands("", allCommands)

	}

	for _, v := range guildIDs {
		if err := m.setupCommands(v, allCommands); err != nil {
			return err
		}
	}

	return nil
}

func inCommands(commands []*discordgo.ApplicationCommand, cmd string) bool {
	for _, v := range commands {
		if v.Name == cmd {
			return true
		}
	}

	return false
}

func (m *Minidis) setupCommands(guildID string, commands []*discordgo.ApplicationCommand) error {
	guildCommands, err := m.session.ApplicationCommands(m.AppID, guildID)
	if err != nil {
		return err
	}

	oldCommands := []string{}
	for _, v := range guildCommands {
		if !inCommands(commands, v.Name) {
			oldCommands = append(oldCommands, v.ID)
		}
	}

	for _, v := range oldCommands {
		if err = m.session.ApplicationCommandDelete(m.AppID, guildID, v); err != nil {
			return err
		}
	}

	_, err = m.session.ApplicationCommandBulkOverwrite(m.AppID, guildID, commands)

	return err
}
