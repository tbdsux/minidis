package client

import (
	"context"
	"fmt"

	"github.com/tbdsux/minidis/discord"
)

// The methods below take what Session.State has, and ask Discord for what it
// does not, so that code works whether or not the state has it, and without
// a State at all. Treat what they return as read-only, as it may be the
// state's.

// GuildCached returns a guild from Session.State, or asks Discord for it if
// the state does not have it or it is unavailable. A guild from Discord has
// its roles, emojis and stickers, but not what only GUILD_CREATE sends, such
// as MemberCount.
func (s *Session) GuildCached(ctx context.Context, guildID string) (*discord.Guild, error) {
	if s.State != nil {
		if g := s.State.Guild(guildID); g != nil && !g.Unavailable {
			return g, nil
		}
	}
	return s.Guild(ctx, guildID)
}

// ChannelCached returns a channel or thread from Session.State, or asks
// Discord for it, as for an archived thread, which the state does not keep.
func (s *Session) ChannelCached(ctx context.Context, channelID string) (*discord.Channel, error) {
	if s.State != nil {
		if c := s.State.Channel(channelID); c != nil {
			return c, nil
		}
	}
	return s.Channel(ctx, channelID)
}

// GuildRoleCached returns a role from Session.State, or asks Discord for it.
func (s *Session) GuildRoleCached(ctx context.Context, guildID, roleID string) (*discord.Role, error) {
	if s.State != nil {
		if r := s.State.Role(guildID, roleID); r != nil {
			return r, nil
		}
	}
	return s.GuildRole(ctx, guildID, roleID)
}

// GuildMemberCached returns a member from Session.State, or asks Discord for
// it. A member from Discord is then kept in the state if its options keep
// that member, so that the next call finds it there.
func (s *Session) GuildMemberCached(ctx context.Context, guildID, userID string) (*discord.Member, error) {
	if s.State != nil {
		if m := s.State.Member(guildID, userID); m != nil {
			return m, nil
		}
	}
	m, err := s.GuildMember(ctx, guildID, userID)
	if err != nil {
		return nil, err
	}
	if s.State != nil {
		s.State.fetchedMember(guildID, m)
	}
	return m, nil
}

// ChannelMessageCached returns a message from Session.State, or asks Discord
// for it.
func (s *Session) ChannelMessageCached(ctx context.Context, channelID, messageID string) (*discord.Message, error) {
	if s.State != nil {
		if m := s.State.Message(channelID, messageID); m != nil {
			return m, nil
		}
	}
	return s.ChannelMessage(ctx, channelID, messageID)
}

// ChannelMemberPermissionsCached computes the permissions of a member in a
// guild channel or thread, as State.Permissions does, getting the channel,
// guild and member it needs with ChannelCached, GuildCached and
// GuildMemberCached. With all of them in the state, it makes no request.
func (s *Session) ChannelMemberPermissionsCached(ctx context.Context, channelID, userID string) (int64, error) {
	if s.State != nil {
		if perms, ok := s.State.Permissions(channelID, userID); ok {
			return perms, nil
		}
	}

	c, err := s.ChannelCached(ctx, channelID)
	if err != nil {
		return 0, err
	}
	if c.GuildID == "" {
		return 0, fmt.Errorf("discord: channel %s is not in a guild", channelID)
	}
	if c.IsThread() {
		if c, err = s.ChannelCached(ctx, c.ParentID); err != nil {
			return 0, err
		}
	}
	g, err := s.GuildCached(ctx, c.GuildID)
	if err != nil {
		return 0, err
	}
	m, err := s.GuildMemberCached(ctx, c.GuildID, userID)
	if err != nil {
		return 0, err
	}
	return discord.MemberPermissions(g, c, m), nil
}
