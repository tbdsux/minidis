// Command music is a music bot that plays through an audio node: the
// minidis node (in the node module: cd node && go run ./cmd/minidis-node) or Lavalink. The node joins
// voice and plays; the bot only sends it commands and keeps the queue.
//
//	cd node && MINIDIS_NODE_PASSWORD=secret go run ./cmd/minidis-node -local-dir ~/Music
//	TOKEN=... GUILD=... NODE_URL=http://localhost:2333 NODE_PASSWORD=secret go run ./example/music
//
// Then, in a voice channel, /play never gonna give you up (a YouTube search),
// /play https://youtu.be/..., /play https://example.com/song.mp3 or /play song.mp3
// (a file in the node's -local-dir).
package main

import (
	"cmp"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tbdsux/minidis"
	"github.com/tbdsux/minidis/discord"
	"github.com/tbdsux/minidis/discord/client"
	"github.com/tbdsux/minidis/lavalink"
)

// queues holds the tracks waiting in each guild; the head is playing.
type queues struct {
	mu     sync.Mutex
	tracks map[string][]*lavalink.Track
}

// add queues t and reports whether it is the only one, so it plays now.
func (q *queues) add(guildID string, t *lavalink.Track) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.tracks[guildID] = append(q.tracks[guildID], t)
	return len(q.tracks[guildID]) == 1
}

// next drops the playing track and returns the one after it, or nil.
func (q *queues) next(guildID string) *lavalink.Track {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.tracks[guildID]) > 0 {
		q.tracks[guildID] = q.tracks[guildID][1:]
	}
	if len(q.tracks[guildID]) == 0 {
		return nil
	}
	return q.tracks[guildID][0]
}

// current returns the playing track, or nil.
func (q *queues) current(guildID string) *lavalink.Track {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.tracks[guildID]) == 0 {
		return nil
	}
	return q.tracks[guildID][0]
}

func (q *queues) clear(guildID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.tracks, guildID)
}

func main() {
	bot := minidis.New(os.Getenv("TOKEN"))
	bot.SetGuilds(os.Getenv("GUILD"))
	// The voice states tell which channel the person running /play is in.
	bot.Session.State = client.NewState(client.StateOptions{VoiceStates: true})

	ll := lavalink.New(cmp.Or(os.Getenv("NODE_URL"), "http://localhost:2333"), os.Getenv("NODE_PASSWORD"))
	ll.Attach(bot.Session)

	q := &queues{tracks: map[string][]*lavalink.Track{}}
	ll.OnEvent(func(e *lavalink.Event) {
		switch {
		case e.Type == lavalink.TrackExceptionEvent:
			log.Printf("guild %s: %s failed: %v", e.GuildID, e.Track.Info.Title, e.Exception)
		case e.Type == lavalink.TrackEndEvent && e.Reason == lavalink.EndCleanup:
			// The node restarted or dropped the session: play the track again.
			if t := q.current(e.GuildID); t != nil {
				if err := ll.Play(context.Background(), e.GuildID, t); err != nil {
					log.Printf("guild %s: play again: %v", e.GuildID, err)
				}
			}
		case e.MayStartNext():
			if t := q.next(e.GuildID); t != nil {
				if err := ll.Play(context.Background(), e.GuildID, t); err != nil {
					log.Printf("guild %s: play next: %v", e.GuildID, err)
				}
			}
		}
	})

	bot.AddCommand(&minidis.Command{
		Name:        "play",
		Description: "Plays a YouTube search, a URL or a file on the node, or queues it.",
		Options: []*discord.ApplicationCommandOption{{
			Type: discord.ApplicationCommandOptionString, Name: "track", Description: "Words to search YouTube for, a link or a file name", Required: true,
		}},
		Execute: func(c *minidis.Context) error {
			vs := c.Session.State.VoiceState(c.GuildID, c.Author.ID)
			if c.GuildID == "" || vs == nil || vs.ChannelID == "" {
				return c.ReplyWith(minidis.Message{Content: "Join a voice channel first.", Ephemeral: true})
			}
			// Loading a URL can take longer than Discord's 3 seconds to reply.
			if err := c.DeferReply(false); err != nil {
				return err
			}
			query := c.String("track")
			if !strings.Contains(query, "://") && filepath.Ext(query) == "" {
				query = "ytsearch:" + query // words, not a link or a file name
			}
			track, err := ll.LoadTrack(c, query)
			if err != nil {
				return c.Edit("Can't play that: " + err.Error())
			}
			if err := ll.Join(c.GuildID, vs.ChannelID); err != nil {
				return err
			}
			if !q.add(c.GuildID, track) {
				return c.Edit("Queued **" + track.Info.Title + "**.")
			}
			if err := ll.Play(c, c.GuildID, track); err != nil {
				q.clear(c.GuildID)
				return err
			}
			return c.Edit("Playing **" + track.Info.Title + "**.")
		},
	})

	for _, cmd := range []struct {
		name, description, reply string
		run                      func(ctx context.Context, guildID string) error
	}{
		{"pause", "Pauses the music.", "Paused.", func(ctx context.Context, g string) error { return ll.Pause(ctx, g, true) }},
		{"resume", "Resumes the music.", "Resumed.", func(ctx context.Context, g string) error { return ll.Pause(ctx, g, false) }},
		{"skip", "Plays the next track in the queue.", "Skipped.", func(ctx context.Context, g string) error {
			if t := q.next(g); t != nil {
				return ll.Play(ctx, g, t)
			}
			return ll.Stop(ctx, g)
		}},
		{"leave", "Stops and leaves the voice channel.", "Bye!", func(ctx context.Context, g string) error {
			q.clear(g)
			return ll.Leave(ctx, g)
		}},
	} {
		bot.AddCommand(&minidis.Command{
			Name:        cmd.name,
			Description: cmd.description,
			Execute: func(c *minidis.Context) error {
				if err := cmd.run(c, c.GuildID); err != nil {
					return c.ReplyWith(minidis.Message{Content: "Nothing is playing.", Ephemeral: true})
				}
				return c.Reply(cmd.reply)
			},
		})
	}

	if err := bot.Start(); err != nil {
		log.Fatalln("Bot stopped with an error:", err)
	}
}
