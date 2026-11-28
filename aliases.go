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

	Page            = interactions.Page
	PaginateOptions = interactions.PaginateOptions

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

// The middleware of the interactions package: Cooldown limits how often a
// command runs for each key, and RequirePermissions lets only members with
// the given permissions run a handler.
var (
	Cooldown           = interactions.Cooldown
	PerUser            = interactions.PerUser
	PerGuild           = interactions.PerGuild
	PerChannel         = interactions.PerChannel
	RequirePermissions = interactions.RequirePermissions
)

// The catalog keys of the built-in replies; see interactions.KeyCooldown.
const (
	KeyCooldown           = interactions.KeyCooldown
	KeyGuildOnly          = interactions.KeyGuildOnly
	KeyMissingPermissions = interactions.KeyMissingPermissions
	KeyPaginateAuthorOnly = interactions.KeyPaginateAuthorOnly
)
