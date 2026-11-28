package minidis

import (
	"testing"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/locale"
)

func TestMessageContextT(t *testing.T) {
	b := New("token")
	c := newMessageContext(&EventContext{Bot: b, Session: b.Session}, &discord.Message{GuildID: "g", Content: "hi"})

	if got := c.T("greet %s", "ann"); got != "greet ann" {
		t.Errorf("T without a catalog = %q, want the key", got)
	}

	// Without the guild in the state, the default locale is used.
	b.SetCatalog(locale.New(discord.French).Add(discord.French, map[string]string{"greet %s": "Bonjour, %s"}))
	if got := c.T("greet %s", "ann"); got != "Bonjour, ann" {
		t.Errorf("T = %q", got)
	}
}
