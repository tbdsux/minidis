package client

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

func TestResourceEndpoints(t *testing.T) {
	tests := []struct {
		want string
		call func(s *Session) error
	}{
		{"GET /channels/C/messages/M", func(s *Session) error { _, err := s.ChannelMessage("C", "M"); return err }},
		{"GET /channels/C/messages", func(s *Session) error { _, err := s.ChannelMessages("C", 0, ""); return err }},
		{"GET /channels/C/messages?before=B&limit=5", func(s *Session) error { _, err := s.ChannelMessages("C", 5, "B"); return err }},
		{"PATCH /channels/C/messages/M", func(s *Session) error { _, err := s.ChannelMessageEdit("C", "M", "x"); return err }},
		{"DELETE /channels/C/messages/M", func(s *Session) error { return s.ChannelMessageDelete("C", "M") }},
		{"POST /channels/C/messages/bulk-delete", func(s *Session) error { return s.ChannelMessagesBulkDelete("C", []string{"1", "2"}) }},
		{"PUT /channels/C/messages/M/reactions/%F0%9F%91%8D/@me", func(s *Session) error { return s.MessageReactionAdd("C", "M", "👍") }},
		{"PUT /channels/C/messages/M/reactions/name:1/@me", func(s *Session) error { return s.MessageReactionAdd("C", "M", "name:1") }},
		{"DELETE /channels/C/messages/M/reactions/x/@me", func(s *Session) error { return s.MessageReactionRemove("C", "M", "x", "") }},
		{"DELETE /channels/C/messages/M/reactions/x/U", func(s *Session) error { return s.MessageReactionRemove("C", "M", "x", "U") }},
		{"DELETE /channels/C/messages/M/reactions", func(s *Session) error { return s.MessageReactionsRemoveAll("C", "M") }},
		{"PUT /channels/C/messages/pins/M", func(s *Session) error { return s.ChannelMessagePin("C", "M") }},
		{"DELETE /channels/C/messages/pins/M", func(s *Session) error { return s.ChannelMessageUnpin("C", "M") }},
		{"GET /channels/C/messages/pins", func(s *Session) error { _, err := s.ChannelPins("C", 0, time.Time{}); return err }},
		{"GET /channels/C/messages/pins?before=2026-01-02T03%3A04%3A05Z&limit=10", func(s *Session) error {
			_, err := s.ChannelPins("C", 10, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
			return err
		}},
		{"POST /guilds/G/roles", func(s *Session) error { _, err := s.GuildRoleCreate("G", &discord.RoleParams{}); return err }},
		{"PATCH /guilds/G/roles/R", func(s *Session) error { _, err := s.GuildRoleEdit("G", "R", &discord.RoleParams{}); return err }},
		{"DELETE /guilds/G/roles/R", func(s *Session) error { return s.GuildRoleDelete("G", "R") }},
		{"GET /channels/C", func(s *Session) error { _, err := s.Channel("C"); return err }},
		{"POST /users/@me/channels", func(s *Session) error { _, err := s.UserChannelCreate("U"); return err }},
		{"GET /guilds/G?with_counts=true", func(s *Session) error { _, err := s.Guild("G"); return err }},
		{"GET /guilds/G/channels", func(s *Session) error { _, err := s.GuildChannels("G"); return err }},
		{"GET /guilds/G/roles", func(s *Session) error { _, err := s.GuildRoles("G"); return err }},
		{"GET /guilds/G/members/U", func(s *Session) error { _, err := s.GuildMember("G", "U"); return err }},
		{"GET /guilds/G/members?after=A&limit=2", func(s *Session) error { _, err := s.GuildMembers("G", 2, "A"); return err }},
		{"DELETE /guilds/G/members/U", func(s *Session) error { return s.GuildMemberRemove("G", "U") }},
		{"PUT /guilds/G/members/U/roles/R", func(s *Session) error { return s.GuildMemberRoleAdd("G", "U", "R") }},
		{"DELETE /guilds/G/members/U/roles/R", func(s *Session) error { return s.GuildMemberRoleRemove("G", "U", "R") }},
		{"GET /guilds/G/bans/U", func(s *Session) error { _, err := s.GuildBan("G", "U"); return err }},
		{"GET /guilds/G/bans?limit=3", func(s *Session) error { _, err := s.GuildBans("G", 3, ""); return err }},
		{"PUT /guilds/G/bans/U", func(s *Session) error { return s.GuildBanCreate("G", "U", 60) }},
		{"DELETE /guilds/G/bans/U", func(s *Session) error { return s.GuildBanDelete("G", "U") }},
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

	pins, err := s.ChannelPins("C", 0, time.Time{})
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

	role, err := s.GuildRoleCreate("G", &discord.RoleParams{
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

	g, err := s.Guild("G")
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "guild" || g.ApproximateMemberCount != 4 || g.Roles[0].Permissions != 8 {
		t.Errorf("guild = %+v", g)
	}
}
