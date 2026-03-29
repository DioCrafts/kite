package plugin

import (
	"fmt"

	"github.com/blang/semver/v4"
)

// resolveDependencies builds a dependency DAG between plugins and returns
// a topologically sorted load order. It detects cycles and validates
// that all declared dependencies are present with compatible versions.
func resolveDependencies(plugins map[string]*LoadedPlugin) ([]string, error) {
	// Build adjacency list and in-degree map for topological sort.
	adj := make(map[string][]string)   // plugin → plugins that depend on it
	inDeg := make(map[string]int)      // plugin → number of dependencies
	versions := make(map[string]string) // plugin → version string

	for name, lp := range plugins {
		inDeg[name] = 0
		versions[name] = lp.Manifest.Version
	}

	for name, lp := range plugins {
		for _, dep := range lp.Manifest.Requires {
			// Verify the dependency is present
			depPlugin, ok := plugins[dep.Name]
			if !ok {
				return nil, fmt.Errorf("plugin %q requires %q which is not installed", name, dep.Name)
			}

			// Verify semver constraint
			if err := checkVersionConstraint(depPlugin.Manifest.Version, dep.Version); err != nil {
				return nil, fmt.Errorf("plugin %q requires %q %s but found %s: %w",
					name, dep.Name, dep.Version, depPlugin.Manifest.Version, err)
			}

			// dep.Name → name (name depends on dep.Name, so dep loads first)
			adj[dep.Name] = append(adj[dep.Name], name)
			inDeg[name]++
		}
	}

	// Kahn's algorithm for topological sort
	var queue []string
	for name, deg := range inDeg {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	// Sort the initial queue for deterministic ordering
	sortStrings(queue)

	var order []string
	for len(queue) > 0 {
		// Pop front
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		// Reduce in-degree for dependents
		dependents := adj[node]
		sortStrings(dependents) // deterministic
		for _, dep := range dependents {
			inDeg[dep]--
			if inDeg[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(plugins) {
		// Cycle detected — find which plugins are in the cycle
		var cycled []string
		for name, deg := range inDeg {
			if deg > 0 {
				cycled = append(cycled, name)
			}
		}
		sortStrings(cycled)
		return nil, fmt.Errorf("dependency cycle detected among plugins: %v", cycled)
	}

	return order, nil
}

// checkVersionConstraint checks if actual satisfies the semver constraint.
// The constraint can be a range string like ">=1.0.0", ">=1.0.0 <2.0.0", etc.
func checkVersionConstraint(actual, constraint string) error {
	v, err := parseSemver(actual)
	if err != nil {
		return fmt.Errorf("parse version %q: %w", actual, err)
	}

	expectedRange, err := semver.ParseRange(constraint)
	if err != nil {
		return fmt.Errorf("parse constraint %q: %w", constraint, err)
	}

	if !expectedRange(v) {
		return fmt.Errorf("version %s does not satisfy %s", actual, constraint)
	}

	return nil
}

// parseSemver parses a version string into a semver.Version.
func parseSemver(v string) (semver.Version, error) {
	return semver.Parse(v)
}

// sortStrings sorts a string slice in place for deterministic output.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
