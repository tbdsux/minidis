package minidis

import (
	"fmt"
	"testing"

	"github.com/tbdsux/minidis/discord"
)

func TestSyncSendsNSFWAndLocalizations(t *testing.T) {
	bot, requests := newTestBot(t)

	fr := map[discord.Locale]string{discord.French: "fr"}
	bot.AddCommand(&Command{
		Name: "c", Description: "d", NSFW: true,
		Subcommands: []*Subcommand{{Name: "s", Description: "d", NameLocalizations: fr}},
		Groups:      []*SubcommandGroup{{Name: "g", Description: "d", DescriptionLocalizations: fr}},
	})
	bot.AddCommand(&Command{Name: "safe", Description: "d"})
	bot.AddUserCommand(&UserCommand{Name: "u", NSFW: true, NameLocalizations: fr})
	bot.AddMessageCommand(&MessageCommand{Name: "m", NameLocalizations: fr})

	if err := bot.SyncCommands(t.Context(), "g"); err != nil {
		t.Fatal(err)
	}

	cmds := commandByName(t, requests()[0].Raw)
	if cmds["c"]["nsfw"] != true || cmds["u"]["nsfw"] != true {
		t.Errorf("nsfw = %v %v", cmds["c"], cmds["u"])
	}
	if _, sent := cmds["safe"]["nsfw"]; sent {
		t.Errorf("nsfw must be left out unless set: %v", cmds["safe"])
	}
	if fmt.Sprint(cmds["u"]["name_localizations"]) != "map[fr:fr]" || fmt.Sprint(cmds["m"]["name_localizations"]) != "map[fr:fr]" {
		t.Errorf("menu localizations = %v %v", cmds["u"], cmds["m"])
	}

	opts := cmds["c"]["options"].([]any)
	if fmt.Sprint(opts[0].(map[string]any)["name_localizations"]) != "map[fr:fr]" ||
		fmt.Sprint(opts[1].(map[string]any)["description_localizations"]) != "map[fr:fr]" {
		t.Errorf("option localizations = %v", opts)
	}
}
