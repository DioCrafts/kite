package plugin

import (
	"fmt"
	"testing"
)

// BenchmarkPermissionCheck measures the hot-path cost of checking a single
// plugin permission. This operation occurs on every plugin HTTP request.
func BenchmarkPermissionCheck(b *testing.B) {
	pe := NewPermissionEnforcer()
	pe.RegisterPlugin("bench-plugin", []Permission{
		{Resource: "pods", Verbs: []string{"get", "list", "watch"}},
		{Resource: "deployments", Verbs: []string{"get", "list", "update", "patch"}},
		{Resource: "secrets", Verbs: []string{"get"}},
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = pe.Check("bench-plugin", "pods", "get")
		}
	})
}

// BenchmarkPermissionCheck_Denied benchmarks the denied-access path (same
// cost as allowed but worth isolating to confirm parity).
func BenchmarkPermissionCheck_Denied(b *testing.B) {
	pe := NewPermissionEnforcer()
	pe.RegisterPlugin("bench-plugin", []Permission{
		{Resource: "pods", Verbs: []string{"list"}},
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = pe.Check("bench-plugin", "secrets", "delete")
		}
	})
}

// BenchmarkRateLimiterAllow measures the token-bucket Allow() call under
// load. Rate is set high enough that no request is ever denied.
func BenchmarkRateLimiterAllow(b *testing.B) {
	rl := NewPluginRateLimiter()
	rl.Register("bench-plugin", 1_000_000) // effectively unlimited

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = rl.Allow("bench-plugin")
		}
	})
}

// BenchmarkAllAITools benchmarks aggregating AI tools across N loaded plugins.
func BenchmarkAllAITools(b *testing.B) {
	pm := newBenchPluginManager(b, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pm.AllAITools()
	}
}

// BenchmarkAllResourceHandlers benchmarks aggregating resource handlers across
// N loaded plugins.
func BenchmarkAllResourceHandlers(b *testing.B) {
	pm := newBenchPluginManager(b, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pm.AllResourceHandlers()
	}
}

// BenchmarkAllFrontendManifests benchmarks collecting frontend manifests
// from N loaded plugins.
func BenchmarkAllFrontendManifests(b *testing.B) {
	pm := newBenchPluginManager(b, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pm.AllFrontendManifests()
	}
}

// newBenchPluginManager builds a PluginManager pre-populated with n loaded
// (but not running) plugins containing synthetic tool and handler data.
func newBenchPluginManager(b *testing.B, n int) *PluginManager {
	b.Helper()
	pm := NewPluginManager("")

	tools := []AITool{
		{Definition: AIToolDefinition{
			Name:        "query",
			Description: "run a query",
			Properties: map[string]interface{}{
				"q": map[string]interface{}{"type": "string"},
			},
		}},
	}
	handlers := map[string]ResourceHandler{
		"widget": &mockResourceHandler{},
	}
	frontend := &FrontendManifest{
		RemoteEntry: "/plugins/bench/remoteEntry.js",
	}

	for i := range n {
		name := fmt.Sprintf("bench-plugin-%d", i)
		pm.plugins[name] = &LoadedPlugin{
			Manifest: PluginManifest{
				Name:     name,
				Version:  "0.1.0",
				Frontend: frontend,
			},
			State:            PluginStateLoaded,
			AITools:          tools,
			ResourceHandlers: handlers,
		}
		pm.loadOrder = append(pm.loadOrder, name)
	}
	return pm
}
