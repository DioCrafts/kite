package plugin

// PluginManifest contains all metadata, permissions, dependencies, and
// frontend configuration for a Kite plugin. It is returned by
// KitePlugin.Manifest() and is also serializable as manifest.yaml on disk.
type PluginManifest struct {
	// Name is the unique identifier for the plugin (e.g. "cost-analyzer").
	Name string `json:"name" yaml:"name"`

	// Version is the semver version string (e.g. "1.2.0").
	Version string `json:"version" yaml:"version"`

	// Description is a short human-readable summary of the plugin.
	Description string `json:"description" yaml:"description"`

	// Author is the plugin author or organization.
	Author string `json:"author" yaml:"author"`

	// Requires declares dependencies on other plugins with semver constraints.
	Requires []Dependency `json:"requires,omitempty" yaml:"requires,omitempty"`

	// Permissions declares the Kubernetes resources and verbs this plugin needs.
	// Kite enforces these at runtime — any undeclared access is denied.
	Permissions []Permission `json:"permissions,omitempty" yaml:"permissions,omitempty"`

	// Frontend holds the Module Federation configuration for the plugin's UI.
	// Nil if the plugin is backend-only.
	Frontend *FrontendManifest `json:"frontend,omitempty" yaml:"frontend,omitempty"`

	// Settings defines the configurable fields exposed in the admin settings UI.
	Settings []SettingField `json:"settings,omitempty" yaml:"settings,omitempty"`

	// Priority determines the order in which plugin middleware is applied.
	// Lower values execute first. Default is 100.
	Priority int `json:"priority,omitempty" yaml:"priority,omitempty"`

	// RateLimit is the maximum requests/second allowed for this plugin's endpoints.
	// Default is 100.
	RateLimit int `json:"rateLimit,omitempty" yaml:"rateLimit,omitempty"`
}

// Dependency declares a required plugin with a semver constraint.
type Dependency struct {
	// Name is the required plugin's unique identifier.
	Name string `json:"name" yaml:"name"`

	// Version is a semver range constraint (e.g. ">=1.0.0", "^2.3.0").
	Version string `json:"version" yaml:"version"`
}

// Permission declares access to a Kubernetes resource type with specific verbs.
type Permission struct {
	// Resource is the Kubernetes resource type (e.g. "pods", "deployments", "prometheus").
	Resource string `json:"resource" yaml:"resource"`

	// Verbs lists the allowed actions. Values: "get", "create", "update", "delete", "log", "exec".
	Verbs []string `json:"verbs" yaml:"verbs"`
}

// SettingField describes a single configurable field in the plugin's settings panel.
type SettingField struct {
	// Name is the key used to store and retrieve this setting.
	Name string `json:"name" yaml:"name"`

	// Label is the human-readable label shown in the settings UI.
	Label string `json:"label" yaml:"label"`

	// Type determines the input widget: "text", "number", "boolean", "select", "textarea".
	Type string `json:"type" yaml:"type"`

	// Default is the default value as a string. For boolean use "true"/"false".
	Default string `json:"default,omitempty" yaml:"default,omitempty"`

	// Description is optional helper text shown below the input field.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Options lists allowed values for "select" type fields.
	Options []SettingOption `json:"options,omitempty" yaml:"options,omitempty"`

	// Required indicates whether the field must have a value.
	Required bool `json:"required,omitempty" yaml:"required,omitempty"`
}

// SettingOption is a label-value pair for select-type settings.
type SettingOption struct {
	Label string `json:"label" yaml:"label"`
	Value string `json:"value" yaml:"value"`
}

// FrontendManifest describes the Module Federation configuration
// for a plugin's frontend bundle.
type FrontendManifest struct {
	// RemoteEntry is the URL to the plugin's remoteEntry.js file.
	// For plugins bundled with Kite, use a relative path like
	// "/plugins/<name>/static/remoteEntry.js".
	RemoteEntry string `json:"remoteEntry" yaml:"remoteEntry"`

	// ExposedModules maps federation module names to human-readable IDs.
	// Example: {"./CostDashboard": "CostDashboard", "./Settings": "CostSettings"}
	ExposedModules map[string]string `json:"exposedModules,omitempty" yaml:"exposedModules,omitempty"`

	// Routes defines the pages the plugin adds to Kite's router.
	Routes []FrontendRoute `json:"routes,omitempty" yaml:"routes,omitempty"`

	// SettingsPanel is the Module Federation module name (e.g. "./Settings")
	// for the plugin's settings panel rendered within Kite's Settings page.
	SettingsPanel string `json:"settingsPanel,omitempty" yaml:"settingsPanel,omitempty"`
}

// FrontendRoute defines a route the plugin adds to Kite's React Router.
type FrontendRoute struct {
	// Path is the URL path (e.g. "/cost"). It is mounted under the Kite base path.
	Path string `json:"path" yaml:"path"`

	// Module is the Module Federation module name to load (e.g. "./CostDashboard").
	Module string `json:"module" yaml:"module"`

	// SidebarEntry, if set, adds a link in Kite's sidebar.
	SidebarEntry *SidebarEntry `json:"sidebarEntry,omitempty" yaml:"sidebarEntry,omitempty"`
}

// SidebarEntry defines how the plugin appears in Kite's sidebar navigation.
type SidebarEntry struct {
	// Title is the display text (e.g. "Cost Analysis").
	Title string `json:"title" yaml:"title"`

	// Icon is a Tabler icon name without the "Icon" prefix (e.g. "currency-dollar").
	Icon string `json:"icon" yaml:"icon"`

	// Section groups the entry under a sidebar section (e.g. "observability", "security").
	Section string `json:"section,omitempty" yaml:"section,omitempty"`

	// Priority determines the order within the section. Lower values appear first.
	Priority int `json:"priority,omitempty" yaml:"priority,omitempty"`
}
