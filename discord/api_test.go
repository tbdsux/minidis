package discord

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOptional(t *testing.T) {
	type params struct {
		A Optional[string] `json:"a,omitzero"`
		B Optional[string] `json:"b,omitzero"`
		C Optional[int]    `json:"c,omitzero"`
	}

	got, err := json.Marshal(params{B: Null[string](), C: Some(0)})
	if err != nil || string(got) != `{"b":null,"c":0}` {
		t.Errorf("marshal = %s, %v", got, err)
	}

	var back params
	if err := json.Unmarshal([]byte(`{"a":"x","b":null}`), &back); err != nil {
		t.Fatal(err)
	}
	if v, ok := back.A.Get(); !ok || v != "x" || !back.B.IsNull() || !back.C.IsZero() {
		t.Errorf("unmarshal = %+v", back)
	}
}

func TestGuildMembersRequestEncoding(t *testing.T) {
	for want, req := range map[string]GuildMembersRequest{
		`{"guild_id":"g","limit":0,"query":""}`:                   {GuildID: "g"},
		`{"guild_id":"g","limit":5,"presences":true,"query":"a"}`: {GuildID: "g", Query: "a", Limit: 5, Presences: true},
		`{"guild_id":"g","nonce":"n","user_ids":["1"]}`:           {GuildID: "g", UserIDs: []string{"1"}, Limit: 5, Nonce: "n"},
	} {
		if got, err := json.Marshal(req); err != nil || string(got) != want {
			t.Errorf("got %s, %v; want %s", got, err, want)
		}
	}
}

func TestSnowflakeTime(t *testing.T) {
	// The example from Discord's documentation.
	want := time.UnixMilli(1462015105796)
	if got := SnowflakeTime("175928847299117063"); !got.Equal(want) {
		t.Errorf("SnowflakeTime = %v, want %v", got, want)
	}
	if !SnowflakeTime("nope").IsZero() {
		t.Error("an invalid ID must give the zero time")
	}
}

func TestMentionsAndURLs(t *testing.T) {
	u := &User{ID: "175928847299117063", Username: "name", Avatar: "a_hash"}
	if u.Mention() != "<@175928847299117063>" || u.DisplayName() != "name" {
		t.Errorf("user = %s %s", u.Mention(), u.DisplayName())
	}
	if got := u.AvatarURL(64); got != CDN+"/avatars/175928847299117063/a_hash.gif?size=64" {
		t.Errorf("animated avatar = %s", got)
	}

	u.Avatar = ""
	if got := u.AvatarURL(0); got != CDN+"/embed/avatars/2.png" { // (id >> 22) % 6
		t.Errorf("default avatar = %s", got)
	}
	u.Discriminator = "0008"
	if got := u.AvatarURL(0); got != CDN+"/embed/avatars/3.png" {
		t.Errorf("legacy default avatar = %s", got)
	}

	m := &Member{User: u, Nick: "nick", Avatar: "hash"}
	if m.DisplayName() != "nick" || m.AvatarURL("g", 0) != CDN+"/guilds/g/users/175928847299117063/avatars/hash.png" {
		t.Errorf("member = %s %s", m.DisplayName(), m.AvatarURL("g", 0))
	}

	custom := &Emoji{ID: "1", Name: "wave", Animated: true}
	if custom.APIName() != "wave:1" || custom.MessageFormat() != "<a:wave:1>" || custom.URL(0) != CDN+"/emojis/1.gif" {
		t.Errorf("emoji = %s %s %s", custom.APIName(), custom.MessageFormat(), custom.URL(0))
	}
	if unicode := (&Emoji{Name: "👍"}); unicode.APIName() != "👍" || unicode.MessageFormat() != "👍" || unicode.URL(0) != "" {
		t.Error("a Unicode emoji is its own name")
	}

	if got := (&Guild{ID: "g"}).IconURL(0); got != "" {
		t.Errorf("icon of a guild without one = %q", got)
	}
	if got := (&Message{ID: "m", ChannelID: "c"}).URL(); got != "https://discord.com/channels/@me/c/m" {
		t.Errorf("message URL = %s", got)
	}
	if got := Timestamp(time.Unix(1618953630, 0), TimestampRelative); got != "<t:1618953630:R>" {
		t.Errorf("timestamp = %s", got)
	}
	if got := ImageData("image/png", []byte("hi")); got != "data:image/png;base64,aGk=" {
		t.Errorf("image data = %s", got)
	}
}

func TestMemberPermissions(t *testing.T) {
	guild := &Guild{ID: "g", OwnerID: "owner", Roles: []*Role{
		{ID: "g", Permissions: PermissionViewChannel | PermissionSendMessages},
		{ID: "mod", Permissions: PermissionManageMessages},
		{ID: "admin", Permissions: PermissionAdministrator},
	}}
	channel := &Channel{PermissionOverwrites: []*PermissionOverwrite{
		{ID: "u", Type: PermissionOverwriteMember, Allow: PermissionAttachFiles | PermissionAddReactions},
		{ID: "hidden", Type: PermissionOverwriteMember, Deny: PermissionViewChannel},
		{ID: "mod", Type: PermissionOverwriteRole, Allow: PermissionSendMessages},
		{ID: "g", Type: PermissionOverwriteRole, Deny: PermissionSendMessages},
		{ID: "other", Type: PermissionOverwriteRole, Allow: PermissionBanMembers},
	}}

	// A forum post: members cannot start posts but can reply in them.
	thread := &Channel{Type: ChannelTypeGuildPublicThread, PermissionOverwrites: []*PermissionOverwrite{
		{ID: "u", Type: PermissionOverwriteMember, Allow: PermissionAttachFiles},
		{ID: "g", Type: PermissionOverwriteRole, Deny: PermissionSendMessages, Allow: PermissionSendMessagesInThreads},
	}}

	member := func(id string, roles ...string) *Member { return &Member{User: &User{ID: id}, Roles: roles} }
	timedOut := func(m *Member) *Member {
		until := time.Now().Add(time.Hour)
		m.TimeoutEnds = &until
		return m
	}
	expired := member("x", "mod")
	past := time.Now().Add(-time.Hour)
	expired.TimeoutEnds = &past

	tests := []struct {
		name    string
		channel *Channel
		member  *Member
		want    int64
	}{
		{"guild-wide", nil, member("u"), PermissionViewChannel | PermissionSendMessages},
		{"everyone is denied; files go with sending", channel, member("u"), PermissionViewChannel | PermissionAddReactions},
		{"nothing without seeing the channel", channel, member("hidden", "mod"), 0},
		{"a timeout leaves viewing", channel, timedOut(member("x", "mod")), PermissionViewChannel},
		{"in the guild too", nil, timedOut(member("x", "mod")), PermissionViewChannel},
		{"but not for administrators", channel, timedOut(member("x", "admin")), PermissionAll},
		{"a timeout that is over does nothing", nil, expired,
			PermissionViewChannel | PermissionSendMessages | PermissionManageMessages},
		{"a role overrides the denial of everyone", channel, member("x", "mod"),
			PermissionViewChannel | PermissionSendMessages | PermissionManageMessages},
		{"administrators skip overwrites", channel, member("x", "admin"), PermissionAll},
		{"so does the owner", channel, member("owner"), PermissionAll},
		{"files go with sending in threads in a thread", thread, member("u"),
			PermissionViewChannel | PermissionSendMessagesInThreads | PermissionAttachFiles},
	}
	for _, tt := range tests {
		if got := MemberPermissions(guild, tt.channel, tt.member); got != tt.want {
			t.Errorf("%s: got %b, want %b", tt.name, got, tt.want)
		}
	}
}

func TestRoleTagsDecodeNullAsPresent(t *testing.T) {
	var role Role
	if err := json.Unmarshal([]byte(`{"id":"r","tags":{"bot_id":"b","guild_connections":null}}`), &role); err != nil {
		t.Fatal(err)
	}
	if role.Tags.BotID != "b" || !role.Tags.GuildConnections || role.Tags.PremiumSubscriber {
		t.Errorf("tags = %+v", role.Tags)
	}
}
