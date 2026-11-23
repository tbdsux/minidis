package client

import (
	"slices"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// fullGuildCreate is guildCreate with stickers, a stage, a scheduled event
// and a soundboard sound.
func fullGuildCreate() obj {
	g := guildCreate()
	g["stickers"] = []obj{{"id": "s1", "name": "wave"}}
	g["stage_instances"] = []obj{{"id": "st1", "channel_id": "c2", "topic": "talk"}}
	g["guild_scheduled_events"] = []obj{{"id": "ev1", "name": "party", "user_count": 5}}
	g["soundboard_sounds"] = []obj{{"sound_id": "snd1", "name": "honk"}}
	return g
}

func TestStateIntents(t *testing.T) {
	always := discord.IntentsGuilds | discord.IntentsGuildExpressions | discord.IntentsGuildScheduledEvents
	tests := []struct {
		opts StateOptions
		want discord.Intent
	}{
		{StateOptions{}, always},
		{StateOptions{KeepMember: func(string, *discord.Member) bool { return true }}, always},
		{StateOptions{ThreadMembers: true}, always | discord.IntentsGuildMembers},
		{StateOptions{Presences: true, VoiceStates: true}, always | discord.IntentsGuildPresences | discord.IntentsGuildVoiceStates},
		{StateOptions{Messages: 1}, always | discord.IntentsGuildMessages | discord.IntentsDirectMessages |
			discord.IntentsGuildMessageReactions | discord.IntentsDirectMessageReactions |
			discord.IntentsGuildMessagePolls | discord.IntentsDirectMessagePolls},
	}
	for _, tt := range tests {
		if got := NewState(tt.opts).Intents(); got != tt.want {
			t.Errorf("Intents(%+v) = %b, want %b", tt.opts, got, tt.want)
		}
	}
}

func TestStateGuildExtras(t *testing.T) {
	st := readyState(t, StateOptions{})
	apply(t, st, "GUILD_CREATE", fullGuildCreate())

	if e := st.Emoji("g1", "e1"); e == nil || e.Name != "wave" {
		t.Errorf("emoji = %+v", e)
	}
	if s := st.Sticker("g1", "s1"); s == nil || s.Name != "wave" {
		t.Errorf("sticker = %+v", s)
	}
	if s := st.StageInstance("g1", "st1"); s == nil || s.Topic != "talk" || s.GuildID != "g1" {
		t.Errorf("stage = %+v", s)
	}
	if ev := st.ScheduledEvent("g1", "ev1"); ev == nil || ev.Name != "party" || ev.GuildID != "g1" || ev.UserCount != 0 {
		t.Errorf("event = %+v", ev)
	}
	if s := st.SoundboardSound("g1", "snd1"); s == nil || s.Name != "honk" || s.GuildID != "g1" {
		t.Errorf("sound = %+v", s)
	}
	if g := st.Guild("g1"); g.StageInstances != nil || g.GuildScheduledEvents != nil || g.SoundboardSounds != nil {
		t.Error("the guild keeps the lists the state keeps apart")
	}

	apply(t, st, "STAGE_INSTANCE_UPDATE", obj{"id": "st1", "guild_id": "g1", "topic": "new topic"})
	apply(t, st, "STAGE_INSTANCE_CREATE", obj{"id": "st2", "guild_id": "g1"})
	apply(t, st, "STAGE_INSTANCE_DELETE", obj{"id": "st2", "guild_id": "g1"})
	if s := st.StageInstance("g1", "st1"); s.Topic != "new topic" || len(st.StageInstances("g1")) != 1 {
		t.Errorf("stages = %+v", st.StageInstances("g1"))
	}

	apply(t, st, "GUILD_SCHEDULED_EVENT_UPDATE", obj{"id": "ev1", "guild_id": "g1", "name": "renamed", "user_count": 9})
	apply(t, st, "GUILD_SCHEDULED_EVENT_CREATE", obj{"id": "ev2", "guild_id": "g1"})
	apply(t, st, "GUILD_SCHEDULED_EVENT_DELETE", obj{"id": "ev2", "guild_id": "g1"})
	if ev := st.ScheduledEvent("g1", "ev1"); ev.Name != "renamed" || ev.UserCount != 0 || len(st.ScheduledEvents("g1")) != 1 {
		t.Errorf("events = %+v (the user count is not kept)", st.ScheduledEvents("g1"))
	}

	apply(t, st, "GUILD_SOUNDBOARD_SOUND_CREATE", obj{"sound_id": "snd2", "guild_id": "g1", "name": "beep"})
	apply(t, st, "GUILD_SOUNDBOARD_SOUNDS_UPDATE", obj{"guild_id": "g1", "soundboard_sounds": []obj{{"sound_id": "snd1", "name": "HONK"}}})
	apply(t, st, "GUILD_SOUNDBOARD_SOUND_DELETE", obj{"sound_id": "snd2", "guild_id": "g1"})
	if s := st.SoundboardSound("g1", "snd1"); s.Name != "HONK" || len(st.SoundboardSounds("g1")) != 1 {
		t.Errorf("sounds = %+v", st.SoundboardSounds("g1"))
	}
	apply(t, st, "SOUNDBOARD_SOUNDS", obj{"guild_id": "g1", "soundboard_sounds": []obj{{"sound_id": "snd3", "name": "all"}}})
	if got := st.SoundboardSounds("g1"); len(got) != 1 || got[0].SoundID != "snd3" {
		t.Errorf("sounds after the full list = %+v", got)
	}
}

func TestStateThreadMembers(t *testing.T) {
	st := readyState(t, StateOptions{ThreadMembers: true, Members: true})
	g := guildCreate()
	// The bot's membership, inside a thread, comes without its IDs.
	g["threads"].([]obj)[0]["member"] = obj{"join_timestamp": "2024-01-01T00:00:00Z"}
	apply(t, st, "GUILD_CREATE", g)

	if m := st.ThreadMember("t1", "42"); m == nil || m.ID != "t1" {
		t.Fatalf("the bot's thread membership = %+v", m)
	}

	apply(t, st, "THREAD_MEMBERS_UPDATE", obj{"id": "t1", "guild_id": "g1", "member_count": 3,
		"added_members":      []obj{{"user_id": "7", "member": member("7")}, {"user_id": "8"}},
		"removed_member_ids": []string{"42"}})
	if got := userIDs(st.ThreadMembers("t1")); !slices.Equal(got, []string{"7", "8"}) {
		t.Errorf("thread members = %v", got)
	}
	if m := st.ThreadMember("t1", "7"); m.ID != "t1" {
		t.Errorf("added member = %+v", m)
	}
	if st.Member("g1", "7") == nil {
		t.Error("the guild member that came with a thread member is not kept")
	}

	apply(t, st, "THREAD_DELETE", obj{"id": "t1", "guild_id": "g1", "parent_id": "c1"})
	if st.ThreadMembers("t1") != nil {
		t.Error("the members of a deleted thread are kept")
	}

	none := readyState(t, StateOptions{})
	apply(t, none, "GUILD_CREATE", g)
	if none.ThreadMember("t1", "42") != nil {
		t.Error("thread members kept without StateOptions.ThreadMembers")
	}
}

func userIDs(members []*discord.ThreadMember) []string {
	var ids []string
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	slices.Sort(ids)
	return ids
}

func TestStateReactions(t *testing.T) {
	st := readyState(t, StateOptions{Messages: 5})
	apply(t, st, "GUILD_CREATE", guildCreate())
	apply(t, st, "MESSAGE_CREATE", obj{"id": "m1", "channel_id": "c1", "guild_id": "g1", "author": user("7"),
		"reactions": []obj{{"count": 1, "count_details": obj{"normal": 1}, "emoji": obj{"name": "👍"}}}})
	before := st.Message("c1", "m1")

	react := func(event, userID string, emoji obj, burst bool) {
		apply(t, st, event, obj{"user_id": userID, "channel_id": "c1", "message_id": "m1", "guild_id": "g1", "emoji": emoji, "burst": burst})
	}
	thumbs, custom := obj{"name": "👍"}, obj{"id": "e1", "name": "wave"}

	react("MESSAGE_REACTION_ADD", "42", thumbs, false)
	react("MESSAGE_REACTION_ADD", "7", custom, true)
	react("MESSAGE_REACTION_ADD", "8", obj{"id": "e1", "name": "renamed"}, false) // the same emoji, by ID

	m := st.Message("c1", "m1")
	if len(m.Reactions) != 2 {
		t.Fatalf("reactions = %+v", m.Reactions)
	}
	if r := m.Reactions[0]; r.Count != 2 || r.CountDetails.Normal != 2 || !r.Me {
		t.Errorf("👍 = %+v", r)
	}
	if r := m.Reactions[1]; r.Count != 2 || r.CountDetails.Burst != 1 || r.CountDetails.Normal != 1 || r.Me || r.MeBurst {
		t.Errorf("custom = %+v", r)
	}
	if len(before.Reactions) != 1 || before.Reactions[0].Count != 1 {
		t.Error("a message that was returned changed")
	}

	react("MESSAGE_REACTION_REMOVE", "42", thumbs, false)
	if r := st.Message("c1", "m1").Reactions[0]; r.Count != 1 || r.Me {
		t.Errorf("👍 after the bot took its reaction back = %+v", r)
	}
	react("MESSAGE_REACTION_REMOVE", "7", thumbs, false)
	if got := st.Message("c1", "m1").Reactions; len(got) != 1 || !sameEmoji(got[0].Emoji, &discord.Emoji{ID: "e1"}) {
		t.Errorf("a reaction without anyone is kept: %+v", got)
	}

	apply(t, st, "MESSAGE_REACTION_REMOVE_EMOJI", obj{"channel_id": "c1", "message_id": "m1", "emoji": custom})
	if got := st.Message("c1", "m1").Reactions; len(got) != 0 {
		t.Errorf("after removing the emoji = %+v", got)
	}
	react("MESSAGE_REACTION_ADD", "7", thumbs, false)
	apply(t, st, "MESSAGE_REACTION_REMOVE_ALL", obj{"channel_id": "c1", "message_id": "m1"})
	if got := st.Message("c1", "m1").Reactions; got != nil {
		t.Errorf("after removing all = %+v", got)
	}
}

func TestStatePollVotes(t *testing.T) {
	st := readyState(t, StateOptions{Messages: 5})
	apply(t, st, "GUILD_CREATE", guildCreate())
	apply(t, st, "MESSAGE_CREATE", obj{"id": "m1", "channel_id": "c1", "guild_id": "g1", "author": user("7"),
		"poll": obj{"question": obj{"text": "?"}, "answers": []obj{{"answer_id": 1}, {"answer_id": 2}}}})

	vote := func(event, userID string, answer int) {
		apply(t, st, event, obj{"user_id": userID, "channel_id": "c1", "message_id": "m1", "guild_id": "g1", "answer_id": answer})
	}
	vote("MESSAGE_POLL_VOTE_ADD", "42", 1)
	vote("MESSAGE_POLL_VOTE_ADD", "7", 1)
	vote("MESSAGE_POLL_VOTE_ADD", "7", 2)
	vote("MESSAGE_POLL_VOTE_REMOVE", "7", 2)

	results := st.Message("c1", "m1").Poll.Results
	if results == nil || len(results.AnswerCounts) != 2 {
		t.Fatalf("results = %+v", results)
	}
	if c := results.AnswerCounts[0]; c.ID != 1 || c.Count != 2 || !c.MeVoted {
		t.Errorf("answer 1 = %+v", c)
	}
	if c := results.AnswerCounts[1]; c.ID != 2 || c.Count != 0 || c.MeVoted {
		t.Errorf("answer 2 = %+v", c)
	}
}

func TestStateMembersFromOtherEvents(t *testing.T) {
	st := readyState(t, StateOptions{Members: true})
	apply(t, st, "GUILD_CREATE", guildCreate())

	apply(t, st, "TYPING_START", obj{"channel_id": "c1", "guild_id": "g1", "user_id": "20", "member": member("20")})
	apply(t, st, "MESSAGE_REACTION_ADD", obj{"user_id": "21", "channel_id": "c1", "message_id": "x", "guild_id": "g1",
		"emoji": obj{"name": "👍"}, "member": member("21")})
	interactionMember := member("22")
	interactionMember["permissions"] = "8"
	apply(t, st, "INTERACTION_CREATE", obj{"id": "i1", "type": 2, "guild_id": "g1", "channel_id": "c1",
		"member": interactionMember, "data": obj{"id": "cmd", "name": "ping", "type": 1}})

	for _, id := range []string{"20", "21", "22"} {
		if st.Member("g1", id) == nil {
			t.Errorf("member %s is not kept", id)
		}
	}
	if m := st.Member("g1", "22"); m != nil && m.Permissions != 0 {
		t.Errorf("the interaction's permissions are kept with the member: %b", m.Permissions)
	}
}

func TestStateDMsAndThreadCounts(t *testing.T) {
	st := readyState(t, StateOptions{})
	apply(t, st, "GUILD_CREATE", guildCreate())

	apply(t, st, "MESSAGE_CREATE", obj{"id": "m1", "channel_id": "dm1", "author": user("7")})
	c := st.Channel("dm1")
	if c == nil || c.Type != discord.ChannelTypeDM || c.LastMessageID != "m1" || len(c.Recipients) != 1 || c.Recipients[0].ID != "7" {
		t.Errorf("DM channel = %+v", c)
	}
	apply(t, st, "MESSAGE_CREATE", obj{"id": "m2", "channel_id": "dm2", "author": user("42")})
	if st.Channel("dm2") != nil {
		t.Error("a DM channel without its recipient is kept")
	}

	apply(t, st, "MESSAGE_CREATE", obj{"id": "m3", "channel_id": "t1", "guild_id": "g1", "author": user("7")})
	if c := st.Channel("t1"); c.MessageCount != 1 || c.TotalMessageSent != 1 || c.LastMessageID != "m3" {
		t.Errorf("thread = %+v", c)
	}
}

func TestStateUserUpdate(t *testing.T) {
	st := readyState(t, StateOptions{})
	apply(t, st, "GUILD_CREATE", guildCreate())
	before := st.Member("g1", "42")

	apply(t, st, "USER_UPDATE", obj{"id": "42", "username": "renamed"})
	if m := st.Member("g1", "42"); m.User.Username != "renamed" || len(m.Roles) != 1 {
		t.Errorf("the bot's member = %+v", m)
	}
	if before.User.Username != "u42" {
		t.Error("a member that was returned changed")
	}
}

func TestStateMessageEditsKeepWhatTheyLeaveOut(t *testing.T) {
	st := readyState(t, StateOptions{Messages: 5})
	apply(t, st, "GUILD_CREATE", guildCreate())
	apply(t, st, "MESSAGE_CREATE", obj{"id": "m1", "channel_id": "c1", "guild_id": "g1", "author": user("7"),
		"member": obj{"roles": []string{}, "joined_at": "2024-01-01T00:00:00Z"},
		"poll":   obj{"question": obj{"text": "?"}, "answers": []obj{{"answer_id": 1}}}})
	apply(t, st, "MESSAGE_REACTION_ADD", obj{"user_id": "8", "channel_id": "c1", "message_id": "m1", "emoji": obj{"name": "👍"}})
	apply(t, st, "MESSAGE_POLL_VOTE_ADD", obj{"user_id": "8", "channel_id": "c1", "message_id": "m1", "answer_id": 1})

	apply(t, st, "MESSAGE_UPDATE", obj{"id": "m1", "channel_id": "c1", "guild_id": "g1", "author": user("7"), "content": "edited",
		"poll": obj{"question": obj{"text": "?"}, "answers": []obj{{"answer_id": 1}}}})
	m := st.Message("c1", "m1")
	if m.Content != "edited" || len(m.Reactions) != 1 || m.Member == nil {
		t.Errorf("edited message = %+v", m)
	}
	if m.Poll.Results == nil || m.Poll.Results.AnswerCounts[0].Count != 1 {
		t.Errorf("poll results after an edit = %+v", m.Poll.Results)
	}
}

func TestStateThreadCountsFollowDeletes(t *testing.T) {
	st := readyState(t, StateOptions{})
	apply(t, st, "GUILD_CREATE", guildCreate())
	send := func(id string) {
		apply(t, st, "MESSAGE_CREATE", obj{"id": id, "channel_id": "t1", "guild_id": "g1", "author": user("7")})
	}

	send("t1") // the first message of a forum post has the thread's ID
	send("m1")
	send("m2")
	send("m3")
	apply(t, st, "MESSAGE_DELETE", obj{"id": "m1", "channel_id": "t1", "guild_id": "g1"})
	apply(t, st, "MESSAGE_DELETE_BULK", obj{"ids": []string{"m2", "t1"}, "channel_id": "t1", "guild_id": "g1"})

	if c := st.Channel("t1"); c.MessageCount != 1 || c.TotalMessageSent != 3 {
		t.Errorf("MessageCount = %d, TotalMessageSent = %d; want 1 and 3", c.MessageCount, c.TotalMessageSent)
	}
}

func TestStateKeepsTheLatestDMChannels(t *testing.T) {
	st := readyState(t, StateOptions{Messages: 5})
	st.maxDMs = 2
	dm := func(channel, id string) {
		apply(t, st, "MESSAGE_CREATE", obj{"id": id, "channel_id": channel, "author": user("7")})
	}

	dm("dm1", "a")
	dm("dm2", "b")
	dm("dm1", "c") // dm1 is now the latest
	dm("dm3", "d") // so dm2 goes, with its messages
	if st.Channel("dm2") != nil || st.Messages("dm2") != nil {
		t.Error("the DM channel least recently written in is kept")
	}
	if st.Channel("dm1") == nil || len(st.Messages("dm1")) != 2 || st.Channel("dm3") == nil {
		t.Error("a recent DM channel was forgotten")
	}

	// The bot's own DMs have no channel here, but their messages count too.
	apply(t, st, "MESSAGE_CREATE", obj{"id": "e", "channel_id": "dm4", "author": user("42")})
	if st.Messages("dm4") == nil || st.Channel("dm1") != nil {
		t.Error("messages in a DM channel without a channel are not limited")
	}
}

func TestKeepMemberMayUseTheState(t *testing.T) {
	var st *State
	st = readyState(t, StateOptions{KeepMember: func(guildID string, m *discord.Member) bool {
		return st.Guild(guildID) != nil && st.Role(guildID, "r1") != nil
	}})

	done := make(chan struct{})
	go func() {
		defer close(done)
		apply(t, st, "GUILD_CREATE", guildCreate())
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a KeepMember that reads the state blocks the update")
	}
	if st.Member("g1", "7") == nil {
		t.Error("the member KeepMember kept is missing")
	}
}
