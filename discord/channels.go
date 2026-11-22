package discord

import "time"

// Channel is a (possibly partial) channel. Threads are channels too: for
// them ParentID is the channel they are in, and OwnerID, ThreadMetadata,
// MessageCount and MemberCount are set.
type Channel struct {
	ID                   string                 `json:"id"`
	GuildID              string                 `json:"guild_id"`
	Name                 string                 `json:"name"`
	Type                 ChannelType            `json:"type"`
	Topic                string                 `json:"topic"`
	NSFW                 bool                   `json:"nsfw"`
	ParentID             string                 `json:"parent_id"`
	Position             int                    `json:"position"`
	PermissionOverwrites []*PermissionOverwrite `json:"permission_overwrites"`
	LastMessageID        string                 `json:"last_message_id"`
	LastPinTimestamp     *time.Time             `json:"last_pin_timestamp"`
	RateLimitPerUser     int                    `json:"rate_limit_per_user"` // slowmode, in seconds
	Flags                ChannelFlags           `json:"flags"`

	// Permissions are those of the user who triggered an interaction, on the
	// channels resolved for it.
	Permissions int64 `json:"permissions,string"`

	// Voice and stage channels.
	Bitrate          int              `json:"bitrate"`
	UserLimit        int              `json:"user_limit"`
	RTCRegion        string           `json:"rtc_region"`
	VideoQualityMode VideoQualityMode `json:"video_quality_mode"`

	// DMs.
	Recipients []*User `json:"recipients"`

	// Group DMs: Icon, and the app that made the channel if it is Managed.
	Icon          string `json:"icon"`
	ApplicationID string `json:"application_id"`
	Managed       bool   `json:"managed"`

	// Threads.
	OwnerID          string          `json:"owner_id"`
	ThreadMetadata   *ThreadMetadata `json:"thread_metadata"`
	Member           *ThreadMember   `json:"member"` // the bot's, if it joined
	MessageCount     int             `json:"message_count"`
	MemberCount      int             `json:"member_count"` // stops counting at 50
	TotalMessageSent int             `json:"total_message_sent"`
	AppliedTags      []string        `json:"applied_tags"` // in a forum or media channel

	// Channels that hold threads.
	DefaultAutoArchiveDuration    int `json:"default_auto_archive_duration"`
	DefaultThreadRateLimitPerUser int `json:"default_thread_rate_limit_per_user"`

	// Forum and media channels.
	AvailableTags        []*ForumTag      `json:"available_tags"`
	DefaultReactionEmoji *DefaultReaction `json:"default_reaction_emoji"`
	DefaultSortOrder     *ForumSortOrder  `json:"default_sort_order"`
	DefaultForumLayout   ForumLayout      `json:"default_forum_layout"`
}

// IsThread reports whether the channel is a thread.
func (c *Channel) IsThread() bool {
	switch c.Type {
	case ChannelTypeGuildNewsThread, ChannelTypeGuildPublicThread, ChannelTypeGuildPrivateThread:
		return true
	}
	return false
}

// ChannelFlags is a bitfield of channel options.
type ChannelFlags int

const (
	ChannelFlagPinned                   ChannelFlags = 1 << 1 // a thread pinned in its forum
	ChannelFlagRequireTag               ChannelFlags = 1 << 4 // a forum whose threads need a tag
	ChannelFlagHideMediaDownloadOptions ChannelFlags = 1 << 15

	// ChannelFlagObfuscated marks a channel received over the gateway whose
	// name and other details are hidden because the bot cannot view it.
	ChannelFlagObfuscated ChannelFlags = 1 << 17

	ChannelFlagIsSpoilerChannel ChannelFlags = 1 << 21 // members opt in to see its contents
)

// VideoQualityMode is the camera quality of a voice channel.
type VideoQualityMode int

const (
	VideoQualityAuto VideoQualityMode = 1
	VideoQualityFull VideoQualityMode = 2 // 720p
)

// ForumSortOrder is how the threads of a forum are sorted.
type ForumSortOrder int

const (
	ForumSortLatestActivity ForumSortOrder = 0
	ForumSortCreationDate   ForumSortOrder = 1
)

// ForumLayout is how the threads of a forum are shown.
type ForumLayout int

const (
	ForumLayoutNotSet      ForumLayout = 0
	ForumLayoutListView    ForumLayout = 1
	ForumLayoutGalleryView ForumLayout = 2
)

// PermissionOverwriteType is what a PermissionOverwrite applies to.
type PermissionOverwriteType int

const (
	PermissionOverwriteRole   PermissionOverwriteType = 0
	PermissionOverwriteMember PermissionOverwriteType = 1
)

// PermissionOverwrite changes the permissions of a role or member in a channel.
type PermissionOverwrite struct {
	ID    string                  `json:"id"` // of the role or user
	Type  PermissionOverwriteType `json:"type"`
	Allow int64                   `json:"allow,string"`
	Deny  int64                   `json:"deny,string"`
}

// ThreadMetadata is the state of a thread.
type ThreadMetadata struct {
	Archived            bool       `json:"archived"`
	AutoArchiveDuration int        `json:"auto_archive_duration"` // minutes: 60, 1440, 4320 or 10080
	ArchiveTimestamp    time.Time  `json:"archive_timestamp"`
	Locked              bool       `json:"locked"`
	Invitable           bool       `json:"invitable"` // private threads: whether non-moderators can add members
	CreateTimestamp     *time.Time `json:"create_timestamp"`
}

// ThreadMember is a user's membership of a thread. ID and UserID are absent
// from the one inside a Channel, and Member is only set when asked for.
type ThreadMember struct {
	ID            string    `json:"id"` // of the thread
	UserID        string    `json:"user_id"`
	JoinTimestamp time.Time `json:"join_timestamp"`
	Flags         int       `json:"flags"`
	Member        *Member   `json:"member"`
}

// ForumTag is a tag that can be applied to the threads of a forum or media
// channel. Set at most one of EmojiID and EmojiName.
type ForumTag struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	Moderated bool   `json:"moderated"` // only members who can manage threads may apply it
	EmojiID   string `json:"emoji_id,omitempty"`
	EmojiName string `json:"emoji_name,omitempty"`
}

// DefaultReaction is the emoji of the reaction button on a forum's threads.
// Set one of the fields.
type DefaultReaction struct {
	EmojiID   string `json:"emoji_id,omitempty"`
	EmojiName string `json:"emoji_name,omitempty"`
}

// ChannelParams is the body of a Create Guild Channel or Modify Channel
// request. A nil or empty field keeps Discord's default on create and is left
// unchanged on modify. Which fields apply depends on the type of the channel.
//
// For a thread, only Name, RateLimitPerUser, Flags, AppliedTags and the
// fields under "Threads" apply.
type ChannelParams struct {
	Name                 string                 `json:"name,omitempty"`
	Type                 *ChannelType           `json:"type,omitempty"` // modify: only between text and announcement
	Topic                *string                `json:"topic,omitempty"`
	NSFW                 *bool                  `json:"nsfw,omitempty"`
	Position             *int                   `json:"position,omitempty"`
	ParentID             Optional[string]       `json:"parent_id,omitzero"` // the category; Null takes it out
	PermissionOverwrites *[]PermissionOverwrite `json:"permission_overwrites,omitempty"`
	RateLimitPerUser     *int                   `json:"rate_limit_per_user,omitempty"`
	Flags                *ChannelFlags          `json:"flags,omitempty"`

	// Voice and stage channels.
	Bitrate          *int             `json:"bitrate,omitempty"`
	UserLimit        *int             `json:"user_limit,omitempty"`
	RTCRegion        Optional[string] `json:"rtc_region,omitzero"` // Null picks one automatically
	VideoQualityMode VideoQualityMode `json:"video_quality_mode,omitempty"`

	// Channels that hold threads.
	DefaultAutoArchiveDuration    *int `json:"default_auto_archive_duration,omitempty"`
	DefaultThreadRateLimitPerUser *int `json:"default_thread_rate_limit_per_user,omitempty"`

	// Forum and media channels.
	AvailableTags        *[]ForumTag               `json:"available_tags,omitempty"`
	DefaultReactionEmoji Optional[DefaultReaction] `json:"default_reaction_emoji,omitzero"`
	DefaultSortOrder     Optional[ForumSortOrder]  `json:"default_sort_order,omitzero"`
	DefaultForumLayout   *ForumLayout              `json:"default_forum_layout,omitempty"`

	// Threads (modify only).
	Archived            *bool     `json:"archived,omitempty"`
	AutoArchiveDuration *int      `json:"auto_archive_duration,omitempty"`
	Locked              *bool     `json:"locked,omitempty"`
	Invitable           *bool     `json:"invitable,omitempty"`
	AppliedTags         *[]string `json:"applied_tags,omitempty"`
}

// ChannelPosition moves a channel in a Modify Guild Channel Positions request.
type ChannelPosition struct {
	ID              string           `json:"id"`
	Position        Optional[int]    `json:"position,omitzero"`
	LockPermissions *bool            `json:"lock_permissions,omitempty"` // sync the overwrites with the new parent
	ParentID        Optional[string] `json:"parent_id,omitzero"`
}

// ThreadParams is the body of a request that starts a thread, from a message
// or on its own.
type ThreadParams struct {
	Name                string `json:"name"`
	AutoArchiveDuration int    `json:"auto_archive_duration,omitempty"` // minutes: 60, 1440, 4320 or 10080
	RateLimitPerUser    *int   `json:"rate_limit_per_user,omitempty"`

	// The fields below only apply to a thread without a message. Discord
	// defaults Type to a private thread.
	Type      ChannelType `json:"type,omitempty"`
	Invitable *bool       `json:"invitable,omitempty"`
}

// ForumThreadParams is the body of a request that starts a thread in a forum
// or media channel. Message is its first message.
type ForumThreadParams struct {
	Name                string       `json:"name"`
	AutoArchiveDuration int          `json:"auto_archive_duration,omitempty"`
	RateLimitPerUser    *int         `json:"rate_limit_per_user,omitempty"`
	AppliedTags         []string     `json:"applied_tags,omitempty"`
	Message             *MessageSend `json:"message"`
}

// ForumThread is a thread just started in a forum or media channel, with its
// first message.
type ForumThread struct {
	Channel
	Message *Message `json:"message"`
}

// ThreadList is a list of threads, with the bot's memberships of those it
// joined. HasMore says whether a further page of archived threads exists.
type ThreadList struct {
	Threads []*Channel      `json:"threads"`
	Members []*ThreadMember `json:"members"`
	HasMore bool            `json:"has_more"`
}

// FollowedChannel says which webhook delivers a followed announcement channel.
type FollowedChannel struct {
	ChannelID string `json:"channel_id"`
	WebhookID string `json:"webhook_id"`
}

// InviteTargetType is what an invite to a voice channel opens.
type InviteTargetType int

const (
	InviteTargetStream              InviteTargetType = 1
	InviteTargetEmbeddedApplication InviteTargetType = 2
)

// Invite is an invite to a guild or channel. The fields from Uses on are only
// set when listing the invites of a guild or channel, and in the result of
// creating one.
type Invite struct {
	Code                     string               `json:"code"`
	Type                     int                  `json:"type"`    // 0 guild, 1 group DM, 2 friend
	Guild                    *Guild               `json:"guild"`   // partial
	Channel                  *Channel             `json:"channel"` // partial
	Inviter                  *User                `json:"inviter"`
	TargetType               InviteTargetType     `json:"target_type"`
	TargetUser               *User                `json:"target_user"`
	TargetApplication        *Application         `json:"target_application"`
	ApproximatePresenceCount int                  `json:"approximate_presence_count"`
	ApproximateMemberCount   int                  `json:"approximate_member_count"`
	ExpiresAt                *time.Time           `json:"expires_at"`
	GuildScheduledEvent      *GuildScheduledEvent `json:"guild_scheduled_event"`

	Uses      int       `json:"uses"`
	MaxUses   int       `json:"max_uses"`
	MaxAge    int       `json:"max_age"` // seconds
	Temporary bool      `json:"temporary"`
	CreatedAt time.Time `json:"created_at"`
}

// InviteParams is the body of a Create Channel Invite request.
type InviteParams struct {
	MaxAge              *int             `json:"max_age,omitempty"`  // seconds; 0 never expires, Discord defaults to a day
	MaxUses             *int             `json:"max_uses,omitempty"` // 0 is unlimited, the default
	Temporary           bool             `json:"temporary,omitempty"`
	Unique              bool             `json:"unique,omitempty"` // do not reuse a similar invite
	TargetType          InviteTargetType `json:"target_type,omitempty"`
	TargetUserID        string           `json:"target_user_id,omitempty"`
	TargetApplicationID string           `json:"target_application_id,omitempty"`
}

// StickerType is where a sticker comes from.
type StickerType int

const (
	StickerTypeStandard StickerType = 1 // from a pack by Discord
	StickerTypeGuild    StickerType = 2
)

// StickerFormat is the file format of a sticker.
type StickerFormat int

const (
	StickerFormatPNG    StickerFormat = 1
	StickerFormatAPNG   StickerFormat = 2
	StickerFormatLottie StickerFormat = 3
	StickerFormatGIF    StickerFormat = 4
)

// Sticker is a sticker that can be sent in messages.
type Sticker struct {
	ID          string        `json:"id"`
	PackID      string        `json:"pack_id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Tags        string        `json:"tags"` // autocomplete suggestions, comma separated
	Type        StickerType   `json:"type"`
	FormatType  StickerFormat `json:"format_type"`
	Available   bool          `json:"available"`
	GuildID     string        `json:"guild_id"`
	User        *User         `json:"user"`
	SortValue   int           `json:"sort_value"`
}

// StickerItem is the part of a sticker sent with a message.
type StickerItem struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	FormatType StickerFormat `json:"format_type"`
}

// StickerPack is a pack of standard stickers.
type StickerPack struct {
	ID             string     `json:"id"`
	Stickers       []*Sticker `json:"stickers"`
	Name           string     `json:"name"`
	SKUID          string     `json:"sku_id"`
	CoverStickerID string     `json:"cover_sticker_id"`
	Description    string     `json:"description"`
	BannerAssetID  string     `json:"banner_asset_id"`
}

// StickerParams is the body of a Create Guild Sticker request. File is a PNG,
// APNG, GIF or Lottie JSON file of at most 512 KiB.
type StickerParams struct {
	Name        string
	Description string
	Tags        string // autocomplete suggestions, such as the name of an emoji
	File        *File
}

// StickerEdit is the body of a Modify Guild Sticker request.
type StickerEdit struct {
	Name        string  `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Tags        string  `json:"tags,omitempty"`
}

// EmojiParams is the body of a request that creates or changes an emoji.
// Image is a data URI (see ImageData), needed to create one and ignored
// otherwise; Roles only applies to the emojis of a guild.
type EmojiParams struct {
	Name  string    `json:"name,omitempty"`
	Image string    `json:"image,omitempty"`
	Roles *[]string `json:"roles,omitempty"` // the roles allowed to use it
}

// WebhookType is the kind of a webhook.
type WebhookType int

const (
	WebhookTypeIncoming        WebhookType = 1
	WebhookTypeChannelFollower WebhookType = 2 // posts the messages of a followed channel
	WebhookTypeApplication     WebhookType = 3 // used with interactions
)

// Webhook posts messages to a channel without a bot user. Token is only set
// for incoming webhooks, and only when the bot may see it.
type Webhook struct {
	ID            string      `json:"id"`
	Type          WebhookType `json:"type"`
	GuildID       string      `json:"guild_id"`
	ChannelID     string      `json:"channel_id"`
	User          *User       `json:"user"` // who created it
	Name          string      `json:"name"`
	Avatar        string      `json:"avatar"`
	Token         string      `json:"token"`
	ApplicationID string      `json:"application_id"`
	SourceGuild   *Guild      `json:"source_guild"`   // channel followers
	SourceChannel *Channel    `json:"source_channel"` // channel followers
	URL           string      `json:"url"`
}

// WebhookSettings is the body of a request that creates or changes a webhook.
// ChannelID moves it, which needs the bot token.
type WebhookSettings struct {
	Name      string           `json:"name,omitempty"`
	Avatar    Optional[string] `json:"avatar,omitzero"` // a data URI; see ImageData
	ChannelID string           `json:"channel_id,omitempty"`
}

// WebhookMessage is the body of an Execute Webhook request.
type WebhookMessage struct {
	WebhookParams

	Username  string `json:"username,omitempty"`   // overrides the webhook's name
	AvatarURL string `json:"avatar_url,omitempty"` // overrides the webhook's avatar

	// ThreadName starts a thread with this name in a forum or media channel,
	// with AppliedTags as its tags.
	ThreadName  string   `json:"thread_name,omitempty"`
	AppliedTags []string `json:"applied_tags,omitempty"`

	// ThreadID sends to a thread of the webhook's channel instead.
	ThreadID string `json:"-"`

	// WithComponents makes Discord take Components from a webhook that is
	// not owned by an app, which can then only use non-interactive ones.
	WithComponents bool `json:"-"`
}

// StagePrivacyLevel is who can see a stage instance or scheduled event.
type StagePrivacyLevel int

// StagePrivacyGuildOnly is the only level in use.
const StagePrivacyGuildOnly StagePrivacyLevel = 2

// StageInstance is a live stage.
type StageInstance struct {
	ID                    string            `json:"id"`
	GuildID               string            `json:"guild_id"`
	ChannelID             string            `json:"channel_id"`
	Topic                 string            `json:"topic"`
	PrivacyLevel          StagePrivacyLevel `json:"privacy_level"`
	GuildScheduledEventID string            `json:"guild_scheduled_event_id"`
}

// StageInstanceParams is the body of a request that starts or changes a stage
// instance. ChannelID, SendStartNotification and GuildScheduledEventID only
// apply when starting one.
type StageInstanceParams struct {
	ChannelID             string            `json:"channel_id,omitempty"`
	Topic                 string            `json:"topic,omitempty"`
	PrivacyLevel          StagePrivacyLevel `json:"privacy_level,omitempty"`
	SendStartNotification bool              `json:"send_start_notification,omitempty"` // notifies @everyone
	GuildScheduledEventID string            `json:"guild_scheduled_event_id,omitempty"`
}

// MessagesQuery selects the messages a channel is asked for. Set at most one
// of Before, After and Around, each a message ID.
type MessagesQuery struct {
	Limit  int // up to 100; Discord defaults to 50
	Before string
	After  string
	Around string
}

// ReactionType is the kind of a reaction.
type ReactionType int

const (
	ReactionNormal ReactionType = 0
	ReactionBurst  ReactionType = 1 // a super reaction
)
