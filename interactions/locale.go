package interactions

import (
	"maps"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/locale"
)

// The keys of the built-in replies, which a catalog can translate. Without
// them, the replies are in English.
const (
	// KeyCooldown is the reply of Cooldown: "You can use this command again
	// %s.", where %s is a relative timestamp such as "in 5 minutes".
	KeyCooldown = "minidis.cooldown"
	// KeyGuildOnly is the reply of RequirePermissions outside a guild:
	// "This only works in a server."
	KeyGuildOnly = "minidis.guild_only"
	// KeyMissingPermissions is the reply of RequirePermissions to a member
	// without the permissions: "You don't have the permissions to use this."
	KeyMissingPermissions = "minidis.missing_permissions"
	// KeyPaginateAuthorOnly is the reply of Paginate to someone else clicking
	// its buttons: "Only <@%s> can turn these pages.", where %s is the user ID.
	KeyPaginateAuthorOnly = "minidis.paginate_author_only"
)

// SetCatalog sets the translations that c.T and the built-in replies, such
// as those of Cooldown, look messages up in, and that SyncCommands fills
// command localizations from. See the locale package.
//
// SyncCommands looks up the name of a command under "command.<name>.name"
// and its description under "command.<name>.description". For subcommands,
// groups, options and choices, their names are added to the path, as in
// "command.settings.notify.channel.description". Localizations set on a
// command take precedence over the catalog.
func (m *Minidis) SetCatalog(c *locale.Catalog) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.catalog = c
}

// Catalog returns the catalog set by SetCatalog, or nil.
func (m *Minidis) Catalog() *locale.Catalog {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.catalog
}

// T returns the message for key from the bot's catalog (see SetCatalog) in
// the user's language, else the guild's, else the catalog's default, else key
// itself, formatted with args like fmt.Sprintf.
func (c *Context) T(key string, args ...any) string {
	return c.translate(key, key, args...)
}

// translate is T with the text to use when the catalog lacks key.
func (c *Context) translate(key, fallback string, args ...any) string {
	locales := []discord.Locale{c.Locale}
	if c.GuildLocale != nil {
		locales = append(locales, *c.GuildLocale)
	}
	if msg, ok := c.catalog.Lookup(key, locales...); ok {
		fallback = msg
	}
	return locale.Format(fallback, args...)
}

// localize fills in the localizations of built commands from the catalog.
// The commands are fresh, but what they point to is the user's, so the maps
// and options it changes are copied.
func localize(cat *locale.Catalog, commands []*discord.ApplicationCommand) {
	if cat == nil {
		return
	}
	for _, cmd := range commands {
		key := "command." + cmd.Name
		cmd.NameLocalizations = withTranslations(cmd.NameLocalizations, cat.Translations(key+".name"))
		// Context menu commands may not have a description.
		if cmd.Description != "" {
			cmd.DescriptionLocalizations = withTranslations(cmd.DescriptionLocalizations, cat.Translations(key+".description"))
		}
		cmd.Options = localizeOptions(cat, key, cmd.Options)
	}
}

func localizeOptions(cat *locale.Catalog, prefix string, options []*discord.ApplicationCommandOption) []*discord.ApplicationCommandOption {
	if len(options) == 0 {
		return options
	}

	out := make([]*discord.ApplicationCommandOption, len(options))
	for i, o := range options {
		key := prefix + "." + o.Name
		opt := *o
		opt.NameLocalizations = merge(o.NameLocalizations, cat.Translations(key+".name"))
		opt.DescriptionLocalizations = merge(o.DescriptionLocalizations, cat.Translations(key+".description"))
		opt.Options = localizeOptions(cat, key, o.Options)

		if len(o.Choices) > 0 {
			opt.Choices = make([]*discord.ApplicationCommandOptionChoice, len(o.Choices))
			for j, ch := range o.Choices {
				choice := *ch
				choice.NameLocalizations = merge(ch.NameLocalizations, cat.Translations(key+"."+ch.Name+".name"))
				opt.Choices[j] = &choice
			}
		}
		out[i] = &opt
	}
	return out
}

// withTranslations is merge for the optional localizations of a command.
func withTranslations(explicit *map[discord.Locale]string, translations map[discord.Locale]string) *map[discord.Locale]string {
	var l map[discord.Locale]string
	if explicit != nil {
		l = *explicit
	}
	return localizations(merge(l, translations))
}

// merge returns the translations with the explicit localizations over them,
// or explicit unchanged if there are none.
func merge(explicit, translations map[discord.Locale]string) map[discord.Locale]string {
	if len(translations) == 0 {
		return explicit
	}
	maps.Copy(translations, explicit) // Translations returns a new map
	return translations
}
