package discord

import (
	"encoding/json"
	"time"
)

// UnmarshalJSON implements json.Unmarshaler, so that the Interaction decodes
// its Data according to its type.
func (e *InteractionCreate) UnmarshalJSON(data []byte) error {
	e.Interaction = &Interaction{}
	return json.Unmarshal(data, e.Interaction)
}

// Resumed is the RESUMED gateway event: the session picked up where a lost
// connection left off, without missing events.
type Resumed struct{}

// GuildUpdate is the GUILD_UPDATE gateway event.
type GuildUpdate struct {
	*Guild
}

// GuildDelete is the GUILD_DELETE gateway event. The bot was removed from
// the guild unless Unavailable is set, which means an outage.
type GuildDelete struct {
	ID          string `json:"id"`
	Unavailable bool   `json:"unavailable"`
}

// GuildAuditLogEntryCreate is the GUILD_AUDIT_LOG_ENTRY_CREATE gateway
// event. It needs the View Audit Log permission.
type GuildAuditLogEntryCreate struct {
	*AuditLogEntry
	GuildID string `json:"guild_id"`
}

// GuildBanAdd is the GUILD_BAN_ADD gateway event.
type GuildBanAdd struct {
	GuildID string `json:"guild_id"`
	User    *User  `json:"user"`
}

// GuildBanRemove is the GUILD_BAN_REMOVE gateway event.
type GuildBanRemove struct {
	GuildID string `json:"guild_id"`
	User    *User  `json:"user"`
}

// GuildEmojisUpdate is the GUILD_EMOJIS_UPDATE gateway event. Emojis is the
// guild's whole list.
type GuildEmojisUpdate struct {
	GuildID string   `json:"guild_id"`
	Emojis  []*Emoji `json:"emojis"`
}

// GuildStickersUpdate is the GUILD_STICKERS_UPDATE gateway event. Stickers
// is the guild's whole list.
type GuildStickersUpdate struct {
	GuildID  string     `json:"guild_id"`
	Stickers []*Sticker `json:"stickers"`
}

// GuildIntegrationsUpdate is the GUILD_INTEGRATIONS_UPDATE gateway event.
type GuildIntegrationsUpdate struct {
	GuildID string `json:"guild_id"`
}

// GuildMemberUpdate is the GUILD_MEMBER_UPDATE gateway event. It needs the
// privileged IntentsGuildMembers intent.
type GuildMemberUpdate struct {
	*Member
	GuildID string `json:"guild_id"`
}

// GuildMembersChunk is the GUILD_MEMBERS_CHUNK gateway event, the answer to
// Session.RequestGuildMembers. A request is answered in ChunkCount chunks.
type GuildMembersChunk struct {
	GuildID    string      `json:"guild_id"`
	Members    []*Member   `json:"members"`
	ChunkIndex int         `json:"chunk_index"`
	ChunkCount int         `json:"chunk_count"`
	NotFound   []string    `json:"not_found"` // requested user IDs that are not members
	Presences  []*Presence `json:"presences"`
	Nonce      string      `json:"nonce"`
}

// GuildRoleCreate is the GUILD_ROLE_CREATE gateway event.
type GuildRoleCreate struct {
	GuildID string `json:"guild_id"`
	Role    *Role  `json:"role"`
}

// GuildRoleUpdate is the GUILD_ROLE_UPDATE gateway event.
type GuildRoleUpdate struct {
	GuildID string `json:"guild_id"`
	Role    *Role  `json:"role"`
}

// GuildRoleDelete is the GUILD_ROLE_DELETE gateway event.
type GuildRoleDelete struct {
	GuildID string `json:"guild_id"`
	RoleID  string `json:"role_id"`
}

// ChannelCreate is the CHANNEL_CREATE gateway event.
type ChannelCreate struct {
	*Channel
}

// ChannelUpdate is the CHANNEL_UPDATE gateway event.
type ChannelUpdate struct {
	*Channel
}

// ChannelDelete is the CHANNEL_DELETE gateway event.
type ChannelDelete struct {
	*Channel
}

// ChannelPinsUpdate is the CHANNEL_PINS_UPDATE gateway event.
type ChannelPinsUpdate struct {
	GuildID          string     `json:"guild_id"`
	ChannelID        string     `json:"channel_id"`
	LastPinTimestamp *time.Time `json:"last_pin_timestamp"`
}

// ThreadCreate is the THREAD_CREATE gateway event, sent when a thread is
// created and when the bot is added to a private one.
type ThreadCreate struct {
	*Channel
	NewlyCreated bool `json:"newly_created"`
}

// ThreadUpdate is the THREAD_UPDATE gateway event.
type ThreadUpdate struct {
	*Channel
}

// ThreadDelete is the THREAD_DELETE gateway event.
type ThreadDelete struct {
	ID       string      `json:"id"`
	GuildID  string      `json:"guild_id"`
	ParentID string      `json:"parent_id"`
	Type     ChannelType `json:"type"`
}

// ThreadListSync is the THREAD_LIST_SYNC gateway event, sent when the bot
// gains access to a channel. Threads are the active threads of ChannelIDs,
// or of the whole guild if that is empty.
type ThreadListSync struct {
	GuildID    string          `json:"guild_id"`
	ChannelIDs []string        `json:"channel_ids"`
	Threads    []*Channel      `json:"threads"`
	Members    []*ThreadMember `json:"members"` // the bot's
}

// ThreadMemberUpdate is the THREAD_MEMBER_UPDATE gateway event, about the
// bot's own membership of a thread.
type ThreadMemberUpdate struct {
	*ThreadMember
	GuildID string `json:"guild_id"`
}

// ThreadMembersUpdate is the THREAD_MEMBERS_UPDATE gateway event. Without
// the privileged IntentsGuildMembers intent it only reports the bot.
type ThreadMembersUpdate struct {
	ID               string          `json:"id"` // of the thread
	GuildID          string          `json:"guild_id"`
	MemberCount      int             `json:"member_count"` // stops counting at 50
	AddedMembers     []*ThreadMember `json:"added_members"`
	RemovedMemberIDs []string        `json:"removed_member_ids"`
}

// MessageDeleteBulk is the MESSAGE_DELETE_BULK gateway event.
type MessageDeleteBulk struct {
	IDs       []string `json:"ids"`
	ChannelID string   `json:"channel_id"`
	GuildID   string   `json:"guild_id"`
}

// MessageReactionRemove is the MESSAGE_REACTION_REMOVE gateway event.
type MessageReactionRemove struct {
	UserID    string       `json:"user_id"`
	ChannelID string       `json:"channel_id"`
	MessageID string       `json:"message_id"`
	GuildID   string       `json:"guild_id"`
	Emoji     *Emoji       `json:"emoji"`
	Burst     bool         `json:"burst"`
	Type      ReactionType `json:"type"`
}

// MessageReactionRemoveAll is the MESSAGE_REACTION_REMOVE_ALL gateway event.
type MessageReactionRemoveAll struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
	GuildID   string `json:"guild_id"`
}

// MessageReactionRemoveEmoji is the MESSAGE_REACTION_REMOVE_EMOJI gateway
// event: every reaction with one emoji was removed from a message.
type MessageReactionRemoveEmoji struct {
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
	GuildID   string `json:"guild_id"`
	Emoji     *Emoji `json:"emoji"`
}

// MessagePollVoteAdd is the MESSAGE_POLL_VOTE_ADD gateway event.
type MessagePollVoteAdd struct {
	UserID    string `json:"user_id"`
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
	GuildID   string `json:"guild_id"`
	AnswerID  int    `json:"answer_id"`
}

// MessagePollVoteRemove is the MESSAGE_POLL_VOTE_REMOVE gateway event.
type MessagePollVoteRemove struct {
	UserID    string `json:"user_id"`
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
	GuildID   string `json:"guild_id"`
	AnswerID  int    `json:"answer_id"`
}

// TypingStart is the TYPING_START gateway event.
type TypingStart struct {
	ChannelID string  `json:"channel_id"`
	GuildID   string  `json:"guild_id"`
	UserID    string  `json:"user_id"`
	Timestamp int64   `json:"timestamp"` // Unix seconds
	Member    *Member `json:"member"`
}

// PresenceUpdateEvent is the PRESENCE_UPDATE gateway event. It needs the
// privileged IntentsGuildPresences intent. (PresenceUpdate is the command
// that sets the bot's own presence.)
type PresenceUpdateEvent struct {
	*Presence
}

// UserUpdate is the USER_UPDATE gateway event: the bot's own user changed.
type UserUpdate struct {
	*User
}

// VoiceStateUpdate is the VOICE_STATE_UPDATE gateway event.
type VoiceStateUpdate struct {
	*VoiceState
}

// VoiceServerUpdate is the VOICE_SERVER_UPDATE gateway event, sent after the
// bot joins a voice channel and when its voice server changes. Endpoint is
// empty while a new server is being allocated.
type VoiceServerUpdate struct {
	Token    string `json:"token"`
	GuildID  string `json:"guild_id"`
	Endpoint string `json:"endpoint"`
}

// VoiceChannelEffectSend is the VOICE_CHANNEL_EFFECT_SEND gateway event:
// someone sent an emoji reaction or a soundboard sound in a voice channel.
type VoiceChannelEffectSend struct {
	ChannelID     string          `json:"channel_id"`
	GuildID       string          `json:"guild_id"`
	UserID        string          `json:"user_id"`
	Emoji         *Emoji          `json:"emoji"`
	AnimationType *int            `json:"animation_type"`
	AnimationID   int             `json:"animation_id"`
	SoundID       json.RawMessage `json:"sound_id"` // a string, or an integer for a default sound
	SoundVolume   float64         `json:"sound_volume"`
}

// VoiceChannelStatusUpdate is the VOICE_CHANNEL_STATUS_UPDATE gateway event.
// Status is empty once the status is cleared.
type VoiceChannelStatusUpdate struct {
	ID      string `json:"id"` // of the channel
	GuildID string `json:"guild_id"`
	Status  string `json:"status"`
}

// VoiceChannelStartTimeUpdate is the VOICE_CHANNEL_START_TIME_UPDATE gateway
// event. VoiceStartTime is when the voice session of the channel started, in
// Unix seconds, or nil when there is none.
type VoiceChannelStartTimeUpdate struct {
	ID             string `json:"id"` // of the channel
	GuildID        string `json:"guild_id"`
	VoiceStartTime *int64 `json:"voice_start_time"`
}

// ChannelInfo is the CHANNEL_INFO gateway event, the answer to
// Session.RequestChannelInfo.
type ChannelInfo struct {
	GuildID  string                `json:"guild_id"`
	Channels []*ChannelInfoChannel `json:"channels"`
}

// ChannelInfoChannel is the short-lived state of a voice channel: the fields
// that were asked for, where the channel has them.
type ChannelInfoChannel struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	VoiceStartTime *int64 `json:"voice_start_time"` // Unix seconds
}

// RateLimited is the RATE_LIMITED gateway event: Discord refused a gateway
// command, such as a request for all members of a guild made too soon after
// the last one.
type RateLimited struct {
	Opcode     int               `json:"opcode"`      // of the refused command
	RetryAfter float64           `json:"retry_after"` // seconds
	Meta       RateLimitMetadata `json:"meta"`
}

// RateLimitMetadata says which request a RateLimited event is about. It
// holds the fields of a refused Request Guild Members command.
type RateLimitMetadata struct {
	GuildID string `json:"guild_id"`
	Nonce   string `json:"nonce"`
}

// WebhooksUpdate is the WEBHOOKS_UPDATE gateway event.
type WebhooksUpdate struct {
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
}

// InviteCreate is the INVITE_CREATE gateway event.
type InviteCreate struct {
	ChannelID         string           `json:"channel_id"`
	GuildID           string           `json:"guild_id"`
	Code              string           `json:"code"`
	CreatedAt         time.Time        `json:"created_at"`
	Inviter           *User            `json:"inviter"`
	MaxAge            int              `json:"max_age"`
	MaxUses           int              `json:"max_uses"`
	TargetType        InviteTargetType `json:"target_type"`
	TargetUser        *User            `json:"target_user"`
	TargetApplication *Application     `json:"target_application"`
	Temporary         bool             `json:"temporary"`
	Uses              int              `json:"uses"`
	ExpiresAt         *time.Time       `json:"expires_at"`
}

// InviteDelete is the INVITE_DELETE gateway event.
type InviteDelete struct {
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
	Code      string `json:"code"`
}

// IntegrationCreate is the INTEGRATION_CREATE gateway event.
type IntegrationCreate struct {
	*Integration
	GuildID string `json:"guild_id"`
}

// IntegrationUpdate is the INTEGRATION_UPDATE gateway event.
type IntegrationUpdate struct {
	*Integration
	GuildID string `json:"guild_id"`
}

// IntegrationDelete is the INTEGRATION_DELETE gateway event.
type IntegrationDelete struct {
	ID            string `json:"id"`
	GuildID       string `json:"guild_id"`
	ApplicationID string `json:"application_id"`
}

// ApplicationCommandPermissionsUpdate is the
// APPLICATION_COMMAND_PERMISSIONS_UPDATE gateway event.
type ApplicationCommandPermissionsUpdate struct {
	*GuildApplicationCommandPermissions
}

// AutoModerationRuleCreate is the AUTO_MODERATION_RULE_CREATE gateway event.
type AutoModerationRuleCreate struct {
	*AutoModerationRule
}

// AutoModerationRuleUpdate is the AUTO_MODERATION_RULE_UPDATE gateway event.
type AutoModerationRuleUpdate struct {
	*AutoModerationRule
}

// AutoModerationRuleDelete is the AUTO_MODERATION_RULE_DELETE gateway event.
type AutoModerationRuleDelete struct {
	*AutoModerationRule
}

// AutoModerationActionExecution is the AUTO_MODERATION_ACTION_EXECUTION
// gateway event: a rule was triggered and one of its actions carried out.
// Content and MatchedContent need the privileged IntentsMessageContent intent.
type AutoModerationActionExecution struct {
	GuildID              string                    `json:"guild_id"`
	Action               AutoModerationAction      `json:"action"`
	RuleID               string                    `json:"rule_id"`
	RuleTriggerType      AutoModerationTriggerType `json:"rule_trigger_type"`
	UserID               string                    `json:"user_id"`
	ChannelID            string                    `json:"channel_id"`
	MessageID            string                    `json:"message_id"`              // empty if the message was blocked
	AlertSystemMessageID string                    `json:"alert_system_message_id"` // of the alert the action posted
	Content              string                    `json:"content"`
	MatchedKeyword       string                    `json:"matched_keyword"`
	MatchedContent       string                    `json:"matched_content"`
}

// GuildScheduledEventCreate is the GUILD_SCHEDULED_EVENT_CREATE gateway event.
type GuildScheduledEventCreate struct {
	*GuildScheduledEvent
}

// GuildScheduledEventUpdate is the GUILD_SCHEDULED_EVENT_UPDATE gateway event.
type GuildScheduledEventUpdate struct {
	*GuildScheduledEvent
}

// GuildScheduledEventDelete is the GUILD_SCHEDULED_EVENT_DELETE gateway event.
type GuildScheduledEventDelete struct {
	*GuildScheduledEvent
}

// GuildScheduledEventUserAdd is the GUILD_SCHEDULED_EVENT_USER_ADD gateway
// event: a user became interested in a scheduled event.
type GuildScheduledEventUserAdd struct {
	GuildScheduledEventID string `json:"guild_scheduled_event_id"`
	UserID                string `json:"user_id"`
	GuildID               string `json:"guild_id"`
}

// GuildScheduledEventUserRemove is the GUILD_SCHEDULED_EVENT_USER_REMOVE
// gateway event.
type GuildScheduledEventUserRemove struct {
	GuildScheduledEventID string `json:"guild_scheduled_event_id"`
	UserID                string `json:"user_id"`
	GuildID               string `json:"guild_id"`
}

// StageInstanceCreate is the STAGE_INSTANCE_CREATE gateway event.
type StageInstanceCreate struct {
	*StageInstance
}

// StageInstanceUpdate is the STAGE_INSTANCE_UPDATE gateway event.
type StageInstanceUpdate struct {
	*StageInstance
}

// StageInstanceDelete is the STAGE_INSTANCE_DELETE gateway event.
type StageInstanceDelete struct {
	*StageInstance
}

// GuildSoundboardSoundCreate is the GUILD_SOUNDBOARD_SOUND_CREATE gateway event.
type GuildSoundboardSoundCreate struct {
	*SoundboardSound
}

// GuildSoundboardSoundUpdate is the GUILD_SOUNDBOARD_SOUND_UPDATE gateway event.
type GuildSoundboardSoundUpdate struct {
	*SoundboardSound
}

// GuildSoundboardSoundDelete is the GUILD_SOUNDBOARD_SOUND_DELETE gateway event.
type GuildSoundboardSoundDelete struct {
	SoundID string `json:"sound_id"`
	GuildID string `json:"guild_id"`
}

// GuildSoundboardSoundsUpdate is the GUILD_SOUNDBOARD_SOUNDS_UPDATE gateway
// event: several sounds of a guild changed at once.
type GuildSoundboardSoundsUpdate struct {
	SoundboardSounds []*SoundboardSound `json:"soundboard_sounds"`
	GuildID          string             `json:"guild_id"`
}

// SoundboardSounds is the SOUNDBOARD_SOUNDS gateway event, the answer to
// Session.RequestSoundboardSounds, sent once for each guild.
type SoundboardSounds struct {
	SoundboardSounds []*SoundboardSound `json:"soundboard_sounds"`
	GuildID          string             `json:"guild_id"`
}

// EntitlementCreate is the ENTITLEMENT_CREATE gateway event.
type EntitlementCreate struct {
	*Entitlement
}

// EntitlementUpdate is the ENTITLEMENT_UPDATE gateway event.
type EntitlementUpdate struct {
	*Entitlement
}

// EntitlementDelete is the ENTITLEMENT_DELETE gateway event, sent when an
// entitlement is refunded or removed, not when it expires.
type EntitlementDelete struct {
	*Entitlement
}

// SubscriptionCreate is the SUBSCRIPTION_CREATE gateway event.
type SubscriptionCreate struct {
	*Subscription
}

// SubscriptionUpdate is the SUBSCRIPTION_UPDATE gateway event.
type SubscriptionUpdate struct {
	*Subscription
}

// SubscriptionDelete is the SUBSCRIPTION_DELETE gateway event.
type SubscriptionDelete struct {
	*Subscription
}

// GuildMembersRequest is the body of the gateway's Request Guild Members
// command. Set either UserIDs, of up to 100 members, or Query, a prefix of
// their usernames. An empty Query with a Limit of 0 asks for every member,
// which needs the privileged IntentsGuildMembers intent.
type GuildMembersRequest struct {
	GuildID   string
	Query     string
	Limit     int // of the members matching Query; up to 100, or 0 for all with an empty Query
	UserIDs   []string
	Presences bool   // include presences; needs IntentsGuildPresences
	Nonce     string // repeated in the GuildMembersChunk events that answer
}

// MarshalJSON implements json.Marshaler: Discord wants query and limit
// exactly when there are no user IDs.
func (r GuildMembersRequest) MarshalJSON() ([]byte, error) {
	body := map[string]any{"guild_id": r.GuildID}
	if len(r.UserIDs) > 0 {
		body["user_ids"] = r.UserIDs
	} else {
		body["query"], body["limit"] = r.Query, r.Limit
	}
	if r.Presences {
		body["presences"] = true
	}
	if r.Nonce != "" {
		body["nonce"] = r.Nonce
	}
	return json.Marshal(body)
}
