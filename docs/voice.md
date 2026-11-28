# Voice and music

minidis plays audio through a separate **audio node**, the way Lavalink works. The bot sends the node commands over HTTP. The node joins the voice channel, decodes the audio and streams it to Discord. Audio work never runs in the bot process, and several bots can share one node.

There are three parts:

| Part | Module | What it is |
| --- | --- | --- |
| `minidis/lavalink` | the core module | The client the bot uses. It speaks the Lavalink v4 protocol, so it also works with Lavalink itself. |
| `minidis/node` | its own module | The audio node: a subset of the Lavalink v4 REST and WebSocket API. It decodes audio with ffmpeg. |
| `minidis/voice` | its own module | The Discord voice connection the node uses: voice gateway v8, UDP and RTP, `aead_aes256_gcm_rtpsize` / `aead_xchacha20_poly1305_rtpsize` encryption, and DAVE end-to-end encryption. |

The node and voice modules are separate so that the core module keeps its one dependency.

## Running the node

The node needs `ffmpeg` and `ffprobe` on its `PATH`, built with libopus. Most builds have it. For YouTube links it also needs [`yt-dlp`](https://github.com/yt-dlp/yt-dlp). Keep yt-dlp up to date (`yt-dlp -U`, or your package manager), because YouTube changes often and old versions stop working.

`node` is its own Go module, so run it from its folder in a checkout of the repository:

```sh
cd node
MINIDIS_NODE_PASSWORD=secret go run ./cmd/minidis-node -addr :2333 -local-dir ~/Music
```

- Every request must send the password as its `Authorization` header.
- The node plays `http(s)` URLs, YouTube links, YouTube searches such as `ytsearch:never gonna give you up`, and files inside `-local-dir`. Without `-local-dir`, local files are disabled. Paths that lead outside the directory are refused, including through symlinks.
- The node fetches any URL a bot gives it, so anyone with the password can make it request addresses on its network. Keep the port and the password private.

## Using it from a bot

```go
bot := minidis.New(os.Getenv("TOKEN"))
bot.Session.State = client.NewState(client.StateOptions{VoiceStates: true}) // to find the user's channel

ll := lavalink.New("http://localhost:2333", "secret")
ll.Attach(bot.Session) // before the bot starts

bot.AddCommand(&minidis.Command{
	Name: "play", Description: "Plays a URL.",
	Options: []*discord.ApplicationCommandOption{{
		Type: discord.ApplicationCommandOptionString, Name: "url", Description: "What to play", Required: true,
	}},
	Execute: func(c *minidis.Context) error {
		vs := c.Session.State.VoiceState(c.GuildID, c.Author.ID)
		if vs == nil {
			return c.Reply("Join a voice channel first.")
		}
		c.DeferReply(false)
		track, err := ll.LoadTrack(c, c.String("url"))
		if err != nil {
			return c.Edit("Can't play that: " + err.Error())
		}
		ll.Join(c.GuildID, vs.ChannelID)
		if err := ll.Play(c, c.GuildID, track); err != nil {
			return err
		}
		return c.Edit("Playing **" + track.Info.Title + "**.")
	},
})
```

`Attach` does three things:
- It asks for the voice states intent.
- It opens the node's WebSocket when the bot is ready.
- It forwards Discord's `VOICE_STATE_UPDATE` and `VOICE_SERVER_UPDATE` for the bot to the node.

`Join` only asks Discord to move the bot. The node connects when those events arrive. A track played before that starts once the node is connected.

The player methods all take the guild ID:

| Method | Does |
| --- | --- |
| `LoadTrack(ctx, identifier)` | Resolves a URL, YouTube link, `ytsearch:` search (to its first result) or file name to a `*Track`. Returns `ErrNoMatches` when nothing is found, and an `*Exception` when loading fails. |
| `LoadTracks(ctx, identifier)` | Like `LoadTrack`, but returns every track: all the results of a search, such as to let someone pick one, or all the tracks of a playlist on nodes that have them. |
| `Play(ctx, guild, track)` | Plays the track, replacing the current one. |
| `Stop`, `Pause(…, bool)`, `Seek(…, time.Duration)`, `SetVolume(…, percent)` | Control the player. |
| `Player(ctx, guild)` | Returns the player's track, position, volume and connection state. |
| `Leave(ctx, guild)` | Leaves the voice channel and destroys the player. |
| `UpdatePlayer(ctx, guild, *UpdatePlayer, noReplace)` | Changes several things in one request. |

## Events and queues

The node keeps no queue. That is the bot's job, as with Lavalink. `OnEvent` receives the node's events:

| Type | When |
| --- | --- |
| `TrackStartEvent` | A track starts playing. |
| `TrackEndEvent` | A track ends. `Reason` is one of `finished`, `loadFailed`, `stopped`, `replaced`, or `cleanup` when the node lost the session it played on. |
| `TrackExceptionEvent` | A track failed. `Exception` says why. |
| `TrackStuckEvent` | No audio for `ThresholdMs` (10 seconds). |
| `WebSocketClosedEvent` | The node's Discord voice connection closed. `Code` is Discord's voice close code. |

Play the next track when `e.MayStartNext()` is true: the track finished or failed to load, not stopped or replaced. [`example/music`](../example/music/main.go) is a bot with a queue and `/play`, `/pause`, `/resume`, `/skip` and `/leave`.

## Reconnecting

Connections drop, and both of them recover on their own. That includes connections that go silent without failing, as after a network change: the voice connection counts as dead when Discord stops acknowledging its heartbeats, and the client and the node ping each other.

- **The node's voice connection to Discord.** When it drops, or Discord closes it with 4015 (voice server crashed), the node reconnects and resumes the voice session, up to 5 times in a row. Audio keeps going meanwhile. Other close codes end the connection with a `WebSocketClosedEvent`, such as 4014 when the bot is removed from the channel, or 4006 when the session is no longer valid.
- **The bot's WebSocket to the node.** When it drops, the client reconnects, waiting longer after each failure, up to 30 seconds. The client turns on resuming when it connects, so the node keeps the session's players playing for 60 seconds, queues their events, and sends them once the client is back. Player methods keep working while the client reconnects.

If the node restarted, or the client took longer than 60 seconds, the node starts a new session. The client then sends the node the bot's voice state for each guild again, so the players reconnect to voice, and sends `OnEvent` a `TrackEndEvent` with the reason `EndCleanup` for each track that was playing. `MayStartNext` is false for it: a bot that keeps a queue should play its current track again, as [`example/music`](../example/music/main.go) does.

`Close` turns resuming off and stops reconnecting, so the node ends the players at once.

## End-to-end encryption

Discord has required DAVE, its end-to-end encryption for voice, since March 2026. The node does DAVE with [dave-go](https://github.com/thomas-vilte/dave-go), a pure Go implementation, so it needs no cgo or system libraries. dave-go hasn't had a third-party security audit. `voice.Params.Dave` takes any `godave.SessionCreateFunc`, so another implementation can replace it.

## Not supported yet

- **Sources and filters:** searches other than `ytsearch:` (such as `ytmsearch:` and `scsearch:`), YouTube playlists (a playlist link is refused, but a video link with `&list=` plays its video), other sites yt-dlp supports, audio filters, and `endTime`.
- **Receiving audio** from Discord.
- **Track decoding.** Encoded tracks from the minidis node are base64 JSON rather than Lavalink's binary format. Lavalink clients that decode tracks themselves can't read them; clients that treat them as opaque strings work.
- **CPU load** in the node's stats is always 0.
