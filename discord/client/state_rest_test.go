package client

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/tbdsux/minidis/discord"
)

func hasRole(role string) func(string, *discord.Member) bool {
	return func(_ string, m *discord.Member) bool { return slices.Contains(m.Roles, role) }
}

func TestStateKeepMember(t *testing.T) {
	st := readyState(t, StateOptions{Members: true, KeepMember: hasRole("r1")})
	apply(t, st, "GUILD_CREATE", guildCreate())

	// The bot is always kept; 7 has no role.
	if st.Member("g1", "42") == nil || st.Member("g1", "7") != nil {
		t.Fatalf("members = %v", st.Members("g1"))
	}

	apply(t, st, "GUILD_MEMBER_UPDATE", obj{"guild_id": "g1", "user": user("7"), "roles": []string{"r1"}})
	if st.Member("g1", "7") == nil {
		t.Error("a member who gained the role is not kept")
	}
	apply(t, st, "GUILD_MEMBER_UPDATE", obj{"guild_id": "g1", "user": user("7"), "roles": []string{}})
	if st.Member("g1", "7") != nil {
		t.Error("a member who lost the role is kept")
	}
	apply(t, st, "GUILD_MEMBERS_CHUNK", obj{"guild_id": "g1", "members": []obj{member("8", "r1"), member("9")}})
	if st.Member("g1", "8") == nil || st.Member("g1", "9") != nil {
		t.Error("the filter is not applied to chunks")
	}
}

func TestStateKeepMemberWithoutTheIntent(t *testing.T) {
	opts := StateOptions{KeepMember: func(string, *discord.Member) bool { return true }}
	if NewState(opts).Intents()&discord.IntentsGuildMembers != 0 {
		t.Error("KeepMember alone asks for the privileged intent")
	}

	st := readyState(t, opts)
	apply(t, st, "GUILD_CREATE", guildCreate())
	apply(t, st, "MESSAGE_CREATE", obj{"id": "m1", "channel_id": "c1", "guild_id": "g1", "author": user("11"),
		"member": obj{"roles": []string{}, "joined_at": "2024-01-01T00:00:00Z"}})
	if st.Member("g1", "11") == nil {
		t.Error("the member of a message is not kept")
	}
}

func TestStatePanicInKeepMemberIsRecovered(t *testing.T) {
	st := readyState(t, StateOptions{KeepMember: func(_ string, m *discord.Member) bool {
		if m.User.ID == "7" {
			panic("boom")
		}
		return true
	}})
	var logged bytes.Buffer
	data, err := json.Marshal(guildCreate())
	if err != nil {
		t.Fatal(err)
	}
	st.apply(slog.New(slog.NewTextHandler(&logged, nil)), "GUILD_CREATE", data)
	if !strings.Contains(logged.String(), "event=GUILD_CREATE panic=boom") {
		t.Errorf("logged %q", logged.String())
	}

	// The state is still usable: the lock was released.
	apply(t, st, "GUILD_MEMBER_ADD", obj{"guild_id": "g1", "user": user("8"), "roles": []string{}})
	if st.Member("g1", "8") == nil {
		t.Error("the state stopped working after a panic")
	}
}

// restDiscord answers the REST requests of the Cached methods, and counts them.
type restDiscord struct {
	mu       sync.Mutex
	requests []string
}

func (d *restDiscord) seen() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.requests)
}

func (d *restDiscord) session(t *testing.T) *Session {
	g := guildCreate()
	c1 := maps.Clone(g["channels"].([]obj)[0])
	c1["guild_id"] = "g1" // sent by the REST API, unlike inside GUILD_CREATE
	replies := map[string]any{
		"/guilds/g1":               obj{"id": "g1", "name": "Fetched", "owner_id": "1", "roles": g["roles"]},
		"/guilds/g1/roles/r1":      obj{"id": "r1", "name": "talker"},
		"/channels/c1":             c1,
		"/channels/t9":             obj{"id": "t9", "guild_id": "g1", "parent_id": "c1", "type": 11},
		"/channels/c1/messages/m1": obj{"id": "m1", "channel_id": "c1", "content": "old"},
		"/guilds/g1/members/7":     member("7", "r1"),
		"/guilds/g1/members/42":    member("42", "r1"),
	}
	return newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.requests = append(d.requests, r.URL.Path)
		d.mu.Unlock()

		reply, ok := replies[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"code":10004,"message":"Unknown"}`))
			return
		}
		json.NewEncoder(w).Encode(reply)
	})
}

func TestCachedMethodsWithoutState(t *testing.T) {
	d := &restDiscord{}
	s := d.session(t)
	ctx := t.Context()

	if g, err := s.GuildCached(ctx, "g1"); err != nil || g.Name != "Fetched" {
		t.Errorf("GuildCached = %+v, %v", g, err)
	}
	if c, err := s.ChannelCached(ctx, "c1"); err != nil || c.Name != "general" {
		t.Errorf("ChannelCached = %+v, %v", c, err)
	}
	if r, err := s.GuildRoleCached(ctx, "g1", "r1"); err != nil || r.Name != "talker" {
		t.Errorf("GuildRoleCached = %+v, %v", r, err)
	}
	if m, err := s.ChannelMessageCached(ctx, "c1", "m1"); err != nil || m.Content != "old" {
		t.Errorf("ChannelMessageCached = %+v, %v", m, err)
	}
	if _, err := s.GuildMemberCached(ctx, "g1", "nobody"); err == nil {
		t.Error("GuildMemberCached found a member Discord does not have")
	}

	// In a thread, with the overwrites of its channel: r1 may view c1.
	perms, err := s.ChannelMemberPermissionsCached(ctx, "t9", "7")
	if want := discord.PermissionViewChannel | discord.PermissionSendMessages; err != nil || perms != want {
		t.Errorf("ChannelMemberPermissionsCached = %b, %v; want %b", perms, err, want)
	}
	want := []string{"/guilds/g1", "/channels/c1", "/guilds/g1/roles/r1", "/channels/c1/messages/m1", "/guilds/g1/members/nobody",
		"/channels/t9", "/channels/c1", "/guilds/g1", "/guilds/g1/members/7"}
	if got := d.seen(); !slices.Equal(got, want) {
		t.Errorf("requests = %v\nwant %v", got, want)
	}
}

func TestCachedMethodsUseTheState(t *testing.T) {
	d := &restDiscord{}
	s := d.session(t)
	s.State = readyState(t, StateOptions{Messages: 10})
	apply(t, s.State, "GUILD_CREATE", guildCreate())
	apply(t, s.State, "MESSAGE_CREATE", obj{"id": "m1", "channel_id": "c1", "guild_id": "g1", "author": user("7"), "content": "cached"})
	ctx := t.Context()

	if g, _ := s.GuildCached(ctx, "g1"); g.Name != "Guild" {
		t.Errorf("GuildCached = %+v", g)
	}
	if c, _ := s.ChannelCached(ctx, "c1"); c.GuildID != "g1" {
		t.Errorf("ChannelCached = %+v", c)
	}
	if r, _ := s.GuildRoleCached(ctx, "g1", "r1"); r.Permissions != discord.PermissionSendMessages {
		t.Errorf("GuildRoleCached = %+v", r)
	}
	if m, _ := s.ChannelMessageCached(ctx, "c1", "m1"); m.Content != "cached" {
		t.Errorf("ChannelMessageCached = %+v", m)
	}
	if _, err := s.ChannelMemberPermissionsCached(ctx, "c1", "42"); err != nil {
		t.Error(err)
	}
	if got := d.seen(); len(got) != 0 {
		t.Errorf("requests with everything in the state: %v", got)
	}

	// 7 is not kept without Members, so each call asks Discord.
	s.ChannelMemberPermissionsCached(ctx, "c1", "7")
	s.GuildMemberCached(ctx, "g1", "7")
	if got := d.seen(); !slices.Equal(got, []string{"/guilds/g1/members/7", "/guilds/g1/members/7"}) {
		t.Errorf("requests = %v", got)
	}
}

func TestFetchedMembersAreKept(t *testing.T) {
	d := &restDiscord{}
	s := d.session(t)
	s.State = readyState(t, StateOptions{KeepMember: hasRole("r1")})
	apply(t, s.State, "GUILD_CREATE", guildCreate())
	ctx := t.Context()

	for range 2 {
		if m, err := s.GuildMemberCached(ctx, "g1", "7"); err != nil || m.User.ID != "7" {
			t.Fatalf("GuildMemberCached = %+v, %v", m, err)
		}
	}
	if got := d.seen(); len(got) != 1 {
		t.Errorf("requests = %v, want one: the member is kept after it", got)
	}

	// The member returned from Discord is the caller's to change.
	s.State = readyState(t, StateOptions{KeepMember: hasRole("r1")})
	apply(t, s.State, "GUILD_CREATE", guildCreate())
	m, _ := s.GuildMemberCached(ctx, "g1", "7")
	m.Roles[0], m.User.Username = "changed", "changed"
	if kept := s.State.Member("g1", "7"); kept.Roles[0] != "r1" || kept.User.Username != "u7" {
		t.Errorf("changing the returned member changed the state's: %+v", kept)
	}

	// A member from the gateway is not replaced by one fetched later, which
	// may be older.
	apply(t, s.State, "GUILD_MEMBER_UPDATE", obj{"guild_id": "g1", "user": user("7"), "nick": "gateway", "roles": []string{"r1"}})
	s.State.fetchedMember(s.logger(), "g1", &discord.Member{User: &discord.User{ID: "7"}, Nick: "rest", Roles: []string{"r1"}}, 0)
	if m := s.State.Member("g1", "7"); m.Nick != "gateway" {
		t.Errorf("nick = %q", m.Nick)
	}

	// Nor is a member kept if one left while it was fetched: it may be this one.
	removals := s.State.memberRemovals("g1")
	apply(t, s.State, "GUILD_MEMBER_REMOVE", obj{"guild_id": "g1", "user": user("7")})
	s.State.fetchedMember(s.logger(), "g1", &discord.Member{User: &discord.User{ID: "7"}, Roles: []string{"r1"}}, removals)
	if m := s.State.Member("g1", "7"); m != nil {
		t.Errorf("a member who left was kept: %+v", m)
	}
}
