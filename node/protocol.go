package node

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/tbdsux/minidis/lavalink"
)

// The wire types are the client's, so both ends agree on the protocol.
type (
	TrackInfo   = lavalink.TrackInfo
	Track       = lavalink.Track
	Exception   = lavalink.Exception
	VoiceState  = lavalink.VoiceState
	PlayerState = lavalink.PlayerState
	PlayerInfo  = lavalink.PlayerInfo
	Stats       = lavalink.Stats
)

var emptyObject = json.RawMessage("{}")

// shortcut: tracks are base64 JSON, not Lavalink's binary format; clients that decode tracks locally won't understand them.
func newTrack(info TrackInfo) *Track {
	info.Position = 0
	b, _ := json.Marshal(info)
	return &Track{Encoded: base64.StdEncoding.EncodeToString(b), Info: info, PluginInfo: emptyObject, UserData: emptyObject}
}

func decodeTrack(encoded string) (*Track, error) {
	b, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid encoded track: %w", err)
	}
	var info TrackInfo
	if err := json.Unmarshal(b, &info); err != nil || info.Identifier == "" {
		return nil, fmt.Errorf("invalid encoded track")
	}
	t := newTrack(info)
	t.Encoded = encoded
	return t, nil
}

// nullString tells an absent field (Set false) from an explicit null (Value nil).
type nullString struct {
	Set   bool
	Value *string
}

func (n *nullString) UnmarshalJSON(b []byte) error {
	n.Set = true
	if string(b) == "null" {
		return nil
	}
	return json.Unmarshal(b, &n.Value)
}

// UpdatePlayer is the PATCH /v4/sessions/{sessionId}/players/{guildId} body.
type UpdatePlayer struct {
	Track *struct {
		Encoded    nullString      `json:"encoded"`
		Identifier *string         `json:"identifier"`
		UserData   json.RawMessage `json:"userData"`
	} `json:"track"`
	EncodedTrack nullString  `json:"encodedTrack"` // deprecated in Lavalink, still sent by older clients
	Identifier   *string     `json:"identifier"`   // deprecated
	Position     *int64      `json:"position"`
	Volume       *int        `json:"volume"`
	Paused       *bool       `json:"paused"`
	Voice        *VoiceState `json:"voice"`
}

// event builds a WebSocket "event" message.
func event(typ, guildID string, fields map[string]any) map[string]any {
	m := map[string]any{"op": "event", "type": typ, "guildId": guildID}
	for k, v := range fields {
		m[k] = v
	}
	return m
}
