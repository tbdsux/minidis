package minidis

import (
	"github.com/tbdsux/minidis/discord"
)

// Command is a slash command.
//
// A command with Subcommands or Groups only runs those: its own Execute and
// Options are not used by Discord.
type Command struct {
	Name        string
	Description string

	NameLocalizations        map[discord.Locale]string
	DescriptionLocalizations map[discord.Locale]string

	// DefaultMemberPermissions is the permission bits a member needs. Nil lets
	// everyone use the command; discord.Int64(0) makes it admin-only.
	DefaultMemberPermissions *int64

	// Contexts says where a global command can be used. Nil means guilds only;
	// add discord.InteractionContextBotDM to allow DMs with the bot.
	// IntegrationTypes says how the app must be installed; add
	// discord.ApplicationIntegrationUserInstall for a user-installable command.
	// Discord only accepts both on global commands, so they are not sent when
	// syncing to guilds.
	Contexts         []discord.InteractionContextType
	IntegrationTypes []discord.ApplicationIntegrationType

	Options []*discord.ApplicationCommandOption
	Execute func(c *Context) error

	// Autocomplete suggests values for options declared with Autocomplete: true.
	// Middleware does not run for it, and it must respond within 3 seconds.
	Autocomplete func(c *AutocompleteContext) error

	Subcommands []*Subcommand
	Groups      []*SubcommandGroup
}

// Subcommand is a subcommand of a Command or of a SubcommandGroup.
type Subcommand struct {
	Name         string
	Description  string
	Options      []*discord.ApplicationCommandOption
	Execute      func(c *Context) error
	Autocomplete func(c *AutocompleteContext) error // see Command.Autocomplete
}

// SubcommandGroup is a group of subcommands, as in "/role add" where role is
// the group.
type SubcommandGroup struct {
	Name        string
	Description string
	Subcommands []*Subcommand
}

// MessageCommand is a command in the context menu of a message.
type MessageCommand struct {
	Name    string
	Execute func(c *MessageCommandContext) error

	// See Command for these.
	DefaultMemberPermissions *int64
	Contexts                 []discord.InteractionContextType
	IntegrationTypes         []discord.ApplicationIntegrationType
}

// UserCommand is a command in the context menu of a user.
type UserCommand struct {
	Name    string
	Execute func(c *UserCommandContext) error

	// See Command for these.
	DefaultMemberPermissions *int64
	Contexts                 []discord.InteractionContextType
	IntegrationTypes         []discord.ApplicationIntegrationType
}

// AddCommand adds slash commands, replacing any of the same name. Discord
// shows them once they are synced with SyncCommands.
func (m *Minidis) AddCommand(cmds ...*Command) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cmd := range cmds {
		m.commands[cmd.Name] = cmd
	}
}

// AddMessageCommand adds message context menu commands, replacing any of the
// same name.
func (m *Minidis) AddMessageCommand(cmds ...*MessageCommand) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cmd := range cmds {
		m.messageCommands[cmd.Name] = cmd
	}
}

// AddUserCommand adds user context menu commands, replacing any of the same
// name.
func (m *Minidis) AddUserCommand(cmds ...*UserCommand) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, cmd := range cmds {
		m.userCommands[cmd.Name] = cmd
	}
}
