package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/zxh326/kite/pkg/ai"
	"github.com/zxh326/kite/pkg/auth"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/handlers"
	"github.com/zxh326/kite/pkg/handlers/resources"
	"github.com/zxh326/kite/pkg/middleware"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/plugin"
	"github.com/zxh326/kite/pkg/rbac"
	"github.com/zxh326/kite/pkg/version"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

func setupAPIRouter(r *gin.RouterGroup, cm *cluster.ClusterManager, pm *plugin.PluginManager) {
	authHandler := auth.NewAuthHandler()

	registerBaseRoutes(r)
	registerAuthRoutes(r, authHandler)
	registerUserRoutes(r, authHandler)
	registerAdminRoutes(r, authHandler, cm, pm)
	registerProtectedRoutes(r, authHandler, cm, pm)
	registerPluginRoutes(r, authHandler, pm)
}

func registerBaseRoutes(r *gin.RouterGroup) {
	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(prometheus.Gatherers{
		prometheus.DefaultGatherer,
		ctrlmetrics.Registry,
	}, promhttp.HandlerOpts{})))
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/api/v1/init_check", handlers.InitCheck)
	r.GET("/api/v1/version", version.GetVersion)
}

func registerAuthRoutes(r *gin.RouterGroup, authHandler *auth.AuthHandler) {
	authGroup := r.Group("/api/auth")
	authGroup.GET("/providers", authHandler.GetProviders)
	authGroup.POST("/login/password", authHandler.PasswordLogin)
	authGroup.POST("/login/ldap", authHandler.LDAPLogin)
	authGroup.GET("/login", authHandler.Login)
	authGroup.GET("/callback", authHandler.Callback)
	authGroup.POST("/logout", authHandler.Logout)
	authGroup.POST("/refresh", authHandler.RefreshToken)
	authGroup.GET("/user", authHandler.RequireAuth(), authHandler.GetUser)
}

func registerUserRoutes(r *gin.RouterGroup, authHandler *auth.AuthHandler) {
	userGroup := r.Group("/api/users")
	userGroup.POST("/sidebar_preference", authHandler.RequireAuth(), handlers.UpdateSidebarPreference)
}

func registerAdminRoutes(r *gin.RouterGroup, authHandler *auth.AuthHandler, cm *cluster.ClusterManager, pm *plugin.PluginManager) {
	adminAPI := r.Group("/api/v1/admin")
	adminAPI.POST("/users/create_super_user", handlers.CreateSuperUser)
	adminAPI.POST("/clusters/import", cm.ImportClustersFromKubeconfig)
	adminAPI.Use(authHandler.RequireAuth(), authHandler.RequireAdmin())

	adminAPI.GET("/audit-logs", handlers.ListAuditLogs)

	oauthProviderAPI := adminAPI.Group("/oauth-providers")
	oauthProviderAPI.GET("/", authHandler.ListOAuthProviders)
	oauthProviderAPI.POST("/", authHandler.CreateOAuthProvider)
	oauthProviderAPI.GET("/:id", authHandler.GetOAuthProvider)
	oauthProviderAPI.PUT("/:id", authHandler.UpdateOAuthProvider)
	oauthProviderAPI.DELETE("/:id", authHandler.DeleteOAuthProvider)

	ldapSettingAPI := adminAPI.Group("/ldap-setting")
	ldapSettingAPI.GET("/", authHandler.GetLDAPSetting)
	ldapSettingAPI.PUT("/", authHandler.UpdateLDAPSetting)

	clusterAPI := adminAPI.Group("/clusters")
	clusterAPI.GET("/", cm.GetClusterList)
	clusterAPI.POST("/", cm.CreateCluster)
	clusterAPI.PUT("/:id", cm.UpdateCluster)
	clusterAPI.DELETE("/:id", cm.DeleteCluster)

	rbacAPI := adminAPI.Group("/roles")
	rbacAPI.GET("/", rbac.ListRoles)
	rbacAPI.POST("/", rbac.CreateRole)
	rbacAPI.GET("/:id", rbac.GetRole)
	rbacAPI.PUT("/:id", rbac.UpdateRole)
	rbacAPI.DELETE("/:id", rbac.DeleteRole)
	rbacAPI.POST("/:id/assign", rbac.AssignRole)
	rbacAPI.DELETE("/:id/assign", rbac.UnassignRole)

	userAPI := adminAPI.Group("/users")
	userAPI.GET("/", handlers.ListUsers)
	userAPI.POST("/", handlers.CreatePasswordUser)
	userAPI.PUT("/:id", handlers.UpdateUser)
	userAPI.DELETE("/:id", handlers.DeleteUser)
	userAPI.POST("/:id/reset_password", handlers.ResetPassword)
	userAPI.POST("/:id/enable", handlers.SetUserEnabled)

	apiKeyAPI := adminAPI.Group("/apikeys")
	apiKeyAPI.GET("/", handlers.ListAPIKeys)
	apiKeyAPI.POST("/", handlers.CreateAPIKey)
	apiKeyAPI.DELETE("/:id", handlers.DeleteAPIKey)

	generalSettingAPI := adminAPI.Group("/general-setting")
	generalSettingAPI.GET("/", ai.HandleGetGeneralSetting)
	generalSettingAPI.PUT("/", ai.HandleUpdateGeneralSetting)

	templateAPI := adminAPI.Group("/templates")
	templateAPI.POST("/", handlers.CreateTemplate)
	templateAPI.PUT("/:id", handlers.UpdateTemplate)
	templateAPI.DELETE("/:id", handlers.DeleteTemplate)

	registerPluginAdminRoutes(adminAPI, pm)
}

func registerProtectedRoutes(r *gin.RouterGroup, authHandler *auth.AuthHandler, cm *cluster.ClusterManager, pm *plugin.PluginManager) {
	api := r.Group("/api/v1")
	api.GET("/clusters", authHandler.RequireAuth(), cm.GetClusters)
	api.Use(authHandler.RequireAuth(), middleware.ClusterMiddleware(cm))

	api.GET("/overview", handlers.GetOverview)

	promHandler := handlers.NewPromHandler()
	api.GET("/prometheus/resource-usage-history", promHandler.GetResourceUsageHistory)
	api.GET("/prometheus/pods/:namespace/:podName/metrics", promHandler.GetPodMetrics)

	logsHandler := handlers.NewLogsHandler()
	api.GET("/logs/:namespace/:podName/ws", logsHandler.HandleLogsWebSocket)

	terminalHandler := handlers.NewTerminalHandler()
	api.GET("/terminal/:namespace/:podName/ws", terminalHandler.HandleTerminalWebSocket)

	nodeTerminalHandler := handlers.NewNodeTerminalHandler()
	api.GET("/node-terminal/:nodeName/ws", nodeTerminalHandler.HandleNodeTerminalWebSocket)

	kubectlTerminalHandler := handlers.NewKubectlTerminalHandler()
	api.GET("/kubectl-terminal/ws", kubectlTerminalHandler.HandleKubectlTerminalWebSocket)

	searchHandler := handlers.NewSearchHandler()
	api.GET("/search", searchHandler.GlobalSearch)

	resourceApplyHandler := handlers.NewResourceApplyHandler()
	api.POST("/resources/apply", resourceApplyHandler.ApplyResource)

	api.GET("/image/tags", handlers.GetImageTags)
	api.GET("/templates", handlers.ListTemplates)

	proxyHandler := handlers.NewProxyHandler()
	proxyHandler.RegisterRoutes(api)

	api.GET("/ai/status", ai.HandleAIStatus)
	api.POST("/ai/chat", pluginManagerMiddleware(pm), ai.HandleChat)
	api.POST("/ai/execute/continue", pluginManagerMiddleware(pm), ai.HandleExecuteContinue)
	api.POST("/ai/input/continue", pluginManagerMiddleware(pm), ai.HandleInputContinue)

	api.Use(middleware.RBACMiddleware())
	resources.RegisterRoutes(api, pm)
}

// pluginManagerMiddleware injects the PluginManager into the Gin context
// so that downstream handlers (e.g. AI handlers) can access plugin tools.
func pluginManagerMiddleware(pm *plugin.PluginManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("pluginManager", pm)
		c.Next()
	}
}

// registerPluginRoutes exposes plugin metadata, HTTP proxy to plugin processes,
// and admin management endpoints.
func registerPluginRoutes(r *gin.RouterGroup, authHandler *auth.AuthHandler, pm *plugin.PluginManager) {
	pluginAPI := r.Group("/api/v1/plugins")
	pluginAPI.Use(authHandler.RequireAuth())

	// List all plugins and their state
	pluginAPI.GET("/", func(c *gin.Context) {
		type pluginInfo struct {
			Name        string                   `json:"name"`
			Version     string                   `json:"version"`
			Description string                   `json:"description"`
			State       plugin.PluginState        `json:"state"`
			Error       string                   `json:"error,omitempty"`
			Frontend    *plugin.FrontendManifest `json:"frontend,omitempty"`
		}

		allPlugins := pm.AllPlugins()
		info := make([]pluginInfo, 0, len(allPlugins))
		for _, lp := range allPlugins {
			info = append(info, pluginInfo{
				Name:        lp.Manifest.Name,
				Version:     lp.Manifest.Version,
				Description: lp.Manifest.Description,
				State:       lp.State,
				Error:       lp.Error,
				Frontend:    lp.Manifest.Frontend,
			})
		}
		c.JSON(http.StatusOK, info)
	})

	// Frontend manifests for the UI to load plugin modules
	pluginAPI.GET("/frontends", func(c *gin.Context) {
		c.JSON(http.StatusOK, pm.AllFrontendManifests())
	})
	// Alias used by the frontend loader and E2E tests (with and without trailing slash)
	pluginAPI.GET("/manifests", func(c *gin.Context) {
		c.JSON(http.StatusOK, pm.AllFrontendManifests())
	})
	pluginAPI.GET("/manifests/", func(c *gin.Context) {
		c.JSON(http.StatusOK, pm.AllFrontendManifests())
	})

	// Execute a plugin AI tool by its prefixed name (plugin_<name>_<tool>).
	// Returns 400 for malformed tool names, 404 if the plugin is not found.
	pluginAPI.POST("/tools/:toolName", func(c *gin.Context) {
		toolName := c.Param("toolName")
		var req struct {
			Arguments map[string]any `json:"arguments"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			req.Arguments = map[string]any{}
		}
		result, isError := pm.ExecutePluginTool(c.Request.Context(), c, toolName, req.Arguments)
		if isError {
			// Distinguish between format errors (400) and missing plugin (404)
			status := http.StatusBadRequest
			if len(toolName) > 0 && strings.HasPrefix(toolName, "plugin_") {
				status = http.StatusNotFound
			}
			c.JSON(status, gin.H{"error": result})
			return
		}
		c.JSON(http.StatusOK, gin.H{"result": result})
	})

	// HTTP proxy: forward requests to plugin processes via gRPC.
	// Note: ClusterMiddleware is NOT applied here; the plugin handler reads the
	// cluster name from the request context only when available.
	pluginAPI.Any("/:pluginName/*path", func(c *gin.Context) {
		pluginName := c.Param("pluginName")
		pm.HandlePluginHTTP(c, pluginName)
	})
}

// registerPluginAdminRoutes adds admin-only plugin management endpoints.
func registerPluginAdminRoutes(adminAPI *gin.RouterGroup, pm *plugin.PluginManager) {
	pluginAdmin := adminAPI.Group("/plugins")

	// List all plugins with full manifest details
	pluginAdmin.GET("/", func(c *gin.Context) {
		type adminPluginInfo struct {
			Name        string                   `json:"name"`
			Version     string                   `json:"version"`
			Description string                   `json:"description"`
			Author      string                   `json:"author"`
			State       plugin.PluginState        `json:"state"`
			Error       string                   `json:"error,omitempty"`
			Priority    int                      `json:"priority"`
			Permissions []plugin.Permission      `json:"permissions"`
			Settings    []plugin.SettingField     `json:"settings"`
			Frontend    *plugin.FrontendManifest `json:"frontend,omitempty"`
		}

		allPlugins := pm.AllPlugins()
		info := make([]adminPluginInfo, 0, len(allPlugins))
		for _, lp := range allPlugins {
			info = append(info, adminPluginInfo{
				Name:        lp.Manifest.Name,
				Version:     lp.Manifest.Version,
				Description: lp.Manifest.Description,
				Author:      lp.Manifest.Author,
				State:       lp.State,
				Error:       lp.Error,
				Priority:    lp.Manifest.Priority,
				Permissions: lp.Manifest.Permissions,
				Settings:    lp.Manifest.Settings,
				Frontend:    lp.Manifest.Frontend,
			})
		}
		c.JSON(http.StatusOK, info)
	})

	// Update plugin settings
	pluginAdmin.PUT("/:name/settings", func(c *gin.Context) {
		name := c.Param("name")
		lp := pm.GetPlugin(name)
		if lp == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
			return
		}

		var settings map[string]any
		if err := c.ShouldBindJSON(&settings); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Persist settings to database
		if err := model.SavePluginSettings(name, settings); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Get plugin settings
	pluginAdmin.GET("/:name/settings", func(c *gin.Context) {
		name := c.Param("name")
		lp := pm.GetPlugin(name)
		if lp == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "plugin not found"})
			return
		}

		settings, err := model.GetPluginSettings(name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, settings)
	})

	// Enable/disable plugin
	pluginAdmin.POST("/:name/enable", func(c *gin.Context) {
		name := c.Param("name")
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := pm.SetPluginEnabled(name, req.Enabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Persist enabled state
		_ = model.SetPluginEnabled(name, req.Enabled)
		c.JSON(http.StatusOK, gin.H{"status": "ok", "enabled": req.Enabled})
	})

	// Hot-reload plugin
	pluginAdmin.POST("/:name/reload", func(c *gin.Context) {
		name := c.Param("name")
		if err := pm.ReloadPlugin(name); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Install a new plugin from an uploaded .tar.gz archive.
	// The multipart field name must be "plugin".
	pluginAdmin.POST("/install", func(c *gin.Context) {
		file, header, err := c.Request.FormFile("plugin")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing 'plugin' file field: " + err.Error()})
			return
		}
		defer file.Close()

		_ = header // filename not used; manifest provides authoritative name

		lp, err := pm.InstallPlugin(file)
		if err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"name":    lp.Manifest.Name,
			"version": lp.Manifest.Version,
			"state":   lp.State,
		})
	})

	// Uninstall (remove) a plugin by name.
	pluginAdmin.DELETE("/:name", func(c *gin.Context) {
		name := c.Param("name")
		if err := pm.UninstallPlugin(name); err != nil {
			if strings.Contains(err.Error(), "not found") {
				c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	})
}

