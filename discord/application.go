package discord

import (
	"encoding/json"
	"time"
)

// Application is an app. Only ID and Flags are set on the one sent in READY.
type Application struct {
	ID                             string          `json:"id"`
	Name                           string          `json:"name"`
	Icon                           string          `json:"icon"`
	Description                    string          `json:"description"`
	BotPublic                      bool            `json:"bot_public"`
	BotRequireCodeGrant            bool            `json:"bot_require_code_grant"`
	Bot                            *User           `json:"bot"`
	TermsOfServiceURL              string          `json:"terms_of_service_url"`
	PrivacyPolicyURL               string          `json:"privacy_policy_url"`
	Owner                          *User           `json:"owner"`
	VerifyKey                      string          `json:"verify_key"` // the public key for HTTP interactions
	Team                           json.RawMessage `json:"team"`
	GuildID                        string          `json:"guild_id"`
	CoverImage                     string          `json:"cover_image"`
	Flags                          int             `json:"flags"`
	ApproximateGuildCount          int             `json:"approximate_guild_count"`
	ApproximateUserInstallCount    int             `json:"approximate_user_install_count"`
	RedirectURIs                   []string        `json:"redirect_uris"`
	InteractionsEndpointURL        string          `json:"interactions_endpoint_url"`
	RoleConnectionsVerificationURL string          `json:"role_connections_verification_url"`
	Tags                           []string        `json:"tags"`
	CustomInstallURL               string          `json:"custom_install_url"`
	InstallParams                  *InstallParams  `json:"install_params"`

	// IntegrationTypesConfig is the default install settings of each
	// installation context the app supports.
	IntegrationTypesConfig map[ApplicationIntegrationType]*IntegrationTypeConfig `json:"integration_types_config"`
}

// Application flags, for Application.Flags.
const (
	ApplicationFlagAutoModerationRuleCreateBadge = 1 << 6
	ApplicationFlagGatewayPresence               = 1 << 12
	ApplicationFlagGatewayPresenceLimited        = 1 << 13
	ApplicationFlagGatewayGuildMembers           = 1 << 14
	ApplicationFlagGatewayGuildMembersLimited    = 1 << 15
	ApplicationFlagVerificationPendingGuildLimit = 1 << 16
	ApplicationFlagEmbedded                      = 1 << 17
	ApplicationFlagGatewayMessageContent         = 1 << 18
	ApplicationFlagGatewayMessageContentLimited  = 1 << 19
	ApplicationFlagApplicationCommandBadge       = 1 << 23
)

// InstallParams are the scopes and permissions an app is installed with.
type InstallParams struct {
	Scopes      []string `json:"scopes"`
	Permissions int64    `json:"permissions,string"`
}

// IntegrationTypeConfig is the install settings of one installation context.
type IntegrationTypeConfig struct {
	OAuth2InstallParams *InstallParams `json:"oauth2_install_params,omitempty"`
}

// ApplicationParams is the body of an Edit Current Application request.
// Unset fields are left unchanged. The images are data URIs; see ImageData.
type ApplicationParams struct {
	CustomInstallURL               *string          `json:"custom_install_url,omitempty"`
	Description                    *string          `json:"description,omitempty"`
	RoleConnectionsVerificationURL *string          `json:"role_connections_verification_url,omitempty"`
	InstallParams                  *InstallParams   `json:"install_params,omitempty"`
	Flags                          *int             `json:"flags,omitempty"` // only the limited gateway intent flags
	Icon                           Optional[string] `json:"icon,omitzero"`
	CoverImage                     Optional[string] `json:"cover_image,omitzero"`
	InteractionsEndpointURL        *string          `json:"interactions_endpoint_url,omitempty"`
	Tags                           *[]string        `json:"tags,omitempty"`

	IntegrationTypesConfig map[ApplicationIntegrationType]*IntegrationTypeConfig `json:"integration_types_config,omitempty"`
}

// CurrentUserParams is the body of a Modify Current User request. The images
// are data URIs (see ImageData); Null removes one.
type CurrentUserParams struct {
	Username string           `json:"username,omitempty"`
	Avatar   Optional[string] `json:"avatar,omitzero"`
	Banner   Optional[string] `json:"banner,omitzero"`
}

// RoleConnectionMetadataType is how a linked role compares a metadata value.
type RoleConnectionMetadataType int

const (
	RoleConnectionIntegerLessThanOrEqual     RoleConnectionMetadataType = 1
	RoleConnectionIntegerGreaterThanOrEqual  RoleConnectionMetadataType = 2
	RoleConnectionIntegerEqual               RoleConnectionMetadataType = 3
	RoleConnectionIntegerNotEqual            RoleConnectionMetadataType = 4
	RoleConnectionDatetimeLessThanOrEqual    RoleConnectionMetadataType = 5
	RoleConnectionDatetimeGreaterThanOrEqual RoleConnectionMetadataType = 6
	RoleConnectionBooleanEqual               RoleConnectionMetadataType = 7
	RoleConnectionBooleanNotEqual            RoleConnectionMetadataType = 8
)

// RoleConnectionMetadata is a value an app reports about a user, which
// linked roles can require.
type RoleConnectionMetadata struct {
	Type                     RoleConnectionMetadataType `json:"type"`
	Key                      string                     `json:"key"`
	Name                     string                     `json:"name"`
	NameLocalizations        map[Locale]string          `json:"name_localizations,omitempty"`
	Description              string                     `json:"description"`
	DescriptionLocalizations map[Locale]string          `json:"description_localizations,omitempty"`
}

// SKUType is the kind of a SKU.
type SKUType int

const (
	SKUTypeDurable           SKUType = 2
	SKUTypeConsumable        SKUType = 3
	SKUTypeSubscription      SKUType = 5
	SKUTypeSubscriptionGroup SKUType = 6 // made by Discord for each subscription SKU
)

// SKU is a premium offering of an app.
type SKU struct {
	ID            string  `json:"id"`
	Type          SKUType `json:"type"`
	ApplicationID string  `json:"application_id"`
	Name          string  `json:"name"`
	Slug          string  `json:"slug"`
	Flags         int     `json:"flags"`
}

// EntitlementsQuery selects the entitlements an app is asked for. Empty
// fields do not filter.
type EntitlementsQuery struct {
	UserID         string
	SKUIDs         []string
	Before         string // an entitlement ID
	After          string
	Limit          int // up to 100, the default
	GuildID        string
	ExcludeEnded   bool
	ExcludeDeleted bool
}

// EntitlementOwnerType is who a test entitlement is for.
type EntitlementOwnerType int

const (
	EntitlementOwnerGuild EntitlementOwnerType = 1
	EntitlementOwnerUser  EntitlementOwnerType = 2
)

// TestEntitlementParams is the body of a Create Test Entitlement request.
type TestEntitlementParams struct {
	SKUID     string               `json:"sku_id"`
	OwnerID   string               `json:"owner_id"`
	OwnerType EntitlementOwnerType `json:"owner_type"`
}

// SubscriptionStatus is the state of a Subscription.
type SubscriptionStatus int

const (
	SubscriptionActive   SubscriptionStatus = 0
	SubscriptionInactive SubscriptionStatus = 1
	SubscriptionEnding   SubscriptionStatus = 2 // active, but will not renew
)

// Subscription is a user's recurring payment for a SKU.
type Subscription struct {
	ID                 string             `json:"id"`
	UserID             string             `json:"user_id"`
	SKUIDs             []string           `json:"sku_ids"`
	EntitlementIDs     []string           `json:"entitlement_ids"`
	RenewalSKUIDs      []string           `json:"renewal_sku_ids"`
	CurrentPeriodStart time.Time          `json:"current_period_start"`
	CurrentPeriodEnd   time.Time          `json:"current_period_end"`
	Status             SubscriptionStatus `json:"status"`
	CanceledAt         *time.Time         `json:"canceled_at"`
	Country            string             `json:"country"`
}

// GatewayBot is what Discord recommends for connecting a bot to the gateway.
type GatewayBot struct {
	URL               string            `json:"url"`
	Shards            int               `json:"shards"` // the recommended number
	SessionStartLimit SessionStartLimit `json:"session_start_limit"`
}

// SessionStartLimit is how many gateway sessions a bot may still start.
type SessionStartLimit struct {
	Total          int `json:"total"`
	Remaining      int `json:"remaining"`
	ResetAfter     int `json:"reset_after"` // milliseconds
	MaxConcurrency int `json:"max_concurrency"`
}
