package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

func messageEvent(id string) map[string]any {
	return map[string]any{"id": id, "channel_id": "c1", "content": id, "author": map[string]string{"id": "7"}}
}

func TestHandlerPanicDoesNotStopTheSession(t *testing.T) {
	survived := make(chan struct{})

	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(1)
		c.send(opDispatch, messageEvent("boom"), 2, "MESSAGE_CREATE")
		c.send(opDispatch, messageEvent("fine"), 3, "MESSAGE_CREATE")
		c.waitClosed()
	})

	s.OnMessageCreate(func(_ *Session, m *discord.MessageCreate) {
		if m.Content == "boom" {
			panic("handler bug")
		}
		close(survived)
	})

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	waitFor(t, survived, "the event after the panic")
}

func TestShutdownWaitsForHandlers(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})

	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(1)
		c.send(opDispatch, messageEvent("slow"), 2, "MESSAGE_CREATE")
		c.waitClosed()
		close(release) // the disconnect happens before the drain
	})

	var finished atomic.Bool
	s.OnMessageCreate(func(*Session, *discord.MessageCreate) {
		close(started)
		<-release
		finished.Store(true)
	})

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, started, "the handler")

	if err := s.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !finished.Load() {
		t.Error("Shutdown returned before the handler finished")
	}
}

func TestShutdownStopsWaitingWhenContextEnds(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)

	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(1)
		c.send(opDispatch, messageEvent("stuck"), 2, "MESSAGE_CREATE")
		c.waitClosed()
	})

	s.OnMessageCreate(func(*Session, *discord.MessageCreate) {
		close(started)
		<-release
	})

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, started, "the handler")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := s.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Shutdown = %v", err)
	}
}

func decodePresence(t *testing.T, p gatewayPayload) discord.PresenceUpdate {
	t.Helper()

	var got discord.PresenceUpdate
	if err := json.Unmarshal(p.D, &got); err != nil {
		t.Errorf("presence %s: %v", p.D, err)
	}
	return got
}

func TestPresenceIsSentAndSurvivesReconnect(t *testing.T) {
	updated, resumed := make(chan struct{}), make(chan struct{})

	s, _ := newFakeGateway(t,
		func(c *fakeConn) {
			c.hello(60000)
			c.read(opIdentify)
			c.ready(1)

			if got := decodePresence(t, c.read(opPresenceUpdate)); got.Status != discord.StatusDND || got.Activities[0].Name != "chess" {
				t.Errorf("initial presence = %+v", got)
			}

			p := c.read(opPresenceUpdate)
			if got := decodePresence(t, p); got.Status != discord.StatusOnline || got.Activities == nil {
				t.Errorf("updated presence = %s (activities must be an array)", p.D)
			}
			close(updated)

			c.closeWith(4000)
		},
		func(c *fakeConn) {
			c.hello(60000)
			c.read(opResume)
			c.send(opDispatch, map[string]any{}, 2, "RESUMED")

			if got := decodePresence(t, c.read(opPresenceUpdate)); got.Status != discord.StatusOnline {
				t.Errorf("presence after resume = %+v", got)
			}
			close(resumed)
			c.waitClosed()
		},
	)

	err := s.UpdatePresence(&discord.PresenceUpdate{
		Status:     discord.StatusDND,
		Activities: []*discord.Activity{{Name: "chess", Type: discord.ActivityPlaying}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.UpdatePresence(&discord.PresenceUpdate{}); err != nil {
		t.Fatal(err)
	}

	waitFor(t, updated, "the presence update")
	waitFor(t, resumed, "the presence after resuming")
}

func TestTypedEvents(t *testing.T) {
	seen := make(chan string, 6)

	s, _ := newFakeGateway(t, func(c *fakeConn) {
		c.hello(60000)
		c.read(opIdentify)
		c.ready(1)
		c.send(opDispatch, map[string]any{"id": "m1", "channel_id": "c1", "content": "new"}, 2, "MESSAGE_UPDATE")
		c.send(opDispatch, map[string]any{"id": "m2", "channel_id": "c1"}, 3, "MESSAGE_DELETE")
		c.send(opDispatch, map[string]any{"user_id": "u1", "channel_id": "c1", "message_id": "m3", "emoji": map[string]any{"id": nil, "name": "ok"}}, 4, "MESSAGE_REACTION_ADD")
		c.send(opDispatch, map[string]any{"id": "g1", "name": "guild", "member_count": 3, "joined_at": nil}, 5, "GUILD_CREATE")
		c.send(opDispatch, map[string]any{"guild_id": "g1", "nick": "n", "user": map[string]string{"id": "u1"}}, 6, "GUILD_MEMBER_ADD")
		c.send(opDispatch, map[string]any{"guild_id": "g1", "user": map[string]string{"id": "u2"}}, 7, "GUILD_MEMBER_REMOVE")
		c.waitClosed()
	})

	s.OnMessageUpdate(func(_ *Session, e *discord.MessageUpdate) { seen <- "update " + e.Content })
	s.OnMessageDelete(func(_ *Session, e *discord.MessageDelete) { seen <- "delete " + e.ID })
	s.OnMessageReactionAdd(func(_ *Session, e *discord.MessageReactionAdd) { seen <- "react " + e.Emoji.Name })
	s.OnGuildCreate(func(_ *Session, e *discord.GuildCreate) { seen <- "guild " + e.Name })
	s.OnGuildMemberAdd(func(_ *Session, e *discord.GuildMemberAdd) {
		seen <- "join " + e.GuildID + " " + e.User.ID + " " + e.Nick
	})
	s.OnGuildMemberRemove(func(_ *Session, e *discord.GuildMemberRemove) { seen <- "leave " + e.User.ID })

	if err := s.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	done := make(chan struct{})
	got := map[string]bool{}
	go func() {
		defer close(done)
		for range 6 {
			got[<-seen] = true
		}
	}()
	waitFor(t, done, "typed events")

	for _, want := range []string{"update new", "delete m2", "react ok", "guild guild", "join g1 u1 n", "leave u2"} {
		if !got[want] {
			t.Errorf("missing %q in %v", want, got)
		}
	}
}

func TestResourceEndpoints(t *testing.T) {
	tests := []struct {
		want string
		call func(s *Session) error
	}{
		{"GET /channels/C/messages/M", func(s *Session) error { _, err := s.ChannelMessage(context.Background(), "C", "M"); return err }},
		{"GET /channels/C/messages", func(s *Session) error { _, err := s.ChannelMessages(context.Background(), "C", 0, ""); return err }},
		{"GET /channels/C/messages?before=B&limit=5", func(s *Session) error { _, err := s.ChannelMessages(context.Background(), "C", 5, "B"); return err }},
		{"PATCH /channels/C/messages/M", func(s *Session) error {
			_, err := s.ChannelMessageEdit(context.Background(), "C", "M", "x")
			return err
		}},
		{"DELETE /channels/C/messages/M", func(s *Session) error { return s.ChannelMessageDelete(context.Background(), "C", "M") }},
		{"POST /channels/C/messages/bulk-delete", func(s *Session) error {
			return s.ChannelMessagesBulkDelete(context.Background(), "C", []string{"1", "2"})
		}},
		{"PUT /channels/C/messages/M/reactions/%F0%9F%91%8D/@me", func(s *Session) error { return s.MessageReactionAdd(context.Background(), "C", "M", "👍") }},
		{"PUT /channels/C/messages/M/reactions/name:1/@me", func(s *Session) error { return s.MessageReactionAdd(context.Background(), "C", "M", "name:1") }},
		{"DELETE /channels/C/messages/M/reactions/x/@me", func(s *Session) error { return s.MessageReactionRemove(context.Background(), "C", "M", "x", "") }},
		{"DELETE /channels/C/messages/M/reactions/x/U", func(s *Session) error { return s.MessageReactionRemove(context.Background(), "C", "M", "x", "U") }},
		{"DELETE /channels/C/messages/M/reactions", func(s *Session) error { return s.MessageReactionsRemoveAll(context.Background(), "C", "M") }},
		{"PUT /channels/C/messages/pins/M", func(s *Session) error { return s.ChannelMessagePin(context.Background(), "C", "M") }},
		{"DELETE /channels/C/messages/pins/M", func(s *Session) error { return s.ChannelMessageUnpin(context.Background(), "C", "M") }},
		{"GET /channels/C/messages/pins", func(s *Session) error { _, err := s.ChannelPins(context.Background(), "C", 0, time.Time{}); return err }},
		{"GET /channels/C/messages/pins?before=2026-01-02T03%3A04%3A05Z&limit=10", func(s *Session) error {
			_, err := s.ChannelPins(context.Background(), "C", 10, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
			return err
		}},
		{"POST /guilds/G/roles", func(s *Session) error {
			_, err := s.GuildRoleCreate(context.Background(), "G", &discord.RoleParams{})
			return err
		}},
		{"PATCH /guilds/G/roles/R", func(s *Session) error {
			_, err := s.GuildRoleEdit(context.Background(), "G", "R", &discord.RoleParams{})
			return err
		}},
		{"DELETE /guilds/G/roles/R", func(s *Session) error { return s.GuildRoleDelete(context.Background(), "G", "R") }},
		{"GET /channels/C", func(s *Session) error { _, err := s.Channel(context.Background(), "C"); return err }},
		{"POST /users/@me/channels", func(s *Session) error { _, err := s.UserChannelCreate(context.Background(), "U"); return err }},
		{"GET /guilds/G?with_counts=true", func(s *Session) error { _, err := s.Guild(context.Background(), "G"); return err }},
		{"GET /guilds/G/channels", func(s *Session) error { _, err := s.GuildChannels(context.Background(), "G"); return err }},
		{"GET /guilds/G/roles", func(s *Session) error { _, err := s.GuildRoles(context.Background(), "G"); return err }},
		{"GET /guilds/G/members/U", func(s *Session) error { _, err := s.GuildMember(context.Background(), "G", "U"); return err }},
		{"GET /guilds/G/members?after=A&limit=2", func(s *Session) error { _, err := s.GuildMembers(context.Background(), "G", 2, "A"); return err }},
		{"DELETE /guilds/G/members/U", func(s *Session) error { return s.GuildMemberRemove(context.Background(), "G", "U") }},
		{"PUT /guilds/G/members/U/roles/R", func(s *Session) error { return s.GuildMemberRoleAdd(context.Background(), "G", "U", "R") }},
		{"DELETE /guilds/G/members/U/roles/R", func(s *Session) error { return s.GuildMemberRoleRemove(context.Background(), "G", "U", "R") }},
		{"GET /guilds/G/bans/U", func(s *Session) error { _, err := s.GuildBan(context.Background(), "G", "U"); return err }},
		{"GET /guilds/G/bans?limit=3", func(s *Session) error { _, err := s.GuildBans(context.Background(), "G", 3, ""); return err }},
		{"PUT /guilds/G/bans/U", func(s *Session) error { return s.GuildBanCreate(context.Background(), "G", "U", 60) }},
		{"DELETE /guilds/G/bans/U", func(s *Session) error { return s.GuildBanDelete(context.Background(), "G", "U") }},
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

func TestChannelPinsDecodesWrapper(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"items":[{"pinned_at":"2026-01-02T03:04:05Z","message":{"id":"M"}}],"has_more":true}`))
	})

	pins, err := s.ChannelPins(context.Background(), "C", 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !pins.HasMore || len(pins.Items) != 1 || pins.Items[0].Message.ID != "M" || pins.Items[0].PinnedAt.Year() != 2026 {
		t.Errorf("pins = %+v", pins)
	}
}

func TestRoleParamsEncoding(t *testing.T) {
	var body string
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.Write([]byte(`{"id":"R","name":"mods","permissions":"8","colors":{"primary_color":255,"secondary_color":null,"tertiary_color":null}}`))
	})

	role, err := s.GuildRoleCreate(context.Background(), "G", &discord.RoleParams{
		Name:        "mods",
		Permissions: discord.Int64(8),
		Colors:      &discord.RoleColors{Primary: 255},
		Hoist:       discord.Bool(false),
	})
	if err != nil {
		t.Fatal(err)
	}

	want := `{"name":"mods","permissions":"8","colors":{"primary_color":255,"secondary_color":null,"tertiary_color":null},"hoist":false}`
	if body != want {
		t.Errorf("body = %s", body)
	}
	if role.ID != "R" || role.Permissions != 8 || role.Colors.Primary != 255 {
		t.Errorf("role = %+v", role)
	}
}

func TestFetchDecodesResources(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"G","name":"guild","approximate_member_count":4,"roles":[{"id":"R","permissions":"8"}]}`))
	})

	g, err := s.Guild(context.Background(), "G")
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "guild" || g.ApproximateMemberCount != 4 || g.Roles[0].Permissions != 8 {
		t.Errorf("guild = %+v", g)
	}
}
