package discord

import (
	"encoding/json"
	"time"
)

// Guild is a Discord server. The fields from JoinedAt on are only sent in the
// GUILD_CREATE event.
type Guild struct {
	ID                          string     `json:"id"`
	Name                        string     `json:"name"`
	Icon                        string     `json:"icon"`
	Splash                      string     `json:"splash"`
	DiscoverySplash             string     `json:"discovery_splash"`
	Banner                      string     `json:"banner"`
	Description                 string     `json:"description"`
	OwnerID                     string     `json:"owner_id"`
	AFKChannelID                string     `json:"afk_channel_id"`
	AFKTimeout                  int        `json:"afk_timeout"` // seconds
	WidgetEnabled               bool       `json:"widget_enabled"`
	WidgetChannelID             string     `json:"widget_channel_id"`
	VerificationLevel           int        `json:"verification_level"`
	DefaultMessageNotifications int        `json:"default_message_notifications"`
	ExplicitContentFilter       int        `json:"explicit_content_filter"`
	Roles                       []*Role    `json:"roles"`
	Emojis                      []*Emoji   `json:"emojis"`
	Stickers                    []*Sticker `json:"stickers"`
	Features                    []string   `json:"features"`
	MFALevel                    int        `json:"mfa_level"`
	SystemChannelID             string     `json:"system_channel_id"`
	SystemChannelFlags          int        `json:"system_channel_flags"`
	RulesChannelID              string     `json:"rules_channel_id"`
	PublicUpdatesChannelID      string     `json:"public_updates_channel_id"`
	SafetyAlertsChannelID       string     `json:"safety_alerts_channel_id"`
	VanityURLCode               string     `json:"vanity_url_code"`
	PremiumTier                 int        `json:"premium_tier"`
	PremiumSubscriptionCount    int        `json:"premium_subscription_count"`
	PremiumProgressBarEnabled   bool       `json:"premium_progress_bar_enabled"`
	PreferredLocale             Locale     `json:"preferred_locale"`
	Locale                      Locale     `json:"locale"` // only on the partial guild of an interaction
	NSFWLevel                   int        `json:"nsfw_level"`
	MaxMembers                  int        `json:"max_members"`
	ApproximateMemberCount      int        `json:"approximate_member_count"`
	ApproximatePresenceCount    int        `json:"approximate_presence_count"`

	IncidentsData *IncidentsData `json:"incidents_data"`

	// Owner and Permissions are about the bot, in Session.CurrentUserGuilds.
	Owner       bool  `json:"owner"`
	Permissions int64 `json:"permissions,string"`

	JoinedAt             *time.Time             `json:"joined_at"`
	Large                bool                   `json:"large"`
	Unavailable          bool                   `json:"unavailable"`
	MemberCount          int                    `json:"member_count"`
	VoiceStates          []*VoiceState          `json:"voice_states"`
	Members              []*Member              `json:"members"`
	Channels             []*Channel             `json:"channels"`
	Threads              []*Channel             `json:"threads"`
	Presences            []*Presence            `json:"presences"`
	StageInstances       []*StageInstance       `json:"stage_instances"`
	GuildScheduledEvents []*GuildScheduledEvent `json:"guild_scheduled_events"`
	SoundboardSounds     []*SoundboardSound     `json:"soundboard_sounds"`
}

// IncidentsData is the state of a guild's security actions.
type IncidentsData struct {
	InvitesDisabledUntil *time.Time `json:"invites_disabled_until"`
	DMsDisabledUntil     *time.Time `json:"dms_disabled_until"`
	DMSpamDetectedAt     *time.Time `json:"dm_spam_detected_at"`
	RaidDetectedAt       *time.Time `json:"raid_detected_at"`
}

// IncidentActions is the body of a Modify Guild Incident Actions request.
// Each can be set up to 24 hours ahead; Null turns it off.
type IncidentActions struct {
	InvitesDisabledUntil Optional[time.Time] `json:"invites_disabled_until,omitzero"`
	DMsDisabledUntil     Optional[time.Time] `json:"dms_disabled_until,omitzero"`
}

// GuildParams is the body of a Modify Guild request. Unset fields are left
// unchanged. The images are data URIs; see ImageData.
type GuildParams struct {
	Name                        string           `json:"name,omitempty"`
	Description                 Optional[string] `json:"description,omitzero"`
	VerificationLevel           Optional[int]    `json:"verification_level,omitzero"`
	DefaultMessageNotifications Optional[int]    `json:"default_message_notifications,omitzero"`
	ExplicitContentFilter       Optional[int]    `json:"explicit_content_filter,omitzero"`
	AFKChannelID                Optional[string] `json:"afk_channel_id,omitzero"`
	AFKTimeout                  *int             `json:"afk_timeout,omitempty"`
	Icon                        Optional[string] `json:"icon,omitzero"`
	Splash                      Optional[string] `json:"splash,omitzero"`
	DiscoverySplash             Optional[string] `json:"discovery_splash,omitzero"`
	Banner                      Optional[string] `json:"banner,omitzero"`
	SystemChannelID             Optional[string] `json:"system_channel_id,omitzero"`
	SystemChannelFlags          *int             `json:"system_channel_flags,omitempty"`
	RulesChannelID              Optional[string] `json:"rules_channel_id,omitzero"`
	PublicUpdatesChannelID      Optional[string] `json:"public_updates_channel_id,omitzero"`
	SafetyAlertsChannelID       Optional[string] `json:"safety_alerts_channel_id,omitzero"`
	PreferredLocale             Locale           `json:"preferred_locale,omitempty"`
	Features                    *[]string        `json:"features,omitempty"`
	PremiumProgressBarEnabled   *bool            `json:"premium_progress_bar_enabled,omitempty"`
}

// GuildPreview is what anyone can see of a discoverable guild.
type GuildPreview struct {
	ID                       string     `json:"id"`
	Name                     string     `json:"name"`
	Icon                     string     `json:"icon"`
	Splash                   string     `json:"splash"`
	DiscoverySplash          string     `json:"discovery_splash"`
	Emojis                   []*Emoji   `json:"emojis"`
	Features                 []string   `json:"features"`
	ApproximateMemberCount   int        `json:"approximate_member_count"`
	ApproximatePresenceCount int        `json:"approximate_presence_count"`
	Description              string     `json:"description"`
	Stickers                 []*Sticker `json:"stickers"`
}

// MemberParams is the body of a Modify Guild Member request. Unset fields
// are left unchanged.
type MemberParams struct {
	Nick  *string   `json:"nick,omitempty"` // a pointer to "" removes it
	Roles *[]string `json:"roles,omitempty"`
	Mute  *bool     `json:"mute,omitempty"` // for a member in a voice channel
	Deaf  *bool     `json:"deaf,omitempty"`
	Flags *int      `json:"flags,omitempty"`

	// ChannelID moves a member who is in a voice channel; Null disconnects them.
	ChannelID Optional[string] `json:"channel_id,omitzero"`

	// TimeoutUntil times the member out until then, up to 28 days ahead;
	// Null ends the timeout.
	TimeoutUntil Optional[time.Time] `json:"communication_disabled_until,omitzero"`
}

// CurrentMemberParams is the body of a Modify Current Member request. The
// images are data URIs (see ImageData); Null removes one.
type CurrentMemberParams struct {
	Nick   Optional[string] `json:"nick,omitzero"`
	Banner Optional[string] `json:"banner,omitzero"`
	Avatar Optional[string] `json:"avatar,omitzero"`
	Bio    Optional[string] `json:"bio,omitzero"`
}

// RolePosition moves a role in a Modify Guild Role Positions request.
type RolePosition struct {
	ID       string        `json:"id"`
	Position Optional[int] `json:"position,omitzero"`
}

// BulkBanResult lists the users a bulk ban banned and those it could not.
type BulkBanResult struct {
	BannedUsers []string `json:"banned_users"`
	FailedUsers []string `json:"failed_users"`
}

// PruneParams selects the members a prune removes: those who have not been
// seen for Days (1 to 30, Discord defaults to 7) and have no role, or only
// roles listed in IncludeRoles.
type PruneParams struct {
	Days         int
	IncludeRoles []string
}

// VoiceState is a user's connection to a voice channel.
type VoiceState struct {
	GuildID                 string     `json:"guild_id"`
	ChannelID               string     `json:"channel_id"` // empty once the user has left
	UserID                  string     `json:"user_id"`
	Member                  *Member    `json:"member"`
	SessionID               string     `json:"session_id"`
	Deaf                    bool       `json:"deaf"` // by the guild
	Mute                    bool       `json:"mute"`
	SelfDeaf                bool       `json:"self_deaf"`
	SelfMute                bool       `json:"self_mute"`
	SelfStream              bool       `json:"self_stream"`
	SelfVideo               bool       `json:"self_video"`
	Suppress                bool       `json:"suppress"` // not allowed to speak
	RequestToSpeakTimestamp *time.Time `json:"request_to_speak_timestamp"`
}

// VoiceStateParams is the body of a request that changes a voice state in a
// stage channel. RequestToSpeak only applies to the bot's own.
type VoiceStateParams struct {
	ChannelID      string              `json:"channel_id,omitempty"`
	Suppress       *bool               `json:"suppress,omitempty"`
	RequestToSpeak Optional[time.Time] `json:"request_to_speak_timestamp,omitzero"`
}

// VoiceRegion is a region voice servers are in.
type VoiceRegion struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Optimal    bool   `json:"optimal"`
	Deprecated bool   `json:"deprecated"`
	Custom     bool   `json:"custom"`
}

// Integration connects a guild to an app or an external service.
type Integration struct {
	ID                string              `json:"id"`
	Name              string              `json:"name"`
	Type              string              `json:"type"` // twitch, youtube, discord or guild_subscription
	Enabled           bool                `json:"enabled"`
	Syncing           bool                `json:"syncing"`
	RoleID            string              `json:"role_id"`
	EnableEmoticons   bool                `json:"enable_emoticons"`
	ExpireBehavior    int                 `json:"expire_behavior"`
	ExpireGracePeriod int                 `json:"expire_grace_period"`
	User              *User               `json:"user"`
	Account           *IntegrationAccount `json:"account"`
	SyncedAt          *time.Time          `json:"synced_at"`
	SubscriberCount   int                 `json:"subscriber_count"`
	Revoked           bool                `json:"revoked"`
	Application       *Application        `json:"application"` // partial, with its Bot
	Scopes            []string            `json:"scopes"`
}

// IntegrationAccount is the external account of an Integration.
type IntegrationAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// GuildWidgetSettings says whether a guild has a widget and where it invites to.
type GuildWidgetSettings struct {
	Enabled   bool   `json:"enabled"`
	ChannelID string `json:"channel_id"`
}

// GuildWidgetParams is the body of a Modify Guild Widget request.
type GuildWidgetParams struct {
	Enabled   *bool            `json:"enabled,omitempty"`
	ChannelID Optional[string] `json:"channel_id,omitzero"`
}

// GuildWidget is the public widget of a guild.
type GuildWidget struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	InstantInvite string     `json:"instant_invite"`
	Channels      []*Channel `json:"channels"` // partial
	Members       []*User    `json:"members"`  // partial and anonymized
	PresenceCount int        `json:"presence_count"`
}

// VanityURL is the custom invite of a guild.
type VanityURL struct {
	Code string `json:"code"`
	Uses int    `json:"uses"`
}

// WelcomeScreen is shown to new members of a community guild.
type WelcomeScreen struct {
	Description     string                  `json:"description"`
	WelcomeChannels []*WelcomeScreenChannel `json:"welcome_channels"`
}

// WelcomeScreenChannel is a channel suggested by a WelcomeScreen.
type WelcomeScreenChannel struct {
	ChannelID   string `json:"channel_id"`
	Description string `json:"description"`
	EmojiID     string `json:"emoji_id,omitempty"`
	EmojiName   string `json:"emoji_name,omitempty"`
}

// WelcomeScreenParams is the body of a Modify Guild Welcome Screen request.
type WelcomeScreenParams struct {
	Enabled         *bool                   `json:"enabled,omitempty"`
	WelcomeChannels *[]WelcomeScreenChannel `json:"welcome_channels,omitempty"`
	Description     *string                 `json:"description,omitempty"`
}

// GuildOnboarding is the onboarding flow of a guild. It doubles as the body
// of a Modify Guild Onboarding request.
type GuildOnboarding struct {
	GuildID           string              `json:"guild_id,omitempty"`
	Prompts           []*OnboardingPrompt `json:"prompts"`
	DefaultChannelIDs []string            `json:"default_channel_ids"`
	Enabled           bool                `json:"enabled"`
	Mode              int                 `json:"mode"` // 0 default, 1 advanced
}

// OnboardingPrompt is a question of a guild's onboarding.
type OnboardingPrompt struct {
	ID           string                    `json:"id,omitempty"`
	Type         int                       `json:"type"` // 0 multiple choice, 1 dropdown
	Options      []*OnboardingPromptOption `json:"options"`
	Title        string                    `json:"title"`
	SingleSelect bool                      `json:"single_select"`
	Required     bool                      `json:"required"`
	InOnboarding bool                      `json:"in_onboarding"`
}

// OnboardingPromptOption is an answer to an OnboardingPrompt. Discord sends
// its emoji as Emoji and takes it as EmojiID, EmojiName and EmojiAnimated.
type OnboardingPromptOption struct {
	ID            string   `json:"id,omitempty"`
	ChannelIDs    []string `json:"channel_ids"`
	RoleIDs       []string `json:"role_ids"`
	Emoji         *Emoji   `json:"emoji,omitempty"`
	EmojiID       string   `json:"emoji_id,omitempty"`
	EmojiName     string   `json:"emoji_name,omitempty"`
	EmojiAnimated bool     `json:"emoji_animated,omitempty"`
	Title         string   `json:"title"`
	Description   string   `json:"description,omitempty"`
}

// GuildTemplate is a snapshot of a guild that new guilds can be created from.
type GuildTemplate struct {
	Code          string          `json:"code"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	UsageCount    int             `json:"usage_count"`
	CreatorID     string          `json:"creator_id"`
	Creator       *User           `json:"creator"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	SourceGuildID string          `json:"source_guild_id"`
	Guild         json.RawMessage `json:"serialized_source_guild"` // a partial guild, with placeholder IDs
	IsDirty       bool            `json:"is_dirty"`                // has unsynced changes
}

// GuildTemplateParams is the body of a request that creates or changes a
// guild template.
type GuildTemplateParams struct {
	Name        string  `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// AuditLogEvent is the kind of action an audit log entry records.
type AuditLogEvent int

const (
	AuditLogGuildUpdate                             AuditLogEvent = 1
	AuditLogChannelCreate                           AuditLogEvent = 10
	AuditLogChannelUpdate                           AuditLogEvent = 11
	AuditLogChannelDelete                           AuditLogEvent = 12
	AuditLogChannelOverwriteCreate                  AuditLogEvent = 13
	AuditLogChannelOverwriteUpdate                  AuditLogEvent = 14
	AuditLogChannelOverwriteDelete                  AuditLogEvent = 15
	AuditLogMemberKick                              AuditLogEvent = 20
	AuditLogMemberPrune                             AuditLogEvent = 21
	AuditLogMemberBanAdd                            AuditLogEvent = 22
	AuditLogMemberBanRemove                         AuditLogEvent = 23
	AuditLogMemberUpdate                            AuditLogEvent = 24
	AuditLogMemberRoleUpdate                        AuditLogEvent = 25
	AuditLogMemberMove                              AuditLogEvent = 26
	AuditLogMemberDisconnect                        AuditLogEvent = 27
	AuditLogBotAdd                                  AuditLogEvent = 28
	AuditLogRoleCreate                              AuditLogEvent = 30
	AuditLogRoleUpdate                              AuditLogEvent = 31
	AuditLogRoleDelete                              AuditLogEvent = 32
	AuditLogInviteCreate                            AuditLogEvent = 40
	AuditLogInviteUpdate                            AuditLogEvent = 41
	AuditLogInviteDelete                            AuditLogEvent = 42
	AuditLogWebhookCreate                           AuditLogEvent = 50
	AuditLogWebhookUpdate                           AuditLogEvent = 51
	AuditLogWebhookDelete                           AuditLogEvent = 52
	AuditLogEmojiCreate                             AuditLogEvent = 60
	AuditLogEmojiUpdate                             AuditLogEvent = 61
	AuditLogEmojiDelete                             AuditLogEvent = 62
	AuditLogMessageDelete                           AuditLogEvent = 72
	AuditLogMessageBulkDelete                       AuditLogEvent = 73
	AuditLogMessagePin                              AuditLogEvent = 74
	AuditLogMessageUnpin                            AuditLogEvent = 75
	AuditLogIntegrationCreate                       AuditLogEvent = 80
	AuditLogIntegrationUpdate                       AuditLogEvent = 81
	AuditLogIntegrationDelete                       AuditLogEvent = 82
	AuditLogStageInstanceCreate                     AuditLogEvent = 83
	AuditLogStageInstanceUpdate                     AuditLogEvent = 84
	AuditLogStageInstanceDelete                     AuditLogEvent = 85
	AuditLogStickerCreate                           AuditLogEvent = 90
	AuditLogStickerUpdate                           AuditLogEvent = 91
	AuditLogStickerDelete                           AuditLogEvent = 92
	AuditLogGuildScheduledEventCreate               AuditLogEvent = 100
	AuditLogGuildScheduledEventUpdate               AuditLogEvent = 101
	AuditLogGuildScheduledEventDelete               AuditLogEvent = 102
	AuditLogThreadCreate                            AuditLogEvent = 110
	AuditLogThreadUpdate                            AuditLogEvent = 111
	AuditLogThreadDelete                            AuditLogEvent = 112
	AuditLogApplicationCommandPermissionUpdate      AuditLogEvent = 121
	AuditLogSoundboardSoundCreate                   AuditLogEvent = 130
	AuditLogSoundboardSoundUpdate                   AuditLogEvent = 131
	AuditLogSoundboardSoundDelete                   AuditLogEvent = 132
	AuditLogAutoModerationRuleCreate                AuditLogEvent = 140
	AuditLogAutoModerationRuleUpdate                AuditLogEvent = 141
	AuditLogAutoModerationRuleDelete                AuditLogEvent = 142
	AuditLogAutoModerationBlockMessage              AuditLogEvent = 143
	AuditLogAutoModerationFlagToChannel             AuditLogEvent = 144
	AuditLogAutoModerationUserCommunicationDisabled AuditLogEvent = 145
	AuditLogCreatorMonetizationRequestCreated       AuditLogEvent = 150
	AuditLogCreatorMonetizationTermsAccepted        AuditLogEvent = 151
	AuditLogOnboardingPromptCreate                  AuditLogEvent = 163
	AuditLogOnboardingPromptUpdate                  AuditLogEvent = 164
	AuditLogOnboardingPromptDelete                  AuditLogEvent = 165
	AuditLogOnboardingCreate                        AuditLogEvent = 166
	AuditLogOnboardingUpdate                        AuditLogEvent = 167
	AuditLogHomeSettingsCreate                      AuditLogEvent = 190
	AuditLogHomeSettingsUpdate                      AuditLogEvent = 191
)

// AuditLog is a page of a guild's audit log, with the objects its entries
// refer to.
type AuditLog struct {
	Entries              []*AuditLogEntry       `json:"audit_log_entries"`
	ApplicationCommands  []*ApplicationCommand  `json:"application_commands"`
	AutoModerationRules  []*AutoModerationRule  `json:"auto_moderation_rules"`
	GuildScheduledEvents []*GuildScheduledEvent `json:"guild_scheduled_events"`
	Integrations         []*Integration         `json:"integrations"` // partial
	Threads              []*Channel             `json:"threads"`
	Users                []*User                `json:"users"`
	Webhooks             []*Webhook             `json:"webhooks"`
}

// AuditLogEntry records one action taken in a guild.
type AuditLogEntry struct {
	ID         string            `json:"id"`
	TargetID   string            `json:"target_id"` // of the affected object
	UserID     string            `json:"user_id"`   // who did it
	ActionType AuditLogEvent     `json:"action_type"`
	Changes    []*AuditLogChange `json:"changes"`
	Options    *AuditLogOptions  `json:"options"`
	Reason     string            `json:"reason"`
}

// AuditLogChange is one changed property of the target of an entry. The
// values are left encoded, as their type depends on Key.
type AuditLogChange struct {
	Key      string          `json:"key"`
	NewValue json.RawMessage `json:"new_value"`
	OldValue json.RawMessage `json:"old_value"`
}

// AuditLogOptions is the extra information of some kinds of entries.
type AuditLogOptions struct {
	ApplicationID                 string `json:"application_id"`
	AutoModerationRuleName        string `json:"auto_moderation_rule_name"`
	AutoModerationRuleTriggerType string `json:"auto_moderation_rule_trigger_type"`
	ChannelID                     string `json:"channel_id"`
	Count                         string `json:"count"`
	DeleteMemberDays              string `json:"delete_member_days"`
	ID                            string `json:"id"`
	MembersRemoved                string `json:"members_removed"`
	MessageID                     string `json:"message_id"`
	RoleName                      string `json:"role_name"`
	Type                          string `json:"type"`
	IntegrationType               string `json:"integration_type"`
	Status                        string `json:"status"` // a voice channel status that was set
}

// AuditLogQuery selects the entries of an audit log. Empty fields do not filter.
type AuditLogQuery struct {
	UserID     string // who took the action
	ActionType AuditLogEvent
	Before     string // an entry ID; entries are returned newest first
	After      string // an entry ID; entries are returned oldest first
	Limit      int    // up to 100; Discord defaults to 50
}

// AutoModerationEventType is when an auto moderation rule is checked.
type AutoModerationEventType int

const (
	AutoModerationEventMessageSend  AutoModerationEventType = 1
	AutoModerationEventMemberUpdate AutoModerationEventType = 2
)

// AutoModerationTriggerType is what an auto moderation rule looks for.
type AutoModerationTriggerType int

const (
	AutoModerationTriggerKeyword       AutoModerationTriggerType = 1
	AutoModerationTriggerSpam          AutoModerationTriggerType = 3
	AutoModerationTriggerKeywordPreset AutoModerationTriggerType = 4
	AutoModerationTriggerMentionSpam   AutoModerationTriggerType = 5
	AutoModerationTriggerMemberProfile AutoModerationTriggerType = 6
)

// AutoModerationActionType is what an auto moderation rule does.
type AutoModerationActionType int

const (
	AutoModerationActionBlockMessage           AutoModerationActionType = 1
	AutoModerationActionSendAlertMessage       AutoModerationActionType = 2
	AutoModerationActionTimeout                AutoModerationActionType = 3
	AutoModerationActionBlockMemberInteraction AutoModerationActionType = 4
)

// AutoModerationRule is a rule of a guild's auto moderation.
type AutoModerationRule struct {
	ID              string                         `json:"id"`
	GuildID         string                         `json:"guild_id"`
	Name            string                         `json:"name"`
	CreatorID       string                         `json:"creator_id"`
	EventType       AutoModerationEventType        `json:"event_type"`
	TriggerType     AutoModerationTriggerType      `json:"trigger_type"`
	TriggerMetadata *AutoModerationTriggerMetadata `json:"trigger_metadata"`
	Actions         []AutoModerationAction         `json:"actions"`
	Enabled         bool                           `json:"enabled"`
	ExemptRoles     []string                       `json:"exempt_roles"`
	ExemptChannels  []string                       `json:"exempt_channels"`
}

// AutoModerationTriggerMetadata configures a rule's trigger. Which fields
// apply depends on the trigger type.
type AutoModerationTriggerMetadata struct {
	KeywordFilter                []string `json:"keyword_filter,omitempty"`
	RegexPatterns                []string `json:"regex_patterns,omitempty"`
	Presets                      []int    `json:"presets,omitempty"` // 1 profanity, 2 sexual content, 3 slurs
	AllowList                    []string `json:"allow_list,omitempty"`
	MentionTotalLimit            int      `json:"mention_total_limit,omitempty"`
	MentionRaidProtectionEnabled bool     `json:"mention_raid_protection_enabled,omitempty"`
}

// AutoModerationAction is something a rule does when it is triggered.
type AutoModerationAction struct {
	Type     AutoModerationActionType      `json:"type"`
	Metadata *AutoModerationActionMetadata `json:"metadata,omitempty"`
}

// AutoModerationActionMetadata configures an AutoModerationAction.
type AutoModerationActionMetadata struct {
	ChannelID       string `json:"channel_id,omitempty"`       // where alerts are sent
	DurationSeconds int    `json:"duration_seconds,omitempty"` // of a timeout, up to 4 weeks
	CustomMessage   string `json:"custom_message,omitempty"`   // shown when a message is blocked
}

// AutoModerationRuleParams is the body of a request that creates or changes
// an auto moderation rule. TriggerType cannot be changed afterwards.
type AutoModerationRuleParams struct {
	Name            string                         `json:"name,omitempty"`
	EventType       AutoModerationEventType        `json:"event_type,omitempty"`
	TriggerType     AutoModerationTriggerType      `json:"trigger_type,omitempty"`
	TriggerMetadata *AutoModerationTriggerMetadata `json:"trigger_metadata,omitempty"`
	Actions         []AutoModerationAction         `json:"actions,omitempty"`
	Enabled         *bool                          `json:"enabled,omitempty"`
	ExemptRoles     *[]string                      `json:"exempt_roles,omitempty"`
	ExemptChannels  *[]string                      `json:"exempt_channels,omitempty"`
}

// GuildScheduledEventStatus is the state of a scheduled event.
type GuildScheduledEventStatus int

const (
	GuildScheduledEventScheduled GuildScheduledEventStatus = 1
	GuildScheduledEventActive    GuildScheduledEventStatus = 2
	GuildScheduledEventCompleted GuildScheduledEventStatus = 3
	GuildScheduledEventCanceled  GuildScheduledEventStatus = 4
)

// GuildScheduledEventEntityType is where a scheduled event takes place.
type GuildScheduledEventEntityType int

const (
	GuildScheduledEventStageInstance GuildScheduledEventEntityType = 1
	GuildScheduledEventVoice         GuildScheduledEventEntityType = 2
	GuildScheduledEventExternal      GuildScheduledEventEntityType = 3
)

// GuildScheduledEvent is an event planned in a guild.
type GuildScheduledEvent struct {
	ID                 string                        `json:"id"`
	GuildID            string                        `json:"guild_id"`
	ChannelID          string                        `json:"channel_id"` // empty for an external event
	CreatorID          string                        `json:"creator_id"`
	Name               string                        `json:"name"`
	Description        string                        `json:"description"`
	ScheduledStartTime time.Time                     `json:"scheduled_start_time"`
	ScheduledEndTime   *time.Time                    `json:"scheduled_end_time"`
	PrivacyLevel       StagePrivacyLevel             `json:"privacy_level"`
	Status             GuildScheduledEventStatus     `json:"status"`
	EntityType         GuildScheduledEventEntityType `json:"entity_type"`
	EntityID           string                        `json:"entity_id"`
	EntityMetadata     *GuildScheduledEventMetadata  `json:"entity_metadata"`
	Creator            *User                         `json:"creator"`
	UserCount          int                           `json:"user_count"` // only when asked for
	Image              string                        `json:"image"`
	RecurrenceRule     json.RawMessage               `json:"recurrence_rule"`
}

// GuildScheduledEventMetadata holds the location of an external event.
type GuildScheduledEventMetadata struct {
	Location string `json:"location,omitempty"`
}

// GuildScheduledEventParams is the body of a request that creates or changes
// a scheduled event. Creating one needs Name, ScheduledStartTime, EntityType
// and PrivacyLevel; an external event also needs ScheduledEndTime and a
// location, and takes no channel. Set Status to start, end or cancel it.
type GuildScheduledEventParams struct {
	ChannelID          Optional[string]              `json:"channel_id,omitzero"`
	EntityMetadata     *GuildScheduledEventMetadata  `json:"entity_metadata,omitempty"`
	Name               string                        `json:"name,omitempty"`
	PrivacyLevel       StagePrivacyLevel             `json:"privacy_level,omitempty"`
	ScheduledStartTime *time.Time                    `json:"scheduled_start_time,omitempty"`
	ScheduledEndTime   *time.Time                    `json:"scheduled_end_time,omitempty"`
	Description        *string                       `json:"description,omitempty"`
	EntityType         GuildScheduledEventEntityType `json:"entity_type,omitempty"`
	Status             GuildScheduledEventStatus     `json:"status,omitempty"`
	Image              string                        `json:"image,omitempty"` // a data URI; see ImageData
	RecurrenceRule     json.RawMessage               `json:"recurrence_rule,omitempty"`
}

// GuildScheduledEventUser is a user who is interested in a scheduled event.
// Member is only set when asked for.
type GuildScheduledEventUser struct {
	GuildScheduledEventID string  `json:"guild_scheduled_event_id"`
	User                  *User   `json:"user"`
	Member                *Member `json:"member"`
}

// SoundboardSound is a sound of a guild's soundboard, or one of the defaults
// that every guild has. EmojiID and EmojiName are its emoji, at most one set.
type SoundboardSound struct {
	Name      string  `json:"name"`
	SoundID   string  `json:"sound_id"`
	Volume    float64 `json:"volume"` // 0 to 1
	EmojiID   string  `json:"emoji_id"`
	EmojiName string  `json:"emoji_name"`
	GuildID   string  `json:"guild_id"`
	Available bool    `json:"available"`
	User      *User   `json:"user"` // who created it
}

// SoundboardSoundParams is the body of a request that creates or changes a
// soundboard sound. Sound is an MP3 or Ogg file as a data URI (see ImageData),
// needed to create one and ignored otherwise.
type SoundboardSoundParams struct {
	Name      string            `json:"name,omitempty"`
	Sound     string            `json:"sound,omitempty"`
	Volume    Optional[float64] `json:"volume,omitzero"`
	EmojiID   Optional[string]  `json:"emoji_id,omitzero"`
	EmojiName Optional[string]  `json:"emoji_name,omitzero"`
}

// Presence is what a user is shown to be doing in a guild. User holds only
// the ID unless more of it changed.
type Presence struct {
	User         *User        `json:"user"`
	GuildID      string       `json:"guild_id"`
	Status       Status       `json:"status"`
	Activities   []*Activity  `json:"activities"`
	ClientStatus ClientStatus `json:"client_status"`
}

// ClientStatus is a user's status on each kind of device, empty where the
// user is offline.
type ClientStatus struct {
	Desktop Status `json:"desktop"`
	Mobile  Status `json:"mobile"`
	Web     Status `json:"web"`
}

// StatusOffline is the status of a user who is not connected. A bot sets
// StatusInvisible to appear so.
const StatusOffline Status = "offline"
