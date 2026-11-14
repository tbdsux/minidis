package minidis

import (
	"fmt"
	"slices"

	"github.com/tbdsux/minidis/discord"
)

// commandTarget is the command or subcommand an interaction is aimed at.
type commandTarget struct {
	execute      func(*Context) error
	autocomplete func(*AutocompleteContext) error
	options      []*discord.ApplicationCommandInteractionDataOption // the values the user gave it
}

func (s *Subcommand) target(options []*discord.ApplicationCommandInteractionDataOption) commandTarget {
	return commandTarget{execute: s.Execute, autocomplete: s.Autocomplete, options: options}
}

func findSubcommand(subs []*Subcommand, name string) *Subcommand {
	if i := slices.IndexFunc(subs, func(s *Subcommand) bool { return s.Name == name }); i >= 0 {
		return subs[i]
	}
	return nil
}

// resolve finds what the given options select. It falls back to the command
// itself when no subcommand matches.
func (cmd *Command) resolve(given []*discord.ApplicationCommandInteractionDataOption) commandTarget {
	root := commandTarget{execute: cmd.Execute, autocomplete: cmd.Autocomplete, options: given}
	if len(given) == 0 {
		return root
	}

	first := given[0]
	switch first.Type {
	case discord.ApplicationCommandOptionSubCommandGroup:
		if len(first.Options) == 0 {
			return root
		}
		i := slices.IndexFunc(cmd.Groups, func(g *SubcommandGroup) bool { return g.Name == first.Name })
		if i < 0 {
			return root
		}
		if sub := findSubcommand(cmd.Groups[i].Subcommands, first.Options[0].Name); sub != nil {
			return sub.target(first.Options[0].Options)
		}
	case discord.ApplicationCommandOptionSubCommand:
		if sub := findSubcommand(cmd.Subcommands, first.Name); sub != nil {
			return sub.target(first.Options)
		}
	}

	return root
}

func (m *Minidis) routeSlash(c *Context, data discord.ApplicationCommandInteractionData) HandlerFunc {
	cmd, ok := m.commands[data.Name]
	if !ok {
		return failWith(fmt.Errorf("%w: command %q", ErrNoHandler, data.Name))
	}

	target := cmd.resolve(data.Options)
	if target.execute == nil {
		return failWith(fmt.Errorf("%w: command %q", ErrNoHandler, data.Name))
	}

	c.setOptions(target.options)

	return target.execute
}
