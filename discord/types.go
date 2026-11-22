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

	// IntentsNone receives no events beyond the basics, which is enough for
	// a bot that only handles interactions.
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
	MessageFlagsCrossposted                      MessageFlags = 1 << 0
	MessageFlagsIsCrosspost                      MessageFlags = 1 << 1
	MessageFlagsSuppressEmbeds                   MessageFlags = 1 << 2
	MessageFlagsSourceMessageDeleted             MessageFlags = 1 << 3
	MessageFlagsUrgent                           MessageFlags = 1 << 4
	MessageFlagsHasThread                        MessageFlags = 1 << 5
	MessageFlagsEphemeral                        MessageFlags = 1 << 6
	MessageFlagsLoading                          MessageFlags = 1 << 7
	MessageFlagsFailedToMentionSomeRolesInThread MessageFlags = 1 << 8
	MessageFlagsSuppressNotifications            MessageFlags = 1 << 12
	MessageFlagsIsVoiceMessage                   MessageFlags = 1 << 13
	MessageFlagsHasSnapshot                      MessageFlags = 1 << 14
	MessageFlagsIsComponentsV2                   MessageFlags = 1 << 15
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

	// The fields below are only sent for the bot's own user.
	MFAEnabled bool   `json:"mfa_enabled"`
	Verified   bool   `json:"verified"`
	Locale     Locale `json:"locale"`
	Flags      int    `json:"flags"`

	PremiumType      int               `json:"premium_type"`
	AvatarDecoration *AvatarDecoration `json:"avatar_decoration_data"`
	PrimaryGuild     *UserPrimaryGuild `json:"primary_guild"`
}

// User flags, for User.PublicFlags and User.Flags.
const (
	UserFlagStaff                 = 1 << 0
	UserFlagPartner               = 1 << 1
	UserFlagHypeSquad             = 1 << 2
	UserFlagBugHunterLevel1       = 1 << 3
	UserFlagHypeSquadBravery      = 1 << 6
	UserFlagHypeSquadBrilliance   = 1 << 7
	UserFlagHypeSquadBalance      = 1 << 8
	UserFlagPremiumEarlySupporter = 1 << 9
	UserFlagTeamPseudoUser        = 1 << 10
	UserFlagBugHunterLevel2       = 1 << 14
	UserFlagVerifiedBot           = 1 << 16
	UserFlagVerifiedDeveloper     = 1 << 17
	UserFlagCertifiedModerator    = 1 << 18
	UserFlagBotHTTPInteractions   = 1 << 19
)

// AvatarDecoration is the decoration shown around an avatar.
type AvatarDecoration struct {
	Asset string `json:"asset"`
	SKUID string `json:"sku_id"`
}

// UserPrimaryGuild is the guild whose tag a user shows next to their name.
type UserPrimaryGuild struct {
	GuildID string `json:"identity_guild_id"`
	Enabled *bool  `json:"identity_enabled"`
	Tag     string `json:"tag"`
	Badge   string `json:"badge"`
}

// Member is a user's membership of a guild. User is absent from members that
// are nested inside a message or a resolved data map.
type Member struct {
	User             *User             `json:"user"`
	Nick             string            `json:"nick"`
	Avatar           string            `json:"avatar"`
	Banner           string            `json:"banner"`
	Roles            []string          `json:"roles"`
	JoinedAt         time.Time         `json:"joined_at"`
	PremiumSince     *time.Time        `json:"premium_since"`
	Deaf             bool              `json:"deaf"`
	Mute             bool              `json:"mute"`
	Flags            int               `json:"flags"`
	Pending          bool              `json:"pending"`
	Permissions      int64             `json:"permissions,string"`
	TimeoutEnds      *time.Time        `json:"communication_disabled_until"`
	AvatarDecoration *AvatarDecoration `json:"avatar_decoration_data"`
}

// Member flags, for Member.Flags. A bot can only change
// MemberFlagBypassesVerification.
const (
	MemberFlagDidRejoin                    = 1 << 0
	MemberFlagCompletedOnboarding          = 1 << 1
	MemberFlagBypassesVerification         = 1 << 2
	MemberFlagStartedOnboarding            = 1 << 3
	MemberFlagIsGuest                      = 1 << 4
	MemberFlagStartedHomeActions           = 1 << 5
	MemberFlagCompletedHomeActions         = 1 << 6
	MemberFlagAutomodQuarantinedUsername   = 1 << 7
	MemberFlagDMSettingsUpsellAcknowledged = 1 << 9
)

// Role is a guild role.
type Role struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Color        int    `json:"color"`
	Hoist        bool   `json:"hoist"`
	Icon         string `json:"icon"`
	UnicodeEmoji string `json:"unicode_emoji"`
	Position     int    `json:"position"`
	Permissions  int64  `json:"permissions,string"`
	Managed      bool   `json:"managed"`
	Mentionable  bool   `json:"mentionable"`
	Flags        int    `json:"flags"`

	Colors *RoleColors `json:"colors"`
	Tags   *RoleTags   `json:"tags"`
}

// RoleTags says what a managed role belongs to. Discord marks the boolean
// tags by sending them as null, so they are decoded by their presence.
type RoleTags struct {
	BotID                 string `json:"bot_id"`
	IntegrationID         string `json:"integration_id"`
	SubscriptionListingID string `json:"subscription_listing_id"`
	PremiumSubscriber     bool   `json:"-"` // the role of the guild's boosters
	AvailableForPurchase  bool   `json:"-"`
	GuildConnections      bool   `json:"-"` // a linked role
}

// UnmarshalJSON implements json.Unmarshaler; see RoleTags.
func (t *RoleTags) UnmarshalJSON(data []byte) error {
	type alias RoleTags
	if err := json.Unmarshal(data, (*alias)(t)); err != nil {
		return err
	}

	var present map[string]json.RawMessage
	if err := json.Unmarshal(data, &present); err != nil {
		return err
	}
	_, t.PremiumSubscriber = present["premium_subscriber"]
	_, t.AvailableForPurchase = present["available_for_purchase"]
	_, t.GuildConnections = present["guild_connections"]
	return nil
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

	// Icon is an image as a data URI (see ImageData) and UnicodeEmoji a
	// standard emoji, for guilds with the ROLE_ICONS feature.
	Icon         Optional[string] `json:"icon,omitzero"`
	UnicodeEmoji Optional[string] `json:"unicode_emoji,omitzero"`
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

// MessageEmbedFooter is the footer of an embed.
type MessageEmbedFooter struct {
	Text         string `json:"text,omitempty"`
	IconURL      string `json:"icon_url,omitempty"`
	ProxyIconURL string `json:"proxy_icon_url,omitempty"`
}

// MessageEmbedImage is the image of an embed.
type MessageEmbedImage struct {
	URL      string `json:"url"`
	ProxyURL string `json:"proxy_url,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

// MessageEmbedThumbnail is the thumbnail of an embed.
type MessageEmbedThumbnail struct {
	URL      string `json:"url"`
	ProxyURL string `json:"proxy_url,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

// MessageEmbedVideo is the video of an embed.
type MessageEmbedVideo struct {
	URL    string `json:"url,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// MessageEmbedProvider is the provider of an embed, such as a website.
type MessageEmbedProvider struct {
	URL  string `json:"url,omitempty"`
	Name string `json:"name,omitempty"`
}

// MessageEmbedAuthor is the author of an embed.
type MessageEmbedAuthor struct {
	URL          string `json:"url,omitempty"`
	Name         string `json:"name,omitempty"`
	IconURL      string `json:"icon_url,omitempty"`
	ProxyIconURL string `json:"proxy_icon_url,omitempty"`
}

// MessageEmbedField is a field of an embed.
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

	// Type is MessageReferenceDefault for a reply, the default, or
	// MessageReferenceForward to forward the message.
	Type MessageReferenceType `json:"type,omitempty"`
}

// MessageReferenceType is what a MessageReference is for.
type MessageReferenceType int

const (
	MessageReferenceDefault MessageReferenceType = 0 // a reply
	MessageReferenceForward MessageReferenceType = 1
)

// MessageType is the kind of a message. Most of the types are system messages.
type MessageType int

const (
	MessageTypeDefault                             MessageType = 0
	MessageTypeRecipientAdd                        MessageType = 1
	MessageTypeRecipientRemove                     MessageType = 2
	MessageTypeCall                                MessageType = 3
	MessageTypeChannelNameChange                   MessageType = 4
	MessageTypeChannelIconChange                   MessageType = 5
	MessageTypeChannelPinnedMessage                MessageType = 6
	MessageTypeUserJoin                            MessageType = 7
	MessageTypeGuildBoost                          MessageType = 8
	MessageTypeGuildBoostTier1                     MessageType = 9
	MessageTypeGuildBoostTier2                     MessageType = 10
	MessageTypeGuildBoostTier3                     MessageType = 11
	MessageTypeChannelFollowAdd                    MessageType = 12
	MessageTypeGuildDiscoveryDisqualified          MessageType = 14
	MessageTypeGuildDiscoveryRequalified           MessageType = 15
	MessageTypeGuildDiscoveryGracePeriodInitial    MessageType = 16
	MessageTypeGuildDiscoveryGracePeriodFinal      MessageType = 17
	MessageTypeThreadCreated                       MessageType = 18
	MessageTypeReply                               MessageType = 19
	MessageTypeChatInputCommand                    MessageType = 20
	MessageTypeThreadStarterMessage                MessageType = 21
	MessageTypeGuildInviteReminder                 MessageType = 22
	MessageTypeContextMenuCommand                  MessageType = 23
	MessageTypeAutoModerationAction                MessageType = 24
	MessageTypeRoleSubscriptionPurchase            MessageType = 25
	MessageTypeInteractionPremiumUpsell            MessageType = 26
	MessageTypeStageStart                          MessageType = 27
	MessageTypeStageEnd                            MessageType = 28
	MessageTypeStageSpeaker                        MessageType = 29
	MessageTypeStageTopic                          MessageType = 31
	MessageTypeGuildApplicationPremiumSubscription MessageType = 32
	MessageTypeGuildIncidentAlertModeEnabled       MessageType = 36
	MessageTypeGuildIncidentAlertModeDisabled      MessageType = 37
	MessageTypeGuildIncidentReportRaid             MessageType = 38
	MessageTypeGuildIncidentReportFalseAlarm       MessageType = 39
	MessageTypePurchaseNotification                MessageType = 44
	MessageTypePollResult                          MessageType = 46
)

// MessageReaction is how often an emoji was used to react to a message.
type MessageReaction struct {
	Count        int                  `json:"count"`
	CountDetails MessageReactionCount `json:"count_details"`
	Me           bool                 `json:"me"`
	MeBurst      bool                 `json:"me_burst"`
	Emoji        *Emoji               `json:"emoji"`
	BurstColors  []string             `json:"burst_colors"`
}

// MessageReactionCount splits a reaction count into normal and super reactions.
type MessageReactionCount struct {
	Burst  int `json:"burst"`
	Normal int `json:"normal"`
}

// ChannelMention is a channel mentioned in a crossposted message.
type ChannelMention struct {
	ID      string      `json:"id"`
	GuildID string      `json:"guild_id"`
	Type    ChannelType `json:"type"`
	Name    string      `json:"name"`
}

// MessageInteractionMetadata describes the interaction a message answers.
type MessageInteractionMetadata struct {
	ID                            string                                `json:"id"`
	Type                          InteractionType                       `json:"type"`
	User                          *User                                 `json:"user"`
	AuthorizingIntegrationOwners  map[ApplicationIntegrationType]string `json:"authorizing_integration_owners"`
	OriginalResponseMessageID     string                                `json:"original_response_message_id"`
	InteractedMessageID           string                                `json:"interacted_message_id"`           // components
	TargetUser                    *User                                 `json:"target_user"`                     // user commands
	TargetMessageID               string                                `json:"target_message_id"`               // message commands
	TriggeringInteractionMetadata *MessageInteractionMetadata           `json:"triggering_interaction_metadata"` // modals
}

// MessageSnapshot is the copy of a forwarded message. Its Message holds only
// the content fields: no ID, author or channel.
type MessageSnapshot struct {
	Message *Message `json:"message"`
}

// MessageCall is the call a MessageTypeCall message is about.
type MessageCall struct {
	Participants   []string   `json:"participants"`
	EndedTimestamp *time.Time `json:"ended_timestamp"`
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
	MentionChannels   []*ChannelMention    `json:"mention_channels"`
	Attachments       []*MessageAttachment `json:"attachments"`
	Embeds            []*MessageEmbed      `json:"embeds"`
	Reactions         []*MessageReaction   `json:"reactions"`
	Components        ComponentList        `json:"components"`
	Pinned            bool                 `json:"pinned"`
	WebhookID         string               `json:"webhook_id"`
	Type              MessageType          `json:"type"`
	ApplicationID     string               `json:"application_id"`
	Flags             MessageFlags         `json:"flags"`
	MessageReference  *MessageReference    `json:"message_reference"`
	MessageSnapshots  []*MessageSnapshot   `json:"message_snapshots"`
	ReferencedMessage *Message             `json:"referenced_message"`
	Poll              *Poll                `json:"poll"`
	Call              *MessageCall         `json:"call"`

	InteractionMetadata *MessageInteractionMetadata `json:"interaction_metadata"`
	Thread              *Channel                    `json:"thread"` // the thread started from this message
	StickerItems        []*StickerItem              `json:"sticker_items"`
	Position            int                         `json:"position"` // roughly, in a thread
	Nonce               json.RawMessage             `json:"nonce"`    // a string or an integer
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
	Poll            *PollCreateRequest      `json:"poll,omitempty"`
	Files           []*File                 `json:"-"`

	// Nonce identifies the message in MESSAGE_CREATE. With EnforceNonce, a
	// second message with the same nonce from the bot within a few minutes is
	// not created, and the first one is returned instead.
	Nonce        string `json:"nonce,omitempty"`
	EnforceNonce bool   `json:"enforce_nonce,omitempty"`
}

// PollLayoutType is how a poll is shown.
type PollLayoutType int

// PollLayoutDefault is the only layout.
const PollLayoutDefault PollLayoutType = 1

// PollMedia is the text of a poll question or answer. Questions only support
// Text, of up to 300 characters; answers up to 55, with an optional Emoji (send
// only its ID or Name).
type PollMedia struct {
	Text  string          `json:"text,omitempty"`
	Emoji *ComponentEmoji `json:"emoji,omitempty"`
}

// PollAnswer is one choice of a poll. AnswerID is set by Discord.
type PollAnswer struct {
	AnswerID  int       `json:"answer_id,omitempty"`
	PollMedia PollMedia `json:"poll_media"`
}

// PollCreateRequest is a poll to send with a message. It can't be combined
// with MessageFlagsIsComponentsV2.
type PollCreateRequest struct {
	Question         PollMedia      `json:"question"`
	Answers          []PollAnswer   `json:"answers"`                     // up to 10
	Duration         int            `json:"duration,omitempty"`          // hours, up to 32 days; Discord defaults to 24
	AllowMultiselect bool           `json:"allow_multiselect,omitempty"` // let users pick more than one answer
	LayoutType       PollLayoutType `json:"layout_type,omitempty"`
}

// Poll is a poll on a received message.
type Poll struct {
	Question         PollMedia      `json:"question"`
	Answers          []PollAnswer   `json:"answers"`
	Expiry           *time.Time     `json:"expiry"`
	AllowMultiselect bool           `json:"allow_multiselect"`
	LayoutType       PollLayoutType `json:"layout_type"`

	// Results is nil when the counts are not known, which does not mean that
	// there are no votes.
	Results *PollResults `json:"results"`
}

// PollResults are the vote counts of a poll.
type PollResults struct {
	IsFinalized  bool              `json:"is_finalized"`
	AnswerCounts []PollAnswerCount `json:"answer_counts"` // an answer without votes may be missing
}

// PollAnswerCount is the number of votes for an answer.
type PollAnswerCount struct {
	ID      int  `json:"id"` // the AnswerID
	Count   int  `json:"count"`
	MeVoted bool `json:"me_voted"`
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

// Emoji is an emoji, of a reaction, a guild or an application. ID is empty
// for a standard Unicode emoji.
type Emoji struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Animated bool   `json:"animated"`

	// The fields below are only set for the emojis of a guild or application.
	Roles         []string `json:"roles"` // the roles allowed to use it; empty means everyone
	User          *User    `json:"user"`  // who created it
	RequireColons bool     `json:"require_colons"`
	Managed       bool     `json:"managed"`
	Available     bool     `json:"available"`
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

	// The fields below are only received, on the activities of other users.
	Details       string              `json:"details,omitempty"`
	ApplicationID string              `json:"application_id,omitempty"`
	CreatedAt     int64               `json:"created_at,omitempty"` // Unix milliseconds
	Timestamps    *ActivityTimestamps `json:"timestamps,omitempty"`
	Emoji         *Emoji              `json:"emoji,omitempty"`
}

// ActivityTimestamps are when an activity started and ends, in Unix milliseconds.
type ActivityTimestamps struct {
	Start int64 `json:"start,omitempty"`
	End   int64 `json:"end,omitempty"`
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
	UserID          string       `json:"user_id"`
	ChannelID       string       `json:"channel_id"`
	MessageID       string       `json:"message_id"`
	GuildID         string       `json:"guild_id"`
	MessageAuthorID string       `json:"message_author_id"`
	Member          *Member      `json:"member"`
	Emoji           *Emoji       `json:"emoji"`
	Burst           bool         `json:"burst"`
	BurstColors     []string     `json:"burst_colors"`
	Type            ReactionType `json:"type"`
}

// jsonNull reports whether raw is absent or a JSON null.
func jsonNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}
