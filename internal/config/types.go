package config

// ResourceFile represents a YAML file containing Keycloak resources
// It can contain either a client or a client scope (not both)
type ResourceFile struct {
	Realm       string             `yaml:"realm"`
	Client      *ClientConfig      `yaml:"client,omitempty"`
	ClientScope *ClientScopeConfig `yaml:"clientScope,omitempty"`
}

// ClientFile represents a YAML file containing a client definition
// Deprecated: Use ResourceFile instead
type ClientFile struct {
	Realm  string       `yaml:"realm"`
	Client ClientConfig `yaml:"client"`
}

// ClientScopeConfig represents a Keycloak client scope configuration
type ClientScopeConfig struct {
	// Name is the unique identifier for the scope
	Name string `yaml:"name"`

	// Description of the scope
	Description string `yaml:"description,omitempty"`

	// Protocol: openid-connect or saml
	Protocol string `yaml:"protocol,omitempty"`

	// ProtocolMappers define how attributes are mapped to tokens
	ProtocolMappers []ProtocolMapperConfig `yaml:"protocolMappers,omitempty"`

	// Attributes for additional settings
	Attributes map[string]string `yaml:"attributes,omitempty"`
}

// ProtocolMapperConfig represents a protocol mapper within a client scope
type ProtocolMapperConfig struct {
	// Name of the mapper
	Name string `yaml:"name"`

	// Protocol: openid-connect or saml
	Protocol string `yaml:"protocol,omitempty"`

	// ProtocolMapper type, e.g.:
	// - oidc-usermodel-attribute-mapper (user attribute -> token claim)
	// - oidc-hardcoded-claim-mapper (hardcoded value)
	// - oidc-audience-mapper (audience)
	ProtocolMapper string `yaml:"protocolMapper"`

	// ConsentRequired for this mapper
	ConsentRequired bool `yaml:"consentRequired,omitempty"`

	// Config contains mapper-specific configuration
	// Common keys for oidc-usermodel-attribute-mapper:
	// - user.attribute: LDAP/user attribute name
	// - claim.name: token claim name
	// - jsonType.label: String, boolean, int, long, JSON
	// - id.token.claim: "true"/"false"
	// - access.token.claim: "true"/"false"
	// - userinfo.token.claim: "true"/"false"
	// - multivalued: "true"/"false"
	Config map[string]string `yaml:"config"`
}

// ClientConfig represents a Keycloak client configuration
type ClientConfig struct {
	// Core identification
	ClientID string `yaml:"clientId"`
	Name     string `yaml:"name,omitempty"`
	// NOTE: ID is the internal Keycloak UUID, not to be confused with ClientID

	// Client type
	PublicClient       bool `yaml:"publicClient,omitempty"`
	BearerOnly         bool `yaml:"bearerOnly,omitempty"`
	ServiceAccountsEnabled bool `yaml:"serviceAccountsEnabled,omitempty"`

	// Protocol
	Protocol string `yaml:"protocol,omitempty"` // openid-connect, saml

	// OAuth/OIDC flows
	StandardFlowEnabled       bool `yaml:"standardFlowEnabled,omitempty"`
	ImplicitFlowEnabled       bool `yaml:"implicitFlowEnabled,omitempty"`
	DirectAccessGrantsEnabled bool `yaml:"directAccessGrantsEnabled,omitempty"`

	// PKCE
	PKCECodeChallengeMethod string `yaml:"pkceCodeChallengeMethod,omitempty"` // S256, plain, or empty

	// URLs
	RootURL     string   `yaml:"rootUrl,omitempty"`
	BaseURL     string   `yaml:"baseUrl,omitempty"`
	RedirectURIs []string `yaml:"redirectUris,omitempty"`
	WebOrigins  []string `yaml:"webOrigins,omitempty"`

	// Scopes
	DefaultClientScopes   []string `yaml:"defaultClientScopes,omitempty"`
	OptionalClientScopes  []string `yaml:"optionalClientScopes,omitempty"`

	// Consent
	ConsentRequired bool `yaml:"consentRequired,omitempty"`

	// Description
	Description string `yaml:"description,omitempty"`

	// Enabled
	Enabled bool `yaml:"enabled,omitempty"`

	// Attributes for additional settings
	Attributes map[string]string `yaml:"attributes,omitempty"`
}
