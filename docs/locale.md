# Translations

The `locale` package holds a bot's messages in the languages Discord supports. Set a catalog on the bot, and handlers look messages up with `c.T`, in the language of the user who ran the command.

## A catalog

```go
cat := locale.New(discord.EnglishUS).
	Add(discord.EnglishUS, map[string]string{"greet": "Hello, %s!"}).
	Add(discord.French, map[string]string{"greet": "Bonjour, %s !"})

bot.SetCatalog(cat)
```

`locale.New` takes the default locale, whose messages are used when a message is missing in the language asked for. `Add` merges messages into a locale, so it can be called several times, also while the bot runs.

Messages can also be loaded from JSON files, usually embedded in the program. Each file is named after its locale, such as `fr.json` or `pt-BR.json`, and holds one object of messages by key:

```go
//go:embed locales/*.json
var locales embed.FS

cat := locale.New(discord.EnglishUS)
if err := cat.LoadFS(locales, "locales/*.json"); err != nil {
	log.Fatal(err)
}
```

```json
{
  "greet": "Bonjour, %s !"
}
```

A file named after no Discord locale, such as `pt_BR.json`, is an error, as Discord would never ask for it. `LoadFS` takes any `fs.FS`, so `os.DirFS("locales")` works too.

## Looking messages up

In an interaction handler, `c.T` looks the key up in the user's language, then the guild's, then the catalog's default, and returns the key itself if none has it:

```go
Execute: func(c *minidis.Context) error {
	return c.Reply(c.T("greet", c.Author.Username))
},
```

The arguments fill the message like `fmt.Sprintf`. Where a language puts them in another order, use explicit indexes, such as `"%[2]s a %[1]s"`. Without arguments the message is returned as it is, so a `%` in it needs no escaping.

As the key is returned when nothing matches, the English text can be the key itself: `c.T("Hello, %s!", name)` works before any translation exists.

Components, modals, context menus and autocomplete have `c.T` too, through the `Context` they embed.

Messages do not say the language of their author, so `T` on a `MessageContext` (and on an `interactions.ChannelMessageContext`) uses the guild's preferred locale when the [state cache](./state.md) has the guild, and the catalog's default otherwise. Elsewhere, such as in an event handler, use the catalog directly:

```go
msg := bot.Catalog().T(guild.PreferredLocale, "welcome", member.User.Username)
```

`cat.Lookup(key, locales...)` reports whether a message exists, trying the locales in order and then the default.

## Built-in replies

`Cooldown` and `RequirePermissions` answer users themselves. With a catalog, their replies are looked up like any other message, under these keys:

| Key                           | English reply                                     |
| ----------------------------- | ------------------------------------------------- |
| `minidis.cooldown`            | `You can use this command again %s.`              |
| `minidis.guild_only`          | `This only works in a server.`                    |
| `minidis.missing_permissions` | `You don't have the permissions to use this.`     |

The `%s` of `minidis.cooldown` is a Discord timestamp that shows as "in 5 minutes" in the user's language. Without a catalog, or in a language it lacks, the replies stay in English. The keys are also constants: `minidis.KeyCooldown`, `minidis.KeyGuildOnly` and `minidis.KeyMissingPermissions`.

## Command names and descriptions

When the bot syncs its commands, it fills in their localizations from the catalog, under `command.<name>.name` and `command.<name>.description`. Subcommands, groups, options and choices add their names to the path:

```json
{
  "command.settings.name": "paramètres",
  "command.settings.description": "Change les paramètres.",
  "command.settings.notify.channel.description": "Le salon des notifications.",
  "command.settings.notify.channel.color.red.name": "rouge"
}
```

Here `notify` is a subcommand of `settings`, `channel` an option of it, and `red` a choice of an option named `color`. Context menu commands take only a name, such as `command.Report message.name`.

Localizations set on a command, such as `NameLocalizations`, take precedence over the catalog. Discord's rules still apply to the translations: slash command and option names must be lowercase, without spaces. The handler and the option getters always use the unlocalized names.

See [`example/locale`](../example/locale/main.go) for a bot with embedded translations.
