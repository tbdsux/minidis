package minidis

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// untilWaiting blocks until n message waiters are registered.
func untilWaiting(t *testing.T, b *Bot, n int) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for b.messageWaiters.Len() != n {
		if time.Now().After(deadline) {
			t.Fatalf("waiters = %d, want %d", b.messageWaiters.Len(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

type waited struct {
	c   *MessageContext
	err error
}

func waitAsync(b *Bot, ctx context.Context, filter func(*MessageContext) bool) <-chan waited {
	got := make(chan waited, 1)
	go func() {
		c, err := b.WaitForMessage(ctx, filter)
		got <- waited{c, err}
	}()
	return got
}

func saying(content string) func(*MessageContext) bool {
	return func(c *MessageContext) bool { return c.Content == content }
}

func TestWaitForMessage(t *testing.T) {
	bot, d := newTestBot(t)

	heard := make(chan string, 4)
	bot.OnMessage(func(c *MessageContext) error { heard <- c.Content; return nil })
	if err := bot.Open(t.Context()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	got := waitAsync(bot, ctx, saying("yes"))
	untilWaiting(t, bot, 1)

	// Other messages pass by, to the other handlers, and so do the bot's own.
	d.send("MESSAGE_CREATE", message("no"))
	own := message("yes")
	own["author"] = map[string]any{"id": "42", "bot": true}
	d.send("MESSAGE_CREATE", own)
	if got := recv(t, heard, "the other message"); got != "no" {
		t.Errorf("heard %q", got)
	}

	d.send("MESSAGE_CREATE", message("yes"))
	w := recv(t, got, "the answer")
	if w.err != nil || w.c.Content != "yes" || w.c.Author.ID != "u1" || w.c.Err() != nil {
		t.Fatalf("got %+v", w)
	}
	if _, err := w.c.Reply("ok"); err != nil {
		t.Fatal(err)
	}
	if got := recv(t, heard, "the answer by OnMessage"); got != "yes" {
		t.Errorf("heard %q", got)
	}

	cancel()
	recv(t, w.c.Done(), "the answer's context to end")
	if n := bot.messageWaiters.Len(); n != 0 {
		t.Errorf("waiters = %d", n)
	}
}

func TestWaitForMessageEnds(t *testing.T) {
	bot, _ := newTestBot(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if c, err := bot.WaitForMessage(ctx, nil); c != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("timeout: c = %v, err = %v", c, err)
	}

	want := messageIntents | discord.IntentsMessageContent
	if bot.Intents()&want != want {
		t.Errorf("intents = %#b, want %#b among them", bot.Intents(), want)
	}

	if err := bot.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := waitAsync(bot, t.Context(), nil)
	untilWaiting(t, bot, 1)
	if err := bot.Close(); err != nil {
		t.Fatal(err)
	}
	if w := recv(t, got, "the waiter"); !errors.Is(w.err, context.Canceled) {
		t.Errorf("close: err = %v", w.err)
	}
	if n := bot.messageWaiters.Len(); n != 0 {
		t.Errorf("waiters = %d", n)
	}
}

func TestWaitForMessageConcurrently(t *testing.T) {
	bot, d := newTestBot(t)
	bot.AddIntents(messageIntents | discord.IntentsMessageContent)
	if err := bot.Open(t.Context()); err != nil {
		t.Fatal(err)
	}

	const n = 10
	var waits []<-chan waited
	for i := range n {
		waits = append(waits, waitAsync(bot, t.Context(), saying(fmt.Sprint("a", i))))
	}
	untilWaiting(t, bot, n)

	for i := range n {
		d.send("MESSAGE_CREATE", message(fmt.Sprint("a", i)))
	}
	for i, got := range waits {
		if w := recv(t, got, "a waiter"); w.err != nil || w.c.Content != fmt.Sprint("a", i) {
			t.Errorf("waiter %d got %+v", i, w)
		}
	}
	if n := bot.messageWaiters.Len(); n != 0 {
		t.Errorf("waiters = %d", n)
	}
}
