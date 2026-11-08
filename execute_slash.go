package minidis

import (
	"fmt"

	"github.com/tbdsux/minidis/discord"
)

// commandTarget is the command or subcommand an interaction is aimed at.
type commandTarget struct {
	execute      func(*SlashContext) error
	autocomplete func(*AutocompleteContext) error
	options      []*discord.ApplicationCommandInteractionDataOption // the values the user gave it
}

func (s *SlashSubcommandProps) target(options []*discord.ApplicationCommandInteractionDataOption) commandTarget {
	return commandTarget{execute: s.Execute, autocomplete: s.Autocomplete, options: options}
}

// resolve finds what the given options select. It falls back to the command
// itself when no registered subcommand matches.
func (s *SlashCommandProps) resolve(given []*discord.ApplicationCommandInteractionDataOption) commandTarget {
	root := commandTarget{execute: s.Execute, autocomplete: s.Autocomplete, options: given}
	if len(given) == 0 {
		return root
	}

	first := given[0]
	switch first.Type {
	case discord.ApplicationCommandOptionSubCommandGroup:
		if len(first.Options) == 0 {
			return root
		}
		if group, ok := s.subcommandGroups[first.Name]; ok {
			if sub, ok := group.subcommands[first.Options[0].Name]; ok {
				return sub.target(first.Options[0].Options)
			}
		}
	case discord.ApplicationCommandOptionSubCommand:
		if sub, ok := s.subcommands[first.Name]; ok {
			return sub.target(first.Options)
		}
	}

	return root
}

// failWith is a handler that reports err, so that it passes through middleware
// and reaches the error handler like any other failure.
func failWith(err error) HandlerFunc {
	return func(*SlashContext) error { return err }
}

func (m *Minidis) routeSlash(c *SlashContext, data discord.ApplicationCommandInteractionData) HandlerFunc {
	cmd, ok := m.Commands[data.Name]
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
