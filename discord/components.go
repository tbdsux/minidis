package discord

import (
	"encoding/json"
	"fmt"
)

// ComponentType identifies a message or modal component.
type ComponentType int

const (
	ActionsRowComponent            ComponentType = 1
	ButtonComponent                ComponentType = 2
	SelectMenuComponent            ComponentType = 3
	TextInputComponent             ComponentType = 4
	UserSelectMenuComponent        ComponentType = 5
	RoleSelectMenuComponent        ComponentType = 6
	MentionableSelectMenuComponent ComponentType = 7
	ChannelSelectMenuComponent     ComponentType = 8
	SectionComponent               ComponentType = 9
	TextDisplayComponent           ComponentType = 10
	ThumbnailComponent             ComponentType = 11
	MediaGalleryComponent          ComponentType = 12
	FileComponentType              ComponentType = 13
	SeparatorComponent             ComponentType = 14
	ContainerComponent             ComponentType = 17
	LabelComponent                 ComponentType = 18
	FileUploadComponent            ComponentType = 19
)

// MessageComponent is any component that can be sent to or received from Discord.
// Components decoded from Discord are pointers, for example *ActionsRow.
type MessageComponent interface {
	json.Marshaler
	Type() ComponentType
}

// ComponentList is a list of components that decodes each one into its concrete type.
type ComponentList []MessageComponent

// UnmarshalJSON implements json.Unmarshaler.
func (l *ComponentList) UnmarshalJSON(data []byte) error {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}

	if raws == nil {
		*l = nil
		return nil
	}

	list := make(ComponentList, 0, len(raws))
	for _, raw := range raws {
		c, err := decodeComponent(raw)
		if err != nil {
			return err
		}
		list = append(list, c)
	}
	*l = list

	return nil
}

func newComponent(t ComponentType) MessageComponent {
	switch t {
	case ActionsRowComponent:
		return &ActionsRow{}
	case ButtonComponent:
		return &Button{}
	case SelectMenuComponent, UserSelectMenuComponent, RoleSelectMenuComponent,
		MentionableSelectMenuComponent, ChannelSelectMenuComponent:
		return &SelectMenu{MenuType: t}
	case TextInputComponent:
		return &TextInput{}
	case SectionComponent:
		return &Section{}
	case TextDisplayComponent:
		return &TextDisplay{}
	case ThumbnailComponent:
		return &Thumbnail{}
	case MediaGalleryComponent:
		return &MediaGallery{}
	case FileComponentType:
		return &FileComponent{}
	case SeparatorComponent:
		return &Separator{}
	case ContainerComponent:
		return &Container{}
	case LabelComponent:
		return &Label{}
	case FileUploadComponent:
		return &FileUpload{}
	default:
		return nil
	}
}

func decodeComponent(raw json.RawMessage) (MessageComponent, error) {
	if jsonNull(raw) {
		return nil, nil
	}

	var head struct {
		Type ComponentType `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, fmt.Errorf("discord: decode component: %w", err)
	}

	c := newComponent(head.Type)
	if c == nil {
		// Discord adds component types over time; keep them instead of failing the event.
		return &UnknownComponent{Kind: head.Type, Raw: raw}, nil
	}

	if err := json.Unmarshal(raw, c); err != nil {
		return nil, fmt.Errorf("discord: decode component type %d: %w", head.Type, err)
	}

	return c, nil
}

// marshalComponent encodes v, a struct with no "type" field of its own, and adds the type.
func marshalComponent(t ComponentType, v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	out := fmt.Appendf(nil, `{"type":%d`, t)
	if len(body) > 2 {
		out = append(out, ',')
	}

	return append(out, body[1:]...), nil
}

// UnknownComponent holds a component type this package does not model yet.
type UnknownComponent struct {
	Kind ComponentType
	Raw  json.RawMessage
}

func (c UnknownComponent) Type() ComponentType          { return c.Kind }
func (c UnknownComponent) MarshalJSON() ([]byte, error) { return c.Raw, nil }

// ActionsRow is a container for buttons or a select menu.
type ActionsRow struct {
	ID         int           `json:"id,omitempty"`
	Components ComponentList `json:"components"`
}

func (ActionsRow) Type() ComponentType { return ActionsRowComponent }
func (c ActionsRow) MarshalJSON() ([]byte, error) {
	type alias ActionsRow
	return marshalComponent(c.Type(), alias(c))
}

// ButtonStyle is the appearance of a button.
type ButtonStyle int

const (
	PrimaryButton   ButtonStyle = 1
	SecondaryButton ButtonStyle = 2
	SuccessButton   ButtonStyle = 3
	DangerButton    ButtonStyle = 4
	LinkButton      ButtonStyle = 5
	PremiumButton   ButtonStyle = 6
)

// ComponentEmoji is an emoji shown on a component.
type ComponentEmoji struct {
	Name     string `json:"name,omitempty"`
	ID       string `json:"id,omitempty"`
	Animated bool   `json:"animated,omitempty"`
}

// Button is a clickable component.
type Button struct {
	ID       int             `json:"id,omitempty"`
	Label    string          `json:"label,omitempty"`
	Style    ButtonStyle     `json:"style"`
	Disabled bool            `json:"disabled,omitempty"`
	Emoji    *ComponentEmoji `json:"emoji,omitempty"`
	URL      string          `json:"url,omitempty"`
	CustomID string          `json:"custom_id,omitempty"`
	SKUID    string          `json:"sku_id,omitempty"`
}

func (Button) Type() ComponentType { return ButtonComponent }
func (c Button) MarshalJSON() ([]byte, error) {
	type alias Button
	return marshalComponent(c.Type(), alias(c))
}

// SelectMenuOption is one choice in a string select.
type SelectMenuOption struct {
	Label       string          `json:"label"`
	Value       string          `json:"value"`
	Description string          `json:"description,omitempty"`
	Emoji       *ComponentEmoji `json:"emoji,omitempty"`
	Default     bool            `json:"default,omitempty"`
}

// SelectMenuDefaultValue pre-selects an entity in an auto-populated select.
type SelectMenuDefaultValue struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "user", "role" or "channel"
}

// SelectMenu is a select component. MenuType defaults to SelectMenuComponent, a string select.
type SelectMenu struct {
	MenuType      ComponentType            `json:"-"`
	ID            int                      `json:"id,omitempty"`
	CustomID      string                   `json:"custom_id"`
	Placeholder   string                   `json:"placeholder,omitempty"`
	MinValues     *int                     `json:"min_values,omitempty"`
	MaxValues     int                      `json:"max_values,omitempty"`
	Options       []SelectMenuOption       `json:"options,omitempty"`
	DefaultValues []SelectMenuDefaultValue `json:"default_values,omitempty"`
	ChannelTypes  []ChannelType            `json:"channel_types,omitempty"`
	Required      *bool                    `json:"required,omitempty"` // modals only
	Disabled      bool                     `json:"disabled,omitempty"` // messages only
	Values        []string                 `json:"values,omitempty"`   // received on modal submit
}

func (c SelectMenu) Type() ComponentType {
	if c.MenuType == 0 {
		return SelectMenuComponent
	}
	return c.MenuType
}

func (c SelectMenu) MarshalJSON() ([]byte, error) {
	type alias SelectMenu
	return marshalComponent(c.Type(), alias(c))
}

// TextInputStyle is the size of a text input.
type TextInputStyle int

const (
	TextInputShort     TextInputStyle = 1
	TextInputParagraph TextInputStyle = 2
)

// TextInput is a text field in a modal. Wrap it in a Label; setting Label on the
// input itself inside an ActionsRow is deprecated by Discord.
type TextInput struct {
	ID          int            `json:"id,omitempty"`
	CustomID    string         `json:"custom_id"`
	Label       string         `json:"label,omitempty"`
	Style       TextInputStyle `json:"style,omitempty"`
	Placeholder string         `json:"placeholder,omitempty"`
	Value       string         `json:"value,omitempty"`
	Required    *bool          `json:"required,omitempty"` // Discord defaults this to true
	MinLength   int            `json:"min_length,omitempty"`
	MaxLength   int            `json:"max_length,omitempty"`
}

func (TextInput) Type() ComponentType { return TextInputComponent }
func (c TextInput) MarshalJSON() ([]byte, error) {
	type alias TextInput
	return marshalComponent(c.Type(), alias(c))
}

// Label titles a single interactive component in a modal.
type Label struct {
	ID          int              `json:"id,omitempty"`
	Label       string           `json:"label,omitempty"`
	Description string           `json:"description,omitempty"`
	Component   MessageComponent `json:"component"`
}

func (Label) Type() ComponentType { return LabelComponent }
func (c Label) MarshalJSON() ([]byte, error) {
	type alias Label
	return marshalComponent(c.Type(), alias(c))
}

// UnmarshalJSON implements json.Unmarshaler.
func (c *Label) UnmarshalJSON(data []byte) (err error) {
	type alias Label
	aux := struct {
		*alias
		Component json.RawMessage `json:"component"`
	}{alias: (*alias)(c)}

	if err = json.Unmarshal(data, &aux); err != nil {
		return err
	}

	c.Component, err = decodeComponent(aux.Component)
	return err
}

// FileUpload lets a user attach files in a modal.
type FileUpload struct {
	ID        int      `json:"id,omitempty"`
	CustomID  string   `json:"custom_id"`
	MinValues *int     `json:"min_values,omitempty"`
	MaxValues int      `json:"max_values,omitempty"`
	Required  *bool    `json:"required,omitempty"`
	FileTypes []string `json:"file_types,omitempty"`
	Values    []string `json:"values,omitempty"` // attachment IDs received on modal submit
}

func (FileUpload) Type() ComponentType { return FileUploadComponent }
func (c FileUpload) MarshalJSON() ([]byte, error) {
	type alias FileUpload
	return marshalComponent(c.Type(), alias(c))
}

// The components below need MessageFlagsIsComponentsV2 on the message.

// UnfurledMediaItem references media by URL or attachment://filename.
type UnfurledMediaItem struct {
	URL string `json:"url"`
}

// Section shows text next to an accessory (a Button or Thumbnail).
type Section struct {
	ID         int              `json:"id,omitempty"`
	Components ComponentList    `json:"components"`
	Accessory  MessageComponent `json:"accessory"`
}

func (Section) Type() ComponentType { return SectionComponent }
func (c Section) MarshalJSON() ([]byte, error) {
	type alias Section
	return marshalComponent(c.Type(), alias(c))
}

// UnmarshalJSON implements json.Unmarshaler.
func (c *Section) UnmarshalJSON(data []byte) (err error) {
	type alias Section
	aux := struct {
		*alias
		Accessory json.RawMessage `json:"accessory"`
	}{alias: (*alias)(c)}

	if err = json.Unmarshal(data, &aux); err != nil {
		return err
	}

	c.Accessory, err = decodeComponent(aux.Accessory)
	return err
}

// TextDisplay is markdown text.
type TextDisplay struct {
	ID      int    `json:"id,omitempty"`
	Content string `json:"content"`
}

func (TextDisplay) Type() ComponentType { return TextDisplayComponent }
func (c TextDisplay) MarshalJSON() ([]byte, error) {
	type alias TextDisplay
	return marshalComponent(c.Type(), alias(c))
}

// Thumbnail is a small image, used as a Section accessory.
type Thumbnail struct {
	ID          int               `json:"id,omitempty"`
	Media       UnfurledMediaItem `json:"media"`
	Description string            `json:"description,omitempty"`
	Spoiler     bool              `json:"spoiler,omitempty"`
}

func (Thumbnail) Type() ComponentType { return ThumbnailComponent }
func (c Thumbnail) MarshalJSON() ([]byte, error) {
	type alias Thumbnail
	return marshalComponent(c.Type(), alias(c))
}

// MediaGalleryItem is one image or video in a MediaGallery.
type MediaGalleryItem struct {
	Media       UnfurledMediaItem `json:"media"`
	Description string            `json:"description,omitempty"`
	Spoiler     bool              `json:"spoiler,omitempty"`
}

// MediaGallery is a grid of media.
type MediaGallery struct {
	ID    int                `json:"id,omitempty"`
	Items []MediaGalleryItem `json:"items"`
}

func (MediaGallery) Type() ComponentType { return MediaGalleryComponent }
func (c MediaGallery) MarshalJSON() ([]byte, error) {
	type alias MediaGallery
	return marshalComponent(c.Type(), alias(c))
}

// FileComponent displays an uploaded file; File must use attachment://.
type FileComponent struct {
	ID      int               `json:"id,omitempty"`
	File    UnfurledMediaItem `json:"file"`
	Spoiler bool              `json:"spoiler,omitempty"`
}

func (FileComponent) Type() ComponentType { return FileComponentType }
func (c FileComponent) MarshalJSON() ([]byte, error) {
	type alias FileComponent
	return marshalComponent(c.Type(), alias(c))
}

// SeparatorSpacing is the padding around a Separator.
type SeparatorSpacing int

const (
	SeparatorSpacingSmall SeparatorSpacing = 1
	SeparatorSpacingLarge SeparatorSpacing = 2
)

// Separator adds vertical space and an optional divider.
type Separator struct {
	ID      int              `json:"id,omitempty"`
	Divider *bool            `json:"divider,omitempty"`
	Spacing SeparatorSpacing `json:"spacing,omitempty"`
}

func (Separator) Type() ComponentType { return SeparatorComponent }
func (c Separator) MarshalJSON() ([]byte, error) {
	type alias Separator
	return marshalComponent(c.Type(), alias(c))
}

// Container groups components with an optional accent color.
type Container struct {
	ID          int           `json:"id,omitempty"`
	Components  ComponentList `json:"components"`
	AccentColor *int          `json:"accent_color,omitempty"`
	Spoiler     bool          `json:"spoiler,omitempty"`
}

func (Container) Type() ComponentType { return ContainerComponent }
func (c Container) MarshalJSON() ([]byte, error) {
	type alias Container
	return marshalComponent(c.Type(), alias(c))
}
