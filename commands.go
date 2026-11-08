package minidis

import (
	"github.com/tbdsux/minidis/discord"
)

type SlashCommandProps struct {
	Name        string // Name of the command
	Description string // Description of the command

	NameLocalizations        *map[discord.Locale]string
	DescriptionLocalizations *map[discord.Locale]string

	// DefaultMemberPermissions is the permission bits a member needs. Nil lets
	// everyone use the command; discord.Int64(0) makes it admin-only.
	DefaultMemberPermissions *int64
	DmPermission             bool

	Options []*discord.ApplicationCommandOption
	Execute func(c *SlashContext) error

	// Autocomplete suggests values for options declared with Autocomplete: true.
	// Middleware does not run for it, and it must respond within 3 seconds.
	Autocomplete func(c *AutocompleteContext) error

	subcommandGroups map[string]*SlashSubcommandGroupProps
	subcommands      map[string]*SlashSubcommandProps
}

type SlashSubcommandProps struct {
	Name         string
	Description  string
	Options      []*discord.ApplicationCommandOption
	Execute      func(c *SlashContext) error
	Autocomplete func(c *AutocompleteContext) error // see SlashCommandProps.Autocomplete
}

type SlashSubcommandGroupProps struct {
	Name        string
	Description string
	subcommands map[string]*SlashSubcommandProps
}

// AddCommand adds a new slash command.
func (m *Minidis) AddCommand(cmd *SlashCommandProps) *SlashCommandProps {
	m.Commands[cmd.Name] = cmd

	cmd.subcommandGroups = map[string]*SlashSubcommandGroupProps{}
	cmd.subcommands = map[string]*SlashSubcommandProps{}

	return cmd
}

type MessageCommandProps struct {
	Name    string
	Execute func(c *MessageCommandContext) error
}

// AddMessageCommand adds a new message command.
func (m *Minidis) AddMessageCommand(cmd *MessageCommandProps) {
	m.MessageCommands[cmd.Name] = cmd
}

type UserCommandProps struct {
	Name    string
	Execute func(c *UserCommandContext) error
}

// AddUserCommand adds a new user command.
func (m *Minidis) AddUserCommand(cmd *UserCommandProps) {
	m.UserCommands[cmd.Name] = cmd
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
	s.subcommands[cmd.Name] = cmd
}

// AddSubcommand adds a new group for sub commands for the parent command.
// Note: this will make your parent command not execute.
func (s *SlashCommandProps) AddSubcommandGroup(group *SlashSubcommandGroupProps) *SlashSubcommandGroupProps {
	s.subcommandGroups[group.Name] = group

	group.subcommands = map[string]*SlashSubcommandProps{}

	return group
}

// AddSubcommand adds a new sub command for the subcommmand group.
// Note: this will make your parent command not execute.
func (s *SlashSubcommandGroupProps) AddSubcommand(cmd *SlashSubcommandProps) {
	s.subcommands[cmd.Name] = cmd
}
