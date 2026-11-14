// Package discord holds the Discord API types. The client that sends and
// receives them is in the discord/client package.
package discord

import (
	"encoding/json"
	"io"
	"time"
)

// Bool returns a pointer to v, for optional boolean fields.
func Bool(v bool) *bool { return &v }

// String returns a pointer to v, for optional text fields such as the Content
// of an edit.
func String(v string) *string { return &v }

// Int returns a pointer to v, for optional integer fields.
func Int(v int) *int { return &v }

// Int64 returns a pointer to v, for optional fields such as
// DefaultMemberPermissions.
func Int64(v int64) *int64 { return &v }

// Locale is a language code supported by Discord.
type Locale string

const (
	EnglishUS    Locale = "en-US"
	EnglishGB    Locale = "en-GB"
	Bulgarian    Locale = "bg"
	ChineseCN    Locale = "zh-CN"
	ChineseTW    Locale = "zh-TW"
	Croatian     Locale = "hr"
	Czech        Locale = "cs"
	Danish       Locale = "da"
	Dutch        Locale = "nl"
	Finnish      Locale = "fi"
	French       Locale = "fr"
	German       Locale = "de"
	Greek        Locale = "el"
	Hindi        Locale = "hi"
	Hungarian    Locale = "hu"
	Indonesian   Locale = "id"
	Italian      Locale = "it"
	Japanese     Locale = "ja"
	Korean       Locale = "ko"
	Lithuanian   Locale = "lt"
	Norwegian    Locale = "no"
	Polish       Locale = "pl"
	PortugueseBR Locale = "pt-BR"
	Romanian     Locale = "ro"
	Russian      Locale = "ru"
	SpanishES    Locale = "es-ES"
	SpanishLATAM Locale = "es-419"
	Swedish      Locale = "sv-SE"
	Thai         Locale = "th"
	Turkish      Locale = "tr"
	Ukrainian    Locale = "uk"
	Vietnamese   Locale = "vi"
)

// Intent is a bitfield of the gateway events a bot subscribes to.
type Intent int

const (
	IntentsGuilds                      Intent = 1 << 0
	IntentsGuildMembers                Intent = 1 << 1
	IntentsGuildModeration             Intent = 1 << 2
	IntentsGuildExpressions            Intent = 1 << 3
	IntentsGuildIntegrations           Intent = 1 << 4
	IntentsGuildWebhooks               Intent = 1 << 5
	IntentsGuildInvites                Intent = 1 << 6
	IntentsGuildVoiceStates            Intent = 1 << 7
	IntentsGuildPresences              Intent = 1 << 8
	IntentsGuildMessages               Intent = 1 << 9
	IntentsGuildMessageReactions       Intent = 1 << 10
	IntentsGuildMessageTyping          Intent = 1 << 11
	IntentsDirectMessages              Intent = 1 << 12
	IntentsDirectMessageReactions      Intent = 1 << 13
	IntentsDirectMessageTyping         Intent = 1 << 14
	IntentsMessageContent              Intent = 1 << 15
	IntentsGuildScheduledEvents        Intent = 1 << 16
	IntentsAutoModerationConfiguration Intent = 1 << 20
	IntentsAutoModerationExecution     Intent = 1 << 21
	IntentsGuildMessagePolls           Intent = 1 << 24
	IntentsDirectMessagePolls          Intent = 1 << 25

	// IntentsPrivileged are the intents that must be enabled in the developer portal.
	IntentsPrivileged = IntentsGuildMembers | IntentsGuildPresences | IntentsMessageContent

	// IntentsAllWithoutPrivileged is the default for a new Session.
	IntentsAllWithoutPrivileged = IntentsGuilds | IntentsGuildModeration | IntentsGuildExpressions |
		IntentsGuildIntegrations | IntentsGuildWebhooks | IntentsGuildInvites | IntentsGuildVoiceStates |
		IntentsGuildMessages | IntentsGuildMessageReactions | IntentsGuildMessageTyping |
		IntentsDirectMessages | IntentsDirectMessageReactions | IntentsDirectMessageTyping |
		IntentsGuildScheduledEvents | IntentsAutoModerationConfiguration | IntentsAutoModerationExecution |
		IntentsGuildMessagePolls | IntentsDirectMessagePolls

	// IntentsAll includes the privileged intents.
	IntentsAll = IntentsAllWithoutPrivileged | IntentsPrivileged

	IntentsNone Intent = 0
)

// ChannelType is the type of a channel.
type ChannelType int

const (
	ChannelTypeGuildText          ChannelType = 0
	ChannelTypeDM                 ChannelType = 1
	ChannelTypeGuildVoice         ChannelType = 2
	ChannelTypeGroupDM            ChannelType = 3
	ChannelTypeGuildCategory      ChannelType = 4
	ChannelTypeGuildNews          ChannelType = 5
	ChannelTypeGuildNewsThread    ChannelType = 10
	ChannelTypeGuildPublicThread  ChannelType = 11
	ChannelTypeGuildPrivateThread ChannelType = 12
	ChannelTypeGuildStageVoice    ChannelType = 13
	ChannelTypeGuildDirectory     ChannelType = 14
	ChannelTypeGuildForum         ChannelType = 15
	ChannelTypeGuildMedia         ChannelType = 16
)

// MessageFlags is a bitfield of message options.
type MessageFlags int

const (
	MessageFlagsSuppressEmbeds        MessageFlags = 1 << 2
	MessageFlagsEphemeral             MessageFlags = 1 << 6
	MessageFlagsSuppressNotifications MessageFlags = 1 << 12
	MessageFlagsIsVoiceMessage        MessageFlags = 1 << 13
	MessageFlagsIsComponentsV2        MessageFlags = 1 << 15
)

// User is a Discord user.
type User struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
	GlobalName    string `json:"global_name"`
	Avatar        string `json:"avatar"`
	Banner        string `json:"banner"`
	AccentColor   int    `json:"accent_color"`
	Bot           bool   `json:"bot"`
	System        bool   `json:"system"`
	PublicFlags   int    `json:"public_flags"`
}

// Member is a user's membership of a guild. User is absent from members that
// are nested inside a message or a resolved data map.
type Member struct {
	User        *User      `json:"user"`
	Nick        string     `json:"nick"`
	Avatar      string     `json:"avatar"`
	Roles       []string   `json:"roles"`
	JoinedAt    time.Time  `json:"joined_at"`
	Deaf        bool       `json:"deaf"`
	Mute        bool       `json:"mute"`
	Flags       int        `json:"flags"`
	Pending     bool       `json:"pending"`
	Permissions int64      `json:"permissions,string"`
	TimeoutEnds *time.Time `json:"communication_disabled_until"`
}

// Role is a guild role.
type Role struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       int    `json:"color"`
	Hoist       bool   `json:"hoist"`
	Position    int    `json:"position"`
	Permissions int64  `json:"permissions,string"`
	Managed     bool   `json:"managed"`
	Mentionable bool   `json:"mentionable"`

	Colors *RoleColors `json:"colors"`
}

// RoleColors is a role's colors. Discord uses the secondary and tertiary colors
// for gradient roles, and they are nil otherwise.
type RoleColors struct {
	Primary   int  `json:"primary_color"`
	Secondary *int `json:"secondary_color"`
	Tertiary  *int `json:"tertiary_color"`
}

// RoleParams is the body of a Create or Modify Guild Role request. A nil or
// empty field keeps Discord's default on create and is left unchanged on modify.
type RoleParams struct {
	Name        string      `json:"name,omitempty"`
	Permissions *int64      `json:"permissions,omitempty,string"`
	Colors      *RoleColors `json:"colors,omitempty"`
	Hoist       *bool       `json:"hoist,omitempty"`
	Mentionable *bool       `json:"mentionable,omitempty"`
}

// Channel is a (possibly partial) channel.
type Channel struct {
	ID       string      `json:"id"`
	GuildID  string      `json:"guild_id"`
	Name     string      `json:"name"`
	Type     ChannelType `json:"type"`
	Topic    string      `json:"topic"`
	NSFW     bool        `json:"nsfw"`
	ParentID string      `json:"parent_id"`
}

// MessageAttachment is a file attached to a message.
type MessageAttachment struct {
	ID          string  `json:"id"`
	Filename    string  `json:"filename"`
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	ContentType string  `json:"content_type,omitempty"`
	Size        int     `json:"size,omitempty"`
	URL         string  `json:"url,omitempty"`
	ProxyURL    string  `json:"proxy_url,omitempty"`
	Width       int     `json:"width,omitempty"`
	Height      int     `json:"height,omitempty"`
	Ephemeral   bool    `json:"ephemeral,omitempty"`
	Duration    float64 `json:"duration_secs,omitempty"`
	Waveform    string  `json:"waveform,omitempty"`
	Flags       int     `json:"flags,omitempty"`
}

// File is a file to upload with a message.
type File struct {
	Name        string
	ContentType string
	Reader      io.Reader
}

// MessageEmbed is a rich embed.
type MessageEmbed struct {
	URL         string                 `json:"url,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description,omitempty"`
	Timestamp   string                 `json:"timestamp,omitempty"`
	Color       int                    `json:"color,omitempty"`
	Footer      *MessageEmbedFooter    `json:"footer,omitempty"`
	Image       *MessageEmbedImage     `json:"image,omitempty"`
	Thumbnail   *MessageEmbedThumbnail `json:"thumbnail,omitempty"`
	Video       *MessageEmbedVideo     `json:"video,omitempty"`
	Provider    *MessageEmbedProvider  `json:"provider,omitempty"`
	Author      *MessageEmbedAuthor    `json:"author,omitempty"`
	Fields      []*MessageEmbedField   `json:"fields,omitempty"`
}

type MessageEmbedFooter struct {
	Text         string `json:"text,omitempty"`
	IconURL      string `json:"icon_url,omitempty"`
	ProxyIconURL string `json:"proxy_icon_url,omitempty"`
}

type MessageEmbedImage struct {
	URL      string `json:"url"`
	ProxyURL string `json:"proxy_url,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

type MessageEmbedThumbnail struct {
	URL      string `json:"url"`
	ProxyURL string `json:"proxy_url,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

type MessageEmbedVideo struct {
	URL    string `json:"url,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

type MessageEmbedProvider struct {
	URL  string `json:"url,omitempty"`
	Name string `json:"name,omitempty"`
}

type MessageEmbedAuthor struct {
	URL          string `json:"url,omitempty"`
	Name         string `json:"name,omitempty"`
	IconURL      string `json:"icon_url,omitempty"`
	ProxyIconURL string `json:"proxy_icon_url,omitempty"`
}

type MessageEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// AllowedMentionType is a category accepted by MessageAllowedMentions.Parse.
type AllowedMentionType string

const (
	AllowedMentionTypeRoles    AllowedMentionType = "roles"
	AllowedMentionTypeUsers    AllowedMentionType = "users"
	AllowedMentionTypeEveryone AllowedMentionType = "everyone"
)

// MessageAllowedMentions controls which mentions in a message notify anyone.
// An empty Parse suppresses all mentions not listed in Roles or Users.
type MessageAllowedMentions struct {
	Parse       []AllowedMentionType `json:"parse"`
	Roles       []string             `json:"roles,omitempty"`
	Users       []string             `json:"users,omitempty"`
	RepliedUser bool                 `json:"replied_user,omitempty"`
}

// MessageReference points at the message being replied to.
type MessageReference struct {
	MessageID       string `json:"message_id,omitempty"`
	ChannelID       string `json:"channel_id,omitempty"`
	GuildID         string `json:"guild_id,omitempty"`
	FailIfNotExists *bool  `json:"fail_if_not_exists,omitempty"`
}

// Message is a message in a channel.
type Message struct {
	ID                string               `json:"id"`
	ChannelID         string               `json:"channel_id"`
	GuildID           string               `json:"guild_id"`
	Author            *User                `json:"author"`
	Member            *Member              `json:"member"`
	Content           string               `json:"content"`
	Timestamp         time.Time            `json:"timestamp"`
	EditedTimestamp   *time.Time           `json:"edited_timestamp"`
	TTS               bool                 `json:"tts"`
	MentionEveryone   bool                 `json:"mention_everyone"`
	Mentions          []*User              `json:"mentions"`
	MentionRoles      []string             `json:"mention_roles"`
	Attachments       []*MessageAttachment `json:"attachments"`
	Embeds            []*MessageEmbed      `json:"embeds"`
	Components        ComponentList        `json:"components"`
	Pinned            bool                 `json:"pinned"`
	WebhookID         string               `json:"webhook_id"`
	Type              int                  `json:"type"`
	Flags             MessageFlags         `json:"flags"`
	MessageReference  *MessageReference    `json:"message_reference"`
	ReferencedMessage *Message             `json:"referenced_message"`
}

// MessageSend is the body of a Create Message request.
type MessageSend struct {
	Content         string                  `json:"content,omitempty"`
	Embeds          []*MessageEmbed         `json:"embeds,omitempty"`
	TTS             bool                    `json:"tts,omitempty"`
	Components      []MessageComponent      `json:"components,omitempty"`
	AllowedMentions *MessageAllowedMentions `json:"allowed_mentions,omitempty"`
	Reference       *MessageReference       `json:"message_reference,omitempty"`
	StickerIDs      []string                `json:"sticker_ids,omitempty"`
	Flags           MessageFlags            `json:"flags,omitempty"`
	Files           []*File                 `json:"-"`
}

// MessageEdit is the body of an Edit Message request. A nil field is left
// unchanged; a pointer to an empty value clears it.
type MessageEdit = WebhookEdit

// MessagePin is a pinned message and when it was pinned.
type MessagePin struct {
	PinnedAt time.Time `json:"pinned_at"`
	Message  *Message  `json:"message"`
}

// ChannelPins is one page of a channel's pinned messages.
type ChannelPins struct {
	Items   []*MessagePin `json:"items"`
	HasMore bool          `json:"has_more"`
}

// Emoji is an emoji in a reaction. ID is empty for a standard Unicode emoji.
type Emoji struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Animated bool   `json:"animated"`
}

// Guild is a Discord server. Channels, MemberCount, JoinedAt, Large and
// Unavailable are only sent in the GUILD_CREATE event.
type Guild struct {
	ID                     string     `json:"id"`
	Name                   string     `json:"name"`
	Icon                   string     `json:"icon"`
	Description            string     `json:"description"`
	OwnerID                string     `json:"owner_id"`
	Roles                  []*Role    `json:"roles"`
	Features               []string   `json:"features"`
	PreferredLocale        Locale     `json:"preferred_locale"`
	ApproximateMemberCount int        `json:"approximate_member_count"`
	Channels               []*Channel `json:"channels"`
	MemberCount            int        `json:"member_count"`
	JoinedAt               *time.Time `json:"joined_at"`
	Large                  bool       `json:"large"`
	Unavailable            bool       `json:"unavailable"`
}

// GuildBan is a user's ban from a guild.
type GuildBan struct {
	Reason string `json:"reason"`
	User   *User  `json:"user"`
}

// Status is a bot's online status.
type Status string

const (
	StatusOnline    Status = "online"
	StatusIdle      Status = "idle"
	StatusDND       Status = "dnd"
	StatusInvisible Status = "invisible"
)

// ActivityType is the kind of an Activity.
type ActivityType int

const (
	ActivityPlaying   ActivityType = 0
	ActivityStreaming ActivityType = 1
	ActivityListening ActivityType = 2
	ActivityWatching  ActivityType = 3
	ActivityCustom    ActivityType = 4 // set the text in State
	ActivityCompeting ActivityType = 5
)

// Activity is what a bot is shown to be doing.
type Activity struct {
	Name  string       `json:"name"`
	Type  ActivityType `json:"type"`
	URL   string       `json:"url,omitempty"`   // streaming only
	State string       `json:"state,omitempty"` // the text of a custom status
}

// PresenceUpdate is the body of the gateway's Update Presence command.
type PresenceUpdate struct {
	Since      *int64      `json:"since"`
	Activities []*Activity `json:"activities"`
	Status     Status      `json:"status"`
	AFK        bool        `json:"afk"`
}

// UnavailableGuild is a guild that has been announced but not yet delivered.
type UnavailableGuild struct {
	ID          string `json:"id"`
	Unavailable bool   `json:"unavailable"`
}

// Application is the partial application object sent in READY.
type Application struct {
	ID    string `json:"id"`
	Flags int    `json:"flags"`
}

// Ready is the READY gateway event.
type Ready struct {
	Version          int                 `json:"v"`
	User             *User               `json:"user"`
	Guilds           []*UnavailableGuild `json:"guilds"`
	SessionID        string              `json:"session_id"`
	ResumeGatewayURL string              `json:"resume_gateway_url"`
	Shard            *[2]int             `json:"shard"`
	Application      *Application        `json:"application"`
}

// MessageCreate is the MESSAGE_CREATE gateway event.
type MessageCreate struct {
	*Message
}

// InteractionCreate is the INTERACTION_CREATE gateway event.
type InteractionCreate struct {
	*Interaction
}

// GuildCreate is the GUILD_CREATE gateway event.
type GuildCreate struct {
	*Guild
}

// GuildMemberAdd is the GUILD_MEMBER_ADD gateway event. It needs the privileged
// IntentsGuildMembers intent.
type GuildMemberAdd struct {
	*Member
	GuildID string `json:"guild_id"`
}

// GuildMemberRemove is the GUILD_MEMBER_REMOVE gateway event. It needs the
// privileged IntentsGuildMembers intent.
type GuildMemberRemove struct {
	GuildID string `json:"guild_id"`
	User    *User  `json:"user"`
}

// MessageUpdate is the MESSAGE_UPDATE gateway event.
type MessageUpdate struct {
	*Message
}

// MessageDelete is the MESSAGE_DELETE gateway event.
type MessageDelete struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
}

// MessageReactionAdd is the MESSAGE_REACTION_ADD gateway event. Member is only
// set in guilds.
type MessageReactionAdd struct {
	UserID          string  `json:"user_id"`
	ChannelID       string  `json:"channel_id"`
	MessageID       string  `json:"message_id"`
	GuildID         string  `json:"guild_id"`
	MessageAuthorID string  `json:"message_author_id"`
	Member          *Member `json:"member"`
	Emoji           *Emoji  `json:"emoji"`
}

// jsonNull reports whether raw is absent or a JSON null.
func jsonNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}
