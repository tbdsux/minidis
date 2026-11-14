package minidis

import (
	"slices"

	"github.com/tbdsux/minidis/discord"
)

// The getters below never panic. The plain forms return the zero value (or nil)
// when an option is missing or of another type; the Optional forms also report
// whether it was given. The option's own StringValue and similar methods panic
// on a type mismatch instead.

func optionValue[T any](c *Context, name string, want discord.ApplicationCommandOptionType) (T, bool) {
	var zero T

	o := c.Options[name]
	if o == nil || o.Type != want {
		return zero, false
	}

	v, ok := o.Value.(T)
	return v, ok
}

// OptionalString returns a string option and whether it was given.
func (c *Context) OptionalString(name string) (string, bool) {
	return optionValue[string](c, name, discord.ApplicationCommandOptionString)
}

// String returns a string option, or "".
func (c *Context) String(name string) string {
	v, _ := c.OptionalString(name)
	return v
}

// OptionalInt returns an integer option and whether it was given.
func (c *Context) OptionalInt(name string) (int64, bool) {
	v, ok := optionValue[float64](c, name, discord.ApplicationCommandOptionInteger) // JSON numbers decode as float64
	return int64(v), ok
}

// Int returns an integer option, or 0.
func (c *Context) Int(name string) int64 {
	v, _ := c.OptionalInt(name)
	return v
}

// OptionalFloat returns a number option and whether it was given.
func (c *Context) OptionalFloat(name string) (float64, bool) {
	return optionValue[float64](c, name, discord.ApplicationCommandOptionNumber)
}

// Float returns a number option, or 0.
func (c *Context) Float(name string) float64 {
	v, _ := c.OptionalFloat(name)
	return v
}

// OptionalBool returns a boolean option and whether it was given.
func (c *Context) OptionalBool(name string) (bool, bool) {
	return optionValue[bool](c, name, discord.ApplicationCommandOptionBoolean)
}

// Bool returns a boolean option, or false.
func (c *Context) Bool(name string) bool {
	v, _ := c.OptionalBool(name)
	return v
}

// resolved returns the ID given to an option and the interaction's resolved
// objects, or nil data if the option is missing or of another type.
func (c *Context) resolved(name string, want ...discord.ApplicationCommandOptionType) (string, *discord.ResolvedData) {
	o := c.Options[name]
	if o == nil || !slices.Contains(want, o.Type) {
		return "", nil
	}

	id, _ := o.Value.(string)

	data, ok := c.Interaction.Data.(discord.ApplicationCommandInteractionData)
	if !ok {
		return "", nil
	}
	return id, data.Resolved
}

// User returns the user given to a user or mentionable option, or nil.
func (c *Context) User(name string) *discord.User {
	id, r := c.resolved(name, discord.ApplicationCommandOptionUser, discord.ApplicationCommandOptionMentionable)
	if r == nil {
		return nil
	}
	return r.Users[id]
}

// OptionMember returns the guild member given to a user or mentionable option,
// with its User filled in, or nil if the option is missing or the user is not
// in the guild.
func (c *Context) OptionMember(name string) *discord.Member {
	id, r := c.resolved(name, discord.ApplicationCommandOptionUser, discord.ApplicationCommandOptionMentionable)
	if r == nil || r.Members[id] == nil {
		return nil
	}

	member := *r.Members[id]
	member.User = r.Users[id]
	return &member
}

// Channel returns the partial channel given to a channel option, or nil.
func (c *Context) Channel(name string) *discord.Channel {
	id, r := c.resolved(name, discord.ApplicationCommandOptionChannel)
	if r == nil {
		return nil
	}
	return r.Channels[id]
}

// Role returns the role given to a role or mentionable option, or nil.
func (c *Context) Role(name string) *discord.Role {
	id, r := c.resolved(name, discord.ApplicationCommandOptionRole, discord.ApplicationCommandOptionMentionable)
	if r == nil {
		return nil
	}
	return r.Roles[id]
}

// Attachment returns the file given to an attachment option, or nil.
func (c *Context) Attachment(name string) *discord.MessageAttachment {
	id, r := c.resolved(name, discord.ApplicationCommandOptionAttachment)
	if r == nil {
		return nil
	}
	return r.Attachments[id]
}
