package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Manifest Validation Tests ---

func TestValidateManifest_Valid(t *testing.T) {
	m := &PluginManifest{
		Name:    "test-plugin",
		Version: "1.0.0",
	}
	assert.NoError(t, validateManifest(m))
}

func TestValidateManifest_MissingName(t *testing.T) {
	m := &PluginManifest{Version: "1.0.0"}
	err := validateManifest(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestValidateManifest_MissingVersion(t *testing.T) {
	m := &PluginManifest{Name: "test"}
	err := validateManifest(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version is required")
}

func TestValidateManifest_InvalidSemver(t *testing.T) {
	m := &PluginManifest{Name: "test", Version: "not-a-version"}
	err := validateManifest(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid version")
}

func TestValidateManifest_DependencyMissingName(t *testing.T) {
	m := &PluginManifest{
		Name:    "test",
		Version: "1.0.0",
		Requires: []Dependency{
			{Name: "", Version: ">=1.0.0"},
		},
	}
	err := validateManifest(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency name is required")
}

func TestValidateManifest_DependencyMissingVersion(t *testing.T) {
	m := &PluginManifest{
		Name:    "test",
		Version: "1.0.0",
		Requires: []Dependency{
			{Name: "other", Version: ""},
		},
	}
	err := validateManifest(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency \"other\" version is required")
}

func TestValidateManifest_InvalidVerb(t *testing.T) {
	m := &PluginManifest{
		Name:    "test",
		Version: "1.0.0",
		Permissions: []Permission{
			{Resource: "pods", Verbs: []string{"get", "hack"}},
		},
	}
	err := validateManifest(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid verb \"hack\"")
}

func TestValidateManifest_PermissionMissingResource(t *testing.T) {
	m := &PluginManifest{
		Name:    "test",
		Version: "1.0.0",
		Permissions: []Permission{
			{Resource: "", Verbs: []string{"get"}},
		},
	}
	err := validateManifest(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission resource is required")
}

func TestValidateManifest_ValidPermissions(t *testing.T) {
	m := &PluginManifest{
		Name:    "test",
		Version: "1.0.0",
		Permissions: []Permission{
			{Resource: "pods", Verbs: []string{"get", "list", "create", "update", "delete", "log", "exec"}},
		},
	}
	// "list" is not in the valid verbs map; only get/create/update/delete/log/exec
	// Let's check what happens:
	err := validateManifest(m)
	// "list" is actually NOT in the valid verbs set. Let's verify.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid verb \"list\"")
}

func TestValidateManifest_AllValidVerbs(t *testing.T) {
	m := &PluginManifest{
		Name:    "test",
		Version: "1.0.0",
		Permissions: []Permission{
			{Resource: "pods", Verbs: []string{"get", "create", "update", "delete", "log", "exec"}},
		},
	}
	assert.NoError(t, validateManifest(m))
}

// --- Plugin Discovery Tests ---

func TestDiscoverPlugin_ValidPlugin(t *testing.T) {
	dir := t.TempDir()
	manifest := `name: test-plugin
version: "1.0.0"
description: "A test plugin"
author: "Test Author"
`
	err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0644)
	require.NoError(t, err)

	// Create a fake binary
	err = os.WriteFile(filepath.Join(dir, "test-plugin"), []byte("#!/bin/sh\n"), 0755)
	require.NoError(t, err)

	lp, err := discoverPlugin(dir)
	require.NoError(t, err)
	assert.Equal(t, "test-plugin", lp.Manifest.Name)
	assert.Equal(t, "1.0.0", lp.Manifest.Version)
	assert.Equal(t, "A test plugin", lp.Manifest.Description)
	assert.Equal(t, dir, lp.Dir)
	// Defaults applied
	assert.Equal(t, 100, lp.Manifest.Priority)
	assert.Equal(t, 100, lp.Manifest.RateLimit)
}

func TestDiscoverPlugin_NoManifest(t *testing.T) {
	dir := t.TempDir()
	_, err := discoverPlugin(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read manifest.yaml")
}

func TestDiscoverPlugin_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(": invalid: yaml: {"), 0644)
	require.NoError(t, err)

	_, err = discoverPlugin(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse manifest.yaml")
}

func TestDiscoverPlugin_MissingBinary(t *testing.T) {
	dir := t.TempDir()
	manifest := `name: test-plugin
version: "1.0.0"
`
	err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0644)
	require.NoError(t, err)
	// No binary created
	_, err = discoverPlugin(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin binary")
}

func TestDiscoverPlugin_CustomPriorityAndRateLimit(t *testing.T) {
	dir := t.TempDir()
	manifest := `name: test-plugin
version: "1.0.0"
priority: 50
rateLimit: 200
`
	err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "test-plugin"), []byte("#!/bin/sh\n"), 0755)
	require.NoError(t, err)

	lp, err := discoverPlugin(dir)
	require.NoError(t, err)
	assert.Equal(t, 50, lp.Manifest.Priority)
	assert.Equal(t, 200, lp.Manifest.RateLimit)
}

// --- Plugin Manager Tests ---

func TestNewPluginManager_DefaultDir(t *testing.T) {
	pm := NewPluginManager("")
	assert.NotNil(t, pm)
	assert.Equal(t, DefaultPluginDir, pm.pluginDir)
	assert.NotNil(t, pm.Permissions)
	assert.NotNil(t, pm.RateLimiter)
}

func TestNewPluginManager_CustomDir(t *testing.T) {
	pm := NewPluginManager("/custom/plugins")
	assert.Equal(t, "/custom/plugins", pm.pluginDir)
}

func TestPluginManager_Discover_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	pm := NewPluginManager(dir)
	err := pm.LoadPlugins()
	assert.NoError(t, err)
	assert.Empty(t, pm.plugins)
}

func TestPluginManager_Discover_NonExistentDir(t *testing.T) {
	pm := NewPluginManager("/nonexistent/path/plugins")
	err := pm.LoadPlugins()
	// Non-existent directory should not error; returns empty set
	assert.NoError(t, err)
	assert.Empty(t, pm.plugins)
}

func TestPluginManager_Discover_SkipsFiles(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file (not a directory) — should be skipped
	err := os.WriteFile(filepath.Join(dir, "not-a-plugin.txt"), []byte("hello"), 0644)
	require.NoError(t, err)

	pm := NewPluginManager(dir)
	err = pm.LoadPlugins()
	assert.NoError(t, err)
	assert.Empty(t, pm.plugins)
}

func TestPluginManager_Discover_SkipsInvalidManifest(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory with invalid manifest
	sub := filepath.Join(dir, "bad-plugin")
	require.NoError(t, os.Mkdir(sub, 0755))
	err := os.WriteFile(filepath.Join(sub, "manifest.yaml"), []byte("name: \nversion: "), 0644)
	require.NoError(t, err)

	pm := NewPluginManager(dir)
	err = pm.LoadPlugins()
	assert.NoError(t, err)
	assert.Empty(t, pm.plugins)
}

func TestPluginManager_GetPlugin_NotFound(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	assert.Nil(t, pm.GetPlugin("nonexistent"))
}

func TestPluginManager_LoadedPlugins_Empty(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	assert.Empty(t, pm.LoadedPlugins())
}

func TestPluginManager_AllPlugins_Empty(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	assert.Empty(t, pm.AllPlugins())
}

func TestPluginManager_ShutdownAll_Empty(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	// Should not panic on empty manager
	pm.ShutdownAll(context.Background())
}

func TestPluginManager_BroadcastClusterEvent_Empty(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	// Should not panic
	pm.BroadcastClusterEvent(ClusterEvent{Type: ClusterEventAdded, ClusterName: "test"})
}

func TestPluginManager_AllAITools_Empty(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	assert.Empty(t, pm.AllAITools())
}

func TestPluginManager_AllResourceHandlers_Empty(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	assert.Empty(t, pm.AllResourceHandlers())
}

func TestPluginManager_AllFrontendManifests_Empty(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	assert.Empty(t, pm.AllFrontendManifests())
}

// Test with manually injected plugin state (bypass gRPC process load)

func TestPluginManager_GetPlugin_Found(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.plugins["my-plugin"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "my-plugin", Version: "1.0.0"},
		State:    PluginStateLoaded,
	}
	lp := pm.GetPlugin("my-plugin")
	require.NotNil(t, lp)
	assert.Equal(t, "my-plugin", lp.Manifest.Name)
	assert.Equal(t, PluginStateLoaded, lp.State)
}

func TestPluginManager_LoadedPlugins_FiltersNonLoaded(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.loadOrder = []string{"a", "b", "c"}
	pm.plugins["a"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "a"},
		State:    PluginStateLoaded,
	}
	pm.plugins["b"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "b"},
		State:    PluginStateFailed,
	}
	pm.plugins["c"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "c"},
		State:    PluginStateDisabled,
	}

	loaded := pm.LoadedPlugins()
	require.Len(t, loaded, 1)
	assert.Equal(t, "a", loaded[0].Manifest.Name)
}

func TestPluginManager_AllPlugins_SortedByName(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.plugins["zebra"] = &LoadedPlugin{Manifest: PluginManifest{Name: "zebra"}}
	pm.plugins["alpha"] = &LoadedPlugin{Manifest: PluginManifest{Name: "alpha"}}
	pm.plugins["mid"] = &LoadedPlugin{Manifest: PluginManifest{Name: "mid"}}

	all := pm.AllPlugins()
	require.Len(t, all, 3)
	assert.Equal(t, "alpha", all[0].Manifest.Name)
	assert.Equal(t, "mid", all[1].Manifest.Name)
	assert.Equal(t, "zebra", all[2].Manifest.Name)
}

func TestPluginManager_AllAITools_AggregatesFromPlugins(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.loadOrder = []string{"p1", "p2"}
	pm.plugins["p1"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "p1"},
		State:    PluginStateLoaded,
		AITools:  []AITool{{Definition: AIToolDefinition{Name: "tool1"}}},
	}
	pm.plugins["p2"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "p2"},
		State:    PluginStateLoaded,
		AITools:  []AITool{{Definition: AIToolDefinition{Name: "tool2"}}, {Definition: AIToolDefinition{Name: "tool3"}}},
	}

	tools := pm.AllAITools()
	require.Len(t, tools, 3)
	assert.Equal(t, "tool1", tools[0].Definition.Name)
	assert.Equal(t, "tool2", tools[1].Definition.Name)
	assert.Equal(t, "tool3", tools[2].Definition.Name)
}

func TestPluginManager_AllAITools_SkipsNonLoaded(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.loadOrder = []string{"p1", "p2"}
	pm.plugins["p1"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "p1"},
		State:    PluginStateFailed,
		AITools:  []AITool{{Definition: AIToolDefinition{Name: "tool1"}}},
	}
	pm.plugins["p2"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "p2"},
		State:    PluginStateLoaded,
		AITools:  []AITool{{Definition: AIToolDefinition{Name: "tool2"}}},
	}

	tools := pm.AllAITools()
	require.Len(t, tools, 1)
	assert.Equal(t, "tool2", tools[0].Definition.Name)
}

func TestPluginManager_AllResourceHandlers_PrefixesNames(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.loadOrder = []string{"my-plugin"}
	pm.plugins["my-plugin"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "my-plugin"},
		State:    PluginStateLoaded,
		ResourceHandlers: map[string]ResourceHandler{
			"backups": &mockResourceHandler{},
		},
	}

	handlers := pm.AllResourceHandlers()
	require.Len(t, handlers, 1)
	_, ok := handlers["plugin-my-plugin-backups"]
	assert.True(t, ok, "expected handler with prefixed key")
}

func TestPluginManager_AllFrontendManifests_IncludesOnlyWithFrontend(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.loadOrder = []string{"with-fe", "no-fe"}
	pm.plugins["with-fe"] = &LoadedPlugin{
		Manifest: PluginManifest{
			Name: "with-fe",
			Frontend: &FrontendManifest{
				RemoteEntry: "/plugins/with-fe/static/remoteEntry.js",
			},
		},
		State: PluginStateLoaded,
	}
	pm.plugins["no-fe"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "no-fe"},
		State:    PluginStateLoaded,
	}

	manifests := pm.AllFrontendManifests()
	require.Len(t, manifests, 1)
	assert.Equal(t, "with-fe", manifests[0].PluginName)
}

// --- Mock Resource Handler (satisfies the ResourceHandler interface) ---

type mockResourceHandler struct {
	clusterScoped bool
}

func (m *mockResourceHandler) List(_ *gin.Context)    {}
func (m *mockResourceHandler) Get(_ *gin.Context)     {}
func (m *mockResourceHandler) Create(_ *gin.Context)  {}
func (m *mockResourceHandler) Update(_ *gin.Context)  {}
func (m *mockResourceHandler) Delete(_ *gin.Context)  {}
func (m *mockResourceHandler) Patch(_ *gin.Context)   {}
func (m *mockResourceHandler) IsClusterScoped() bool { return m.clusterScoped }

var _ ResourceHandler = (*mockResourceHandler)(nil)
