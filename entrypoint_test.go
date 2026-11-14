package minidis

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tbdsux/minidis/discord"
)

// entryPointServer is a fake Discord API whose app already has an entry point
// command, and which records the commands each bulk overwrite sends.
func entryPointServer(t *testing.T) (*Minidis, func() [][]map[string]any) {
	t.Helper()

	var mu sync.Mutex
	var puts [][]map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/applications/app/commands":
			if r.URL.Query().Get("with_localizations") != "true" {
				t.Errorf("entry point lookup must keep localizations: %s", r.URL)
			}
			io.WriteString(w, `[{"id":"ep","type":4,"name":"launch","description":"Launch","handler":2,"name_localizations":{"fr":"lancer"}},{"id":"x","type":1,"name":"old","description":"d"}]`)
		case r.Method == http.MethodPut:
			var cmds []map[string]any
			_ = json.NewDecoder(r.Body).Decode(&cmds)
			mu.Lock()
			puts = append(puts, cmds)
			mu.Unlock()
			io.WriteString(w, `[]`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
		}
	}))
	t.Cleanup(srv.Close)

	bot := New("token")
	bot.Session.BaseURL = srv.URL
	bot.appID = "app"

	return bot, func() [][]map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return puts
	}
}

func TestGlobalOverwritesKeepTheEntryPoint(t *testing.T) {
	bot, puts := entryPointServer(t)
	bot.AddCommand(&Command{Name: "ping", Description: "d"})

	if err := bot.SyncCommands(); err != nil {
		t.Fatal(err)
	}
	if err := bot.ClearCommands(); err != nil {
		t.Fatal(err)
	}
	if err := bot.SyncCommands("g"); err != nil { // guilds have no entry point
		t.Fatal(err)
	}

	got := puts()
	if len(got) != 3 {
		t.Fatalf("puts = %v", got)
	}

	names := func(cmds []map[string]any) string {
		var out []string
		for _, c := range cmds {
			out = append(out, fmt.Sprint(c["name"]))
		}
		return strings.Join(out, ",")
	}
	if names(got[0]) != "ping,launch" || names(got[1]) != "launch" || names(got[2]) != "ping" {
		t.Errorf("synced %q, cleared %q, guild %q", names(got[0]), names(got[1]), names(got[2]))
	}
	if kept := got[1][0]; kept["id"] != "ep" || fmt.Sprint(kept["name_localizations"]) != "map[fr:lancer]" {
		t.Errorf("entry point = %v", kept)
	}
}

func TestSetEntryPoint(t *testing.T) {
	bot, puts := entryPointServer(t)

	ran := false
	bot.SetEntryPoint(&EntryPointCommand{
		Name: "play", Description: "Play", Handler: discord.EntryPointHandlerApp,
		Execute: func(c *Context) error { ran = true; return nil },
	})

	if err := bot.SyncCommands(); err != nil {
		t.Fatal(err)
	}

	got := puts()
	if len(got) != 1 || len(got[0]) != 1 {
		t.Fatalf("puts = %v", got)
	}
	if ep := got[0][0]; ep["name"] != "play" || ep["type"] != float64(4) || ep["handler"] != float64(1) {
		t.Errorf("entry point = %v", ep)
	}

	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"play","type":4}}`))
	if !ran {
		t.Error("entry point handler did not run")
	}
}
