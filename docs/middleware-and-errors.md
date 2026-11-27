# Middleware and errors

## Middleware

Middleware wraps every command, component and modal handler, to log, check permissions, add values to the context, or stop a handler from running:

```go
bot.Use(func(next minidis.HandlerFunc) minidis.HandlerFunc {
	return func(c *minidis.Context) error {
		start := time.Now()
		err := next(c)
		log.Printf("interaction %s by %s took %s", c.ID, c.Author.Username, time.Since(start))
		return err
	}
})
```

The first middleware added runs first. Autocomplete is left out, as it runs on every keystroke, and so are messages and events, which are not interactions.

`c.WithContext` passes values on to the handler:

```go
type requestIDKey struct{}

bot.Use(func(next minidis.HandlerFunc) minidis.HandlerFunc {
	return func(c *minidis.Context) error {
		return next(c.WithContext(context.WithValue(c, requestIDKey{}, newRequestID())))
	}
})

// in a handler:
id := c.Value(requestIDKey{}).(string)
```

To stop a handler from running, respond and return without calling `next`:

```go
func adminsOnly(next minidis.HandlerFunc) minidis.HandlerFunc {
	return func(c *minidis.Context) error {
		if c.Member == nil || c.Member.Permissions&discord.PermissionAdministrator == 0 {
			return c.ReplyWith(minidis.Message{Content: "Admins only.", Ephemeral: true})
		}
		return next(c)
	}
}
```

### Permissions

`minidis.RequirePermissions` is built-in middleware that runs a handler only for members with the given permissions in the channel, and answers others with an ephemeral message:

```go
bot.AddCommand(&minidis.Command{
	Name:        "ban",
	Description: "Bans a member.",
	Execute:     minidis.RequirePermissions(discord.PermissionBanMembers)(ban), // this command alone
})
```

With `bot.Use` it covers every interaction. It answers in DMs that the command only works in a server. Unlike `DefaultMemberPermissions`, which server admins can change, it cannot be turned off from Discord.

Middleware sees the `Context` of every kind of interaction. Check `c.Interaction.Type` to tell a command (`discord.InteractionApplicationCommand`) from a component (`discord.InteractionMessageComponent`) or a modal (`discord.InteractionModalSubmit`) before reading its data: `c.Interaction.ApplicationCommandData()` and the others panic on another type.

## Interaction errors

An error returned by a handler or middleware, or a panic, goes to the function set with `bot.HandleError`. Without one, it is logged:

```go
bot.HandleError(func(c *minidis.Context, err error) {
	log.Println("interaction failed:", err)

	if c.Interaction.Type == discord.InteractionApplicationCommandAutocomplete {
		return // autocomplete cannot show a message
	}
	msg := minidis.Message{Content: "Something went wrong.", Ephemeral: true}
	if c.Responded() {
		c.FollowupWith(msg)
	} else {
		c.ReplyWith(msg)
	}
})
```

`c` is the copy that middleware passed on, with its values. A panic in the error handler is logged.

The errors to know:

| Error                    | Means                                                                                                       |
| ------------------------ | ----------------------------------------------------------------------------------------------------------- |
| `*minidis.PanicError`    | A handler panicked. It has the `Value` and the `Stack`, and `errors.Is` and `errors.As` see through it.      |
| `minidis.ErrNoHandler`   | Wrapped for a command or component that has no handler, such as one removed from the bot.                    |
| `minidis.ErrNoResponse`  | The cause of a context canceled because nothing responded within 3 seconds; read it with `context.Cause`. It also matches `context.Canceled`. |
| `*client.RESTError`      | Discord refused a request; see [The Discord client](./client.md#errors).                                    |

```go
var pe *minidis.PanicError
if errors.As(err, &pe) {
	log.Printf("panic: %v\n%s", pe.Value, pe.Stack)
}
```

## Events and messages

The other kinds of handlers have error handlers of their own, which also receive panics as a `*minidis.PanicError`:

| Handlers                                   | Errors go to                                                              |
| ------------------------------------------ | ------------------------------------------------------------------------- |
| `minidis.On`                               | `bot.HandleEventError`                                                    |
| `bot.OnMessage` and text commands          | `bot.HandleMessageError`, or `bot.HandleEventError` when it is not set    |
| `bot.HandleChannelMessage`                 | `bot.HandleChannelMessageError`                                           |

Each logs the error when no handler is set.

```go
bot.HandleMessageError(func(c *minidis.MessageContext, err error) {
	log.Printf("%s: %v", c.Content, err)
	c.Reply("Something went wrong.")
})
```

Next: [Receiving interactions over HTTP](./http.md).
