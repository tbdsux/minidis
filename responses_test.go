package minidis

import (
	"net/http"
	"testing"

	"github.com/tbdsux/minidis/discord"
)

func TestReplyFlagsPollAndTTS(t *testing.T) {
	bot, requests := newTestBot(t)

	poll := &discord.PollCreateRequest{
		Question: discord.PollMedia{Text: "Best?"},
		Answers:  []discord.PollAnswer{{PollMedia: discord.PollMedia{Text: "Go"}}},
	}
	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		if err := c.ReplyWith(Message{Poll: poll, TTS: true, Ephemeral: true, Flags: discord.MessageFlagsSuppressNotifications}); err != nil {
			return err
		}
		_, err := c.FollowupWith(Message{Flags: discord.MessageFlagsIsComponentsV2, Components: []discord.MessageComponent{discord.TextDisplay{Content: "v2"}}})
		return err
	}})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))

	reqs := requests()
	data := reqs[0].Body["data"].(map[string]any)
	wantFlags := float64(discord.MessageFlagsEphemeral | discord.MessageFlagsSuppressNotifications)
	if data["flags"] != wantFlags || data["tts"] != true || data["poll"].(map[string]any)["question"].(map[string]any)["text"] != "Best?" {
		t.Errorf("reply = %s", reqs[0].Raw)
	}
	if reqs[1].Body["flags"] != float64(discord.MessageFlagsIsComponentsV2) {
		t.Errorf("followup = %s", reqs[1].Raw)
	}
}

func TestDeferredReplyBecomesComponentsV2(t *testing.T) {
	bot, requests := newTestBot(t)

	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		if err := c.DeferReply(false); err != nil {
			return err
		}
		return c.EditWith(MessageEdit{
			Flags:      discord.MessageFlagsIsComponentsV2,
			Components: []discord.MessageComponent{discord.TextDisplay{Content: "done"}},
		})
	}})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))

	edit := requests()[1]
	if edit.Method != http.MethodPatch || edit.Body["flags"] != float64(discord.MessageFlagsIsComponentsV2) {
		t.Errorf("edit = %s %s %s", edit.Method, edit.Path, edit.Raw)
	}
	if content, sent := edit.Body["content"]; !sent || content != nil {
		t.Errorf("content must be null to turn a message into Components V2: %s", edit.Raw)
	}
}

func TestResponseAndLaunchActivity(t *testing.T) {
	bot, requests := newTestBot(t)

	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		if err := c.LaunchActivity(); err != nil {
			return err
		}
		msg, err := c.Response()
		if err == nil && msg.ID != "sent" {
			t.Errorf("message = %+v", msg)
		}
		return err
	}})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))

	reqs := requests()
	if len(reqs) != 2 || reqs[0].Body["type"] != float64(12) || reqs[1].Method != http.MethodGet || reqs[1].Path != "/webhooks/app/tok/messages/@original" {
		t.Errorf("requests = %+v", reqs)
	}
}
