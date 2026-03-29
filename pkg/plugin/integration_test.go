package plugin

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestRouter creates a minimal Gin router for integration tests.
// It wires up plugin HTTP proxying at /api/v1/plugins/:pluginName/*path.
func newTestRouter(pm *PluginManager) *gin.Engine {
	r := gin.New()
	api := r.Group("/api/v1/plugins")
	api.Any("/:pluginName/*path", func(c *gin.Context) {
		pluginName := c.Param("pluginName")
		pm.HandlePluginHTTP(c, pluginName)
	})
	return r
}

// --- Handle Plugin HTTP: plugin not found ---

func TestHandlePluginHTTP_PluginNotFound(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	r := newTestRouter(pm)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/nonexistent/data", nil)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "not found")
}

// --- Handle Plugin HTTP: plugin unavailable (not loaded) ---

func TestHandlePluginHTTP_PluginNotLoaded(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	// Register permissions so the permission check passes
	pm.Permissions.RegisterPlugin("broken", []Permission{
		{Resource: "data", Verbs: []string{"get"}},
	})
	// Insert a plugin in "failed" state — no running process
	pm.plugins["broken"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "broken", Version: "1.0.0"},
		State:    PluginStateFailed,
		client:   nil,
	}
	r := newTestRouter(pm)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/broken/data", nil)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// --- Handle Plugin HTTP: permission denied ---

func TestHandlePluginHTTP_PermissionDenied(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.Permissions.RegisterPlugin("locked-plugin", []Permission{
		{Resource: "pods", Verbs: []string{"get"}},
	})
	pm.plugins["locked-plugin"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "locked-plugin", Version: "1.0.0"},
		State:    PluginStateLoaded,
		client:   nil, // nil — will hit permission check before client check
	}
	r := newTestRouter(pm)

	// POST to "pods" resource — not permitted (only "get" is allowed, POST→"create")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/locked-plugin/pods", nil)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "not permitted")
}

// --- Handle Plugin HTTP: rate limit exceeded ---

func TestHandlePluginHTTP_RateLimitExceeded(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	// Register permissions so the permission check passes
	pm.Permissions.RegisterPlugin("rate-plugin", []Permission{
		{Resource: "data", Verbs: []string{"get", "list"}},
	})
	// Register plugin with 1 request/second, burst=2
	pm.RateLimiter.Register("rate-plugin", 1)
	pm.plugins["rate-plugin"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "rate-plugin", Version: "1.0.0"},
		State:    PluginStateLoaded,
		client:   nil,
	}
	r := newTestRouter(pm)

	// Exhaust the burst capacity
	var lastCode int
	for i := 0; i < 10; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/rate-plugin/data", nil)
		r.ServeHTTP(rec, req)
		lastCode = rec.Code
	}
	// After exhausting burst (2 tokens), we should hit 429 before 503
	assert.Equal(t, http.StatusTooManyRequests, lastCode)
}

// --- ExecutePluginTool: invalid tool name format ---

func TestExecutePluginTool_InvalidName(t *testing.T) {
	pm := NewPluginManager(t.TempDir())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	result, isError := pm.ExecutePluginTool(c.Request.Context(), c, "bad_name", nil)
	assert.True(t, isError)
	assert.Contains(t, result, "Invalid plugin tool name")
}

func TestExecutePluginTool_WrongPrefix(t *testing.T) {
	pm := NewPluginManager(t.TempDir())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	result, isError := pm.ExecutePluginTool(c.Request.Context(), c, "notplugin_foo_bar", nil)
	assert.True(t, isError)
	assert.Contains(t, result, "Invalid plugin tool name")
}

func TestExecutePluginTool_PluginNotFound(t *testing.T) {
	pm := NewPluginManager(t.TempDir())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	result, isError := pm.ExecutePluginTool(c.Request.Context(), c, "plugin_missing_tool", nil)
	assert.True(t, isError)
	assert.Contains(t, result, "not found")
}

func TestExecutePluginTool_PluginNotAvailable(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.plugins["myplugin"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "myplugin"},
		State:    PluginStateFailed,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	result, isError := pm.ExecutePluginTool(c.Request.Context(), c, "plugin_myplugin_get_cost", nil)
	assert.True(t, isError)
	assert.Contains(t, result, "not available")
}

// --- extractResourceFromPath ---

func TestExtractResourceFromPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/pods", "pods"},
		{"/pods/my-pod", "pods"},
		{"/", ""},
		{"", ""},
		{"/deployments/nginx/scale", "deployments"},
		{"pods", "pods"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractResourceFromPath(tt.path)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// --- AI Tools: AllAITools aggregation ---

func TestAllAITools_MultiplePluigns(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.loadOrder = []string{"a", "b"}
	pm.plugins["a"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "a"},
		State:    PluginStateLoaded,
		AITools: []AITool{
			{Definition: AIToolDefinition{Name: "get_cost", Description: "Get cost"}},
		},
	}
	pm.plugins["b"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "b"},
		State:    PluginStateLoaded,
		AITools: []AITool{
			{Definition: AIToolDefinition{Name: "list_backups", Description: "List backups"}},
			{Definition: AIToolDefinition{Name: "create_backup", Description: "Create backup"}},
		},
	}

	tools := pm.AllAITools()
	require.Len(t, tools, 3)
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Definition.Name
	}
	assert.Contains(t, names, "get_cost")
	assert.Contains(t, names, "list_backups")
	assert.Contains(t, names, "create_backup")
}

// --- Resource Handlers ---

func TestAllResourceHandlers_ConflictsPrevented(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.loadOrder = []string{"plugin-a", "plugin-b"}
	pm.plugins["plugin-a"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "plugin-a"},
		State:    PluginStateLoaded,
		ResourceHandlers: map[string]ResourceHandler{
			"backups": &mockResourceHandler{},
		},
	}
	pm.plugins["plugin-b"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "plugin-b"},
		State:    PluginStateLoaded,
		ResourceHandlers: map[string]ResourceHandler{
			"backups": &mockResourceHandler{},
		},
	}

	// Each plugin's "backups" handler gets a unique key with plugin prefix
	handlers := pm.AllResourceHandlers()
	require.Len(t, handlers, 2)
	_, hasA := handlers["plugin-plugin-a-backups"]
	_, hasB := handlers["plugin-plugin-b-backups"]
	assert.True(t, hasA, "expected plugin-plugin-a-backups")
	assert.True(t, hasB, "expected plugin-plugin-b-backups")
}

// --- Rate Limiter: token bucket behavior ---

func TestRateLimiter_AllowsWithinBurst(t *testing.T) {
	rl := NewPluginRateLimiter()
	rl.Register("myplugin", 10) // 10/s, burst=20

	// Should allow burst capacity (20) immediately
	for i := 0; i < 20; i++ {
		assert.True(t, rl.Allow("myplugin"), "request %d should be allowed within burst", i+1)
	}
	// 21st request should be denied (burst exhausted)
	assert.False(t, rl.Allow("myplugin"))
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	rl := NewPluginRateLimiter()
	rl.Register("myplugin", 100) // 100/s, burst=200

	// Exhaust the burst
	for i := 0; i < 200; i++ {
		rl.Allow("myplugin")
	}
	assert.False(t, rl.Allow("myplugin"))

	// Wait for refill (100ms = ~10 tokens at 100/s)
	time.Sleep(100 * time.Millisecond)
	assert.True(t, rl.Allow("myplugin"), "should be allowed after refill")
}

func TestRateLimiter_UnknownPlugin_Allowed(t *testing.T) {
	rl := NewPluginRateLimiter()
	assert.True(t, rl.Allow("does-not-exist"))
}

func TestRateLimiter_Unregister(t *testing.T) {
	rl := NewPluginRateLimiter()
	rl.Register("myplugin", 1) // 1/s = burst 2
	rl.Unregister("myplugin")
	// After unregister, should always allow (no bucket)
	for i := 0; i < 100; i++ {
		assert.True(t, rl.Allow("myplugin"))
	}
}

// --- RateLimitMiddleware ---

func TestRateLimitMiddleware_ExceedsLimit(t *testing.T) {
	rl := NewPluginRateLimiter()
	rl.Register("fast-plugin", 1) // 1/s, burst=2

	r := gin.New()
	r.Use(rl.RateLimitMiddleware(func(c *gin.Context) string {
		return "fast-plugin"
	}))
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	var lastCode int
	for i := 0; i < 10; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(rec, req)
		lastCode = rec.Code
	}
	assert.Equal(t, http.StatusTooManyRequests, lastCode)
}

// --- Permission Enforcer: comprehensive verb/resource checks ---

func TestPermissionEnforcer_CheckMethod_GET(t *testing.T) {
	pe := NewPermissionEnforcer()
	pe.RegisterPlugin("plugin", []Permission{
		{Resource: "pods", Verbs: []string{"get"}},
	})
	assert.NoError(t, pe.CheckHTTPMethod("plugin", "pods", http.MethodGet))
}

func TestPermissionEnforcer_CheckMethod_POST_AsCreate(t *testing.T) {
	pe := NewPermissionEnforcer()
	pe.RegisterPlugin("plugin", []Permission{
		{Resource: "deployments", Verbs: []string{"create"}},
	})
	assert.NoError(t, pe.CheckHTTPMethod("plugin", "deployments", http.MethodPost))
}

func TestPermissionEnforcer_CheckMethod_DELETE_Denied(t *testing.T) {
	pe := NewPermissionEnforcer()
	pe.RegisterPlugin("plugin", []Permission{
		{Resource: "pods", Verbs: []string{"get"}},
	})
	err := pe.CheckHTTPMethod("plugin", "pods", http.MethodDelete)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not permitted")
}

func TestPermissionEnforcer_UnknownPlugin(t *testing.T) {
	pe := NewPermissionEnforcer()
	err := pe.Check("nonexistent", "pods", "get")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no registered permissions")
}

func TestPermissionEnforcer_UnknownResource(t *testing.T) {
	pe := NewPermissionEnforcer()
	pe.RegisterPlugin("plugin", []Permission{
		{Resource: "pods", Verbs: []string{"get"}},
	})
	err := pe.Check("plugin", "secrets", "get")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not permitted to access resource")
}

func TestPermissionEnforcer_UnregisterPlugin(t *testing.T) {
	pe := NewPermissionEnforcer()
	pe.RegisterPlugin("plugin", []Permission{
		{Resource: "pods", Verbs: []string{"get"}},
	})
	pe.UnregisterPlugin("plugin")
	err := pe.Check("plugin", "pods", "get")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no registered permissions")
}

func TestPermissionEnforcer_CaseInsensitive(t *testing.T) {
	pe := NewPermissionEnforcer()
	pe.RegisterPlugin("plugin", []Permission{
		{Resource: "Pods", Verbs: []string{"GET"}},
	})
	assert.NoError(t, pe.Check("plugin", "PODS", "get"))
	assert.NoError(t, pe.Check("plugin", "pods", "GET"))
}

// --- SetPluginEnabled ---

func TestPluginManager_SetPluginEnabled_NotFound(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	err := pm.SetPluginEnabled("ghost", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPluginManager_SetPluginEnabled_Disable(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.Permissions.RegisterPlugin("p", []Permission{{Resource: "pods", Verbs: []string{"get"}}})
	pm.RateLimiter.Register("p", 10)
	pm.plugins["p"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "p"},
		State:    PluginStateLoaded,
		client:   nil, // no real process
	}

	err := pm.SetPluginEnabled("p", false)
	require.NoError(t, err)
	assert.Equal(t, PluginStateDisabled, pm.plugins["p"].State)
	// After disable, permissions should be removed
	assert.Error(t, pm.Permissions.Check("p", "pods", "get"))
	// Rate limiter bucket removed — unknown plugin always allows
	assert.True(t, pm.RateLimiter.Allow("p"))
}

// --- ReloadPlugin: not found ---

func TestPluginManager_ReloadPlugin_NotFound(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	err := pm.ReloadPlugin("ghost")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- BroadcastClusterEvent ---

func TestPluginManager_BroadcastClusterEvent_SkipsNonLoaded(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.loadOrder = []string{"ok", "bad"}
	pm.plugins["ok"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "ok"},
		State:    PluginStateLoaded,
		// nil client — will be skipped (IsAlive returns false)
	}
	pm.plugins["bad"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "bad"},
		State:    PluginStateFailed,
	}

	// Should not panic even with nil clients
	assert.NotPanics(t, func() {
		pm.BroadcastClusterEvent(ClusterEvent{Type: ClusterEventAdded, ClusterName: "test"})
	})
}

// --- ShutdownAll: stops plugins in reverse order ---

func TestPluginManager_ShutdownAll_ReverseOrder(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.loadOrder = []string{"base", "dependent"}
	pm.plugins["base"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "base"},
		State:    PluginStateLoaded,
	}
	pm.plugins["dependent"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "dependent"},
		State:    PluginStateLoaded,
	}

	// Should not panic with nil clients (no real processes)
	assert.NotPanics(t, func() {
		pm.ShutdownAll(t.Context())
	})

	// Both should be marked stopped
	assert.Equal(t, PluginStateStopped, pm.plugins["base"].State)
	assert.Equal(t, PluginStateStopped, pm.plugins["dependent"].State)
}

// --- FrontendManifests ---

func TestAllFrontendManifests_MultiplePlugins(t *testing.T) {
	pm := NewPluginManager(t.TempDir())
	pm.loadOrder = []string{"cost-analyzer", "backup-manager", "backend-only"}
	pm.plugins["cost-analyzer"] = &LoadedPlugin{
		Manifest: PluginManifest{
			Name: "cost-analyzer",
			Frontend: &FrontendManifest{
				RemoteEntry: "/plugins/cost-analyzer/remoteEntry.js",
				Routes: []FrontendRoute{
					{Path: "/cost", Module: "./CostDashboard"},
				},
			},
		},
		State: PluginStateLoaded,
	}
	pm.plugins["backup-manager"] = &LoadedPlugin{
		Manifest: PluginManifest{
			Name: "backup-manager",
			Frontend: &FrontendManifest{
				RemoteEntry: "/plugins/backup-manager/remoteEntry.js",
			},
		},
		State: PluginStateLoaded,
	}
	pm.plugins["backend-only"] = &LoadedPlugin{
		Manifest: PluginManifest{Name: "backend-only"},
		State:    PluginStateLoaded,
	}

	manifests := pm.AllFrontendManifests()
	require.Len(t, manifests, 2)

	names := make([]string, len(manifests))
	for i, m := range manifests {
		names[i] = m.PluginName
	}
	assert.Contains(t, names, "cost-analyzer")
	assert.Contains(t, names, "backup-manager")
	assert.NotContains(t, names, "backend-only")
}

// --- PluginManager discover: duplicate name ---

func TestPluginManager_Discover_DuplicateName(t *testing.T) {
	dir := t.TempDir()
	manifest := `name: shared-name
version: "1.0.0"
`
	// Create two directories with the same plugin name
	for _, sub := range []string{"plugin-dir-a", "plugin-dir-b"} {
		d := filepath.Join(dir, sub)
		require.NoError(t, os.Mkdir(d, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(d, "manifest.yaml"), []byte(manifest), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(d, "shared-name"), []byte("#!/bin/sh\n"), 0755))
	}

	pm := NewPluginManager(dir)
	// discover() returns a map — won't error, but logs a warning and uses the first
	discovered, err := pm.discover()
	require.NoError(t, err)
	// Only one gets into the map
	assert.Len(t, discovered, 1)
	assert.Equal(t, "shared-name", discovered["shared-name"].Manifest.Name)
}

// --- http verb → kubernetes verb mapping ---

func TestHTTPMethodToVerb(t *testing.T) {
	tests := []struct {
		method string
		verb   string
	}{
		{http.MethodGet, "get"},
		{http.MethodHead, "get"},
		{http.MethodPost, "create"},
		{http.MethodPut, "update"},
		{http.MethodPatch, "update"},
		{http.MethodDelete, "delete"},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			got := httpMethodToVerb(tt.method)
			assert.Equal(t, tt.verb, got)
		})
	}
}

func TestPermissionEnforcer_PluginPermissions(t *testing.T) {
	pe := NewPermissionEnforcer()
	perms := []Permission{
		{Resource: "pods", Verbs: []string{"get", "create"}},
		{Resource: "nodes", Verbs: []string{"get"}},
	}
	pe.RegisterPlugin("plugin", perms)
	got := pe.PluginPermissions("plugin")
	require.Len(t, got, 2)

	resourceSet := make(map[string]bool)
	for _, p := range got {
		resourceSet[p.Resource] = true
	}
	assert.True(t, resourceSet["pods"])
	assert.True(t, resourceSet["nodes"])
}

func TestPermissionEnforcer_PluginPermissions_NotRegistered(t *testing.T) {
	pe := NewPermissionEnforcer()
	got := pe.PluginPermissions("nobody")
	assert.Nil(t, got)
}

// --- Sanitized env helper ---

func TestSanitizedEnvForPlugin_ExcludesSensitiveVars(t *testing.T) {
	t.Setenv("JWT_SECRET", "super-secret")
	t.Setenv("DB_DSN", "postgres://...")
	t.Setenv("KITE_ENCRYPT_KEY", "my-key")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("HOME", "/root")

	env := sanitizedEnvForPlugin("test-plugin")

	// Build a quick lookup
	envMap := make(map[string]bool)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = true
		}
	}

	// Sensitive vars must be stripped
	assert.False(t, envMap["JWT_SECRET"], "JWT_SECRET should be excluded")
	assert.False(t, envMap["DB_DSN"], "DB_DSN should be excluded")
	assert.False(t, envMap["KITE_ENCRYPT_KEY"], "KITE_ENCRYPT_KEY should be excluded")
	assert.False(t, envMap["OPENAI_API_KEY"], "OPENAI_API_KEY should be excluded")

	// Safe vars should be present
	assert.True(t, envMap["HOME"], "HOME should be included")

	// KITE_PLUGIN_NAME should be injected
	assert.True(t, envMap["KITE_PLUGIN_NAME"], "KITE_PLUGIN_NAME should be injected")
}
