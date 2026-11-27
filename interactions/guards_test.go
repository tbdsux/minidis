package interactions

import (
	"strings"
	"testing"

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
