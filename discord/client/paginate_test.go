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
