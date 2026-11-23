# Responding to interactions

Every command, component and modal handler gets a `Context`, directly or embedded, as in `ComponentContext`. Its methods respond to the interaction.

Discord gives a bot **3 seconds** to respond, and then **15 minutes** to edit the response and send followups.

## Replying

```go
c.Reply("pong")
c.ReplyWith(minidis.Message{Content: "only you can see this", Ephemeral: true})
```

A `Message` has `Content`, `Embeds`, `Components`, `Files`, `AllowedMentions`, a `Poll`, `TTS`, `Ephemeral` and further `Flags`:

```go
c.ReplyWith(minidis.Message{
	Embeds: []*discord.MessageEmbed{{Title: "Report", Description: "All good.", Color: 0x57F287}},
	Files:  []*discord.File{{Name: "report.txt", Reader: strings.NewReader(report)}},
})
```

Ephemeral messages are seen only by the user who caused the interaction, and are dismissed by them. When the content repeats what users wrote, set `AllowedMentions` to `&discord.MessageAllowedMentions{}` so that it cannot ping anyone.

## Taking longer than 3 seconds

Defer first, then edit the response when the work is done:

```go
c.DeferReply(false) // true makes the loading state and the final message ephemeral
result := slowWork(c)
c.Edit(result)
```

Without a response after 3 seconds, Discord shows the user that the interaction failed, and the handler's context is canceled with `minidis.ErrNoResponse` as its cause.

## Editing and deleting

```go
c.Edit("done")
c.EditWith(minidis.MessageEdit{Content: discord.String("done"), Components: []discord.MessageComponent{}})
c.Delete()
```

In a `MessageEdit`, a nil field is left unchanged and an empty one clears it, such as an empty `Components` to remove the buttons. A slice built with `append` that ends up empty is still nil, so start from an empty slice when the result should clear.

- `Files` are added to the message. `Attachments`, when not nil, lists the existing files to keep, and an empty non-nil slice removes them.
- `Flags` replaces the message's flags, so include those to keep. Only `discord.MessageFlagsSuppressEmbeds` and `discord.MessageFlagsIsComponentsV2` can be set.
- A `Poll` can only be added when editing a deferred response.

`c.Response()` fetches the message that was sent, such as to learn its ID.

## Followups

A followup is another message after the response, up to 15 minutes later:

```go
f, err := c.Followup("one more thing")
// later:
f.Edit("one more thing, edited")
f.Delete()
```

`FollowupWith` takes a `Message`, and can be ephemeral on its own.

`c.Responded()` reports whether the interaction has been responded to, with a reply, a deferral, a modal or an update, so that a message goes through `Followup` instead of `Reply` afterwards. A response Discord refused does not count.

## Opening a modal

A command or component can respond by showing a form:

```go
c.ReplyModal(minidis.Modal{CustomID: "feedback", Title: "Feedback", Components: ...})
```

A modal must be the first response, so it cannot follow a deferral. See [Components and modals](./components-and-modals.md#modals).

## Components V2 layouts

Set `discord.MessageFlagsIsComponentsV2` to lay a message out with components only, such as `discord.Container`, `discord.Section`, `discord.TextDisplay`, `discord.MediaGallery` and `discord.Separator`. `Content`, `Embeds` and `Poll` must then be empty:

```go
c.ReplyWith(minidis.Message{
	Flags: discord.MessageFlagsIsComponentsV2,
	Components: []discord.MessageComponent{
		discord.Container{Components: []discord.MessageComponent{
			discord.TextDisplay{Content: "## Server status"},
			discord.TextDisplay{Content: "Everything is running."},
		}},
	},
})
```

A deferred reply becomes one through `c.EditWith(minidis.MessageEdit{Flags: discord.MessageFlagsIsComponentsV2, Components: ...})`, which clears its text, embeds and poll. Turning Components V2 on cannot be undone.

## The handler's context

Every handler context is also a `context.Context` for the handler's own work. It is canceled:

- when the handler returns;
- 15 minutes after the interaction arrived, when Discord stops accepting responses;
- after 3 seconds if nothing responded by then, with `minidis.ErrNoResponse` as its cause;
- when a closing bot stops waiting for its handlers.

Pass it to the calls the handler makes, including those on `c.Session`:

```go
rows, err := db.QueryContext(c, "SELECT ...")
_, err = c.Session.ChannelMessageSend(c, logChannelID, "ran /ping")
```

The response methods (`Reply`, `Edit`, `Followup` and the rest) keep working after the handler returns, until Discord's 15 minutes are up or the bot closes, so a followup can be edited later, such as from a goroutine the handler started.

Respond through these methods rather than `c.Session`: a response sent another way is not seen, so the context is canceled after 3 seconds. For a response the methods do not cover, `c.SendResponse` sends a `discord.InteractionResponse` built by hand and returns what Discord reports about it.

Next: [Components and modals](./components-and-modals.md).
