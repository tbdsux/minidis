package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// fetch performs a GET request and decodes the response into a T.
func fetch[T any](s *Session, path string) (T, error) {
	var out T
	err := s.Request(context.Background(), http.MethodGet, path, nil, &out)
	return out, err
}

// pageQuery encodes the limit and cursor of a list endpoint. A zero limit and
// an empty cursor are left out so that Discord's defaults apply.
func pageQuery(limit int, cursorName, cursor string) string {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if cursor != "" {
		q.Set(cursorName, cursor)
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

func messagePath(channelID, messageID string) string {
	return "/channels/" + channelID + "/messages/" + messageID
}

// ChannelMessage fetches a message.
func (s *Session) ChannelMessage(channelID, messageID string) (*discord.Message, error) {
	return fetch[*discord.Message](s, messagePath(channelID, messageID))
}

// ChannelMessages lists a channel's messages, newest first. Up to limit
// messages are returned (Discord's default when 0), older than beforeID if it
// is not empty.
func (s *Session) ChannelMessages(channelID string, limit int, beforeID string) ([]*discord.Message, error) {
	return fetch[[]*discord.Message](s, "/channels/"+channelID+"/messages"+pageQuery(limit, "before", beforeID))
}

// ChannelMessageEditComplex edits a message.
func (s *Session) ChannelMessageEditComplex(channelID, messageID string, edit *discord.MessageEdit) (*discord.Message, error) {
	out := &discord.Message{}
	if err := s.requestWithFiles(context.Background(), http.MethodPatch, messagePath(channelID, messageID), edit, edit.Files, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ChannelMessageEdit replaces the text of a message.
func (s *Session) ChannelMessageEdit(channelID, messageID, content string) (*discord.Message, error) {
	return s.ChannelMessageEditComplex(channelID, messageID, &discord.MessageEdit{Content: &content})
}

// ChannelMessageDelete deletes a message.
func (s *Session) ChannelMessageDelete(channelID, messageID string) error {
	return s.Request(context.Background(), http.MethodDelete, messagePath(channelID, messageID), nil, nil)
}

// ChannelMessagesBulkDelete deletes 2 to 100 messages that are under 14 days old.
func (s *Session) ChannelMessagesBulkDelete(channelID string, messageIDs []string) error {
	body := struct {
		Messages []string `json:"messages"`
	}{messageIDs}
	return s.Request(context.Background(), http.MethodPost, "/channels/"+channelID+"/messages/bulk-delete", body, nil)
}

func pinPath(channelID, messageID string) string {
	return "/channels/" + channelID + "/messages/pins/" + messageID
}

// ChannelMessagePin pins a message.
func (s *Session) ChannelMessagePin(channelID, messageID string) error {
	return s.Request(context.Background(), http.MethodPut, pinPath(channelID, messageID), nil, nil)
}

// ChannelMessageUnpin unpins a message.
func (s *Session) ChannelMessageUnpin(channelID, messageID string) error {
	return s.Request(context.Background(), http.MethodDelete, pinPath(channelID, messageID), nil, nil)
}

// ChannelPins lists up to limit pinned messages (Discord's default of 50 when
// 0), pinned before the given time if it is not zero. To get the next page while
// HasMore is set, pass the PinnedAt of the last item as before.
func (s *Session) ChannelPins(channelID string, limit int, before time.Time) (*discord.ChannelPins, error) {
	cursor := ""
	if !before.IsZero() {
		cursor = before.Format(time.RFC3339Nano)
	}
	return fetch[*discord.ChannelPins](s, "/channels/"+channelID+"/messages/pins"+pageQuery(limit, "before", cursor))
}

// reactionsPath builds the path of a message's reactions. emoji is a Unicode
// emoji, or "name:id" for a custom one.
func reactionsPath(channelID, messageID, emoji string) string {
	return messagePath(channelID, messageID) + "/reactions/" + url.PathEscape(emoji)
}

// MessageReactionAdd adds the bot's reaction to a message. emoji is a Unicode
// emoji, or "name:id" for a custom one.
func (s *Session) MessageReactionAdd(channelID, messageID, emoji string) error {
	return s.Request(context.Background(), http.MethodPut, reactionsPath(channelID, messageID, emoji)+"/@me", nil, nil)
}

// MessageReactionRemove removes a user's reaction from a message. An empty
// userID means the bot's own reaction.
func (s *Session) MessageReactionRemove(channelID, messageID, emoji, userID string) error {
	if userID == "" {
		userID = "@me"
	}
	return s.Request(context.Background(), http.MethodDelete, reactionsPath(channelID, messageID, emoji)+"/"+userID, nil, nil)
}

// MessageReactionsRemoveAll removes every reaction from a message.
func (s *Session) MessageReactionsRemoveAll(channelID, messageID string) error {
	return s.Request(context.Background(), http.MethodDelete, messagePath(channelID, messageID)+"/reactions", nil, nil)
}

// Channel fetches a channel.
func (s *Session) Channel(channelID string) (*discord.Channel, error) {
	return fetch[*discord.Channel](s, "/channels/"+channelID)
}

// UserChannelCreate opens a DM channel with a user. Send to it with the
// ChannelMessage* methods.
func (s *Session) UserChannelCreate(userID string) (*discord.Channel, error) {
	body := struct {
		RecipientID string `json:"recipient_id"`
	}{userID}

	out := &discord.Channel{}
	if err := s.Request(context.Background(), http.MethodPost, "/users/@me/channels", body, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Guild fetches a guild, including its approximate member count.
func (s *Session) Guild(guildID string) (*discord.Guild, error) {
	return fetch[*discord.Guild](s, "/guilds/"+guildID+"?with_counts=true")
}

// GuildChannels lists a guild's channels.
func (s *Session) GuildChannels(guildID string) ([]*discord.Channel, error) {
	return fetch[[]*discord.Channel](s, "/guilds/"+guildID+"/channels")
}

// GuildRoles lists a guild's roles.
func (s *Session) GuildRoles(guildID string) ([]*discord.Role, error) {
	return fetch[[]*discord.Role](s, "/guilds/"+guildID+"/roles")
}

// GuildRoleCreate creates a role.
func (s *Session) GuildRoleCreate(guildID string, params *discord.RoleParams) (*discord.Role, error) {
	out := &discord.Role{}
	if err := s.Request(context.Background(), http.MethodPost, "/guilds/"+guildID+"/roles", params, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GuildRoleEdit changes a role.
func (s *Session) GuildRoleEdit(guildID, roleID string, params *discord.RoleParams) (*discord.Role, error) {
	out := &discord.Role{}
	if err := s.Request(context.Background(), http.MethodPatch, "/guilds/"+guildID+"/roles/"+roleID, params, out); err != nil {
		return nil, err
	}
	return out, nil
}

// GuildRoleDelete deletes a role.
func (s *Session) GuildRoleDelete(guildID, roleID string) error {
	return s.Request(context.Background(), http.MethodDelete, "/guilds/"+guildID+"/roles/"+roleID, nil, nil)
}

func memberPath(guildID, userID string) string {
	return "/guilds/" + guildID + "/members/" + userID
}

// GuildMember fetches a member of a guild.
func (s *Session) GuildMember(guildID, userID string) (*discord.Member, error) {
	return fetch[*discord.Member](s, memberPath(guildID, userID))
}

// GuildMembers lists up to limit members (Discord's default when 0), with IDs
// after afterID if it is not empty. It needs the privileged IntentsGuildMembers
// intent to be enabled in the developer portal.
func (s *Session) GuildMembers(guildID string, limit int, afterID string) ([]*discord.Member, error) {
	return fetch[[]*discord.Member](s, "/guilds/"+guildID+"/members"+pageQuery(limit, "after", afterID))
}

// GuildMemberRemove kicks a member from a guild.
func (s *Session) GuildMemberRemove(guildID, userID string) error {
	return s.Request(context.Background(), http.MethodDelete, memberPath(guildID, userID), nil, nil)
}

// GuildMemberRoleAdd gives a member a role.
func (s *Session) GuildMemberRoleAdd(guildID, userID, roleID string) error {
	return s.Request(context.Background(), http.MethodPut, memberPath(guildID, userID)+"/roles/"+roleID, nil, nil)
}

// GuildMemberRoleRemove takes a role from a member.
func (s *Session) GuildMemberRoleRemove(guildID, userID, roleID string) error {
	return s.Request(context.Background(), http.MethodDelete, memberPath(guildID, userID)+"/roles/"+roleID, nil, nil)
}

func banPath(guildID, userID string) string {
	return "/guilds/" + guildID + "/bans/" + userID
}

// GuildBan fetches a user's ban from a guild.
func (s *Session) GuildBan(guildID, userID string) (*discord.GuildBan, error) {
	return fetch[*discord.GuildBan](s, banPath(guildID, userID))
}

// GuildBans lists up to limit bans (Discord's default when 0), with user IDs
// after afterID if it is not empty.
func (s *Session) GuildBans(guildID string, limit int, afterID string) ([]*discord.GuildBan, error) {
	return fetch[[]*discord.GuildBan](s, "/guilds/"+guildID+"/bans"+pageQuery(limit, "after", afterID))
}

// GuildBanCreate bans a user from a guild, deleting the messages they sent in
// the last deleteMessageSeconds (up to 604800, or 0 for none).
func (s *Session) GuildBanCreate(guildID, userID string, deleteMessageSeconds int) error {
	body := struct {
		DeleteMessageSeconds int `json:"delete_message_seconds,omitempty"`
	}{deleteMessageSeconds}
	return s.Request(context.Background(), http.MethodPut, banPath(guildID, userID), body, nil)
}

// GuildBanDelete removes a user's ban from a guild.
func (s *Session) GuildBanDelete(guildID, userID string) error {
	return s.Request(context.Background(), http.MethodDelete, banPath(guildID, userID), nil, nil)
}
