package config

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

const (
	// DefaultConfigDir is the default path where declarative config files are mounted.
	DefaultConfigDir = "/etc/kite/config.d"

	// EnvConfigDir overrides the default config directory.
	EnvConfigDir = "KITE_CONFIG_DIR"

	// debounceDelay prevents rapid re-reconciliation when multiple files change at once
	// (e.g., ConfigMap atomic update replaces all symlinks simultaneously).
	debounceDelay = 2 * time.Second

	// resyncInterval is a safety-net full re-read in case fsnotify misses an event
	// (e.g., some NFS/FUSE drivers don't emit inotify events).
	resyncInterval = 5 * time.Minute
)

// Watcher monitors a config directory for YAML files and triggers reconciliation
// whenever a file is created, modified, or deleted.
type Watcher struct {
	dir        string
	reconciler *Reconciler
	mu         sync.Mutex
	lastHash   string // SHA-256 of the merged config to skip no-op reconciliations
}

// NewWatcher creates a new file-based config watcher.
// It returns nil if the config directory does not exist (declarative config disabled).
func NewWatcher() *Watcher {
	dir := os.Getenv(EnvConfigDir)
	if dir == "" {
		dir = DefaultConfigDir
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		klog.Infof("Declarative config directory %q not found — file-based config disabled", dir)
		return nil
	}

	klog.Infof("Declarative config directory found: %s", dir)
	return &Watcher{
		dir:        dir,
		reconciler: NewReconciler(),
	}
}

// Start begins watching the config directory. It performs an initial reconciliation,
// then watches for changes via fsnotify with a periodic resync as safety net.
// Blocks until ctx is canceled.
func (w *Watcher) Start(ctx context.Context) {
	// Initial reconciliation
	w.reconcileFromDisk()

	// Set up fsnotify watcher
	fswatcher, err := fsnotify.NewWatcher()
	if err != nil {
		klog.Errorf("Failed to create fsnotify watcher: %v — falling back to polling only", err)
		w.pollLoop(ctx)
		return
	}
	defer fswatcher.Close()

	// Watch the config directory itself.
	// For ConfigMap volume mounts, Kubernetes uses a symlink swap pattern:
	//   ..data -> ..2024_01_01_00_00_00.123456789
	// When the ConfigMap is updated, a new timestamped dir is created and ..data
	// is atomically re-pointed. We watch the parent dir to catch this.
	if err := fswatcher.Add(w.dir); err != nil {
		klog.Errorf("Failed to watch directory %s: %v — falling back to polling only", w.dir, err)
		w.pollLoop(ctx)
		return
	}

	// Also watch the ..data symlink target if it exists (ConfigMap mount pattern)
	dataLink := filepath.Join(w.dir, "..data")
	if target, err := filepath.EvalSymlinks(dataLink); err == nil {
		_ = fswatcher.Add(target)
	}

	klog.Infof("Watching %s for declarative config changes (fsnotify + %s resync)", w.dir, resyncInterval)

	resyncTicker := time.NewTicker(resyncInterval)
	defer resyncTicker.Stop()

	var debounceTimer *time.Timer

	for {
		select {
		case <-ctx.Done():
			klog.Info("Declarative config watcher stopped")
			return

		case event, ok := <-fswatcher.Events:
			if !ok {
				return
			}
			// Only react to meaningful events
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			klog.V(2).Infof("Config file event: %s %s", event.Op, event.Name)

			// Debounce: reset timer on each event
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounceDelay, func() {
				w.reconcileFromDisk()
				// Re-watch ..data symlink target in case it changed (ConfigMap rotation)
				if target, err := filepath.EvalSymlinks(dataLink); err == nil {
					_ = fswatcher.Add(target)
				}
			})

		case err, ok := <-fswatcher.Errors:
			if !ok {
				return
			}
			klog.Warningf("fsnotify error: %v", err)

		case <-resyncTicker.C:
			w.reconcileFromDisk()
		}
	}
}

// pollLoop is a fallback when fsnotify is unavailable. It polls the directory
// for changes at resyncInterval.
func (w *Watcher) pollLoop(ctx context.Context) {
	klog.Info("Using polling fallback for declarative config changes")
	ticker := time.NewTicker(resyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.reconcileFromDisk()
		}
	}
}

// reconcileFromDisk reads all YAML files from the config directory, merges them,
// and reconciles the result to the database. Skips reconciliation if the merged
// config hasn't changed since the last run.
func (w *Watcher) reconcileFromDisk() {
	w.mu.Lock()
	defer w.mu.Unlock()

	cfg, raw, err := w.loadAndMerge()
	if err != nil {
		klog.Errorf("Failed to load declarative config: %v", err)
		return
	}

	// Compute hash to detect actual changes
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	if hash == w.lastHash {
		klog.V(2).Info("Declarative config unchanged — skipping reconciliation")
		return
	}

	klog.Infof("Declarative config changed (hash %s…) — reconciling", hash[:12])

	if err := w.reconciler.Reconcile(cfg); err != nil {
		klog.Errorf("Declarative config reconciliation failed: %v", err)
		return
	}

	w.lastHash = hash
	klog.Info("Declarative config reconciliation completed successfully")
}

// loadAndMerge reads all *.yaml and *.yml files from the config directory,
// sorts them alphabetically, and merges them into a single KiteConfig.
// Returns the merged config and the raw concatenated bytes (for hashing).
func (w *Watcher) loadAndMerge() (*KiteConfig, []byte, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil, nil, fmt.Errorf("reading config directory %s: %w", w.dir, err)
	}

	// Collect YAML file paths, sorted alphabetically
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip hidden files and Kubernetes ConfigMap metadata
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "..") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".yaml" || ext == ".yml" {
			files = append(files, filepath.Join(w.dir, name))
		}
	}
	sort.Strings(files)

	if len(files) == 0 {
		klog.V(1).Info("No YAML files found in config directory — nothing to reconcile")
		return &KiteConfig{}, []byte{}, nil
	}

	merged := &KiteConfig{}
	var allRaw []byte

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, fmt.Errorf("reading config file %s: %w", filepath.Base(f), err)
		}
		if len(strings.TrimSpace(string(data))) == 0 {
			continue
		}
		allRaw = append(allRaw, data...)

		var fragment KiteConfig
		if err := yaml.UnmarshalStrict(data, &fragment); err != nil {
			return nil, nil, fmt.Errorf("invalid config in %s: %w", filepath.Base(f), err)
		}

		klog.V(2).Infof("Loaded config fragment: %s", filepath.Base(f))
		mergeConfig(merged, &fragment)
	}

	// Deduplicate providers: if multiple fragments declare the same provider name,
	// merge their fields (last-write-wins for non-empty fields).
	if merged.OAuth != nil {
		merged.OAuth.Providers = deduplicateProviders(merged.OAuth.Providers)
	}

	// Deduplicate roles: if multiple fragments declare the same role name,
	// merge their assignments (union) so conf.d splits don't cause revocations.
	merged.Roles = deduplicateRoles(merged.Roles)

	return merged, allRaw, nil
}

// deduplicateProviders coalesces OAuth providers with the same name after merging
// all fragments. Last-write-wins for non-empty scalar fields; the Enabled pointer
// is overwritten only when set.
func deduplicateProviders(providers []OAuthProviderConfig) []OAuthProviderConfig {
	order := make([]string, 0, len(providers))
	byName := make(map[string]*OAuthProviderConfig, len(providers))

	for _, p := range providers {
		name := strings.ToLower(p.Name)
		if existing, ok := byName[name]; ok {
			if p.IssuerURL != "" {
				existing.IssuerURL = p.IssuerURL
			}
			if p.ClientID != "" {
				existing.ClientID = p.ClientID
			}
			if p.ClientSecret != "" {
				existing.ClientSecret = p.ClientSecret
			}
			if p.AuthURL != "" {
				existing.AuthURL = p.AuthURL
			}
			if p.TokenURL != "" {
				existing.TokenURL = p.TokenURL
			}
			if p.UserInfoURL != "" {
				existing.UserInfoURL = p.UserInfoURL
			}
			if p.Scopes != "" {
				existing.Scopes = p.Scopes
			}
			if p.Enabled != nil {
				existing.Enabled = p.Enabled
			}
		} else {
			copy := p
			byName[name] = &copy
			order = append(order, name)
		}
	}

	result := make([]OAuthProviderConfig, 0, len(order))
	for _, name := range order {
		result = append(result, *byName[name])
	}
	return result
}

// deduplicateRoles coalesces roles with the same name after merging all fragments.
// The last definition of scope fields (description, clusters, namespaces, resources, verbs)
// wins, while assignments are unioned across all entries to prevent conf.d splits from
// accidentally revoking access during orphan cleanup.
func deduplicateRoles(roles []RoleConfig) []RoleConfig {
	order := make([]string, 0, len(roles))
	byName := make(map[string]*RoleConfig, len(roles))

	for _, r := range roles {
		name := strings.ToLower(r.Name)
		if existing, ok := byName[name]; ok {
			// Last-write-wins for scope fields.
			// Use nil-checks (not len > 0) so a later fragment can
			// explicitly clear a scope with an empty list (e.g. namespaces: []).
			if r.Description != "" {
				existing.Description = r.Description
			}
			if r.Clusters != nil {
				existing.Clusters = r.Clusters
			}
			if r.Namespaces != nil {
				existing.Namespaces = r.Namespaces
			}
			if r.Resources != nil {
				existing.Resources = r.Resources
			}
			if r.Verbs != nil {
				existing.Verbs = r.Verbs
			}
			// Union assignments
			existing.Assignments = append(existing.Assignments, r.Assignments...)
		} else {
			copy := r
			byName[name] = &copy
			order = append(order, name)
		}
	}

	result := make([]RoleConfig, 0, len(order))
	for _, name := range order {
		result = append(result, *byName[name])
	}
	return result
}

// mergeConfig merges src into dst.
// - OAuth providers and roles are appended (later files can add more).
// - GeneralSettings fields from src override dst when set (non-nil).
func mergeConfig(dst, src *KiteConfig) {
	// Merge OAuth providers
	if src.OAuth != nil {
		if dst.OAuth == nil {
			dst.OAuth = &OAuthConfig{}
		}
		dst.OAuth.Providers = append(dst.OAuth.Providers, src.OAuth.Providers...)
	}

	// Merge roles
	dst.Roles = append(dst.Roles, src.Roles...)

	// Merge general settings (last-write-wins per field)
	if src.GeneralSettings != nil {
		if dst.GeneralSettings == nil {
			dst.GeneralSettings = &GeneralSettingsConfig{}
		}
		mergeGeneralSettings(dst.GeneralSettings, src.GeneralSettings)
	}
}

// mergeGeneralSettings copies non-nil fields from src to dst.
func mergeGeneralSettings(dst, src *GeneralSettingsConfig) {
	if src.AIAgentEnabled != nil {
		dst.AIAgentEnabled = src.AIAgentEnabled
	}
	if src.AIProvider != nil {
		dst.AIProvider = src.AIProvider
	}
	if src.AIModel != nil {
		dst.AIModel = src.AIModel
	}
	if src.AIBaseURL != nil {
		dst.AIBaseURL = src.AIBaseURL
	}
	if src.AIMaxTokens != nil {
		dst.AIMaxTokens = src.AIMaxTokens
	}
	if src.KubectlEnabled != nil {
		dst.KubectlEnabled = src.KubectlEnabled
	}
	if src.KubectlImage != nil {
		dst.KubectlImage = src.KubectlImage
	}
	if src.NodeTerminalImage != nil {
		dst.NodeTerminalImage = src.NodeTerminalImage
	}
	if src.EnableAnalytics != nil {
		dst.EnableAnalytics = src.EnableAnalytics
	}
	if src.EnableVersionCheck != nil {
		dst.EnableVersionCheck = src.EnableVersionCheck
	}
}
