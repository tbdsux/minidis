package discord

import (
	"encoding/json"
	"fmt"
	"time"
)

// ApplicationCommandType is the kind of an application command.
type ApplicationCommandType int

const (
	ChatApplicationCommand    ApplicationCommandType = 1
	UserApplicationCommand    ApplicationCommandType = 2
	MessageApplicationCommand ApplicationCommandType = 3

	// PrimaryEntryPointApplicationCommand launches an Activity. An app has at
	// most one, and only as a global command.
	PrimaryEntryPointApplicationCommand ApplicationCommandType = 4
)

// EntryPointHandlerType says who handles a PrimaryEntryPointApplicationCommand.
type EntryPointHandlerType int

const (
	// EntryPointHandlerApp sends the interaction to the app, which responds,
	// for example with InteractionResponseLaunchActivity.
	EntryPointHandlerApp EntryPointHandlerType = 1

	// EntryPointHandlerDiscordLaunchActivity has Discord launch the Activity
	// itself. The app receives no interaction.
	EntryPointHandlerDiscordLaunchActivity EntryPointHandlerType = 2
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
	Name                     string                        `json:"name,omitempty"` // omitted so that edits can leave it unchanged
	NameLocalizations        *map[Locale]string            `json:"name_localizations,omitempty"`
	Description              string                        `json:"description,omitempty"`
	DescriptionLocalizations *map[Locale]string            `json:"description_localizations,omitempty"`
	DefaultMemberPermissions *int64                        `json:"default_member_permissions,omitempty,string"`
	NSFW                     *bool                         `json:"nsfw,omitempty"`
	Contexts                 *[]InteractionContextType     `json:"contexts,omitempty"`          // global commands only
	IntegrationTypes         *[]ApplicationIntegrationType `json:"integration_types,omitempty"` // global commands only
	Options                  []*ApplicationCommandOption   `json:"options,omitempty"`
	Handler                  EntryPointHandlerType         `json:"handler,omitempty"` // PrimaryEntryPointApplicationCommand only
}

// ApplicationCommandPermissionType is what an ApplicationCommandPermission
// applies to.
type ApplicationCommandPermissionType int

const (
	ApplicationCommandPermissionRole    ApplicationCommandPermissionType = 1
	ApplicationCommandPermissionUser    ApplicationCommandPermissionType = 2
	ApplicationCommandPermissionChannel ApplicationCommandPermissionType = 3
)

// ApplicationCommandPermission allows or denies a command for a role, user or
// channel. The guild ID as a role ID means @everyone, and the guild ID minus
// one as a channel ID means all channels.
type ApplicationCommandPermission struct {
	ID         string                           `json:"id"`
	Type       ApplicationCommandPermissionType `json:"type"`
	Permission bool                             `json:"permission"`
}

// GuildApplicationCommandPermissions are the permission overwrites of a command
// in a guild. ID is the command's, or the application's for overwrites that
// apply to all of its commands.
type GuildApplicationCommandPermissions struct {
	ID            string                          `json:"id"`
	ApplicationID string                          `json:"application_id"`
	GuildID       string                          `json:"guild_id"`
	Permissions   []*ApplicationCommandPermission `json:"permissions"`
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
	Guild          *Guild                 `json:"guild"` // partial: ID, Features and Locale
	GuildID        string                 `json:"guild_id"`
	Channel        *Channel               `json:"channel"` // partial
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

	// Entitlements are the app's SKUs the user or guild has.
	Entitlements []*Entitlement `json:"entitlements"`

	// AuthorizingIntegrationOwners says how the app was installed for this
	// interaction. ApplicationIntegrationGuildInstall maps to the guild ID, or
	// "0" in a DM with the bot; ApplicationIntegrationUserInstall maps to the
	// ID of the user who installed it.
	AuthorizingIntegrationOwners map[ApplicationIntegrationType]string `json:"authorizing_integration_owners"`

	// AttachmentSizeLimit is the largest file, in bytes, a response can have.
	AttachmentSizeLimit int `json:"attachment_size_limit"`
}

// EntitlementType is how an Entitlement was obtained.
type EntitlementType int

const (
	EntitlementPurchase                EntitlementType = 1
	EntitlementPremiumSubscription     EntitlementType = 2
	EntitlementDeveloperGift           EntitlementType = 3
	EntitlementTestModePurchase        EntitlementType = 4
	EntitlementFreePurchase            EntitlementType = 5
	EntitlementUserGift                EntitlementType = 6
	EntitlementPremiumPurchase         EntitlementType = 7
	EntitlementApplicationSubscription EntitlementType = 8
)

// Entitlement is access to a premium offering (SKU) of the app.
type Entitlement struct {
	ID            string          `json:"id"`
	SKUID         string          `json:"sku_id"`
	ApplicationID string          `json:"application_id"`
	UserID        string          `json:"user_id"`
	Type          EntitlementType `json:"type"`
	Deleted       bool            `json:"deleted"`
	StartsAt      *time.Time      `json:"starts_at"`
	EndsAt        *time.Time      `json:"ends_at"`
	GuildID       string          `json:"guild_id"`
	Consumed      *bool           `json:"consumed"` // consumable SKUs only
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
	InteractionResponseLaunchActivity                   InteractionResponseType = 12
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
	Poll            *PollCreateRequest                `json:"poll,omitempty"`
	Files           []*File                           `json:"-"`
}

// MarshalJSON implements json.Marshaler. With MessageFlagsIsComponentsV2 set,
// an unset Content or Poll is sent as null, which Discord requires to turn an
// existing message into a Components V2 one.
func (d InteractionResponseData) MarshalJSON() ([]byte, error) {
	type alias InteractionResponseData
	if d.Flags&MessageFlagsIsComponentsV2 == 0 {
		return json.Marshal(alias(d))
	}
	// Content and Poll without omitempty shadow those of alias, so nil is null.
	return json.Marshal(struct {
		alias
		Content *string            `json:"content"`
		Poll    *PollCreateRequest `json:"poll"`
	}{alias(d), d.Content, d.Poll})
}

// InteractionCallbackResponse is returned by an interaction response sent with
// with_response=true.
type InteractionCallbackResponse struct {
	Interaction InteractionCallback          `json:"interaction"`
	Resource    *InteractionCallbackResource `json:"resource"`
}

// InteractionCallback describes the interaction that was responded to.
type InteractionCallback struct {
	ID                       string          `json:"id"`
	Type                     InteractionType `json:"type"`
	ActivityInstanceID       string          `json:"activity_instance_id"`
	ResponseMessageID        string          `json:"response_message_id"`
	ResponseMessageLoading   bool            `json:"response_message_loading"`
	ResponseMessageEphemeral bool            `json:"response_message_ephemeral"`
}

// InteractionCallbackResource is what the response created.
type InteractionCallbackResource struct {
	Type             InteractionResponseType `json:"type"`
	ActivityInstance *ActivityInstance       `json:"activity_instance"` // InteractionResponseLaunchActivity only
	Message          *Message                `json:"message"`           // message and update responses only
}

// ActivityInstance is a launched Activity.
type ActivityInstance struct {
	ID string `json:"id"`
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
	Poll            *PollCreateRequest      `json:"poll,omitempty"`
	Files           []*File                 `json:"-"`
}

// WebhookEdit is the body of an edit request. A nil field is left unchanged;
// a pointer to an empty value clears it.
//
// Flags may only hold MessageFlagsSuppressEmbeds and MessageFlagsIsComponentsV2;
// nil leaves them unchanged, and a pointer to 0 clears SuppressEmbeds.
// Components V2 can be turned on but not off. Turning it on sends an unset
// Content and Poll as null and unset Embeds as empty, as Discord requires. A
// Poll can only be added to a deferred interaction response.
type WebhookEdit struct {
	Content         *string                 `json:"content,omitempty"`
	Embeds          *[]*MessageEmbed        `json:"embeds,omitempty"`
	Components      *[]MessageComponent     `json:"components,omitempty"`
	Attachments     *[]*MessageAttachment   `json:"attachments,omitempty"`
	AllowedMentions *MessageAllowedMentions `json:"allowed_mentions,omitempty"`
	Flags           *MessageFlags           `json:"flags,omitempty"`
	Poll            *PollCreateRequest      `json:"poll,omitempty"`
	Files           []*File                 `json:"-"`
}

// MarshalJSON implements json.Marshaler; see the Flags field.
func (e WebhookEdit) MarshalJSON() ([]byte, error) {
	type alias WebhookEdit
	if e.Flags == nil || *e.Flags&MessageFlagsIsComponentsV2 == 0 {
		return json.Marshal(alias(e))
	}
	if e.Embeds == nil {
		cleared := []*MessageEmbed{}
		e.Embeds = &cleared
	}
	// Content and Poll without omitempty shadow those of alias, so nil is null.
	return json.Marshal(struct {
		alias
		Content *string            `json:"content"`
		Poll    *PollCreateRequest `json:"poll"`
	}{alias(e), e.Content, e.Poll})
}
