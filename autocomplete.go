package minidis

import (
	"fmt"
	"strconv"

	"github.com/tbdsux/minidis/discord"
)

// maxAutocompleteChoices is the most choices Discord accepts in one response.
const maxAutocompleteChoices = 25

// AutocompleteContext is passed to a command's Autocomplete handler while a user
// types into an option declared with Autocomplete: true.
type AutocompleteContext struct {
	*Context

	// Focused is the option being typed into. Check Focused.Name when a command
	// has several autocomplete options.
	Focused *discord.ApplicationCommandInteractionDataOption
}

// Input returns what the user has typed so far. Use it instead of the option
// getters: Discord does not validate input during autocomplete, so even a
// number option can hold partial text.
func (c *AutocompleteContext) Input() string {
	if c.Focused == nil {
		return ""
	}

	switch v := c.Focused.Value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

// Respond sends the suggestions to the user. Discord accepts 25 at most, so any
// beyond that are dropped; no choices shows the user that nothing matched.
func (c *AutocompleteContext) Respond(choices ...*discord.ApplicationCommandOptionChoice) error {
	if len(choices) > maxAutocompleteChoices {
		choices = choices[:maxAutocompleteChoices]
	}
	if choices == nil {
		choices = []*discord.ApplicationCommandOptionChoice{}
	}

	return c.Session.InteractionRespond(c.Interaction, &discord.InteractionResponse{
		Type: discord.InteractionApplicationCommandAutocompleteResult,
		Data: &discord.InteractionResponseData{Choices: choices},
	})
}

// Choice builds an option choice. value must match the option's type.
func Choice(name string, value any) *discord.ApplicationCommandOptionChoice {
	return &discord.ApplicationCommandOptionChoice{Name: name, Value: value}
}

func (m *Minidis) routeAutocomplete(c *Context) HandlerFunc {
	data := c.Interaction.ApplicationCommandData()

	cmd, ok := m.commands[data.Name]
	if !ok {
		return failWith(fmt.Errorf("%w: autocomplete for command %q", ErrNoHandler, data.Name))
	}

	target := cmd.resolve(data.Options)
	if target.autocomplete == nil {
		return failWith(fmt.Errorf("%w: autocomplete for command %q", ErrNoHandler, data.Name))
	}

	c.setOptions(target.options)

	var focused *discord.ApplicationCommandInteractionDataOption
	for _, o := range target.options {
		if o.Focused {
			focused = o
			break
		}
	}

	return func(c *Context) error {
		return target.autocomplete(&AutocompleteContext{Context: c, Focused: focused})
	}
}
