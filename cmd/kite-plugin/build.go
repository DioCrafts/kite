package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// runBuild compiles the plugin Go binary and, if a frontend/ directory
// exists, builds the frontend bundle too.
func runBuild() error {
	// Check we're in a plugin directory (has manifest.yaml)
	if _, err := os.Stat("manifest.yaml"); err != nil {
		return fmt.Errorf("manifest.yaml not found — are you in a plugin directory?")
	}

	fmt.Println("→ Building plugin binary...")

	// Determine plugin name from current directory
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	pluginName := filepath.Base(wd)

	// Build Go binary
	build := exec.Command("go", "build", "-o", pluginName, ".")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}
	fmt.Printf("  ✓ Built binary: ./%s\n", pluginName)

	// Build frontend if present
	if info, err := os.Stat("frontend"); err == nil && info.IsDir() {
		fmt.Println("→ Building frontend...")

		install := exec.Command("pnpm", "install")
		install.Dir = "frontend"
		install.Stdout = os.Stdout
		install.Stderr = os.Stderr
		if err := install.Run(); err != nil {
			return fmt.Errorf("pnpm install failed: %w", err)
		}

		bundle := exec.Command("pnpm", "build")
		bundle.Dir = "frontend"
		bundle.Stdout = os.Stdout
		bundle.Stderr = os.Stderr
		if err := bundle.Run(); err != nil {
			return fmt.Errorf("frontend build failed: %w", err)
		}
		fmt.Println("  ✓ Frontend built: frontend/dist/")
	}

	fmt.Println("\n✓ Plugin build complete")
	return nil
}
