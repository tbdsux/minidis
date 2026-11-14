package minidis

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/tbdsux/minidis/discord"
)

// SyncCommands registers the added commands with Discord, replacing any others.
// Called with no guild IDs, it registers them globally. Empty IDs are ignored,
// but IDs that are all empty are an error, so that an unset variable does not
// replace the global commands by mistake. It can be called before Open.
func (m *Minidis) SyncCommands(ctx context.Context, guildIDs ...string) error {
	if err := m.validateCommands(); err != nil {
		return err
	}

	if len(guildIDs) == 0 {
		return m.overwriteCommands(ctx, "", m.buildCommands(true))
	}

	guildIDs = nonEmpty(guildIDs)
	if len(guildIDs) == 0 {
		return errors.New("minidis: SyncCommands was given only empty guild IDs; call it with none to register the commands globally")
	}

	commands := m.buildCommands(false)
	for _, id := range guildIDs {
		if err := m.overwriteCommands(ctx, id, commands); err != nil {
			return err
		}
	}
	return nil
}

// ClearCommands removes the application commands from the given guilds.
// Called with no guilds, it removes the global commands instead, except the
// entry point command (see SetEntryPoint). Empty IDs are ignored, but IDs that
// are all empty are an error rather than a request to clear the global ones.
func (m *Minidis) ClearCommands(ctx context.Context, guildIDs ...string) error {
	if len(guildIDs) == 0 {
		return m.overwriteCommands(ctx, "", nil)
	}

	guildIDs = nonEmpty(guildIDs)
	if len(guildIDs) == 0 {
		return errors.New("minidis: ClearCommands was given only empty guild IDs; call it with none to clear the global commands")
	}

	for _, id := range guildIDs {
		if err := m.overwriteCommands(ctx, id, nil); err != nil {
			return err
		}
	}
	return nil
}

// nonEmpty returns the IDs with surrounding spaces removed, without empty ones.
func nonEmpty(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// validateCommands catches what Discord would reject, or what would never
// run, for every command at once, with clearer messages.
func (m *Minidis) validateCommands() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errs []error
	report := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("minidis: "+format, args...))
	}

	for _, name := range slices.Sorted(maps.Keys(m.commands)) {
		cmd := m.commands[name]

		if !cmd.hasSubcommands() {
			checkOptions(report, name, cmd.Options, cmd.Autocomplete != nil)
			continue
		}

		if len(cmd.Options) > 0 {
			report("command %q has both Options and subcommands; move the options into the subcommands", name)
		}
		if cmd.Execute != nil || cmd.Autocomplete != nil {
			report("command %q has subcommands, so its own Execute and Autocomplete never run; set them on the subcommands", name)
		}
		for _, sub := range cmd.Subcommands {
			checkOptions(report, name+" "+sub.Name, sub.Options, sub.Autocomplete != nil)
		}
		for _, g := range cmd.Groups {
			for _, sub := range g.Subcommands {
				checkOptions(report, name+" "+g.Name+" "+sub.Name, sub.Options, sub.Autocomplete != nil)
			}
		}
	}
	return errors.Join(errs...)
}

// checkOptions reports autocomplete options that nothing answers, and options
// with both choices and autocomplete, which Discord rejects.
func checkOptions(report func(string, ...any), command string, options []*discord.ApplicationCommandOption, hasAutocomplete bool) {
	for _, o := range options {
		switch {
		case o.Autocomplete && len(o.Choices) > 0:
			report("option %q of %q has both Choices and Autocomplete", o.Name, command)
		case o.Autocomplete && !hasAutocomplete:
			report("option %q of %q has Autocomplete but the command has no Autocomplete handler", o.Name, command)
		}
	}
}

func (m *Minidis) overwriteCommands(ctx context.Context, guildID string, commands []*discord.ApplicationCommand) error {
	appID, err := m.applicationID(ctx)
	if err != nil {
		return err
	}

	// Discord creates an entry point command for apps with an Activity, allows
	// only one, and refuses to remove it in a bulk update. So keep it, or
	// update it in place with the one set by SetEntryPoint.
	if guildID == "" {
		existing, err := m.globalEntryPoint(ctx, appID)
		if err != nil {
			return err
		}
		if existing != nil {
			commands = slices.Clone(commands)
			if i := slices.IndexFunc(commands, isEntryPoint); i >= 0 {
				replacement := *commands[i]
				replacement.ID = existing.ID
				commands[i] = &replacement
			} else {
				commands = append(commands, existing)
			}
		}
	}

	_, err = m.Session.ApplicationCommandBulkOverwrite(ctx, appID, guildID, commands)
	return err
}

func isEntryPoint(cmd *discord.ApplicationCommand) bool {
	return cmd.Type == discord.PrimaryEntryPointApplicationCommand
}

// globalEntryPoint returns the entry point command Discord has, or nil. It
// carries only the fields a bulk overwrite takes, so that it can be sent back.
func (m *Minidis) globalEntryPoint(ctx context.Context, appID string) (*discord.ApplicationCommand, error) {
	var commands []*discord.ApplicationCommand
	path := "/applications/" + appID + "/commands?with_localizations=true"
	if err := m.Session.Request(ctx, http.MethodGet, path, nil, &commands); err != nil {
		return nil, fmt.Errorf("minidis: looking for the entry point command: %w", err)
	}

	i := slices.IndexFunc(commands, isEntryPoint)
	if i < 0 {
		return nil, nil
	}

	c := commands[i]
	return &discord.ApplicationCommand{
		ID:                       c.ID,
		Type:                     c.Type,
		Name:                     c.Name,
		NameLocalizations:        c.NameLocalizations,
		Description:              c.Description,
		DescriptionLocalizations: c.DescriptionLocalizations,
		DefaultMemberPermissions: c.DefaultMemberPermissions,
		NSFW:                     c.NSFW,
		Contexts:                 c.Contexts,
		IntegrationTypes:         c.IntegrationTypes,
		Handler:                  c.Handler,
	}, nil
}

// applicationID returns the bot's application ID, asking Discord for it if the
// session is not open.
func (m *Minidis) applicationID(ctx context.Context) (string, error) {
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
	if err := m.Session.Request(ctx, http.MethodGet, "/applications/@me", nil, &app); err != nil {
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
	if res.Handler == 0 {
		res.Handler = discord.EntryPointHandlerDiscordLaunchActivity
		if cmd.Execute != nil {
			res.Handler = discord.EntryPointHandlerApp
		}
	}

	// Activities are made for DMs and group chats as much as for guilds, so
	// nil contexts keep Discord's default of everywhere.
	if cmd.Contexts != nil {
		res.Contexts = &cmd.Contexts
	}
	if cmd.IntegrationTypes != nil {
		res.IntegrationTypes = &cmd.IntegrationTypes
	}
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
// contexts mean guilds only, where member permissions apply, except for
// user-installable commands: those are mostly used outside guilds, so nil
// keeps Discord's default of everywhere.
func setGlobalScope(cmd *discord.ApplicationCommand, contexts []discord.InteractionContextType, types []discord.ApplicationIntegrationType) {
	if contexts == nil && !slices.Contains(types, discord.ApplicationIntegrationUserInstall) {
		contexts = []discord.InteractionContextType{discord.InteractionContextGuild}
	}
	if contexts != nil {
		cmd.Contexts = &contexts
	}

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
