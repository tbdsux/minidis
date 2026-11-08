package minidis

import (
	"github.com/tbdsux/minidis/discord"
)

// SyncCommands registers the added commands, replacing any others. With no
// guild IDs the commands are registered globally.
func (m *Minidis) SyncCommands(guildIDs ...string) error {
	if len(guildIDs) == 0 {
		_, err := m.Session.ApplicationCommandBulkOverwrite(m.AppID, "", m.buildCommands(true))
		return err
	}

	commands := m.buildCommands(false)
	for _, v := range guildIDs {
		if _, err := m.Session.ApplicationCommandBulkOverwrite(m.AppID, v, commands); err != nil {
			return err
		}
	}

	return nil
}

// setGlobalScope sets the fields Discord only accepts on global commands.
func setGlobalScope(cmd *discord.ApplicationCommand, contexts []discord.InteractionContextType, types []discord.ApplicationIntegrationType) {
	if contexts != nil {
		cmd.Contexts = &contexts
	}
	if types != nil {
		cmd.IntegrationTypes = &types
	}
}

func (m *Minidis) buildCommands(global bool) []*discord.ApplicationCommand {
	allCommands := []*discord.ApplicationCommand{}

	// parse message commands
	for _, v := range m.MessageCommands {
		cmd := &discord.ApplicationCommand{
			Name:                     v.Name,
			Type:                     discord.MessageApplicationCommand,
			DefaultMemberPermissions: v.DefaultMemberPermissions,
		}
		if global {
			setGlobalScope(cmd, v.Contexts, v.IntegrationTypes)
		}

		allCommands = append(allCommands, cmd)
	}

	// parse user commands
	for _, v := range m.UserCommands {
		cmd := &discord.ApplicationCommand{
			Name:                     v.Name,
			Type:                     discord.UserApplicationCommand,
			DefaultMemberPermissions: v.DefaultMemberPermissions,
		}
		if global {
			setGlobalScope(cmd, v.Contexts, v.IntegrationTypes)
		}

		allCommands = append(allCommands, cmd)
	}

	// parse slash commands
	for _, v := range m.Commands {
		cmd := &discord.ApplicationCommand{
			Name:                     v.Name,
			Description:              v.Description,
			NameLocalizations:        v.NameLocalizations,
			DescriptionLocalizations: v.DescriptionLocalizations,
			DefaultMemberPermissions: v.DefaultMemberPermissions,
			Options:                  v.Options,
		}

		if global {
			contexts := v.Contexts
			if contexts == nil {
				contexts = []discord.InteractionContextType{discord.InteractionContextGuild}
				if v.DmPermission {
					contexts = append(contexts, discord.InteractionContextBotDM)
				}
			}
			setGlobalScope(cmd, contexts, v.IntegrationTypes)
		}

		for _, g := range v.subcommandGroups {
			group := &discord.ApplicationCommandOption{
				Name:        g.Name,
				Description: g.Description,
				Type:        discord.ApplicationCommandOptionSubCommandGroup,
			}

			for _, scmd := range g.subcommands {
				group.Options = append(group.Options, &discord.ApplicationCommandOption{
					Name:        scmd.Name,
					Description: scmd.Description,
					Type:        discord.ApplicationCommandOptionSubCommand,
					Options:     scmd.Options,
				})
			}

			cmd.Options = append(cmd.Options, group)
		}

		for _, scmd := range v.subcommands {
			cmd.Options = append(cmd.Options, &discord.ApplicationCommandOption{
				Name:        scmd.Name,
				Description: scmd.Description,
				Type:        discord.ApplicationCommandOptionSubCommand,
				Options:     scmd.Options,
			})
		}

		allCommands = append(allCommands, cmd)
	}

	return allCommands
}
