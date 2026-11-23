# Slash commands

A slash command has a name, a description, and the function that runs it:

```go
bot.AddCommand(&minidis.Command{
	Name:        "ping",
	Description: "Replies with pong.",
	Execute: func(c *minidis.Context) error {
		return c.Reply("pong")
	},
})
```

Discord's rules apply: names are 1 to 32 lowercase characters without spaces, and descriptions are 1 to 100 characters. `AddCommand` takes several commands at once, and a command added again under the same name replaces the first.

The `Context` describes who ran the command and where: `c.Author`, `c.Member` (nil in a DM), `c.IsDM`, `c.GuildID`, `c.ChannelID`, `c.Locale` and the full `c.Interaction`. `c.Session` is the [client](./client.md), for anything else. Responding is covered in [Responding to interactions](./responding.md).

## Options

Options are the values a user fills in after the command name:

```go
minValue := 1.0

bot.AddCommand(&minidis.Command{
	Name:        "roll",
	Description: "Rolls dice.",
	Options: []*discord.ApplicationCommandOption{
		{
			Type:        discord.ApplicationCommandOptionInteger,
			Name:        "sides",
			Description: "How many sides the die has.",
			Required:    true,
			Choices: []*discord.ApplicationCommandOptionChoice{
				minidis.Choice("d6", 6),
				minidis.Choice("d20", 20),
			},
		},
		{
			Type:        discord.ApplicationCommandOptionInteger,
			Name:        "count",
			Description: "How many to roll.",
			MinValue:    &minValue,
		},
		{
			Type:         discord.ApplicationCommandOptionChannel,
			Name:         "channel",
			Description:  "Where to post the result.",
			ChannelTypes: []discord.ChannelType{discord.ChannelTypeGuildText},
		},
	},
	Execute: roll,
})
```

| Option type                                | Read with                                      |
| ------------------------------------------ | ---------------------------------------------- |
| `ApplicationCommandOptionString`           | `c.String`, `c.OptionalString`                 |
| `ApplicationCommandOptionInteger`          | `c.Int`, `c.OptionalInt` (an `int64`)          |
| `ApplicationCommandOptionNumber`           | `c.Float`, `c.OptionalFloat`                   |
| `ApplicationCommandOptionBoolean`          | `c.Bool`, `c.OptionalBool`                     |
| `ApplicationCommandOptionUser`             | `c.User`, and `c.OptionMember` in a guild      |
| `ApplicationCommandOptionChannel`          | `c.Channel`                                    |
| `ApplicationCommandOptionRole`             | `c.Role`                                       |
| `ApplicationCommandOptionMentionable`      | `c.User` or `c.Role`, whichever it is          |
| `ApplicationCommandOptionAttachment`       | `c.Attachment`                                 |

Required options come before optional ones. Further limits are `MinValue` and `MaxValue` for numbers, `MinLength` and `MaxLength` for strings, `Choices` for a fixed list of up to 25 values, and `Autocomplete` for [suggestions computed as the user types](./autocomplete.md).

### Reading options

The getters never panic. A missing option, or one of another type, gives the zero value or `nil`, and the `Optional…` getters also report whether it was given:

```go
func roll(c *minidis.Context) error {
	sides := c.Int("sides")
	count, given := c.OptionalInt("count")
	if !given {
		count = 1
	}
	target := c.Channel("channel") // nil when not given
	// ...
}
```

The channel, user, role and attachment getters return what Discord sent with the interaction, so no request is needed. `c.Options` holds the raw values.

## Subcommands

```go
bot.AddCommand(&minidis.Command{
	Name:        "role",
	Description: "Manage roles.",
	Subcommands: []*minidis.Subcommand{
		{Name: "list", Description: "List roles.", Execute: listRoles},
	},
	Groups: []*minidis.SubcommandGroup{
		{Name: "member", Description: "Roles of a member.", Subcommands: []*minidis.Subcommand{
			{Name: "add", Description: "Give a role.", Options: roleOptions, Execute: addRole},
		}},
	},
})
```

This makes `/role list` and `/role member add`. Each subcommand has its own `Options`, `Execute` and `Autocomplete`, and `c.String` and the other getters read the options of the subcommand that ran.

A command with subcommands only runs them, so it cannot have `Options`, an `Execute` or an `Autocomplete` of its own. Registering the commands reports such a command, and also an option with `Autocomplete: true` that no handler answers, or one with both `Choices` and `Autocomplete`.

## Where commands can be used

### Permissions

A command is usable by everyone unless `DefaultMemberPermissions` is set:

```go
DefaultMemberPermissions: discord.Int64(0),                             // admins only
DefaultMemberPermissions: discord.Int64(discord.PermissionManageGuild), // members who can manage the server
```

This is only the default: server admins can change who may use a command under **Server Settings > Integrations**. Check permissions in the handler too when it matters, such as with `c.Member.Permissions`.

`NSFW: true` shows a command only in age-restricted channels.

### Contexts and user-installable apps

Global commands are usable in guilds only, where member permissions apply, unless `Contexts` says otherwise:

```go
Contexts: []discord.InteractionContextType{
	discord.InteractionContextGuild,          // servers
	discord.InteractionContextBotDM,          // DMs with the bot
	discord.InteractionContextPrivateChannel, // other DMs and group DMs, for user-installed apps
},
```

A command whose `IntegrationTypes` include `discord.ApplicationIntegrationUserInstall` can be used by people who installed the app to their account, wherever they are. Such commands, and the entry point command, keep Discord's default of every context when `Contexts` is nil.

Discord only accepts `Contexts` and `IntegrationTypes` on global commands, so they are left out when registering in guilds. Register user-installable commands globally.

## Context menu commands

Message and user commands appear when right-clicking a message or a user, under **Apps**. They have a name but no description or options:

```go
bot.AddMessageCommand(&minidis.MessageCommand{
	Name: "Quote",
	Execute: func(c *minidis.MessageCommandContext) error {
		return c.Reply("> " + c.Message.Content) // the message it was used on
	},
})

bot.AddUserCommand(&minidis.UserCommand{
	Name: "Avatar",
	Execute: func(c *minidis.UserCommandContext) error {
		return c.Reply(c.TargetUser.AvatarURL(1024)) // c.TargetMember is set in guilds
	},
})
```

Unlike slash commands, their names may have capitals and spaces. Both embed a `Context`, so they respond the same way, and they take `DefaultMemberPermissions`, `NSFW`, `Contexts` and `IntegrationTypes` like slash commands do.

## Localization

Names, descriptions and choices can be translated. Discord shows the one matching the user's language, and `c.Locale` tells which that is:

```go
bot.AddCommand(&minidis.Command{
	Name:                     "hello",
	Description:              "Says hello.",
	NameLocalizations:        map[discord.Locale]string{discord.German: "hallo"},
	DescriptionLocalizations: map[discord.Locale]string{discord.German: "Sagt hallo."},
	Execute: func(c *minidis.Context) error {
		if c.Locale == discord.German {
			return c.Reply("Hallo!")
		}
		return c.Reply("Hello!")
	},
})
```

The handler and the option getters always use the unlocalized names.

## Activities

`bot.SetEntryPoint` sets the command that launches the app's Activity from the App Launcher:

```go
bot.SetEntryPoint(&minidis.EntryPointCommand{
	Name:        "launch",
	Description: "Start the game.",
	Handler:     discord.EntryPointHandlerApp,
	Execute: func(c *minidis.Context) error {
		return c.LaunchActivity()
	},
})
```

With `discord.EntryPointHandlerDiscordLaunchActivity`, Discord launches the Activity itself and `Execute` is not used. The entry point is global only. Without one, registering global commands keeps the entry point Discord created, which it refuses to remove in a bulk update.

## Registering commands

Discord has to know a command before anyone can use it. A `minidis.Bot` registers its commands each time it runs, in the guilds given to `bot.SetGuilds` or globally, replacing whatever Discord had; see [Getting started](./getting-started.md#registering-commands). To do it yourself:

```go
bot.DisableSync() // for minidis.Bot, so that Run leaves the commands alone

err := bot.SyncCommands(ctx)              // globally
err = bot.SyncCommands(ctx, "1234", "5678") // in these guilds
```

`SyncCommands` can be called before the bot connects, such as from a deploy step. It checks the commands first and reports every mistake it finds at once.

`bot.ClearCommands(ctx, guildIDs...)` removes the commands from the given guilds, or the global ones (except the entry point) with no IDs. Use it to clean up a test guild, since commands registered there stay after the bot moves to global commands.

Both treat empty IDs as missing, but IDs that are all empty are an error rather than a request for the global commands, so that an unset variable cannot replace them by mistake.

Next: [Responding to interactions](./responding.md).
