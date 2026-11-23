package client

import (
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

type obj = map[string]any

func apply(t *testing.T, st *State, event string, payload any) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	st.apply(event, data)
}

func user(id string) obj { return obj{"id": id, "username": "u" + id} }

func member(id string, roles ...string) obj {
	if roles == nil {
		roles = []string{}
	}
	return obj{"user": user(id), "roles": roles, "joined_at": "2024-01-01T00:00:00Z"}
}

// guildCreate is guild g1 owned by user 1, where @everyone may view
// channels and role r1 may send messages. c1 is a text channel that denies
// @everyone viewing it but allows r1 to; t1 is a thread in it.
func guildCreate() obj {
	return obj{
		"id": "g1", "name": "Guild", "owner_id": "1", "member_count": 3,
		"roles": []obj{
			{"id": "g1", "name": "@everyone", "permissions": "1024"},
			{"id": "r1", "name": "talker", "permissions": "2048"},
		},
		"emojis": []obj{{"id": "e1", "name": "wave"}},
		"channels": []obj{
			{"id": "c1", "name": "general", "type": 0, "permission_overwrites": []obj{
				{"id": "g1", "type": 0, "allow": "0", "deny": "1024"},
				{"id": "r1", "type": 0, "allow": "1024", "deny": "0"},
			}},
			{"id": "c2", "name": "voice", "type": 2},
		},
		"threads":      []obj{{"id": "t1", "guild_id": "g1", "parent_id": "c1", "type": 11, "thread_metadata": obj{"archived": false}}},
		"members":      []obj{member("42", "r1"), member("7")},
		"voice_states": []obj{{"user_id": "7", "channel_id": "c2", "session_id": "s"}},
		"presences": []obj{
			{"user": obj{"id": "7"}, "status": "online"},
			{"user": obj{"id": "42"}, "status": "offline"},
		},
	}
}

func readyState(t *testing.T, opts StateOptions) *State {
	t.Helper()
	st := NewState(opts)
	apply(t, st, "READY", obj{"user": user("42"), "guilds": []obj{{"id": "g1", "unavailable": true}, {"id": "g2", "unavailable": true}}})
	return st
}

func TestStateGuildCreate(t *testing.T) {
	st := readyState(t, StateOptions{})

	if g := st.Guild("g1"); g == nil || !g.Unavailable {
		t.Fatalf("before GUILD_CREATE: %+v", g)
	}
	apply(t, st, "GUILD_CREATE", guildCreate())

	g := st.Guild("g1")
	if g == nil || g.Name != "Guild" || g.Unavailable || len(g.Roles) != 2 || len(g.Emojis) != 1 {
		t.Fatalf("guild = %+v", g)
	}
	if g.Channels != nil || g.Members != nil || g.Threads != nil || g.VoiceStates != nil || g.Presences != nil {
		t.Errorf("the guild keeps the lists the state keeps apart: %+v", g)
	}
	if n := len(st.Guilds()); n != 2 {
		t.Errorf("%d guilds, want g1 and the unavailable g2", n)
	}

	if c := st.Channel("c1"); c == nil || c.GuildID != "g1" || c.Name != "general" {
		t.Errorf("channel = %+v", c)
	}
	if got := ids(st.GuildChannels("g1")); !slices.Equal(got, []string{"c1", "c2"}) {
		t.Errorf("guild channels = %v", got)
	}
	if got := ids(st.GuildThreads("g1")); !slices.Equal(got, []string{"t1"}) {
		t.Errorf("guild threads = %v", got)
	}
	if r := st.Role("g1", "r1"); r == nil || r.Name != "talker" {
		t.Errorf("role = %+v", r)
	}

	// Without Members, only the bot's own member is kept, and nothing of
	// voice and presences.
	if st.Member("g1", "42") == nil || st.Member("g1", "7") != nil {
		t.Errorf("members = %v", st.Members("g1"))
	}
	if st.VoiceState("g1", "7") != nil || st.Presence("g1", "7") != nil {
		t.Error("voice states or presences kept without being asked for")
	}
}

func ids(channels []*discord.Channel) []string {
	var ids []string
	for _, c := range channels {
		ids = append(ids, c.ID)
	}
	slices.Sort(ids)
	return ids
}

func TestStatePermissions(t *testing.T) {
	st := readyState(t, StateOptions{Members: true})
	apply(t, st, "GUILD_CREATE", guildCreate())

	tests := []struct {
		channel, user string
		want          int64
		ok            bool
	}{
		{"c1", "42", discord.PermissionViewChannel | discord.PermissionSendMessages, true},
		{"t1", "42", discord.PermissionViewChannel | discord.PermissionSendMessages, true}, // as in c1
		{"c1", "7", 0, true},                             // cannot view it
		{"c2", "7", discord.PermissionViewChannel, true}, // no overwrites
		{"c1", "nobody", 0, false},
		{"missing", "42", 0, false},
	}
	for _, tt := range tests {
		got, ok := st.Permissions(tt.channel, tt.user)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Permissions(%s, %s) = %b, %v; want %b, %v", tt.channel, tt.user, got, ok, tt.want, tt.ok)
		}
	}
}

func TestStateGuildUpdatesKeepEarlierValues(t *testing.T) {
	st := readyState(t, StateOptions{Members: true})
	apply(t, st, "GUILD_CREATE", guildCreate())
	before := st.Guild("g1")

	apply(t, st, "GUILD_UPDATE", obj{"id": "g1", "name": "Renamed", "owner_id": "1"})
	g := st.Guild("g1")
	if g.Name != "Renamed" || g.MemberCount != 3 || len(g.Roles) != 2 || len(g.Emojis) != 1 {
		t.Errorf("after GUILD_UPDATE: %+v", g)
	}
	if before.Name != "Guild" {
		t.Error("a guild that was returned changed")
	}

	apply(t, st, "GUILD_ROLE_CREATE", obj{"guild_id": "g1", "role": obj{"id": "r2", "name": "new"}})
	apply(t, st, "GUILD_ROLE_UPDATE", obj{"guild_id": "g1", "role": obj{"id": "r1", "name": "renamed", "permissions": "2048"}})
	apply(t, st, "GUILD_ROLE_DELETE", obj{"guild_id": "g1", "role_id": "g1"})
	var names []string
	for _, r := range st.Guild("g1").Roles {
		names = append(names, r.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"new", "renamed"}) {
		t.Errorf("roles = %v", names)
	}
	if len(g.Roles) != 2 || g.Roles[0].Name != "@everyone" {
		t.Error("the roles of a guild that was returned changed")
	}

	apply(t, st, "GUILD_EMOJIS_UPDATE", obj{"guild_id": "g1", "emojis": []obj{}})
	if n := len(st.Guild("g1").Emojis); n != 0 {
		t.Errorf("%d emojis after they were all removed", n)
	}

	apply(t, st, "GUILD_DELETE", obj{"id": "g1", "unavailable": true})
	if g := st.Guild("g1"); g == nil || !g.Unavailable || g.Name != "" {
		t.Errorf("in an outage: %+v", g)
	}
	if st.Channel("c1") != nil || st.Member("g1", "42") != nil {
		t.Error("the channels and members of an unavailable guild are kept")
	}

	apply(t, st, "GUILD_DELETE", obj{"id": "g1"})
	if st.Guild("g1") != nil {
		t.Error("a guild the bot left is kept")
	}
}

func TestStateMembers(t *testing.T) {
	st := readyState(t, StateOptions{Members: true})
	apply(t, st, "GUILD_CREATE", guildCreate())

	apply(t, st, "GUILD_MEMBER_ADD", obj{"guild_id": "g1", "user": user("8"), "roles": []string{}, "deaf": true})
	if m := st.Member("g1", "8"); m == nil || st.Guild("g1").MemberCount != 4 {
		t.Fatalf("after GUILD_MEMBER_ADD: %+v, count %d", m, st.Guild("g1").MemberCount)
	}

	// GUILD_MEMBER_UPDATE does not say whether the member is deafened.
	apply(t, st, "GUILD_MEMBER_UPDATE", obj{"guild_id": "g1", "user": user("8"), "nick": "eight", "roles": []string{"r1"}})
	if m := st.Member("g1", "8"); m.Nick != "eight" || !m.Deaf || len(m.Roles) != 1 {
		t.Errorf("after GUILD_MEMBER_UPDATE: %+v", m)
	}

	apply(t, st, "GUILD_MEMBER_REMOVE", obj{"guild_id": "g1", "user": user("8")})
	if st.Member("g1", "8") != nil || st.Guild("g1").MemberCount != 3 {
		t.Error("a member who left is kept")
	}

	apply(t, st, "GUILD_MEMBERS_CHUNK", obj{"guild_id": "g1", "members": []obj{member("9"), member("10")}})
	if len(st.Members("g1")) != 4 {
		t.Errorf("members = %d after a chunk of two", len(st.Members("g1")))
	}

	// The member of a message, which comes without its user.
	apply(t, st, "MESSAGE_CREATE", obj{"id": "m1", "channel_id": "c1", "guild_id": "g1", "author": user("11"),
		"member": obj{"roles": []string{"r1"}, "joined_at": "2024-01-01T00:00:00Z"}})
	if m := st.Member("g1", "11"); m == nil || m.User.ID != "11" || len(m.Roles) != 1 {
		t.Errorf("member from a message = %+v", m)
	}
	if c := st.Channel("c1"); c.LastMessageID != "m1" {
		t.Errorf("last message = %q", c.LastMessageID)
	}
}

func TestStateChannelsAndThreads(t *testing.T) {
	st := readyState(t, StateOptions{Messages: 10})
	apply(t, st, "GUILD_CREATE", guildCreate())

	apply(t, st, "CHANNEL_CREATE", obj{"id": "c3", "guild_id": "g1", "name": "new", "type": 0})
	apply(t, st, "CHANNEL_UPDATE", obj{"id": "c3", "guild_id": "g1", "name": "renamed", "type": 0})
	if c := st.Channel("c3"); c == nil || c.Name != "renamed" {
		t.Errorf("channel = %+v", c)
	}
	apply(t, st, "CHANNEL_CREATE", obj{"id": "dm", "type": 1, "recipients": []obj{user("7")}})
	if st.Channel("dm") == nil {
		t.Error("a DM channel is not kept")
	}

	pinned := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	apply(t, st, "CHANNEL_PINS_UPDATE", obj{"guild_id": "g1", "channel_id": "c3", "last_pin_timestamp": pinned})
	if c := st.Channel("c3"); c.LastPinTimestamp == nil || !c.LastPinTimestamp.Equal(pinned) {
		t.Errorf("last pin = %v", c.LastPinTimestamp)
	}

	// THREAD_UPDATE does not say whether the bot is a member.
	apply(t, st, "THREAD_MEMBER_UPDATE", obj{"id": "t1", "user_id": "42", "guild_id": "g1"})
	apply(t, st, "THREAD_UPDATE", obj{"id": "t1", "guild_id": "g1", "parent_id": "c1", "type": 11, "name": "renamed"})
	if c := st.Channel("t1"); c.Name != "renamed" || c.Member == nil {
		t.Errorf("thread = %+v", c)
	}
	apply(t, st, "THREAD_MEMBERS_UPDATE", obj{"id": "t1", "guild_id": "g1", "member_count": 5, "removed_member_ids": []string{"42"}})
	if c := st.Channel("t1"); c.MemberCount != 5 || c.Member != nil {
		t.Errorf("thread after the bot left = %+v", c)
	}

	apply(t, st, "THREAD_UPDATE", obj{"id": "t1", "guild_id": "g1", "parent_id": "c1", "type": 11, "thread_metadata": obj{"archived": true}})
	if st.Channel("t1") != nil {
		t.Error("an archived thread is kept")
	}

	apply(t, st, "THREAD_LIST_SYNC", obj{"guild_id": "g1", "channel_ids": []string{"c1"},
		"threads": []obj{{"id": "t2", "parent_id": "c1", "type": 11}, {"id": "t3", "parent_id": "c1", "type": 12}},
		"members": []obj{{"id": "t3", "user_id": "42"}}})
	if got := ids(st.GuildThreads("g1")); !slices.Equal(got, []string{"t2", "t3"}) {
		t.Errorf("threads after sync = %v", got)
	}
	if c := st.Channel("t3"); c.GuildID != "g1" || c.Member == nil {
		t.Errorf("synced thread = %+v", c)
	}

	apply(t, st, "MESSAGE_CREATE", obj{"id": "m1", "channel_id": "c1", "guild_id": "g1", "author": user("7")})
	apply(t, st, "THREAD_DELETE", obj{"id": "t2", "guild_id": "g1", "parent_id": "c1"})
	if st.Channel("t2") != nil {
		t.Error("a deleted thread is kept")
	}

	// Deleting a channel takes its threads and messages with it.
	apply(t, st, "CHANNEL_DELETE", obj{"id": "c1", "guild_id": "g1", "type": 0})
	if st.Channel("c1") != nil || st.Channel("t3") != nil || st.Messages("c1") != nil {
		t.Error("a deleted channel, its threads or its messages are kept")
	}
	if got := ids(st.GuildChannels("g1")); !slices.Equal(got, []string{"c2", "c3"}) {
		t.Errorf("guild channels = %v", got)
	}
}

func TestStateMessages(t *testing.T) {
	st := readyState(t, StateOptions{Messages: 3})
	apply(t, st, "GUILD_CREATE", guildCreate())

	for _, id := range []string{"m1", "m2", "m3", "m4"} {
		apply(t, st, "MESSAGE_CREATE", obj{"id": id, "channel_id": "c1", "guild_id": "g1", "author": user("7"), "content": id})
	}
	messageIDs := func() []string {
		var ids []string
		for _, m := range st.Messages("c1") {
			ids = append(ids, m.ID)
		}
		return ids
	}
	if got := messageIDs(); !slices.Equal(got, []string{"m2", "m3", "m4"}) {
		t.Fatalf("messages = %v", got)
	}
	before := st.Messages("c1")

	apply(t, st, "MESSAGE_UPDATE", obj{"id": "m3", "channel_id": "c1", "guild_id": "g1", "author": user("7"), "content": "edited"})
	apply(t, st, "MESSAGE_UPDATE", obj{"id": "m4", "channel_id": "c1", "embeds": []obj{}}) // partial: ignored
	if m := st.Message("c1", "m3"); m == nil || m.Content != "edited" {
		t.Errorf("updated message = %+v", m)
	}
	if m := st.Message("c1", "m4"); m == nil || m.Content != "m4" {
		t.Errorf("partly updated message = %+v", m)
	}
	if before[1].Content != "m3" {
		t.Error("a message that was returned changed")
	}

	apply(t, st, "MESSAGE_DELETE", obj{"id": "m2", "channel_id": "c1"})
	apply(t, st, "MESSAGE_DELETE_BULK", obj{"ids": []string{"m4", "nope"}, "channel_id": "c1"})
	if got := messageIDs(); !slices.Equal(got, []string{"m3"}) {
		t.Errorf("messages after deletes = %v", got)
	}
	if len(before) != 3 {
		t.Error("a list of messages that was returned changed")
	}

	apply(t, st, "MESSAGE_CREATE", obj{"id": "d1", "channel_id": "dm", "author": user("7")})
	if st.Message("dm", "d1") == nil {
		t.Error("a DM message is not kept")
	}
}

func TestStateVoiceAndPresences(t *testing.T) {
	st := readyState(t, StateOptions{VoiceStates: true, Presences: true})
	apply(t, st, "GUILD_CREATE", guildCreate())

	if v := st.VoiceState("g1", "7"); v == nil || v.ChannelID != "c2" || v.GuildID != "g1" {
		t.Errorf("voice state = %+v", v)
	}
	if p := st.Presence("g1", "7"); p == nil || p.Status != discord.StatusOnline || p.GuildID != "g1" {
		t.Errorf("presence = %+v", p)
	}
	if st.Presence("g1", "42") != nil {
		t.Error("an offline presence is kept")
	}

	apply(t, st, "VOICE_STATE_UPDATE", obj{"guild_id": "g1", "user_id": "8", "channel_id": "c2", "member": member("8")})
	apply(t, st, "VOICE_STATE_UPDATE", obj{"guild_id": "g1", "user_id": "7", "channel_id": nil})
	if st.VoiceState("g1", "7") != nil || st.VoiceState("g1", "8") == nil || len(st.VoiceStates("g1")) != 1 {
		t.Errorf("voice states = %+v", st.VoiceStates("g1"))
	}
	if st.Member("g1", "8") != nil {
		t.Error("a member is kept without StateOptions.Members")
	}

	apply(t, st, "PRESENCE_UPDATE", obj{"guild_id": "g1", "user": obj{"id": "7"}, "status": "offline"})
	apply(t, st, "PRESENCE_UPDATE", obj{"guild_id": "g1", "user": obj{"id": "8"}, "status": "dnd"})
	if st.Presence("g1", "7") != nil || st.Presence("g1", "8") == nil {
		t.Error("presences not updated")
	}
}

func TestStateReadyStartsOverForItsShard(t *testing.T) {
	st := NewState(StateOptions{})
	// Guild 1<<22 goes to shard 1 of 2, and guild 2<<22 to shard 0.
	g1, g2 := "4194304", "8388608"
	apply(t, st, "READY", obj{"user": user("42"), "shard": []int{0, 2}, "guilds": []obj{{"id": g2}}})
	apply(t, st, "READY", obj{"user": user("42"), "shard": []int{1, 2}, "guilds": []obj{{"id": g1}}})
	apply(t, st, "GUILD_CREATE", obj{"id": g1, "name": "one", "channels": []obj{{"id": "c1"}}})
	apply(t, st, "GUILD_CREATE", obj{"id": g2, "name": "two", "channels": []obj{{"id": "c2"}}})
	apply(t, st, "CHANNEL_CREATE", obj{"id": "dm", "type": 1})

	// Shard 0 starts a new session.
	apply(t, st, "READY", obj{"user": user("42"), "shard": []int{0, 2}, "guilds": []obj{{"id": g2, "unavailable": true}}})
	if g := st.Guild(g1); g == nil || g.Name != "one" || st.Channel("c1") == nil {
		t.Error("the guild of another shard was forgotten")
	}
	if g := st.Guild(g2); g == nil || !g.Unavailable || st.Channel("c2") != nil {
		t.Errorf("the guild of the shard that started over = %+v", g)
	}
	if st.Channel("dm") != nil {
		t.Error("DMs, which shard 0 receives, were kept")
	}
}

func TestStateConcurrentReads(t *testing.T) {
	st := readyState(t, StateOptions{Members: true, Messages: 5})
	apply(t, st, "GUILD_CREATE", guildCreate())

	var wg sync.WaitGroup
	done := make(chan struct{})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				if g := st.Guild("g1"); g != nil {
					_ = len(g.Roles)
				}
				for _, m := range st.Messages("c1") {
					_ = m.Content
				}
				st.Permissions("c1", "42")
			}
		})
	}
	for i := range 200 {
		apply(t, st, "GUILD_ROLE_UPDATE", obj{"guild_id": "g1", "role": obj{"id": "r1", "permissions": "2048"}})
		apply(t, st, "MESSAGE_CREATE", obj{"id": string(rune('a' + i%26)), "channel_id": "c1", "guild_id": "g1", "author": user("7")})
	}
	close(done)
	wg.Wait()
}

func TestSessionKeepsItsState(t *testing.T) {
	seen := make(chan struct{})

	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)

		var identify struct {
			Intents discord.Intent `json:"intents"`
		}
		if err := json.Unmarshal(c.read(opIdentify).D, &identify); err != nil {
			t.Error(err)
		}
		if want := discord.IntentsGuildModeration | discord.IntentsGuilds | discord.IntentsGuildExpressions | discord.IntentsGuildScheduledEvents | discord.IntentsGuildMembers; identify.Intents != want {
			t.Errorf("identified with intents %b, want %b", identify.Intents, want)
		}

		c.ready(1)
		c.send(opDispatch, guildCreate(), 2, "GUILD_CREATE")
		c.send(opDispatch, obj{"id": "42", "username": "renamed"}, 3, "USER_UPDATE")
		c.send(opDispatch, obj{"guild_id": "g1", "user": user("8"), "roles": []string{}}, 4, "GUILD_MEMBER_ADD")
		c.waitClosed()
	})
	s.Intents = discord.IntentsGuildModeration
	s.State = NewState(StateOptions{Members: true})

	// The state has the event applied before its handlers run.
	s.OnGuildMemberAdd(func(s *Session, e *discord.GuildMemberAdd) {
		if s.State.Member("g1", "8") == nil || s.State.Guild("g1").Name != "Guild" {
			t.Error("a handler ran before the state was updated")
		}
		if u := s.User(); u.Username != "renamed" {
			t.Errorf("Session.User after USER_UPDATE = %+v", u)
		}
		close(seen)
	})

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	waitFor(t, seen, "GUILD_MEMBER_ADD")
}
