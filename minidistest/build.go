package minidistest

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// The IDs of what the builders make, unless a test changes them. AppID is
// also the ID of the bot user.
const (
	AppID     = "1000"
	GuildID   = "2000"
	ChannelID = "3000"
	UserID    = "4000"
)

var lastID atomic.Int64

// newID returns an ID for an interaction or a message, each different.
func newID() string {
	return strconv.FormatInt(100000+lastID.Add(1), 10)
}

// User returns the user who sends the interactions and messages of the
// builders, with the ID UserID.
func User() *discord.User {
	return &discord.User{ID: UserID, Username: "user", GlobalName: "User"}
}

// interaction returns an interaction from User, as a member of the guild
// GuildID without permissions, in its channel ChannelID, with the locale
// en-US. The bot has every permission there.
func interaction(typ discord.InteractionType, data any) *discord.Interaction {
	id := newID()
	locale := discord.EnglishUS
	return &discord.Interaction{
		ID:             id,
		AppID:          AppID,
		Type:           typ,
		Data:           data,
		Guild:          &discord.Guild{ID: GuildID},
		GuildID:        GuildID,
		Channel:        &discord.Channel{ID: ChannelID, GuildID: GuildID, Type: discord.ChannelTypeGuildText},
		ChannelID:      ChannelID,
		Member:         &discord.Member{User: User(), JoinedAt: time.Now()},
		Token:          "token-" + id,
		Version:        1,
		AppPermissions: discord.PermissionAll,
		Locale:         discord.EnglishUS,
		GuildLocale:    &locale,
		Context:        discord.InteractionContextGuild,
	}
}

// DM moves an interaction from the guild to the bot's DM with its user, and
// returns it.
func DM(i *discord.Interaction) *discord.Interaction {
	if i.Member != nil {
		i.User = i.Member.User
	}
	i.Member, i.Guild, i.GuildID, i.GuildLocale = nil, nil, "", nil
	i.ChannelID = "dm-" + i.User.ID
	i.Channel = &discord.Channel{ID: i.ChannelID, Type: discord.ChannelTypeDM}
	i.Context = discord.InteractionContextBotDM
	return i
}

// Slash returns the interaction of a slash command, with options made with
// Option and Subcommand. Change its fields for another user, guild or locale:
//
//	i := minidistest.Slash("greet", minidistest.Option("name", "Ann"))
//	i.Locale = discord.French
//	i.Member.Permissions = discord.PermissionManageMessages
func Slash(name string, options ...*discord.ApplicationCommandInteractionDataOption) *discord.Interaction {
	return interaction(discord.InteractionApplicationCommand, discord.ApplicationCommandInteractionData{
		ID: newID(), Name: name, CommandType: discord.ChatApplicationCommand, Options: options,
	})
}

// Option returns the value of an option: a string, a bool, an int (or
// int64) for an integer option or a float64 for a number option. Others,
// such as users and channels, are made as a
// discord.ApplicationCommandInteractionDataOption, with the objects they
// name in the Resolved of the command's data.
func Option(name string, value any) *discord.ApplicationCommandInteractionDataOption {
	var typ discord.ApplicationCommandOptionType
	switch value.(type) {
	case string:
		typ = discord.ApplicationCommandOptionString
	case bool:
		typ = discord.ApplicationCommandOptionBoolean
	case int, int64:
		typ = discord.ApplicationCommandOptionInteger
	case float64:
		typ = discord.ApplicationCommandOptionNumber
	default:
		panic(fmt.Sprintf("minidistest: option %q has a %T; make it as a discord.ApplicationCommandInteractionDataOption", name, value))
	}
	return &discord.ApplicationCommandInteractionDataOption{Name: name, Type: typ, Value: value}
}

// Subcommand returns a subcommand with its options, for Slash and
// Autocomplete.
func Subcommand(name string, options ...*discord.ApplicationCommandInteractionDataOption) *discord.ApplicationCommandInteractionDataOption {
	return &discord.ApplicationCommandInteractionDataOption{Name: name, Type: discord.ApplicationCommandOptionSubCommand, Options: options}
}

// Focused returns the string option that the user is typing, for
// Autocomplete.
func Focused(name, input string) *discord.ApplicationCommandInteractionDataOption {
	o := Option(name, input)
	o.Focused = true
	return o
}

// Autocomplete returns the interaction of the user typing an option of a
// slash command: the one made with Focused, among the others.
func Autocomplete(name string, options ...*discord.ApplicationCommandInteractionDataOption) *discord.Interaction {
	i := Slash(name, options...)
	i.Type = discord.InteractionApplicationCommandAutocomplete
	return i
}

// UserCommand returns the interaction of a user command used on target.
func UserCommand(name string, target *discord.User) *discord.Interaction {
	return interaction(discord.InteractionApplicationCommand, discord.ApplicationCommandInteractionData{
		ID: newID(), Name: name, CommandType: discord.UserApplicationCommand, TargetID: target.ID,
		Resolved: &discord.ResolvedData{Users: map[string]*discord.User{target.ID: target}},
	})
}

// MessageCommand returns the interaction of a message command used on
// target, such as one made with Message.
func MessageCommand(name string, target *discord.Message) *discord.Interaction {
	return interaction(discord.InteractionApplicationCommand, discord.ApplicationCommandInteractionData{
		ID: newID(), Name: name, CommandType: discord.MessageApplicationCommand, TargetID: target.ID,
		Resolved: &discord.ResolvedData{Messages: map[string]*discord.Message{target.ID: target}},
	})
}

// Component returns the interaction of a button press, or of a choice in a
// string select menu if values are given, on a message of the bot.
func Component(customID string, values ...string) *discord.Interaction {
	typ := discord.ButtonComponent
	if len(values) > 0 {
		typ = discord.SelectMenuComponent
	}
	i := interaction(discord.InteractionMessageComponent, discord.MessageComponentInteractionData{
		CustomID: customID, ComponentType: typ, Values: values,
	})
	i.Message = &discord.Message{
		ID: newID(), ChannelID: ChannelID, GuildID: GuildID, Timestamp: time.Now(),
		Author: &discord.User{ID: AppID, Username: "bot", Bot: true},
	}
	return i
}

// ModalSubmit returns the interaction of a modal being submitted, with its
// components as filled in:
//
//	minidistest.ModalSubmit("feedback", &discord.Label{
//		Component: &discord.TextInput{CustomID: "text", Value: "Great bot"},
//	})
func ModalSubmit(customID string, components ...discord.MessageComponent) *discord.Interaction {
	return interaction(discord.InteractionModalSubmit, discord.ModalSubmitInteractionData{
		CustomID: customID, Components: components,
	})
}

// Message returns a message from User in the channel ChannelID of the guild
// GuildID, for Dispatch with a discord.MessageCreate:
//
//	minidistest.Dispatch(kit, &discord.MessageCreate{Message: minidistest.Message("!ping")})
//
// Clear its GuildID and Member for a DM.
func Message(content string) *discord.Message {
	return &discord.Message{
		ID:        newID(),
		ChannelID: ChannelID,
		GuildID:   GuildID,
		Author:    User(),
		Member:    &discord.Member{JoinedAt: time.Now()},
		Content:   content,
		Timestamp: time.Now(),
		Type:      discord.MessageTypeDefault,
	}
}
