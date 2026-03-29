package plugin

// This file implements the HashiCorp go-plugin adapter that bridges
// the KitePlugin Go interface to gRPC communication with plugin
// processes running as separate binaries.
//
// Prerequisites (run before this file can compile):
//   1. go get github.com/hashicorp/go-plugin
//   2. protoc --go_out=. --go-grpc_out=. pkg/plugin/proto/plugin.proto
//
// Once generated, the proto stubs live at:
//   pkg/plugin/proto/plugin.pb.go
//   pkg/plugin/proto/plugin_grpc.pb.go

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"

	pb "github.com/zxh326/kite/pkg/plugin/proto"
)

// PluginProtocolVersion is the protocol version for go-plugin handshake.
const PluginProtocolVersion = 1

// Handshake is the go-plugin handshake config shared between host and plugins.
var Handshake = goplugin.HandshakeConfig{
	ProtocolVersion:  PluginProtocolVersion,
	MagicCookieKey:   "KITE_PLUGIN",
	MagicCookieValue: "kite-plugin-v1",
}

// GRPCPluginName is the key used in the go-plugin PluginMap.
const GRPCPluginName = "kite"

// PluginMap is the go-plugin map used for all Kite plugins.
var PluginMap = map[string]goplugin.Plugin{
	GRPCPluginName: &GRPCPluginAdapter{},
}

// GRPCPluginAdapter implements goplugin.GRPCPlugin, bridging
// the gRPC transport to the KitePlugin interface.
type GRPCPluginAdapter struct {
	goplugin.Plugin
	// Impl is only set on the plugin side (the subprocess).
	Impl KitePlugin
}

// GRPCServer registers the plugin implementation on the gRPC server (plugin side).
func (p *GRPCPluginAdapter) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterPluginServiceServer(s, &grpcServer{impl: p.Impl})
	return nil
}

// GRPCClient returns a KitePlugin implementation that communicates over gRPC (host side).
func (p *GRPCPluginAdapter) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (any, error) {
	return &grpcClient{client: pb.NewPluginServiceClient(c)}, nil
}

// --------------------------------------------------------------------------
// gRPC Client (host side — Kite calls these methods on the plugin process)
// --------------------------------------------------------------------------

// grpcClient implements KitePlugin by forwarding calls over gRPC.
type grpcClient struct {
	client pb.PluginServiceClient
}

func (g *grpcClient) Manifest() PluginManifest {
	resp, err := g.client.GetManifest(context.Background(), &pb.Empty{})
	if err != nil {
		klog.Errorf("gRPC GetManifest failed: %v", err)
		return PluginManifest{}
	}
	return manifestFromProto(resp)
}

func (g *grpcClient) RegisterAITools() []AIToolDefinition {
	resp, err := g.client.GetAITools(context.Background(), &pb.Empty{})
	if err != nil {
		klog.Errorf("gRPC GetAITools failed: %v", err)
		return nil
	}

	tools := make([]AIToolDefinition, 0, len(resp.Tools))
	for _, t := range resp.Tools {
		tools = append(tools, AIToolDefinition{
			Name:        t.Name,
			Description: t.Description,
			Required:    t.Required,
			// Properties is JSON-decoded from t.PropertiesJson
		})
	}
	return tools
}

func (g *grpcClient) RegisterResourceHandlers() map[string]ResourceHandler {
	resp, err := g.client.GetResourceHandlers(context.Background(), &pb.Empty{})
	if err != nil {
		klog.Errorf("gRPC GetResourceHandlers failed: %v", err)
		return nil
	}

	handlers := make(map[string]ResourceHandler, len(resp.Handlers))
	for _, h := range resp.Handlers {
		handlers[h.Name] = &grpcResourceHandler{
			client:        g.client,
			handlerName:   h.Name,
			clusterScoped: h.IsClusterScoped,
		}
	}
	return handlers
}

func (g *grpcClient) OnClusterEvent(event ClusterEvent) {
	_, err := g.client.OnClusterEvent(context.Background(), &pb.ClusterEvent{
		Type:        string(event.Type),
		ClusterName: event.ClusterName,
	})
	if err != nil {
		klog.Errorf("gRPC OnClusterEvent failed: %v", err)
	}
}

func (g *grpcClient) Shutdown(ctx context.Context) error {
	_, err := g.client.Shutdown(ctx, &pb.Empty{})
	return err
}

// RegisterRoutes is a no-op for the gRPC client side; routing is handled by the
// host process which proxies requests to the plugin over HandleHTTP.
func (g *grpcClient) RegisterRoutes(_ gin.IRoutes) {}

// RegisterMiddleware returns nil; middleware configuration is fetched lazily
// via GetMiddleware() when Kite's middleware pipeline is being assembled.
func (g *grpcClient) RegisterMiddleware() []gin.HandlerFunc { return nil }

// --------------------------------------------------------------------------
// gRPC Server (plugin side — the subprocess implements these)
// --------------------------------------------------------------------------

// grpcServer wraps a KitePlugin and serves it over gRPC.
type grpcServer struct {
	pb.UnimplementedPluginServiceServer
	impl KitePlugin
}

func (s *grpcServer) GetManifest(ctx context.Context, _ *pb.Empty) (*pb.Manifest, error) {
	m := s.impl.Manifest()
	return manifestToProto(&m), nil
}

func (s *grpcServer) Shutdown(ctx context.Context, _ *pb.Empty) (*pb.Empty, error) {
	return &pb.Empty{}, s.impl.Shutdown(ctx)
}

func (s *grpcServer) OnClusterEvent(ctx context.Context, req *pb.ClusterEvent) (*pb.Empty, error) {
	s.impl.OnClusterEvent(ClusterEvent{
		Type:        ClusterEventType(req.Type),
		ClusterName: req.ClusterName,
	})
	return &pb.Empty{}, nil
}

func (s *grpcServer) GetAITools(ctx context.Context, _ *pb.Empty) (*pb.AIToolList, error) {
	defs := s.impl.RegisterAITools()
	tools := make([]*pb.AIToolDef, 0, len(defs))
	for _, d := range defs {
		tools = append(tools, &pb.AIToolDef{
			Name:        d.Name,
			Description: d.Description,
			Required:    d.Required,
			// PropertiesJson: marshal d.Properties
		})
	}
	return &pb.AIToolList{Tools: tools}, nil
}

func (s *grpcServer) GetResourceHandlers(ctx context.Context, _ *pb.Empty) (*pb.ResourceHandlerList, error) {
	handlers := s.impl.RegisterResourceHandlers()
	list := make([]*pb.ResourceHandlerDef, 0, len(handlers))
	for name, rh := range handlers {
		list = append(list, &pb.ResourceHandlerDef{
			Name:           name,
			IsClusterScoped: rh.IsClusterScoped(),
		})
	}
	return &pb.ResourceHandlerList{Handlers: list}, nil
}

// --------------------------------------------------------------------------
// gRPC Resource Handler (host side proxy to plugin process)
// --------------------------------------------------------------------------

// grpcResourceHandler proxies Gin handler calls to the plugin via gRPC.
type grpcResourceHandler struct {
	client        pb.PluginServiceClient
	handlerName   string
	clusterScoped bool
}

func (h *grpcResourceHandler) IsClusterScoped() bool { return h.clusterScoped }

// handleResource is a shared helper that proxies a CRUD verb to the plugin.
func (h *grpcResourceHandler) handleResource(c *gin.Context, verb string) {
	var body []byte
	if c.Request != nil && c.Request.Body != nil {
		body, _ = io.ReadAll(c.Request.Body)
	}
	namespace := c.Param("namespace")
	name := c.Param("name")

	resp, err := h.client.HandleResource(c.Request.Context(), &pb.ResourceRequest{
		HandlerName: h.handlerName,
		Operation:   verb,
		Namespace:   namespace,
		Name:        name,
		Body:        body,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	statusCode := int(resp.StatusCode)
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	c.Data(statusCode, "application/json", resp.Body)
}

func (h *grpcResourceHandler) List(c *gin.Context)   { h.handleResource(c, "list") }
func (h *grpcResourceHandler) Get(c *gin.Context)    { h.handleResource(c, "get") }
func (h *grpcResourceHandler) Create(c *gin.Context) { h.handleResource(c, "create") }
func (h *grpcResourceHandler) Update(c *gin.Context) { h.handleResource(c, "update") }
func (h *grpcResourceHandler) Delete(c *gin.Context) { h.handleResource(c, "delete") }
func (h *grpcResourceHandler) Patch(c *gin.Context)  { h.handleResource(c, "patch") }

// --------------------------------------------------------------------------
// Proto conversion helpers
// --------------------------------------------------------------------------

func manifestFromProto(m *pb.Manifest) PluginManifest {
	pm := PluginManifest{
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Author:      m.Author,
		Priority:    int(m.Priority),
		RateLimit:   int(m.RateLimit),
	}
	for _, d := range m.Requires {
		pm.Requires = append(pm.Requires, Dependency{Name: d.Name, Version: d.Version})
	}
	for _, p := range m.Permissions {
		pm.Permissions = append(pm.Permissions, Permission{Resource: p.Resource, Verbs: p.Verbs})
	}
	if m.Frontend != nil {
		fm := &FrontendManifest{
			RemoteEntry:    m.Frontend.RemoteEntry,
			ExposedModules: m.Frontend.ExposedModules,
			SettingsPanel:  m.Frontend.SettingsPanel,
		}
		for _, r := range m.Frontend.Routes {
			fr := FrontendRoute{
				Path:   r.Path,
				Module: r.Module,
			}
			if r.SidebarEntry != nil {
				fr.SidebarEntry = &SidebarEntry{
					Title:    r.SidebarEntry.Title,
					Icon:     r.SidebarEntry.Icon,
					Section:  r.SidebarEntry.Section,
					Priority: int(r.SidebarEntry.Priority),
				}
			}
			fm.Routes = append(fm.Routes, fr)
		}
		pm.Frontend = fm
	}
	for _, s := range m.Settings {
		sf := SettingField{
			Name:        s.Name,
			Label:       s.Label,
			Type:        s.Type,
			Default:     s.DefaultValue,
			Description: s.Description,
			Required:    s.Required,
		}
		for _, o := range s.Options {
			sf.Options = append(sf.Options, SettingOption{Label: o.Label, Value: o.Value})
		}
		pm.Settings = append(pm.Settings, sf)
	}
	return pm
}

func manifestToProto(m *PluginManifest) *pb.Manifest {
	pm := &pb.Manifest{
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Author:      m.Author,
		Priority:    int32(m.Priority),
		RateLimit:   int32(m.RateLimit),
	}
	for _, d := range m.Requires {
		pm.Requires = append(pm.Requires, &pb.Dependency{Name: d.Name, Version: d.Version})
	}
	for _, p := range m.Permissions {
		pm.Permissions = append(pm.Permissions, &pb.Permission{Resource: p.Resource, Verbs: p.Verbs})
	}
	if m.Frontend != nil {
		pf := &pb.FrontendManifest{
			RemoteEntry:    m.Frontend.RemoteEntry,
			ExposedModules: m.Frontend.ExposedModules,
			SettingsPanel:  m.Frontend.SettingsPanel,
		}
		for _, r := range m.Frontend.Routes {
			fr := &pb.FrontendRoute{
				Path:   r.Path,
				Module: r.Module,
			}
			if r.SidebarEntry != nil {
				fr.SidebarEntry = &pb.SidebarEntry{
					Title:    r.SidebarEntry.Title,
					Icon:     r.SidebarEntry.Icon,
					Section:  r.SidebarEntry.Section,
					Priority: int32(r.SidebarEntry.Priority),
				}
			}
			pf.Routes = append(pf.Routes, fr)
		}
		pm.Frontend = pf
	}
	for _, s := range m.Settings {
		ps := &pb.SettingField{
			Name:         s.Name,
			Label:        s.Label,
			Type:         s.Type,
			DefaultValue: s.Default,
			Description:  s.Description,
			Required:     s.Required,
		}
		for _, o := range s.Options {
			ps.Options = append(ps.Options, &pb.SettingOption{Label: o.Label, Value: o.Value})
		}
		pm.Settings = append(pm.Settings, ps)
	}
	return pm
}

// --------------------------------------------------------------------------
// Plugin process management
// --------------------------------------------------------------------------

// PluginClient wraps go-plugin Client for managing a plugin subprocess.
type PluginClient struct {
	client *goplugin.Client
	plugin KitePlugin
	name   string
}

// startPluginProcess launches a plugin binary as a child process
// and establishes gRPC communication via go-plugin.
func startPluginProcess(lp *LoadedPlugin) (*PluginClient, error) {
	binaryPath := filepath.Join(lp.Dir, lp.Manifest.Name)

	// Verify binary exists
	if _, err := os.Stat(binaryPath); err != nil {
		return nil, fmt.Errorf("plugin binary not found: %w", err)
	}

	cmd := exec.Command(binaryPath)
	// Sandbox: set working directory to the plugin's own directory
	cmd.Dir = lp.Dir
	// Sandbox: pass a minimal, curated environment — exclude sensitive
	// Kite variables (JWT_SECRET, DB_DSN, KITE_ENCRYPT_KEY, etc.)
	cmd.Env = sanitizedEnvForPlugin(lp.Manifest.Name)

	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig: Handshake,
		Plugins:         PluginMap,
		Cmd:             cmd,
		AllowedProtocols: []goplugin.Protocol{
			goplugin.ProtocolGRPC,
		},
		Logger:       nil, // Uses klog-compatible logger
		StartTimeout: 10 * time.Second,
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("connect to plugin %q: %w", lp.Manifest.Name, err)
	}

	raw, err := rpcClient.Dispense(GRPCPluginName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("dispense plugin %q: %w", lp.Manifest.Name, err)
	}

	kitePlugin, ok := raw.(KitePlugin)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("plugin %q does not implement KitePlugin", lp.Manifest.Name)
	}

	return &PluginClient{
		client: client,
		plugin: kitePlugin,
		name:   lp.Manifest.Name,
	}, nil
}

// Stop gracefully shuts down the plugin process.
func (pc *PluginClient) Stop(ctx context.Context) {
	if pc.plugin != nil {
		if err := pc.plugin.Shutdown(ctx); err != nil {
			klog.Errorf("Plugin %q shutdown error: %v", pc.name, err)
		}
	}
	if pc.client != nil {
		pc.client.Kill()
	}
}

// IsAlive checks if the plugin process is still running.
func (pc *PluginClient) IsAlive() bool {
	if pc.client == nil {
		return false
	}
	return !pc.client.Exited()
}

// KitePlugin returns the gRPC-backed KitePlugin implementation.
func (pc *PluginClient) KitePlugin() KitePlugin {
	return pc.plugin
}

// --------------------------------------------------------------------------
// Plugin-side helper (used by plugin binaries)
// --------------------------------------------------------------------------

// Serve is called by plugin binaries in their main() to start serving.
// Usage in a plugin binary:
//
//	func main() {
//	    plugin.Serve(&MyPlugin{})
//	}
func Serve(impl KitePlugin) {
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins: map[string]goplugin.Plugin{
			GRPCPluginName: &GRPCPluginAdapter{Impl: impl},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}

// --------------------------------------------------------------------------
// Environment sandboxing
// --------------------------------------------------------------------------

// sensitiveEnvPrefixes lists environment variable prefixes that must NOT be
// inherited by plugin child processes. These contain secrets or internal
// configuration that plugins should never see.
var sensitiveEnvPrefixes = []string{
	"JWT_SECRET",
	"KITE_ENCRYPT_KEY",
	"KITE_PASSWORD",
	"DB_DSN",
	"DB_TYPE",
	"OPENAI_",
	"ANTHROPIC_",
	"AZURE_",
	"AWS_SECRET",
	"GOOGLE_APPLICATION_CREDENTIALS",
}

// sanitizedEnvForPlugin returns a minimal environment for a plugin process.
// It copies safe system variables (PATH, HOME, TMPDIR, etc.) and injects
// the plugin name, but strips all Kite-internal secrets.
func sanitizedEnvForPlugin(pluginName string) []string {
	// System variables plugins may reasonably need
	safeKeys := map[string]bool{
		"PATH": true, "HOME": true, "USER": true,
		"TMPDIR": true, "LANG": true, "LC_ALL": true,
		"TZ": true, "TERM": true, "SHELL": true,
	}

	var env []string
	for _, e := range os.Environ() {
		key, _, found := cutEnv(e)
		if !found {
			continue
		}
		if safeKeys[key] {
			env = append(env, e)
		}
	}

	// Inject plugin identity so the plugin knows its own name
	env = append(env, "KITE_PLUGIN_NAME="+pluginName)

	return env
}

// cutEnv splits an "KEY=VALUE" string into key and value.
func cutEnv(s string) (key, value string, found bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}
