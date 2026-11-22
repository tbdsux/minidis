package client

import (
	"fmt"
	"reflect"

	"github.com/tbdsux/minidis/discord"
)

// eventNames maps the type of each gateway event to its name.
var eventNames = map[reflect.Type]string{
	reflect.TypeFor[discord.Ready]():                               "READY",
	reflect.TypeFor[discord.Resumed]():                             "RESUMED",
	reflect.TypeFor[discord.MessageCreate]():                       "MESSAGE_CREATE",
	reflect.TypeFor[discord.MessageUpdate]():                       "MESSAGE_UPDATE",
	reflect.TypeFor[discord.MessageDelete]():                       "MESSAGE_DELETE",
	reflect.TypeFor[discord.MessageDeleteBulk]():                   "MESSAGE_DELETE_BULK",
	reflect.TypeFor[discord.MessageReactionAdd]():                  "MESSAGE_REACTION_ADD",
	reflect.TypeFor[discord.MessageReactionRemove]():               "MESSAGE_REACTION_REMOVE",
	reflect.TypeFor[discord.MessageReactionRemoveAll]():            "MESSAGE_REACTION_REMOVE_ALL",
	reflect.TypeFor[discord.MessageReactionRemoveEmoji]():          "MESSAGE_REACTION_REMOVE_EMOJI",
	reflect.TypeFor[discord.MessagePollVoteAdd]():                  "MESSAGE_POLL_VOTE_ADD",
	reflect.TypeFor[discord.MessagePollVoteRemove]():               "MESSAGE_POLL_VOTE_REMOVE",
	reflect.TypeFor[discord.TypingStart]():                         "TYPING_START",
	reflect.TypeFor[discord.InteractionCreate]():                   "INTERACTION_CREATE",
	reflect.TypeFor[discord.GuildCreate]():                         "GUILD_CREATE",
	reflect.TypeFor[discord.GuildUpdate]():                         "GUILD_UPDATE",
	reflect.TypeFor[discord.GuildDelete]():                         "GUILD_DELETE",
	reflect.TypeFor[discord.GuildAuditLogEntryCreate]():            "GUILD_AUDIT_LOG_ENTRY_CREATE",
	reflect.TypeFor[discord.GuildBanAdd]():                         "GUILD_BAN_ADD",
	reflect.TypeFor[discord.GuildBanRemove]():                      "GUILD_BAN_REMOVE",
	reflect.TypeFor[discord.GuildEmojisUpdate]():                   "GUILD_EMOJIS_UPDATE",
	reflect.TypeFor[discord.GuildStickersUpdate]():                 "GUILD_STICKERS_UPDATE",
	reflect.TypeFor[discord.GuildIntegrationsUpdate]():             "GUILD_INTEGRATIONS_UPDATE",
	reflect.TypeFor[discord.GuildMemberAdd]():                      "GUILD_MEMBER_ADD",
	reflect.TypeFor[discord.GuildMemberRemove]():                   "GUILD_MEMBER_REMOVE",
	reflect.TypeFor[discord.GuildMemberUpdate]():                   "GUILD_MEMBER_UPDATE",
	reflect.TypeFor[discord.GuildMembersChunk]():                   "GUILD_MEMBERS_CHUNK",
	reflect.TypeFor[discord.GuildRoleCreate]():                     "GUILD_ROLE_CREATE",
	reflect.TypeFor[discord.GuildRoleUpdate]():                     "GUILD_ROLE_UPDATE",
	reflect.TypeFor[discord.GuildRoleDelete]():                     "GUILD_ROLE_DELETE",
	reflect.TypeFor[discord.ChannelCreate]():                       "CHANNEL_CREATE",
	reflect.TypeFor[discord.ChannelUpdate]():                       "CHANNEL_UPDATE",
	reflect.TypeFor[discord.ChannelDelete]():                       "CHANNEL_DELETE",
	reflect.TypeFor[discord.ChannelPinsUpdate]():                   "CHANNEL_PINS_UPDATE",
	reflect.TypeFor[discord.ThreadCreate]():                        "THREAD_CREATE",
	reflect.TypeFor[discord.ThreadUpdate]():                        "THREAD_UPDATE",
	reflect.TypeFor[discord.ThreadDelete]():                        "THREAD_DELETE",
	reflect.TypeFor[discord.ThreadListSync]():                      "THREAD_LIST_SYNC",
	reflect.TypeFor[discord.ThreadMemberUpdate]():                  "THREAD_MEMBER_UPDATE",
	reflect.TypeFor[discord.ThreadMembersUpdate]():                 "THREAD_MEMBERS_UPDATE",
	reflect.TypeFor[discord.PresenceUpdateEvent]():                 "PRESENCE_UPDATE",
	reflect.TypeFor[discord.UserUpdate]():                          "USER_UPDATE",
	reflect.TypeFor[discord.VoiceStateUpdate]():                    "VOICE_STATE_UPDATE",
	reflect.TypeFor[discord.VoiceServerUpdate]():                   "VOICE_SERVER_UPDATE",
	reflect.TypeFor[discord.VoiceChannelEffectSend]():              "VOICE_CHANNEL_EFFECT_SEND",
	reflect.TypeFor[discord.VoiceChannelStatusUpdate]():            "VOICE_CHANNEL_STATUS_UPDATE",
	reflect.TypeFor[discord.VoiceChannelStartTimeUpdate]():         "VOICE_CHANNEL_START_TIME_UPDATE",
	reflect.TypeFor[discord.ChannelInfo]():                         "CHANNEL_INFO",
	reflect.TypeFor[discord.RateLimited]():                         "RATE_LIMITED",
	reflect.TypeFor[discord.WebhooksUpdate]():                      "WEBHOOKS_UPDATE",
	reflect.TypeFor[discord.InviteCreate]():                        "INVITE_CREATE",
	reflect.TypeFor[discord.InviteDelete]():                        "INVITE_DELETE",
	reflect.TypeFor[discord.IntegrationCreate]():                   "INTEGRATION_CREATE",
	reflect.TypeFor[discord.IntegrationUpdate]():                   "INTEGRATION_UPDATE",
	reflect.TypeFor[discord.IntegrationDelete]():                   "INTEGRATION_DELETE",
	reflect.TypeFor[discord.ApplicationCommandPermissionsUpdate](): "APPLICATION_COMMAND_PERMISSIONS_UPDATE",
	reflect.TypeFor[discord.AutoModerationRuleCreate]():            "AUTO_MODERATION_RULE_CREATE",
	reflect.TypeFor[discord.AutoModerationRuleUpdate]():            "AUTO_MODERATION_RULE_UPDATE",
	reflect.TypeFor[discord.AutoModerationRuleDelete]():            "AUTO_MODERATION_RULE_DELETE",
	reflect.TypeFor[discord.AutoModerationActionExecution]():       "AUTO_MODERATION_ACTION_EXECUTION",
	reflect.TypeFor[discord.GuildScheduledEventCreate]():           "GUILD_SCHEDULED_EVENT_CREATE",
	reflect.TypeFor[discord.GuildScheduledEventUpdate]():           "GUILD_SCHEDULED_EVENT_UPDATE",
	reflect.TypeFor[discord.GuildScheduledEventDelete]():           "GUILD_SCHEDULED_EVENT_DELETE",
	reflect.TypeFor[discord.GuildScheduledEventUserAdd]():          "GUILD_SCHEDULED_EVENT_USER_ADD",
	reflect.TypeFor[discord.GuildScheduledEventUserRemove]():       "GUILD_SCHEDULED_EVENT_USER_REMOVE",
	reflect.TypeFor[discord.StageInstanceCreate]():                 "STAGE_INSTANCE_CREATE",
	reflect.TypeFor[discord.StageInstanceUpdate]():                 "STAGE_INSTANCE_UPDATE",
	reflect.TypeFor[discord.StageInstanceDelete]():                 "STAGE_INSTANCE_DELETE",
	reflect.TypeFor[discord.GuildSoundboardSoundCreate]():          "GUILD_SOUNDBOARD_SOUND_CREATE",
	reflect.TypeFor[discord.GuildSoundboardSoundUpdate]():          "GUILD_SOUNDBOARD_SOUND_UPDATE",
	reflect.TypeFor[discord.GuildSoundboardSoundDelete]():          "GUILD_SOUNDBOARD_SOUND_DELETE",
	reflect.TypeFor[discord.GuildSoundboardSoundsUpdate]():         "GUILD_SOUNDBOARD_SOUNDS_UPDATE",
	reflect.TypeFor[discord.SoundboardSounds]():                    "SOUNDBOARD_SOUNDS",
	reflect.TypeFor[discord.EntitlementCreate]():                   "ENTITLEMENT_CREATE",
	reflect.TypeFor[discord.EntitlementUpdate]():                   "ENTITLEMENT_UPDATE",
	reflect.TypeFor[discord.EntitlementDelete]():                   "ENTITLEMENT_DELETE",
	reflect.TypeFor[discord.SubscriptionCreate]():                  "SUBSCRIPTION_CREATE",
	reflect.TypeFor[discord.SubscriptionUpdate]():                  "SUBSCRIPTION_UPDATE",
	reflect.TypeFor[discord.SubscriptionDelete]():                  "SUBSCRIPTION_DELETE",
}

// EventName returns the name of the gateway event that is decoded into an E,
// such as "GUILD_MEMBER_ADD" for a discord.GuildMemberAdd, or "" if E is not
// the type of an event.
func EventName[E any]() string {
	return eventNames[reflect.TypeFor[E]()]
}

// Handle registers a handler for the gateway event that is decoded into an
// E. It does what the On method of that event does, for code that is generic
// over events:
//
//	client.Handle(s, func(s *client.Session, e *discord.GuildMemberAdd) { ... })
//
// It panics if E is not the type of an event.
func Handle[E any](s *Session, h func(*Session, *E)) {
	name := EventName[E]()
	if name == "" {
		panic(fmt.Sprintf("discord: %s is not a gateway event", reflect.TypeFor[E]()))
	}
	on(s, name, h)
}

// eventIntents maps the gateway events that are only sent with an intent to
// the intents that deliver them. Events with two are sent for guilds with
// one and for DMs with the other.
var eventIntents = map[string]discord.Intent{
	"AUTO_MODERATION_ACTION_EXECUTION":  discord.IntentsAutoModerationExecution,
	"AUTO_MODERATION_RULE_CREATE":       discord.IntentsAutoModerationConfiguration,
	"AUTO_MODERATION_RULE_DELETE":       discord.IntentsAutoModerationConfiguration,
	"AUTO_MODERATION_RULE_UPDATE":       discord.IntentsAutoModerationConfiguration,
	"CHANNEL_CREATE":                    discord.IntentsGuilds,
	"CHANNEL_DELETE":                    discord.IntentsGuilds,
	"CHANNEL_PINS_UPDATE":               discord.IntentsGuilds | discord.IntentsDirectMessages,
	"CHANNEL_UPDATE":                    discord.IntentsGuilds,
	"GUILD_AUDIT_LOG_ENTRY_CREATE":      discord.IntentsGuildModeration,
	"GUILD_BAN_ADD":                     discord.IntentsGuildModeration,
	"GUILD_BAN_REMOVE":                  discord.IntentsGuildModeration,
	"GUILD_CREATE":                      discord.IntentsGuilds,
	"GUILD_DELETE":                      discord.IntentsGuilds,
	"GUILD_EMOJIS_UPDATE":               discord.IntentsGuildExpressions,
	"GUILD_INTEGRATIONS_UPDATE":         discord.IntentsGuildIntegrations,
	"GUILD_MEMBER_ADD":                  discord.IntentsGuildMembers,
	"GUILD_MEMBER_REMOVE":               discord.IntentsGuildMembers,
	"GUILD_MEMBER_UPDATE":               discord.IntentsGuildMembers,
	"GUILD_ROLE_CREATE":                 discord.IntentsGuilds,
	"GUILD_ROLE_DELETE":                 discord.IntentsGuilds,
	"GUILD_ROLE_UPDATE":                 discord.IntentsGuilds,
	"GUILD_SCHEDULED_EVENT_CREATE":      discord.IntentsGuildScheduledEvents,
	"GUILD_SCHEDULED_EVENT_DELETE":      discord.IntentsGuildScheduledEvents,
	"GUILD_SCHEDULED_EVENT_UPDATE":      discord.IntentsGuildScheduledEvents,
	"GUILD_SCHEDULED_EVENT_USER_ADD":    discord.IntentsGuildScheduledEvents,
	"GUILD_SCHEDULED_EVENT_USER_REMOVE": discord.IntentsGuildScheduledEvents,
	"GUILD_SOUNDBOARD_SOUNDS_UPDATE":    discord.IntentsGuildExpressions,
	"GUILD_SOUNDBOARD_SOUND_CREATE":     discord.IntentsGuildExpressions,
	"GUILD_SOUNDBOARD_SOUND_DELETE":     discord.IntentsGuildExpressions,
	"GUILD_SOUNDBOARD_SOUND_UPDATE":     discord.IntentsGuildExpressions,
	"GUILD_STICKERS_UPDATE":             discord.IntentsGuildExpressions,
	"GUILD_UPDATE":                      discord.IntentsGuilds,
	"INTEGRATION_CREATE":                discord.IntentsGuildIntegrations,
	"INTEGRATION_DELETE":                discord.IntentsGuildIntegrations,
	"INTEGRATION_UPDATE":                discord.IntentsGuildIntegrations,
	"INVITE_CREATE":                     discord.IntentsGuildInvites,
	"INVITE_DELETE":                     discord.IntentsGuildInvites,
	"MESSAGE_CREATE":                    discord.IntentsGuildMessages | discord.IntentsDirectMessages,
	"MESSAGE_DELETE":                    discord.IntentsGuildMessages | discord.IntentsDirectMessages,
	"MESSAGE_DELETE_BULK":               discord.IntentsGuildMessages,
	"MESSAGE_POLL_VOTE_ADD":             discord.IntentsGuildMessagePolls | discord.IntentsDirectMessagePolls,
	"MESSAGE_POLL_VOTE_REMOVE":          discord.IntentsGuildMessagePolls | discord.IntentsDirectMessagePolls,
	"MESSAGE_REACTION_ADD":              discord.IntentsGuildMessageReactions | discord.IntentsDirectMessageReactions,
	"MESSAGE_REACTION_REMOVE":           discord.IntentsGuildMessageReactions | discord.IntentsDirectMessageReactions,
	"MESSAGE_REACTION_REMOVE_ALL":       discord.IntentsGuildMessageReactions | discord.IntentsDirectMessageReactions,
	"MESSAGE_REACTION_REMOVE_EMOJI":     discord.IntentsGuildMessageReactions | discord.IntentsDirectMessageReactions,
	"MESSAGE_UPDATE":                    discord.IntentsGuildMessages | discord.IntentsDirectMessages,
	"PRESENCE_UPDATE":                   discord.IntentsGuildPresences,
	"STAGE_INSTANCE_CREATE":             discord.IntentsGuilds,
	"STAGE_INSTANCE_DELETE":             discord.IntentsGuilds,
	"STAGE_INSTANCE_UPDATE":             discord.IntentsGuilds,
	"THREAD_CREATE":                     discord.IntentsGuilds,
	"THREAD_DELETE":                     discord.IntentsGuilds,
	"THREAD_LIST_SYNC":                  discord.IntentsGuilds,
	"THREAD_MEMBERS_UPDATE":             discord.IntentsGuilds,
	"THREAD_MEMBER_UPDATE":              discord.IntentsGuilds,
	"THREAD_UPDATE":                     discord.IntentsGuilds,
	"TYPING_START":                      discord.IntentsGuildMessageTyping | discord.IntentsDirectMessageTyping,
	"VOICE_CHANNEL_EFFECT_SEND":         discord.IntentsGuildVoiceStates,
	"VOICE_CHANNEL_START_TIME_UPDATE":   discord.IntentsGuilds,
	"VOICE_CHANNEL_STATUS_UPDATE":       discord.IntentsGuilds,
	"VOICE_STATE_UPDATE":                discord.IntentsGuildVoiceStates,
	"WEBHOOKS_UPDATE":                   discord.IntentsGuildWebhooks,
}

// EventIntents returns the intents a session needs to receive the gateway
// event with the given name: those for guilds and those for DMs, where the
// event has both. It is 0 for an event that is always sent, such as READY
// and INTERACTION_CREATE.
//
// A message event also needs the privileged discord.IntentsMessageContent
// for the content of messages that neither mention the bot nor are sent to
// it in a DM, which is not included.
func EventIntents(event string) discord.Intent {
	return eventIntents[event]
}
