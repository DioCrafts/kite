package sdk

import (
	"context"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"github.com/zxh326/kite/pkg/plugin"
	pb "github.com/zxh326/kite/pkg/plugin/proto"
)

// Serve starts the go-plugin gRPC server for a Kite plugin.
// This should be the last call in a plugin's main() function.
//
//	func main() {
//	    sdk.Serve(&MyPlugin{})
//	}
func Serve(impl plugin.KitePlugin) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			plugin.GRPCPluginName: &plugin.GRPCPluginAdapter{Impl: impl},
		},
		GRPCServer: func(opts []grpc.ServerOption) *grpc.Server {
			return grpc.NewServer(opts...)
		},
	})
}

// NewAITool creates an AIToolDefinition with the given name, description,
// and parameter schema. This is a convenience wrapper for plugin authors.
//
//	sdk.NewAITool("list_costs", "List resource costs",
//	    map[string]any{
//	        "namespace": map[string]any{ "type": "string", "description": "Kubernetes namespace" },
//	    },
//	    []string{"namespace"},
//	)
func NewAITool(name, description string, properties map[string]any, required []string) plugin.AIToolDefinition {
	return plugin.AIToolDefinition{
		Name:        name,
		Description: description,
		Properties:  properties,
		Required:    required,
	}
}

// NewAIToolFull creates an AITool with definition, executor, and optional authorizer.
func NewAIToolFull(def plugin.AIToolDefinition, exec plugin.AIToolExecutor, auth plugin.AIToolAuthorizer) plugin.AITool {
	return plugin.AITool{
		Definition: def,
		Execute:    exec,
		Authorize:  auth,
	}
}

// Logger returns a structured logger that writes to stderr (captured by go-plugin).
func Logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// --------------------------------------------------------------------------
// BasePlugin provides default no-op implementations for all KitePlugin methods.
// Plugin authors can embed this and only override the methods they need.
// --------------------------------------------------------------------------

// BasePlugin is an embeddable struct that satisfies the KitePlugin interface
// with no-op defaults. Plugin authors embed this to avoid implementing
// every method when only a subset is needed.
//
//	type MyPlugin struct { sdk.BasePlugin }
//
//	func (p *MyPlugin) Manifest() plugin.PluginManifest { ... }
//	func (p *MyPlugin) RegisterAITools() []plugin.AIToolDefinition { ... }
type BasePlugin struct{}

func (BasePlugin) Manifest() plugin.PluginManifest                { return plugin.PluginManifest{} }
func (BasePlugin) RegisterRoutes(_ gin.IRoutes)                    {}
func (BasePlugin) RegisterMiddleware() []gin.HandlerFunc           { return nil }
func (BasePlugin) RegisterAITools() []plugin.AIToolDefinition      { return nil }
func (BasePlugin) RegisterResourceHandlers() map[string]plugin.ResourceHandler {
	return nil
}
func (BasePlugin) OnClusterEvent(_ plugin.ClusterEvent) {}
func (BasePlugin) Shutdown(_ context.Context) error      { return nil }

// Ensure BasePlugin satisfies the interface at compile time.
var _ plugin.KitePlugin = (*BasePlugin)(nil)

// compile-time check that proto package is importable
var _ *pb.Empty
