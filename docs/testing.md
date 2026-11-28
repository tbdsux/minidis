# Testing handlers

The `minidistest` package runs a bot's handlers in a unit test, without a token or a connection to Discord. A `Kit` answers the bot's REST calls in memory and records them, and feeds the bot interactions, messages and other gateway events. They run through the bot's middleware and error handlers, as they would when Discord sends them.

```go
import (
	"testing"

	"github.com/tbdsux/minidis"
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/minidistest"
)

func TestPing(t *testing.T) {
	bot := newBot() // the bot of the program, with its commands and handlers
	kit := minidistest.New(t, bot.Session)

	res := kit.Interact(minidistest.Slash("ping"))

	if len(res) != 1 || res[0].Content != "pong" || res[0].Ephemeral {
		t.Errorf("responses = %+v", res)
	}
}
```

`minidistest.New` takes the `Session` of a `minidis.Bot`, an `interactions.Minidis` or a `client.Session` alone. It replaces the session's `HTTPClient`, so no request leaves the test, and makes the session ready as connecting would: `Session.User()` is the bot user and `c.Bot` is set in handlers. Add the handlers before calling it, as the `OnReady` ones run then.

## Interactions

The builders make an interaction from `minidistest.User()`, a member of the guild `minidistest.GuildID` without permissions, in the channel `minidistest.ChannelID`, with the locale `en-US`. The bot has every permission there.

| Builder                                     | Interaction                                              |
| ------------------------------------------- | -------------------------------------------------------- |
| `Slash(name, options...)`                   | A slash command                                          |
| `Option(name, value)`                       | An option: a string, bool, int or float64                |
| `Subcommand(name, options...)`              | A subcommand, as an option of `Slash`                    |
| `Autocomplete(name, options...)`            | Typing into the option made with `Focused(name, input)`  |
| `UserCommand(name, user)`                   | A user command                                           |
| `MessageCommand(name, message)`             | A message command                                        |
| `Component(customID, values...)`            | A button press, or a choice in a select menu with values |
| `ModalSubmit(customID, components...)`      | A modal being submitted, with its inputs filled in       |
| `DM(i)`                                     | `i` moved to the bot's DM with its user                  |

They return a `*discord.Interaction`, so change its fields for another user, guild, locale or permissions:

```go
i := minidistest.Slash("ban", minidistest.Option("reason", "spam"))
i.Locale = discord.French
i.Member.Permissions = discord.PermissionBanMembers

res := kit.Interact(i)
```

Other option types, such as users and channels, are a `discord.ApplicationCommandInteractionDataOption`, with what they name in the `Resolved` of the command's data.

`kit.Interact` returns once the interaction's handlers have, with every response they sent to it, in order. Each `Response` has a `Kind`:

| Kind       | Sent by                                                                                  |
| ---------- | ---------------------------------------------------------------------------------------- |
| `Callback` | The initial response, whose `Type` tells a reply, a deferral, an update, a modal or autocomplete choices apart |
| `Edit`     | `Edit` and `EditWith`, or an edit of a followup; `MessageID` is `"@original"` or the followup's ID |
| `Followup` | `Followup` and `FollowupWith`                                                            |
| `Delete`   | `Delete`, or a deletion of a followup                                                    |

and the `Content`, `Flags`, `Ephemeral`, `Embeds`, `Components` and `Choices` it sent, and the `CustomID` and `Title` of a modal:

```go
res := kit.Interact(minidistest.Slash("report"))

if res[0].Type != discord.InteractionResponseDeferredChannelMessageWithSource || !res[0].Ephemeral {
	t.Error("not deferred privately")
}
if res[1].Kind != minidistest.Edit || res[1].Content != "Report sent" {
	t.Errorf("edit = %+v", res[1])
}
```

A handler's error, or its panic, goes to the handler set with `HandleError`, as usual, which a test can set to see it. `kit.Responses(i)` returns the responses sent to `i` so far, for those a handler sends from a goroutine after it returns.

## Messages and events

`minidistest.Dispatch` sends any gateway event, typed as in `minidis.On`, and returns once its handlers have. `minidistest.Message(content)` makes a message from the same user, in the same guild and channel, for `OnMessage` and text commands:

```go
minidistest.Dispatch(kit, &discord.MessageCreate{Message: minidistest.Message("!echo hi")})
minidistest.Dispatch(kit, &discord.GuildMemberAdd{
	Member:  &discord.Member{User: &discord.User{ID: "9", Username: "ann"}},
	GuildID: minidistest.GuildID,
})
```

Clear the message's `GuildID` and `Member` for a DM. Errors go to `HandleEventError` and `HandleMessageError`.

## REST calls

`kit.Requests()` returns every REST call the bot made, in order, with its `Method`, its `Path` without the API version, such as `/channels/3000/messages`, its `Query` and its JSON `Body`, which `Decode` decodes:

```go
reqs := kit.Requests()

var sent struct {
	Content string `json:"content"`
}
if err := reqs[0].Decode(&sent); err != nil || sent.Content != "hi" {
	t.Errorf("sent %+v", sent)
}
```

A call is answered with its own body and an `id` added, which is what Discord answers when something is created. `GET /users/@me` gets the bot user and `DELETE` gets 204. `kit.Respond` sets the answer of a call, such as a member to look up or an error:

```go
kit.Respond("GET", "/guilds/"+minidistest.GuildID+"/members/9", 200, &discord.Member{Nick: "Ann"})
kit.Respond("POST", "/channels/"+minidistest.ChannelID+"/messages", 403, map[string]any{
	"code": 50013, "message": "Missing Permissions",
})
```

## Limits

- `Interact` and `Dispatch` return once the handlers have. A handler that waits for another interaction or message, such as with `WaitForComponent`, holds them up until it times out, and the kit cannot tell when it has begun waiting, so test such steps through handlers of their own.
- Interactions arrive as they do over the gateway, so the signature checks of `HTTPHandler` are not run. Its own tests cover them.
- REST calls fail only where `kit.Respond` sets an error.
