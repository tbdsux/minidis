package interactions

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/locale"
)

// localized is a slash command run in a guild by a user with the given
// languages and permissions.
func localized(name, userLocale, guildLocale, perms string) string {
	return strings.Replace(memberSlash(name, "u1", perms), `{"type":2,`,
		`{"type":2,"locale":"`+userLocale+`","guild_locale":"`+guildLocale+`",`, 1)
}

func TestContextT(t *testing.T) {
	bot, _ := newTestBot(t)
	bot.HandleError(func(_ *Context, err error) { t.Error(err) })
	bot.SetCatalog(locale.New(discord.EnglishUS).
		Add(discord.EnglishUS, map[string]string{"a": "en a", "b": "en b", "c": "en c"}).
		Add(discord.German, map[string]string{"a": "de a", "b": "de b"}).
		Add(discord.French, map[string]string{"a": "fr %s"}))

	var got []string
	bot.AddCommand(&Command{Name: "t", Execute: func(c *Context) error {
		got = append(got, c.T("a", "x"), c.T("b"), c.T("c"), c.T("missing %d", 1))
		return nil
	}})
	bot.handleInteraction(bot.Session, interaction(t, localized("t", "fr", "de", "0")))

	if want := "fr x|de b|en c|missing 1"; strings.Join(got, "|") != want {
		t.Errorf("T = %q, want user, guild, default, then key: %q", strings.Join(got, "|"), want)
	}
}

func TestGuardsUseTheCatalog(t *testing.T) {
	bot, requests := newTestBot(t)
	bot.SetCatalog(locale.New(discord.EnglishUS).Add(discord.French, map[string]string{
		KeyCooldown:           "Réessaie %s.",
		KeyMissingPermissions: "Permission refusée.",
	}))
	bot.AddCommand(&Command{Name: "slow", Execute: Cooldown(time.Hour, PerUser)(func(*Context) error { return nil })})
	bot.AddCommand(&Command{Name: "ban", Execute: RequirePermissions(discord.PermissionBanMembers)(func(*Context) error { return nil })})

	bot.handleInteraction(bot.Session, interaction(t, localized("slow", "fr", "", "0")))
	bot.handleInteraction(bot.Session, interaction(t, localized("slow", "fr", "", "0")))
	if got := lastEphemeral(t, requests); !strings.HasPrefix(got, "Réessaie <t:") || !strings.HasSuffix(got, ":R>.") {
		t.Errorf("cooldown reply = %q", got)
	}

	bot.handleInteraction(bot.Session, interaction(t, localized("ban", "fr", "", "0")))
	if got := lastEphemeral(t, requests); got != "Permission refusée." {
		t.Errorf("permissions reply = %q", got)
	}

	// A language the catalog lacks keeps the English reply.
	bot.handleInteraction(bot.Session, interaction(t, localized("ban", "de", "", "0")))
	if got := lastEphemeral(t, requests); got != "You don't have the permissions to use this." {
		t.Errorf("permissions reply = %q", got)
	}
}

func TestSyncFillsLocalizationsFromTheCatalog(t *testing.T) {
	bot, requests := newTestBot(t)
	bot.SetCatalog(locale.New(discord.EnglishUS).Add(discord.French, map[string]string{
		"command.c.name":               "fc",
		"command.c.description":        "fd",
		"command.c.s.name":             "fs",
		"command.c.s.o.description":    "fo",
		"command.c.s.o.red.name":       "rouge",
		"command.m.name":               "fm",
		"command.m.description":        "never sent",
		"command.explicit.name":        "from the catalog",
		"command.explicit.description": "fe",
	}).Add(discord.German, map[string]string{"command.explicit.name": "de"}))

	choice := &discord.ApplicationCommandOptionChoice{Name: "red", Value: "r"}
	option := &discord.ApplicationCommandOption{Name: "o", Description: "d", Type: discord.ApplicationCommandOptionString,
		Choices: []*discord.ApplicationCommandOptionChoice{choice}}
	explicit := map[discord.Locale]string{discord.French: "fx"}
	bot.AddCommand(&Command{Name: "c", Description: "d",
		Subcommands: []*Subcommand{{Name: "s", Description: "d", Options: []*discord.ApplicationCommandOption{option}}}})
	bot.AddCommand(&Command{Name: "explicit", Description: "d", NameLocalizations: explicit})
	bot.AddMessageCommand(&MessageCommand{Name: "m"})

	if err := bot.SyncCommands(t.Context(), "g"); err != nil {
		t.Fatal(err)
	}

	cmds := commandByName(t, requests()[0].Raw)
	sub := cmds["c"]["options"].([]any)[0].(map[string]any)
	opt := sub["options"].([]any)[0].(map[string]any)
	for _, tt := range []struct {
		what string
		got  any
		want string
	}{
		{"command name", cmds["c"]["name_localizations"], "map[fr:fc]"},
		{"command description", cmds["c"]["description_localizations"], "map[fr:fd]"},
		{"subcommand name", sub["name_localizations"], "map[fr:fs]"},
		{"option description", opt["description_localizations"], "map[fr:fo]"},
		{"choice name", opt["choices"].([]any)[0].(map[string]any)["name_localizations"], "map[fr:rouge]"},
		{"menu name", cmds["m"]["name_localizations"], "map[fr:fm]"},
		{"menu description", cmds["m"]["description_localizations"], "<nil>"},
		{"explicit over catalog", cmds["explicit"]["name_localizations"], "map[de:de fr:fx]"},
	} {
		if fmt.Sprint(tt.got) != tt.want {
			t.Errorf("%s = %v, want %s", tt.what, tt.got, tt.want)
		}
	}

	if option.DescriptionLocalizations != nil || choice.NameLocalizations != nil || len(explicit) != 1 {
		t.Errorf("sync changed the command's own values: %+v %+v %v", option, choice, explicit)
	}
}
