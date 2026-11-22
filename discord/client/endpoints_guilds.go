package client

import (
	"context"
	"net/http"
	"time"

	"github.com/tbdsux/minidis/discord"
)

func guildPath(guildID string) string { return "/guilds/" + esc(guildID) }

// GuildEdit changes the settings of a guild.
func (s *Session) GuildEdit(ctx context.Context, guildID string, params *discord.GuildParams) (*discord.Guild, error) {
	return call[*discord.Guild](ctx, s, http.MethodPatch, guildPath(guildID), params)
}

// GuildPreview fetches the public preview of a guild. The bot need not be
// in it if the guild is discoverable.
func (s *Session) GuildPreview(ctx context.Context, guildID string) (*discord.GuildPreview, error) {
	return fetch[*discord.GuildPreview](ctx, s, guildPath(guildID)+"/preview")
}

// GuildLeave makes the bot leave a guild.
func (s *Session) GuildLeave(ctx context.Context, guildID string) error {
	return s.Request(ctx, http.MethodDelete, "/users/@me/guilds/"+esc(guildID), nil, nil)
}

// GuildChannelCreate creates a channel in a guild. It needs a Name, and
// Discord makes a text channel unless Type says otherwise.
func (s *Session) GuildChannelCreate(ctx context.Context, guildID string, params *discord.ChannelParams) (*discord.Channel, error) {
	return call[*discord.Channel](ctx, s, http.MethodPost, guildPath(guildID)+"/channels", params)
}

// GuildChannelsReorder moves channels of a guild. Only the channels that
// change need to be listed.
func (s *Session) GuildChannelsReorder(ctx context.Context, guildID string, positions []discord.ChannelPosition) error {
	return s.Request(ctx, http.MethodPatch, guildPath(guildID)+"/channels", positions, nil)
}

// GuildMembersSearch lists up to limit members (1 when 0, 1000 at most)
// whose username or nickname starts with the query.
func (s *Session) GuildMembersSearch(ctx context.Context, guildID, search string, limit int) ([]*discord.Member, error) {
	path := new(query).str("query", search).num("limit", limit).on(guildPath(guildID) + "/members/search")
	return fetch[[]*discord.Member](ctx, s, path)
}

// GuildMemberEdit changes a member: their nickname, roles, timeout, or
// their state in a voice channel.
func (s *Session) GuildMemberEdit(ctx context.Context, guildID, userID string, params *discord.MemberParams) (*discord.Member, error) {
	return call[*discord.Member](ctx, s, http.MethodPatch, memberPath(guildID, userID), params)
}

// GuildMemberTimeout times a member out until the given time, up to 28 days
// ahead. A zero time ends the timeout.
func (s *Session) GuildMemberTimeout(ctx context.Context, guildID, userID string, until time.Time) error {
	params := &discord.MemberParams{TimeoutUntil: discord.Null[time.Time]()}
	if !until.IsZero() {
		params.TimeoutUntil = discord.Some(until)
	}
	_, err := s.GuildMemberEdit(ctx, guildID, userID, params)
	return err
}

// GuildMemberNickname sets the nickname of a member, or removes it if nick
// is empty.
func (s *Session) GuildMemberNickname(ctx context.Context, guildID, userID, nick string) error {
	_, err := s.GuildMemberEdit(ctx, guildID, userID, &discord.MemberParams{Nick: &nick})
	return err
}

// GuildMemberMove moves a member who is in a voice channel to another one,
// or disconnects them if channelID is empty.
func (s *Session) GuildMemberMove(ctx context.Context, guildID, userID, channelID string) error {
	params := &discord.MemberParams{ChannelID: discord.Null[string]()}
	if channelID != "" {
		params.ChannelID = discord.Some(channelID)
	}
	_, err := s.GuildMemberEdit(ctx, guildID, userID, params)
	return err
}

// CurrentMemberEdit changes the bot's own member of a guild: its nickname
// and its profile for that guild.
func (s *Session) CurrentMemberEdit(ctx context.Context, guildID string, params *discord.CurrentMemberParams) (*discord.Member, error) {
	return call[*discord.Member](ctx, s, http.MethodPatch, memberPath(guildID, "@me"), params)
}

// GuildBanBulk bans up to 200 users from a guild, deleting the messages
// they sent in the last deleteMessageSeconds (up to 604800, or 0 for none).
// It fails only if none of them could be banned.
func (s *Session) GuildBanBulk(ctx context.Context, guildID string, userIDs []string, deleteMessageSeconds int) (*discord.BulkBanResult, error) {
	body := struct {
		UserIDs              []string `json:"user_ids"`
		DeleteMessageSeconds int      `json:"delete_message_seconds,omitempty"`
	}{userIDs, deleteMessageSeconds}
	return call[*discord.BulkBanResult](ctx, s, http.MethodPost, guildPath(guildID)+"/bulk-ban", body)
}

// GuildRole fetches a role of a guild.
func (s *Session) GuildRole(ctx context.Context, guildID, roleID string) (*discord.Role, error) {
	return fetch[*discord.Role](ctx, s, guildPath(guildID)+"/roles/"+esc(roleID))
}

// GuildRolesReorder moves roles of a guild, and returns all of its roles.
// Only the roles that change need to be listed.
func (s *Session) GuildRolesReorder(ctx context.Context, guildID string, positions []discord.RolePosition) ([]*discord.Role, error) {
	return call[[]*discord.Role](ctx, s, http.MethodPatch, guildPath(guildID)+"/roles", positions)
}

// GuildRoleMemberCounts returns how many members have each role of a guild,
// by role ID. The @everyone role is not counted.
func (s *Session) GuildRoleMemberCounts(ctx context.Context, guildID string) (map[string]int, error) {
	return fetch[map[string]int](ctx, s, guildPath(guildID)+"/roles/member-counts")
}

// GuildPruneCount returns how many members a prune with the same parameters
// would remove.
func (s *Session) GuildPruneCount(ctx context.Context, guildID string, params discord.PruneParams) (int, error) {
	path := new(query).num("days", params.Days).str("include_roles", joinIDs(params.IncludeRoles)).on(guildPath(guildID) + "/prune")
	out, err := fetch[struct {
		Pruned int `json:"pruned"`
	}](ctx, s, path)
	return out.Pruned, err
}

// GuildPrune kicks the inactive members params selects, and returns how many
// there were.
func (s *Session) GuildPrune(ctx context.Context, guildID string, params discord.PruneParams) (int, error) {
	body := struct {
		Days         int      `json:"days,omitempty"`
		IncludeRoles []string `json:"include_roles,omitempty"`
	}{params.Days, params.IncludeRoles}
	out, err := call[struct {
		Pruned int `json:"pruned"`
	}](ctx, s, http.MethodPost, guildPath(guildID)+"/prune", body)
	return out.Pruned, err
}

// GuildInvites lists the invites to a guild.
func (s *Session) GuildInvites(ctx context.Context, guildID string) ([]*discord.Invite, error) {
	return fetch[[]*discord.Invite](ctx, s, guildPath(guildID)+"/invites")
}

// GuildIntegrations lists the integrations of a guild, up to 50.
func (s *Session) GuildIntegrations(ctx context.Context, guildID string) ([]*discord.Integration, error) {
	return fetch[[]*discord.Integration](ctx, s, guildPath(guildID)+"/integrations")
}

// GuildIntegrationDelete removes an integration from a guild, along with
// its webhooks and its bot.
func (s *Session) GuildIntegrationDelete(ctx context.Context, guildID, integrationID string) error {
	return s.Request(ctx, http.MethodDelete, guildPath(guildID)+"/integrations/"+esc(integrationID), nil, nil)
}

// GuildWidgetSettings fetches the settings of a guild's widget.
func (s *Session) GuildWidgetSettings(ctx context.Context, guildID string) (*discord.GuildWidgetSettings, error) {
	return fetch[*discord.GuildWidgetSettings](ctx, s, guildPath(guildID)+"/widget")
}

// GuildWidgetEdit changes the settings of a guild's widget.
func (s *Session) GuildWidgetEdit(ctx context.Context, guildID string, params *discord.GuildWidgetParams) (*discord.GuildWidgetSettings, error) {
	return call[*discord.GuildWidgetSettings](ctx, s, http.MethodPatch, guildPath(guildID)+"/widget", params)
}

// GuildWidget fetches the public widget of a guild that has one enabled.
func (s *Session) GuildWidget(ctx context.Context, guildID string) (*discord.GuildWidget, error) {
	return fetch[*discord.GuildWidget](ctx, s, guildPath(guildID)+"/widget.json")
}

// GuildVanityURL fetches the custom invite of a guild that has one.
func (s *Session) GuildVanityURL(ctx context.Context, guildID string) (*discord.VanityURL, error) {
	return fetch[*discord.VanityURL](ctx, s, guildPath(guildID)+"/vanity-url")
}

// GuildWelcomeScreen fetches the welcome screen of a guild.
func (s *Session) GuildWelcomeScreen(ctx context.Context, guildID string) (*discord.WelcomeScreen, error) {
	return fetch[*discord.WelcomeScreen](ctx, s, guildPath(guildID)+"/welcome-screen")
}

// GuildWelcomeScreenEdit changes the welcome screen of a guild.
func (s *Session) GuildWelcomeScreenEdit(ctx context.Context, guildID string, params *discord.WelcomeScreenParams) (*discord.WelcomeScreen, error) {
	return call[*discord.WelcomeScreen](ctx, s, http.MethodPatch, guildPath(guildID)+"/welcome-screen", params)
}

// GuildOnboarding fetches the onboarding of a guild.
func (s *Session) GuildOnboarding(ctx context.Context, guildID string) (*discord.GuildOnboarding, error) {
	return fetch[*discord.GuildOnboarding](ctx, s, guildPath(guildID)+"/onboarding")
}

// GuildOnboardingEdit replaces the onboarding of a guild.
func (s *Session) GuildOnboardingEdit(ctx context.Context, guildID string, onboarding *discord.GuildOnboarding) (*discord.GuildOnboarding, error) {
	return call[*discord.GuildOnboarding](ctx, s, http.MethodPut, guildPath(guildID)+"/onboarding", onboarding)
}

// GuildIncidentActionsEdit pauses invites or DMs between members of a guild
// for a while, or resumes them.
func (s *Session) GuildIncidentActionsEdit(ctx context.Context, guildID string, actions *discord.IncidentActions) (*discord.IncidentsData, error) {
	return call[*discord.IncidentsData](ctx, s, http.MethodPut, guildPath(guildID)+"/incident-actions", actions)
}

// GuildAuditLog fetches entries of a guild's audit log, which needs the
// View Audit Log permission.
func (s *Session) GuildAuditLog(ctx context.Context, guildID string, q discord.AuditLogQuery) (*discord.AuditLog, error) {
	path := new(query).str("user_id", q.UserID).num("action_type", int(q.ActionType)).
		str("before", q.Before).str("after", q.After).num("limit", q.Limit).
		on(guildPath(guildID) + "/audit-logs")
	return fetch[*discord.AuditLog](ctx, s, path)
}

// GuildVoiceRegions lists the voice regions a guild can use.
func (s *Session) GuildVoiceRegions(ctx context.Context, guildID string) ([]*discord.VoiceRegion, error) {
	return fetch[[]*discord.VoiceRegion](ctx, s, guildPath(guildID)+"/regions")
}

func voiceStatePath(guildID, userID string) string {
	if userID == "" {
		userID = "@me"
	}
	return guildPath(guildID) + "/voice-states/" + esc(userID)
}

// VoiceState fetches the voice state of a user in a guild, or the bot's own
// if userID is empty. It fails with a 404 for a user who is not in a voice
// channel.
func (s *Session) VoiceState(ctx context.Context, guildID, userID string) (*discord.VoiceState, error) {
	return fetch[*discord.VoiceState](ctx, s, voiceStatePath(guildID, userID))
}

// VoiceStateEdit changes the voice state of a user in a stage channel, or
// the bot's own if userID is empty: whether they are a speaker, and for the
// bot, its request to speak.
func (s *Session) VoiceStateEdit(ctx context.Context, guildID, userID string, params *discord.VoiceStateParams) error {
	return s.Request(ctx, http.MethodPatch, voiceStatePath(guildID, userID), params, nil)
}

func emojiPath(guildID, emojiID string) string {
	return guildPath(guildID) + "/emojis/" + esc(emojiID)
}

// GuildEmojis lists the emojis of a guild.
func (s *Session) GuildEmojis(ctx context.Context, guildID string) ([]*discord.Emoji, error) {
	return fetch[[]*discord.Emoji](ctx, s, guildPath(guildID)+"/emojis")
}

// GuildEmoji fetches an emoji of a guild.
func (s *Session) GuildEmoji(ctx context.Context, guildID, emojiID string) (*discord.Emoji, error) {
	return fetch[*discord.Emoji](ctx, s, emojiPath(guildID, emojiID))
}

// GuildEmojiCreate adds an emoji to a guild. It needs a Name and an Image
// of at most 256 KiB.
func (s *Session) GuildEmojiCreate(ctx context.Context, guildID string, params *discord.EmojiParams) (*discord.Emoji, error) {
	return call[*discord.Emoji](ctx, s, http.MethodPost, guildPath(guildID)+"/emojis", params)
}

// GuildEmojiEdit changes the name of an emoji of a guild, or the roles that
// may use it.
func (s *Session) GuildEmojiEdit(ctx context.Context, guildID, emojiID string, params *discord.EmojiParams) (*discord.Emoji, error) {
	return call[*discord.Emoji](ctx, s, http.MethodPatch, emojiPath(guildID, emojiID), params)
}

// GuildEmojiDelete deletes an emoji of a guild.
func (s *Session) GuildEmojiDelete(ctx context.Context, guildID, emojiID string) error {
	return s.Request(ctx, http.MethodDelete, emojiPath(guildID, emojiID), nil, nil)
}

func autoModPath(guildID, ruleID string) string {
	return guildPath(guildID) + "/auto-moderation/rules/" + esc(ruleID)
}

// AutoModerationRules lists the auto moderation rules of a guild.
func (s *Session) AutoModerationRules(ctx context.Context, guildID string) ([]*discord.AutoModerationRule, error) {
	return fetch[[]*discord.AutoModerationRule](ctx, s, guildPath(guildID)+"/auto-moderation/rules")
}

// AutoModerationRule fetches an auto moderation rule.
func (s *Session) AutoModerationRule(ctx context.Context, guildID, ruleID string) (*discord.AutoModerationRule, error) {
	return fetch[*discord.AutoModerationRule](ctx, s, autoModPath(guildID, ruleID))
}

// AutoModerationRuleCreate creates an auto moderation rule. It needs a
// Name, an EventType, a TriggerType and at least one action.
func (s *Session) AutoModerationRuleCreate(ctx context.Context, guildID string, params *discord.AutoModerationRuleParams) (*discord.AutoModerationRule, error) {
	return call[*discord.AutoModerationRule](ctx, s, http.MethodPost, guildPath(guildID)+"/auto-moderation/rules", params)
}

// AutoModerationRuleEdit changes an auto moderation rule.
func (s *Session) AutoModerationRuleEdit(ctx context.Context, guildID, ruleID string, params *discord.AutoModerationRuleParams) (*discord.AutoModerationRule, error) {
	return call[*discord.AutoModerationRule](ctx, s, http.MethodPatch, autoModPath(guildID, ruleID), params)
}

// AutoModerationRuleDelete deletes an auto moderation rule.
func (s *Session) AutoModerationRuleDelete(ctx context.Context, guildID, ruleID string) error {
	return s.Request(ctx, http.MethodDelete, autoModPath(guildID, ruleID), nil, nil)
}

func scheduledEventPath(guildID, eventID string) string {
	return guildPath(guildID) + "/scheduled-events/" + esc(eventID)
}

// GuildScheduledEvents lists the scheduled events of a guild, with the
// number of interested users in UserCount if withUserCount is set.
func (s *Session) GuildScheduledEvents(ctx context.Context, guildID string, withUserCount bool) ([]*discord.GuildScheduledEvent, error) {
	path := new(query).flag("with_user_count", withUserCount).on(guildPath(guildID) + "/scheduled-events")
	return fetch[[]*discord.GuildScheduledEvent](ctx, s, path)
}

// GuildScheduledEvent fetches a scheduled event; see GuildScheduledEvents.
func (s *Session) GuildScheduledEvent(ctx context.Context, guildID, eventID string, withUserCount bool) (*discord.GuildScheduledEvent, error) {
	path := new(query).flag("with_user_count", withUserCount).on(scheduledEventPath(guildID, eventID))
	return fetch[*discord.GuildScheduledEvent](ctx, s, path)
}

// GuildScheduledEventCreate schedules an event in a guild.
func (s *Session) GuildScheduledEventCreate(ctx context.Context, guildID string, params *discord.GuildScheduledEventParams) (*discord.GuildScheduledEvent, error) {
	return call[*discord.GuildScheduledEvent](ctx, s, http.MethodPost, guildPath(guildID)+"/scheduled-events", params)
}

// GuildScheduledEventEdit changes a scheduled event. Setting its Status
// starts, ends or cancels it.
func (s *Session) GuildScheduledEventEdit(ctx context.Context, guildID, eventID string, params *discord.GuildScheduledEventParams) (*discord.GuildScheduledEvent, error) {
	return call[*discord.GuildScheduledEvent](ctx, s, http.MethodPatch, scheduledEventPath(guildID, eventID), params)
}

// GuildScheduledEventDelete deletes a scheduled event.
func (s *Session) GuildScheduledEventDelete(ctx context.Context, guildID, eventID string) error {
	return s.Request(ctx, http.MethodDelete, scheduledEventPath(guildID, eventID), nil, nil)
}

// GuildScheduledEventUsers lists up to limit users interested in a scheduled
// event (100 at most, the default when 0), with IDs after afterID if it is
// not empty, and with their guild members if withMember is set.
func (s *Session) GuildScheduledEventUsers(ctx context.Context, guildID, eventID string, limit int, withMember bool, afterID string) ([]*discord.GuildScheduledEventUser, error) {
	path := new(query).num("limit", limit).flag("with_member", withMember).str("after", afterID).
		on(scheduledEventPath(guildID, eventID) + "/users")
	return fetch[[]*discord.GuildScheduledEventUser](ctx, s, path)
}

// GuildTemplate fetches a guild template by its code.
func (s *Session) GuildTemplate(ctx context.Context, code string) (*discord.GuildTemplate, error) {
	return fetch[*discord.GuildTemplate](ctx, s, "/guilds/templates/"+esc(code))
}

func templatePath(guildID, code string) string {
	return guildPath(guildID) + "/templates/" + esc(code)
}

// GuildTemplates lists the templates of a guild.
func (s *Session) GuildTemplates(ctx context.Context, guildID string) ([]*discord.GuildTemplate, error) {
	return fetch[[]*discord.GuildTemplate](ctx, s, guildPath(guildID)+"/templates")
}

// GuildTemplateCreate makes a template of a guild as it is now.
func (s *Session) GuildTemplateCreate(ctx context.Context, guildID string, params *discord.GuildTemplateParams) (*discord.GuildTemplate, error) {
	return call[*discord.GuildTemplate](ctx, s, http.MethodPost, guildPath(guildID)+"/templates", params)
}

// GuildTemplateSync updates a template to the guild as it is now.
func (s *Session) GuildTemplateSync(ctx context.Context, guildID, code string) (*discord.GuildTemplate, error) {
	return call[*discord.GuildTemplate](ctx, s, http.MethodPut, templatePath(guildID, code), nil)
}

// GuildTemplateEdit changes the name or description of a template.
func (s *Session) GuildTemplateEdit(ctx context.Context, guildID, code string, params *discord.GuildTemplateParams) (*discord.GuildTemplate, error) {
	return call[*discord.GuildTemplate](ctx, s, http.MethodPatch, templatePath(guildID, code), params)
}

// GuildTemplateDelete deletes a template.
func (s *Session) GuildTemplateDelete(ctx context.Context, guildID, code string) error {
	return s.Request(ctx, http.MethodDelete, templatePath(guildID, code), nil, nil)
}

// CurrentUser fetches the bot's own user.
func (s *Session) CurrentUser(ctx context.Context) (*discord.User, error) {
	return fetch[*discord.User](ctx, s, "/users/@me")
}

// UserGet fetches a user. (User returns the bot's user from the gateway
// session.)
func (s *Session) UserGet(ctx context.Context, userID string) (*discord.User, error) {
	return fetch[*discord.User](ctx, s, "/users/"+esc(userID))
}

// CurrentUserEdit changes the bot's username, avatar or banner.
func (s *Session) CurrentUserEdit(ctx context.Context, params *discord.CurrentUserParams) (*discord.User, error) {
	return call[*discord.User](ctx, s, http.MethodPatch, "/users/@me", params)
}

// CurrentUserGuilds lists up to limit guilds the bot is in (200 at most,
// the default when 0), with IDs after afterID if it is not empty. The guilds
// are partial, with their approximate member counts.
func (s *Session) CurrentUserGuilds(ctx context.Context, limit int, afterID string) ([]*discord.Guild, error) {
	path := new(query).num("limit", limit).str("after", afterID).str("with_counts", "true").on("/users/@me/guilds")
	return fetch[[]*discord.Guild](ctx, s, path)
}

// VoiceRegions lists the voice regions.
func (s *Session) VoiceRegions(ctx context.Context) ([]*discord.VoiceRegion, error) {
	return fetch[[]*discord.VoiceRegion](ctx, s, "/voice/regions")
}

// CurrentApplication fetches the bot's application.
func (s *Session) CurrentApplication(ctx context.Context) (*discord.Application, error) {
	return fetch[*discord.Application](ctx, s, "/applications/@me")
}

// CurrentApplicationEdit changes the bot's application.
func (s *Session) CurrentApplicationEdit(ctx context.Context, params *discord.ApplicationParams) (*discord.Application, error) {
	return call[*discord.Application](ctx, s, http.MethodPatch, "/applications/@me", params)
}

func appPath(appID string) string { return "/applications/" + esc(appID) }

// ApplicationEmojis lists the emojis of an app, which the bot can use
// anywhere.
func (s *Session) ApplicationEmojis(ctx context.Context, appID string) ([]*discord.Emoji, error) {
	out, err := fetch[struct {
		Items []*discord.Emoji `json:"items"`
	}](ctx, s, appPath(appID)+"/emojis")
	return out.Items, err
}

// ApplicationEmoji fetches an emoji of an app.
func (s *Session) ApplicationEmoji(ctx context.Context, appID, emojiID string) (*discord.Emoji, error) {
	return fetch[*discord.Emoji](ctx, s, appPath(appID)+"/emojis/"+esc(emojiID))
}

// ApplicationEmojiCreate adds an emoji to an app. It needs a Name and an
// Image of at most 256 KiB.
func (s *Session) ApplicationEmojiCreate(ctx context.Context, appID string, params *discord.EmojiParams) (*discord.Emoji, error) {
	return call[*discord.Emoji](ctx, s, http.MethodPost, appPath(appID)+"/emojis", params)
}

// ApplicationEmojiEdit renames an emoji of an app.
func (s *Session) ApplicationEmojiEdit(ctx context.Context, appID, emojiID, name string) (*discord.Emoji, error) {
	return call[*discord.Emoji](ctx, s, http.MethodPatch, appPath(appID)+"/emojis/"+esc(emojiID), &discord.EmojiParams{Name: name})
}

// ApplicationEmojiDelete deletes an emoji of an app.
func (s *Session) ApplicationEmojiDelete(ctx context.Context, appID, emojiID string) error {
	return s.Request(ctx, http.MethodDelete, appPath(appID)+"/emojis/"+esc(emojiID), nil, nil)
}

// RoleConnectionMetadata lists the metadata an app reports for linked roles.
func (s *Session) RoleConnectionMetadata(ctx context.Context, appID string) ([]*discord.RoleConnectionMetadata, error) {
	return fetch[[]*discord.RoleConnectionMetadata](ctx, s, appPath(appID)+"/role-connections/metadata")
}

// RoleConnectionMetadataSet replaces the metadata an app reports for linked
// roles, of which there can be up to five.
func (s *Session) RoleConnectionMetadataSet(ctx context.Context, appID string, metadata []*discord.RoleConnectionMetadata) ([]*discord.RoleConnectionMetadata, error) {
	return call[[]*discord.RoleConnectionMetadata](ctx, s, http.MethodPut, appPath(appID)+"/role-connections/metadata", metadata)
}

// SKUs lists the premium offerings of an app.
func (s *Session) SKUs(ctx context.Context, appID string) ([]*discord.SKU, error) {
	return fetch[[]*discord.SKU](ctx, s, appPath(appID)+"/skus")
}

// Entitlements lists entitlements to the SKUs of an app.
func (s *Session) Entitlements(ctx context.Context, appID string, q discord.EntitlementsQuery) ([]*discord.Entitlement, error) {
	path := new(query).str("user_id", q.UserID).str("sku_ids", joinIDs(q.SKUIDs)).
		str("before", q.Before).str("after", q.After).num("limit", q.Limit).str("guild_id", q.GuildID).
		flag("exclude_ended", q.ExcludeEnded).flag("exclude_deleted", q.ExcludeDeleted).
		on(appPath(appID) + "/entitlements")
	return fetch[[]*discord.Entitlement](ctx, s, path)
}

// Entitlement fetches an entitlement.
func (s *Session) Entitlement(ctx context.Context, appID, entitlementID string) (*discord.Entitlement, error) {
	return fetch[*discord.Entitlement](ctx, s, appPath(appID)+"/entitlements/"+esc(entitlementID))
}

// EntitlementConsume marks the entitlement to a one-time purchase as used.
func (s *Session) EntitlementConsume(ctx context.Context, appID, entitlementID string) error {
	return s.Request(ctx, http.MethodPost, appPath(appID)+"/entitlements/"+esc(entitlementID)+"/consume", nil, nil)
}

// TestEntitlementCreate gives a user or guild an entitlement for testing,
// without a purchase.
func (s *Session) TestEntitlementCreate(ctx context.Context, appID string, params *discord.TestEntitlementParams) (*discord.Entitlement, error) {
	return call[*discord.Entitlement](ctx, s, http.MethodPost, appPath(appID)+"/entitlements", params)
}

// TestEntitlementDelete deletes an entitlement made for testing.
func (s *Session) TestEntitlementDelete(ctx context.Context, appID, entitlementID string) error {
	return s.Request(ctx, http.MethodDelete, appPath(appID)+"/entitlements/"+esc(entitlementID), nil, nil)
}

// SKUSubscriptions lists up to limit subscriptions to a SKU (100 at most,
// Discord's default of 50 when 0), with IDs after afterID if it is not
// empty. userID limits them to those of one user, and is required unless the
// request is made with an OAuth2 token.
func (s *Session) SKUSubscriptions(ctx context.Context, skuID, userID string, limit int, afterID string) ([]*discord.Subscription, error) {
	path := new(query).str("user_id", userID).num("limit", limit).str("after", afterID).on("/skus/" + esc(skuID) + "/subscriptions")
	return fetch[[]*discord.Subscription](ctx, s, path)
}

// SKUSubscription fetches a subscription to a SKU.
func (s *Session) SKUSubscription(ctx context.Context, skuID, subscriptionID string) (*discord.Subscription, error) {
	return fetch[*discord.Subscription](ctx, s, "/skus/"+esc(skuID)+"/subscriptions/"+esc(subscriptionID))
}
