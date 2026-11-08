package minidis

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tbdsux/minidis/discord"
)

type recordedRequest struct {
	Method string
	Path   string
	Body   map[string]any
	Raw    []byte
}

// newTestBot returns a bot whose REST calls are recorded by a fake Discord server.
func newTestBot(t *testing.T) (*Minidis, func() []recordedRequest) {
	t.Helper()

	var mu sync.Mutex
	var requests []recordedRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)

		rec := recordedRequest{Method: r.Method, Path: r.URL.Path, Raw: raw}
		_ = json.Unmarshal(raw, &rec.Body)

		mu.Lock()
		requests = append(requests, rec)
		mu.Unlock()

		if r.Method == http.MethodPut {
			io.WriteString(w, `[]`)
			return
		}
		io.WriteString(w, `{"id":"sent"}`)
	}))
	t.Cleanup(srv.Close)

	bot := New("token")
	bot.Session.BaseURL = srv.URL
	bot.AppID = "app"

	return bot, func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedRequest{}, requests...)
	}
}

func interaction(t *testing.T, payload string) *discord.InteractionCreate {
	t.Helper()

	i := &discord.Interaction{}
	if err := json.Unmarshal([]byte(payload), i); err != nil {
		t.Fatal(err)
	}
	return &discord.InteractionCreate{Interaction: i}
}

const guildFields = `"id":"i1","application_id":"app","token":"tok","guild_id":"g","channel_id":"c","app_permissions":"0","member":{"user":{"id":"u1","username":"name"},"permissions":"0"}`

func TestNewUsesBotAuthorization(t *testing.T) {
	if got := New("abc").Session.Token; got != "Bot abc" {
		t.Errorf("token = %q", got)
	}
}

func TestSlashCommandReply(t *testing.T) {
	bot, requests := newTestBot(t)

	bot.AddCommand(&SlashCommandProps{
		Name: "ping",
		Execute: func(c *SlashContext) error {
			if c.Author.ID != "u1" || c.GuildId != "g" || c.IsDM || c.AppID != "app" {
				t.Errorf("context = %+v", c)
			}
			if got := c.Options["who"].StringValue(); got != "x" {
				t.Errorf("option = %q", got)
			}
			return c.ReplyEphemeral("pong", &discord.MessageEmbed{Title: "t"})
		},
	})

	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"ping","type":1,"options":[{"name":"who","type":3,"value":"x"}]}}`))

	reqs := requests()
	if len(reqs) != 1 || reqs[0].Path != "/interactions/i1/tok/callback" {
		t.Fatalf("requests = %+v", reqs)
	}

	data := reqs[0].Body["data"].(map[string]any)
	if reqs[0].Body["type"] != float64(4) || data["content"] != "pong" || data["flags"] != float64(64) || len(data["embeds"].([]any)) != 1 {
		t.Errorf("body = %s", reqs[0].Raw)
	}
}

func TestDirectMessageContext(t *testing.T) {
	bot, _ := newTestBot(t)

	called := false
	bot.AddCommand(&SlashCommandProps{Name: "dm", Execute: func(c *SlashContext) error {
		called = true
		if !c.IsDM || c.Author.ID != "u2" || c.Member != nil {
			t.Errorf("context = %+v", c)
		}
		return nil
	}})

	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,"id":"i","token":"t","app_permissions":"0","user":{"id":"u2"},"data":{"name":"dm","type":1}}`))

	if !called {
		t.Error("handler was not called")
	}
}

func TestSubcommandRouting(t *testing.T) {
	bot, _ := newTestBot(t)

	var ran []string
	mark := func(name string) func(*SlashContext) error {
		return func(*SlashContext) error { ran = append(ran, name); return nil }
	}

	cmd := bot.AddCommand(&SlashCommandProps{Name: "root", Execute: mark("root")})
	cmd.AddSubcommand(&SlashSubcommandProps{Name: "sub", Execute: mark("sub")})
	cmd.AddSubcommandGroup(&SlashSubcommandGroupProps{Name: "grp"}).AddSubcommand(&SlashSubcommandProps{Name: "nested", Execute: mark("nested")})

	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"root","type":1}}`))
	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"root","type":1,"options":[{"name":"sub","type":1}]}}`))
	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"root","type":1,"options":[{"name":"grp","type":2,"options":[{"name":"nested","type":1}]}]}}`))

	want := []string{"root", "sub", "nested"}
	if len(ran) != 3 || ran[0] != want[0] || ran[1] != want[1] || ran[2] != want[2] {
		t.Errorf("ran = %v", ran)
	}
}

func TestContextMenuCommands(t *testing.T) {
	bot, _ := newTestBot(t)

	gotMessage, gotUser := "", ""
	bot.AddMessageCommand(&MessageCommandProps{Name: "m", Execute: func(c *MessageCommandContext) error {
		gotMessage = c.Message.Content
		return nil
	}})
	bot.AddUserCommand(&UserCommandProps{Name: "u", Execute: func(c *UserCommandContext) error {
		gotUser = c.User.Username + "/" + c.Member.Nick
		return nil
	}})

	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"m","type":3,"target_id":"t","resolved":{"messages":{"t":{"id":"t","content":"body"}}}}}`))
	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"u","type":2,"target_id":"t","resolved":{"users":{"t":{"id":"t","username":"ann"}},"members":{"t":{"nick":"a","permissions":"0"}}}}}`))

	if gotMessage != "body" || gotUser != "ann/a" {
		t.Errorf("message=%q user=%q", gotMessage, gotUser)
	}
}

func TestComponentAndModalHandlers(t *testing.T) {
	bot, _ := newTestBot(t)

	var value, fallback, modal string
	bot.AddComponentHandler(&ComponentInteractionProps{ID: "sel", Execute: func(_ *SlashContext, c *ComponentContext) error {
		value = c.Data.Values[0]
		return nil
	}})
	bot.AddCustomComponentHandler(func(_ *SlashContext, c *ComponentContext) error {
		fallback = c.Data.CustomID
		return nil
	})
	bot.AddModalSubmitHandler(&ModalInteractionProps{ID: "form", Execute: func(_ *SlashContext, c *ModalSubmitContext) error {
		modal = c.Data.Components[0].(*discord.Label).Component.(*discord.TextInput).Value
		return nil
	}})

	bot.handleInteraction(bot.Session, interaction(t, `{"type":3,`+guildFields+`,"data":{"custom_id":"sel","component_type":3,"values":["go"]}}`))
	bot.handleInteraction(bot.Session, interaction(t, `{"type":3,`+guildFields+`,"data":{"custom_id":"other_1","component_type":2}}`))
	bot.handleInteraction(bot.Session, interaction(t, `{"type":5,`+guildFields+`,"data":{"custom_id":"form","components":[{"type":18,"component":{"type":4,"custom_id":"a","value":"typed"}}]}}`))

	if value != "go" || fallback != "other_1" || modal != "typed" {
		t.Errorf("value=%q fallback=%q modal=%q", value, fallback, modal)
	}
}

func TestResponseLifecycle(t *testing.T) {
	bot, requests := newTestBot(t)

	bot.AddCommand(&SlashCommandProps{Name: "c", Execute: func(c *SlashContext) error {
		if err := c.DeferReply(true); err != nil {
			return err
		}
		if err := c.Edit("edited"); err != nil {
			return err
		}
		f, err := c.Followup("more")
		if err != nil {
			return err
		}
		if err := f.Edit("more2"); err != nil {
			return err
		}
		if err := f.Delete(); err != nil {
			return err
		}
		return c.Delete()
	}})

	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"c","type":1}}`))

	want := []string{
		"POST /interactions/i1/tok/callback",
		"PATCH /webhooks/app/tok/messages/@original",
		"POST /webhooks/app/tok",
		"PATCH /webhooks/app/tok/messages/sent",
		"DELETE /webhooks/app/tok/messages/sent",
		"DELETE /webhooks/app/tok/messages/@original",
	}

	reqs := requests()
	if len(reqs) != len(want) {
		t.Fatalf("requests = %+v", reqs)
	}
	for i, w := range want {
		if got := reqs[i].Method + " " + reqs[i].Path; got != w {
			t.Errorf("request %d = %q, want %q", i, got, w)
		}
	}

	if data := reqs[0].Body["data"].(map[string]any); reqs[0].Body["type"] != float64(5) || data["flags"] != float64(64) {
		t.Errorf("defer = %s", reqs[0].Raw)
	}
}

func TestReplyModal(t *testing.T) {
	bot, requests := newTestBot(t)

	bot.AddCommand(&SlashCommandProps{Name: "m", Execute: func(c *SlashContext) error {
		return c.ReplyModal(ReplyModalProps{Title: "T", CustomID: "id", Components: []discord.MessageComponent{
			discord.Label{Label: "L", Component: discord.TextInput{CustomID: "x", Style: discord.TextInputShort}},
		}})
	}})

	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"m","type":1}}`))

	body := requests()[0].Body
	data := body["data"].(map[string]any)
	label := data["components"].([]any)[0].(map[string]any)
	if body["type"] != float64(9) || data["title"] != "T" || data["custom_id"] != "id" || label["type"] != float64(18) ||
		label["component"].(map[string]any)["type"] != float64(4) {
		t.Errorf("body = %s", requests()[0].Raw)
	}
}

func commandByName(t *testing.T, raw []byte) map[string]map[string]any {
	t.Helper()

	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}

	out := map[string]map[string]any{}
	for _, c := range list {
		out[c["name"].(string)] = c
	}
	return out
}

func registerAll(bot *Minidis) {
	exec := func(*SlashContext) error { return nil }

	bot.AddCommand(&SlashCommandProps{Name: "admin", Description: "d", DefaultMemberPermissions: discord.Int64(0), Execute: exec})
	bot.AddCommand(&SlashCommandProps{Name: "open", Description: "d", DefaultMemberPermissions: discord.Int64(1 << 31), DmPermission: true, Execute: exec})
	bot.AddCommand(&SlashCommandProps{Name: "everyone", Description: "d", Execute: exec})

	parent := bot.AddCommand(&SlashCommandProps{Name: "parent", Description: "d"})
	parent.AddSubcommand(&SlashSubcommandProps{Name: "sub", Description: "s"})
	parent.AddSubcommandGroup(&SlashSubcommandGroupProps{Name: "grp", Description: "g"}).
		AddSubcommand(&SlashSubcommandProps{Name: "nested", Description: "n"})

	bot.AddMessageCommand(&MessageCommandProps{Name: "msg"})
	bot.AddUserCommand(&UserCommandProps{Name: "usr"})
}

func TestSyncGlobalCommands(t *testing.T) {
	bot, requests := newTestBot(t)
	registerAll(bot)

	if err := bot.SyncCommands(); err != nil {
		t.Fatal(err)
	}

	reqs := requests()
	if len(reqs) != 1 || reqs[0].Method != "PUT" || reqs[0].Path != "/applications/app/commands" {
		t.Fatalf("requests = %+v", reqs)
	}

	cmds := commandByName(t, reqs[0].Raw)

	if c := cmds["admin"]; c["default_member_permissions"] != "0" || len(c["contexts"].([]any)) != 1 {
		t.Errorf("admin = %v", c)
	}
	if c := cmds["open"]; c["default_member_permissions"] != "2147483648" || len(c["contexts"].([]any)) != 2 {
		t.Errorf("open = %v", c)
	}
	if _, sent := cmds["everyone"]["default_member_permissions"]; sent {
		t.Errorf("unset permissions must be left out, or the command is admin-only: %v", cmds["everyone"])
	}
	if _, deprecated := cmds["open"]["dm_permission"]; deprecated {
		t.Error("dm_permission is deprecated and should not be sent")
	}
	if cmds["msg"]["type"] != float64(3) || cmds["usr"]["type"] != float64(2) {
		t.Errorf("context menus = %v %v", cmds["msg"], cmds["usr"])
	}

	opts := map[string]map[string]any{}
	for _, o := range cmds["parent"]["options"].([]any) {
		opts[o.(map[string]any)["name"].(string)] = o.(map[string]any)
	}
	if opts["sub"]["type"] != float64(1) || opts["grp"]["type"] != float64(2) || len(opts["grp"]["options"].([]any)) != 1 {
		t.Errorf("options = %v", opts)
	}
}

func TestSyncGuildCommandsOmitContexts(t *testing.T) {
	bot, requests := newTestBot(t)
	registerAll(bot)

	if err := bot.SyncCommands("g1", "g2"); err != nil {
		t.Fatal(err)
	}

	reqs := requests()
	if len(reqs) != 2 || reqs[0].Path != "/applications/app/guilds/g1/commands" || reqs[1].Path != "/applications/app/guilds/g2/commands" {
		t.Fatalf("requests = %+v", reqs)
	}

	for name, c := range commandByName(t, reqs[0].Raw) {
		if _, ok := c["contexts"]; ok {
			t.Errorf("%s has contexts, which are global-only", name)
		}
	}
}

func TestSubcommandOptionsAreRegisteredAndPassedToHandlers(t *testing.T) {
	bot, requests := newTestBot(t)

	option := []*discord.ApplicationCommandOption{{Type: discord.ApplicationCommandOptionString, Name: "who", Description: "d"}}

	var got []string
	read := func(c *SlashContext) error {
		got = append(got, c.Options["who"].StringValue())
		return nil
	}

	cmd := bot.AddCommand(&SlashCommandProps{Name: "root", Description: "d"})
	cmd.AddSubcommand(&SlashSubcommandProps{Name: "sub", Description: "s", Options: option, Execute: read})
	cmd.AddSubcommandGroup(&SlashSubcommandGroupProps{Name: "grp", Description: "g"}).
		AddSubcommand(&SlashSubcommandProps{Name: "nested", Description: "n", Options: option, Execute: read})

	if err := bot.SyncCommands("g"); err != nil {
		t.Fatal(err)
	}

	sent := map[string]map[string]any{}
	for _, o := range commandByName(t, requests()[0].Raw)["root"]["options"].([]any) {
		sent[o.(map[string]any)["name"].(string)] = o.(map[string]any)
	}
	nested := sent["grp"]["options"].([]any)[0].(map[string]any)
	if len(sent["sub"]["options"].([]any)) != 1 || len(nested["options"].([]any)) != 1 {
		t.Errorf("registered options = %v", sent)
	}

	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"root","type":1,"options":[{"name":"sub","type":1,"options":[{"name":"who","type":3,"value":"a"}]}]}}`))
	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"root","type":1,"options":[{"name":"grp","type":2,"options":[{"name":"nested","type":1,"options":[{"name":"who","type":3,"value":"b"}]}]}]}}`))

	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("handler options = %v", got)
	}
}

func TestClearCommandsScope(t *testing.T) {
	tests := []struct {
		name   string
		guilds []string
		want   []string
	}{
		{"no guilds clears global", nil, []string{"/applications/app/commands"}},
		{"guilds leave global alone", []string{"g1", "g2"}, []string{"/applications/app/guilds/g1/commands", "/applications/app/guilds/g2/commands"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot, requests := newTestBot(t)

			if err := bot.ClearCommands(tt.guilds...); err != nil {
				t.Fatal(err)
			}

			reqs := requests()
			if len(reqs) != len(tt.want) {
				t.Fatalf("requests = %+v", reqs)
			}
			for i, want := range tt.want {
				if reqs[i].Method != "PUT" || reqs[i].Path != want || string(reqs[i].Raw) != "[]" {
					t.Errorf("request %d = %s %s %s", i, reqs[i].Method, reqs[i].Path, reqs[i].Raw)
				}
			}
		})
	}
}

// TestEndToEnd drives OpenSession and Run against a fake gateway and REST API.
func TestEndToEnd(t *testing.T) {
	type sent struct {
		path string
		body map[string]any
	}
	rest := make(chan sent, 10)
	release := make(chan struct{})

	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	mux.HandleFunc("/gateway/bot", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"url":"ws://%s/gw","session_start_limit":{"remaining":10,"reset_after":0}}`, r.Host)
	})
	mux.HandleFunc("/gw", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer ws.Close()

		write := func(op int, seq int, event string, d string) {
			if err := ws.WriteJSON(map[string]any{"op": op, "s": seq, "t": event, "d": json.RawMessage(d)}); err != nil {
				t.Error(err)
			}
		}

		write(10, 0, "", `{"heartbeat_interval":60000}`)
		if _, _, err := ws.ReadMessage(); err != nil { // Identify
			t.Error(err)
			return
		}
		write(0, 1, "READY", `{"session_id":"s","resume_gateway_url":"ws://`+r.Host+`/gw","user":{"id":"bot"},"application":{"id":"app"}}`)

		<-release

		write(0, 2, "MESSAGE_CREATE", `{"id":"own","channel_id":"c1","content":"loop","author":{"id":"bot"}}`)
		write(0, 3, "MESSAGE_CREATE", `{"id":"m1","channel_id":"c1","guild_id":"g","content":"x","author":{"id":"other"}}`)
		write(0, 4, "INTERACTION_CREATE", `{"type":2,`+guildFields+`,"data":{"name":"ping","type":1}}`)

		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		rest <- sent{r.URL.Path, body}
		io.WriteString(w, `{"id":"sent"}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	bot := New("token")
	bot.Session.BaseURL = srv.URL

	// Repeated registration must not run handlers twice; the check for an extra
	// request at the end of this test would catch it.
	bot.Run()
	bot.Run()

	bot.AddCommand(&SlashCommandProps{Name: "ping", Execute: func(c *SlashContext) error {
		if c.Bot.ID != "bot" {
			t.Errorf("bot user = %+v", c.Bot)
		}
		return c.ReplyString("pong")
	}})
	bot.OnChannelMessageCreate(func(c *ChannelMessageContext) error {
		_, err := c.Reply("hello", true)
		return err
	}, "c1")

	if err := bot.OpenSession(); err != nil {
		t.Fatal(err)
	}
	defer bot.CloseSession()

	if bot.AppID != "app" {
		t.Errorf("AppID = %q", bot.AppID)
	}

	close(release)

	got := map[string]map[string]any{}
	for range 2 {
		select {
		case r := <-rest:
			got[r.path] = r.body
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out, got %v", got)
		}
	}

	reply := got["/channels/c1/messages"]
	if reply == nil || reply["content"] != "hello" {
		t.Fatalf("channel reply = %v", got)
	}
	ref := reply["message_reference"].(map[string]any)
	if ref["message_id"] != "m1" || ref["fail_if_not_exists"] != false {
		t.Errorf("reference = %v (a reply to the bot's own message would be a loop)", ref)
	}

	callback := got["/interactions/i1/tok/callback"]
	if callback == nil || callback["data"].(map[string]any)["content"] != "pong" {
		t.Errorf("callback = %v", got)
	}

	select {
	case r := <-rest:
		t.Errorf("unexpected extra request: %+v", r)
	case <-time.After(100 * time.Millisecond):
	}
}
