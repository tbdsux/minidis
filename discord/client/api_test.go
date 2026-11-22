package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

func TestAPIEndpoints(t *testing.T) {
	ctx := context.Background()
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		want string
		call func(s *Session) error
	}{
		// Channels and messages.
		{"PATCH /channels/C", func(s *Session) error { _, err := s.ChannelEdit(ctx, "C", &discord.ChannelParams{}); return err }},
		{"DELETE /channels/C", func(s *Session) error { _, err := s.ChannelDelete(ctx, "C"); return err }},
		{"POST /channels/C/typing", func(s *Session) error { return s.ChannelTyping(ctx, "C") }},
		{"GET /channels/C/messages?around=M&limit=3", func(s *Session) error {
			_, err := s.ChannelMessagesQuery(ctx, "C", discord.MessagesQuery{Limit: 3, Around: "M"})
			return err
		}},
		{"POST /channels/C/messages/M/crosspost", func(s *Session) error { _, err := s.ChannelMessageCrosspost(ctx, "C", "M"); return err }},
		{"POST /channels/C/followers", func(s *Session) error { _, err := s.ChannelNewsFollow(ctx, "C", "T"); return err }},
		{"PUT /channels/C/permissions/R", func(s *Session) error {
			return s.ChannelPermissionSet(ctx, "C", &discord.PermissionOverwrite{ID: "R"})
		}},
		{"DELETE /channels/C/permissions/R", func(s *Session) error { return s.ChannelPermissionDelete(ctx, "C", "R") }},
		{"GET /channels/C/messages/M/reactions/x?after=A&limit=5", func(s *Session) error {
			_, err := s.MessageReactions(ctx, "C", "M", "x", 5, "A")
			return err
		}},
		{"DELETE /channels/C/messages/M/reactions/x", func(s *Session) error { return s.MessageReactionsRemoveEmoji(ctx, "C", "M", "x") }},
		{"PUT /channels/C/voice-status", func(s *Session) error { return s.ChannelVoiceStatusSet(ctx, "C", "live") }},
		{"GET /channels/C/permissions/member/U", func(s *Session) error { _, err := s.ChannelMemberPermissions(ctx, "C", "U"); return err }},
		{"GET /channels/C/permissions/role/R", func(s *Session) error { _, err := s.ChannelRolePermissions(ctx, "C", "R"); return err }},
		{"GET /guilds/G/permissions/member/U", func(s *Session) error { _, err := s.GuildMemberPermissions(ctx, "G", "U"); return err }},
		{"GET /guilds/G/permissions/role/R", func(s *Session) error { _, err := s.GuildRolePermissions(ctx, "G", "R"); return err }},
		{"GET /channels/C/polls/M/answers/2?limit=5", func(s *Session) error {
			_, err := s.PollAnswerVoters(ctx, "C", "M", 2, 5, "")
			return err
		}},
		{"POST /channels/C/polls/M/expire", func(s *Session) error { _, err := s.PollExpire(ctx, "C", "M"); return err }},

		// Invites.
		{"GET /channels/C/invites", func(s *Session) error { _, err := s.ChannelInvites(ctx, "C"); return err }},
		{"POST /channels/C/invites", func(s *Session) error { _, err := s.ChannelInviteCreate(ctx, "C", nil); return err }},
		{"GET /invites/code?with_counts=true", func(s *Session) error { _, err := s.Invite(ctx, "code"); return err }},
		{"DELETE /invites/code", func(s *Session) error { _, err := s.InviteDelete(ctx, "code"); return err }},
		{"GET /guilds/G/invites", func(s *Session) error { _, err := s.GuildInvites(ctx, "G"); return err }},

		// Threads.
		{"POST /channels/C/messages/M/threads", func(s *Session) error {
			_, err := s.MessageThreadStart(ctx, "C", "M", &discord.ThreadParams{Name: "t"})
			return err
		}},
		{"POST /channels/C/threads", func(s *Session) error {
			_, err := s.ThreadStart(ctx, "C", &discord.ThreadParams{Name: "t"})
			return err
		}},
		{"PUT /channels/T/thread-members/@me", func(s *Session) error { return s.ThreadJoin(ctx, "T") }},
		{"DELETE /channels/T/thread-members/@me", func(s *Session) error { return s.ThreadLeave(ctx, "T") }},
		{"PUT /channels/T/thread-members/U", func(s *Session) error { return s.ThreadMemberAdd(ctx, "T", "U") }},
		{"DELETE /channels/T/thread-members/U", func(s *Session) error { return s.ThreadMemberRemove(ctx, "T", "U") }},
		{"GET /channels/T/thread-members/U?with_member=true", func(s *Session) error {
			_, err := s.ThreadMember(ctx, "T", "U", true)
			return err
		}},
		{"GET /channels/T/thread-members", func(s *Session) error { _, err := s.ThreadMembers(ctx, "T", false, 0, ""); return err }},
		{"GET /channels/C/threads/archived/public?before=2026-01-02T03%3A04%3A05Z&limit=2", func(s *Session) error {
			_, err := s.ThreadsArchived(ctx, "C", 2, when)
			return err
		}},
		{"GET /channels/C/threads/archived/private", func(s *Session) error {
			_, err := s.ThreadsPrivateArchived(ctx, "C", 0, time.Time{})
			return err
		}},
		{"GET /channels/C/users/@me/threads/archived/private?before=B", func(s *Session) error {
			_, err := s.ThreadsPrivateJoinedArchived(ctx, "C", 0, "B")
			return err
		}},
		{"GET /guilds/G/threads/active", func(s *Session) error { _, err := s.GuildThreadsActive(ctx, "G"); return err }},

		// Webhooks.
		{"POST /channels/C/webhooks", func(s *Session) error {
			_, err := s.WebhookCreate(ctx, "C", &discord.WebhookSettings{Name: "w"})
			return err
		}},
		{"GET /channels/C/webhooks", func(s *Session) error { _, err := s.ChannelWebhooks(ctx, "C"); return err }},
		{"GET /guilds/G/webhooks", func(s *Session) error { _, err := s.GuildWebhooks(ctx, "G"); return err }},
		{"GET /webhooks/W", func(s *Session) error { _, err := s.Webhook(ctx, "W", ""); return err }},
		{"GET /webhooks/W/tok", func(s *Session) error { _, err := s.Webhook(ctx, "W", "tok"); return err }},
		{"PATCH /webhooks/W", func(s *Session) error {
			_, err := s.WebhookEdit(ctx, "W", "", &discord.WebhookSettings{})
			return err
		}},
		{"DELETE /webhooks/W/tok", func(s *Session) error { return s.WebhookDelete(ctx, "W", "tok") }},
		{"POST /webhooks/W/tok?thread_id=T&wait=true", func(s *Session) error {
			_, err := s.WebhookExecute(ctx, "W", "tok", true, &discord.WebhookMessage{ThreadID: "T"})
			return err
		}},
		{"POST /webhooks/W/tok?with_components=true", func(s *Session) error {
			_, err := s.WebhookExecute(ctx, "W", "tok", false, &discord.WebhookMessage{WithComponents: true})
			return err
		}},
		{"GET /webhooks/W/tok/messages/M", func(s *Session) error { _, err := s.WebhookMessage(ctx, "W", "tok", "M", ""); return err }},
		{"PATCH /webhooks/W/tok/messages/M?thread_id=T", func(s *Session) error {
			_, err := s.WebhookMessageEdit(ctx, "W", "tok", "M", "T", &discord.WebhookEdit{})
			return err
		}},
		{"DELETE /webhooks/W/tok/messages/M", func(s *Session) error { return s.WebhookMessageDelete(ctx, "W", "tok", "M", "") }},

		// Guilds.
		{"PATCH /guilds/G", func(s *Session) error { _, err := s.GuildEdit(ctx, "G", &discord.GuildParams{}); return err }},
		{"GET /guilds/G/preview", func(s *Session) error { _, err := s.GuildPreview(ctx, "G"); return err }},
		{"DELETE /users/@me/guilds/G", func(s *Session) error { return s.GuildLeave(ctx, "G") }},
		{"POST /guilds/G/channels", func(s *Session) error {
			_, err := s.GuildChannelCreate(ctx, "G", &discord.ChannelParams{Name: "c"})
			return err
		}},
		{"PATCH /guilds/G/channels", func(s *Session) error { return s.GuildChannelsReorder(ctx, "G", nil) }},
		{"GET /guilds/G/members/search?limit=5&query=jo", func(s *Session) error {
			_, err := s.GuildMembersSearch(ctx, "G", "jo", 5)
			return err
		}},
		{"PATCH /guilds/G/members/U", func(s *Session) error {
			_, err := s.GuildMemberEdit(ctx, "G", "U", &discord.MemberParams{})
			return err
		}},
		{"PATCH /guilds/G/members/@me", func(s *Session) error {
			_, err := s.CurrentMemberEdit(ctx, "G", &discord.CurrentMemberParams{})
			return err
		}},
		{"POST /guilds/G/bulk-ban", func(s *Session) error { _, err := s.GuildBanBulk(ctx, "G", []string{"1"}, 0); return err }},
		{"GET /guilds/G/roles/R", func(s *Session) error { _, err := s.GuildRole(ctx, "G", "R"); return err }},
		{"PATCH /guilds/G/roles", func(s *Session) error { _, err := s.GuildRolesReorder(ctx, "G", nil); return err }},
		{"GET /guilds/G/roles/member-counts", func(s *Session) error { _, err := s.GuildRoleMemberCounts(ctx, "G"); return err }},
		{"GET /guilds/G/prune?days=3&include_roles=1%2C2", func(s *Session) error {
			_, err := s.GuildPruneCount(ctx, "G", discord.PruneParams{Days: 3, IncludeRoles: []string{"1", "2"}})
			return err
		}},
		{"POST /guilds/G/prune", func(s *Session) error { _, err := s.GuildPrune(ctx, "G", discord.PruneParams{}); return err }},
		{"GET /guilds/G/integrations", func(s *Session) error { _, err := s.GuildIntegrations(ctx, "G"); return err }},
		{"DELETE /guilds/G/integrations/I", func(s *Session) error { return s.GuildIntegrationDelete(ctx, "G", "I") }},
		{"GET /guilds/G/widget", func(s *Session) error { _, err := s.GuildWidgetSettings(ctx, "G"); return err }},
		{"PATCH /guilds/G/widget", func(s *Session) error {
			_, err := s.GuildWidgetEdit(ctx, "G", &discord.GuildWidgetParams{})
			return err
		}},
		{"GET /guilds/G/widget.json", func(s *Session) error { _, err := s.GuildWidget(ctx, "G"); return err }},
		{"GET /guilds/G/vanity-url", func(s *Session) error { _, err := s.GuildVanityURL(ctx, "G"); return err }},
		{"GET /guilds/G/welcome-screen", func(s *Session) error { _, err := s.GuildWelcomeScreen(ctx, "G"); return err }},
		{"PATCH /guilds/G/welcome-screen", func(s *Session) error {
			_, err := s.GuildWelcomeScreenEdit(ctx, "G", &discord.WelcomeScreenParams{})
			return err
		}},
		{"GET /guilds/G/onboarding", func(s *Session) error { _, err := s.GuildOnboarding(ctx, "G"); return err }},
		{"PUT /guilds/G/onboarding", func(s *Session) error {
			_, err := s.GuildOnboardingEdit(ctx, "G", &discord.GuildOnboarding{})
			return err
		}},
		{"PUT /guilds/G/incident-actions", func(s *Session) error {
			_, err := s.GuildIncidentActionsEdit(ctx, "G", &discord.IncidentActions{})
			return err
		}},
		{"GET /guilds/G/audit-logs?action_type=22&limit=10&user_id=U", func(s *Session) error {
			_, err := s.GuildAuditLog(ctx, "G", discord.AuditLogQuery{UserID: "U", ActionType: discord.AuditLogMemberBanAdd, Limit: 10})
			return err
		}},
		{"GET /guilds/G/regions", func(s *Session) error { _, err := s.GuildVoiceRegions(ctx, "G"); return err }},
		{"GET /guilds/G/voice-states/@me", func(s *Session) error { _, err := s.VoiceState(ctx, "G", ""); return err }},
		{"PATCH /guilds/G/voice-states/U", func(s *Session) error {
			return s.VoiceStateEdit(ctx, "G", "U", &discord.VoiceStateParams{})
		}},

		// Emojis, stickers and sounds.
		{"GET /guilds/G/emojis", func(s *Session) error { _, err := s.GuildEmojis(ctx, "G"); return err }},
		{"GET /guilds/G/emojis/E", func(s *Session) error { _, err := s.GuildEmoji(ctx, "G", "E"); return err }},
		{"POST /guilds/G/emojis", func(s *Session) error {
			_, err := s.GuildEmojiCreate(ctx, "G", &discord.EmojiParams{Name: "e"})
			return err
		}},
		{"PATCH /guilds/G/emojis/E", func(s *Session) error {
			_, err := s.GuildEmojiEdit(ctx, "G", "E", &discord.EmojiParams{Name: "e"})
			return err
		}},
		{"DELETE /guilds/G/emojis/E", func(s *Session) error { return s.GuildEmojiDelete(ctx, "G", "E") }},
		{"GET /applications/A/emojis", func(s *Session) error { _, err := s.ApplicationEmojis(ctx, "A"); return err }},
		{"GET /applications/A/emojis/E", func(s *Session) error { _, err := s.ApplicationEmoji(ctx, "A", "E"); return err }},
		{"POST /applications/A/emojis", func(s *Session) error {
			_, err := s.ApplicationEmojiCreate(ctx, "A", &discord.EmojiParams{Name: "e"})
			return err
		}},
		{"PATCH /applications/A/emojis/E", func(s *Session) error { _, err := s.ApplicationEmojiEdit(ctx, "A", "E", "n"); return err }},
		{"DELETE /applications/A/emojis/E", func(s *Session) error { return s.ApplicationEmojiDelete(ctx, "A", "E") }},
		{"GET /stickers/S", func(s *Session) error { _, err := s.Sticker(ctx, "S"); return err }},
		{"GET /sticker-packs", func(s *Session) error { _, err := s.StickerPacks(ctx); return err }},
		{"GET /sticker-packs/P", func(s *Session) error { _, err := s.StickerPack(ctx, "P"); return err }},
		{"GET /guilds/G/stickers", func(s *Session) error { _, err := s.GuildStickers(ctx, "G"); return err }},
		{"GET /guilds/G/stickers/S", func(s *Session) error { _, err := s.GuildSticker(ctx, "G", "S"); return err }},
		{"PATCH /guilds/G/stickers/S", func(s *Session) error {
			_, err := s.GuildStickerEdit(ctx, "G", "S", &discord.StickerEdit{})
			return err
		}},
		{"DELETE /guilds/G/stickers/S", func(s *Session) error { return s.GuildStickerDelete(ctx, "G", "S") }},
		{"POST /channels/C/send-soundboard-sound", func(s *Session) error { return s.SoundboardSoundSend(ctx, "C", "S", "") }},
		{"GET /soundboard-default-sounds", func(s *Session) error { _, err := s.SoundboardDefaultSounds(ctx); return err }},
		{"GET /guilds/G/soundboard-sounds", func(s *Session) error { _, err := s.GuildSoundboardSounds(ctx, "G"); return err }},
		{"GET /guilds/G/soundboard-sounds/S", func(s *Session) error { _, err := s.GuildSoundboardSound(ctx, "G", "S"); return err }},
		{"POST /guilds/G/soundboard-sounds", func(s *Session) error {
			_, err := s.GuildSoundboardSoundCreate(ctx, "G", &discord.SoundboardSoundParams{})
			return err
		}},
		{"PATCH /guilds/G/soundboard-sounds/S", func(s *Session) error {
			_, err := s.GuildSoundboardSoundEdit(ctx, "G", "S", &discord.SoundboardSoundParams{})
			return err
		}},
		{"DELETE /guilds/G/soundboard-sounds/S", func(s *Session) error { return s.GuildSoundboardSoundDelete(ctx, "G", "S") }},

		// Stages, auto moderation, scheduled events and templates.
		{"POST /stage-instances", func(s *Session) error {
			_, err := s.StageInstanceCreate(ctx, &discord.StageInstanceParams{})
			return err
		}},
		{"GET /stage-instances/C", func(s *Session) error { _, err := s.StageInstance(ctx, "C"); return err }},
		{"PATCH /stage-instances/C", func(s *Session) error {
			_, err := s.StageInstanceEdit(ctx, "C", &discord.StageInstanceParams{})
			return err
		}},
		{"DELETE /stage-instances/C", func(s *Session) error { return s.StageInstanceDelete(ctx, "C") }},
		{"GET /guilds/G/auto-moderation/rules", func(s *Session) error { _, err := s.AutoModerationRules(ctx, "G"); return err }},
		{"GET /guilds/G/auto-moderation/rules/R", func(s *Session) error { _, err := s.AutoModerationRule(ctx, "G", "R"); return err }},
		{"POST /guilds/G/auto-moderation/rules", func(s *Session) error {
			_, err := s.AutoModerationRuleCreate(ctx, "G", &discord.AutoModerationRuleParams{})
			return err
		}},
		{"PATCH /guilds/G/auto-moderation/rules/R", func(s *Session) error {
			_, err := s.AutoModerationRuleEdit(ctx, "G", "R", &discord.AutoModerationRuleParams{})
			return err
		}},
		{"DELETE /guilds/G/auto-moderation/rules/R", func(s *Session) error { return s.AutoModerationRuleDelete(ctx, "G", "R") }},
		{"GET /guilds/G/scheduled-events?with_user_count=true", func(s *Session) error {
			_, err := s.GuildScheduledEvents(ctx, "G", true)
			return err
		}},
		{"GET /guilds/G/scheduled-events/E", func(s *Session) error { _, err := s.GuildScheduledEvent(ctx, "G", "E", false); return err }},
		{"POST /guilds/G/scheduled-events", func(s *Session) error {
			_, err := s.GuildScheduledEventCreate(ctx, "G", &discord.GuildScheduledEventParams{})
			return err
		}},
		{"PATCH /guilds/G/scheduled-events/E", func(s *Session) error {
			_, err := s.GuildScheduledEventEdit(ctx, "G", "E", &discord.GuildScheduledEventParams{})
			return err
		}},
		{"DELETE /guilds/G/scheduled-events/E", func(s *Session) error { return s.GuildScheduledEventDelete(ctx, "G", "E") }},
		{"GET /guilds/G/scheduled-events/E/users?after=A&limit=5&with_member=true", func(s *Session) error {
			_, err := s.GuildScheduledEventUsers(ctx, "G", "E", 5, true, "A")
			return err
		}},
		{"GET /guilds/templates/code", func(s *Session) error { _, err := s.GuildTemplate(ctx, "code"); return err }},
		{"GET /guilds/G/templates", func(s *Session) error { _, err := s.GuildTemplates(ctx, "G"); return err }},
		{"POST /guilds/G/templates", func(s *Session) error {
			_, err := s.GuildTemplateCreate(ctx, "G", &discord.GuildTemplateParams{Name: "t"})
			return err
		}},
		{"PUT /guilds/G/templates/code", func(s *Session) error { _, err := s.GuildTemplateSync(ctx, "G", "code"); return err }},
		{"PATCH /guilds/G/templates/code", func(s *Session) error {
			_, err := s.GuildTemplateEdit(ctx, "G", "code", &discord.GuildTemplateParams{})
			return err
		}},
		{"DELETE /guilds/G/templates/code", func(s *Session) error { return s.GuildTemplateDelete(ctx, "G", "code") }},

		// Users, the application and monetization.
		{"GET /users/@me", func(s *Session) error { _, err := s.CurrentUser(ctx); return err }},
		{"GET /users/U", func(s *Session) error { _, err := s.UserGet(ctx, "U"); return err }},
		{"PATCH /users/@me", func(s *Session) error {
			_, err := s.CurrentUserEdit(ctx, &discord.CurrentUserParams{})
			return err
		}},
		{"GET /users/@me/guilds?after=A&limit=2&with_counts=true", func(s *Session) error {
			_, err := s.CurrentUserGuilds(ctx, 2, "A")
			return err
		}},
		{"GET /voice/regions", func(s *Session) error { _, err := s.VoiceRegions(ctx); return err }},
		{"GET /applications/@me", func(s *Session) error { _, err := s.CurrentApplication(ctx); return err }},
		{"PATCH /applications/@me", func(s *Session) error {
			_, err := s.CurrentApplicationEdit(ctx, &discord.ApplicationParams{})
			return err
		}},
		{"GET /applications/A/role-connections/metadata", func(s *Session) error { _, err := s.RoleConnectionMetadata(ctx, "A"); return err }},
		{"PUT /applications/A/role-connections/metadata", func(s *Session) error {
			_, err := s.RoleConnectionMetadataSet(ctx, "A", nil)
			return err
		}},
		{"GET /applications/A/skus", func(s *Session) error { _, err := s.SKUs(ctx, "A"); return err }},
		{"GET /applications/A/entitlements?exclude_ended=true&sku_ids=1%2C2&user_id=U", func(s *Session) error {
			_, err := s.Entitlements(ctx, "A", discord.EntitlementsQuery{UserID: "U", SKUIDs: []string{"1", "2"}, ExcludeEnded: true})
			return err
		}},
		{"GET /applications/A/entitlements/E", func(s *Session) error { _, err := s.Entitlement(ctx, "A", "E"); return err }},
		{"POST /applications/A/entitlements/E/consume", func(s *Session) error { return s.EntitlementConsume(ctx, "A", "E") }},
		{"POST /applications/A/entitlements", func(s *Session) error {
			_, err := s.TestEntitlementCreate(ctx, "A", &discord.TestEntitlementParams{})
			return err
		}},
		{"DELETE /applications/A/entitlements/E", func(s *Session) error { return s.TestEntitlementDelete(ctx, "A", "E") }},
		{"GET /skus/K/subscriptions?user_id=U", func(s *Session) error { _, err := s.SKUSubscriptions(ctx, "K", "U", 0, ""); return err }},
		{"GET /skus/K/subscriptions/S", func(s *Session) error { _, err := s.SKUSubscription(ctx, "K", "S"); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			var got string
			s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
				got = r.Method + " " + r.URL.RequestURI()
				w.WriteHeader(http.StatusNoContent)
			})

			if err := tt.call(s); err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// captureBody runs call against a server that answers reply, and returns the
// JSON body of the request it made.
func captureBody(t *testing.T, reply string, call func(s *Session) error) map[string]any {
	t.Helper()

	var body map[string]any
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("request body: %v", err)
		}
		io.WriteString(w, reply)
	})

	if err := call(s); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestMemberEditBodies(t *testing.T) {
	ctx := context.Background()
	until := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	body := captureBody(t, `{}`, func(s *Session) error { return s.GuildMemberTimeout(ctx, "G", "U", until) })
	if len(body) != 1 || body["communication_disabled_until"] != "2026-01-02T03:04:05Z" {
		t.Errorf("timeout body = %v", body)
	}

	body = captureBody(t, `{}`, func(s *Session) error { return s.GuildMemberTimeout(ctx, "G", "U", time.Time{}) })
	if v, ok := body["communication_disabled_until"]; !ok || v != nil || len(body) != 1 {
		t.Errorf("ending a timeout must send null, got %v", body)
	}

	body = captureBody(t, `{}`, func(s *Session) error { return s.GuildMemberMove(ctx, "G", "U", "") })
	if v, ok := body["channel_id"]; !ok || v != nil {
		t.Errorf("disconnecting must send a null channel, got %v", body)
	}

	body = captureBody(t, `{}`, func(s *Session) error { return s.GuildMemberNickname(ctx, "G", "U", "") })
	if v, ok := body["nick"]; !ok || v != "" || len(body) != 1 {
		t.Errorf("nickname body = %v", body)
	}
}

func TestChannelParamsLeavesUnsetFieldsOut(t *testing.T) {
	ctx := context.Background()

	body := captureBody(t, `{"id":"C"}`, func(s *Session) error {
		_, err := s.ChannelEdit(ctx, "C", &discord.ChannelParams{Name: "new", ParentID: discord.Null[string]()})
		return err
	})
	if v, ok := body["parent_id"]; len(body) != 2 || body["name"] != "new" || !ok || v != nil {
		t.Errorf("body = %v", body)
	}

	body = captureBody(t, ``, func(s *Session) error {
		return s.ChannelPermissionSet(ctx, "C", &discord.PermissionOverwrite{
			ID: "R", Type: discord.PermissionOverwriteMember, Allow: discord.PermissionSendMessages,
		})
	})
	if len(body) != 3 || body["type"] != float64(1) || body["allow"] != "2048" || body["deny"] != "0" {
		t.Errorf("overwrite body = %v", body)
	}
}

func TestWrappedListsAreUnwrapped(t *testing.T) {
	ctx := context.Background()

	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/emojis"):
			io.WriteString(w, `{"items":[{"id":"1","name":"e"}]}`)
		case strings.HasSuffix(r.URL.Path, "/sticker-packs"):
			io.WriteString(w, `{"sticker_packs":[{"id":"2"}]}`)
		case strings.Contains(r.URL.Path, "/polls/"):
			io.WriteString(w, `{"users":[{"id":"3"}]}`)
		case strings.Contains(r.URL.Path, "/permissions/"):
			io.WriteString(w, `{"permissions":"2048"}`)
		case strings.HasSuffix(r.URL.Path, "/prune"):
			io.WriteString(w, `{"pruned":7}`)
		case strings.HasSuffix(r.URL.Path, "/threads"):
			io.WriteString(w, `{"id":"T","name":"post","message":{"id":"M"}}`)
		}
	})

	if emojis, err := s.ApplicationEmojis(ctx, "A"); err != nil || len(emojis) != 1 || emojis[0].Name != "e" {
		t.Errorf("emojis = %v, %v", emojis, err)
	}
	if packs, err := s.StickerPacks(ctx); err != nil || len(packs) != 1 || packs[0].ID != "2" {
		t.Errorf("packs = %v, %v", packs, err)
	}
	if users, err := s.PollAnswerVoters(ctx, "C", "M", 1, 0, ""); err != nil || len(users) != 1 || users[0].ID != "3" {
		t.Errorf("voters = %v, %v", users, err)
	}
	if n, err := s.GuildPrune(ctx, "G", discord.PruneParams{}); err != nil || n != 7 {
		t.Errorf("pruned = %d, %v", n, err)
	}

	if perms, err := s.ChannelMemberPermissions(ctx, "C", "U"); err != nil || perms != discord.PermissionSendMessages {
		t.Errorf("permissions = %d, %v", perms, err)
	}

	thread, err := s.ForumThreadStart(ctx, "C", &discord.ForumThreadParams{Name: "post", Message: &discord.MessageSend{Content: "hi"}})
	if err != nil || thread.ID != "T" || thread.Message == nil || thread.Message.ID != "M" {
		t.Errorf("forum thread = %+v, %v", thread, err)
	}
}

func TestWebhookExecuteWithoutWaitReturnsNoMessage(t *testing.T) {
	body := captureBody(t, ``, func(s *Session) error {
		m, err := s.WebhookExecute(context.Background(), "W", "tok", false, &discord.WebhookMessage{
			WebhookParams: discord.WebhookParams{Content: "hi"}, Username: "hook", ThreadID: "T",
		})
		if m != nil {
			t.Errorf("message = %+v, want nil", m)
		}
		return err
	})
	if body["content"] != "hi" || body["username"] != "hook" || len(body) != 2 {
		t.Errorf("body = %v", body)
	}
}

func TestGuildStickerCreateSendsForm(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatal(err)
		}

		got := map[string]string{}
		reader := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(part)
			got[part.FormName()] = string(data)
			if part.FormName() == "file" && (part.FileName() != "s.png" || part.Header.Get("Content-Type") != "image/png") {
				t.Errorf("file part = %q %q", part.FileName(), part.Header.Get("Content-Type"))
			}
		}

		want := map[string]string{"name": "wave", "description": "hi", "tags": "wave", "file": "PNG"}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %q, want %q", k, got[k], v)
			}
		}
		io.WriteString(w, `{"id":"S"}`)
	})

	sticker, err := s.GuildStickerCreate(context.Background(), "G", &discord.StickerParams{
		Name: "wave", Description: "hi", Tags: "wave",
		File: &discord.File{Name: "s.png", ContentType: "image/png", Reader: strings.NewReader("PNG")},
	})
	if err != nil || sticker.ID != "S" {
		t.Errorf("sticker = %+v, %v", sticker, err)
	}

	if _, err := s.GuildStickerCreate(context.Background(), "G", &discord.StickerParams{Name: "x"}); err == nil {
		t.Error("a sticker without a file was accepted")
	}
}

func TestGatewayBot(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/gateway/bot" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"url":"wss://gateway.example","shards":3,"session_start_limit":{"total":1000,"remaining":9,"reset_after":5,"max_concurrency":2}}`)
	})

	info, err := s.GatewayBot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.URL != "wss://gateway.example" || info.Shards != 3 || info.SessionStartLimit.Remaining != 9 || info.SessionStartLimit.MaxConcurrency != 2 {
		t.Errorf("info = %+v", info)
	}
}

func TestGatewayBotWithoutABodyIsAnError(t *testing.T) {
	for _, body := range []string{"", "null"} {
		s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, body) })

		if info, err := s.GatewayBot(context.Background()); err == nil || info != nil {
			t.Errorf("body %q: info = %v, err = %v", body, info, err)
		}
		// Open reads the answer, and must fail rather than panic.
		if err := s.Open(t.Context()); err == nil {
			t.Errorf("body %q: Open succeeded", body)
		}
	}
}

func TestGatewayCommandsNeedConnection(t *testing.T) {
	s := New("Bot token")

	if err := s.RequestGuildMembers(discord.GuildMembersRequest{GuildID: "G"}); !errors.Is(err, ErrNotConnected) {
		t.Errorf("RequestGuildMembers = %v", err)
	}
	if err := s.UpdateVoiceState("G", "C", false, false); !errors.Is(err, ErrNotConnected) {
		t.Errorf("UpdateVoiceState = %v", err)
	}
}

func TestGatewayCommandsAndNewEvents(t *testing.T) {
	type sent struct {
		op int
		d  map[string]any
	}
	commands := make(chan sent, 5)
	seen := make(chan string, 32)
	ready := make(chan struct{})

	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(1)

		for _, op := range []int{opRequestGuildMembers, opRequestGuildMembers, opVoiceStateUpdate, opVoiceStateUpdate, opRequestChannelInfo} {
			p := c.read(op)
			var d map[string]any
			if err := json.Unmarshal(p.D, &d); err != nil {
				t.Error(err)
			}
			commands <- sent{p.Op, d}
		}

		events := []struct {
			name string
			data any
		}{
			{"RESUMED", nil},
			{"CHANNEL_CREATE", map[string]any{"id": "c1", "name": "general", "permission_overwrites": []any{map[string]any{"id": "r", "type": 0, "allow": "8", "deny": "0"}}}},
			{"THREAD_CREATE", map[string]any{"id": "t1", "type": 11, "newly_created": true, "thread_metadata": map[string]any{"archived": false, "archive_timestamp": "2026-01-02T03:04:05Z"}}},
			{"GUILD_ROLE_CREATE", map[string]any{"guild_id": "g1", "role": map[string]any{"id": "r1", "permissions": "8", "tags": map[string]any{"premium_subscriber": nil}}}},
			{"GUILD_MEMBERS_CHUNK", map[string]any{"guild_id": "g1", "members": []any{map[string]any{"user": map[string]any{"id": "u1"}}}, "chunk_index": 0, "chunk_count": 1, "nonce": "n"}},
			{"PRESENCE_UPDATE", map[string]any{"user": map[string]any{"id": "u1"}, "status": "idle", "client_status": map[string]any{"web": "idle"}}},
			{"VOICE_STATE_UPDATE", map[string]any{"guild_id": "g1", "channel_id": nil, "user_id": "u1", "session_id": "s"}},
			{"VOICE_SERVER_UPDATE", map[string]any{"guild_id": "g1", "token": "tok", "endpoint": "voice.example"}},
			{"MESSAGE_REACTION_REMOVE", map[string]any{"user_id": "u1", "message_id": "m1", "emoji": map[string]any{"id": "9", "name": "x"}}},
			{"AUTO_MODERATION_ACTION_EXECUTION", map[string]any{"guild_id": "g1", "rule_id": "r", "action": map[string]any{"type": 1}, "matched_keyword": "bad"}},
			{"RATE_LIMITED", map[string]any{"opcode": 8, "retry_after": 1.5, "meta": map[string]any{"guild_id": "g1", "nonce": "n"}}},
			{"VOICE_CHANNEL_STATUS_UPDATE", map[string]any{"id": "c1", "guild_id": "g1", "status": nil}},
			{"ENTITLEMENT_CREATE", map[string]any{"id": "e1", "sku_id": "k1", "type": 8}},
			{"INTERACTION_CREATE", map[string]any{"id": "i1", "type": 2, "data": map[string]any{"name": "ping"}}},
		}
		<-ready
		for i, e := range events {
			c.send(opDispatch, e.data, i+2, e.name)
		}
		c.waitClosed()
	})

	s.OnResumed(func(*Session, *discord.Resumed) { seen <- "resumed" })
	s.OnChannelCreate(func(_ *Session, e *discord.ChannelCreate) {
		seen <- "channel " + e.Name + " " + e.Mention() + " " + e.PermissionOverwrites[0].ID
	})
	s.OnThreadCreate(func(_ *Session, e *discord.ThreadCreate) {
		if e.NewlyCreated && e.IsThread() && e.ThreadMetadata != nil {
			seen <- "thread " + e.ID
		}
	})
	s.OnGuildRoleCreate(func(_ *Session, e *discord.GuildRoleCreate) {
		if e.Role.Tags != nil && e.Role.Tags.PremiumSubscriber && !e.Role.Tags.GuildConnections && e.Role.Permissions == 8 {
			seen <- "role " + e.Role.ID
		}
	})
	s.OnGuildMembersChunk(func(_ *Session, e *discord.GuildMembersChunk) {
		seen <- "chunk " + e.Nonce + " " + e.Members[0].User.ID
	})
	s.OnPresenceUpdate(func(_ *Session, e *discord.PresenceUpdateEvent) { seen <- "presence " + string(e.ClientStatus.Web) })
	s.OnVoiceStateUpdate(func(_ *Session, e *discord.VoiceStateUpdate) { seen <- "voice state " + e.UserID + e.ChannelID })
	s.OnVoiceServerUpdate(func(_ *Session, e *discord.VoiceServerUpdate) { seen <- "voice server " + e.Endpoint })
	s.OnMessageReactionRemove(func(_ *Session, e *discord.MessageReactionRemove) { seen <- "unreact " + e.Emoji.APIName() })
	s.OnAutoModerationActionExecution(func(_ *Session, e *discord.AutoModerationActionExecution) {
		seen <- "automod " + e.MatchedKeyword
	})
	s.OnRateLimited(func(_ *Session, e *discord.RateLimited) {
		if e.Opcode == opRequestGuildMembers && e.RetryAfter == 1.5 {
			seen <- "rate limited " + e.Meta.GuildID + " " + e.Meta.Nonce
		}
	})
	s.OnVoiceChannelStatusUpdate(func(_ *Session, e *discord.VoiceChannelStatusUpdate) { seen <- "status " + e.ID + e.Status })
	s.OnEntitlementCreate(func(_ *Session, e *discord.EntitlementCreate) { seen <- "entitlement " + e.SKUID })
	s.OnInteractionCreate(func(_ *Session, e *discord.InteractionCreate) {
		seen <- "interaction " + e.ApplicationCommandData().Name
	})
	// Two handlers of one event share a single decoded value.
	s.OnEntitlementCreate(func(_ *Session, e *discord.EntitlementCreate) { seen <- "entitlement again " + e.ID })

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.RequestGuildMembers(discord.GuildMembersRequest{GuildID: "g1", Nonce: "n"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestGuildMembers(discord.GuildMembersRequest{GuildID: "g1", UserIDs: []string{"u1"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateVoiceState("g1", "c1", true, false); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateVoiceState("g1", "", false, false); err != nil {
		t.Fatal(err)
	}

	all := <-commands
	if q, ok := all.d["query"]; !ok || q != "" || all.d["limit"] != float64(0) || all.d["nonce"] != "n" {
		t.Errorf("request for all members = %v", all.d)
	}
	byID := <-commands
	if _, ok := byID.d["query"]; ok || len(byID.d["user_ids"].([]any)) != 1 {
		t.Errorf("request by ID = %v", byID.d)
	}
	join := <-commands
	if join.d["channel_id"] != "c1" || join.d["self_mute"] != true || join.d["self_deaf"] != false {
		t.Errorf("join = %v", join.d)
	}
	leave := <-commands
	if v, ok := leave.d["channel_id"]; !ok || v != nil {
		t.Errorf("leaving must send a null channel, got %v", leave.d)
	}

	// Without guilds there is nothing to request, and without fields all are.
	if err := s.RequestSoundboardSounds(); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestChannelInfo("g1"); err != nil {
		t.Fatal(err)
	}
	info := <-commands
	if fields, _ := info.d["fields"].([]any); info.op != opRequestChannelInfo || len(fields) != 2 {
		t.Errorf("channel info request = %v", info.d)
	}
	close(ready)

	want := []string{
		"resumed", "channel general <#c1> r", "thread t1", "role r1", "chunk n u1", "presence idle",
		"voice state u1", "voice server voice.example", "unreact x:9", "automod bad",
		"entitlement k1", "entitlement again e1", "interaction ping", "rate limited g1 n", "status c1",
	}

	done := make(chan struct{})
	got := map[string]bool{}
	go func() {
		defer close(done)
		for range want {
			got[<-seen] = true
		}
	}()
	waitFor(t, done, "new events")

	for _, w := range want {
		if !got[w] {
			t.Errorf("missing %q in %v", w, got)
		}
	}
}
