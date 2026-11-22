package discord

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// CDN is the base URL of Discord's image CDN.
const CDN = "https://cdn.discordapp.com"

// discordEpoch is the first millisecond of 2015, which snowflake IDs count from.
const discordEpoch = 1420070400000

// SnowflakeTime returns when the object with the given ID was created, or
// the zero time if id is not a snowflake.
func SnowflakeTime(id string) time.Time {
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli(int64(n>>22) + discordEpoch)
}

// ImageData encodes a file as the data URI that Discord takes for images
// and sounds, such as an avatar, an emoji or a soundboard sound. contentType
// is its media type, such as "image/png".
func ImageData(contentType string, data []byte) string {
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// cdnURL builds the URL of an image. An animated one, whose hash starts with
// "a_", is served as a GIF and the others as PNG. A size of 0 leaves it to
// Discord; otherwise it is a power of two from 16 to 4096.
func cdnURL(path, hash string, size int) string {
	if hash == "" {
		return ""
	}

	ext := ".png"
	if strings.HasPrefix(hash, "a_") {
		ext = ".gif"
	}

	u := CDN + "/" + path + "/" + hash + ext
	if size > 0 {
		u += "?size=" + strconv.Itoa(size)
	}
	return u
}

// Mention returns the text that mentions the user in a message.
func (u *User) Mention() string { return "<@" + u.ID + ">" }

// DisplayName returns the name the user is shown with outside of guilds.
func (u *User) DisplayName() string {
	if u.GlobalName != "" {
		return u.GlobalName
	}
	return u.Username
}

// AvatarURL returns the URL of the user's avatar, or of the default avatar
// Discord shows for a user without one. See Guild.IconURL for size.
func (u *User) AvatarURL(size int) string {
	if u.Avatar != "" {
		return cdnURL("avatars/"+u.ID, u.Avatar, size)
	}

	// Users on the current username system are spread over six avatars by
	// ID, those with a legacy discriminator over five by that.
	index := uint64(0)
	if d, err := strconv.ParseUint(u.Discriminator, 10, 64); err == nil && d != 0 {
		index = d % 5
	} else if id, err := strconv.ParseUint(u.ID, 10, 64); err == nil {
		index = (id >> 22) % 6
	}
	return CDN + "/embed/avatars/" + strconv.FormatUint(index, 10) + ".png"
}

// BannerURL returns the URL of the user's banner, or "" without one.
func (u *User) BannerURL(size int) string {
	return cdnURL("banners/"+u.ID, u.Banner, size)
}

// Mention returns the text that mentions the member in a message, or "" if
// the member has no User.
func (m *Member) Mention() string {
	if m.User == nil {
		return ""
	}
	return m.User.Mention()
}

// DisplayName returns the name the member is shown with in the guild.
func (m *Member) DisplayName() string {
	switch {
	case m.Nick != "":
		return m.Nick
	case m.User != nil:
		return m.User.DisplayName()
	}
	return ""
}

// AvatarURL returns the URL of the avatar the member is shown with in the
// guild: the one set for that guild, or else the user's.
func (m *Member) AvatarURL(guildID string, size int) string {
	if m.User == nil {
		return ""
	}
	if m.Avatar != "" {
		return cdnURL("guilds/"+guildID+"/users/"+m.User.ID+"/avatars", m.Avatar, size)
	}
	return m.User.AvatarURL(size)
}

// TimedOut reports whether the member is timed out at the moment.
func (m *Member) TimedOut() bool {
	return m.TimeoutEnds != nil && m.TimeoutEnds.After(time.Now())
}

// Mention returns the text that mentions the role in a message.
func (r *Role) Mention() string { return "<@&" + r.ID + ">" }

// Mention returns the text that links to the channel in a message.
func (c *Channel) Mention() string { return "<#" + c.ID + ">" }

// IconURL returns the URL of the guild's icon, or "" without one. A size of
// 0 leaves it to Discord; otherwise it is a power of two from 16 to 4096.
func (g *Guild) IconURL(size int) string {
	return cdnURL("icons/"+g.ID, g.Icon, size)
}

// BannerURL returns the URL of the guild's banner, or "" without one.
func (g *Guild) BannerURL(size int) string {
	return cdnURL("banners/"+g.ID, g.Banner, size)
}

// APIName returns the emoji in the form the reaction endpoints take: the
// emoji itself for a Unicode one, and "name:id" for a custom one.
func (e *Emoji) APIName() string {
	if e.ID == "" {
		return e.Name
	}
	return e.Name + ":" + e.ID
}

// MessageFormat returns the text that shows the emoji in a message.
func (e *Emoji) MessageFormat() string {
	switch {
	case e.ID == "":
		return e.Name
	case e.Animated:
		return "<a:" + e.Name + ":" + e.ID + ">"
	}
	return "<:" + e.Name + ":" + e.ID + ">"
}

// URL returns the URL of a custom emoji's image, or "" for a Unicode emoji.
func (e *Emoji) URL(size int) string {
	if e.ID == "" {
		return ""
	}

	ext := ".png"
	if e.Animated {
		ext = ".gif"
	}

	u := CDN + "/emojis/" + e.ID + ext
	if size > 0 {
		u += "?size=" + strconv.Itoa(size)
	}
	return u
}

// URL returns the link that opens the message in Discord.
func (m *Message) URL() string {
	guild := m.GuildID
	if guild == "" {
		guild = "@me"
	}
	return "https://discord.com/channels/" + guild + "/" + m.ChannelID + "/" + m.ID
}

// TimestampStyle is how Discord shows a timestamp made by Timestamp.
type TimestampStyle string

const (
	TimestampShortTime     TimestampStyle = "t" // 16:20
	TimestampLongTime      TimestampStyle = "T" // 16:20:30
	TimestampShortDate     TimestampStyle = "d" // 20/04/2021
	TimestampLongDate      TimestampStyle = "D" // 20 April 2021
	TimestampShortDateTime TimestampStyle = "f" // 20 April 2021 16:20, the default
	TimestampLongDateTime  TimestampStyle = "F" // Tuesday, 20 April 2021 16:20
	TimestampRelative      TimestampStyle = "R" // 2 months ago
)

// Timestamp returns the text that shows t in a message, in each reader's own
// time zone and language. An empty style is TimestampShortDateTime.
func Timestamp(t time.Time, style TimestampStyle) string {
	if style == "" {
		return "<t:" + strconv.FormatInt(t.Unix(), 10) + ">"
	}
	return "<t:" + strconv.FormatInt(t.Unix(), 10) + ":" + string(style) + ">"
}

// MemberPermissions computes the permissions of a member in a channel from
// the guild's roles and the channel's overwrites, the way Discord does. Pass
// a nil channel for the member's permissions in the guild as a whole.
// guild needs its ID, OwnerID and Roles, and member its User and Roles.
func MemberPermissions(guild *Guild, channel *Channel, member *Member) int64 {
	userID := ""
	if member.User != nil {
		userID = member.User.ID
	}
	if userID != "" && userID == guild.OwnerID {
		return PermissionAll
	}

	held := map[string]bool{guild.ID: true} // the @everyone role has the guild's ID
	for _, id := range member.Roles {
		held[id] = true
	}

	var perms int64
	for _, role := range guild.Roles {
		if held[role.ID] {
			perms |= role.Permissions
		}
	}
	if perms&PermissionAdministrator != 0 {
		return PermissionAll
	}
	if channel == nil {
		return perms
	}

	// Overwrites apply in order: @everyone, then the member's roles together,
	// then the member.
	var roleAllow, roleDeny, memberAllow, memberDeny int64
	for _, o := range channel.PermissionOverwrites {
		switch {
		case o.Type == PermissionOverwriteRole && o.ID == guild.ID:
			perms = perms&^o.Deny | o.Allow
		case o.Type == PermissionOverwriteRole && held[o.ID]:
			roleAllow |= o.Allow
			roleDeny |= o.Deny
		case o.Type == PermissionOverwriteMember && o.ID == userID:
			memberAllow |= o.Allow
			memberDeny |= o.Deny
		}
	}
	perms = perms&^roleDeny | roleAllow
	perms = perms&^memberDeny | memberAllow

	return perms
}
