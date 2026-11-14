package minidis

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

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

// refusingBot is a bot whose fake API refuses every interaction callback the
// way Discord refuses an invalid body.
func refusingBot(t *testing.T) (*Minidis, func() []string) {
	t.Helper()

	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()

		if strings.HasSuffix(r.URL.Path, "/callback") {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"code":50035,"message":"Invalid Form Body"}`)
			return
		}
		io.WriteString(w, `{"id":"sent"}`)
	}))
	t.Cleanup(srv.Close)

	bot := New("token")
	bot.Session.BaseURL = srv.URL
	return bot, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(paths)
	}
}

func TestARefusedResponseDoesNotCount(t *testing.T) {
	bot, _ := refusingBot(t)

	var responded, inErrorHandler bool
	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		err := c.Reply("too long, say")
		responded = c.Responded()
		return err
	}})
	bot.HandleError(func(c *Context, err error) { inErrorHandler = c.Responded() })

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))

	if responded || inErrorHandler {
		t.Errorf("Responded after a refused reply: in handler %v, in error handler %v", responded, inErrorHandler)
	}
}

func TestSendResponseCounts(t *testing.T) {
	bot, requests := newTestBot(t)

	var out *discord.InteractionCallbackResponse
	var responded bool
	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		var err error
		out, err = c.SendResponse(&discord.InteractionResponse{Type: discord.InteractionResponseLaunchActivity})
		responded = c.Responded()
		return err
	}})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))

	reqs := requests()
	if out == nil || !responded || len(reqs) != 1 || reqs[0].Body["type"] != float64(12) {
		t.Errorf("out = %+v, responded = %v, requests = %+v", out, responded, reqs)
	}
}

func TestResponsesWorkAfterTheHandlerReturns(t *testing.T) {
	bot, requests := newTestBot(t)

	var held *Context
	var followup *FollowupMessage
	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		held = c
		if err := c.Reply("first"); err != nil {
			return err
		}
		var err error
		followup, err = c.Followup("second")
		return err
	}})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))

	if held.Err() == nil {
		t.Fatal("the handler context outlived the handler")
	}
	if err := held.Edit("first, edited"); err != nil {
		t.Errorf("Edit after return: %v", err)
	}
	if err := followup.Edit("second, edited"); err != nil {
		t.Errorf("followup Edit after return: %v", err)
	}
	if n := len(requests()); n != 4 {
		t.Errorf("%d requests", n)
	}
}

// inMemoryAPI answers every request with {"id":"sent"} without a network,
// which a synctest bubble needs: idle connections would block it forever.
type inMemoryAPI struct{}

func (inMemoryAPI) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"sent"}`)),
		Request:    r,
	}, nil
}

func TestDeferredWorkOutlivesTheCutoff(t *testing.T) {
	bot := New("token")
	bot.Session.HTTPClient = &http.Client{Transport: inMemoryAPI{}}

	var ctxErr, editErr, lateErr error
	bot.AddCommand(&Command{Name: "slow", Execute: func(c *Context) error {
		if err := c.DeferReply(false); err != nil {
			return err
		}
		time.Sleep(2 * responseWindow)
		ctxErr, editErr = c.Err(), c.Edit("done")
		return nil
	}})
	bot.AddCommand(&Command{Name: "late", Execute: func(c *Context) error {
		time.Sleep(2 * responseWindow)
		lateErr = c.Reply("too late")
		return nil
	}})

	synctest.Test(t, func(t *testing.T) {
		bot.handleInteraction(bot.Session, interaction(t, slashPayload("slow", "")))
		bot.handleInteraction(bot.Session, interaction(t, slashPayload("late", "")))
	})

	if ctxErr != nil || editErr != nil {
		t.Errorf("a deferred handler was cut off: ctx err %v, edit err %v", ctxErr, editErr)
	}
	if !errors.Is(lateErr, ErrNoResponse) {
		t.Errorf("late reply = %v, want ErrNoResponse", lateErr)
	}
}

func TestEditSendsOnlyWhatIsSet(t *testing.T) {
	bot, requests := newTestBot(t)

	file := &discord.File{Name: "new.png", Reader: strings.NewReader("x")}
	keep := []*discord.MessageAttachment{{ID: "111"}}
	bot.AddCommand(&Command{Name: "c", Execute: func(c *Context) error {
		if err := c.DeferReply(false); err != nil {
			return err
		}
		if err := c.Edit("text"); err != nil {
			return err
		}
		return c.EditWith(MessageEdit{Files: []*discord.File{file}, Attachments: keep})
	}})

	bot.handleInteraction(bot.Session, interaction(t, slashPayload("c", "")))

	reqs := requests()
	if _, sent := reqs[1].Body["flags"]; sent {
		t.Errorf("an edit without Flags sent flags, which would clear them: %s", reqs[1].Raw)
	}

	// The second edit is multipart; its JSON part names the kept attachment
	// and the new file by its index.
	raw := string(reqs[2].Raw)
	if !strings.Contains(raw, `"attachments":[{"id":"111"`) || !strings.Contains(raw, `{"id":"0","filename":"new.png"`) {
		t.Errorf("attachments = %s", raw)
	}
}
