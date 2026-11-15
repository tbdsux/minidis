// Package minidis is a framework for Discord bots built on interactions:
// slash commands, context menu commands, components such as buttons and
// select menus, modals and autocomplete.
//
// A bot is created with New, given its commands with AddCommand,
// AddMessageCommand and AddUserCommand, and its handlers with HandleComponent,
// HandleModal, HandleChannelMessage and HandleError. SyncCommands registers
// the commands with Discord, and Run connects to the gateway until its context
// ends:
//
//	bot := minidis.New(os.Getenv("TOKEN"))
//	bot.AddCommand(&minidis.Command{
//		Name:        "ping",
//		Description: "Replies with pong.",
//		Execute: func(c *minidis.Context) error {
//			return c.Reply("pong")
//		},
//	})
//
//	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
//	defer stop()
//
//	if err := bot.SyncCommands(ctx); err != nil {
//		log.Fatal(err)
//	}
//	if err := bot.Run(ctx); err != nil {
//		log.Fatal(err)
//	}
//
// Every handler receives a Context, directly or inside a type that embeds it,
// such as ComponentContext. Its methods respond to the interaction (Reply,
// DeferReply, ReplyModal, then Edit and Followup) and keep working after the
// handler returns, until Discord stops accepting responses 15 minutes later.
// The Context is also a context.Context for the handler's own work, canceled
// when the handler returns, or with ErrNoResponse as its cause if nothing
// responded within the 3 seconds Discord allows. Middleware added with Use
// wraps every handler but autocomplete.
//
// Instead of the gateway, a bot can receive interactions over HTTP at the
// app's Interactions Endpoint URL, by serving HTTPHandler.
//
// The discord package holds the Discord API types, and discord/client the
// REST and gateway client, reachable as Minidis.Session for anything this
// package does not wrap.
package minidis
