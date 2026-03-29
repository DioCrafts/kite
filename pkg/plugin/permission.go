package plugin

import (
	"fmt"
	"strings"
	"sync"
)

// PermissionEnforcer validates that plugin operations stay within the
// permissions declared in the plugin's manifest.yaml. Every API call
// a plugin makes is checked against its declared resources and verbs.
type PermissionEnforcer struct {
	// allowedPerms maps pluginName → resource → set of verbs.
	allowedPerms map[string]map[string]map[string]bool
	mu           sync.RWMutex
}

// NewPermissionEnforcer creates a new enforcer.
func NewPermissionEnforcer() *PermissionEnforcer {
	return &PermissionEnforcer{
		allowedPerms: make(map[string]map[string]map[string]bool),
	}
}

// RegisterPlugin registers the permission set for a plugin based on its manifest.
func (pe *PermissionEnforcer) RegisterPlugin(name string, permissions []Permission) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	resources := make(map[string]map[string]bool, len(permissions))
	for _, p := range permissions {
		verbs := make(map[string]bool, len(p.Verbs))
		for _, v := range p.Verbs {
			verbs[strings.ToLower(v)] = true
		}
		resources[strings.ToLower(p.Resource)] = verbs
	}
	pe.allowedPerms[name] = resources
}

// UnregisterPlugin removes a plugin's permission data (e.g. on unload).
func (pe *PermissionEnforcer) UnregisterPlugin(name string) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	delete(pe.allowedPerms, name)
}

// Check returns nil if the plugin is allowed to perform the given verb on
// the given resource. Returns a descriptive error if access is denied.
func (pe *PermissionEnforcer) Check(pluginName, resource, verb string) error {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	resources, ok := pe.allowedPerms[pluginName]
	if !ok {
		return fmt.Errorf("plugin %q has no registered permissions", pluginName)
	}

	r := strings.ToLower(resource)
	v := strings.ToLower(verb)

	verbs, ok := resources[r]
	if !ok {
		return fmt.Errorf("plugin %q is not permitted to access resource %q", pluginName, resource)
	}

	if !verbs[v] {
		return fmt.Errorf("plugin %q is not permitted to %q resource %q (allowed: %s)",
			pluginName, verb, resource, joinVerbs(verbs))
	}

	return nil
}

// CheckHTTPMethod is a convenience method that converts an HTTP method to a
// Kubernetes API verb and checks permissions.
func (pe *PermissionEnforcer) CheckHTTPMethod(pluginName, resource, httpMethod string) error {
	verb := httpMethodToVerb(httpMethod)
	return pe.Check(pluginName, resource, verb)
}

// PluginPermissions returns the permissions registered for a plugin.
func (pe *PermissionEnforcer) PluginPermissions(pluginName string) []Permission {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	resources, ok := pe.allowedPerms[pluginName]
	if !ok {
		return nil
	}

	perms := make([]Permission, 0, len(resources))
	for resource, verbs := range resources {
		verbList := make([]string, 0, len(verbs))
		for v := range verbs {
			verbList = append(verbList, v)
		}
		perms = append(perms, Permission{Resource: resource, Verbs: verbList})
	}
	return perms
}

func httpMethodToVerb(method string) string {
	switch strings.ToUpper(method) {
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return "get"
	}
}

func joinVerbs(verbs map[string]bool) string {
	parts := make([]string, 0, len(verbs))
	for v := range verbs {
		parts = append(parts, v)
	}
	return strings.Join(parts, ", ")
}
