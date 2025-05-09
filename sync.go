package minidis

import (
	"github.com/bwmarrin/discordgo"
)

func (m *Minidis) SyncCommands(guildIDs ...string) error {
	allCommands := []*discordgo.ApplicationCommand{}

	// parse message commands
	for _, v := range m.MessageCommands {
		cmd := &discordgo.ApplicationCommand{
			Name: v.Name,
			Type: discordgo.MessageApplicationCommand,
		}

		allCommands = append(allCommands, cmd)
	}

	// parse user commands
	for _, v := range m.UserCommands {
		cmd := &discordgo.ApplicationCommand{
			Name: v.Command,
			Type: discordgo.UserApplicationCommand,
		}

		allCommands = append(allCommands, cmd)
	}

	// parse slash commands
	for _, v := range m.Commands {
		cmd := &discordgo.ApplicationCommand{
			Name:                     v.Name,
			Description:              v.Description,
			NameLocalizations:        v.NameLocalizations,
			DescriptionLocalizations: v.DescriptionLocalizations,
			DefaultMemberPermissions: &v.DefaultMemberPermissions,
			DMPermission:             &v.DmPermission,
			Options:                  v.Options,
		}

		for _, g := range v.subcommandGroups {
			group := &discordgo.ApplicationCommandOption{
				Name:        g.Name,
				Description: g.Description,
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
			}

			for _, scmd := range g.subcommands {
				group.Options = append(group.Options, &discordgo.ApplicationCommandOption{
					Name:        scmd.Name,
					Description: scmd.Description,
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				})
			}

			cmd.Options = append(cmd.Options, group)
		}

		for _, scmd := range v.subcommands {
			cmd.Options = append(cmd.Options, &discordgo.ApplicationCommandOption{
				Name:        scmd.Name,
				Description: scmd.Description,
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			})
		}

		allCommands = append(allCommands, cmd)
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
	guildCommands, err := m.Session.ApplicationCommands(m.AppID, guildID)
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
		if err = m.Session.ApplicationCommandDelete(m.AppID, guildID, v); err != nil {
			return err
		}
	}

	_, err = m.Session.ApplicationCommandBulkOverwrite(m.AppID, guildID, commands)

	return err
}
