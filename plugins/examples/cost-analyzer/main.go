package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/plugin"
	"github.com/zxh326/kite/pkg/plugin/sdk"
)

// CostAnalyzerPlugin estimates Kubernetes resource costs per namespace.
type CostAnalyzerPlugin struct {
	sdk.BasePlugin

	mu       sync.RWMutex
	settings costSettings
}

type costSettings struct {
	CPUPricePerHour       float64 `json:"cpuPricePerHour"`
	MemoryPricePerGBHour  float64 `json:"memoryPricePerGBHour"`
}

func defaultSettings() costSettings {
	return costSettings{
		CPUPricePerHour:      0.05,
		MemoryPricePerGBHour: 0.01,
	}
}

// namespaceCost represents estimated cost breakdown for a single namespace.
type namespaceCost struct {
	Namespace   string  `json:"namespace"`
	CPUCores    float64 `json:"cpuCores"`
	MemoryGB    float64 `json:"memoryGB"`
	CPUCost     float64 `json:"cpuCostPerHour"`
	MemoryCost  float64 `json:"memoryCostPerHour"`
	TotalCost   float64 `json:"totalCostPerHour"`
	PodCount    int     `json:"podCount"`
}

func (p *CostAnalyzerPlugin) Manifest() plugin.PluginManifest {
	return plugin.PluginManifest{
		Name:        "cost-analyzer",
		Version:     "1.0.0",
		Description: "Kubernetes cost estimation by namespace",
		Author:      "Kite Team",
		Permissions: []plugin.Permission{
			{Resource: "pods", Verbs: []string{"get", "list"}},
			{Resource: "nodes", Verbs: []string{"get", "list"}},
		},
		Frontend: &plugin.FrontendManifest{
			RemoteEntry: "/plugins/cost-analyzer/static/remoteEntry.js",
			ExposedModules: map[string]string{
				"./CostDashboard": "CostDashboard",
				"./CostSettings":  "CostSettings",
			},
			Routes: []plugin.FrontendRoute{
				{
					Path:   "/cost",
					Module: "./CostDashboard",
					SidebarEntry: &plugin.SidebarEntry{
						Title:   "Cost Analysis",
						Icon:    "currency-dollar",
						Section: "observability",
					},
				},
			},
			SettingsPanel: "./CostSettings",
		},
		Settings: []plugin.SettingField{
			{Name: "cpuPricePerHour", Type: "number", Default: "0.05", Label: "CPU price per core/hour (USD)"},
			{Name: "memoryPricePerGBHour", Type: "number", Default: "0.01", Label: "Memory price per GB/hour (USD)"},
		},
	}
}

func (p *CostAnalyzerPlugin) RegisterRoutes(group gin.IRoutes) {
	group.GET("/costs", p.handleCosts)
	group.GET("/costs/summary", p.handleCostsSummary)
	group.PUT("/settings", p.handleUpdateSettings)
}

func (p *CostAnalyzerPlugin) RegisterAITools() []plugin.AIToolDefinition {
	return []plugin.AIToolDefinition{
		sdk.NewAITool(
			"get_namespace_cost",
			"Calculate the estimated hourly cost for a Kubernetes namespace based on CPU and memory requests of running pods",
			map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "The Kubernetes namespace to calculate costs for",
				},
			},
			[]string{"namespace"},
		),
	}
}

func (p *CostAnalyzerPlugin) Shutdown(ctx context.Context) error {
	sdk.Logger().Info("cost-analyzer plugin shutting down")
	return nil
}

// handleCosts returns cost estimation for a specific namespace or all namespaces.
func (p *CostAnalyzerPlugin) handleCosts(c *gin.Context) {
	namespace := c.Query("namespace")

	p.mu.RLock()
	settings := p.settings
	p.mu.RUnlock()

	// In a real implementation, this would query the Kubernetes API
	// via the cluster context to get actual pod resource requests.
	// For this example, we return simulated data.
	costs := p.simulateCosts(namespace, settings)

	c.JSON(http.StatusOK, gin.H{
		"costs":    costs,
		"settings": settings,
	})
}

// handleCostsSummary returns an aggregated cost summary across all namespaces.
func (p *CostAnalyzerPlugin) handleCostsSummary(c *gin.Context) {
	p.mu.RLock()
	settings := p.settings
	p.mu.RUnlock()

	allCosts := p.simulateCosts("", settings)

	var totalCPU, totalMemory, totalCost float64
	var totalPods int
	for _, nc := range allCosts {
		totalCPU += nc.CPUCores
		totalMemory += nc.MemoryGB
		totalCost += nc.TotalCost
		totalPods += nc.PodCount
	}

	c.JSON(http.StatusOK, gin.H{
		"namespaces":       allCosts,
		"totalCPUCores":    totalCPU,
		"totalMemoryGB":    totalMemory,
		"totalCostPerHour": totalCost,
		"totalPods":        totalPods,
	})
}

// handleUpdateSettings updates cost calculation settings.
func (p *CostAnalyzerPlugin) handleUpdateSettings(c *gin.Context) {
	var newSettings costSettings
	if err := c.ShouldBindJSON(&newSettings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid settings: " + err.Error()})
		return
	}

	if newSettings.CPUPricePerHour < 0 || newSettings.MemoryPricePerGBHour < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prices must be non-negative"})
		return
	}

	p.mu.Lock()
	p.settings = newSettings
	p.mu.Unlock()

	sdk.Logger().Info("settings updated",
		"cpuPrice", newSettings.CPUPricePerHour,
		"memoryPrice", newSettings.MemoryPricePerGBHour,
	)

	c.JSON(http.StatusOK, gin.H{"message": "settings updated", "settings": newSettings})
}

// simulateCosts generates simulated cost data.
// In a production plugin, this would query the Kubernetes API for real pod specs.
func (p *CostAnalyzerPlugin) simulateCosts(namespace string, settings costSettings) []namespaceCost {
	// Simulated namespace data — a real implementation would iterate
	// over pods from the cluster client and sum resource requests.
	simulated := []namespaceCost{
		{Namespace: "default", CPUCores: 2.5, MemoryGB: 4.0, PodCount: 8},
		{Namespace: "production", CPUCores: 12.0, MemoryGB: 24.0, PodCount: 35},
		{Namespace: "staging", CPUCores: 4.0, MemoryGB: 8.0, PodCount: 15},
		{Namespace: "monitoring", CPUCores: 3.0, MemoryGB: 6.0, PodCount: 10},
		{Namespace: "kube-system", CPUCores: 1.5, MemoryGB: 3.0, PodCount: 12},
	}

	var result []namespaceCost
	for _, nc := range simulated {
		if namespace != "" && nc.Namespace != namespace {
			continue
		}
		nc.CPUCost = nc.CPUCores * settings.CPUPricePerHour
		nc.MemoryCost = nc.MemoryGB * settings.MemoryPricePerGBHour
		nc.TotalCost = nc.CPUCost + nc.MemoryCost
		result = append(result, nc)
	}
	return result
}

// AI tool execution — called via gRPC when the Kite AI agent invokes get_namespace_cost.
// The tool receives arguments as a JSON string and returns a human-readable result.
// In the real flow, this is handled by the gRPC server-side implementation;
// the helper below shows the computation logic that would be wired in.
func (p *CostAnalyzerPlugin) executeGetNamespaceCost(args map[string]any) (string, error) {
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("namespace parameter is required")
	}

	p.mu.RLock()
	settings := p.settings
	p.mu.RUnlock()

	costs := p.simulateCosts(namespace, settings)
	if len(costs) == 0 {
		return fmt.Sprintf("No cost data found for namespace %q", namespace), nil
	}

	nc := costs[0]
	result := fmt.Sprintf(
		"Cost estimate for namespace %q:\n"+
			"  Pods: %d\n"+
			"  CPU: %.1f cores → $%.4f/hour\n"+
			"  Memory: %.1f GB → $%.4f/hour\n"+
			"  Total: $%.4f/hour ($%.2f/month est.)",
		nc.Namespace, nc.PodCount,
		nc.CPUCores, nc.CPUCost,
		nc.MemoryGB, nc.MemoryCost,
		nc.TotalCost, nc.TotalCost*730,
	)
	return result, nil
}

func main() {
	p := &CostAnalyzerPlugin{
		settings: defaultSettings(),
	}

	// Load settings from environment if provided (via plugin settings API)
	if v := getEnvFloat("COST_CPU_PRICE", 0); v > 0 {
		p.settings.CPUPricePerHour = v
	}
	if v := getEnvFloat("COST_MEMORY_PRICE", 0); v > 0 {
		p.settings.MemoryPricePerGBHour = v
	}

	sdk.Serve(p)
}

func getEnvFloat(key string, fallback float64) float64 {
	if v, ok := lookupEnv(key); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func lookupEnv(key string) (string, bool) {
	// We intentionally don't import "os" for this — in the sandboxed
	// plugin environment, env vars are curated by Kite. This is a
	// compile-time placeholder; the real values come via the settings API.
	return "", false
}

// Ensure unused imports are consumed
var _ = json.Marshal
