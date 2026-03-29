package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

// DefaultPluginDir is the directory scanned for plugins when KITE_PLUGIN_DIR is unset.
const DefaultPluginDir = "./plugins"

// PluginState represents the runtime state of a loaded plugin.
type PluginState string

const (
	PluginStateLoaded   PluginState = "loaded"
	PluginStateFailed   PluginState = "failed"
	PluginStateDisabled PluginState = "disabled"
	PluginStateStopped  PluginState = "stopped"
)

// LoadedPlugin holds the runtime state for a single plugin.
type LoadedPlugin struct {
	Manifest PluginManifest
	State    PluginState
	Error    string // Non-empty if State == PluginStateFailed
	Dir      string // Absolute path to plugin directory

	// Runtime handles — populated after successful load
	AITools          []AITool
	ResourceHandlers map[string]ResourceHandler

	// Process client — populated when using gRPC mode (go-plugin)
	client *PluginClient

	mu sync.RWMutex
}

// PluginManager is responsible for discovering, loading, and managing
// the lifecycle of all Kite plugins.
type PluginManager struct {
	pluginDir string
	plugins   map[string]*LoadedPlugin
	loadOrder []string // Topologically sorted plugin names
	mu        sync.RWMutex

	// Permissions enforces capability-based access control per plugin.
	Permissions *PermissionEnforcer

	// RateLimiter enforces per-plugin request rate limits.
	RateLimiter *PluginRateLimiter
}

// NewPluginManager creates a new PluginManager.
// pluginDir is the directory to scan for plugins. If empty, DefaultPluginDir is used.
func NewPluginManager(pluginDir string) *PluginManager {
	if pluginDir == "" {
		pluginDir = DefaultPluginDir
	}
	return &PluginManager{
		pluginDir:   pluginDir,
		plugins:     make(map[string]*LoadedPlugin),
		Permissions: NewPermissionEnforcer(),
		RateLimiter: NewPluginRateLimiter(),
	}
}

// LoadPlugins discovers, validates, resolves dependencies, and loads all plugins.
func (pm *PluginManager) LoadPlugins() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	discovered, err := pm.discover()
	if err != nil {
		return fmt.Errorf("plugin discovery: %w", err)
	}

	if len(discovered) == 0 {
		klog.Info("No plugins found")
		return nil
	}

	// Resolve dependency order
	order, err := resolveDependencies(discovered)
	if err != nil {
		return fmt.Errorf("plugin dependency resolution: %w", err)
	}
	pm.loadOrder = order

	// Load plugins in dependency order
	for _, name := range order {
		lp := discovered[name]

		if err := pm.loadPlugin(lp); err != nil {
			lp.State = PluginStateFailed
			lp.Error = err.Error()
			klog.Errorf("Failed to load plugin %q: %v", name, err)
		} else {
			lp.State = PluginStateLoaded
			pm.Permissions.RegisterPlugin(name, lp.Manifest.Permissions)
			pm.RateLimiter.Register(name, lp.Manifest.RateLimit)
			klog.Infof("Plugin loaded: %s v%s", lp.Manifest.Name, lp.Manifest.Version)
		}

		pm.plugins[name] = lp
	}

	loaded := 0
	for _, lp := range pm.plugins {
		if lp.State == PluginStateLoaded {
			loaded++
		}
	}
	klog.Infof("Plugins: %d discovered, %d loaded", len(discovered), loaded)

	return nil
}

// GetPlugin returns a loaded plugin by name, or nil if not found.
func (pm *PluginManager) GetPlugin(name string) *LoadedPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.plugins[name]
}

// LoadedPlugins returns all successfully loaded plugins in dependency order.
func (pm *PluginManager) LoadedPlugins() []*LoadedPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var result []*LoadedPlugin
	for _, name := range pm.loadOrder {
		if lp, ok := pm.plugins[name]; ok && lp.State == PluginStateLoaded {
			result = append(result, lp)
		}
	}
	return result
}

// AllPlugins returns all discovered plugins (including failed ones).
func (pm *PluginManager) AllPlugins() []*LoadedPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]*LoadedPlugin, 0, len(pm.plugins))
	for _, lp := range pm.plugins {
		result = append(result, lp)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Manifest.Name < result[j].Manifest.Name
	})
	return result
}

// ShutdownAll gracefully shuts down all loaded plugins.
func (pm *PluginManager) ShutdownAll(ctx context.Context) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Shutdown in reverse dependency order
	for i := len(pm.loadOrder) - 1; i >= 0; i-- {
		name := pm.loadOrder[i]
		lp, ok := pm.plugins[name]
		if !ok || lp.State != PluginStateLoaded {
			continue
		}
		klog.Infof("Shutting down plugin: %s", name)
		lp.mu.Lock()
		if lp.client != nil {
			lp.client.Stop(ctx)
		}
		lp.State = PluginStateStopped
		lp.mu.Unlock()
	}

	klog.Info("All plugins shut down")
}

// BroadcastClusterEvent sends a cluster event to all loaded plugins.
func (pm *PluginManager) BroadcastClusterEvent(event ClusterEvent) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, name := range pm.loadOrder {
		lp, ok := pm.plugins[name]
		if !ok || lp.State != PluginStateLoaded {
			continue
		}
		lp.mu.RLock()
		if lp.client != nil && lp.client.IsAlive() {
			lp.client.KitePlugin().OnClusterEvent(event)
		}
		lp.mu.RUnlock()
		klog.V(2).Infof("Sent cluster event %s to plugin %s", event.Type, name)
	}
}

// AllAITools returns AI tool definitions from all loaded plugins.
func (pm *PluginManager) AllAITools() []AITool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var tools []AITool
	for _, name := range pm.loadOrder {
		lp, ok := pm.plugins[name]
		if !ok || lp.State != PluginStateLoaded {
			continue
		}
		lp.mu.RLock()
		tools = append(tools, lp.AITools...)
		lp.mu.RUnlock()
	}
	return tools
}

// AllResourceHandlers returns resource handlers from all loaded plugins,
// keyed as "plugin-<pluginName>-<handlerName>".
func (pm *PluginManager) AllResourceHandlers() map[string]ResourceHandler {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	handlers := make(map[string]ResourceHandler)
	for _, name := range pm.loadOrder {
		lp, ok := pm.plugins[name]
		if !ok || lp.State != PluginStateLoaded {
			continue
		}
		lp.mu.RLock()
		for hName, h := range lp.ResourceHandlers {
			key := "plugin-" + name + "-" + hName
			handlers[key] = h
		}
		lp.mu.RUnlock()
	}
	return handlers
}

// AllFrontendManifests returns frontend manifests from all loaded plugins.
func (pm *PluginManager) AllFrontendManifests() []FrontendManifestWithPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	manifests := make([]FrontendManifestWithPlugin, 0)
	for _, name := range pm.loadOrder {
		lp, ok := pm.plugins[name]
		if !ok || lp.State != PluginStateLoaded {
			continue
		}
		if lp.Manifest.Frontend != nil {
			manifests = append(manifests, FrontendManifestWithPlugin{
				PluginName: name,
				Frontend:   *lp.Manifest.Frontend,
			})
		}
	}
	return manifests
}

// FrontendManifestWithPlugin pairs a frontend manifest with its plugin name.
type FrontendManifestWithPlugin struct {
	PluginName string           `json:"pluginName"`
	Frontend   FrontendManifest `json:"frontend"`
}

// discover scans the plugin directory for valid plugin subdirectories.
func (pm *PluginManager) discover() (map[string]*LoadedPlugin, error) {
	plugins := make(map[string]*LoadedPlugin)

	absDir, err := filepath.Abs(pm.pluginDir)
	if err != nil {
		return nil, fmt.Errorf("resolve plugin dir: %w", err)
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		if os.IsNotExist(err) {
			klog.V(1).Infof("Plugin directory %s does not exist, skipping", absDir)
			return plugins, nil
		}
		return nil, fmt.Errorf("read plugin dir %s: %w", absDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginPath := filepath.Join(absDir, entry.Name())
		lp, err := discoverPlugin(pluginPath)
		if err != nil {
			klog.Warningf("Skipping directory %s: %v", entry.Name(), err)
			continue
		}

		if existing, ok := plugins[lp.Manifest.Name]; ok {
			klog.Warningf("Duplicate plugin name %q in %s and %s, using first",
				lp.Manifest.Name, existing.Dir, lp.Dir)
			continue
		}

		plugins[lp.Manifest.Name] = lp
		klog.Infof("Discovered plugin: %s v%s at %s", lp.Manifest.Name, lp.Manifest.Version, pluginPath)
	}

	return plugins, nil
}

// discoverPlugin reads and validates a single plugin directory.
func discoverPlugin(dir string) (*LoadedPlugin, error) {
	manifestPath := filepath.Join(dir, "manifest.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read manifest.yaml: %w", err)
	}

	var manifest PluginManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest.yaml: %w", err)
	}

	if err := validateManifest(&manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	// Check that plugin binary exists
	binaryPath := filepath.Join(dir, manifest.Name)
	if _, err := os.Stat(binaryPath); err != nil {
		return nil, fmt.Errorf("plugin binary %q not found: %w", manifest.Name, err)
	}

	// Apply defaults
	if manifest.Priority == 0 {
		manifest.Priority = 100
	}
	if manifest.RateLimit == 0 {
		manifest.RateLimit = 100
	}

	return &LoadedPlugin{
		Manifest: manifest,
		Dir:      dir,
	}, nil
}

// validateManifest checks required fields in a plugin manifest.
func validateManifest(m *PluginManifest) error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}

	// Validate semver format
	if _, err := parseSemver(m.Version); err != nil {
		return fmt.Errorf("invalid version %q: %w", m.Version, err)
	}

	// Validate dependency version constraints
	for _, dep := range m.Requires {
		if dep.Name == "" {
			return fmt.Errorf("dependency name is required")
		}
		if dep.Version == "" {
			return fmt.Errorf("dependency %q version is required", dep.Name)
		}
	}

	// Validate permissions
	validVerbs := map[string]bool{
		"get": true, "create": true, "update": true,
		"delete": true, "log": true, "exec": true,
	}
	for _, perm := range m.Permissions {
		if perm.Resource == "" {
			return fmt.Errorf("permission resource is required")
		}
		for _, verb := range perm.Verbs {
			if !validVerbs[verb] {
				return fmt.Errorf("invalid verb %q for resource %q", verb, perm.Resource)
			}
		}
	}

	return nil
}

// loadPlugin starts a plugin subprocess via go-plugin, connects over gRPC,
// and collects AI tools and resource handlers.
func (pm *PluginManager) loadPlugin(lp *LoadedPlugin) error {
	klog.V(1).Infof("Loading plugin %s from %s", lp.Manifest.Name, lp.Dir)

	// Start the plugin process
	pc, err := startPluginProcess(lp)
	if err != nil {
		return fmt.Errorf("start process: %w", err)
	}
	lp.client = pc

	kitePlugin := pc.KitePlugin()

	// Collect AI tool definitions and wrap with gRPC executors
	toolDefs := kitePlugin.RegisterAITools()
	lp.AITools = make([]AITool, 0, len(toolDefs))
	for _, td := range toolDefs {
		lp.AITools = append(lp.AITools, AITool{
			Definition: td,
			// Execute and Authorize are wired in Phase 2 when we integrate
			// with the AI subsystem and have cluster context available.
		})
	}

	// Collect resource handlers (gRPC-backed proxies)
	lp.ResourceHandlers = kitePlugin.RegisterResourceHandlers()
	if lp.ResourceHandlers == nil {
		lp.ResourceHandlers = make(map[string]ResourceHandler)
	}

	klog.V(1).Infof("Plugin %s: %d AI tools, %d resource handlers",
		lp.Manifest.Name, len(lp.AITools), len(lp.ResourceHandlers))

	return nil
}
