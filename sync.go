package minidis

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"

	"github.com/tbdsux/minidis/discord"
)

// SyncCommands registers the added commands with Discord, replacing any others.
// With no guild IDs the commands are registered globally. It can be called
// before Open.
func (m *Minidis) SyncCommands(guildIDs ...string) error {
	if len(guildIDs) == 0 {
		return m.overwriteCommands("", m.buildCommands(true))
	}

	commands := m.buildCommands(false)
	for _, id := range guildIDs {
		if err := m.overwriteCommands(id, commands); err != nil {
			return err
		}
	}
	return nil
}

// ClearCommands removes the application commands from the given guilds.
// If no guilds are given, the global commands are removed instead, except
// the entry point command (see SetEntryPoint).
func (m *Minidis) ClearCommands(guildIDs ...string) error {
	if len(guildIDs) == 0 {
		return m.overwriteCommands("", nil)
	}

	for _, id := range guildIDs {
		if err := m.overwriteCommands(id, nil); err != nil {
			return err
		}
	}
	return nil
}

func (m *Minidis) overwriteCommands(guildID string, commands []*discord.ApplicationCommand) error {
	appID, err := m.applicationID()
	if err != nil {
		return err
	}

	// Discord refuses to remove the entry point command in a bulk update, and
	// creates one itself for apps with an Activity, so keep it unless replaced.
	if guildID == "" && !slices.ContainsFunc(commands, isEntryPoint) {
		existing, err := m.globalEntryPoint(appID)
		if err != nil {
			return err
		}
		if existing != nil {
			commands = append(slices.Clone(commands), existing)
		}
	}

	_, err = m.Session.ApplicationCommandBulkOverwrite(appID, guildID, commands)
	return err
}

func isEntryPoint(cmd *discord.ApplicationCommand) bool {
	return cmd.Type == discord.PrimaryEntryPointApplicationCommand
}

// globalEntryPoint returns the entry point command Discord has, or nil.
func (m *Minidis) globalEntryPoint(appID string) (*discord.ApplicationCommand, error) {
	var commands []*discord.ApplicationCommand
	path := "/applications/" + appID + "/commands?with_localizations=true"
	if err := m.Session.Request(context.Background(), http.MethodGet, path, nil, &commands); err != nil {
		return nil, fmt.Errorf("minidis: looking for the entry point command: %w", err)
	}

	if i := slices.IndexFunc(commands, isEntryPoint); i >= 0 {
		return commands[i], nil
	}
	return nil, nil
}

// applicationID returns the bot's application ID, asking Discord for it if the
// session is not open.
func (m *Minidis) applicationID() (string, error) {
	if id := m.Session.ApplicationID(); id != "" {
		return id, nil
	}

	m.appIDMu.Lock()
	defer m.appIDMu.Unlock()

	if m.appID != "" {
		return m.appID, nil
	}

	var app struct {
		ID string `json:"id"`
	}
	if err := m.Session.Request(context.Background(), http.MethodGet, "/applications/@me", nil, &app); err != nil {
		return "", fmt.Errorf("minidis: looking up the application ID: %w", err)
	}

	m.appID = app.ID
	return app.ID, nil
}

// buildCommands converts the added commands, sorted by name so that the
// result is the same each time.
func (m *Minidis) buildCommands(global bool) []*discord.ApplicationCommand {
	m.mu.RLock()
	defer m.mu.RUnlock()

	all := []*discord.ApplicationCommand{}

	for _, name := range slices.Sorted(maps.Keys(m.commands)) {
		all = append(all, m.commands[name].build(global))
	}

	for _, name := range slices.Sorted(maps.Keys(m.messageCommands)) {
		all = append(all, m.messageCommands[name].build(global))
	}

	for _, name := range slices.Sorted(maps.Keys(m.userCommands)) {
		all = append(all, m.userCommands[name].build(global))
	}

	// Entry points can only be global.
	if global && m.entryPoint != nil {
		all = append(all, m.entryPoint.build())
	}

	return all
}

func (cmd *Command) build(global bool) *discord.ApplicationCommand {
	res := &discord.ApplicationCommand{
		Name:                     cmd.Name,
		Description:              cmd.Description,
		NameLocalizations:        localizations(cmd.NameLocalizations),
		DescriptionLocalizations: localizations(cmd.DescriptionLocalizations),
		DefaultMemberPermissions: cmd.DefaultMemberPermissions,
		NSFW:                     nsfw(cmd.NSFW),
		Options:                  slices.Clone(cmd.Options), // appended to below
	}

	for _, sub := range cmd.Subcommands {
		res.Options = append(res.Options, sub.option())
	}

	for _, g := range cmd.Groups {
		group := &discord.ApplicationCommandOption{
			Name:                     g.Name,
			Description:              g.Description,
			NameLocalizations:        g.NameLocalizations,
			DescriptionLocalizations: g.DescriptionLocalizations,
			Type:                     discord.ApplicationCommandOptionSubCommandGroup,
		}
		for _, sub := range g.Subcommands {
			group.Options = append(group.Options, sub.option())
		}
		res.Options = append(res.Options, group)
	}

	if global {
		setGlobalScope(res, cmd.Contexts, cmd.IntegrationTypes)
	}
	return res
}

func (s *Subcommand) option() *discord.ApplicationCommandOption {
	return &discord.ApplicationCommandOption{
		Name:                     s.Name,
		Description:              s.Description,
		NameLocalizations:        s.NameLocalizations,
		DescriptionLocalizations: s.DescriptionLocalizations,
		Type:                     discord.ApplicationCommandOptionSubCommand,
		Options:                  s.Options,
	}
}

func (cmd *MessageCommand) build(global bool) *discord.ApplicationCommand {
	res := &discord.ApplicationCommand{
		Name:                     cmd.Name,
		NameLocalizations:        localizations(cmd.NameLocalizations),
		Type:                     discord.MessageApplicationCommand,
		DefaultMemberPermissions: cmd.DefaultMemberPermissions,
		NSFW:                     nsfw(cmd.NSFW),
	}
	if global {
		setGlobalScope(res, cmd.Contexts, cmd.IntegrationTypes)
	}
	return res
}

func (cmd *UserCommand) build(global bool) *discord.ApplicationCommand {
	res := &discord.ApplicationCommand{
		Name:                     cmd.Name,
		NameLocalizations:        localizations(cmd.NameLocalizations),
		Type:                     discord.UserApplicationCommand,
		DefaultMemberPermissions: cmd.DefaultMemberPermissions,
		NSFW:                     nsfw(cmd.NSFW),
	}
	if global {
		setGlobalScope(res, cmd.Contexts, cmd.IntegrationTypes)
	}
	return res
}

func (cmd *EntryPointCommand) build() *discord.ApplicationCommand {
	res := &discord.ApplicationCommand{
		Name:                     cmd.Name,
		Description:              cmd.Description,
		NameLocalizations:        localizations(cmd.NameLocalizations),
		DescriptionLocalizations: localizations(cmd.DescriptionLocalizations),
		Type:                     discord.PrimaryEntryPointApplicationCommand,
		Handler:                  cmd.Handler,
		DefaultMemberPermissions: cmd.DefaultMemberPermissions,
		NSFW:                     nsfw(cmd.NSFW),
	}
	setGlobalScope(res, cmd.Contexts, cmd.IntegrationTypes)
	return res
}

// nsfw leaves the field out unless set, as Discord defaults it to false.
func nsfw(v bool) *bool {
	if !v {
		return nil
	}
	return discord.Bool(true)
}

// setGlobalScope sets the fields Discord only accepts on global commands. Nil
// contexts mean guilds only.
func setGlobalScope(cmd *discord.ApplicationCommand, contexts []discord.InteractionContextType, types []discord.ApplicationIntegrationType) {
	if contexts == nil {
		contexts = []discord.InteractionContextType{discord.InteractionContextGuild}
	}
	cmd.Contexts = &contexts

	if types != nil {
		cmd.IntegrationTypes = &types
	}
}

func localizations(l map[discord.Locale]string) *map[discord.Locale]string {
	if l == nil {
		return nil
	}
	return &l
}
