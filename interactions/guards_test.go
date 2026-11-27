package interactions

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// memberSlash is a slash command run in a guild by userID, whose permissions
// in the channel are perms.
func memberSlash(name, userID, perms string) string {
	fields := strings.NewReplacer(`"id":"u1"`, `"id":"`+userID+`"`, `"permissions":"0"}`, `"permissions":"`+perms+`"}`).Replace(guildFields)
	return `{"type":2,` + fields + `,"data":{"name":"` + name + `","type":1}}`
}

// lastEphemeral returns the content of the last request, failing unless it is
// an ephemeral reply.
func lastEphemeral(t *testing.T, requests func() []recordedRequest) string {
	t.Helper()
	reqs := requests()
	if len(reqs) == 0 {
		t.Fatal("nothing was sent")
	}
	data, _ := reqs[len(reqs)-1].Body["data"].(map[string]any)
	if reqs[len(reqs)-1].Body["type"] != float64(4) || data["flags"] != float64(64) {
		t.Fatalf("last request = %+v, want an ephemeral reply", reqs[len(reqs)-1])
	}
	content, _ := data["content"].(string)
	return content
}

func TestCooldownLimitsEachCommandAndKey(t *testing.T) {
	bot, requests := newTestBot(t)
	bot.HandleError(func(_ *Context, err error) { t.Error(err) })

	ran := map[string]int{}
	handler := func(c *Context) error {
		ran[c.Interaction.ApplicationCommandData().Name+" "+c.Author.ID]++
		return nil
	}
	limit := Cooldown(time.Hour, PerUser)
	bot.AddCommand(&Command{Name: "a", Execute: limit(handler)})
	bot.AddCommand(&Command{Name: "b", Execute: limit(handler)})

	run := func(name, user string) {
		bot.handleInteraction(bot.Session, interaction(t, memberSlash(name, user, "0")))
	}

	run("a", "u1")
	run("a", "u1")
	if ran["a u1"] != 1 {
		t.Fatalf("a ran %d times for u1 within the cooldown, want 1", ran["a u1"])
	}
	if got := lastEphemeral(t, requests); !strings.Contains(got, "again <t:") {
		t.Errorf("cooldown reply = %q", got)
	}

	run("a", "u2") // another user
	run("b", "u1") // another command
	if ran["a u2"] != 1 || ran["b u1"] != 1 {
		t.Errorf("ran = %v; another user or command was limited", ran)
	}
}

func TestCooldownEnds(t *testing.T) {
	bot, _ := newTestBot(t)

	ran := 0
	bot.Use(Cooldown(20*time.Millisecond, PerGuild))
	bot.AddCommand(&Command{Name: "a", Execute: func(*Context) error { ran++; return nil }})

	bot.handleInteraction(bot.Session, interaction(t, memberSlash("a", "u1", "0")))
	time.Sleep(40 * time.Millisecond)
	bot.handleInteraction(bot.Session, interaction(t, memberSlash("a", "u2", "0")))

	if ran != 2 {
		t.Errorf("ran %d times, want 2 once the cooldown ended", ran)
	}
}

func TestCooldownLeavesComponentsAlone(t *testing.T) {
	bot, _ := newTestBot(t)

	ran := 0
	bot.Use(Cooldown(time.Hour, PerUser))
	bot.HandleComponent("b", func(*ComponentContext) error { ran++; return nil })

	bot.handleInteraction(bot.Session, interaction(t, componentPayload("b")))
	bot.handleInteraction(bot.Session, interaction(t, componentPayload("b")))

	if ran != 2 {
		t.Errorf("component ran %d times, want 2", ran)
	}
}

func TestRequirePermissions(t *testing.T) {
	need := discord.PermissionBanMembers | discord.PermissionKickMembers // 6

	for _, tc := range []struct {
		name    string
		payload string
		runs    bool
		reply   string
	}{
		{"missing one", memberSlash("ban", "u1", "4"), false, "permissions"},
		{"has all", memberSlash("ban", "u1", "6"), true, ""},
		{"administrator", memberSlash("ban", "u1", "8"), true, ""},
		{"in a DM", `{"type":2,"id":"i1","application_id":"app","token":"tok","channel_id":"dm","user":{"id":"u1","username":"name"},"data":{"name":"ban","type":1}}`, false, "server"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bot, requests := newTestBot(t)
			bot.HandleError(func(_ *Context, err error) { t.Error(err) })

			ran := false
			bot.AddCommand(&Command{Name: "ban", Execute: RequirePermissions(need)(func(*Context) error {
				ran = true
				return nil
			})})

			bot.handleInteraction(bot.Session, interaction(t, tc.payload))

			if ran != tc.runs {
				t.Fatalf("ran = %v, want %v", ran, tc.runs)
			}
			if !tc.runs {
				if got := lastEphemeral(t, requests); !strings.Contains(got, tc.reply) {
					t.Errorf("reply = %q, want it to mention %q", got, tc.reply)
				}
			}
		})
	}
}

func TestCooldownKeepsCommandTypesApart(t *testing.T) {
	bot, _ := newTestBot(t)

	ran := map[string]int{}
	bot.Use(Cooldown(time.Hour, PerUser))
	bot.AddCommand(&Command{Name: "info", Execute: func(*Context) error { ran["slash"]++; return nil }})
	bot.AddUserCommand(&UserCommand{Name: "info", Execute: func(*UserCommandContext) error { ran["user"]++; return nil }})

	bot.handleInteraction(bot.Session, interaction(t, memberSlash("info", "u1", "0")))
	bot.handleInteraction(bot.Session, interaction(t, `{"type":2,`+guildFields+`,"data":{"name":"info","type":2,"target_id":"t","resolved":{"users":{"t":{"id":"t","username":"ann"}}}}}`))

	if ran["slash"] != 1 || ran["user"] != 1 {
		t.Errorf("ran = %v; a context menu command shared the cooldown of a slash command of its name", ran)
	}
}

func TestCooldownOfZeroOrLessLimitsNothing(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		bot, _ := newTestBot(t)
		ran := 0
		bot.Use(Cooldown(d, PerUser))
		bot.AddCommand(&Command{Name: "a", Execute: func(*Context) error { ran++; return nil }})

		bot.handleInteraction(bot.Session, interaction(t, memberSlash("a", "u1", "0")))
		bot.handleInteraction(bot.Session, interaction(t, memberSlash("a", "u1", "0")))
		if ran != 2 {
			t.Errorf("d = %v: ran %d times, want 2", d, ran)
		}
	}
}

func TestCooldownTimeIsNeverInThePast(t *testing.T) {
	bot, requests := newTestBot(t)
	const d = 1500 * time.Millisecond
	bot.Use(Cooldown(d, PerUser))
	bot.AddCommand(&Command{Name: "a", Execute: func(*Context) error { return nil }})

	earliest := time.Now().Add(d) // the cooldown ends no sooner
	bot.handleInteraction(bot.Session, interaction(t, memberSlash("a", "u1", "0")))
	bot.handleInteraction(bot.Session, interaction(t, memberSlash("a", "u1", "0")))

	reply := lastEphemeral(t, requests)
	_, rest, _ := strings.Cut(reply, "<t:")
	secs, err := strconv.ParseInt(strings.TrimSuffix(rest, ":R>."), 10, 64)
	if err != nil {
		t.Fatalf("reply = %q", reply)
	}
	if shown := time.Unix(secs, 0); shown.Before(earliest) {
		t.Errorf("shown %v, before the cooldown ends at %v or later", shown, earliest)
	}
}

func TestCooldownNeedsAKey(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Cooldown with a nil key did not panic")
		}
	}()
	Cooldown(time.Second, nil)
}
