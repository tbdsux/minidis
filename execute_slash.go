package minidis

import (
	"github.com/bwmarrin/discordgo"
)

func (m *Minidis) executeSlash(s *discordgo.Session, i *discordgo.Interaction) (error, bool) {

	if cmd, ok := m.commands[i.ApplicationCommandData().Name]; ok {
		context := m.NewSlashContext(s, i, true)

		if len(i.ApplicationCommandData().Options) > 0 {
			scmd := i.ApplicationCommandData().Options[0]

			switch scmd.Type {
			case discordgo.ApplicationCommandOptionSubCommandGroup:
				{
					if len(scmd.Options) > 0 {
						if subgroup, ok := cmd.subcommandGroups[scmd.Name]; ok {
							if subgroupCmd, ok := subgroup.subcommands[scmd.Options[0].Name]; ok {

								return subgroupCmd.Execute(context), true
							}
						}
					}

					// it is a subcommand group break it here
					break
				}
			case discordgo.ApplicationCommandOptionSubCommand:
				{
					if scmd, ok := cmd.subcommands[scmd.Name]; ok {
						return scmd.Execute(context), true
					}

					// it is a subcommand break it here
					break
				}
			}
		}

		return cmd.Execute(context), true
	}

	return nil, false
}
