package discord

import "encoding/json"

// Optional is a request field that can be left unchanged, set, or cleared.
// The zero value leaves the field out of the request; Some sets it; Null
// sends a JSON null, which is how Discord removes a value such as a channel's
// category, a member's timeout or a guild's icon:
//
//	discord.ChannelParams{ParentID: discord.Null[string]()}
//	discord.MemberParams{TimeoutUntil: discord.Some(time.Now().Add(time.Hour))}
type Optional[T any] struct {
	value T
	state uint8 // 0 unset, 1 set, 2 null
}

// Some returns an Optional holding v.
func Some[T any](v T) Optional[T] { return Optional[T]{value: v, state: 1} }

// Null returns an Optional that clears the field.
func Null[T any]() Optional[T] { return Optional[T]{state: 2} }

// IsZero reports whether the field is left out of the request.
func (o Optional[T]) IsZero() bool { return o.state == 0 }

// IsNull reports whether the field is cleared.
func (o Optional[T]) IsNull() bool { return o.state == 2 }

// Get returns the value and whether there is one.
func (o Optional[T]) Get() (T, bool) { return o.value, o.state == 1 }

// MarshalJSON implements json.Marshaler. Fields of this type are tagged
// omitzero, so an unset one is not marshaled at all.
func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if o.state != 1 {
		return []byte("null"), nil
	}
	return json.Marshal(o.value)
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*o = Null[T]()
		return nil
	}
	o.state = 1
	return json.Unmarshal(data, &o.value)
}
