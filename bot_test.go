package minidis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

var (
	_ context.Context = (*EventContext)(nil)
	_ context.Context = (*MessageContext)(nil)
)

// fakeDiscord is the REST API and a gateway that sends the events it is given.
type fakeDiscord struct {
	t      *testing.T
	events chan [2]any // name and payload
	close  chan int    // a close code for the gateway to end with

	mu       sync.Mutex
	requests []string       // "METHOD /path"
	bodies   map[string]any // the last body of each request
	intents  discord.Intent // of the Identify
	identify chan struct{}
}

func (d *fakeDiscord) send(event string, payload any) { d.events <- [2]any{event, payload} }

func (d *fakeDiscord) seen() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.requests)
}

func (d *fakeDiscord) body(request string) map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	body, _ := d.bodies[request].(map[string]any)
	return body
}

var testBots atomic.Int64

// newTestBot returns a bot wired to a fake Discord.
func newTestBot(t *testing.T) (*Bot, *fakeDiscord) {
	t.Helper()

	d := &fakeDiscord{
		t: t, events: make(chan [2]any, 16), close: make(chan int, 1),
		bodies: map[string]any{}, identify: make(chan struct{}),
	}
	upgrader := websocket.Upgrader{}

	mux := http.NewServeMux()
	mux.HandleFunc("/gateway/bot", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"url":"ws://%s/gw","shards":1,"session_start_limit":{"total":1000,"remaining":1000,"reset_after":0,"max_concurrency":1}}`, r.Host)
	})
	mux.HandleFunc("/gw", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer ws.Close()

		ws.WriteJSON(map[string]any{"op": 10, "d": map[string]any{"heartbeat_interval": 60000}})

		var identify struct {
			D struct {
				Intents discord.Intent `json:"intents"`
			} `json:"d"`
		}
		if err := ws.ReadJSON(&identify); err != nil {
			return
		}
		d.mu.Lock()
		d.intents = identify.D.Intents
		d.mu.Unlock()
		close(d.identify)

		select {
		case code := <-d.close:
			ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, "test"), time.Now().Add(time.Second))
			return
		default:
		}

		ws.WriteJSON(map[string]any{"op": 0, "s": 1, "t": "READY", "d": map[string]any{
			"v": 10, "session_id": "s", "resume_gateway_url": "ws://" + r.Host + "/gw",
			"user": map[string]any{"id": "42", "username": "bot", "bot": true}, "guilds": []any{},
			"application": map[string]any{"id": "99"},
		}})

		gone := make(chan struct{})
		go func() {
			defer close(gone)
			for {
				if _, _, err := ws.ReadMessage(); err != nil {
					return
				}
			}
		}()

		for seq := 2; ; seq++ {
			select {
			case e := <-d.events:
				ws.WriteJSON(map[string]any{"op": 0, "s": seq, "t": e[0], "d": e[1]})
			case <-gone:
				return
			}
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		request := r.Method + " " + r.URL.Path

		var body any
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)

		d.mu.Lock()
		d.requests = append(d.requests, request)
		d.bodies[request] = body
		d.mu.Unlock()

		switch {
		case strings.HasSuffix(r.URL.Path, "/commands"):
			io.WriteString(w, `[]`)
		case r.URL.Path == "/applications/@me":
			io.WriteString(w, `{"id":"99"}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/messages"):
			io.WriteString(w, `{"id":"sent"}`)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	// Identifies are spaced out for each token, so every test bot has its own.
	bot := New(fmt.Sprintf("token %d", testBots.Add(1)))
	bot.Session.BaseURL = server.URL
	t.Cleanup(func() { bot.Close() })

	return bot, d
}

func recv[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

func message(content string) map[string]any {
	return map[string]any{
		"id": "m1", "channel_id": "c1", "guild_id": "g1", "content": content,
		"author": map[string]any{"id": "u1", "username": "ann"},
		"member": map[string]any{"nick": "Ann", "roles": []string{"r1"}},
	}
}

func TestIntentsComeFromHandlers(t *testing.T) {
	none := func(*Bot) {}

	tests := []struct {
		name  string
		setup func(b *Bot)
		want  discord.Intent
	}{
		{"interactions need none", func(b *Bot) {
			b.AddCommand(&Command{Name: "ping", Description: "d"})
			b.HandleComponent("x", func(*ComponentContext) error { return nil })
		}, discord.IntentsNone},
		{"nothing at all", none, discord.IntentsNone},
		{"a member event", func(b *Bot) {
			On(b, func(*EventContext, *discord.GuildMemberAdd) error { return nil })
		}, discord.IntentsGuildMembers},
		{"an event of guilds and DMs", func(b *Bot) {
			On(b, func(*EventContext, *discord.MessageReactionAdd) error { return nil })
		}, discord.IntentsGuildMessageReactions | discord.IntentsDirectMessageReactions},
		{"an event that is always sent", func(b *Bot) {
			On(b, func(*EventContext, *discord.Ready) error { return nil })
		}, discord.IntentsNone},
		{"raw message events do not ask for content", func(b *Bot) {
			On(b, func(*EventContext, *discord.MessageCreate) error { return nil })
		}, messageIntents},
		{"message handlers read content", func(b *Bot) {
			b.OnMessage(func(*MessageContext) error { return nil })
		}, messageIntents | discord.IntentsMessageContent},
		{"text commands by mention need no content", func(b *Bot) {
			b.AddTextCommand(&TextCommand{Name: "ping"})
		}, messageIntents},
		{"a prefix does", func(b *Bot) {
			b.SetPrefix("!")
			b.AddTextCommand(&TextCommand{Name: "ping"})
		}, messageIntents | discord.IntentsMessageContent},
		{"channel messages", func(b *Bot) {
			b.HandleChannelMessage("c", nil)
		}, messageIntents | discord.IntentsMessageContent},
		{"added on top", func(b *Bot) {
			b.AddIntents(discord.IntentsGuilds)
			On(b, func(*EventContext, *discord.GuildBanAdd) error { return nil })
		}, discord.IntentsGuilds | discord.IntentsGuildModeration},
		{"with those set on the session", func(b *Bot) {
			b.Session.Intents = discord.IntentsGuildVoiceStates
			On(b, func(*EventContext, *discord.GuildBanAdd) error { return nil })
		}, discord.IntentsGuildVoiceStates | discord.IntentsGuildModeration},
		{"with those of the state", func(b *Bot) {
			b.Session.State = client.NewState(client.StateOptions{Members: true})
		}, discord.IntentsGuilds | discord.IntentsGuildExpressions | discord.IntentsGuildScheduledEvents | discord.IntentsGuildMembers},
		{"set explicitly", func(b *Bot) {
			b.SetIntents(discord.IntentsGuilds)
			b.OnMessage(func(*MessageContext) error { return nil })
			b.AddIntents(discord.IntentsGuildInvites)
		}, discord.IntentsGuilds | discord.IntentsGuildInvites},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New("token")
			tt.setup(b)
			if got := b.Intents(); got != tt.want {
				t.Errorf("intents = %b, want %b", got, tt.want)
			}
		})
	}
}

func TestOnPanicsForATypeThatIsNotAnEvent(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a handler of discord.User was accepted")
		}
	}()
	On(New("token"), func(*EventContext, *discord.User) error { return nil })
}

func TestRunSyncsConnectsAndHandlesEvents(t *testing.T) {
	bot, d := newTestBot(t)
	bot.SetGuilds("", " g1 , g2", "g3 g4")
	bot.AddCommand(&Command{Name: "ping", Description: "Pong.", Execute: func(c *Context) error { return c.Reply("pong") }})

	joined := make(chan string, 1)
	var held *EventContext
	On(bot, func(c *EventContext, e *discord.GuildMemberAdd) error {
		held = c
		if c.Err() != nil || c.Bot != bot || c.Event != "GUILD_MEMBER_ADD" {
			t.Errorf("context = %+v, err %v", c, c.Err())
		}
		joined <- e.GuildID + " " + e.User.ID
		return nil
	})

	failed := make(chan string, 2)
	bot.HandleEventError(func(c *EventContext, err error) { failed <- c.Event + ": " + err.Error() })
	On(bot, func(*EventContext, *discord.GuildBanAdd) error { return errors.New("boom") })
	On(bot, func(*EventContext, *discord.GuildBanRemove) error { panic("oops") })

	ctx, stop := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- bot.Run(ctx) }()

	recv(t, d.identify, "the Identify")
	if want := discord.IntentsGuildMembers | discord.IntentsGuildModeration; d.intents != want {
		t.Errorf("identified with intents %b, want %b", d.intents, want)
	}
	for _, guild := range []string{"g1", "g2", "g3", "g4"} {
		if got := d.seen(); !slices.Contains(got, "PUT /applications/99/guilds/"+guild+"/commands") {
			t.Errorf("commands were not registered in guild %s: %v", guild, got)
		}
	}

	d.send("GUILD_MEMBER_ADD", map[string]any{"guild_id": "g1", "user": map[string]any{"id": "u1"}})
	if got := recv(t, joined, "the member event"); got != "g1 u1" {
		t.Errorf("joined = %q", got)
	}

	d.send("GUILD_BAN_ADD", map[string]any{"guild_id": "g1", "user": map[string]any{"id": "u1"}})
	if got := recv(t, failed, "the error"); got != "GUILD_BAN_ADD: boom" {
		t.Errorf("error = %q", got)
	}
	d.send("GUILD_BAN_REMOVE", map[string]any{"guild_id": "g1", "user": map[string]any{"id": "u1"}})
	if got := recv(t, failed, "the panic"); !strings.Contains(got, "GUILD_BAN_REMOVE") || !strings.Contains(got, "oops") {
		t.Errorf("panic = %q", got)
	}

	stop()
	if err := recv(t, done, "Run to return"); err != nil {
		t.Errorf("Run = %v", err)
	}
	if held.Err() == nil {
		t.Error("the handler's context outlived the handler")
	}
}

func TestRunLeavesCommandsAloneWithoutAny(t *testing.T) {
	for name, setup := range map[string]func(b *Bot){
		"no commands": func(*Bot) {},
		"sync disabled": func(b *Bot) {
			b.AddCommand(&Command{Name: "ping", Description: "d"})
			b.DisableSync()
		},
	} {
		t.Run(name, func(t *testing.T) {
			bot, d := newTestBot(t)
			setup(bot)

			ctx, stop := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- bot.Run(ctx) }()

			recv(t, d.identify, "the Identify")
			stop()
			recv(t, done, "Run to return")

			for _, request := range d.seen() {
				if strings.Contains(request, "/commands") {
					t.Errorf("commands were touched: %s", request)
				}
			}
		})
	}
}

func TestRunExplainsRefusedIntents(t *testing.T) {
	bot, d := newTestBot(t)
	bot.OnMessage(func(*MessageContext) error { return nil })
	On(bot, func(*EventContext, *discord.GuildMemberAdd) error { return nil })
	d.close <- disallowedIntents

	err := bot.Run(t.Context())

	var closed *client.GatewayCloseError
	if !errors.As(err, &closed) || closed.Code != disallowedIntents {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"Server Members Intent", "Message Content Intent", "developer portal"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "Presence Intent") {
		t.Errorf("error names an intent the bot did not ask for: %v", err)
	}
}

func TestMessagesAndTextCommands(t *testing.T) {
	bot, d := newTestBot(t)
	bot.SetPrefix("!")

	heard := make(chan string, 4)
	bot.OnMessage(func(c *MessageContext) error {
		if c.Member == nil || c.Member.User != c.Author || c.Member.DisplayName() != "Ann" || c.IsDM {
			t.Errorf("context = %+v", c)
		}
		heard <- c.Content
		return nil
	})

	ran := make(chan *MessageContext, 4)
	bot.AddTextCommand(
		&TextCommand{Name: "Echo", Aliases: []string{"say"}, Execute: func(c *MessageContext) error {
			if _, err := c.Reply(c.ArgString); err != nil {
				return err
			}
			if err := c.React("👍"); err != nil {
				return err
			}
			ran <- c
			return nil
		}},
		&TextCommand{Name: "fail", Execute: func(c *MessageContext) error { return errors.New("nope") }},
	)

	failed := make(chan string, 1)
	bot.HandleMessageError(func(c *MessageContext, err error) { failed <- c.Command + ": " + err.Error() })

	if err := bot.Open(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Bots, this one included, and webhooks are not answered.
	fromBot := message("!echo loop")
	fromBot["author"] = map[string]any{"id": "42", "bot": true}
	d.send("MESSAGE_CREATE", fromBot)
	fromHook := message("!echo hook")
	fromHook["webhook_id"] = "w1"
	d.send("MESSAGE_CREATE", fromHook)

	// Neither are Discord's notices, which name a user as their author.
	joined := message("!echo joined")
	joined["type"] = discord.MessageTypeUserJoin
	d.send("MESSAGE_CREATE", joined)

	d.send("MESSAGE_CREATE", message("!ECHO  hello   there "))
	if got := recv(t, heard, "the message"); got != "!ECHO  hello   there " {
		t.Errorf("heard %q", got)
	}
	c := recv(t, ran, "the command")
	if c.Command != "echo" || !slices.Equal(c.Args, []string{"hello", "there"}) || c.ArgString != "hello   there" {
		t.Errorf("command = %q %q %q", c.Command, c.Args, c.ArgString)
	}

	reply := d.body("POST /channels/c1/messages")
	if reply["content"] != "hello   there" || reply["message_reference"].(map[string]any)["message_id"] != "m1" {
		t.Errorf("reply = %v", reply)
	}
	if got := d.seen(); !slices.Contains(got, "PUT /channels/c1/messages/m1/reactions/👍/@me") {
		t.Errorf("no reaction in %v", got)
	}

	// By alias and by mention, which works whatever the prefix.
	d.send("MESSAGE_CREATE", message("<@42> say hi"))
	recv(t, heard, "the mention")
	if c := recv(t, ran, "the command by mention"); c.Command != "say" || c.ArgString != "hi" {
		t.Errorf("command = %q %q", c.Command, c.ArgString)
	}

	d.send("MESSAGE_CREATE", message("!fail"))
	if got := recv(t, failed, "the error"); got != "fail: nope" {
		t.Errorf("error = %q", got)
	}

	// Not commands: unknown names, and text that only looks like one. A reply
	// is a message like any other.
	d.send("MESSAGE_CREATE", message("!unknown"))
	d.send("MESSAGE_CREATE", message("say hi"))
	d.send("MESSAGE_CREATE", message("!"))
	answer := message("a reply")
	answer["type"] = discord.MessageTypeReply
	d.send("MESSAGE_CREATE", answer)
	for range 5 {
		recv(t, heard, "the other messages")
	}

	select {
	case c := <-ran:
		t.Errorf("a command ran for %q", c.Content)
	case got := <-heard:
		t.Errorf("a handler heard a bot, a webhook or Discord: %q", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSplitCommand(t *testing.T) {
	bot := &discord.User{ID: "42"}

	tests := []struct {
		content, prefix string
		name, rest      string
		ok              bool
	}{
		{"!ping", "!", "ping", "", true},
		{"! ping  a b", "!", "ping", "a b", true},
		{"!Ping\nline two", "!", "ping", "line two", true},
		{"??ping", "??", "ping", "", true},
		{"<@42> ping", "", "ping", "", true},
		{"<@!42>ping x", "!", "ping", "x", true},
		{"<@43> ping", "", "", "", false},
		{"ping", "!", "", "", false},
		{"!ping", "", "", "", false},
		{"!", "!", "", "", false},
		{"<@42>", "", "", "", false},
	}
	for _, tt := range tests {
		name, rest, ok := splitCommand(tt.content, tt.prefix, bot)
		if name != tt.name || rest != tt.rest || ok != tt.ok {
			t.Errorf("splitCommand(%q, %q) = %q, %q, %v", tt.content, tt.prefix, name, rest, ok)
		}
	}

	if _, _, ok := splitCommand("<@42> ping", "", nil); ok {
		t.Error("a mention matched before the bot knew its own ID")
	}
}

func TestTextCommandsReplaceAndList(t *testing.T) {
	bot := New("token")
	bot.AddTextCommand(&TextCommand{Name: "b", Aliases: []string{"old"}}, &TextCommand{Name: "a"})
	bot.AddTextCommand(&TextCommand{Name: "B", Aliases: []string{"new"}})

	var names []string
	for _, cmd := range bot.TextCommands() {
		names = append(names, cmd.Name)
	}
	if !slices.Equal(names, []string{"B", "a"}) {
		t.Errorf("commands = %v", names)
	}
	if bot.textCommands["old"] != nil || bot.textCommands["new"] == nil || bot.textCommands["b"].Name != "B" {
		t.Errorf("lookup = %v", bot.textCommands)
	}
}

func TestALaterCommandTakesOverNames(t *testing.T) {
	bot := New("token")
	b := &TextCommand{Name: "b", Aliases: []string{"bee"}}
	c := &TextCommand{Name: "c", Aliases: []string{"shared"}}
	bot.AddTextCommand(b, c)

	// a takes the name of b, which could then not be run, and an alias of c.
	a := &TextCommand{Name: "a", Aliases: []string{"B", "shared"}}
	bot.AddTextCommand(a)

	if got := bot.TextCommands(); !slices.Equal(got, []*TextCommand{a, c}) {
		t.Errorf("commands = %v", got)
	}
	for name, want := range map[string]*TextCommand{"a": a, "b": a, "shared": a, "c": c, "bee": nil} {
		if got := bot.textCommands[name]; got != want {
			t.Errorf("%q runs %v, want %v", name, got, want)
		}
	}

	// Adding a command again does not list it twice.
	bot.AddTextCommand(a)
	if got := bot.TextCommands(); len(got) != 2 {
		t.Errorf("commands = %v", got)
	}
}

func TestAHandlerAddedTooLateIsReported(t *testing.T) {
	bot, d := newTestBot(t)
	On(bot, func(*EventContext, *discord.GuildBanAdd) error { return nil })

	var logged strings.Builder
	log.SetOutput(&logged)
	defer log.SetOutput(os.Stderr)

	if err := bot.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	recv(t, d.identify, "the Identify")

	// Covered by what the bot connected with, or needing nothing.
	On(bot, func(*EventContext, *discord.GuildBanRemove) error { return nil })
	bot.AddCommand(&Command{Name: "late", Description: "d"})
	if logged.Len() != 0 {
		t.Errorf("logged for a handler the bot has the intents of: %s", logged.String())
	}

	On(bot, func(*EventContext, *discord.GuildMemberAdd) error { return nil })
	if !strings.Contains(logged.String(), "while the bot is running") {
		t.Errorf("a handler without its intent was not reported: %q", logged.String())
	}

	// Closed, the bot can take it: it connects with the intent next time.
	if err := bot.Close(); err != nil {
		t.Fatal(err)
	}
	logged.Reset()
	On(bot, func(*EventContext, *discord.PresenceUpdateEvent) error { return nil })
	if logged.Len() != 0 {
		t.Errorf("logged while closed: %s", logged.String())
	}
}

func TestMessageContextOutsideGuilds(t *testing.T) {
	c := newMessageContext(&EventContext{}, &discord.Message{ID: "m", ChannelID: "c", Author: &discord.User{ID: "u"}})
	if !c.IsDM || c.Member != nil || c.Err() != nil {
		t.Errorf("context = %+v", c)
	}
}

func TestIntentsOfAConnectionAreNotKept(t *testing.T) {
	b := New("token")
	b.OnMessage(func(*MessageContext) error { return nil })
	b.connect()
	b.disconnected()

	b.SetIntents(discord.IntentsGuilds)
	if got := b.Intents(); got != discord.IntentsGuilds {
		t.Errorf("intents = %b after reconnecting with fewer", got)
	}
}

func TestIntentsOfAConnectionThatStoppedAreNotKept(t *testing.T) {
	b := New("token")
	b.OnMessage(func(*MessageContext) error { return nil })
	b.connect()
	// The connection stops on its own, and the bot is opened again without Close.

	b.SetIntents(discord.IntentsGuilds)
	b.connect()
	if got := b.Session.Intents; got != discord.IntentsGuilds {
		t.Errorf("reopened with intents %b, want %b", got, discord.IntentsGuilds)
	}
}
