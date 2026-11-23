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

### Members, voice and presence

`RequestGuildMembers` asks for the members of a guild, answered in `OnGuildMembersChunk`. Discord limits how often all members can be requested, and reports a refused request in `OnRateLimited`.

`UpdateVoiceState` joins and leaves voice channels, but sending and receiving audio is not part of this library: `OnVoiceStateUpdate` and `OnVoiceServerUpdate` give a voice library what it needs to connect.

## Sharding

Discord requires sharding from 2,500 guilds: each gateway connection, or shard, gets a share of the guilds. Set the shard before opening:

```go
bot.Session.ShardID = shardID       // from 0 to ShardCount-1
bot.Session.ShardCount = shardCount // sharding is on when greater than 1
```

`s.GatewayBot(ctx)` returns the number of shards Discord recommends in `Shards`. A session is one shard, so run a bot or session per shard, in one process or several. Sessions of the same token in one process wait their turn to connect, as Discord requires. Commands are the same for every shard, so let one register them and call `bot.DisableSync()` on the others.

## Helpers

`discord` has the small things a bot keeps needing: `User.Mention`, `User.AvatarURL`, `Member.DisplayName`, `Emoji.MessageFormat`, `Message.URL`, `discord.Timestamp`, `discord.SnowflakeTime`, `discord.ImageData` for uploading avatars and emojis, and `discord.MemberPermissions` to compute what a member may do in a channel.

Next: [State cache](./state.md).
