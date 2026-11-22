package client

import (
	"errors"

	"github.com/tbdsux/minidis/discord"
)

// ErrNotConnected is returned by the gateway commands while the session has
// no ready connection to the gateway: before Open, after Close, and during a
// reconnect.
var ErrNotConnected = errors.New("discord: session is not connected to the gateway")

// command sends a gateway command on the current connection.
func (s *Session) command(op int, d any) error {
	c := s.conn.Load()
	if c == nil {
		return ErrNotConnected
	}
	return c.send(op, d)
}

// RequestGuildMembers asks Discord for members of a guild, which arrive in
// GUILD_MEMBERS_CHUNK events; see OnGuildMembersChunk. Set a Nonce to tell
// the answers to several requests apart.
func (s *Session) RequestGuildMembers(req discord.GuildMembersRequest) error {
	return s.command(opRequestGuildMembers, req)
}

// RequestSoundboardSounds asks Discord for the soundboard sounds of guilds,
// which arrive in SOUNDBOARD_SOUNDS events; see OnSoundboardSounds. Without
// guilds there is nothing to ask, and nothing is sent.
func (s *Session) RequestSoundboardSounds(guildIDs ...string) error {
	if len(guildIDs) == 0 {
		return nil
	}
	return s.command(opRequestSoundboardSounds, struct {
		GuildIDs []string `json:"guild_ids"`
	}{guildIDs})
}

// RequestChannelInfo asks Discord for the short-lived state of the voice
// channels of a guild, which arrives in a CHANNEL_INFO event; see
// OnChannelInfo. fields are "status", "voice_start_time", or both, which is
// also what is asked for when none are given.
func (s *Session) RequestChannelInfo(guildID string, fields ...string) error {
	if len(fields) == 0 {
		fields = []string{"status", "voice_start_time"}
	}
	return s.command(opRequestChannelInfo, struct {
		GuildID string   `json:"guild_id"`
		Fields  []string `json:"fields"`
	}{guildID, fields})
}

// UpdateVoiceState joins the voice channel channelID of a guild, moves the
// bot there from another one, or leaves the bot's voice channel if channelID
// is empty. Discord answers with VOICE_STATE_UPDATE and VOICE_SERVER_UPDATE
// events, which hold what a voice connection needs.
//
// The bot shows up in the channel, but this package does not send or receive
// audio: use the events with a voice library for that.
func (s *Session) UpdateVoiceState(guildID, channelID string, selfMute, selfDeaf bool) error {
	body := struct {
		GuildID   string  `json:"guild_id"`
		ChannelID *string `json:"channel_id"`
		SelfMute  bool    `json:"self_mute"`
		SelfDeaf  bool    `json:"self_deaf"`
	}{GuildID: guildID, SelfMute: selfMute, SelfDeaf: selfDeaf}
	if channelID != "" {
		body.ChannelID = &channelID
	}
	return s.command(opVoiceStateUpdate, body)
}
