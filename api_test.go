package minidis

import (
	"context"
	"encoding/json"
	"testing"
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

func TestShutdownCancelsHandlerContexts(t *testing.T) {
	bot, _ := newTestBot(t)

	ctx, cancel := context.WithCancel(context.Background())
	bot.running.Store(&runState{ctx: ctx, cancel: cancel})

	started, done := make(chan struct{}), make(chan error)
	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		close(started)
		<-c.Done()
		done <- c.Err()
		return nil
	}})

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
	case <-time.After(5 * time.Second):
		t.Fatal("handler context was not canceled")
	}
}

func TestSyncLooksUpTheApplicationID(t *testing.T) {
	bot, requests := newTestBot(t)
	bot.appID = ""

	for range 2 {
		if err := bot.SyncCommands(); err != nil {
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
		&Command{Name: "b", Description: "d", Options: options, Subcommands: []*Subcommand{{Name: "s2"}, {Name: "s1"}}},
		&Command{Name: "a", Description: "d"},
	)

	if err := bot.SyncCommands("g"); err != nil {
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

	if len(sent) != 2 || sent[0].Name != "a" || sent[1].Name != "b" {
		t.Fatalf("commands = %+v", sent)
	}
	opts := sent[1].Options
	if len(opts) != 3 || opts[0].Name != "o" || opts[1].Name != "s2" || opts[2].Name != "s1" {
		t.Errorf("options = %+v, want declaration order", opts)
	}
}

func TestOnMessageCreateAddsHandlers(t *testing.T) {
	bot, _ := newTestBot(t)

	// Registering must not replace the channel message dispatch set up by New.
	bot.OnMessageCreate(func(*client.Session, *discord.MessageCreate) {})

	ran := false
	bot.OnChannelMessageCreate(func(*ChannelMessageContext) error { ran = true; return nil }, "c1")
	bot.handleChannelMessage(bot.Session, &discord.MessageCreate{Message: &discord.Message{ChannelID: "c1", Author: &discord.User{ID: "other"}}})

	if !ran {
		t.Error("channel handler did not run")
	}
}
