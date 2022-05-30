package minidis

import (
	"github.com/bwmarrin/discordgo"
)

type SlashCommandProps struct {
	Command          string
	Description      string
	Options          []*discordgo.ApplicationCommandOption
	Execute          func(c *SlashContext) error
	subcommandGroups map[string]*SlashSubcommandGroupProps
	subcommands      map[string]*SlashSubcommandProps
}

type SlashSubcommandProps struct {
	Command     string
	Description string
	Options     []*discordgo.ApplicationCommandOption
	Execute     func(c *SlashContext) error
}

type SlashSubcommandGroupProps struct {
	Command     string
	Description string
	subcommands map[string]*SlashSubcommandProps
}

// AddCommand adds a new slash command.
func (m *Minidis) AddCommand(cmd *SlashCommandProps) *SlashCommandProps {
	m.commands[cmd.Command] = cmd

	cmd.subcommandGroups = map[string]*SlashSubcommandGroupProps{}
	cmd.subcommands = map[string]*SlashSubcommandProps{}

	return cmd
}

type MessageCommandProps struct {
	Command string
	Execute func(c *MessageCommandContext) error
}

// AddMessageCommand adds a new message command.
func (m *Minidis) AddMessageCommand(cmd *MessageCommandProps) {
	m.messageCommands[cmd.Command] = cmd
}

type UserCommandProps struct {
	Command string
	Execute func(c *UserCommandContext) error
}

// AddUserCommand adds a new user command.
func (m *Minidis) AddUserCommand(cmd *UserCommandProps) {
	m.userCommands[cmd.Command] = cmd
}

// RegisterCommands is used for adding multiple already defined commands.
// It does not return anything. It just wrap the `AddCommand` function.
func (m *Minidis) RegisterCommands(cmds ...*SlashCommandProps) {
	for _, v := range cmds {
		m.AddCommand(v)
	}
}

// AddSubcommand adds a new sub command for the parent command.
// Note: this will make your parent command not execute.
func (s *SlashCommandProps) AddSubcommand(cmd *SlashSubcommandProps) {
	s.subcommands[cmd.Command] = cmd
}

// AddSubcommand adds a new group for sub commands for the parent command.
// Note: this will make your parent command not execute.
func (s *SlashCommandProps) AddSubcommandGroup(group *SlashSubcommandGroupProps) *SlashSubcommandGroupProps {
	s.subcommandGroups[group.Command] = group

	group.subcommands = map[string]*SlashSubcommandProps{}

	return group
}

// AddSubcommand adds a new sub command for the subcommmand group.
// Note: this will make your parent command not execute.
func (s *SlashSubcommandGroupProps) AddSubcommand(cmd *SlashSubcommandProps) {
	s.subcommands[cmd.Command] = cmd
}
