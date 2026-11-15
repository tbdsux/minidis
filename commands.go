package minidis

import (
	"github.com/tbdsux/minidis/discord"
)

// Command is a slash command.
//
// A command with Subcommands or Groups only runs those: Discord rejects it
// with Options of its own, and its Execute and Autocomplete are not used.
type Command struct {
	Name        string
	Description string

	NameLocalizations        map[discord.Locale]string
	DescriptionLocalizations map[discord.Locale]string

	// DefaultMemberPermissions is the permission bits a member needs. Nil lets
	// everyone use the command; discord.Int64(0) makes it admin-only.
	DefaultMemberPermissions *int64

	// NSFW commands are only shown in age-restricted channels.
	NSFW bool

	// Contexts says where a global command can be used. Nil means guilds only,
	// as member permissions do not apply elsewhere, except for commands with
	// discord.ApplicationIntegrationUserInstall, where nil leaves Discord's
	// default of everywhere. Add discord.InteractionContextBotDM to allow DMs
	// with the bot.
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
	Name                     string
	Description              string
	NameLocalizations        map[discord.Locale]string
	DescriptionLocalizations map[discord.Locale]string

	Options      []*discord.ApplicationCommandOption
	Execute      func(c *Context) error
	Autocomplete func(c *AutocompleteContext) error // see Command.Autocomplete
}

// SubcommandGroup is a group of subcommands, as in "/role add" where role is
// the group.
type SubcommandGroup struct {
	Name                     string
	Description              string
	NameLocalizations        map[discord.Locale]string
	DescriptionLocalizations map[discord.Locale]string

	Subcommands []*Subcommand
}

// MessageCommand is a command in the context menu of a message.
type MessageCommand struct {
	Name              string
	NameLocalizations map[discord.Locale]string
	Execute           func(c *MessageCommandContext) error

	// See Command for these.
	DefaultMemberPermissions *int64
	NSFW                     bool
	Contexts                 []discord.InteractionContextType
	IntegrationTypes         []discord.ApplicationIntegrationType
}

// UserCommand is a command in the context menu of a user.
type UserCommand struct {
	Name              string
	NameLocalizations map[discord.Locale]string
	Execute           func(c *UserCommandContext) error

	// See Command for these.
	DefaultMemberPermissions *int64
	NSFW                     bool
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

// EntryPointCommand is the command that launches the app's Activity, shown in
// the App Launcher. Discord creates a default one when Activities are enabled.
type EntryPointCommand struct {
	Name                     string
	Description              string
	NameLocalizations        map[discord.Locale]string
	DescriptionLocalizations map[discord.Locale]string

	// Handler says who launches the Activity. With
	// discord.EntryPointHandlerDiscordLaunchActivity, Discord launches it and
	// Execute is not used. With discord.EntryPointHandlerApp, Execute runs and
	// responds, usually with c.LaunchActivity. Zero picks the app when Execute
	// is set and Discord otherwise.
	Handler discord.EntryPointHandlerType
	Execute func(c *Context) error

	// See Command for these, except that nil Contexts leaves Discord's
	// default of everywhere.
	DefaultMemberPermissions *int64
	NSFW                     bool
	Contexts                 []discord.InteractionContextType
	IntegrationTypes         []discord.ApplicationIntegrationType
}

// SetEntryPoint sets the app's entry point command, which is global only. It
// replaces the one Discord has, whatever its name. Without one, syncing global
// commands keeps the entry point Discord already has, because Discord refuses
// to remove it in a bulk update.
func (m *Minidis) SetEntryPoint(cmd *EntryPointCommand) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entryPoint = cmd
}
