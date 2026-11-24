package client

import (
	"encoding/json"

	"github.com/tbdsux/minidis/discord"
)

// eventHandlers are the typed handlers of one gateway event.
type eventHandlers struct {
	decode   func(json.RawMessage) (any, error)
	handlers []func(*Session, any)
}

// on registers a handler for the event with the given name, which is decoded
// into a T.
func on[T any](s *Session, event string, h func(*Session, *T)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()

	e := s.events[event]
	if e == nil {
		e = &eventHandlers{decode: func(data json.RawMessage) (any, error) {
			v := new(T)
			// RESUMED and the like carry no data worth decoding.
			if len(data) == 0 || string(data) == "null" {
				return v, nil
			}
			return v, json.Unmarshal(data, v)
		}}
		s.events[event] = e
	}
	e.handlers = append(e.handlers, func(s *Session, v any) { h(s, v.(*T)) })
}

// OnRawEvent registers a handler that receives every dispatch event as its
// name, such as "GUILD_CREATE", and undecoded JSON payload. It is the way to
// handle an event that has no typed handler.
func (s *Session) OnRawEvent(h func(s *Session, event string, data json.RawMessage)) {
	s.handlersMu.Lock()
	defer s.handlersMu.Unlock()
	s.onRawEvents = append(s.onRawEvents, h)
}

// OnReady registers a handler for the READY event, sent when a shard starts a
// gateway session: on Open, once for each shard, and after a reconnect that
// could not resume.
func (s *Session) OnReady(h func(*Session, *discord.Ready)) {
	on(s, "READY", h)
}

// OnResumed registers a handler for the RESUMED event.
func (s *Session) OnResumed(h func(*Session, *discord.Resumed)) {
	on(s, "RESUMED", h)
}

// OnMessageCreate registers a handler for the MESSAGE_CREATE event.
func (s *Session) OnMessageCreate(h func(*Session, *discord.MessageCreate)) {
	on(s, "MESSAGE_CREATE", h)
}

// OnMessageUpdate registers a handler for the MESSAGE_UPDATE event.
func (s *Session) OnMessageUpdate(h func(*Session, *discord.MessageUpdate)) {
	on(s, "MESSAGE_UPDATE", h)
}

// OnMessageDelete registers a handler for the MESSAGE_DELETE event.
func (s *Session) OnMessageDelete(h func(*Session, *discord.MessageDelete)) {
	on(s, "MESSAGE_DELETE", h)
}

// OnMessageDeleteBulk registers a handler for the MESSAGE_DELETE_BULK event.
func (s *Session) OnMessageDeleteBulk(h func(*Session, *discord.MessageDeleteBulk)) {
	on(s, "MESSAGE_DELETE_BULK", h)
}

// OnMessageReactionAdd registers a handler for the MESSAGE_REACTION_ADD event.
func (s *Session) OnMessageReactionAdd(h func(*Session, *discord.MessageReactionAdd)) {
	on(s, "MESSAGE_REACTION_ADD", h)
}

// OnMessageReactionRemove registers a handler for the MESSAGE_REACTION_REMOVE
// event.
func (s *Session) OnMessageReactionRemove(h func(*Session, *discord.MessageReactionRemove)) {
	on(s, "MESSAGE_REACTION_REMOVE", h)
}

// OnMessageReactionRemoveAll registers a handler for the
// MESSAGE_REACTION_REMOVE_ALL event.
func (s *Session) OnMessageReactionRemoveAll(h func(*Session, *discord.MessageReactionRemoveAll)) {
	on(s, "MESSAGE_REACTION_REMOVE_ALL", h)
}

// OnMessageReactionRemoveEmoji registers a handler for the
// MESSAGE_REACTION_REMOVE_EMOJI event.
func (s *Session) OnMessageReactionRemoveEmoji(h func(*Session, *discord.MessageReactionRemoveEmoji)) {
	on(s, "MESSAGE_REACTION_REMOVE_EMOJI", h)
}

// OnMessagePollVoteAdd registers a handler for the MESSAGE_POLL_VOTE_ADD
// event.
func (s *Session) OnMessagePollVoteAdd(h func(*Session, *discord.MessagePollVoteAdd)) {
	on(s, "MESSAGE_POLL_VOTE_ADD", h)
}

// OnMessagePollVoteRemove registers a handler for the MESSAGE_POLL_VOTE_REMOVE
// event.
func (s *Session) OnMessagePollVoteRemove(h func(*Session, *discord.MessagePollVoteRemove)) {
	on(s, "MESSAGE_POLL_VOTE_REMOVE", h)
}

// OnTypingStart registers a handler for the TYPING_START event.
func (s *Session) OnTypingStart(h func(*Session, *discord.TypingStart)) {
	on(s, "TYPING_START", h)
}

// OnInteractionCreate registers a handler for the INTERACTION_CREATE event.
func (s *Session) OnInteractionCreate(h func(*Session, *discord.InteractionCreate)) {
	on(s, "INTERACTION_CREATE", h)
}

// OnGuildCreate registers a handler for the GUILD_CREATE event, sent for each
// guild as the session starts, and when the bot joins one.
func (s *Session) OnGuildCreate(h func(*Session, *discord.GuildCreate)) {
	on(s, "GUILD_CREATE", h)
}

// OnGuildUpdate registers a handler for the GUILD_UPDATE event.
func (s *Session) OnGuildUpdate(h func(*Session, *discord.GuildUpdate)) {
	on(s, "GUILD_UPDATE", h)
}

// OnGuildDelete registers a handler for the GUILD_DELETE event.
func (s *Session) OnGuildDelete(h func(*Session, *discord.GuildDelete)) {
	on(s, "GUILD_DELETE", h)
}

// OnGuildAuditLogEntryCreate registers a handler for the
// GUILD_AUDIT_LOG_ENTRY_CREATE event.
func (s *Session) OnGuildAuditLogEntryCreate(h func(*Session, *discord.GuildAuditLogEntryCreate)) {
	on(s, "GUILD_AUDIT_LOG_ENTRY_CREATE", h)
}

// OnGuildBanAdd registers a handler for the GUILD_BAN_ADD event.
func (s *Session) OnGuildBanAdd(h func(*Session, *discord.GuildBanAdd)) {
	on(s, "GUILD_BAN_ADD", h)
}

// OnGuildBanRemove registers a handler for the GUILD_BAN_REMOVE event.
func (s *Session) OnGuildBanRemove(h func(*Session, *discord.GuildBanRemove)) {
	on(s, "GUILD_BAN_REMOVE", h)
}

// OnGuildEmojisUpdate registers a handler for the GUILD_EMOJIS_UPDATE event.
func (s *Session) OnGuildEmojisUpdate(h func(*Session, *discord.GuildEmojisUpdate)) {
	on(s, "GUILD_EMOJIS_UPDATE", h)
}

// OnGuildStickersUpdate registers a handler for the GUILD_STICKERS_UPDATE
// event.
func (s *Session) OnGuildStickersUpdate(h func(*Session, *discord.GuildStickersUpdate)) {
	on(s, "GUILD_STICKERS_UPDATE", h)
}

// OnGuildIntegrationsUpdate registers a handler for the
// GUILD_INTEGRATIONS_UPDATE event.
func (s *Session) OnGuildIntegrationsUpdate(h func(*Session, *discord.GuildIntegrationsUpdate)) {
	on(s, "GUILD_INTEGRATIONS_UPDATE", h)
}

// OnGuildMemberAdd registers a handler for the GUILD_MEMBER_ADD event, which
// needs the privileged IntentsGuildMembers intent.
func (s *Session) OnGuildMemberAdd(h func(*Session, *discord.GuildMemberAdd)) {
	on(s, "GUILD_MEMBER_ADD", h)
}

// OnGuildMemberRemove registers a handler for the GUILD_MEMBER_REMOVE event,
// which needs the privileged IntentsGuildMembers intent.
func (s *Session) OnGuildMemberRemove(h func(*Session, *discord.GuildMemberRemove)) {
	on(s, "GUILD_MEMBER_REMOVE", h)
}

// OnGuildMemberUpdate registers a handler for the GUILD_MEMBER_UPDATE event,
// which needs the privileged IntentsGuildMembers intent.
func (s *Session) OnGuildMemberUpdate(h func(*Session, *discord.GuildMemberUpdate)) {
	on(s, "GUILD_MEMBER_UPDATE", h)
}

// OnGuildMembersChunk registers a handler for the GUILD_MEMBERS_CHUNK event,
// the answer to RequestGuildMembers.
func (s *Session) OnGuildMembersChunk(h func(*Session, *discord.GuildMembersChunk)) {
	on(s, "GUILD_MEMBERS_CHUNK", h)
}

// OnGuildRoleCreate registers a handler for the GUILD_ROLE_CREATE event.
func (s *Session) OnGuildRoleCreate(h func(*Session, *discord.GuildRoleCreate)) {
	on(s, "GUILD_ROLE_CREATE", h)
}

// OnGuildRoleUpdate registers a handler for the GUILD_ROLE_UPDATE event.
func (s *Session) OnGuildRoleUpdate(h func(*Session, *discord.GuildRoleUpdate)) {
	on(s, "GUILD_ROLE_UPDATE", h)
}

// OnGuildRoleDelete registers a handler for the GUILD_ROLE_DELETE event.
func (s *Session) OnGuildRoleDelete(h func(*Session, *discord.GuildRoleDelete)) {
	on(s, "GUILD_ROLE_DELETE", h)
}

// OnChannelCreate registers a handler for the CHANNEL_CREATE event.
func (s *Session) OnChannelCreate(h func(*Session, *discord.ChannelCreate)) {
	on(s, "CHANNEL_CREATE", h)
}

// OnChannelUpdate registers a handler for the CHANNEL_UPDATE event.
func (s *Session) OnChannelUpdate(h func(*Session, *discord.ChannelUpdate)) {
	on(s, "CHANNEL_UPDATE", h)
}

// OnChannelDelete registers a handler for the CHANNEL_DELETE event.
func (s *Session) OnChannelDelete(h func(*Session, *discord.ChannelDelete)) {
	on(s, "CHANNEL_DELETE", h)
}

// OnChannelPinsUpdate registers a handler for the CHANNEL_PINS_UPDATE event.
func (s *Session) OnChannelPinsUpdate(h func(*Session, *discord.ChannelPinsUpdate)) {
	on(s, "CHANNEL_PINS_UPDATE", h)
}

// OnThreadCreate registers a handler for the THREAD_CREATE event.
func (s *Session) OnThreadCreate(h func(*Session, *discord.ThreadCreate)) {
	on(s, "THREAD_CREATE", h)
}

// OnThreadUpdate registers a handler for the THREAD_UPDATE event.
func (s *Session) OnThreadUpdate(h func(*Session, *discord.ThreadUpdate)) {
	on(s, "THREAD_UPDATE", h)
}

// OnThreadDelete registers a handler for the THREAD_DELETE event.
func (s *Session) OnThreadDelete(h func(*Session, *discord.ThreadDelete)) {
	on(s, "THREAD_DELETE", h)
}

// OnThreadListSync registers a handler for the THREAD_LIST_SYNC event.
func (s *Session) OnThreadListSync(h func(*Session, *discord.ThreadListSync)) {
	on(s, "THREAD_LIST_SYNC", h)
}

// OnThreadMemberUpdate registers a handler for the THREAD_MEMBER_UPDATE event.
func (s *Session) OnThreadMemberUpdate(h func(*Session, *discord.ThreadMemberUpdate)) {
	on(s, "THREAD_MEMBER_UPDATE", h)
}

// OnThreadMembersUpdate registers a handler for the THREAD_MEMBERS_UPDATE
// event.
func (s *Session) OnThreadMembersUpdate(h func(*Session, *discord.ThreadMembersUpdate)) {
	on(s, "THREAD_MEMBERS_UPDATE", h)
}

// OnPresenceUpdate registers a handler for the PRESENCE_UPDATE event, which
// needs the privileged IntentsGuildPresences intent.
func (s *Session) OnPresenceUpdate(h func(*Session, *discord.PresenceUpdateEvent)) {
	on(s, "PRESENCE_UPDATE", h)
}

// OnUserUpdate registers a handler for the USER_UPDATE event.
func (s *Session) OnUserUpdate(h func(*Session, *discord.UserUpdate)) {
	on(s, "USER_UPDATE", h)
}

// OnVoiceStateUpdate registers a handler for the VOICE_STATE_UPDATE event.
func (s *Session) OnVoiceStateUpdate(h func(*Session, *discord.VoiceStateUpdate)) {
	on(s, "VOICE_STATE_UPDATE", h)
}

// OnVoiceServerUpdate registers a handler for the VOICE_SERVER_UPDATE event.
func (s *Session) OnVoiceServerUpdate(h func(*Session, *discord.VoiceServerUpdate)) {
	on(s, "VOICE_SERVER_UPDATE", h)
}

// OnVoiceChannelEffectSend registers a handler for the
// VOICE_CHANNEL_EFFECT_SEND event.
func (s *Session) OnVoiceChannelEffectSend(h func(*Session, *discord.VoiceChannelEffectSend)) {
	on(s, "VOICE_CHANNEL_EFFECT_SEND", h)
}

// OnVoiceChannelStatusUpdate registers a handler for the
// VOICE_CHANNEL_STATUS_UPDATE event.
func (s *Session) OnVoiceChannelStatusUpdate(h func(*Session, *discord.VoiceChannelStatusUpdate)) {
	on(s, "VOICE_CHANNEL_STATUS_UPDATE", h)
}

// OnVoiceChannelStartTimeUpdate registers a handler for the
// VOICE_CHANNEL_START_TIME_UPDATE event.
func (s *Session) OnVoiceChannelStartTimeUpdate(h func(*Session, *discord.VoiceChannelStartTimeUpdate)) {
	on(s, "VOICE_CHANNEL_START_TIME_UPDATE", h)
}

// OnChannelInfo registers a handler for the CHANNEL_INFO event, the answer
// to RequestChannelInfo.
func (s *Session) OnChannelInfo(h func(*Session, *discord.ChannelInfo)) {
	on(s, "CHANNEL_INFO", h)
}

// OnRateLimited registers a handler for the RATE_LIMITED event, sent when
// Discord refuses a gateway command such as RequestGuildMembers.
func (s *Session) OnRateLimited(h func(*Session, *discord.RateLimited)) {
	on(s, "RATE_LIMITED", h)
}

// OnWebhooksUpdate registers a handler for the WEBHOOKS_UPDATE event.
func (s *Session) OnWebhooksUpdate(h func(*Session, *discord.WebhooksUpdate)) {
	on(s, "WEBHOOKS_UPDATE", h)
}

// OnInviteCreate registers a handler for the INVITE_CREATE event.
func (s *Session) OnInviteCreate(h func(*Session, *discord.InviteCreate)) {
	on(s, "INVITE_CREATE", h)
}

// OnInviteDelete registers a handler for the INVITE_DELETE event.
func (s *Session) OnInviteDelete(h func(*Session, *discord.InviteDelete)) {
	on(s, "INVITE_DELETE", h)
}

// OnIntegrationCreate registers a handler for the INTEGRATION_CREATE event.
func (s *Session) OnIntegrationCreate(h func(*Session, *discord.IntegrationCreate)) {
	on(s, "INTEGRATION_CREATE", h)
}

// OnIntegrationUpdate registers a handler for the INTEGRATION_UPDATE event.
func (s *Session) OnIntegrationUpdate(h func(*Session, *discord.IntegrationUpdate)) {
	on(s, "INTEGRATION_UPDATE", h)
}

// OnIntegrationDelete registers a handler for the INTEGRATION_DELETE event.
func (s *Session) OnIntegrationDelete(h func(*Session, *discord.IntegrationDelete)) {
	on(s, "INTEGRATION_DELETE", h)
}

// OnApplicationCommandPermissionsUpdate registers a handler for the
// APPLICATION_COMMAND_PERMISSIONS_UPDATE event.
func (s *Session) OnApplicationCommandPermissionsUpdate(h func(*Session, *discord.ApplicationCommandPermissionsUpdate)) {
	on(s, "APPLICATION_COMMAND_PERMISSIONS_UPDATE", h)
}

// OnAutoModerationRuleCreate registers a handler for the
// AUTO_MODERATION_RULE_CREATE event.
func (s *Session) OnAutoModerationRuleCreate(h func(*Session, *discord.AutoModerationRuleCreate)) {
	on(s, "AUTO_MODERATION_RULE_CREATE", h)
}

// OnAutoModerationRuleUpdate registers a handler for the
// AUTO_MODERATION_RULE_UPDATE event.
func (s *Session) OnAutoModerationRuleUpdate(h func(*Session, *discord.AutoModerationRuleUpdate)) {
	on(s, "AUTO_MODERATION_RULE_UPDATE", h)
}

// OnAutoModerationRuleDelete registers a handler for the
// AUTO_MODERATION_RULE_DELETE event.
func (s *Session) OnAutoModerationRuleDelete(h func(*Session, *discord.AutoModerationRuleDelete)) {
	on(s, "AUTO_MODERATION_RULE_DELETE", h)
}

// OnAutoModerationActionExecution registers a handler for the
// AUTO_MODERATION_ACTION_EXECUTION event.
func (s *Session) OnAutoModerationActionExecution(h func(*Session, *discord.AutoModerationActionExecution)) {
	on(s, "AUTO_MODERATION_ACTION_EXECUTION", h)
}

// OnGuildScheduledEventCreate registers a handler for the
// GUILD_SCHEDULED_EVENT_CREATE event.
func (s *Session) OnGuildScheduledEventCreate(h func(*Session, *discord.GuildScheduledEventCreate)) {
	on(s, "GUILD_SCHEDULED_EVENT_CREATE", h)
}

// OnGuildScheduledEventUpdate registers a handler for the
// GUILD_SCHEDULED_EVENT_UPDATE event.
func (s *Session) OnGuildScheduledEventUpdate(h func(*Session, *discord.GuildScheduledEventUpdate)) {
	on(s, "GUILD_SCHEDULED_EVENT_UPDATE", h)
}

// OnGuildScheduledEventDelete registers a handler for the
// GUILD_SCHEDULED_EVENT_DELETE event.
func (s *Session) OnGuildScheduledEventDelete(h func(*Session, *discord.GuildScheduledEventDelete)) {
	on(s, "GUILD_SCHEDULED_EVENT_DELETE", h)
}

// OnGuildScheduledEventUserAdd registers a handler for the
// GUILD_SCHEDULED_EVENT_USER_ADD event.
func (s *Session) OnGuildScheduledEventUserAdd(h func(*Session, *discord.GuildScheduledEventUserAdd)) {
	on(s, "GUILD_SCHEDULED_EVENT_USER_ADD", h)
}

// OnGuildScheduledEventUserRemove registers a handler for the
// GUILD_SCHEDULED_EVENT_USER_REMOVE event.
func (s *Session) OnGuildScheduledEventUserRemove(h func(*Session, *discord.GuildScheduledEventUserRemove)) {
	on(s, "GUILD_SCHEDULED_EVENT_USER_REMOVE", h)
}

// OnStageInstanceCreate registers a handler for the STAGE_INSTANCE_CREATE
// event.
func (s *Session) OnStageInstanceCreate(h func(*Session, *discord.StageInstanceCreate)) {
	on(s, "STAGE_INSTANCE_CREATE", h)
}

// OnStageInstanceUpdate registers a handler for the STAGE_INSTANCE_UPDATE
// event.
func (s *Session) OnStageInstanceUpdate(h func(*Session, *discord.StageInstanceUpdate)) {
	on(s, "STAGE_INSTANCE_UPDATE", h)
}

// OnStageInstanceDelete registers a handler for the STAGE_INSTANCE_DELETE
// event.
func (s *Session) OnStageInstanceDelete(h func(*Session, *discord.StageInstanceDelete)) {
	on(s, "STAGE_INSTANCE_DELETE", h)
}

// OnGuildSoundboardSoundCreate registers a handler for the
// GUILD_SOUNDBOARD_SOUND_CREATE event.
func (s *Session) OnGuildSoundboardSoundCreate(h func(*Session, *discord.GuildSoundboardSoundCreate)) {
	on(s, "GUILD_SOUNDBOARD_SOUND_CREATE", h)
}

// OnGuildSoundboardSoundUpdate registers a handler for the
// GUILD_SOUNDBOARD_SOUND_UPDATE event.
func (s *Session) OnGuildSoundboardSoundUpdate(h func(*Session, *discord.GuildSoundboardSoundUpdate)) {
	on(s, "GUILD_SOUNDBOARD_SOUND_UPDATE", h)
}

// OnGuildSoundboardSoundDelete registers a handler for the
// GUILD_SOUNDBOARD_SOUND_DELETE event.
func (s *Session) OnGuildSoundboardSoundDelete(h func(*Session, *discord.GuildSoundboardSoundDelete)) {
	on(s, "GUILD_SOUNDBOARD_SOUND_DELETE", h)
}

// OnGuildSoundboardSoundsUpdate registers a handler for the
// GUILD_SOUNDBOARD_SOUNDS_UPDATE event.
func (s *Session) OnGuildSoundboardSoundsUpdate(h func(*Session, *discord.GuildSoundboardSoundsUpdate)) {
	on(s, "GUILD_SOUNDBOARD_SOUNDS_UPDATE", h)
}

// OnSoundboardSounds registers a handler for the SOUNDBOARD_SOUNDS event, the
// answer to RequestSoundboardSounds.
func (s *Session) OnSoundboardSounds(h func(*Session, *discord.SoundboardSounds)) {
	on(s, "SOUNDBOARD_SOUNDS", h)
}

// OnEntitlementCreate registers a handler for the ENTITLEMENT_CREATE event.
func (s *Session) OnEntitlementCreate(h func(*Session, *discord.EntitlementCreate)) {
	on(s, "ENTITLEMENT_CREATE", h)
}

// OnEntitlementUpdate registers a handler for the ENTITLEMENT_UPDATE event.
func (s *Session) OnEntitlementUpdate(h func(*Session, *discord.EntitlementUpdate)) {
	on(s, "ENTITLEMENT_UPDATE", h)
}

// OnEntitlementDelete registers a handler for the ENTITLEMENT_DELETE event.
func (s *Session) OnEntitlementDelete(h func(*Session, *discord.EntitlementDelete)) {
	on(s, "ENTITLEMENT_DELETE", h)
}

// OnSubscriptionCreate registers a handler for the SUBSCRIPTION_CREATE event.
func (s *Session) OnSubscriptionCreate(h func(*Session, *discord.SubscriptionCreate)) {
	on(s, "SUBSCRIPTION_CREATE", h)
}

// OnSubscriptionUpdate registers a handler for the SUBSCRIPTION_UPDATE event.
func (s *Session) OnSubscriptionUpdate(h func(*Session, *discord.SubscriptionUpdate)) {
	on(s, "SUBSCRIPTION_UPDATE", h)
}

// OnSubscriptionDelete registers a handler for the SUBSCRIPTION_DELETE event.
func (s *Session) OnSubscriptionDelete(h func(*Session, *discord.SubscriptionDelete)) {
	on(s, "SUBSCRIPTION_DELETE", h)
}
