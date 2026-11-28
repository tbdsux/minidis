package minidistest_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/tbdsux/minidis"
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
	"github.com/tbdsux/minidis/interactions"
	"github.com/tbdsux/minidis/minidistest"
)

func TestSlashCommandRunsMiddleware(t *testing.T) {
	bot := minidis.New("token")

	var order []string
	bot.Use(func(next minidis.HandlerFunc) minidis.HandlerFunc {
		return func(c *minidis.Context) error {
			order = append(order, "middleware")
			return next(c)
		}
	})
	bot.AddCommand(&minidis.Command{Name: "greet", Execute: func(c *minidis.Context) error {
		order = append(order, "command")
		if c.Bot == nil || c.Bot.ID != minidistest.AppID || c.Locale != discord.French || c.Int("times") != 2 {
			t.Errorf("context = %+v", c)
		}
		return c.ReplyWith(minidis.Message{Content: "salut " + c.String("name"), Ephemeral: true})
	}})

	kit := minidistest.New(t, bot.Session)

	i := minidistest.Slash("greet", minidistest.Option("name", "Ann"), minidistest.Option("times", 2))
	i.Locale = discord.French
	res := kit.Interact(i)

	if len(order) != 2 || order[0] != "middleware" {
		t.Errorf("order = %v", order)
	}
	if len(res) != 1 || res[0].Kind != minidistest.Callback || res[0].Type != discord.InteractionResponseChannelMessageWithSource ||
		res[0].Content != "salut Ann" || !res[0].Ephemeral {
		t.Errorf("responses = %+v", res)
	}
}

func TestDeferEditFollowupAndErrors(t *testing.T) {
	bot := interactions.New("token")

	var got error
	bot.HandleError(func(c *interactions.Context, err error) { got = err })
	bot.AddCommand(&interactions.Command{Name: "work", Subcommands: []*interactions.Subcommand{{
		Name: "start",
		Execute: func(c *interactions.Context) error {
			if err := c.DeferReply(true); err != nil {
				return err
			}
			if err := c.Edit("done"); err != nil {
				return err
			}
			if _, err := c.Followup("more"); err != nil {
				return err
			}
			return errors.New("kaput")
		},
	}}})

	kit := minidistest.New(t, bot.Session)
	res := kit.Interact(minidistest.Slash("work", minidistest.Subcommand("start")))

	if got == nil || got.Error() != "kaput" {
		t.Errorf("error handler got %v", got)
	}
	if len(res) != 3 ||
		res[0].Type != discord.InteractionResponseDeferredChannelMessageWithSource || !res[0].Ephemeral ||
		res[1].Kind != minidistest.Edit || res[1].MessageID != "@original" || res[1].Content != "done" ||
		res[2].Kind != minidistest.Followup || res[2].Content != "more" {
		t.Errorf("responses = %+v", res)
	}
}

func TestModalComponentAutocompleteAndContextMenu(t *testing.T) {
	bot := minidis.New("token")
	bot.AddCommand(&minidis.Command{
		Name: "search",
		Execute: func(c *minidis.Context) error {
			return c.ReplyModal(minidis.Modal{CustomID: "form", Title: "Search", Components: []discord.MessageComponent{
				&discord.Label{Label: "Query", Component: &discord.TextInput{CustomID: "q", Style: discord.TextInputShort}},
			}})
		},
		Autocomplete: func(c *minidis.AutocompleteContext) error {
			return c.Respond(minidis.Choice(c.Input()+"!", c.Input()))
		},
	})
	bot.HandleModal("form", func(c *minidis.ModalSubmitContext) error { return c.Reply("searched " + c.Text("q")) })
	bot.HandleComponent("pick", func(c *minidis.ComponentContext) error { return c.Update("picked " + c.Data.Values[0]) })
	bot.AddUserCommand(&minidis.UserCommand{Name: "Wave", Execute: func(c *minidis.UserCommandContext) error {
		return c.Reply("waves at " + c.TargetUser.Username)
	}})

	kit := minidistest.New(t, bot.Session)

	tests := []struct {
		name string
		i    *discord.Interaction
		want func(*minidistest.Response) bool
	}{
		{"modal", minidistest.Slash("search"), func(r *minidistest.Response) bool {
			return r.Type == discord.InteractionResponseModal && r.CustomID == "form" && r.Title == "Search" && len(r.Components) == 1
		}},
		{"autocomplete", minidistest.Autocomplete("search", minidistest.Focused("query", "go")), func(r *minidistest.Response) bool {
			return r.Type == discord.InteractionApplicationCommandAutocompleteResult && len(r.Choices) == 1 && r.Choices[0].Name == "go!"
		}},
		{"modal submit", minidistest.ModalSubmit("form", &discord.Label{Component: &discord.TextInput{CustomID: "q", Value: "cats"}}), func(r *minidistest.Response) bool {
			return r.Type == discord.InteractionResponseChannelMessageWithSource && r.Content == "searched cats"
		}},
		{"select menu", minidistest.Component("pick", "red"), func(r *minidistest.Response) bool {
			return r.Type == discord.InteractionResponseUpdateMessage && r.Content == "picked red"
		}},
		{"user command in a DM", minidistest.DM(minidistest.UserCommand("Wave", &discord.User{ID: "9", Username: "ann"})), func(r *minidistest.Response) bool {
			return r.Content == "waves at ann"
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := kit.Interact(tt.i)
			if len(res) != 1 || !tt.want(res[0]) {
				t.Errorf("responses = %+v", res)
			}
		})
	}
}

func TestEventsAndMessagesRecordRequests(t *testing.T) {
	bot := minidis.New("token")
	bot.SetPrefix("!")

	var eventErr error
	bot.HandleEventError(func(c *minidis.EventContext, err error) { eventErr = err })
	bot.AddTextCommand(&minidis.TextCommand{Name: "echo", Execute: func(c *minidis.MessageContext) error {
		_, err := c.Reply(c.ArgString)
		return err
	}})
	minidis.On(bot, func(c *minidis.EventContext, e *discord.GuildMemberAdd) error {
		_, err := c.Session.ChannelMessageSend(c, minidistest.ChannelID, "welcome "+e.User.Username)
		return err
	})

	kit := minidistest.New(t, bot.Session)

	minidistest.Dispatch(kit, &discord.MessageCreate{Message: minidistest.Message("!echo hi there")})
	minidistest.Dispatch(kit, &discord.GuildMemberAdd{Member: &discord.Member{User: &discord.User{ID: "9", Username: "ann"}}, GuildID: minidistest.GuildID})

	reqs := kit.Requests()
	var reply, welcome struct {
		Content string `json:"content"`
	}
	if len(reqs) != 2 || reqs[0].Path != "/channels/"+minidistest.ChannelID+"/messages" ||
		reqs[0].Decode(&reply) != nil || reply.Content != "hi there" || reqs[1].Decode(&welcome) != nil || welcome.Content != "welcome ann" {
		t.Errorf("requests = %+v", reqs)
	}

	// A canned answer reaches the handler, and its error the error handler.
	kit.Respond(http.MethodPost, "/channels/"+minidistest.ChannelID+"/messages", http.StatusForbidden, map[string]any{"code": 50013, "message": "Missing Permissions"})
	minidistest.Dispatch(kit, &discord.GuildMemberAdd{Member: &discord.Member{User: &discord.User{ID: "9", Username: "ann"}}, GuildID: minidistest.GuildID})

	var apiErr *client.RESTError
	if !errors.As(eventErr, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("event error = %v", eventErr)
	}
}
