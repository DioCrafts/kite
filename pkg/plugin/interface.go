package plugin

import (
	"context"

	"github.com/gin-gonic/gin"
)

// KitePlugin is the contract that every Kite plugin must implement.
// Backend plugins are loaded as separate processes via HashiCorp go-plugin
// and communicate with Kite over gRPC (stdio transport).
type KitePlugin interface {
	// Manifest returns the plugin's metadata, permissions, and frontend config.
	Manifest() PluginManifest

	// RegisterRoutes adds custom HTTP endpoints under /api/v1/plugins/<name>/.
	// The provided router group is already scoped to the plugin path with
	// auth and cluster middleware applied.
	RegisterRoutes(group gin.IRoutes)

	// RegisterMiddleware returns middleware handlers that Kite inserts into
	// the global HTTP pipeline. Return nil if no middleware is needed.
	RegisterMiddleware() []gin.HandlerFunc

	// RegisterAITools returns AI tool definitions that are injected into
	// the Kite AI agent, making them invocable by users via natural language.
	RegisterAITools() []AIToolDefinition

	// RegisterResourceHandlers returns a map of resource-name → handler
	// for custom Kubernetes resource types managed by this plugin.
	// The handlers follow the same interface as Kite's built-in resource handlers.
	RegisterResourceHandlers() map[string]ResourceHandler

	// OnClusterEvent is called when a cluster-level event occurs
	// (e.g. cluster added, removed, resource changed).
	OnClusterEvent(event ClusterEvent)

	// Shutdown is called during graceful shutdown. Plugins should release
	// resources and stop background goroutines within the provided context deadline.
	Shutdown(ctx context.Context) error
}

// ResourceHandler mirrors the interface used by Kite's built-in resource
// handlers (pkg/handlers/resources), allowing plugins to register
// fully-featured CRUD endpoints for custom resource types.
type ResourceHandler interface {
	List(c *gin.Context)
	Get(c *gin.Context)
	Create(c *gin.Context)
	Update(c *gin.Context)
	Delete(c *gin.Context)
	Patch(c *gin.Context)
	IsClusterScoped() bool
}

// ClusterEventType enumerates the types of cluster-level events.
type ClusterEventType string

const (
	ClusterEventAdded   ClusterEventType = "added"
	ClusterEventRemoved ClusterEventType = "removed"
	ClusterEventUpdated ClusterEventType = "updated"
)

// ClusterEvent represents a cluster-level event sent to plugins.
type ClusterEvent struct {
	Type        ClusterEventType `json:"type"`
	ClusterName string           `json:"clusterName"`
}
