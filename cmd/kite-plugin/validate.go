package main

import (
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

// runValidate checks that the current plugin directory has a valid structure
// and a well-formed manifest.yaml.
func runValidate() error {
	fmt.Println("→ Validating plugin structure...")

	// 1. Check required files
	required := []string{"manifest.yaml", "main.go", "go.mod"}
	for _, f := range required {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("missing required file: %s", f)
		}
	}
	fmt.Println("  ✓ Required files present")

	// 2. Parse and validate manifest
	data, err := os.ReadFile("manifest.yaml")
	if err != nil {
		return fmt.Errorf("read manifest.yaml: %w", err)
	}

	var manifest struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Permissions []struct {
			Resource string   `json:"resource"`
			Verbs    []string `json:"verbs"`
		} `json:"permissions"`
		Frontend *struct {
			RemoteEntry string `json:"remoteEntry"`
		} `json:"frontend"`
	}

	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse manifest.yaml: %w", err)
	}

	var errors []string

	if manifest.Name == "" {
		errors = append(errors, "manifest: 'name' is required")
	}
	if manifest.Version == "" {
		errors = append(errors, "manifest: 'version' is required")
	}

	// Validate permissions
	for i, p := range manifest.Permissions {
		if p.Resource == "" {
			errors = append(errors, fmt.Sprintf("manifest: permissions[%d].resource is empty", i))
		}
		if len(p.Verbs) == 0 {
			errors = append(errors, fmt.Sprintf("manifest: permissions[%d].verbs is empty", i))
		}
	}

	// If frontend is declared, check the directory exists
	if manifest.Frontend != nil {
		if _, err := os.Stat("frontend"); err != nil {
			errors = append(errors, "manifest declares frontend but frontend/ directory is missing")
		}
	}

	fmt.Println("  ✓ Manifest parsed successfully")

	if len(errors) > 0 {
		fmt.Println("\n✗ Validation failed:")
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
		return fmt.Errorf("%d validation error(s) found", len(errors))
	}

	fmt.Printf("\n✓ Plugin %q v%s is valid\n", manifest.Name, manifest.Version)
	return nil
}

// isValidVerb checks if a Kubernetes API verb is recognized.
func isValidVerb(verb string) bool {
	switch strings.ToLower(verb) {
	case "get", "list", "watch", "create", "update", "patch", "delete", "deletecollection":
		return true
	}
	return false
}
