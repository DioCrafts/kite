// Package config implements file-based declarative configuration for Kite.
// It watches a config directory (conf.d pattern) for YAML files and reconciles
// OAuth providers, RBAC roles/assignments, and general settings to the database.
//
// This replaces the need for a CRD-based controller — configuration is managed
// entirely through Helm values rendered into a ConfigMap and mounted as files.
package config

// KiteConfig is the top-level structure for a declarative configuration file.
// Multiple files are merged alphabetically: later files override earlier ones
// for scalar fields; list fields (providers, roles) are concatenated.
type KiteConfig struct {
	OAuth           *OAuthConfig           `json:"oauth,omitempty" yaml:"oauth,omitempty"`
	Roles           []RoleConfig           `json:"roles,omitempty" yaml:"roles,omitempty"`
	GeneralSettings *GeneralSettingsConfig `json:"generalSettings,omitempty" yaml:"generalSettings,omitempty"`
}

// OAuthConfig defines the desired OAuth providers.
type OAuthConfig struct {
	Providers []OAuthProviderConfig `json:"providers,omitempty" yaml:"providers,omitempty"`
}

// OAuthProviderConfig declares a single OAuth/OIDC provider.
type OAuthProviderConfig struct {
	// Name is the unique identifier (lowercased automatically).
	Name string `json:"name" yaml:"name"`
	// IssuerURL is the OIDC issuer URL.
	IssuerURL string `json:"issuerUrl,omitempty" yaml:"issuerUrl,omitempty"`
	// ClientID for the OAuth application.
	ClientID string `json:"clientId,omitempty" yaml:"clientId,omitempty"`
	// ClientSecret for the OAuth application.
	// For production, pass this via a Kubernetes Secret env var or secretRef.
	ClientSecret string `json:"clientSecret,omitempty" yaml:"clientSecret,omitempty"`
	// AuthURL overrides the authorization endpoint.
	AuthURL string `json:"authUrl,omitempty" yaml:"authUrl,omitempty"`
	// TokenURL overrides the token endpoint.
	TokenURL string `json:"tokenUrl,omitempty" yaml:"tokenUrl,omitempty"`
	// UserInfoURL overrides the userinfo endpoint.
	UserInfoURL string `json:"userInfoUrl,omitempty" yaml:"userInfoUrl,omitempty"`
	// Scopes to request (space- or comma-separated).
	Scopes string `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	// Enabled controls whether the provider is active. Defaults to true.
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}

// RoleConfig declares a Kite RBAC role and its subject assignments.
type RoleConfig struct {
	// Name is the unique role name.
	Name string `json:"name" yaml:"name"`
	// Description of the role.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Clusters this role applies to (glob patterns). Defaults to ["*"].
	Clusters []string `json:"clusters,omitempty" yaml:"clusters,omitempty"`
	// Namespaces this role can access (glob patterns).
	Namespaces []string `json:"namespaces,omitempty" yaml:"namespaces,omitempty"`
	// Resources this role can access (glob patterns). Defaults to ["*"].
	Resources []string `json:"resources,omitempty" yaml:"resources,omitempty"`
	// Verbs allowed: get, list, watch, create, update, delete, log, terminal, "*".
	Verbs []string `json:"verbs,omitempty" yaml:"verbs,omitempty"`
	// Assignments maps subjects (users/groups) to this role.
	Assignments []AssignmentConfig `json:"assignments,omitempty" yaml:"assignments,omitempty"`
}

// AssignmentConfig binds a subject to a role.
type AssignmentConfig struct {
	// SubjectType is "user" or "group".
	SubjectType string `json:"subjectType" yaml:"subjectType"`
	// Subject is the username or OIDC group ID.
	Subject string `json:"subject" yaml:"subject"`
}

// GeneralSettingsConfig mirrors model.GeneralSetting fields that can be set declaratively.
// Pointer types allow distinguishing "not set" from "set to zero/false".
type GeneralSettingsConfig struct {
	AIAgentEnabled     *bool   `json:"aiAgentEnabled,omitempty" yaml:"aiAgentEnabled,omitempty"`
	AIProvider         *string `json:"aiProvider,omitempty" yaml:"aiProvider,omitempty"`
	AIModel            *string `json:"aiModel,omitempty" yaml:"aiModel,omitempty"`
	AIBaseURL          *string `json:"aiBaseUrl,omitempty" yaml:"aiBaseUrl,omitempty"`
	AIMaxTokens        *int    `json:"aiMaxTokens,omitempty" yaml:"aiMaxTokens,omitempty"`
	KubectlEnabled     *bool   `json:"kubectlEnabled,omitempty" yaml:"kubectlEnabled,omitempty"`
	KubectlImage       *string `json:"kubectlImage,omitempty" yaml:"kubectlImage,omitempty"`
	NodeTerminalImage  *string `json:"nodeTerminalImage,omitempty" yaml:"nodeTerminalImage,omitempty"`
	EnableAnalytics    *bool   `json:"enableAnalytics,omitempty" yaml:"enableAnalytics,omitempty"`
	EnableVersionCheck *bool   `json:"enableVersionCheck,omitempty" yaml:"enableVersionCheck,omitempty"`
}
