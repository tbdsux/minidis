# Components and modals

## Buttons and select menus

Components go in a message's `Components`, inside `discord.ActionsRow`s of up to five buttons or one select menu. Each has a `CustomID`, which picks the handler that runs when it is used:

```go
bot.AddCommand(&minidis.Command{
	Name:        "poll",
	Description: "Ask a yes or no question.",
	Execute: func(c *minidis.Context) error {
		return c.ReplyWith(minidis.Message{
			Content: "Do you like Go?",
			Components: []discord.MessageComponent{
				discord.ActionsRow{Components: []discord.MessageComponent{
					discord.Button{Label: "Yes", Style: discord.SuccessButton, CustomID: "vote:yes"},
					discord.Button{Label: "No", Style: discord.DangerButton, CustomID: "vote:no"},
					discord.Button{Label: "Docs", Style: discord.LinkButton, URL: "https://go.dev"},
				}},
			},
		})
	},
})

bot.HandleComponent("vote:{answer}", func(c *minidis.ComponentContext) error {
	return c.Update("You voted " + c.Params["answer"]) // edits the message with the button
})
```

Link buttons have a `URL` instead of a custom ID, and run no handler. A select menu's chosen values are in `c.Data.Values`:

```go
discord.SelectMenu{
	CustomID:    "language",
	Placeholder: "Pick a language",
	Options: []discord.SelectMenuOption{
		{Label: "Go", Value: "go"},
		{Label: "Rust", Value: "rust"},
	},
}

bot.HandleComponent("language", func(c *minidis.ComponentContext) error {
	return c.Update("You picked " + c.Data.Values[0])
})
```

## Responding to a component

A component handler can respond like a command, with `Reply`, `ReplyModal` and the others, or change the message that holds the component:

| Method              | Does                                                                         |
| ------------------- | ---------------------------------------------------------------------------- |
| `c.Update(text)`    | Replaces the message's text.                                                 |
| `c.UpdateWith(edit)`| Changes the message with a `MessageEdit`, such as to disable the buttons.    |
| `c.DeferUpdate()`   | Acknowledges the click, for slow work; change the message later with `Edit`.  |

```go
// An empty, non-nil slice removes the buttons
return c.UpdateWith(minidis.MessageEdit{
	Content:    discord.String("Thanks for voting!"),
	Components: []discord.MessageComponent{},
})
```

Components keep working as long as the message exists, also after the bot restarts, so put what the handler needs into the custom ID rather than in memory.

## Waiting for a click

`bot.WaitForComponent` lets a command wait for the next use of a component, such as for a confirmation, instead of handling it elsewhere. It returns the click to respond to, or `ctx`'s error once `ctx` ends:

```go
bot.AddCommand(&minidis.Command{
	Name:        "reset",
	Description: "Reset your progress.",
	Execute: func(c *minidis.Context) error {
		id := "reset:" + c.ID // unique, so that other runs of the command do not take the click
		err := c.ReplyWith(minidis.Message{
			Content:   "Reset your progress?",
			Ephemeral: true,
			Components: []discord.MessageComponent{
				discord.ActionsRow{Components: []discord.MessageComponent{
					discord.Button{Label: "Reset", Style: discord.DangerButton, CustomID: id},
				}},
			},
		})
		if err != nil {
			return err
		}

		ctx, cancel := context.WithTimeout(c, time.Minute)
		defer cancel()

		click, err := bot.WaitForComponent(ctx, func(cc *minidis.ComponentContext) bool {
			return cc.Data.CustomID == id && cc.Author.ID == c.Author.ID
		})
		if err != nil {
			return c.EditWith(minidis.MessageEdit{Content: discord.String("Timed out."), Components: []discord.MessageComponent{}})
		}

		resetProgress(c.Author.ID)
		return click.UpdateWith(minidis.MessageEdit{Content: discord.String("Done."), Components: []discord.MessageComponent{}})
	},
})
```

The filter picks the clicks to take; the others go to their handlers as usual, and so does a click that comes after the wait ended, which is why the example removes the button then. A taken click goes through middleware but not to a handler, and must be responded to within 3 seconds like any other. It works for interactions from the gateway and [over HTTP](./http.md). The wait also ends, with `context.Canceled`, when the bot closes, and does not survive a restart.

## Handler IDs

A handler ID can capture `:`-separated segments with `{name}`, which end up in `c.Params`:

```go
bot.HandleComponent("ticket:close:{id}", closeTicket)    // exact segments win...
bot.HandleComponent("ticket:{action}:{id}", ticketAction) // ...over captures
```

An exact ID wins over a pattern, and a pattern with more fixed segments wins over one with fewer, so `ticket:close:42` runs `closeTicket`. A capture is one segment, so a value put there must not contain `:`. Custom IDs are limited to 100 characters.

`HandleComponentFallback` receives the IDs that no handler matches, such as those of messages sent by an older version of the bot. Without it, such interactions reach the [error handler](./middleware-and-errors.md) as `minidis.ErrNoHandler`.

## Modals

A modal is a form. A command or component opens one with `ReplyModal`, as its first response:

```go
bot.AddCommand(&minidis.Command{
	Name:        "feedback",
	Description: "Send feedback.",
	Execute: func(c *minidis.Context) error {
		return c.ReplyModal(minidis.Modal{
			CustomID: "feedback",
			Title:    "Feedback",
			Components: []discord.MessageComponent{
				discord.Label{
					Label: "What do you think?",
					Component: discord.TextInput{
						CustomID:  "opinion",
						Style:     discord.TextInputParagraph,
						MaxLength: 1000,
					},
				},
				discord.Label{
					Label:     "May we contact you?",
					Component: discord.Checkbox{CustomID: "contact"},
				},
			},
		})
	},
})

bot.HandleModal("feedback", func(c *minidis.ModalSubmitContext) error {
	log.Println(c.Author.Username, "said", c.Text("opinion"), "contact:", c.Checked("contact"))
	return c.ReplyWith(minidis.Message{Content: "Thanks!", Ephemeral: true})
})
```

Each input goes in a `discord.Label`, which gives it a title. Modal IDs match handlers like component IDs do, with captures in `c.Params`, and `HandleModalFallback` receives the rest.

The submitted values are read by the custom IDs of their components:

| Component                                         | Read with                     |
| ------------------------------------------------- | ----------------------------- |
| `discord.TextInput`                               | `c.Text`                      |
| `discord.RadioGroup`                              | `c.Text`, the chosen value    |
| `discord.SelectMenu`, `discord.CheckboxGroup`     | `c.Selected`, the chosen values |
| `discord.Checkbox`                                | `c.Checked`                   |
| `discord.FileUpload`                              | `c.Attachments`, or `c.Selected` for their IDs |

`c.Component(id)` returns the submitted component itself. See the [modals example](../example/modals/main.go) for every kind of input.

A modal handler responds like a command. A modal opened from a component can also change that component's message with `c.Update`, `c.UpdateWith` and `c.DeferUpdate`.

Next: [Autocomplete](./autocomplete.md).
