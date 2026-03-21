package rbac

import (
	"fmt"
	"slices"

	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
	"k8s.io/klog/v2"
)

// CanAccess checks if user/oidcGroup can access resource with verb in cluster/namespace.
// Uses pre-compiled regex patterns — zero regexp.Compile calls on the hot path.
func CanAccess(user model.User, resource, verb, cluster, namespace string) bool {
	roles := getCompiledUserRoles(user)
	for i := range roles {
		r := &roles[i]
		if matchCompiled(r.clusters, cluster) &&
			matchCompiled(r.namespaces, namespace) &&
			matchCompiled(r.resources, resource) &&
			matchCompiled(r.verbs, verb) {
			klog.V(1).Infof("RBAC Check - User: %s, OIDC Groups: %v, Resource: %s, Verb: %s, Cluster: %s, Namespace: %s, Hit Role: %v",
				user.Key(), user.OIDCGroups, resource, verb, cluster, namespace, r.Name)
			return true
		}
	}
	klog.V(1).Infof("RBAC Check - User: %s, OIDC Groups: %v, Resource: %s, Verb: %s, Cluster: %s, Namespace: %s, No Access",
		user.Key(), user.OIDCGroups, resource, verb, cluster, namespace)
	return false
}

func CanAccessCluster(user model.User, name string) bool {
	roles := getCompiledUserRoles(user)
	for i := range roles {
		if matchCompiled(roles[i].clusters, name) {
			return true
		}
	}
	return false
}

func CanAccessNamespace(user model.User, cluster, name string) bool {
	roles := getCompiledUserRoles(user)
	for i := range roles {
		r := &roles[i]
		if matchCompiled(r.clusters, cluster) && matchCompiled(r.namespaces, name) {
			return true
		}
	}
	return false
}

// GetUserRoles returns all roles for a user/oidcGroups.
// Kept public for external consumers that need the raw common.Role slice
// (e.g. API responses, AI agent). Hot-path callers use getCompiledUserRoles.
func GetUserRoles(user model.User) []common.Role {
	if user.Roles != nil {
		return user.Roles
	}
	roles := getCompiledUserRoles(user)
	out := make([]common.Role, len(roles))
	for i := range roles {
		out[i] = roles[i].Role
	}
	return out
}

// getCompiledUserRoles resolves the user's compiled roles from the current config.
// When user.Roles is pre-populated (the common path via RequireAuth), we look up
// each role by name in the pre-compiled cache to avoid re-running regexp.Compile
// on every request. Only roles not found in the cache are compiled on-the-fly.
func getCompiledUserRoles(user model.User) []compiledRole {
	// Common path: user already has pre-resolved raw roles (populated by RequireAuth).
	// Look them up in the pre-compiled cache instead of recompiling.
	if user.Roles != nil {
		rwlock.RLock()
		defer rwlock.RUnlock()
		result := make([]compiledRole, 0, len(user.Roles))
		for _, r := range user.Roles {
			if cr := findCompiledRole(r.Name); cr != nil {
				result = append(result, *cr)
			} else {
				// Role not in cache (e.g. dynamically assigned) — compile on the fly
				result = append(result, compileRole(r))
			}
		}
		return result
	}

	rolesMap := make(map[string]*compiledRole)
	rwlock.RLock()
	defer rwlock.RUnlock()
	for _, mapping := range RBACConfig.RoleMapping {
		if contains(mapping.Users, "*") || contains(mapping.Users, user.Key()) {
			if r := findCompiledRole(mapping.Name); r != nil {
				rolesMap[r.Name] = r
			}
		}
		for _, group := range user.OIDCGroups {
			if contains(mapping.OIDCGroups, group) {
				if r := findCompiledRole(mapping.Name); r != nil {
					rolesMap[r.Name] = r
				}
			}
		}
	}
	roles := make([]compiledRole, 0, len(rolesMap))
	for _, role := range rolesMap {
		roles = append(roles, *role)
	}
	return roles
}

// findCompiledRole looks up a pre-compiled role by name.
// Must be called under rwlock.RLock.
func findCompiledRole(name string) *compiledRole {
	for i := range compiledRoles {
		if compiledRoles[i].Name == name {
			return &compiledRoles[i]
		}
	}
	return nil
}

func contains(list []string, val string) bool {
	return slices.Contains(list, val)
}

func NoAccess(user, verb, resource, ns, cluster string) string {
	if ns == "" {
		return fmt.Sprintf("user %s does not have permission to %s %s on cluster %s",
			user, verb, resource, cluster)
	}
	if ns == "_all" {
		ns = "All"
	}
	return fmt.Sprintf("user %s does not have permission to %s %s in namespace %s on cluster %s",
		user, verb, resource, ns, cluster)
}

func UserHasRole(user model.User, roleName string) bool {
	roles := GetUserRoles(user)
	for _, role := range roles {
		if role.Name == roleName {
			return true
		}
	}
	return false
}
