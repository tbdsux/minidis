package lavalink

import (
	"encoding/json"
	"fmt"
)

// Lavalink v4 wire types, shared by this client and the minidis node.

type TrackInfo struct {
	Identifier string  `json:"identifier"`
	IsSeekable bool    `json:"isSeekable"`
	Author     string  `json:"author"`
	Length     int64   `json:"length"` // ms
	IsStream   bool    `json:"isStream"`
	Position   int64   `json:"position"` // ms
	Title      string  `json:"title"`
	URI        *string `json:"uri"`
	ArtworkURL *string `json:"artworkUrl"`
	ISRC       *string `json:"isrc"`
	SourceName string  `json:"sourceName"`
}

// Track is a playable track. Encoded is what the node plays; pass the whole
// Track back to Client.Play.
type Track struct {
	Encoded    string          `json:"encoded"`
	Info       TrackInfo       `json:"info"`
	PluginInfo json.RawMessage `json:"pluginInfo"`
	UserData   json.RawMessage `json:"userData"`
}

// Exception is a node error about a track. It is an error.
type Exception struct {
	Message  string `json:"message"`
	Severity string `json:"severity"` // common, suspicious or fault
	Cause    string `json:"cause"`
}

func (e *Exception) Error() string {
	return fmt.Sprintf("lavalink: %s (%s)", e.Message, e.Severity)
}

// VoiceState is what the node needs from Discord to join a voice channel.
type VoiceState struct {
	Token     string `json:"token"`
	Endpoint  string `json:"endpoint"`
	SessionID string `json:"sessionId"`
	ChannelID string `json:"channelId"`
}

type PlayerState struct {
	Time      int64 `json:"time"`     // unix ms
	Position  int64 `json:"position"` // ms
	Connected bool  `json:"connected"`
	Ping      int64 `json:"ping"` // ms, -1 when not connected
}

type PlayerInfo struct {
	GuildID string          `json:"guildId"`
	Track   *Track          `json:"track"`
	Volume  int             `json:"volume"`
	Paused  bool            `json:"paused"`
	State   PlayerState     `json:"state"`
	Voice   VoiceState      `json:"voice"`
	Filters json.RawMessage `json:"filters"`
}

// UpdatePlayer is the body of Client.UpdatePlayer. Nil fields are left as
// they are. To stop the track, use Client.Stop.
type UpdatePlayer struct {
	Track    *UpdateTrack `json:"track,omitempty"`
	Position *int64       `json:"position,omitempty"` // ms
	Volume   *int         `json:"volume,omitempty"`   // 0-1000, 100 is unchanged
	Paused   *bool        `json:"paused,omitempty"`
	Voice    *VoiceState  `json:"voice,omitempty"`
}

// UpdateTrack sets the track by Encoded, or by an Identifier the node loads.
type UpdateTrack struct {
	Encoded    string          `json:"encoded,omitempty"`
	Identifier string          `json:"identifier,omitempty"`
	UserData   json.RawMessage `json:"userData,omitempty"`
}

type Stats struct {
	Players        int   `json:"players"`
	PlayingPlayers int   `json:"playingPlayers"`
	Uptime         int64 `json:"uptime"` // ms
	Memory         struct {
		Free       uint64 `json:"free"`
		Used       uint64 `json:"used"`
		Allocated  uint64 `json:"allocated"`
		Reservable uint64 `json:"reservable"`
	} `json:"memory"`
	CPU struct {
		Cores        int     `json:"cores"`
		SystemLoad   float64 `json:"systemLoad"`
		LavalinkLoad float64 `json:"lavalinkLoad"`
	} `json:"cpu"`
	FrameStats any `json:"frameStats"`
}

// Event types.
const (
	TrackStartEvent      = "TrackStartEvent"
	TrackEndEvent        = "TrackEndEvent"
	TrackExceptionEvent  = "TrackExceptionEvent"
	TrackStuckEvent      = "TrackStuckEvent"
	WebSocketClosedEvent = "WebSocketClosedEvent"
)

// Track end reasons. MayStartNext tells which ones leave the player free
// for the next track of a queue.
const (
	EndFinished   = "finished"
	EndLoadFailed = "loadFailed"
	EndStopped    = "stopped"
	EndReplaced   = "replaced"
	EndCleanup    = "cleanup"
)

// Event is a player event from the node. Which fields are set depends on Type.
type Event struct {
	Type    string `json:"type"`
	GuildID string `json:"guildId"`
	Track   *Track `json:"track"`

	Reason      string     `json:"reason"`      // TrackEndEvent and WebSocketClosedEvent
	Exception   *Exception `json:"exception"`   // TrackExceptionEvent
	ThresholdMs int64      `json:"thresholdMs"` // TrackStuckEvent
	Code        int        `json:"code"`        // WebSocketClosedEvent: Discord's voice close code
	ByRemote    bool       `json:"byRemote"`    // WebSocketClosedEvent
}

// MayStartNext reports a TrackEndEvent after which a queue should play its
// next track: the track finished or failed to load.
func (e *Event) MayStartNext() bool {
	return e.Type == TrackEndEvent && (e.Reason == EndFinished || e.Reason == EndLoadFailed)
}
