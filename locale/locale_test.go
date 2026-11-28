package locale

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tbdsux/minidis/discord"
)

func TestFallbackChain(t *testing.T) {
	cat := New(discord.EnglishUS).
		Add(discord.EnglishUS, map[string]string{"a": "en a", "b": "en b", "c": "en c"}).
		Add(discord.French, map[string]string{"a": "fr a"}).
		Add(discord.German, map[string]string{"a": "de a", "b": "de b"})

	for _, tt := range []struct {
		key     string
		locales []discord.Locale
		want    string
	}{
		{"a", []discord.Locale{discord.French, discord.German}, "fr a"},
		{"b", []discord.Locale{discord.French, discord.German}, "de b"}, // the guild's
		{"c", []discord.Locale{discord.French, discord.German}, "en c"}, // the default
		{"a", []discord.Locale{"", discord.German}, "de a"},
		{"a", nil, "en a"},
	} {
		if got, _ := cat.Lookup(tt.key, tt.locales...); got != tt.want {
			t.Errorf("Lookup(%q, %v) = %q, want %q", tt.key, tt.locales, got, tt.want)
		}
	}
	if _, ok := cat.Lookup("missing", discord.French); ok {
		t.Error("Lookup found a missing key")
	}
}

func TestT(t *testing.T) {
	cat := New(discord.EnglishUS).Add(discord.French, map[string]string{
		"greet": "Bonjour, %s !",
		"order": "%[2]s %[1]s",
		"pct":   "100%",
	})

	for _, tt := range []struct{ got, want string }{
		{cat.T(discord.French, "greet", "ann"), "Bonjour, ann !"},
		{cat.T(discord.French, "order", "a", "b"), "b a"},
		{cat.T(discord.French, "pct"), "100%"},            // no args, no formatting
		{cat.T(discord.German, "Hi %s", "ann"), "Hi ann"}, // the key itself
		{(*Catalog)(nil).T(discord.French, "greet %d", 1), "greet 1"},
	} {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestTranslations(t *testing.T) {
	cat := New(discord.EnglishUS).
		Add(discord.French, map[string]string{"k": "fr"}).
		Add(discord.German, map[string]string{"other": "de"})

	if got := fmt.Sprint(cat.Translations("k")); got != "map[fr:fr]" {
		t.Errorf("Translations = %s", got)
	}
	if got := cat.Translations("none"); got != nil {
		t.Errorf("Translations of a missing key = %v, want nil", got)
	}
}

func TestLoadFS(t *testing.T) {
	cat := New(discord.EnglishUS)
	err := cat.LoadFS(fstest.MapFS{
		"locales/fr.json":    {Data: []byte(`{"greet": "Bonjour"}`)},
		"locales/pt-BR.json": {Data: []byte(`{"greet": "Olá"}`)},
		"other/de.json":      {Data: []byte(`{"greet": "Hallo"}`)},
	}, "locales/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if cat.T(discord.French, "greet") != "Bonjour" || cat.T(discord.PortugueseBR, "greet") != "Olá" {
		t.Errorf("messages not loaded: %v", cat.Translations("greet"))
	}
	if _, ok := cat.Lookup("greet", discord.German); ok {
		t.Error("loaded a file the pattern does not match")
	}

	for name, data := range map[string]string{
		"pt_BR.json": `{}`,
		"fr.json":    `{"greet": 1}`,
	} {
		err := New(discord.EnglishUS).LoadFS(fstest.MapFS{name: {Data: []byte(data)}}, "*.json")
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("LoadFS(%s) = %v, want an error naming the file", name, err)
		}
	}
}
