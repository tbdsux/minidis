package client

import (
	"context"
	"iter"

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

// userID returns the ID of u, or "" if it is missing, which ends the paging.
func userID(u *discord.User) string {
	if u == nil {
		return ""
	}
	return u.ID
}
