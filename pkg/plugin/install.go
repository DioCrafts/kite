package plugin

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// InstallPlugin extracts a gzipped-tar plugin archive into the plugin directory,
// validates its manifest, and loads it into the manager.
//
// The tarball must contain a single top-level directory whose name matches the
// plugin binary name declared in the manifest.  The directory must contain at
// least a "manifest.yaml" and the plugin binary.
//
// On success the newly-loaded plugin is returned.  If a plugin with the same
// name is already registered an error is returned (use ReloadPlugin for
// hot-reloads).
func (pm *PluginManager) InstallPlugin(tarball io.Reader) (*LoadedPlugin, error) {
	// --- 1. Extract the tarball into a temporary directory ---
	tmpDir, err := os.MkdirTemp("", "kite-plugin-install-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := extractTarGz(tarball, tmpDir); err != nil {
		return nil, fmt.Errorf("extract plugin tarball: %w", err)
	}

	// --- 2. Discover the plugin from the extracted directory ---
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, fmt.Errorf("read extracted dir: %w", err)
	}

	var pluginSrcDir string
	for _, e := range entries {
		if e.IsDir() {
			pluginSrcDir = filepath.Join(tmpDir, e.Name())
			break
		}
	}
	if pluginSrcDir == "" {
		return nil, fmt.Errorf("tarball must contain exactly one top-level directory")
	}

	lp, err := discoverPlugin(pluginSrcDir)
	if err != nil {
		return nil, fmt.Errorf("invalid plugin: %w", err)
	}

	pluginName := lp.Manifest.Name

	// --- 3. Check for conflicts ---
	pm.mu.RLock()
	_, exists := pm.plugins[pluginName]
	pm.mu.RUnlock()
	if exists {
		return nil, fmt.Errorf("plugin %q is already installed; use reload or uninstall first", pluginName)
	}

	// --- 4. Move extracted directory into the plugin dir ---
	absPluginDir, err := filepath.Abs(pm.pluginDir)
	if err != nil {
		return nil, fmt.Errorf("resolve plugin dir: %w", err)
	}

	if err := os.MkdirAll(absPluginDir, 0o755); err != nil {
		return nil, fmt.Errorf("create plugin dir: %w", err)
	}

	destDir := filepath.Join(absPluginDir, pluginName)
	if err := os.Rename(pluginSrcDir, destDir); err != nil {
		// os.Rename may fail across filesystems; fall back to copy+remove.
		if copyErr := copyDir(pluginSrcDir, destDir); copyErr != nil {
			return nil, fmt.Errorf("install plugin to %s: %w", destDir, copyErr)
		}
	}
	lp.Dir = destDir

	// --- 5. Load the plugin ---
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if err := pm.loadPlugin(lp); err != nil {
		// Clean up on load failure so the directory doesn't linger.
		_ = os.RemoveAll(destDir)
		return nil, fmt.Errorf("load plugin %q: %w", pluginName, err)
	}

	lp.State = PluginStateLoaded
	pm.Permissions.RegisterPlugin(pluginName, lp.Manifest.Permissions)
	pm.RateLimiter.Register(pluginName, lp.Manifest.RateLimit)
	pm.plugins[pluginName] = lp
	pm.loadOrder = append(pm.loadOrder, pluginName)

	klog.Infof("Plugin installed: %s v%s", lp.Manifest.Name, lp.Manifest.Version)
	return lp, nil
}

// UninstallPlugin stops the named plugin, removes it from the manager, and
// deletes its directory from disk.
func (pm *PluginManager) UninstallPlugin(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	lp, ok := pm.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}

	// Stop the plugin process gracefully.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	lp.mu.Lock()
	if lp.client != nil {
		lp.client.Stop(ctx)
		lp.client = nil
	}
	lp.State = PluginStateStopped
	lp.mu.Unlock()

	// Remove from registry.
	delete(pm.plugins, name)
	for i, n := range pm.loadOrder {
		if n == name {
			pm.loadOrder = append(pm.loadOrder[:i], pm.loadOrder[i+1:]...)
			break
		}
	}
	pm.Permissions.UnregisterPlugin(name)
	pm.RateLimiter.Unregister(name)

	// Delete plugin directory.
	if lp.Dir != "" {
		if err := os.RemoveAll(lp.Dir); err != nil {
			return fmt.Errorf("remove plugin dir %s: %w", lp.Dir, err)
		}
	}

	klog.Infof("Plugin uninstalled: %s", name)
	return nil
}

// extractTarGz decompresses a .tar.gz stream into destDir, rejecting any
// path-traversal entries (entries whose resolved path would reach outside destDir).
func extractTarGz(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("open gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		// Sanitize the path to prevent path-traversal (CWE-22).
		target := filepath.Join(destDir, filepath.Clean("/"+hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) &&
			target != filepath.Clean(destDir) {
			return fmt.Errorf("tar entry %q would escape destination directory", hdr.Name)
		}

		//nolint:exhaustive — only regular files and directories in plugin archives.
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create dir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create parent dir for %s: %w", target, err)
			}
			// Preserve executable bit for the plugin binary; cap at 0o755.
			mode := hdr.FileInfo().Mode() & 0o755
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return fmt.Errorf("create file %s: %w", target, err)
			}
			// Limit individual file extraction to 256 MiB to resist decompression bombs.
			if _, err := io.Copy(f, io.LimitReader(tr, 256<<20)); err != nil {
				f.Close()
				return fmt.Errorf("write file %s: %w", target, err)
			}
			f.Close()
		}
	}
	return nil
}

// copyDir recursively copies src into dst, creating dst if necessary.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode&0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
