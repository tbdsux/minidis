package minidis

import (
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

func (m *Minidis) executeSlash(s *client.Session, i *discord.Interaction) (error, bool) {
	if cmd, ok := m.Commands[i.ApplicationCommandData().Name]; ok {
		context := m.NewSlashContext(s, i, true)

		if len(i.ApplicationCommandData().Options) > 0 {
			scmd := i.ApplicationCommandData().Options[0]

			switch scmd.Type {
			case discord.ApplicationCommandOptionSubCommandGroup:
				{
					if len(scmd.Options) > 0 {
						if subgroup, ok := cmd.subcommandGroups[scmd.Name]; ok {
							if subgroupCmd, ok := subgroup.subcommands[scmd.Options[0].Name]; ok {
								context.setOptions(scmd.Options[0].Options)

								return subgroupCmd.Execute(context), true
							}
						}
					}

					// it is a subcommand group break it here
					break
				}
			case discord.ApplicationCommandOptionSubCommand:
				{
					if subcmd, ok := cmd.subcommands[scmd.Name]; ok {
						context.setOptions(scmd.Options)

						return subcmd.Execute(context), true
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
