package minidis

import (
	"fmt"
	"testing"

	"github.com/tbdsux/minidis/discord"
)

func TestModalValueHelpers(t *testing.T) {
	bot, _ := newTestBot(t)

	components := `[
		{"type":1,"components":[{"type":4,"custom_id":"legacy","value":"old style"}]},
		{"type":18,"component":{"type":4,"custom_id":"name","value":"Ann"}},
		{"type":18,"component":{"type":21,"custom_id":"size","value":"large"}},
		{"type":18,"component":{"type":21,"custom_id":"none","value":null}},
		{"type":18,"component":{"type":22,"custom_id":"toppings","values":["a","b"]}},
		{"type":18,"component":{"type":23,"custom_id":"agree","value":true}},
		{"type":18,"component":{"type":3,"custom_id":"lang","values":["go"]}},
		{"type":18,"component":{"type":19,"custom_id":"files","values":["f1","missing"]}}
	]`
	resolved := `{"attachments":{"f1":{"id":"f1","filename":"a.png"}}}`

	called := false
	bot.HandleModal("form", func(c *ModalSubmitContext) error {
		called = true

		values := map[string]string{"legacy": "old style", "name": "Ann", "size": "large", "none": "", "nope": ""}
		for id, want := range values {
			if got := c.Value(id); got != want {
				t.Errorf("Value(%q) = %q, want %q", id, got, want)
			}
		}

		if fmt.Sprint(c.Values("toppings")) != "[a b]" || fmt.Sprint(c.Values("lang")) != "[go]" || c.Values("name") != nil {
			t.Errorf("Values = %v %v %v", c.Values("toppings"), c.Values("lang"), c.Values("name"))
		}
		if !c.Checked("agree") || c.Checked("name") || c.Checked("nope") {
			t.Error("Checked is wrong")
		}
		if files := c.Attachments("files"); len(files) != 1 || files[0].ID != "f1" {
			t.Errorf("Attachments = %+v", files)
		}
		if _, ok := c.Component("size").(*discord.RadioGroup); !ok {
			t.Errorf("Component = %T", c.Component("size"))
		}
		return nil
	})

	bot.handleInteraction(bot.Session, interaction(t, `{"type":5,`+guildFields+`,"data":{"custom_id":"form","components":`+components+`,"resolved":`+resolved+`}}`))

	if !called {
		t.Error("modal handler did not run")
	}
}
