package minidis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
)

var (
	_ context.Context = (*Context)(nil)
	_ context.Context = (*ComponentContext)(nil)
	_ context.Context = (*ChannelMessageContext)(nil)
)

func TestEditWithoutContentKeepsTheText(t *testing.T) {
	bot, requests := newTestBot(t)

	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		if err := c.EditWith(MessageEdit{Embeds: []*discord.MessageEmbed{{Title: "t"}}}); err != nil {
			return err
		}
		return c.EditWith(MessageEdit{Content: discord.String("")})
	}})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))

	reqs := requests()
	if _, sent := reqs[0].Body["content"]; sent {
		t.Errorf("unset content was sent, which erases the text: %s", reqs[0].Raw)
	}
	if content, sent := reqs[1].Body["content"]; !sent || content != "" {
		t.Errorf("empty content must be sent to clear the text: %s", reqs[1].Raw)
	}
}

func TestComponentUpdateContent(t *testing.T) {
	tests := []struct {
		name string
		edit MessageEdit
		want any // nil means not sent
	}{
		{"unset is left alone", MessageEdit{Components: []discord.MessageComponent{}}, nil},
		{"empty clears", MessageEdit{Content: discord.String("")}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := componentRespond(t, func(c *ComponentContext) error { return c.UpdateWith(tt.edit) })

			content, sent := req.Body["data"].(map[string]any)["content"]
			if (tt.want == nil) == sent || sent && content != tt.want {
				t.Errorf("body = %s", req.Raw)
			}
		})
	}
}

func TestHandlerContextLifetime(t *testing.T) {
	bot, _ := newTestBot(t)

	var held *Context
	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		held = c
		if c.Err() != nil {
			t.Errorf("canceled during the handler: %v", c.Err())
		}
		if deadline, ok := c.Deadline(); !ok || time.Until(deadline) > interactionLifetime {
			t.Errorf("deadline = %v %v", deadline, ok)
		}
		return nil
	}})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))

	if held == nil || held.Err() == nil {
		t.Error("the context must be canceled once the handler returns")
	}
}

func TestShutdownEndsHandlerContexts(t *testing.T) {
	bot, _ := newTestBot(t)

	started, done := make(chan struct{}), make(chan error)
	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		if err := c.DeferReply(false); err != nil {
			return err
		}
		close(started)
		<-c.Done()
		done <- c.Err()
		return nil
	}})

	// Called directly, the handler is not one Shutdown waits for, so the
	// shutdown ends at once and cancels it.
	go bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))
	<-started

	if err := bot.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("handler context was not canceled")
	}
}

func TestSyncLooksUpTheApplicationID(t *testing.T) {
	bot, requests := newTestBot(t)
	bot.appID = ""

	for range 2 {
		if err := bot.SyncCommands(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	var got []string
	for _, r := range requests() {
		got = append(got, r.Method+" "+r.Path)
	}

	want := []string{"GET /applications/@me", "PUT /applications/sent/commands", "PUT /applications/sent/commands"}
	if len(got) != len(want) {
		t.Fatalf("requests = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSyncIsDeterministicAndLeavesCommandsAlone(t *testing.T) {
	bot, requests := newTestBot(t)

	options := make([]*discord.ApplicationCommandOption, 1, 4)
	options[0] = &discord.ApplicationCommandOption{Name: "o", Description: "d", Type: discord.ApplicationCommandOptionString}

	bot.AddCommand(
		&Command{Name: "c", Description: "d", Subcommands: []*Subcommand{{Name: "s2"}, {Name: "s1"}}},
		&Command{Name: "b", Description: "d", Options: options},
		&Command{Name: "a", Description: "d"},
	)

	if err := bot.SyncCommands(t.Context(), "g"); err != nil {
		t.Fatal(err)
	}

	if spare := options[:2][1]; spare != nil {
		t.Errorf("sync wrote into the command's Options: %+v", spare)
	}

	var sent []struct {
		Name    string `json:"name"`
		Options []struct {
			Name string `json:"name"`
		} `json:"options"`
	}
	if err := json.Unmarshal(requests()[0].Raw, &sent); err != nil {
		t.Fatal(err)
	}

	if len(sent) != 3 || sent[0].Name != "a" || sent[1].Name != "b" || sent[2].Name != "c" {
		t.Fatalf("commands = %+v", sent)
	}
	if opts := sent[1].Options; len(opts) != 1 || opts[0].Name != "o" {
		t.Errorf("options = %+v", opts)
	}
	if subs := sent[2].Options; len(subs) != 2 || subs[0].Name != "s2" || subs[1].Name != "s1" {
		t.Errorf("subcommands = %+v, want declaration order", subs)
	}
}

func TestOnMessageCreateAddsHandlers(t *testing.T) {
	bot, _ := newTestBot(t)

	// Registering must not replace the channel message dispatch set up by New.
	bot.OnMessageCreate(func(*client.Session, *discord.MessageCreate) {})

	ran := false
	bot.HandleChannelMessage("c1", func(*ChannelMessageContext) error { ran = true; return nil })
	bot.handleChannelMessage(bot.Session, &discord.MessageCreate{Message: &discord.Message{ChannelID: "c1", Author: &discord.User{ID: "other"}}})

	if !ran {
		t.Error("channel handler did not run")
	}
}

func TestUnansweredInteractionsAreCanceledAfterThreeSeconds(t *testing.T) {
	bot, _ := newTestBot(t)

	var cause error
	var waited time.Duration
	bot.AddCommand(&Command{Name: "slow", Execute: func(c *Context) error {
		start := time.Now()
		<-c.Done()
		waited, cause = time.Since(start), context.Cause(c)
		return nil
	}})

	synctest.Test(t, func(t *testing.T) {
		bot.handleInteraction(bot.Session, interaction(t, slashPayload("slow", "")))
	})

	if !errors.Is(cause, ErrNoResponse) || waited != responseWindow {
		t.Errorf("cause = %v after %v", cause, waited)
	}
}

func TestRespondedAndWithContext(t *testing.T) {
	bot, _ := newTestBot(t)

	type key struct{}
	bot.Use(func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			return next(c.WithContext(context.WithValue(c, key{}, "traced")))
		}
	})

	var before, after bool
	var value any
	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		value = c.Value(key{})
		before = c.Responded()
		err := c.Reply("hi")
		after = c.Responded()
		return err
	}})

	var inErrorHandler bool
	bot.HandleError(func(c *Context, err error) { inErrorHandler = c.Responded() })
	bot.AddCommand(&Command{Name: "fails", Execute: func(c *Context) error {
		if err := c.WithContext(c).DeferReply(false); err != nil { // a copy shares the state
			return err
		}
		return errors.New("later")
	}})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))
	bot.handleInteraction(bot.Session, interaction(t, slashPayload("fails", "")))

	if value != "traced" || before || !after || !inErrorHandler {
		t.Errorf("value = %v, responded before = %v, after = %v, in error handler = %v", value, before, after, inErrorHandler)
	}
}

func TestPanicErrorUnwraps(t *testing.T) {
	cause := io.ErrUnexpectedEOF
	if err := error(&PanicError{Value: cause}); !errors.Is(err, cause) {
		t.Error("errors.Is does not see through PanicError")
	}
	if (&PanicError{Value: "text"}).Unwrap() != nil {
		t.Error("a non-error panic value unwrapped to an error")
	}
}

func TestSyncEmptyGuildIDsAndValidation(t *testing.T) {
	bot, requests := newTestBot(t)
	bot.AddCommand(&Command{Name: "c", Description: "d"})

	// strings.Split of an unset variable gives [""], which must not replace
	// the global commands by mistake.
	if err := bot.SyncCommands(t.Context(), strings.Split("", ",")...); err == nil {
		t.Error("only empty IDs were accepted")
	}
	if err := bot.SyncCommands(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := bot.SyncCommands(t.Context(), " g1 ", ""); err != nil {
		t.Fatal(err)
	}

	reqs := requests()
	if len(reqs) != 2 || reqs[0].Path != "/applications/app/commands" || reqs[1].Path != "/applications/app/guilds/g1/commands" {
		t.Errorf("requests = %+v", reqs)
	}

	bot.AddCommand(&Command{
		Name: "mixed", Description: "d",
		Options:     []*discord.ApplicationCommandOption{{Name: "o", Description: "d", Type: discord.ApplicationCommandOptionString}},
		Subcommands: []*Subcommand{{Name: "s", Description: "d"}},
	})
	if err := bot.SyncCommands(t.Context()); err == nil || !strings.Contains(err.Error(), `"mixed"`) {
		t.Errorf("err = %v", err)
	}
	if len(requests()) != 2 {
		t.Error("an invalid command was sent")
	}
}

func TestChannelMessageSendWithAndReplyWith(t *testing.T) {
	bot, requests := newTestBot(t)

	none := &discord.MessageAllowedMentions{Parse: []discord.AllowedMentionType{}}
	bot.HandleChannelMessage("c1", func(c *ChannelMessageContext) error {
		if _, err := c.SendWith(Message{Content: "@everyone", AllowedMentions: none}); err != nil {
			return err
		}
		_, err := c.ReplyWith(Message{Content: "re"})
		return err
	})
	bot.HandleChannelMessageError(func(_ *ChannelMessageContext, err error) { t.Error(err) })

	bot.handleChannelMessage(bot.Session, &discord.MessageCreate{Message: &discord.Message{ID: "m1", ChannelID: "c1", Author: &discord.User{ID: "u"}}})

	reqs := requests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %+v", reqs)
	}
	if _, ok := reqs[0].Body["allowed_mentions"]; !ok || reqs[0].Body["message_reference"] != nil {
		t.Errorf("send = %s", reqs[0].Raw)
	}
	if ref, _ := reqs[1].Body["message_reference"].(map[string]any); ref["message_id"] != "m1" {
		t.Errorf("reply = %s", reqs[1].Raw)
	}
}

func TestModalSubmitCanUpdateTheMessage(t *testing.T) {
	bot, requests := newTestBot(t)

	var errs []error
	bot.HandleModal("form", func(c *ModalSubmitContext) error {
		errs = append(errs, c.Update("submitted"))
		if c.Interaction.Message == nil {
			errs = append(errs, c.DeferUpdate())
		}
		return nil
	})

	// Opened from a button, so the submission carries its message.
	bot.handleInteraction(bot.Session, interaction(t, `{"type":5,`+guildFields+`,"message":{"id":"m1"},"data":{"custom_id":"form","components":[]}}`))
	// Opened by a command: there is no message to update.
	bot.handleInteraction(bot.Session, interaction(t, `{"type":5,`+guildFields+`,"data":{"custom_id":"form","components":[]}}`))

	reqs := requests()
	if len(reqs) != 1 || reqs[0].Body["type"] != float64(discord.InteractionResponseUpdateMessage) || reqs[0].Body["data"].(map[string]any)["content"] != "submitted" {
		t.Fatalf("requests = %+v", reqs)
	}
	if len(errs) != 3 || errs[0] != nil || !errors.Is(errs[1], errNoModalMessage) || !errors.Is(errs[2], errNoModalMessage) {
		t.Errorf("errors = %v", errs)
	}
}

func TestDefaultContexts(t *testing.T) {
	bot, requests := newTestBot(t)

	userInstall := []discord.ApplicationIntegrationType{discord.ApplicationIntegrationUserInstall}
	bot.AddCommand(
		&Command{Name: "guild", Description: "d"},
		&Command{Name: "anywhere", Description: "d", IntegrationTypes: userInstall},
	)
	bot.AddUserCommand(&UserCommand{Name: "user", IntegrationTypes: userInstall})

	if err := bot.SyncCommands(t.Context()); err != nil {
		t.Fatal(err)
	}

	cmds := commandByName(t, requests()[0].Raw)
	if fmt.Sprint(cmds["guild"]["contexts"]) != "[0]" {
		t.Errorf("guild = %v", cmds["guild"])
	}
	for _, name := range []string{"anywhere", "user"} {
		if _, sent := cmds[name]["contexts"]; sent {
			t.Errorf("%s: user-installable commands keep Discord's default contexts: %v", name, cmds[name])
		}
	}
}

func TestErrorHandlerSeesMiddlewareValues(t *testing.T) {
	bot, _ := newTestBot(t)

	type key struct{}
	bot.Use(func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			return next(c.WithContext(context.WithValue(c, key{}, "rid1")))
		}
	})
	bot.AddCommand(&Command{Name: "fail", Execute: func(*Context) error { return errors.New("nope") }})

	var got any
	bot.HandleError(func(c *Context, _ error) { got = c.Value(key{}) })

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("fail", "")))

	if got != "rid1" {
		t.Errorf("error handler saw %v", got)
	}
}

func TestSpecificPatternsWin(t *testing.T) {
	bot, _ := newTestBot(t)

	got := ""
	bot.HandleComponent("ticket:{action}:{id}", func(*ComponentContext) error { got = "generic"; return nil })
	bot.HandleComponent("ticket:close:{id}", func(*ComponentContext) error { got = "close"; return nil })

	bot.handleInteraction(bot.Session, interaction(t, `{"type":3,`+guildFields+`,"data":{"custom_id":"ticket:close:1","component_type":2}}`))
	if got != "close" {
		t.Errorf("ticket:close:1 went to %q", got)
	}

	bot.handleInteraction(bot.Session, interaction(t, `{"type":3,`+guildFields+`,"data":{"custom_id":"ticket:open:1","component_type":2}}`))
	if got != "generic" {
		t.Errorf("ticket:open:1 went to %q", got)
	}
}

func TestSyncValidation(t *testing.T) {
	exec := func(*Context) error { return nil }
	complete := func(*AutocompleteContext) error { return nil }
	search := &discord.ApplicationCommandOption{Name: "q", Description: "d", Type: discord.ApplicationCommandOptionString, Autocomplete: true}

	tests := []struct {
		name string
		cmd  *Command
		want string // empty when valid
	}{
		{"autocomplete answered", &Command{Name: "a", Description: "d", Options: []*discord.ApplicationCommandOption{search}, Autocomplete: complete}, ""},
		{"autocomplete unanswered", &Command{Name: "a", Description: "d", Options: []*discord.ApplicationCommandOption{search}}, "no Autocomplete handler"},
		{"unanswered in a subcommand", &Command{Name: "a", Description: "d", Subcommands: []*Subcommand{{Name: "s", Description: "d", Options: []*discord.ApplicationCommandOption{search}}}}, `"a s"`},
		{"choices and autocomplete", &Command{Name: "a", Description: "d", Autocomplete: complete, Options: []*discord.ApplicationCommandOption{{
			Name: "q", Description: "d", Type: discord.ApplicationCommandOptionString, Autocomplete: true,
			Choices: []*discord.ApplicationCommandOptionChoice{{Name: "x", Value: "x"}},
		}}}, "both Choices and Autocomplete"},
		{"root Execute with subcommands", &Command{Name: "a", Description: "d", Execute: exec, Subcommands: []*Subcommand{{Name: "s", Description: "d"}}}, "never run"},
		{"unanswered in a group", &Command{Name: "a", Description: "d", Groups: []*SubcommandGroup{{Name: "g", Description: "d", Subcommands: []*Subcommand{
			{Name: "s", Description: "d", Options: []*discord.ApplicationCommandOption{search}},
		}}}}, `"a g s"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot, _ := newTestBot(t)
			bot.AddCommand(tt.cmd)

			err := bot.SyncCommands(t.Context(), "g")
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestClearCommandsRefusesOnlyEmptyIDs(t *testing.T) {
	bot, requests := newTestBot(t)

	if err := bot.ClearCommands(t.Context(), strings.Split("", ",")...); err == nil {
		t.Error("an unset variable cleared the global commands")
	}
	if len(requests()) != 0 {
		t.Errorf("requests = %+v", requests())
	}
}
