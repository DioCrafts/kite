package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// runPackage creates a distributable .tar.gz archive of the plugin.
// The archive includes the binary, manifest.yaml, and optionally
// the frontend/dist/ directory.
func runPackage() error {
	if _, err := os.Stat("manifest.yaml"); err != nil {
		return fmt.Errorf("manifest.yaml not found — are you in a plugin directory?")
	}

	// Read manifest to get name + version for the archive filename
	data, err := os.ReadFile("manifest.yaml")
	if err != nil {
		return fmt.Errorf("read manifest.yaml: %w", err)
	}

	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse manifest.yaml: %w", err)
	}

	if manifest.Name == "" || manifest.Version == "" {
		return fmt.Errorf("manifest must have 'name' and 'version'")
	}

	// Check binary exists
	binaryPath := manifest.Name
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("binary %q not found — run 'kite-plugin build' first", binaryPath)
	}

	archiveName := fmt.Sprintf("%s-%s.tar.gz", manifest.Name, manifest.Version)
	fmt.Printf("→ Packaging plugin as %s...\n", archiveName)

	outFile, err := os.Create(archiveName)
	if err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	prefix := manifest.Name + "/"

	// Add binary
	if err := addFileToTar(tw, binaryPath, prefix+binaryPath); err != nil {
		return fmt.Errorf("add binary: %w", err)
	}

	// Add manifest
	if err := addFileToTar(tw, "manifest.yaml", prefix+"manifest.yaml"); err != nil {
		return fmt.Errorf("add manifest: %w", err)
	}

	// Add frontend dist if present
	frontendDist := "frontend/dist"
	if info, err := os.Stat(frontendDist); err == nil && info.IsDir() {
		err := filepath.Walk(frontendDist, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			archivePath := prefix + path
			return addFileToTar(tw, path, archivePath)
		})
		if err != nil {
			return fmt.Errorf("add frontend dist: %w", err)
		}
	}

	fmt.Printf("✓ Package created: %s\n", archiveName)
	return nil
}

func addFileToTar(tw *tar.Writer, srcPath, archivePath string) error {
	// Prevent path traversal
	cleaned := filepath.Clean(archivePath)
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("invalid archive path: %s", archivePath)
	}

	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name: archivePath,
		Size: stat.Size(),
		Mode: int64(stat.Mode()),
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tw, f)
	return err
}
