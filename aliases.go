package minidis

import "github.com/tbdsux/minidis/interactions"

// The types of the interactions package, under the same names, so that a bot
// needs only this package for them. Their documentation is there.
type (
	Command           = interactions.Command
	Subcommand        = interactions.Subcommand
	SubcommandGroup   = interactions.SubcommandGroup
	MessageCommand    = interactions.MessageCommand
	UserCommand       = interactions.UserCommand
	EntryPointCommand = interactions.EntryPointCommand

	Context               = interactions.Context
	AutocompleteContext   = interactions.AutocompleteContext
	ComponentContext      = interactions.ComponentContext
	ModalSubmitContext    = interactions.ModalSubmitContext
	MessageCommandContext = interactions.MessageCommandContext
	UserCommandContext    = interactions.UserCommandContext
	ChannelMessageContext = interactions.ChannelMessageContext

	Message         = interactions.Message
	MessageEdit     = interactions.MessageEdit
	Modal           = interactions.Modal
	FollowupMessage = interactions.FollowupMessage

	HandlerFunc = interactions.HandlerFunc
	Middleware  = interactions.Middleware
	PanicError  = interactions.PanicError
)

// The errors of the interactions package.
var (
	ErrNoHandler  = interactions.ErrNoHandler
	ErrNoResponse = interactions.ErrNoResponse
)

// Choice is interactions.Choice: an autocomplete suggestion.
var Choice = interactions.Choice

// RequirePermissions is interactions.RequirePermissions: middleware that lets
// only members with the given permissions run a handler.
var RequirePermissions = interactions.RequirePermissions
