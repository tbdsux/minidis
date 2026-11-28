package client

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// pagedUsers serves total users with IDs 1 to total, a page at a time after
// the "after" cursor, and records each request's query.
func pagedUsers(t *testing.T, total int) (*Session, func() []string) {
	t.Helper()
	var (
		mu      sync.Mutex
		queries []string
	)
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.RawQuery)
		mu.Unlock()

		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		after, _ := strconv.Atoi(r.URL.Query().Get("after"))
		users := []*discord.User{}
		for id := after + 1; id <= total && len(users) < limit; id++ {
			users = append(users, &discord.User{ID: strconv.Itoa(id)})
		}
		json.NewEncoder(w).Encode(users)
	})
	return s, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(queries)
	}
}

func TestPaginateFetchesEveryPage(t *testing.T) {
	s, queries := pagedUsers(t, 250)

	var ids []string
	for u, err := range s.MessageReactionsAll(context.Background(), "C", "M", "👍") {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, u.ID)
	}

	if len(ids) != 250 || ids[0] != "1" || ids[249] != "250" {
		t.Errorf("got %d users, first %q, last %q", len(ids), ids[0], ids[len(ids)-1])
	}
	want := []string{"limit=100", "after=100&limit=100", "after=200&limit=100"}
	if got := queries(); !slices.Equal(got, want) {
		t.Errorf("queries = %q, want %q", got, want)
	}
}

func TestPaginateAFullLastPageEndsWithAnEmptyOne(t *testing.T) {
	s, queries := pagedUsers(t, 200)

	n := 0
	for _, err := range s.MessageReactionsAll(context.Background(), "C", "M", "👍") {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 200 || len(queries()) != 3 {
		t.Errorf("got %d users in %d requests, want 200 in 3", n, len(queries()))
	}
}

func TestPaginateStopsFetchingWhenTheLoopBreaks(t *testing.T) {
	s, queries := pagedUsers(t, 250)

	for u, err := range s.MessageReactionsAll(context.Background(), "C", "M", "👍") {
		if err != nil {
			t.Fatal(err)
		}
		if u.ID == "150" {
			break
		}
	}
	if got := len(queries()); got != 2 {
		t.Errorf("made %d requests after breaking on the second page, want 2", got)
	}
}

func TestPaginateYieldsAnErrorAndStops(t *testing.T) {
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"code": 50013, "message": "Missing Permissions"}`))
	})

	n := 0
	for m, err := range s.ChannelMessagesAll(context.Background(), "C") {
		n++
		var re *RESTError
		if !errors.As(err, &re) || re.StatusCode != http.StatusForbidden || m != nil {
			t.Errorf("got %v, %v", m, err)
		}
	}
	if n != 1 {
		t.Errorf("yielded %d times, want 1", n)
	}
}

func TestPaginateCursors(t *testing.T) {
	var (
		mu      sync.Mutex
		queries []string
	)
	// The first page is full, with the last item's ID "L"; the next is empty.
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()

		q := r.URL.Query()
		if q.Has("before") || q.Has("after") {
			w.Write([]byte("[]"))
			return
		}
		limit, _ := strconv.Atoi(q.Get("limit"))
		page := make([]obj, limit)
		for i := range page {
			id := strconv.Itoa(i)
			if i == limit-1 {
				id = "L"
			}
			if r.URL.Path == "/channels/C/messages" {
				page[i] = obj{"id": id}
			} else {
				page[i] = obj{"user": obj{"id": id}}
			}
		}
		json.NewEncoder(w).Encode(page)
	})
	ctx := context.Background()

	if n := drain(t, s.ChannelMessagesAll(ctx, "C")); n != 100 {
		t.Errorf("got %d messages", n)
	}
	if n := drain(t, s.GuildMembersAll(ctx, "G")); n != 1000 {
		t.Errorf("got %d members", n)
	}
	if n := drain(t, s.GuildBansAll(ctx, "G")); n != 1000 {
		t.Errorf("got %d bans", n)
	}

	want := []string{
		"/channels/C/messages?limit=100", "/channels/C/messages?before=L&limit=100",
		"/guilds/G/members?limit=1000", "/guilds/G/members?after=L&limit=1000",
		"/guilds/G/bans?limit=1000", "/guilds/G/bans?after=L&limit=1000",
	}
	if !slices.Equal(queries, want) {
		t.Errorf("queries = %q, want %q", queries, want)
	}
}

func drain[T any](t *testing.T, seq iter.Seq2[T, error]) int {
	t.Helper()
	n := 0
	for _, err := range seq {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	return n
}

func TestPaginateStopsWhenTheCursorDoesNotAdvance(t *testing.T) {
	for _, cursor := range []string{"same", ""} {
		calls := 0
		page := func(string) ([]string, error) { calls++; return []string{"a", "b"}, nil }
		for range paginate(2, page, func(string) string { return cursor }) {
		}
		if want := map[string]int{"same": 2, "": 1}[cursor]; calls != want {
			t.Errorf("cursor %q: %d pages fetched, want %d", cursor, calls, want)
		}
	}
}

func TestPaginateMoreCursors(t *testing.T) {
	ctx := context.Background()
	pinned := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, c := range []struct {
		name string
		size int
		item func(last bool) obj // the last item of a page holds the cursor
		wrap string              // the field a page is wrapped in, if any
		all  func(s *Session) int
		want []string
	}{
		{"pins", 50, func(last bool) obj {
			if last {
				return obj{"pinned_at": pinned}
			}
			return obj{"pinned_at": pinned.Add(time.Hour)}
		}, "items", func(s *Session) int { return drain(t, s.ChannelPinsAll(ctx, "C")) },
			[]string{"/channels/C/messages/pins?limit=50", "/channels/C/messages/pins?before=2026-01-02T03%3A04%3A05Z&limit=50"}},
		{"poll voters", 100, idItem("id"), "users", func(s *Session) int { return drain(t, s.PollAnswerVotersAll(ctx, "C", "M", 1)) },
			[]string{"/channels/C/polls/M/answers/1?limit=100", "/channels/C/polls/M/answers/1?after=L&limit=100"}},
		{"thread members", 100, idItem("user_id"), "", func(s *Session) int { return drain(t, s.ThreadMembersAll(ctx, "T")) },
			[]string{"/channels/T/thread-members?limit=100&with_member=true", "/channels/T/thread-members?after=L&limit=100&with_member=true"}},
		{"event users", 100, func(last bool) obj { return obj{"user": idItem("id")(last)} }, "",
			func(s *Session) int { return drain(t, s.GuildScheduledEventUsersAll(ctx, "G", "E", false)) },
			[]string{"/guilds/G/scheduled-events/E/users?limit=100", "/guilds/G/scheduled-events/E/users?after=L&limit=100"}},
		{"audit log", 100, idItem("id"), "audit_log_entries",
			func(s *Session) int {
				return drain(t, s.GuildAuditLogAll(ctx, "G", discord.AuditLogQuery{UserID: "U", Before: "B", After: "A", Limit: 5}))
			},
			[]string{"/guilds/G/audit-logs?before=B&limit=100&user_id=U", "/guilds/G/audit-logs?before=L&limit=100&user_id=U"}},
		{"guilds", 200, idItem("id"), "", func(s *Session) int { return drain(t, s.CurrentUserGuildsAll(ctx)) },
			[]string{"/users/@me/guilds?limit=200&with_counts=true", "/users/@me/guilds?after=L&limit=200&with_counts=true"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var (
				mu      sync.Mutex
				queries []string
			)
			// The first page is full; the next is empty.
			s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
				first := len(queries) == 1
				mu.Unlock()

				page := []obj{}
				for i := 0; first && i < c.size; i++ {
					page = append(page, c.item(i == c.size-1))
				}
				if c.wrap != "" {
					json.NewEncoder(w).Encode(obj{c.wrap: page})
					return
				}
				json.NewEncoder(w).Encode(page)
			})

			if n := c.all(s); n != c.size {
				t.Errorf("got %d items, want %d", n, c.size)
			}
			if !slices.Equal(queries, c.want) {
				t.Errorf("queries = %q, want %q", queries, c.want)
			}
		})
	}
}

// idItem returns items with an ID in field, "L" for the last of a page.
func idItem(field string) func(last bool) obj {
	return func(last bool) obj {
		if last {
			return obj{field: "L"}
		}
		return obj{field: "1"}
	}
}

func TestPaginateStopsAtAMemberOrBanWithoutAUser(t *testing.T) {
	calls := 0
	s := newRESTSession(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		page := make([]obj, 1000) // a full page, whose last item has no user
		for i := range page[:999] {
			page[i] = obj{"user": obj{"id": strconv.Itoa(i)}}
		}
		page[999] = obj{}
		json.NewEncoder(w).Encode(page)
	})
	ctx := context.Background()

	if n := drain(t, s.GuildMembersAll(ctx, "G")) + drain(t, s.GuildBansAll(ctx, "G")); n != 2000 || calls != 2 {
		t.Errorf("got %d items in %d requests, want 2000 in 2", n, calls)
	}
}
