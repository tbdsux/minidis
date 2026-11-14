package minidis

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tbdsux/minidis/discord"
)

func slashPayload(name, options string) string {
	return `{"type":2,` + guildFields + `,"data":{"name":"` + name + `","type":1,"options":[` + options + `]}}`
}

func TestPanicReachesErrorHandler(t *testing.T) {
	bot, _ := newTestBot(t)

	var got error
	bot.HandleError(func(c *Context, err error) {
		if c.Author.ID != "u1" {
			t.Errorf("context = %+v", c)
		}
		got = err
	})
	bot.AddCommand(&Command{Name: "boom", Execute: func(*Context) error { panic("kaput") }})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("boom", "")))

	var p *PanicError
	if !errors.As(got, &p) || p.Value != "kaput" || len(p.Stack) == 0 {
		t.Errorf("err = %#v", got)
	}
}

func TestReturnedErrorReachesErrorHandler(t *testing.T) {
	bot, _ := newTestBot(t)

	want := errors.New("nope")
	var got error
	bot.HandleError(func(_ *Context, err error) { got = err })
	bot.AddCommand(&Command{Name: "fail", Execute: func(*Context) error { return want }})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("fail", "")))

	if got != want {
		t.Errorf("err = %v", got)
	}
}

func TestChannelMessagePanicReachesItsErrorHandler(t *testing.T) {
	bot, _ := newTestBot(t)

	var got error
	bot.HandleChannelMessageError(func(c *ChannelMessageContext, err error) {
		if c.ChannelID != "c1" {
			t.Errorf("context = %+v", c)
		}
		got = err
	})
	bot.HandleChannelMessage("c1", func(*ChannelMessageContext) error { panic("kaput") })

	bot.handleChannelMessage(bot.Session, &discord.MessageCreate{Message: &discord.Message{ChannelID: "c1", Author: &discord.User{ID: "other"}}})

	var p *PanicError
	if !errors.As(got, &p) {
		t.Errorf("err = %#v", got)
	}
}

func TestStaleCommandsReportNoHandler(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{"unknown command", slashPayload("gone", "")},
		{"parent without Execute", slashPayload("parent", "")},
		{"stale subcommand", slashPayload("parent", `{"name":"old","type":1}`)},
		{"stale group", slashPayload("parent", `{"name":"oldgrp","type":2,"options":[{"name":"x","type":1}]}`)},
		{"unknown modal", `{"type":5,` + guildFields + `,"data":{"custom_id":"nope","components":[]}}`},
		{"unknown autocomplete", `{"type":4,` + guildFields + `,"data":{"name":"gone","type":1}}`},
		{"unknown user command", `{"type":2,` + guildFields + `,"data":{"name":"gone","type":2,"target_id":"t"}}`},
		{"unknown message command", `{"type":2,` + guildFields + `,"data":{"name":"gone","type":3,"target_id":"t"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot, _ := newTestBot(t)

			var got error
			bot.HandleError(func(_ *Context, err error) { got = err })

			bot.AddCommand(&Command{Name: "parent", Subcommands: []*Subcommand{{Name: "sub"}}})

			bot.handleInteraction(bot.Session, interaction(t, tt.payload))

			if !errors.Is(got, ErrNoHandler) {
				t.Errorf("err = %v", got)
			}
		})
	}
}

func TestUnhandledComponentReportsNoHandler(t *testing.T) {
	bot, _ := newTestBot(t)

	var got error
	bot.HandleError(func(_ *Context, err error) { got = err })

	bot.handleInteraction(bot.Session, interaction(t, `{"type":3,`+guildFields+`,"data":{"custom_id":"nope","component_type":2}}`))

	if !errors.Is(got, ErrNoHandler) {
		t.Errorf("err = %v", got)
	}
}

func autocompletePayload(options string) string {
	return `{"type":4,` + guildFields + `,"data":{"name":"find","type":1,"options":[` + options + `]}}`
}

func TestAutocompleteOnCommand(t *testing.T) {
	bot, requests := newTestBot(t)

	bot.AddCommand(&Command{
		Name: "find",
		Autocomplete: func(c *AutocompleteContext) error {
			if c.Focused.Name != "q" || c.Input() != "go" || c.String("other") != "x" {
				t.Errorf("focused = %+v input = %q", c.Focused, c.Input())
			}
			return c.Respond(Choice("Golang", "go"))
		},
	})

	bot.handleInteraction(bot.Session, interaction(t, autocompletePayload(`{"name":"other","type":3,"value":"x"},{"name":"q","type":3,"value":"go","focused":true}`)))

	reqs := requests()
	if len(reqs) != 1 || reqs[0].Path != "/interactions/i1/tok/callback" || reqs[0].Body["type"] != float64(8) {
		t.Fatalf("requests = %+v", reqs)
	}
	choice := reqs[0].Body["data"].(map[string]any)["choices"].([]any)[0].(map[string]any)
	if choice["name"] != "Golang" || choice["value"] != "go" {
		t.Errorf("choice = %v", choice)
	}
}

func TestAutocompleteOnNestedSubcommands(t *testing.T) {
	tests := []struct {
		name    string
		options string
	}{
		{"subcommand", `{"name":"sub","type":1,"options":[{"name":"q","type":3,"value":"a","focused":true}]}`},
		{"group", `{"name":"grp","type":2,"options":[{"name":"nested","type":1,"options":[{"name":"q","type":3,"value":"a","focused":true}]}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot, _ := newTestBot(t)

			got := ""
			complete := func(c *AutocompleteContext) error { got = c.Input(); return nil }

			bot.AddCommand(&Command{
				Name:        "find",
				Subcommands: []*Subcommand{{Name: "sub", Autocomplete: complete}},
				Groups:      []*SubcommandGroup{{Name: "grp", Subcommands: []*Subcommand{{Name: "nested", Autocomplete: complete}}}},
			})

			bot.handleInteraction(bot.Session, interaction(t, autocompletePayload(tt.options)))

			if got != "a" {
				t.Errorf("input = %q", got)
			}
		})
	}
}

func TestAutocompleteInputOfNumberOptions(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"whole number", `1000000`, "1000000"},
		{"partial text", `"12a"`, "12a"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot, _ := newTestBot(t)

			got := ""
			bot.AddCommand(&Command{Name: "find", Autocomplete: func(c *AutocompleteContext) error {
				got = c.Input()
				return nil
			}})

			bot.handleInteraction(bot.Session, interaction(t, autocompletePayload(`{"name":"n","type":4,"value":`+tt.value+`,"focused":true}`)))

			if got != tt.want {
				t.Errorf("input = %q", got)
			}
		})
	}
}

func TestAutocompleteResponseLimits(t *testing.T) {
	many := make([]*discord.ApplicationCommandOptionChoice, 30)
	for i := range many {
		many[i] = Choice(fmt.Sprint(i), i)
	}

	tests := []struct {
		name    string
		choices []*discord.ApplicationCommandOptionChoice
		want    int
	}{
		{"none is an empty array", nil, 0},
		{"excess is dropped", many, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot, requests := newTestBot(t)

			bot.AddCommand(&Command{Name: "find", Autocomplete: func(c *AutocompleteContext) error {
				return c.Respond(tt.choices...)
			}})

			bot.handleInteraction(bot.Session, interaction(t, autocompletePayload("")))

			choices, ok := requests()[0].Body["data"].(map[string]any)["choices"].([]any)
			if !ok || len(choices) != tt.want {
				t.Errorf("body = %s", requests()[0].Raw)
			}
		})
	}
}

func TestMiddlewareOrderAndCoverage(t *testing.T) {
	bot, _ := newTestBot(t)

	var ran []string
	tag := func(name string) Middleware {
		return func(next HandlerFunc) HandlerFunc {
			return func(c *Context) error {
				ran = append(ran, name+":before")
				err := next(c)
				ran = append(ran, name+":after")
				return err
			}
		}
	}
	bot.Use(tag("a"), tag("b"))

	mark := func() error { ran = append(ran, "handler"); return nil }

	bot.AddCommand(&Command{
		Name:    "find",
		Execute: func(*Context) error { return mark() },
		Autocomplete: func(*AutocompleteContext) error {
			t.Log("autocomplete ran")
			return nil
		},
	})
	bot.HandleComponent("b", func(*ComponentContext) error { return mark() })
	bot.HandleModal("f", func(*ModalSubmitContext) error { return mark() })
	bot.AddMessageCommand(&MessageCommand{Name: "m", Execute: func(*MessageCommandContext) error { return mark() }})

	want := "a:before b:before handler b:after a:after"

	payloads := map[string]string{
		"slash":     slashPayload("find", ""),
		"component": `{"type":3,` + guildFields + `,"data":{"custom_id":"b","component_type":2}}`,
		"modal":     `{"type":5,` + guildFields + `,"data":{"custom_id":"f","components":[]}}`,
		"menu":      `{"type":2,` + guildFields + `,"data":{"name":"m","type":3,"target_id":"t","resolved":{"messages":{"t":{"id":"t"}}}}}`,
	}
	for name, payload := range payloads {
		ran = nil
		bot.handleInteraction(bot.Session, interaction(t, payload))
		if got := strings.Join(ran, " "); got != want {
			t.Errorf("%s: ran %q", name, got)
		}
	}

	ran = nil
	bot.handleInteraction(bot.Session, interaction(t, autocompletePayload("")))
	if len(ran) != 0 {
		t.Errorf("middleware ran for autocomplete: %v", ran)
	}
}

func TestMiddlewareCanStopAnInteraction(t *testing.T) {
	bot, _ := newTestBot(t)

	bot.Use(func(HandlerFunc) HandlerFunc {
		return func(*Context) error { return nil }
	})
	bot.AddCommand(&Command{Name: "c", Execute: func(*Context) error {
		t.Error("handler ran")
		return nil
	}})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))
}

func TestPanicInMiddlewareIsRecovered(t *testing.T) {
	bot, _ := newTestBot(t)

	var got error
	bot.HandleError(func(_ *Context, err error) { got = err })
	bot.Use(func(HandlerFunc) HandlerFunc {
		return func(*Context) error { panic("mw") }
	})
	bot.AddCommand(&Command{Name: "c", Execute: func(*Context) error { return nil }})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))

	var p *PanicError
	if !errors.As(got, &p) || p.Value != "mw" {
		t.Errorf("err = %v", got)
	}
}

func componentRespond(t *testing.T, respond func(*ComponentContext) error) recordedRequest {
	t.Helper()

	bot, requests := newTestBot(t)
	bot.HandleError(func(_ *Context, err error) { t.Error(err) })
	bot.HandleComponent("b", func(c *ComponentContext) error {
		return respond(c)
	})

	bot.handleInteraction(bot.Session, interaction(t, `{"type":3,`+guildFields+`,"data":{"custom_id":"b","component_type":2}}`))

	reqs := requests()
	if len(reqs) != 1 || reqs[0].Path != "/interactions/i1/tok/callback" {
		t.Fatalf("requests = %+v", reqs)
	}
	return reqs[0]
}

func TestComponentUpdate(t *testing.T) {
	req := componentRespond(t, func(c *ComponentContext) error { return c.Update("changed") })

	data := req.Body["data"].(map[string]any)
	if req.Body["type"] != float64(7) || data["content"] != "changed" {
		t.Errorf("body = %s", req.Raw)
	}
	if _, sent := data["components"]; sent {
		t.Errorf("nil components must leave the message's alone: %s", req.Raw)
	}
}

func TestComponentUpdateCanClearComponents(t *testing.T) {
	req := componentRespond(t, func(c *ComponentContext) error {
		return c.UpdateWith(MessageEdit{Components: []discord.MessageComponent{}})
	})

	components, ok := req.Body["data"].(map[string]any)["components"].([]any)
	if !ok || len(components) != 0 {
		t.Errorf("body = %s", req.Raw)
	}
}

func TestComponentDeferUpdate(t *testing.T) {
	req := componentRespond(t, func(c *ComponentContext) error { return c.DeferUpdate() })

	if req.Body["type"] != float64(6) {
		t.Errorf("body = %s", req.Raw)
	}
}

func TestEditCanClearComponents(t *testing.T) {
	bot, requests := newTestBot(t)

	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		if err := c.Edit("text"); err != nil {
			return err
		}
		return c.EditWith(MessageEdit{Content: discord.String("text"), Components: []discord.MessageComponent{}})
	}})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))

	reqs := requests()
	if _, sent := reqs[0].Body["components"]; sent {
		t.Errorf("nil components were sent: %s", reqs[0].Raw)
	}
	if components, ok := reqs[1].Body["components"].([]any); !ok || len(components) != 0 {
		t.Errorf("empty components were not sent: %s", reqs[1].Raw)
	}
}

func TestContextMenusCanUseTheFullReplyAPI(t *testing.T) {
	bot, requests := newTestBot(t)

	bot.AddMessageCommand(&MessageCommand{Name: "m", Execute: func(c *MessageCommandContext) error {
		if c.Author.ID != "u1" || c.Message.ID != "t" {
			t.Errorf("author = %+v message = %+v", c.Author, c.Message)
		}
		return c.DeferReply(true)
	}})
	bot.AddUserCommand(&UserCommand{Name: "u", Execute: func(c *UserCommandContext) error {
		if c.TargetUser.ID != "t" || c.Author.ID != "u1" || c.Member.User.ID != "u1" {
			t.Errorf("target = %+v author = %+v", c.TargetUser, c.Author)
		}
		return c.ReplyWith(Message{Content: "hi", Ephemeral: true})
	}})

	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"m","type":3,"target_id":"t","resolved":{"messages":{"t":{"id":"t"}}}}}`))
	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"u","type":2,"target_id":"t","resolved":{"users":{"t":{"id":"t"}},"members":{"t":{"permissions":"0"}}}}}`))

	reqs := requests()
	if len(reqs) != 2 || reqs[0].Body["type"] != float64(5) || reqs[1].Body["type"] != float64(4) {
		t.Errorf("requests = %+v", reqs)
	}
}

func TestCommandsOfDifferentTypesCanShareAName(t *testing.T) {
	bot, _ := newTestBot(t)

	ran := ""
	bot.AddCommand(&Command{Name: "x", Execute: func(*Context) error { ran += "slash "; return nil }})
	bot.AddUserCommand(&UserCommand{Name: "x", Execute: func(*UserCommandContext) error { ran += "user "; return nil }})
	bot.AddMessageCommand(&MessageCommand{Name: "x", Execute: func(*MessageCommandContext) error { ran += "message "; return nil }})

	resolved := `"target_id":"t","resolved":{"users":{"t":{"id":"t"}},"messages":{"t":{"id":"t"}}}`
	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"x","type":3,`+resolved+`}}`))
	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"x","type":2,`+resolved+`}}`))
	bot.handleInteraction(bot.Session, interaction(t, slashPayload("x", "")))

	if ran != "message user slash " {
		t.Errorf("ran = %q", ran)
	}
}

func TestCustomIDPatterns(t *testing.T) {
	tests := []struct {
		name     string
		customID string
		want     string
		params   map[string]string
	}{
		{"exact wins over pattern", "vote:exact", "exact", nil},
		{"single capture", "vote:42", "one", map[string]string{"poll": "42"}},
		{"two captures", "vote:42:yes", "two", map[string]string{"poll": "42", "choice": "yes"}},
		{"literal segment must match", "ballot:42", "fallback", nil},
		{"segment count must match", "vote:42:yes:extra", "fallback", nil},
		{"capture must not be empty", "vote:", "fallback", nil},
		{"partial braces are literal", "tag-{x}:1", "literal", map[string]string{"n": "1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot, _ := newTestBot(t)

			got := ""
			var params map[string]string
			add := func(id, name string) {
				bot.HandleComponent(id, func(c *ComponentContext) error {
					got, params = name, c.Params
					return nil
				})
			}
			add("vote:exact", "exact")
			add("vote:{poll}", "one")
			add("vote:{poll}:{choice}", "two")
			add("tag-{x}:{n}", "literal")
			bot.HandleComponentFallback(func(c *ComponentContext) error {
				got, params = "fallback", c.Params
				return nil
			})

			bot.handleInteraction(bot.Session, interaction(t, `{"type":3,`+guildFields+`,"data":{"custom_id":"`+tt.customID+`","component_type":2}}`))

			if got != tt.want || fmt.Sprint(params) != fmt.Sprint(tt.params) && len(params)+len(tt.params) > 0 {
				t.Errorf("got %q %v, want %q %v", got, params, tt.want, tt.params)
			}
		})
	}
}

func TestReregisteringAPatternReplacesIt(t *testing.T) {
	bot, _ := newTestBot(t)

	got := ""
	add := func(name string) {
		bot.HandleComponent("a:{b}", func(*ComponentContext) error {
			got = name
			return nil
		})
	}
	add("first")
	add("second")

	bot.handleInteraction(bot.Session, interaction(t, `{"type":3,`+guildFields+`,"data":{"custom_id":"a:1","component_type":2}}`))

	if got != "second" || len(bot.components.patterns) != 1 {
		t.Errorf("got %q, routes %d", got, len(bot.components.patterns))
	}
}

func TestModalIDPattern(t *testing.T) {
	bot, _ := newTestBot(t)

	got := ""
	bot.HandleModal("form:{id}", func(c *ModalSubmitContext) error {
		got = c.Params["id"]
		return nil
	})

	bot.handleInteraction(bot.Session, interaction(t, `{"type":5,`+guildFields+`,"data":{"custom_id":"form:7","components":[]}}`))

	if got != "7" {
		t.Errorf("param = %q", got)
	}
}

func TestOptionGetters(t *testing.T) {
	bot, _ := newTestBot(t)

	options := `{"name":"s","type":3,"value":"text"},{"name":"i","type":4,"value":5},{"name":"f","type":10,"value":1.5},{"name":"b","type":5,"value":true},` +
		`{"name":"u","type":6,"value":"10"},{"name":"c","type":7,"value":"20"},{"name":"r","type":8,"value":"30"},{"name":"a","type":11,"value":"40"},{"name":"m","type":9,"value":"30"}`
	resolved := `"resolved":{"users":{"10":{"id":"10"}},"members":{"10":{"nick":"n","permissions":"0"}},"channels":{"20":{"id":"20"}},"roles":{"30":{"id":"30","permissions":"0"}},"attachments":{"40":{"id":"40"}}}`

	called := false
	bot.HandleError(func(_ *Context, err error) { t.Error(err) })
	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		called = true

		if c.String("s") != "text" || c.Int("i") != 5 || c.Float("f") != 1.5 || !c.Bool("b") {
			t.Errorf("values: %q %d %v %v", c.String("s"), c.Int("i"), c.Float("f"), c.Bool("b"))
		}

		if v, ok := c.OptionalInt("i"); v != 5 || !ok {
			t.Errorf("OptionalInt = %d %v", v, ok)
		}
		if _, ok := c.OptionalString("missing"); ok {
			t.Error("missing option reported as given")
		}
		if c.String("i") != "" || c.Int("s") != 0 || c.Bool("s") || c.Float("i") != 0 || c.User("s") != nil {
			t.Error("a type mismatch must give the zero value")
		}

		if c.User("u").ID != "10" || c.Channel("c").ID != "20" || c.Role("r").ID != "30" || c.Attachment("a").ID != "40" || c.Role("m").ID != "30" {
			t.Error("resolved objects were not found")
		}
		if m := c.OptionMember("u"); m.Nick != "n" || m.User.ID != "10" {
			t.Errorf("member = %+v", m)
		}

		if c.User("missing") != nil || c.OptionMember("missing") != nil || c.Channel("missing") != nil || c.Role("missing") != nil || c.Attachment("missing") != nil {
			t.Error("missing options must give nil")
		}
		return nil
	}})

	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"c","type":1,"options":[`+options+`],`+resolved+`}}`))

	if !called {
		t.Error("handler was not called")
	}
}

func TestContextsAndIntegrationTypes(t *testing.T) {
	bot, requests := newTestBot(t)

	contexts := []discord.InteractionContextType{discord.InteractionContextPrivateChannel}
	types := []discord.ApplicationIntegrationType{discord.ApplicationIntegrationUserInstall}

	bot.AddCommand(&Command{Name: "s", Description: "d", Contexts: contexts, IntegrationTypes: types})
	bot.AddMessageCommand(&MessageCommand{Name: "m", Contexts: contexts, IntegrationTypes: types, DefaultMemberPermissions: discord.Int64(8)})
	bot.AddUserCommand(&UserCommand{Name: "u", Contexts: contexts, IntegrationTypes: types})
	bot.AddMessageCommand(&MessageCommand{Name: "plain"})

	if err := bot.SyncCommands(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := bot.SyncCommands(t.Context(), "g"); err != nil {
		t.Fatal(err)
	}

	global := commandByName(t, requests()[0].Raw)
	for _, name := range []string{"s", "m", "u"} {
		if c := global[name]; fmt.Sprint(c["contexts"]) != "[2]" || fmt.Sprint(c["integration_types"]) != "[1]" {
			t.Errorf("%s = %v", name, c)
		}
	}
	if global["m"]["default_member_permissions"] != "8" {
		t.Errorf("m = %v", global["m"])
	}
	if fmt.Sprint(global["plain"]["contexts"]) != "[0]" {
		t.Errorf("unset contexts must mean guilds only: %v", global["plain"])
	}

	for name, c := range commandByName(t, requests()[1].Raw) {
		if _, sent := c["integration_types"]; sent {
			t.Errorf("%s has integration_types, which are global-only", name)
		}
	}
}
