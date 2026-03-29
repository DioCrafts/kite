package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"

	pb "github.com/zxh326/kite/pkg/plugin/proto"
)

// HandlePluginHTTP proxies an HTTP request to the named plugin's gRPC HandleHTTP RPC.
func (pm *PluginManager) HandlePluginHTTP(c *gin.Context, pluginName string) {
	lp := pm.GetPlugin(pluginName)
	if lp == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("plugin %q not found", pluginName)})
		return
	}

	// Enforce capability-based permissions
	pluginPath := c.Param("path")
	resource := extractResourceFromPath(pluginPath)
	if resource != "" {
		if err := pm.Permissions.CheckHTTPMethod(pluginName, resource, c.Request.Method); err != nil {
			klog.Warningf("[PLUGIN:%s] permission denied: %v", pluginName, err)
			pm.auditPluginAction(c, pluginName, "", resource, httpMethodToVerb(c.Request.Method), false, err.Error())
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
	}

	// Enforce rate limit
	if !pm.RateLimiter.Allow(pluginName) {
		klog.Warningf("[PLUGIN:%s] rate limit exceeded", pluginName)
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded for plugin " + pluginName})
		return
	}

	lp.mu.RLock()
	client := lp.client
	state := lp.State
	lp.mu.RUnlock()

	if state != PluginStateLoaded || client == nil || !client.IsAlive() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("plugin %q is not available", pluginName)})
		return
	}

	// Read request body
	var body []byte
	if c.Request.Body != nil {
		var err error
		body, err = io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}
	}

	// Extract headers
	headers := make(map[string]string, len(c.Request.Header))
	for k, v := range c.Request.Header {
		headers[k] = strings.Join(v, ", ")
	}

	// Extract query params
	queryParams := make(map[string]string, len(c.Request.URL.Query()))
	for k, v := range c.Request.URL.Query() {
		queryParams[k] = strings.Join(v, ",")
	}

	// Extract user and cluster context from Gin
	userJSON := "{}"
	if rawUser, ok := c.Get("user"); ok {
		if b, err := json.Marshal(rawUser); err == nil {
			userJSON = string(b)
		}
	}
	clusterName := ""
	if cs, ok := c.Get("cluster"); ok {
		if csTyped, ok := cs.(interface{ GetName() string }); ok {
			clusterName = csTyped.GetName()
		}
	}

	// (pluginPath was already captured above for the permission check)

	grpcClient, ok := client.plugin.(*grpcClient)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "plugin does not support HTTP proxy"})
		return
	}

	resp, err := grpcClient.client.HandleHTTP(c.Request.Context(), &pb.HTTPRequest{
		Method:      c.Request.Method,
		Path:        pluginPath,
		Headers:     headers,
		QueryParams: queryParams,
		Body:        body,
		UserJson:    userJSON,
		ClusterName: clusterName,
	})
	if err != nil {
		klog.Errorf("Plugin %q HandleHTTP error: %v", pluginName, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("plugin error: %v", err)})
		return
	}

	// Write response headers
	for k, v := range resp.Headers {
		c.Header(k, v)
	}

	c.Data(int(resp.StatusCode), resp.Headers["Content-Type"], resp.Body)
}

// ExecutePluginTool finds and executes a plugin AI tool by its prefixed name.
// Tool names follow the pattern "plugin_<pluginName>_<toolName>".
func (pm *PluginManager) ExecutePluginTool(ctx context.Context, c *gin.Context, toolName string, args map[string]any) (string, bool) {
	// Parse "plugin_<pluginName>_<toolName>"
	parts := strings.SplitN(toolName, "_", 3)
	if len(parts) != 3 || parts[0] != "plugin" {
		return fmt.Sprintf("Invalid plugin tool name: %s", toolName), true
	}

	pluginName := parts[1]
	actualToolName := parts[2]

	lp := pm.GetPlugin(pluginName)
	if lp == nil {
		return fmt.Sprintf("Plugin %q not found", pluginName), true
	}

	lp.mu.RLock()
	client := lp.client
	state := lp.State
	lp.mu.RUnlock()

	if state != PluginStateLoaded || client == nil || !client.IsAlive() {
		return fmt.Sprintf("Plugin %q is not available", pluginName), true
	}

	grpcCl, ok := client.plugin.(*grpcClient)
	if !ok {
		return fmt.Sprintf("Plugin %q does not support gRPC", pluginName), true
	}

	// Serialize args
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return fmt.Sprintf("Failed to serialize args: %v", err), true
	}

	// Extract user and cluster from Gin context
	userJSON := "{}"
	if rawUser, ok := c.Get("user"); ok {
		if b, err := json.Marshal(rawUser); err == nil {
			userJSON = string(b)
		}
	}
	clusterName := ""
	if cs, ok := c.Get("clusterName"); ok {
		if name, ok := cs.(string); ok {
			clusterName = name
		}
	}

	// First check authorization
	authResp, err := grpcCl.client.AuthorizeAITool(ctx, &pb.AIToolAuthRequest{
		ToolName:    actualToolName,
		ArgsJson:    string(argsJSON),
		ClusterName: clusterName,
		UserJson:    userJSON,
	})
	if err != nil {
		pm.auditPluginAction(c, pluginName, actualToolName, "", "execute", false, err.Error())
		return fmt.Sprintf("Plugin tool authorization failed: %v", err), true
	}
	if !authResp.Allowed {
		reason := authResp.Reason
		if reason == "" {
			reason = "forbidden"
		}
		klog.Warningf("[PLUGIN:%s] tool %q access denied: %s", pluginName, actualToolName, reason)
		pm.auditPluginAction(c, pluginName, actualToolName, "", "execute", false, reason)
		return fmt.Sprintf("Plugin tool access denied: %s", reason), true
	}

	// Execute the tool
	toolResp, err := grpcCl.client.ExecuteAITool(ctx, &pb.AIToolRequest{
		ToolName:    actualToolName,
		ArgsJson:    string(argsJSON),
		ClusterName: clusterName,
		UserJson:    userJSON,
	})
	if err != nil {
		pm.auditPluginAction(c, pluginName, actualToolName, "", "execute", false, err.Error())
		return fmt.Sprintf("Plugin tool execution failed: %v", err), true
	}

	pm.auditPluginAction(c, pluginName, actualToolName, "", "execute", !toolResp.IsError, "")
	return toolResp.Result, toolResp.IsError
}

// PluginMiddleware returns a Gin middleware that checks loaded plugin middleware
// configurations and, for enabled plugin middleware, proxies requests through
// the plugin's ApplyMiddleware gRPC RPC.
func (pm *PluginManager) PluginMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		plugins := pm.LoadedPlugins()

		for _, lp := range plugins {
			lp.mu.RLock()
			client := lp.client
			lp.mu.RUnlock()

			if client == nil || !client.IsAlive() {
				continue
			}

			grpcCl, ok := client.plugin.(*grpcClient)
			if !ok {
				continue
			}

			// Check if middleware is enabled
			mwConfig, err := grpcCl.client.GetMiddleware(c.Request.Context(), &pb.Empty{})
			if err != nil || !mwConfig.Enabled {
				continue
			}

			// Extract request info for the middleware
			headers := make(map[string]string, len(c.Request.Header))
			for k, v := range c.Request.Header {
				headers[k] = strings.Join(v, ", ")
			}

			userJSON := "{}"
			if rawUser, ok := c.Get("user"); ok {
				if b, err := json.Marshal(rawUser); err == nil {
					userJSON = string(b)
				}
			}
			clusterName := ""
			if cs, ok := c.Get("clusterName"); ok {
				if name, ok := cs.(string); ok {
					clusterName = name
				}
			}

			resp, err := grpcCl.client.ApplyMiddleware(c.Request.Context(), &pb.MiddlewareRequest{
				Method:      c.Request.Method,
				Path:        c.Request.URL.Path,
				Headers:     headers,
				UserJson:    userJSON,
				ClusterName: clusterName,
			})
			if err != nil {
				klog.Errorf("Plugin %q middleware error: %v", lp.Manifest.Name, err)
				continue
			}

			switch resp.Action {
			case pb.MiddlewareResponse_ABORT:
				c.Data(int(resp.AbortStatusCode), "application/json", resp.AbortBody)
				c.Abort()
				return
			case pb.MiddlewareResponse_CONTINUE:
				// Apply modified headers
				for k, v := range resp.ModifiedHeaders {
					c.Request.Header.Set(k, v)
				}
			}
		}

		c.Next()
	}
}

// ReloadPlugin stops a running plugin and reloads it from disk.
func (pm *PluginManager) ReloadPlugin(name string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	lp, ok := pm.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}

	// Stop the current process
	if lp.client != nil {
		lp.client.Stop(context.Background())
	}

	// Re-discover manifest
	newLP, err := discoverPlugin(lp.Dir)
	if err != nil {
		lp.State = PluginStateFailed
		lp.Error = err.Error()
		return fmt.Errorf("re-discover plugin: %w", err)
	}

	// Update manifest
	lp.Manifest = newLP.Manifest

	// Reload
	if err := pm.loadPlugin(lp); err != nil {
		lp.State = PluginStateFailed
		lp.Error = err.Error()
		return fmt.Errorf("reload plugin: %w", err)
	}

	lp.State = PluginStateLoaded
	lp.Error = ""
	pm.Permissions.RegisterPlugin(name, lp.Manifest.Permissions)
	pm.RateLimiter.Register(name, lp.Manifest.RateLimit)
	klog.Infof("Plugin reloaded: %s v%s", lp.Manifest.Name, lp.Manifest.Version)
	return nil
}

// SetPluginEnabled enables or disables a plugin. Disabled plugins are stopped
// and won't be loaded on next restart.
func (pm *PluginManager) SetPluginEnabled(name string, enabled bool) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	lp, ok := pm.plugins[name]
	if !ok {
		return fmt.Errorf("plugin %q not found", name)
	}

	if !enabled {
		// Stop the process
		if lp.client != nil {
			lp.client.Stop(context.Background())
			lp.client = nil
		}
		lp.State = PluginStateDisabled
		pm.Permissions.UnregisterPlugin(name)
		pm.RateLimiter.Unregister(name)
		klog.Infof("Plugin disabled: %s", name)
	} else {
		// Re-load
		if err := pm.loadPlugin(lp); err != nil {
			lp.State = PluginStateFailed
			lp.Error = err.Error()
			return err
		}
		lp.State = PluginStateLoaded
		pm.Permissions.RegisterPlugin(name, lp.Manifest.Permissions)
		pm.RateLimiter.Register(name, lp.Manifest.RateLimit)
		klog.Infof("Plugin enabled: %s", name)
	}

	return nil
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// extractResourceFromPath attempts to extract a Kubernetes resource type
// from a plugin-scoped path. Plugin paths are typically structured as
// /<resource> or /<resource>/<name>. Returns "" if no resource can be inferred.
func extractResourceFromPath(path string) string {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return ""
	}
	parts := strings.SplitN(path, "/", 2)
	return parts[0]
}

// auditPluginAction logs a plugin operation to the audit log (klog).
// In a full deployment this writes to the ResourceHistory table via model.DB.
func (pm *PluginManager) auditPluginAction(c *gin.Context, pluginName, toolName, resource, action string, success bool, errMsg string) {
	// Extract user identity
	userName := "unknown"
	var userID uint
	if rawUser, ok := c.Get("user"); ok {
		type userLike interface {
			GetID() uint
			GetKey() string
		}
		if u, ok := rawUser.(userLike); ok {
			userName = u.GetKey()
			userID = u.GetID()
		}
	}

	clusterName := ""
	if cs, ok := c.Get("clusterName"); ok {
		if name, ok := cs.(string); ok {
			clusterName = name
		}
	}

	// Structured log for audit trail
	status := "success"
	if !success {
		status = "denied"
	}

	fields := fmt.Sprintf("[PLUGIN:%s] user=%s action=%s", pluginName, userName, action)
	if toolName != "" {
		fields += fmt.Sprintf(" tool=%s", toolName)
	}
	if resource != "" {
		fields += fmt.Sprintf(" resource=%s", resource)
	}
	if clusterName != "" {
		fields += fmt.Sprintf(" cluster=%s", clusterName)
	}
	fields += fmt.Sprintf(" status=%s", status)
	if errMsg != "" {
		fields += fmt.Sprintf(" error=%q", errMsg)
	}

	klog.Infof("%s", fields)

	// Persist to database if available
	pm.persistAuditRecord(pluginName, toolName, resource, action, clusterName, userID, success, errMsg)
}
