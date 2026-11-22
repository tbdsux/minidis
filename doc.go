// Package minidis is a simple, opinionated framework for Discord bots, built
// directly on the Discord API (https://docs.discord.com/developers/intro).
//
// A Bot handles slash commands and other interactions, messages, and every
// other gateway event, and runs with one call:
//
//	bot := minidis.New(os.Getenv("TOKEN"))
//
//	bot.AddCommand(&minidis.Command{
//		Name:        "ping",
//		Description: "Replies with pong.",
//		Execute: func(c *minidis.Context) error {
//			return c.Reply("pong")
//		},
//	})
//
//	minidis.On(bot, func(c *minidis.EventContext, e *discord.GuildMemberAdd) error {
//		log.Println(e.User.Username, "joined")
//		return nil
//	})
//
//	if err := bot.Start(); err != nil {
//		log.Fatal(err)
//	}
//
// Start, or Run with a context of your own, registers the commands with
// Discord, connects, and keeps the bot running until the program is
// interrupted. The bot asks Discord for the gateway intents its handlers
// need, so there are none to set.
//
// Every handler returns an error and receives a context that is also a
// context.Context for its own work: a Context for interactions, a
// MessageContext for messages and an EventContext for other events. Errors
// and panics go to the handlers set with HandleError, HandleMessageError and
// HandleEventError, and are logged by default.
//
// The framework is a layer over the module's other packages, which can be
// used without it:
//
//   - interactions is the framework for interactions alone. A Bot embeds its
//     bot, so everything documented there, such as components, modals,
//     autocomplete and middleware, is available on a Bot, and its types are
//     available under the same names here.
//   - discord/client is the REST and gateway client, reachable as
//     Bot.Session.
//   - discord holds the Discord API types.
package minidis
