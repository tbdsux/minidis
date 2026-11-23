# Autocomplete

Autocomplete suggests values for an option while the user types, for lists too long or too dynamic for `Choices`. Set `Autocomplete: true` on the option, and an `Autocomplete` handler on the command or subcommand:

```go
var languages = []string{"Go", "Rust", "Python", "TypeScript", "Zig"}

bot.AddCommand(&minidis.Command{
	Name:        "language",
	Description: "Pick a programming language.",
	Options: []*discord.ApplicationCommandOption{{
		Type:         discord.ApplicationCommandOptionString,
		Name:         "name",
		Description:  "Start typing a language.",
		Required:     true,
		Autocomplete: true,
	}},
	Autocomplete: func(c *minidis.AutocompleteContext) error {
		input := strings.ToLower(c.Input())

		var choices []*discord.ApplicationCommandOptionChoice
		for _, l := range languages {
			if strings.Contains(strings.ToLower(l), input) {
				choices = append(choices, minidis.Choice(l, l))
			}
		}
		return c.Respond(choices...)
	},
	Execute: func(c *minidis.Context) error {
		return c.Reply("You picked " + c.String("name"))
	},
})
```

- `c.Input()` is what the user has typed so far. Use it rather than the option getters: Discord does not check input during autocomplete, so even a number option can hold partial text.
- `c.Focused` is the option being typed into. Check `c.Focused.Name` when a command has several autocomplete options. The other options already filled in are readable with the getters.
- `c.Respond` sends up to 25 suggestions, and drops the rest. Sending none shows the user that nothing matched.
- `minidis.Choice(name, value)` builds a suggestion. The value must match the option's type: a string, an integer or a number.

The handler runs on every keystroke and has 3 seconds to respond, so keep it fast. `Respond` is the only valid response, and middleware does not run for it.

Suggestions are only suggestions: the user may still submit anything, so `Execute` should check the value. An option cannot have both `Choices` and `Autocomplete`, and registering the commands reports one that has `Autocomplete: true` but no handler.

See the [autocomplete example](../example/autocomplete/main.go).

Next: [Messages and events](./messages-and-events.md).
