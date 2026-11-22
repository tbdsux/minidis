package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// call sends a request with a JSON body and decodes the response into a T.
func call[T any](ctx context.Context, s *Session, method, path string, body any) (T, error) {
	var out T
	err := s.Request(ctx, method, path, body, &out)
	return out, err
}

// query builds the query string of a request, leaving out empty values so
// that Discord's defaults apply.
type query struct{ v url.Values }

func (q *query) str(name, value string) *query {
	if value != "" {
		if q.v == nil {
			q.v = url.Values{}
		}
		q.v.Set(name, value)
	}
	return q
}

func (q *query) num(name string, value int) *query {
	if value > 0 {
		q.str(name, strconv.Itoa(value))
	}
	return q
}

func (q *query) flag(name string, value bool) *query {
	if value {
		q.str(name, "true")
	}
	return q
}

// on appends the query to path.
func (q *query) on(path string) string {
	if len(q.v) == 0 {
		return path
	}
	return path + "?" + q.v.Encode()
}

func channelPath(channelID string) string { return "/channels/" + esc(channelID) }

// ChannelEdit changes a channel or a thread.
func (s *Session) ChannelEdit(ctx context.Context, channelID string, params *discord.ChannelParams) (*discord.Channel, error) {
	return call[*discord.Channel](ctx, s, http.MethodPatch, channelPath(channelID), params)
}

// ChannelDelete deletes a channel or a thread, or closes a DM. Deleting a
// guild channel cannot be undone.
func (s *Session) ChannelDelete(ctx context.Context, channelID string) (*discord.Channel, error) {
	return call[*discord.Channel](ctx, s, http.MethodDelete, channelPath(channelID), nil)
}

// ChannelTyping shows the bot as typing in a channel for 10 seconds, or
// until it sends a message.
func (s *Session) ChannelTyping(ctx context.Context, channelID string) error {
	return s.Request(ctx, http.MethodPost, channelPath(channelID)+"/typing", nil, nil)
}

// ChannelMessagesQuery lists a channel's messages, newest first, before,
// after or around a message.
func (s *Session) ChannelMessagesQuery(ctx context.Context, channelID string, q discord.MessagesQuery) ([]*discord.Message, error) {
	path := new(query).num("limit", q.Limit).str("before", q.Before).str("after", q.After).str("around", q.Around).
		on(channelPath(channelID) + "/messages")
	return fetch[[]*discord.Message](ctx, s, path)
}

// ChannelMessageCrosspost publishes a message of an announcement channel to
// the channels that follow it.
func (s *Session) ChannelMessageCrosspost(ctx context.Context, channelID, messageID string) (*discord.Message, error) {
	return call[*discord.Message](ctx, s, http.MethodPost, messagePath(channelID, messageID)+"/crosspost", nil)
}

// ChannelNewsFollow makes an announcement channel post its published
// messages to targetChannelID as well.
func (s *Session) ChannelNewsFollow(ctx context.Context, channelID, targetChannelID string) (*discord.FollowedChannel, error) {
	body := struct {
		WebhookChannelID string `json:"webhook_channel_id"`
	}{targetChannelID}
	return call[*discord.FollowedChannel](ctx, s, http.MethodPost, channelPath(channelID)+"/followers", body)
}

// ChannelPermissionSet sets the permissions a role or member is allowed and
// denied in a channel, replacing an earlier overwrite for it.
func (s *Session) ChannelPermissionSet(ctx context.Context, channelID string, overwrite *discord.PermissionOverwrite) error {
	body := struct {
		Type  discord.PermissionOverwriteType `json:"type"`
		Allow int64                           `json:"allow,string"`
		Deny  int64                           `json:"deny,string"`
	}{overwrite.Type, overwrite.Allow, overwrite.Deny}
	return s.Request(ctx, http.MethodPut, channelPath(channelID)+"/permissions/"+esc(overwrite.ID), body, nil)
}

// ChannelPermissionDelete removes the overwrite of a role or member from a
// channel.
func (s *Session) ChannelPermissionDelete(ctx context.Context, channelID, overwriteID string) error {
	return s.Request(ctx, http.MethodDelete, channelPath(channelID)+"/permissions/"+esc(overwriteID), nil, nil)
}

// ChannelVoiceStatusSet sets the status shown on a voice channel, of up to
// 500 characters, or clears it if status is empty.
func (s *Session) ChannelVoiceStatusSet(ctx context.Context, channelID, status string) error {
	body := struct {
		Status *string `json:"status"`
	}{}
	if status != "" {
		body.Status = &status
	}
	return s.Request(ctx, http.MethodPut, channelPath(channelID)+"/voice-status", body, nil)
}

// permissions fetches the permissions Discord computed for a member or role.
func (s *Session) permissions(ctx context.Context, path string) (int64, error) {
	out, err := fetch[struct {
		Permissions int64 `json:"permissions,string"`
	}](ctx, s, path)
	return out.Permissions, err
}

// ChannelMemberPermissions asks Discord for the permissions of a member in
// a channel, with their roles and the channel's overwrites applied.
// discord.MemberPermissions computes the same from objects already at hand.
func (s *Session) ChannelMemberPermissions(ctx context.Context, channelID, userID string) (int64, error) {
	return s.permissions(ctx, channelPath(channelID)+"/permissions/member/"+esc(userID))
}

// ChannelRolePermissions asks Discord for the permissions of a role in a
// channel, with the channel's overwrites applied.
func (s *Session) ChannelRolePermissions(ctx context.Context, channelID, roleID string) (int64, error) {
	return s.permissions(ctx, channelPath(channelID)+"/permissions/role/"+esc(roleID))
}

// GuildMemberPermissions asks Discord for the permissions of a member in a
// guild: those of their roles and of @everyone.
func (s *Session) GuildMemberPermissions(ctx context.Context, guildID, userID string) (int64, error) {
	return s.permissions(ctx, "/guilds/"+esc(guildID)+"/permissions/member/"+esc(userID))
}

// GuildRolePermissions asks Discord for the permissions of a role in a
// guild, combined with those of @everyone.
func (s *Session) GuildRolePermissions(ctx context.Context, guildID, roleID string) (int64, error) {
	return s.permissions(ctx, "/guilds/"+esc(guildID)+"/permissions/role/"+esc(roleID))
}

// MessageReactions lists up to limit users who reacted to a message with an
// emoji (Discord's default of 25 when 0), with IDs after afterID if it is not
// empty. emoji is a Unicode emoji, or "name:id" for a custom one.
func (s *Session) MessageReactions(ctx context.Context, channelID, messageID, emoji string, limit int, afterID string) ([]*discord.User, error) {
	return fetch[[]*discord.User](ctx, s, reactionsPath(channelID, messageID, emoji)+pageQuery(limit, "after", afterID))
}

// MessageReactionsRemoveEmoji removes every reaction with one emoji from a
// message.
func (s *Session) MessageReactionsRemoveEmoji(ctx context.Context, channelID, messageID, emoji string) error {
	return s.Request(ctx, http.MethodDelete, reactionsPath(channelID, messageID, emoji), nil, nil)
}

// PollAnswerVoters lists up to limit users who voted for an answer of a poll
// (Discord's default of 25 when 0), with IDs after afterID if it is not empty.
func (s *Session) PollAnswerVoters(ctx context.Context, channelID, messageID string, answerID, limit int, afterID string) ([]*discord.User, error) {
	path := channelPath(channelID) + "/polls/" + esc(messageID) + "/answers/" + strconv.Itoa(answerID)
	out, err := fetch[struct {
		Users []*discord.User `json:"users"`
	}](ctx, s, path+pageQuery(limit, "after", afterID))
	return out.Users, err
}

// PollExpire ends a poll the bot sent before its time.
func (s *Session) PollExpire(ctx context.Context, channelID, messageID string) (*discord.Message, error) {
	return call[*discord.Message](ctx, s, http.MethodPost, channelPath(channelID)+"/polls/"+esc(messageID)+"/expire", nil)
}

// ChannelInvites lists the invites to a channel.
func (s *Session) ChannelInvites(ctx context.Context, channelID string) ([]*discord.Invite, error) {
	return fetch[[]*discord.Invite](ctx, s, channelPath(channelID)+"/invites")
}

// ChannelInviteCreate creates an invite to a channel. A nil params makes one
// that lasts a day and has no limit of uses.
func (s *Session) ChannelInviteCreate(ctx context.Context, channelID string, params *discord.InviteParams) (*discord.Invite, error) {
	if params == nil {
		params = &discord.InviteParams{}
	}
	return call[*discord.Invite](ctx, s, http.MethodPost, channelPath(channelID)+"/invites", params)
}

// Invite fetches an invite by its code, with the approximate member counts
// of its guild.
func (s *Session) Invite(ctx context.Context, code string) (*discord.Invite, error) {
	return fetch[*discord.Invite](ctx, s, "/invites/"+esc(code)+"?with_counts=true")
}

// InviteDelete revokes an invite.
func (s *Session) InviteDelete(ctx context.Context, code string) (*discord.Invite, error) {
	return call[*discord.Invite](ctx, s, http.MethodDelete, "/invites/"+esc(code), nil)
}

// MessageThreadStart starts a thread from a message. The thread gets the ID
// of the message.
func (s *Session) MessageThreadStart(ctx context.Context, channelID, messageID string, params *discord.ThreadParams) (*discord.Channel, error) {
	return call[*discord.Channel](ctx, s, http.MethodPost, messagePath(channelID, messageID)+"/threads", params)
}

// ThreadStart starts a thread that is not attached to a message. Set
// params.Type to discord.ChannelTypeGuildPublicThread for a public one.
func (s *Session) ThreadStart(ctx context.Context, channelID string, params *discord.ThreadParams) (*discord.Channel, error) {
	return call[*discord.Channel](ctx, s, http.MethodPost, channelPath(channelID)+"/threads", params)
}

// ForumThreadStart starts a thread in a forum or media channel. The returned
// thread holds its first message in Message.
func (s *Session) ForumThreadStart(ctx context.Context, channelID string, params *discord.ForumThreadParams) (*discord.ForumThread, error) {
	var files []*discord.File
	if params.Message != nil {
		files = params.Message.Files
	}

	out := &discord.ForumThread{}
	if err := s.requestWithFiles(ctx, http.MethodPost, channelPath(channelID)+"/threads", params, files, out); err != nil {
		return nil, err
	}
	return out, nil
}

func threadMemberPath(threadID, userID string) string {
	if userID == "" {
		userID = "@me"
	}
	return channelPath(threadID) + "/thread-members/" + esc(userID)
}

// ThreadJoin adds the bot to a thread.
func (s *Session) ThreadJoin(ctx context.Context, threadID string) error {
	return s.Request(ctx, http.MethodPut, threadMemberPath(threadID, ""), nil, nil)
}

// ThreadLeave removes the bot from a thread.
func (s *Session) ThreadLeave(ctx context.Context, threadID string) error {
	return s.Request(ctx, http.MethodDelete, threadMemberPath(threadID, ""), nil, nil)
}

// ThreadMemberAdd adds a user to a thread.
func (s *Session) ThreadMemberAdd(ctx context.Context, threadID, userID string) error {
	return s.Request(ctx, http.MethodPut, threadMemberPath(threadID, userID), nil, nil)
}

// ThreadMemberRemove removes a user from a thread.
func (s *Session) ThreadMemberRemove(ctx context.Context, threadID, userID string) error {
	return s.Request(ctx, http.MethodDelete, threadMemberPath(threadID, userID), nil, nil)
}

// ThreadMember fetches a user's membership of a thread, with their guild
// member if withMember is set. It fails with a 404 for a user who is not in
// the thread.
func (s *Session) ThreadMember(ctx context.Context, threadID, userID string, withMember bool) (*discord.ThreadMember, error) {
	return fetch[*discord.ThreadMember](ctx, s, new(query).flag("with_member", withMember).on(threadMemberPath(threadID, userID)))
}

// ThreadMembers lists the members of a thread, which needs the privileged
// IntentsGuildMembers intent. With withMember set, each holds their guild
// member and the list is paged: up to limit (100 at most, the default when
// 0), with user IDs after afterID if it is not empty.
func (s *Session) ThreadMembers(ctx context.Context, threadID string, withMember bool, limit int, afterID string) ([]*discord.ThreadMember, error) {
	path := new(query).flag("with_member", withMember).num("limit", limit).str("after", afterID).
		on(channelPath(threadID) + "/thread-members")
	return fetch[[]*discord.ThreadMember](ctx, s, path)
}

func archiveCursor(before time.Time) string {
	if before.IsZero() {
		return ""
	}
	return before.Format(time.RFC3339Nano)
}

// ThreadsArchived lists the archived public threads of a channel, most
// recently archived first: up to limit (Discord's default when 0), archived
// before the given time if it is not zero.
func (s *Session) ThreadsArchived(ctx context.Context, channelID string, limit int, before time.Time) (*discord.ThreadList, error) {
	return fetch[*discord.ThreadList](ctx, s,
		channelPath(channelID)+"/threads/archived/public"+pageQuery(limit, "before", archiveCursor(before)))
}

// ThreadsPrivateArchived lists the archived private threads of a channel,
// like ThreadsArchived. It needs the Manage Threads permission.
func (s *Session) ThreadsPrivateArchived(ctx context.Context, channelID string, limit int, before time.Time) (*discord.ThreadList, error) {
	return fetch[*discord.ThreadList](ctx, s,
		channelPath(channelID)+"/threads/archived/private"+pageQuery(limit, "before", archiveCursor(before)))
}

// ThreadsPrivateJoinedArchived lists the archived private threads of a
// channel that the bot has joined, newest first: up to limit, with IDs
// before beforeID if it is not empty.
func (s *Session) ThreadsPrivateJoinedArchived(ctx context.Context, channelID string, limit int, beforeID string) (*discord.ThreadList, error) {
	return fetch[*discord.ThreadList](ctx, s,
		channelPath(channelID)+"/users/@me/threads/archived/private"+pageQuery(limit, "before", beforeID))
}

// GuildThreadsActive lists the threads of a guild that are not archived.
func (s *Session) GuildThreadsActive(ctx context.Context, guildID string) (*discord.ThreadList, error) {
	return fetch[*discord.ThreadList](ctx, s, "/guilds/"+esc(guildID)+"/threads/active")
}

// WebhookCreate creates a webhook in a channel. The name cannot contain
// "clyde" or "discord".
func (s *Session) WebhookCreate(ctx context.Context, channelID string, params *discord.WebhookSettings) (*discord.Webhook, error) {
	return call[*discord.Webhook](ctx, s, http.MethodPost, channelPath(channelID)+"/webhooks", params)
}

// ChannelWebhooks lists the webhooks of a channel.
func (s *Session) ChannelWebhooks(ctx context.Context, channelID string) ([]*discord.Webhook, error) {
	return fetch[[]*discord.Webhook](ctx, s, channelPath(channelID)+"/webhooks")
}

// GuildWebhooks lists the webhooks of a guild.
func (s *Session) GuildWebhooks(ctx context.Context, guildID string) ([]*discord.Webhook, error) {
	return fetch[[]*discord.Webhook](ctx, s, "/guilds/"+esc(guildID)+"/webhooks")
}

// webhookPath is the path of a webhook. With a token, the request is
// authorized by it rather than by the bot.
func webhookPath(webhookID, token string) string {
	if token == "" {
		return "/webhooks/" + esc(webhookID)
	}
	return "/webhooks/" + esc(webhookID) + "/" + esc(token)
}

// Webhook fetches a webhook. With its token, the bot needs no access to the
// webhook's channel, and an empty token uses the bot's permissions instead.
func (s *Session) Webhook(ctx context.Context, webhookID, token string) (*discord.Webhook, error) {
	return fetch[*discord.Webhook](ctx, s, webhookPath(webhookID, token))
}

// WebhookEdit changes a webhook. With a token it cannot be moved to another
// channel; see Webhook.
func (s *Session) WebhookEdit(ctx context.Context, webhookID, token string, params *discord.WebhookSettings) (*discord.Webhook, error) {
	return call[*discord.Webhook](ctx, s, http.MethodPatch, webhookPath(webhookID, token), params)
}

// WebhookDelete deletes a webhook; see Webhook for the token.
func (s *Session) WebhookDelete(ctx context.Context, webhookID, token string) error {
	return s.Request(ctx, http.MethodDelete, webhookPath(webhookID, token), nil, nil)
}

// WebhookExecute sends a message through a webhook. With wait set, it
// returns the message once Discord has stored it; otherwise it returns nil
// as soon as Discord has accepted the request.
func (s *Session) WebhookExecute(ctx context.Context, webhookID, token string, wait bool, msg *discord.WebhookMessage) (*discord.Message, error) {
	path := new(query).flag("wait", wait).str("thread_id", msg.ThreadID).flag("with_components", msg.WithComponents).
		on(webhookPath(webhookID, token))

	if !wait {
		return nil, s.requestWithFiles(ctx, http.MethodPost, path, msg, msg.Files, nil)
	}

	out := &discord.Message{}
	if err := s.requestWithFiles(ctx, http.MethodPost, path, msg, msg.Files, out); err != nil {
		return nil, err
	}
	return out, nil
}

// webhookMessageURL is the path of a message a webhook sent. threadID is the
// thread the message is in, if it is in one.
func webhookMessageURL(webhookID, token, messageID, threadID string) string {
	return new(query).str("thread_id", threadID).on(webhookPath(webhookID, token) + "/messages/" + esc(messageID))
}

// WebhookMessage fetches a message a webhook sent. threadID is the thread
// the message is in, or empty.
func (s *Session) WebhookMessage(ctx context.Context, webhookID, token, messageID, threadID string) (*discord.Message, error) {
	return fetch[*discord.Message](ctx, s, webhookMessageURL(webhookID, token, messageID, threadID))
}

// WebhookMessageEdit edits a message a webhook sent.
func (s *Session) WebhookMessageEdit(ctx context.Context, webhookID, token, messageID, threadID string, edit *discord.WebhookEdit) (*discord.Message, error) {
	out := &discord.Message{}
	path := webhookMessageURL(webhookID, token, messageID, threadID)
	if err := s.requestWithFiles(ctx, http.MethodPatch, path, edit, edit.Files, out); err != nil {
		return nil, err
	}
	return out, nil
}

// WebhookMessageDelete deletes a message a webhook sent.
func (s *Session) WebhookMessageDelete(ctx context.Context, webhookID, token, messageID, threadID string) error {
	return s.Request(ctx, http.MethodDelete, webhookMessageURL(webhookID, token, messageID, threadID), nil, nil)
}

// StageInstanceCreate starts a stage in a stage channel.
func (s *Session) StageInstanceCreate(ctx context.Context, params *discord.StageInstanceParams) (*discord.StageInstance, error) {
	return call[*discord.StageInstance](ctx, s, http.MethodPost, "/stage-instances", params)
}

// StageInstance fetches the live stage of a stage channel.
func (s *Session) StageInstance(ctx context.Context, channelID string) (*discord.StageInstance, error) {
	return fetch[*discord.StageInstance](ctx, s, "/stage-instances/"+esc(channelID))
}

// StageInstanceEdit changes the topic or privacy of a live stage.
func (s *Session) StageInstanceEdit(ctx context.Context, channelID string, params *discord.StageInstanceParams) (*discord.StageInstance, error) {
	return call[*discord.StageInstance](ctx, s, http.MethodPatch, "/stage-instances/"+esc(channelID), params)
}

// StageInstanceDelete ends the live stage of a stage channel.
func (s *Session) StageInstanceDelete(ctx context.Context, channelID string) error {
	return s.Request(ctx, http.MethodDelete, "/stage-instances/"+esc(channelID), nil, nil)
}

// Sticker fetches a sticker.
func (s *Session) Sticker(ctx context.Context, stickerID string) (*discord.Sticker, error) {
	return fetch[*discord.Sticker](ctx, s, "/stickers/"+esc(stickerID))
}

// StickerPacks lists the packs of standard stickers.
func (s *Session) StickerPacks(ctx context.Context) ([]*discord.StickerPack, error) {
	out, err := fetch[struct {
		Packs []*discord.StickerPack `json:"sticker_packs"`
	}](ctx, s, "/sticker-packs")
	return out.Packs, err
}

// StickerPack fetches a pack of standard stickers.
func (s *Session) StickerPack(ctx context.Context, packID string) (*discord.StickerPack, error) {
	return fetch[*discord.StickerPack](ctx, s, "/sticker-packs/"+esc(packID))
}

func guildStickerPath(guildID, stickerID string) string {
	return "/guilds/" + esc(guildID) + "/stickers/" + esc(stickerID)
}

// GuildStickers lists the stickers of a guild.
func (s *Session) GuildStickers(ctx context.Context, guildID string) ([]*discord.Sticker, error) {
	return fetch[[]*discord.Sticker](ctx, s, "/guilds/"+esc(guildID)+"/stickers")
}

// GuildSticker fetches a sticker of a guild.
func (s *Session) GuildSticker(ctx context.Context, guildID, stickerID string) (*discord.Sticker, error) {
	return fetch[*discord.Sticker](ctx, s, guildStickerPath(guildID, stickerID))
}

// GuildStickerCreate adds a sticker to a guild.
func (s *Session) GuildStickerCreate(ctx context.Context, guildID string, params *discord.StickerParams) (*discord.Sticker, error) {
	if params.File == nil || params.File.Reader == nil {
		return nil, fmt.Errorf("discord: a sticker needs a file")
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	for name, value := range map[string]string{"name": params.Name, "description": params.Description, "tags": params.Tags} {
		if err := w.WriteField(name, value); err != nil {
			return nil, err
		}
	}

	contentType := params.File.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, multipartEscaper.Replace(params.File.Name)))
	header.Set("Content-Type", contentType)

	part, err := w.CreatePart(header)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, params.File.Reader); err != nil {
		return nil, fmt.Errorf("discord: read sticker file: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	out := &discord.Sticker{}
	if err := s.send(ctx, http.MethodPost, "/guilds/"+esc(guildID)+"/stickers", w.FormDataContentType(), buf.Bytes(), out); err != nil {
		return nil, err
	}
	return out, nil
}

// GuildStickerEdit changes a sticker of a guild.
func (s *Session) GuildStickerEdit(ctx context.Context, guildID, stickerID string, params *discord.StickerEdit) (*discord.Sticker, error) {
	return call[*discord.Sticker](ctx, s, http.MethodPatch, guildStickerPath(guildID, stickerID), params)
}

// GuildStickerDelete deletes a sticker of a guild.
func (s *Session) GuildStickerDelete(ctx context.Context, guildID, stickerID string) error {
	return s.Request(ctx, http.MethodDelete, guildStickerPath(guildID, stickerID), nil, nil)
}

// SoundboardSoundSend plays a soundboard sound in the voice channel the bot
// is connected to. sourceGuildID is the guild the sound is from, or empty
// for a default sound or one of the channel's own guild.
func (s *Session) SoundboardSoundSend(ctx context.Context, channelID, soundID, sourceGuildID string) error {
	body := struct {
		SoundID       string `json:"sound_id"`
		SourceGuildID string `json:"source_guild_id,omitempty"`
	}{soundID, sourceGuildID}
	return s.Request(ctx, http.MethodPost, channelPath(channelID)+"/send-soundboard-sound", body, nil)
}

// SoundboardDefaultSounds lists the sounds every guild has.
func (s *Session) SoundboardDefaultSounds(ctx context.Context) ([]*discord.SoundboardSound, error) {
	return fetch[[]*discord.SoundboardSound](ctx, s, "/soundboard-default-sounds")
}

func soundPath(guildID, soundID string) string {
	return "/guilds/" + esc(guildID) + "/soundboard-sounds/" + esc(soundID)
}

// GuildSoundboardSounds lists the soundboard sounds of a guild.
func (s *Session) GuildSoundboardSounds(ctx context.Context, guildID string) ([]*discord.SoundboardSound, error) {
	out, err := fetch[struct {
		Items []*discord.SoundboardSound `json:"items"`
	}](ctx, s, "/guilds/"+esc(guildID)+"/soundboard-sounds")
	return out.Items, err
}

// GuildSoundboardSound fetches a soundboard sound of a guild.
func (s *Session) GuildSoundboardSound(ctx context.Context, guildID, soundID string) (*discord.SoundboardSound, error) {
	return fetch[*discord.SoundboardSound](ctx, s, soundPath(guildID, soundID))
}

// GuildSoundboardSoundCreate adds a sound to a guild's soundboard.
func (s *Session) GuildSoundboardSoundCreate(ctx context.Context, guildID string, params *discord.SoundboardSoundParams) (*discord.SoundboardSound, error) {
	return call[*discord.SoundboardSound](ctx, s, http.MethodPost, "/guilds/"+esc(guildID)+"/soundboard-sounds", params)
}

// GuildSoundboardSoundEdit changes a soundboard sound of a guild.
func (s *Session) GuildSoundboardSoundEdit(ctx context.Context, guildID, soundID string, params *discord.SoundboardSoundParams) (*discord.SoundboardSound, error) {
	return call[*discord.SoundboardSound](ctx, s, http.MethodPatch, soundPath(guildID, soundID), params)
}

// GuildSoundboardSoundDelete deletes a soundboard sound of a guild.
func (s *Session) GuildSoundboardSoundDelete(ctx context.Context, guildID, soundID string) error {
	return s.Request(ctx, http.MethodDelete, soundPath(guildID, soundID), nil, nil)
}

// joinIDs joins IDs for a query parameter that takes a comma separated list.
func joinIDs(ids []string) string { return strings.Join(ids, ",") }
