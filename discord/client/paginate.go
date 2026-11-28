package client

import (
	"context"
	"iter"
	"time"

	"github.com/tbdsux/minidis/discord"
)

// paginate yields every item of a list endpoint, fetching size items a page,
// each page from the cursor of the last item before it, until a page comes
// back short, or the cursor stops advancing. A page that fails is yielded as
// an error, which ends it.
func paginate[T any](size int, page func(cursor string) ([]T, error), cursor func(T) string) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		next := ""
		for {
			items, err := page(next)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			for _, item := range items {
				if !yield(item, nil) {
					return
				}
			}
			if len(items) < size {
				return
			}
			// A cursor that does not advance would fetch the same page forever.
			c := cursor(items[len(items)-1])
			if c == "" || c == next {
				return
			}
			next = c
		}
	}
}

// ChannelMessagesAll iterates over a channel's messages, newest first. Pages
// of 100 are fetched as the loop reaches them, so breaking out of it stops
// the fetching:
//
//	for m, err := range s.ChannelMessagesAll(ctx, channelID) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(m.Content)
//	}
func (s *Session) ChannelMessagesAll(ctx context.Context, channelID string) iter.Seq2[*discord.Message, error] {
	return paginate(100, func(before string) ([]*discord.Message, error) {
		return s.ChannelMessages(ctx, channelID, 100, before)
	}, func(m *discord.Message) string { return m.ID })
}

// GuildMembersAll iterates over a guild's members by user ID, fetching pages
// of 1000 as the loop reaches them; see ChannelMessagesAll. Like GuildMembers,
// it needs the privileged IntentsGuildMembers intent to be enabled in the
// developer portal.
func (s *Session) GuildMembersAll(ctx context.Context, guildID string) iter.Seq2[*discord.Member, error] {
	return paginate(1000, func(after string) ([]*discord.Member, error) {
		return s.GuildMembers(ctx, guildID, 1000, after)
	}, func(m *discord.Member) string { return userID(m.User) })
}

// GuildBansAll iterates over a guild's bans by user ID, fetching pages of
// 1000 as the loop reaches them; see ChannelMessagesAll.
func (s *Session) GuildBansAll(ctx context.Context, guildID string) iter.Seq2[*discord.GuildBan, error] {
	return paginate(1000, func(after string) ([]*discord.GuildBan, error) {
		return s.GuildBans(ctx, guildID, 1000, after)
	}, func(b *discord.GuildBan) string { return userID(b.User) })
}

// MessageReactionsAll iterates over the users who reacted to a message with
// an emoji, by user ID, fetching pages of 100 as the loop reaches them; see
// ChannelMessagesAll. emoji is a Unicode emoji, or "name:id" for a custom one.
func (s *Session) MessageReactionsAll(ctx context.Context, channelID, messageID, emoji string) iter.Seq2[*discord.User, error] {
	return paginate(100, func(after string) ([]*discord.User, error) {
		return s.MessageReactions(ctx, channelID, messageID, emoji, 100, after)
	}, func(u *discord.User) string { return u.ID })
}

// ChannelPinsAll iterates over a channel's pinned messages, most recently
// pinned first, fetching pages of 50 as the loop reaches them; see
// ChannelMessagesAll.
func (s *Session) ChannelPinsAll(ctx context.Context, channelID string) iter.Seq2[*discord.MessagePin, error] {
	return paginate(50, func(before string) ([]*discord.MessagePin, error) {
		var t time.Time
		if before != "" {
			t, _ = time.Parse(time.RFC3339Nano, before) // formatted by the cursor below
		}
		pins, err := s.ChannelPins(ctx, channelID, 50, t)
		if err != nil {
			return nil, err
		}
		return pins.Items, nil
	}, func(p *discord.MessagePin) string { return p.PinnedAt.Format(time.RFC3339Nano) })
}

// PollAnswerVotersAll iterates over the users who voted for an answer of a
// poll, by user ID, fetching pages of 100 as the loop reaches them; see
// ChannelMessagesAll.
func (s *Session) PollAnswerVotersAll(ctx context.Context, channelID, messageID string, answerID int) iter.Seq2[*discord.User, error] {
	return paginate(100, func(after string) ([]*discord.User, error) {
		return s.PollAnswerVoters(ctx, channelID, messageID, answerID, 100, after)
	}, func(u *discord.User) string { return u.ID })
}

// ThreadMembersAll iterates over the members of a thread by user ID, each
// with their guild member, fetching pages of 100 as the loop reaches them;
// see ChannelMessagesAll. Like ThreadMembers, it needs the privileged
// IntentsGuildMembers intent.
func (s *Session) ThreadMembersAll(ctx context.Context, threadID string) iter.Seq2[*discord.ThreadMember, error] {
	return paginate(100, func(after string) ([]*discord.ThreadMember, error) {
		return s.ThreadMembers(ctx, threadID, true, 100, after) // only paged with members
	}, func(m *discord.ThreadMember) string { return m.UserID })
}

// GuildScheduledEventUsersAll iterates over the users interested in a
// scheduled event by user ID, with their guild members if withMember is set,
// fetching pages of 100 as the loop reaches them; see ChannelMessagesAll.
func (s *Session) GuildScheduledEventUsersAll(ctx context.Context, guildID, eventID string, withMember bool) iter.Seq2[*discord.GuildScheduledEventUser, error] {
	return paginate(100, func(after string) ([]*discord.GuildScheduledEventUser, error) {
		return s.GuildScheduledEventUsers(ctx, guildID, eventID, 100, withMember, after)
	}, func(u *discord.GuildScheduledEventUser) string { return userID(u.User) })
}

// GuildAuditLogAll iterates over the entries of a guild's audit log that
// match q, newest first, starting before q.Before if it is set, fetching
// pages of 100 as the loop reaches them; see ChannelMessagesAll. q.After and
// q.Limit are ignored. The users, webhooks and other objects an audit log
// page comes with are not kept; use GuildAuditLog for those.
func (s *Session) GuildAuditLogAll(ctx context.Context, guildID string, q discord.AuditLogQuery) iter.Seq2[*discord.AuditLogEntry, error] {
	return paginate(100, func(before string) ([]*discord.AuditLogEntry, error) {
		p := q
		if before != "" {
			p.Before = before
		}
		p.After, p.Limit = "", 100
		log, err := s.GuildAuditLog(ctx, guildID, p)
		if err != nil {
			return nil, err
		}
		return log.Entries, nil
	}, func(e *discord.AuditLogEntry) string { return e.ID })
}

// CurrentUserGuildsAll iterates over the guilds the bot is in by ID,
// fetching pages of 200 as the loop reaches them; see ChannelMessagesAll.
// The guilds are partial, as with CurrentUserGuilds.
func (s *Session) CurrentUserGuildsAll(ctx context.Context) iter.Seq2[*discord.Guild, error] {
	return paginate(200, func(after string) ([]*discord.Guild, error) {
		return s.CurrentUserGuilds(ctx, 200, after)
	}, func(g *discord.Guild) string { return g.ID })
}

// userID returns the ID of u, or "" if it is missing, which ends the paging.
func userID(u *discord.User) string {
	if u == nil {
		return ""
	}
	return u.ID
}
