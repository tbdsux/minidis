package discord

import (
	"encoding/json"
	"fmt"
)

// ApplicationCommandType is the kind of an application command.
type ApplicationCommandType int

const (
	ChatApplicationCommand    ApplicationCommandType = 1
	UserApplicationCommand    ApplicationCommandType = 2
	MessageApplicationCommand ApplicationCommandType = 3
)

// InteractionContextType is where a command can be used.
type InteractionContextType int

const (
	InteractionContextGuild          InteractionContextType = 0
	InteractionContextBotDM          InteractionContextType = 1
	InteractionContextPrivateChannel InteractionContextType = 2
)

// ApplicationIntegrationType is how an app can be installed.
type ApplicationIntegrationType int

const (
	ApplicationIntegrationGuildInstall ApplicationIntegrationType = 0
	ApplicationIntegrationUserInstall  ApplicationIntegrationType = 1
)

// ApplicationCommand is a slash command or context menu command.
type ApplicationCommand struct {
	ID                       string                        `json:"id,omitempty"`
	ApplicationID            string                        `json:"application_id,omitempty"`
	GuildID                  string                        `json:"guild_id,omitempty"`
	Version                  string                        `json:"version,omitempty"`
	Type                     ApplicationCommandType        `json:"type,omitempty"`
	Name                     string                        `json:"name"`
	NameLocalizations        *map[Locale]string            `json:"name_localizations,omitempty"`
	Description              string                        `json:"description,omitempty"`
	DescriptionLocalizations *map[Locale]string            `json:"description_localizations,omitempty"`
	DefaultMemberPermissions *int64                        `json:"default_member_permissions,omitempty,string"`
	NSFW                     *bool                         `json:"nsfw,omitempty"`
	Contexts                 *[]InteractionContextType     `json:"contexts,omitempty"`          // global commands only
	IntegrationTypes         *[]ApplicationIntegrationType `json:"integration_types,omitempty"` // global commands only
	Options                  []*ApplicationCommandOption   `json:"options,omitempty"`
}

// ApplicationCommandOptionType is the value type of a command option.
type ApplicationCommandOptionType int

const (
	ApplicationCommandOptionSubCommand      ApplicationCommandOptionType = 1
	ApplicationCommandOptionSubCommandGroup ApplicationCommandOptionType = 2
	ApplicationCommandOptionString          ApplicationCommandOptionType = 3
	ApplicationCommandOptionInteger         ApplicationCommandOptionType = 4
	ApplicationCommandOptionBoolean         ApplicationCommandOptionType = 5
	ApplicationCommandOptionUser            ApplicationCommandOptionType = 6
	ApplicationCommandOptionChannel         ApplicationCommandOptionType = 7
	ApplicationCommandOptionRole            ApplicationCommandOptionType = 8
	ApplicationCommandOptionMentionable     ApplicationCommandOptionType = 9
	ApplicationCommandOptionNumber          ApplicationCommandOptionType = 10
	ApplicationCommandOptionAttachment      ApplicationCommandOptionType = 11
)

// ApplicationCommandOption is a parameter of a command.
type ApplicationCommandOption struct {
	Type                     ApplicationCommandOptionType      `json:"type"`
	Name                     string                            `json:"name"`
	NameLocalizations        map[Locale]string                 `json:"name_localizations,omitempty"`
	Description              string                            `json:"description,omitempty"`
	DescriptionLocalizations map[Locale]string                 `json:"description_localizations,omitempty"`
	Required                 bool                              `json:"required,omitempty"`
	Choices                  []*ApplicationCommandOptionChoice `json:"choices,omitempty"`
	Options                  []*ApplicationCommandOption       `json:"options,omitempty"`
	ChannelTypes             []ChannelType                     `json:"channel_types,omitempty"`
	MinValue                 *float64                          `json:"min_value,omitempty"`
	MaxValue                 *float64                          `json:"max_value,omitempty"`
	MinLength                *int                              `json:"min_length,omitempty"`
	MaxLength                int                               `json:"max_length,omitempty"`
	Autocomplete             bool                              `json:"autocomplete,omitempty"`
	FileTypes                []string                          `json:"file_types,omitempty"`
}

// ApplicationCommandOptionChoice is a fixed value for an option.
type ApplicationCommandOptionChoice struct {
	Name              string            `json:"name"`
	NameLocalizations map[Locale]string `json:"name_localizations,omitempty"`
	Value             any               `json:"value"`
}

// InteractionType is the kind of an interaction.
type InteractionType int

const (
	InteractionPing                           InteractionType = 1
	InteractionApplicationCommand             InteractionType = 2
	InteractionMessageComponent               InteractionType = 3
	InteractionApplicationCommandAutocomplete InteractionType = 4
	InteractionModalSubmit                    InteractionType = 5
)

// Interaction is an event triggered by a user using the app.
type Interaction struct {
	ID             string                 `json:"id"`
	AppID          string                 `json:"application_id"`
	Type           InteractionType        `json:"type"`
	Data           any                    `json:"-"`
	GuildID        string                 `json:"guild_id"`
	ChannelID      string                 `json:"channel_id"`
	Member         *Member                `json:"member"`
	User           *User                  `json:"user"`
	Token          string                 `json:"token"`
	Version        int                    `json:"version"`
	Message        *Message               `json:"message"`
	AppPermissions int64                  `json:"app_permissions,string"`
	Locale         Locale                 `json:"locale"`
	GuildLocale    *Locale                `json:"guild_locale"`
	Context        InteractionContextType `json:"context"`
}

// UnmarshalJSON decodes Data according to the interaction type.
func (i *Interaction) UnmarshalJSON(data []byte) error {
	type alias Interaction
	aux := struct {
		*alias
		Data json.RawMessage `json:"data"`
	}{alias: (*alias)(i)}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if jsonNull(aux.Data) {
		return nil
	}

	var err error
	switch i.Type {
	case InteractionApplicationCommand, InteractionApplicationCommandAutocomplete:
		var d ApplicationCommandInteractionData
		err = json.Unmarshal(aux.Data, &d)
		i.Data = d
	case InteractionMessageComponent:
		var d MessageComponentInteractionData
		err = json.Unmarshal(aux.Data, &d)
		i.Data = d
	case InteractionModalSubmit:
		var d ModalSubmitInteractionData
		err = json.Unmarshal(aux.Data, &d)
		i.Data = d
	}
	if err != nil {
		return fmt.Errorf("discord: decode interaction data: %w", err)
	}

	return nil
}

// ApplicationCommandData returns the data of a command interaction. It panics on any other type.
func (i *Interaction) ApplicationCommandData() ApplicationCommandInteractionData {
	return i.Data.(ApplicationCommandInteractionData)
}

// MessageComponentData returns the data of a component interaction. It panics on any other type.
func (i *Interaction) MessageComponentData() MessageComponentInteractionData {
	return i.Data.(MessageComponentInteractionData)
}

// ModalSubmitData returns the data of a modal submit interaction. It panics on any other type.
func (i *Interaction) ModalSubmitData() ModalSubmitInteractionData {
	return i.Data.(ModalSubmitInteractionData)
}

// ResolvedData holds the full objects for IDs referenced by an interaction, keyed by ID.
type ResolvedData struct {
	Users       map[string]*User              `json:"users"`
	Members     map[string]*Member            `json:"members"`
	Roles       map[string]*Role              `json:"roles"`
	Channels    map[string]*Channel           `json:"channels"`
	Messages    map[string]*Message           `json:"messages"`
	Attachments map[string]*MessageAttachment `json:"attachments"`
}

// ApplicationCommandInteractionData is the payload of a command interaction.
type ApplicationCommandInteractionData struct {
	ID          string                                     `json:"id"`
	Name        string                                     `json:"name"`
	CommandType ApplicationCommandType                     `json:"type"`
	Resolved    *ResolvedData                              `json:"resolved"`
	Options     []*ApplicationCommandInteractionDataOption `json:"options"`
	GuildID     string                                     `json:"guild_id"`
	TargetID    string                                     `json:"target_id"`
}

// GetOption returns the option called name, or nil.
func (d ApplicationCommandInteractionData) GetOption(name string) *ApplicationCommandInteractionDataOption {
	return findOption(d.Options, name)
}

// ApplicationCommandInteractionDataOption is a value a user gave for an option.
type ApplicationCommandInteractionDataOption struct {
	Name    string                                     `json:"name"`
	Type    ApplicationCommandOptionType               `json:"type"`
	Value   any                                        `json:"value"`
	Options []*ApplicationCommandInteractionDataOption `json:"options"`
	Focused bool                                       `json:"focused"`
}

// GetOption returns the nested option called name, or nil.
func (o ApplicationCommandInteractionDataOption) GetOption(name string) *ApplicationCommandInteractionDataOption {
	return findOption(o.Options, name)
}

func findOption(options []*ApplicationCommandInteractionDataOption, name string) *ApplicationCommandInteractionDataOption {
	for _, o := range options {
		if o.Name == name {
			return o
		}
	}
	return nil
}

func (o ApplicationCommandInteractionDataOption) mustBe(want ...ApplicationCommandOptionType) {
	for _, t := range want {
		if o.Type == t {
			return
		}
	}
	panic(fmt.Sprintf("discord: option %q has type %d, want one of %v", o.Name, o.Type, want))
}

// StringValue returns the value of a string option. It panics on any other type.
func (o ApplicationCommandInteractionDataOption) StringValue() string {
	o.mustBe(ApplicationCommandOptionString)
	return o.Value.(string)
}

// IntValue returns the value of an integer option. It panics on any other type.
func (o ApplicationCommandInteractionDataOption) IntValue() int64 {
	o.mustBe(ApplicationCommandOptionInteger)
	return int64(o.Value.(float64))
}

// FloatValue returns the value of a number option. It panics on any other type.
func (o ApplicationCommandInteractionDataOption) FloatValue() float64 {
	o.mustBe(ApplicationCommandOptionNumber)
	return o.Value.(float64)
}

// BoolValue returns the value of a boolean option. It panics on any other type.
func (o ApplicationCommandInteractionDataOption) BoolValue() bool {
	o.mustBe(ApplicationCommandOptionBoolean)
	return o.Value.(bool)
}

// IDValue returns the ID given to a user, channel, role, mentionable or attachment
// option; the full object is in the interaction's Resolved data. It panics on any other type.
func (o ApplicationCommandInteractionDataOption) IDValue() string {
	o.mustBe(ApplicationCommandOptionUser, ApplicationCommandOptionChannel, ApplicationCommandOptionRole,
		ApplicationCommandOptionMentionable, ApplicationCommandOptionAttachment)
	return o.Value.(string)
}

// MessageComponentInteractionData is the payload of a component interaction.
type MessageComponentInteractionData struct {
	CustomID      string        `json:"custom_id"`
	ComponentType ComponentType `json:"component_type"`
	Values        []string      `json:"values"`
	Resolved      *ResolvedData `json:"resolved"`
}

// ModalSubmitInteractionData is the payload of a modal submit interaction.
type ModalSubmitInteractionData struct {
	CustomID   string        `json:"custom_id"`
	Components ComponentList `json:"components"`
	Resolved   *ResolvedData `json:"resolved"`
}

// InteractionResponseType is the type of an interaction callback.
type InteractionResponseType int

const (
	InteractionResponsePong                             InteractionResponseType = 1
	InteractionResponseChannelMessageWithSource         InteractionResponseType = 4
	InteractionResponseDeferredChannelMessageWithSource InteractionResponseType = 5
	InteractionResponseDeferredMessageUpdate            InteractionResponseType = 6
	InteractionResponseUpdateMessage                    InteractionResponseType = 7
	InteractionApplicationCommandAutocompleteResult     InteractionResponseType = 8
	InteractionResponseModal                            InteractionResponseType = 9
)

// InteractionResponse is the initial reply to an interaction.
type InteractionResponse struct {
	Type InteractionResponseType  `json:"type"`
	Data *InteractionResponseData `json:"data,omitempty"`
}

// InteractionResponseData is the content of an InteractionResponse. Content,
// Embeds, Components and Choices are left out when nil. An empty value that is
// not nil is sent, which clears it on a message update and, for Choices, means
// "no results" for autocomplete.
type InteractionResponseData struct {
	TTS             bool                              `json:"tts,omitempty"`
	Content         *string                           `json:"content,omitempty"`
	Embeds          []*MessageEmbed                   `json:"embeds,omitzero"`
	AllowedMentions *MessageAllowedMentions           `json:"allowed_mentions,omitempty"`
	Flags           MessageFlags                      `json:"flags,omitempty"`
	Components      []MessageComponent                `json:"components,omitzero"`
	Attachments     *[]*MessageAttachment             `json:"attachments,omitempty"`
	Choices         []*ApplicationCommandOptionChoice `json:"choices,omitzero"`    // autocomplete
	CustomID        string                            `json:"custom_id,omitempty"` // modal
	Title           string                            `json:"title,omitempty"`     // modal
	Files           []*File                           `json:"-"`
}

// WebhookParams is the body of a Create Followup Message request.
type WebhookParams struct {
	Content         string                  `json:"content,omitempty"`
	TTS             bool                    `json:"tts,omitempty"`
	Embeds          []*MessageEmbed         `json:"embeds,omitempty"`
	AllowedMentions *MessageAllowedMentions `json:"allowed_mentions,omitempty"`
	Components      []MessageComponent      `json:"components,omitempty"`
	Attachments     []*MessageAttachment    `json:"attachments,omitempty"`
	Flags           MessageFlags            `json:"flags,omitempty"`
	Files           []*File                 `json:"-"`
}

// WebhookEdit is the body of an edit request. A nil field is left unchanged;
// a pointer to an empty value clears it.
type WebhookEdit struct {
	Content         *string                 `json:"content,omitempty"`
	Embeds          *[]*MessageEmbed        `json:"embeds,omitempty"`
	Components      *[]MessageComponent     `json:"components,omitempty"`
	Attachments     *[]*MessageAttachment   `json:"attachments,omitempty"`
	AllowedMentions *MessageAllowedMentions `json:"allowed_mentions,omitempty"`
	Files           []*File                 `json:"-"`
}
