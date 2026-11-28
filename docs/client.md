# The Discord client

`discord/client` is the whole Discord API for bots: REST and the gateway. A bot made with `minidis.New` or `interactions.New` has its client as `bot.Session`, and every handler context has it as `c.Session`. It also works on its own, without the frameworks:

```go
s := client.New("Bot " + os.Getenv("TOKEN")) // the full Authorization value
s.Intents = discord.IntentsGuilds | discord.IntentsGuildMessages | discord.IntentsMessageContent

s.OnMessageCreate(func(s *client.Session, e *discord.MessageCreate) {
	if e.Content == "!ping" {
		s.ChannelMessageSend(context.Background(), e.ChannelID, "pong")
	}
})

if err := s.Open(ctx); err != nil {
	log.Fatal(err)
}
defer s.Close()

select {
case <-ctx.Done():
case <-s.Done(): // Discord ended the connection for good
	log.Println(s.Err())
}
```

See the [client example](../example/client/main.go).

## REST

Every method takes a `context.Context` first. They cover:

- messages, reactions, pins, polls and crossposting;
- channels, permission overwrites, computed permissions, voice channel statuses, invites, typing and followed announcement channels;
- threads and forum posts, their members and the archived ones;
- guilds, members (nicknames, roles, timeouts, moving between voice channels), roles, bans, pruning, the audit log, integrations, the widget, the welcome screen, onboarding and templates;
- webhooks and the messages they send;
- emojis of guilds and of the app, stickers and soundboard sounds;
- auto moderation rules, scheduled events and stage instances;
- the bot's user and application, application commands, interaction responses, linked role metadata, SKUs, entitlements and subscriptions.

The names start with what they act on, such as `ChannelMessageSend`, `GuildMemberRoleAdd` or `GuildBanCreate`; browse them on [pkg.go.dev](https://pkg.go.dev/github.com/tbdsux/minidis/discord/client#Session).

```go
s.GuildMemberTimeout(ctx, guildID, userID, time.Now().Add(time.Hour))
s.ChannelEdit(ctx, channelID, &discord.ChannelParams{Name: "general", ParentID: discord.Null[string]()})
s.GuildBanCreate(client.WithAuditLogReason(ctx, "spam"), guildID, userID, 0)
```

`client.WithAuditLogReason` attaches a reason that shows in the server's audit log to the requests made with that context.

### Every page

`ChannelMessagesAll`, `ChannelPinsAll`, `GuildMembersAll`, `GuildBansAll`, `GuildAuditLogAll`, `GuildScheduledEventUsersAll`, `CurrentUserGuildsAll`, `ThreadMembersAll`, `MessageReactionsAll` and `PollAnswerVotersAll` go through a whole list, fetching the next page as the loop reaches it. Breaking out of the loop stops the fetching, and a page that fails ends it with the error:

```go
for m, err := range s.GuildMembersAll(ctx, guildID) {
	if err != nil {
		return err
	}
	fmt.Println(m.DisplayName())
}
```

### Optional fields

Fields that Discord clears with `null` are a `discord.Optional`: leave it unset to keep the value, or use `discord.Some(v)` to set it and `discord.Null[T]()` to clear it. Other pointer fields take `discord.String`, `discord.Int64` and `discord.Bool`.

### Any endpoint

`Session.Request` reaches an endpoint without a typed method, with the same rate limiting:

```go
var channels []*discord.Channel
err := s.Request(ctx, "GET", "/guilds/"+guildID+"/channels", nil, &channels)
```

The path is relative to the versioned base URL, and the body and result are JSON.

### Rate limits

The client keeps to Discord's rate limits by itself: requests wait for their bucket, and stay under the global limit of 50 a second. A 429 response is waited out and retried, and so is a 502, 503 or 504 for requests that are safe to repeat. A canceled context stops the waiting.

It also counts the 401, 403 and 429 responses that get an IP banned by Discord for a while, and logs a warning as they add up.

### Metrics

`Session.RequestHook`, if set, is called after every attempt at a request, retries included, with a `client.RequestInfo`: the method, the `Route` with its IDs and tokens replaced (such as `/channels/:id/messages`), the status (0 if no response came back), the latency, the attempt number, whether it got a 429, how long it waited for its rate limit before it was sent, and the error if no response came back. The library depends on no metrics package, so this is where one plugs in, such as Prometheus:

```go
requests := prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name: "discord_rest_request_seconds",
}, []string{"method", "route", "status"})
prometheus.MustRegister(requests)

s.RequestHook = func(info client.RequestInfo) {
	requests.WithLabelValues(info.Method, info.Route, strconv.Itoa(info.Status)).
		Observe(info.Latency.Seconds())
}
```

The hook runs inline, before the request returns or is retried, so it must be quick: a slow hook slows every request. It may be called from several goroutines at once. Set it before making requests.

### Errors

A response Discord refused is a `*client.RESTError`, with the `StatusCode`, Discord's error `Code` and `Message`, and the per-field `Errors`:

```go
_, err := s.ChannelMessageSend(ctx, channelID, "hi")

var re *client.RESTError
if errors.As(err, &re) && re.Code == 50013 { // Missing Permissions
	log.Println("cannot post in", channelID)
}
```

Discord's error codes are listed in its documentation, under Opcodes and Status Codes.

## Gateway

There is an `On…` method for every gateway event, such as `OnMessageCreate`, `OnGuildMemberAdd`, `OnChannelUpdate`, `OnThreadCreate`, `OnVoiceStateUpdate` and `OnPresenceUpdate`. `OnRawEvent` receives them all undecoded, and `client.Handle(s, func(s *client.Session, e *discord.GuildMemberAdd) {...})` adds a handler chosen by type, for code that is generic over events.

Each handler runs in its own goroutine, so a slow one does not block the gateway, and a panic is logged without stopping the bot.

Some events need an intent in `Session.Intents`, and `client.EventIntents("GUILD_MEMBER_ADD")` says which. The member, presence and message content intents are privileged: enable them in the Developer Portal.

### Connecting

| Method              | Does                                                                                                   |
| ------------------- | ------------------------------------------------------------------------------------------------------ |
| `s.Open(ctx)`       | Connects and returns once the session is ready, or gives up after 30 seconds.                          |
| `s.Close()`         | Disconnects, and gives running handlers up to 10 seconds.                                              |
| `s.Shutdown(ctx)`   | Disconnects, and waits for running handlers until `ctx` ends.                                          |
| `s.Done()`, `s.Err()` | A channel closed when the connection stops for good, and why, such as a `*client.GatewayCloseError`. |

Between `Open` and `Close`, the session reconnects by itself, and resumes when Discord allows it, so no events are lost. `s.User()` is the bot's user once it is ready.

What the gateway sends is compressed with Discord's zlib-stream, which cuts its traffic several times over for little CPU. Set `s.DisableCompression = true` before `Open` to turn it off.

### Members, voice and presence

`RequestGuildMembers` asks for the members of a guild, answered in `OnGuildMembersChunk`. Discord limits how often all members can be requested, and reports a refused request in `OnRateLimited`.

`UpdateVoiceState` joins and leaves voice channels. The bot itself sends no audio: `OnVoiceStateUpdate` and `OnVoiceServerUpdate` give a voice library or an audio node what it needs to connect. The `lavalink` package does that to play music through an audio node; see [Voice and music](voice.md).

## Sharding

Discord requires sharding from 2,500 guilds: each gateway connection, or shard, gets a share of the guilds. A session shards by itself. `Open` connects as many shards as Discord recommends, one after another as Discord requires, and returns once they are all ready. Their events go to the same handlers and the same state cache, and the gateway commands about a guild, such as `UpdateVoiceState` and `RequestGuildMembers`, go to the shard that has it. A bot under 2,500 guilds has one shard, and nothing changes.

To choose the number of shards rather than take Discord's, set it before opening:

```go
bot.Session.TotalShards = 16
```

The number is settled when the session opens. As a bot grows, Discord recommends more shards, and eventually closes a shard that has too many guilds. A session that took Discord's number then reconnects every shard with the number Discord recommends at that point, or twice as many as before if that is more. A session given `TotalShards` stops instead, as its number is yours to change.

> Up to version 0.2, `ShardCount` and `ShardID` made a session one shard. `TotalShards` replaces `ShardCount`, and without `Shards` a session now connects every shard: give each process its `Shards`, including the one that connected shard 0.

`READY` arrives once for each shard, with the shard in `e.Shard`, and `s.ShardOf(guildID)` says which shard has a guild. If Discord ends one shard for good, such as for intents the bot is not allowed, they all stop, and `s.Done()` and `s.Err()` say why.

### Across processes

A large bot can split its shards across processes, or machines, by giving each process's session its shards:

```go
bot.Session.TotalShards = 16
bot.Session.Shards = []int{0, 1, 2, 3} // this process's; the next has 4 to 7, and so on
```

Every process must use the same `TotalShards`. A gateway command for a guild on another process's shard fails with `client.ErrNotConnected`; with several shards, one for a guild ID that is not a number fails too, as its shard is unknown. Commands are the same for every shard, so let one process register them and call `bot.DisableSync()` on the others. Sessions of the same token in one process wait their turn to connect, but sessions in different processes do not know of each other: start them some seconds apart, or Discord may refuse some of their connections, which they retry.

## Logging

The session logs reconnects, invalid events and panics in handlers with `log/slog`, to `slog.Default()` unless `Session.Logger` is set. The bot's own logs, such as errors with no error handler, go to the same logger:

```go
s.Logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

// or, with the framework:
bot.Session.Logger = logger

// to log nothing:
s.Logger = slog.New(slog.DiscardHandler)
```

## Helpers

`discord` has the small things a bot keeps needing: `User.Mention`, `User.AvatarURL`, `Member.DisplayName`, `Emoji.MessageFormat`, `Message.URL`, `discord.Timestamp`, `discord.SnowflakeTime`, `discord.ImageData` for uploading avatars and emojis, and `discord.MemberPermissions` to compute what a member may do in a channel.

Next: [State cache](./state.md).
