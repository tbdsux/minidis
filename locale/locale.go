// Package locale translates a bot's replies into the languages of Discord.
//
// A Catalog holds messages by locale and key. Set it on the bot with
// SetCatalog, and handlers look messages up with c.T, in the language of the
// user, else of the guild, else the catalog's default:
//
//	cat := locale.New(discord.EnglishUS).
//		Add(discord.EnglishUS, map[string]string{"greet": "Hello, %s!"}).
//		Add(discord.French, map[string]string{"greet": "Bonjour, %s !"})
//	bot.SetCatalog(cat)
//
//	return c.Reply(c.T("greet", c.Author.Username))
package locale

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/tbdsux/minidis/discord"
)

// Catalog holds translated messages. It is safe for concurrent use, so
// messages may be added while the bot runs. A nil Catalog has no messages.
type Catalog struct {
	def discord.Locale

	mu       sync.RWMutex
	messages map[discord.Locale]map[string]string
}

// New creates an empty catalog whose messages in def are used when a
// message is missing in the language asked for.
func New(def discord.Locale) *Catalog {
	return &Catalog{def: def, messages: map[discord.Locale]map[string]string{}}
}

// Default returns the locale given to New.
func (c *Catalog) Default() discord.Locale {
	if c == nil {
		return ""
	}
	return c.def
}

// Add adds the messages of a locale, by key, replacing those with the same
// keys. A message may hold fmt verbs, such as %s, filled from the arguments of
// T; use explicit indexes, such as %[2]s, where a language orders them
// differently. It returns c, so that calls can be chained.
func (c *Catalog) Add(l discord.Locale, messages map[string]string) *Catalog {
	c.mu.Lock()
	defer c.mu.Unlock()

	m := c.messages[l]
	if m == nil {
		m = make(map[string]string, len(messages))
		c.messages[l] = m
	}
	for k, v := range messages {
		m[k] = v
	}
	return c
}

// LoadFS adds the JSON files of fsys, such as an embed.FS, that match
// pattern, in the syntax of path.Match. Each file is named after its locale,
// such as fr.json or pt-BR.json, and holds one object of messages by key:
//
//	{"greet": "Bonjour, %s !"}
//
// A file named after no Discord locale is an error, as Discord would never ask
// for it.
func (c *Catalog) LoadFS(fsys fs.FS, pattern string) error {
	files, err := fs.Glob(fsys, pattern)
	if err != nil {
		return fmt.Errorf("locale: %w", err)
	}
	for _, name := range files {
		l := discord.Locale(strings.TrimSuffix(path.Base(name), path.Ext(name)))
		if !known[l] {
			return fmt.Errorf("locale: %s is not named after a Discord locale, such as fr.json or pt-BR.json", name)
		}

		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("locale: %w", err)
		}
		var messages map[string]string
		if err := json.Unmarshal(data, &messages); err != nil {
			return fmt.Errorf("locale: %s: %w", name, err)
		}
		c.Add(l, messages)
	}
	return nil
}

// Lookup returns the message for key in the first of locales that has it,
// else in the default locale. Empty locales are skipped.
func (c *Catalog) Lookup(key string, locales ...discord.Locale) (string, bool) {
	if c == nil {
		return "", false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, l := range locales {
		if msg, ok := c.messages[l][key]; ok && l != "" {
			return msg, true
		}
	}
	msg, ok := c.messages[c.def][key]
	return msg, ok
}

// T returns the message for key in l, else in the default locale, else key
// itself, formatted with args like fmt.Sprintf.
func (c *Catalog) T(l discord.Locale, key string, args ...any) string {
	msg, ok := c.Lookup(key, l)
	if !ok {
		msg = key
	}
	return Format(msg, args...)
}

// Translations returns the message for key in each locale that has it, or nil
// if none has it. It is what command localizations take.
func (c *Catalog) Translations(key string) map[discord.Locale]string {
	if c == nil {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	var out map[discord.Locale]string
	for l, m := range c.messages {
		if msg, ok := m[key]; ok {
			if out == nil {
				out = map[discord.Locale]string{}
			}
			out[l] = msg
		}
	}
	return out
}

// Format fills msg with args like fmt.Sprintf. Without args, msg is returned
// as it is, so that a % in it needs no escaping.
func Format(msg string, args ...any) string {
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}

// known are the locales Discord sends.
var known = map[discord.Locale]bool{
	discord.Indonesian: true, discord.Danish: true, discord.German: true,
	discord.EnglishGB: true, discord.EnglishUS: true, discord.SpanishES: true,
	discord.SpanishLATAM: true, discord.French: true, discord.Croatian: true,
	discord.Italian: true, discord.Lithuanian: true, discord.Hungarian: true,
	discord.Dutch: true, discord.Norwegian: true, discord.Polish: true,
	discord.PortugueseBR: true, discord.Romanian: true, discord.Finnish: true,
	discord.Swedish: true, discord.Vietnamese: true, discord.Turkish: true,
	discord.Czech: true, discord.Greek: true, discord.Bulgarian: true,
	discord.Russian: true, discord.Ukrainian: true, discord.Hindi: true,
	discord.Thai: true, discord.ChineseCN: true, discord.ChineseTW: true,
	discord.Japanese: true, discord.Korean: true,
}
