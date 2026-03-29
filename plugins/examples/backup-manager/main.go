package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/plugin"
	"github.com/zxh326/kite/pkg/plugin/sdk"
)

// BackupManagerPlugin provides simulated Kubernetes namespace backup/restore.
type BackupManagerPlugin struct {
	sdk.BasePlugin

	mu       sync.RWMutex
	backups  []backup
	settings backupSettings
	nextID   int
}

type backup struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Namespace   string    `json:"namespace"`
	Status      string    `json:"status"` // "completed", "in-progress", "failed"
	CreatedAt   time.Time `json:"createdAt"`
	SizeBytes   int64     `json:"sizeBytes"`
	ResourceCount int     `json:"resourceCount"`
}

type backupSettings struct {
	RetentionDays    int    `json:"retentionDays"`
	MaxBackups       int    `json:"maxBackups"`
	DefaultNamespace string `json:"defaultNamespace"`
}

func defaultSettings() backupSettings {
	return backupSettings{
		RetentionDays:    30,
		MaxBackups:       50,
		DefaultNamespace: "",
	}
}

func (p *BackupManagerPlugin) Manifest() plugin.PluginManifest {
	return plugin.PluginManifest{
		Name:        "backup-manager",
		Version:     "1.0.0",
		Description: "Kubernetes backup management with simulated CRD support",
		Author:      "Kite Team",
		Permissions: []plugin.Permission{
			{Resource: "pods", Verbs: []string{"get", "list"}},
			{Resource: "deployments", Verbs: []string{"get", "list"}},
			{Resource: "namespaces", Verbs: []string{"get", "list"}},
		},
		Frontend: &plugin.FrontendManifest{
			RemoteEntry: "/plugins/backup-manager/static/remoteEntry.js",
			ExposedModules: map[string]string{
				"./BackupList":     "BackupList",
				"./BackupSettings": "BackupSettings",
			},
			Routes: []plugin.FrontendRoute{
				{
					Path:   "/backups",
					Module: "./BackupList",
					SidebarEntry: &plugin.SidebarEntry{
						Title:   "Backups",
						Icon:    "database",
						Section: "operations",
					},
				},
			},
			SettingsPanel: "./BackupSettings",
		},
		Settings: []plugin.SettingField{
			{Name: "retentionDays", Type: "number", Default: "30", Label: "Backup retention period (days)"},
			{Name: "maxBackups", Type: "number", Default: "50", Label: "Maximum number of stored backups"},
			{Name: "defaultNamespace", Type: "text", Default: "", Label: "Default namespace for backups"},
		},
	}
}

func (p *BackupManagerPlugin) RegisterRoutes(group gin.IRoutes) {
	group.GET("/backups", p.handleListBackups)
	group.GET("/backups/:id", p.handleGetBackup)
	group.POST("/backups", p.handleCreateBackup)
	group.DELETE("/backups/:id", p.handleDeleteBackup)
	group.POST("/backups/:id/restore", p.handleRestoreBackup)
	group.PUT("/settings", p.handleUpdateSettings)
}

func (p *BackupManagerPlugin) RegisterAITools() []plugin.AIToolDefinition {
	return []plugin.AIToolDefinition{
		sdk.NewAITool(
			"create_backup",
			"Create a backup of a Kubernetes namespace including all its resources",
			map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "The Kubernetes namespace to back up",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Optional name for the backup. If omitted, auto-generated.",
				},
			},
			[]string{"namespace"},
		),
		sdk.NewAITool(
			"list_backups",
			"List recent Kubernetes backups with status and metadata",
			map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "Filter backups by namespace. Omit to list all.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of backups to return. Default 10.",
				},
			},
			nil,
		),
		sdk.NewAITool(
			"restore_backup",
			"Restore a previously created Kubernetes backup by name",
			map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "The name of the backup to restore",
				},
			},
			[]string{"name"},
		),
	}
}

func (p *BackupManagerPlugin) RegisterResourceHandlers() map[string]plugin.ResourceHandler {
	return map[string]plugin.ResourceHandler{
		"backups": &backupResourceHandler{plugin: p},
	}
}

func (p *BackupManagerPlugin) Shutdown(ctx context.Context) error {
	sdk.Logger().Info("backup-manager plugin shutting down")
	return nil
}

// ---- HTTP Handlers ----

func (p *BackupManagerPlugin) handleListBackups(c *gin.Context) {
	ns := c.Query("namespace")

	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []backup
	for _, b := range p.backups {
		if ns != "" && b.Namespace != ns {
			continue
		}
		result = append(result, b)
	}
	if result == nil {
		result = []backup{}
	}
	c.JSON(http.StatusOK, gin.H{"backups": result, "total": len(result)})
}

func (p *BackupManagerPlugin) handleGetBackup(c *gin.Context) {
	id := c.Param("id")

	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, b := range p.backups {
		if fmt.Sprintf("%d", b.ID) == id || b.Name == id {
			c.JSON(http.StatusOK, b)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
}

type createBackupRequest struct {
	Namespace string `json:"namespace" binding:"required"`
	Name      string `json:"name"`
}

func (p *BackupManagerPlugin) handleCreateBackup(c *gin.Context) {
	var req createBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.settings.MaxBackups > 0 && len(p.backups) >= p.settings.MaxBackups {
		c.JSON(http.StatusConflict, gin.H{"error": "maximum number of backups reached"})
		return
	}

	p.nextID++
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("backup-%s-%s", req.Namespace, time.Now().Format("2006-01-02-150405"))
	}

	b := backup{
		ID:            p.nextID,
		Name:          name,
		Namespace:     req.Namespace,
		Status:        "completed",
		CreatedAt:     time.Now(),
		SizeBytes:     int64(1024 * 1024 * (10 + p.nextID*5)), // simulated size
		ResourceCount: 10 + p.nextID*3,                         // simulated resource count
	}
	p.backups = append(p.backups, b)

	sdk.Logger().Info("backup created", "name", b.Name, "namespace", b.Namespace)
	c.JSON(http.StatusCreated, b)
}

func (p *BackupManagerPlugin) handleDeleteBackup(c *gin.Context) {
	id := c.Param("id")

	p.mu.Lock()
	defer p.mu.Unlock()

	for i, b := range p.backups {
		if fmt.Sprintf("%d", b.ID) == id || b.Name == id {
			p.backups = append(p.backups[:i], p.backups[i+1:]...)
			sdk.Logger().Info("backup deleted", "name", b.Name)
			c.JSON(http.StatusOK, gin.H{"message": "backup deleted", "name": b.Name})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
}

func (p *BackupManagerPlugin) handleRestoreBackup(c *gin.Context) {
	id := c.Param("id")

	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, b := range p.backups {
		if fmt.Sprintf("%d", b.ID) == id || b.Name == id {
			sdk.Logger().Info("backup restored", "name", b.Name, "namespace", b.Namespace)
			c.JSON(http.StatusOK, gin.H{
				"message":   "backup restored successfully",
				"name":      b.Name,
				"namespace": b.Namespace,
				"resources": b.ResourceCount,
			})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
}

func (p *BackupManagerPlugin) handleUpdateSettings(c *gin.Context) {
	var newSettings backupSettings
	if err := c.ShouldBindJSON(&newSettings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid settings: " + err.Error()})
		return
	}

	if newSettings.RetentionDays < 0 || newSettings.MaxBackups < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "retention days and max backups must be non-negative"})
		return
	}

	p.mu.Lock()
	p.settings = newSettings
	p.mu.Unlock()

	sdk.Logger().Info("settings updated",
		"retentionDays", newSettings.RetentionDays,
		"maxBackups", newSettings.MaxBackups,
	)
	c.JSON(http.StatusOK, gin.H{"message": "settings updated", "settings": newSettings})
}

// ---- Resource Handler (simulated CRD: backups.kite.io/v1) ----

type backupResourceHandler struct {
	plugin *BackupManagerPlugin
}

func (h *backupResourceHandler) List(c *gin.Context) {
	h.plugin.handleListBackups(c)
}

func (h *backupResourceHandler) Get(c *gin.Context) {
	h.plugin.handleGetBackup(c)
}

func (h *backupResourceHandler) Create(c *gin.Context) {
	h.plugin.handleCreateBackup(c)
}

func (h *backupResourceHandler) Update(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "update not supported for backups, create a new backup instead"})
}

func (h *backupResourceHandler) Delete(c *gin.Context) {
	h.plugin.handleDeleteBackup(c)
}

func (h *backupResourceHandler) Patch(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "patch not supported for backups"})
}

func (h *backupResourceHandler) IsClusterScoped() bool {
	return false
}

// ---- AI Tool execution helpers ----

func (p *BackupManagerPlugin) executeCreateBackup(args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)
	if namespace == "" {
		return "", fmt.Errorf("namespace parameter is required")
	}

	name, _ := args["name"].(string)
	if name == "" {
		name = fmt.Sprintf("backup-%s-%s", namespace, time.Now().Format("2006-01-02-150405"))
	}

	p.mu.Lock()
	p.nextID++
	b := backup{
		ID:            p.nextID,
		Name:          name,
		Namespace:     namespace,
		Status:        "completed",
		CreatedAt:     time.Now(),
		SizeBytes:     int64(1024 * 1024 * 15),
		ResourceCount: 20,
	}
	p.backups = append(p.backups, b)
	p.mu.Unlock()

	return fmt.Sprintf("Backup %q created for namespace %q.\nResources backed up: %d\nSize: %.1f MB\nStatus: %s",
		b.Name, b.Namespace, b.ResourceCount, float64(b.SizeBytes)/(1024*1024), b.Status), nil
}

func (p *BackupManagerPlugin) executeListBackups(args map[string]any) (string, error) {
	namespace, _ := args["namespace"].(string)
	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	var filtered []backup
	for _, b := range p.backups {
		if namespace != "" && b.Namespace != namespace {
			continue
		}
		filtered = append(filtered, b)
	}

	if len(filtered) == 0 {
		if namespace != "" {
			return fmt.Sprintf("No backups found for namespace %q", namespace), nil
		}
		return "No backups found", nil
	}

	// Return most recent first, up to limit
	start := 0
	if len(filtered) > limit {
		start = len(filtered) - limit
	}
	recent := filtered[start:]

	result := fmt.Sprintf("Found %d backup(s):\n", len(recent))
	for _, b := range recent {
		result += fmt.Sprintf("  - %s (ns: %s, status: %s, created: %s, resources: %d)\n",
			b.Name, b.Namespace, b.Status, b.CreatedAt.Format(time.RFC3339), b.ResourceCount)
	}
	return result, nil
}

func (p *BackupManagerPlugin) executeRestoreBackup(args map[string]any) (string, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return "", fmt.Errorf("name parameter is required")
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, b := range p.backups {
		if b.Name == name {
			return fmt.Sprintf("Backup %q restored successfully to namespace %q.\nResources restored: %d",
				b.Name, b.Namespace, b.ResourceCount), nil
		}
	}
	return "", fmt.Errorf("backup %q not found", name)
}

func main() {
	p := &BackupManagerPlugin{
		settings: defaultSettings(),
		backups:  seedBackups(),
		nextID:   3,
	}
	sdk.Serve(p)
}

// seedBackups returns sample data so the plugin has data to show immediately.
func seedBackups() []backup {
	now := time.Now()
	return []backup{
		{
			ID: 1, Name: "backup-production-2025-01-15", Namespace: "production",
			Status: "completed", CreatedAt: now.Add(-48 * time.Hour),
			SizeBytes: 52428800, ResourceCount: 45,
		},
		{
			ID: 2, Name: "backup-staging-2025-01-16", Namespace: "staging",
			Status: "completed", CreatedAt: now.Add(-24 * time.Hour),
			SizeBytes: 31457280, ResourceCount: 28,
		},
		{
			ID: 3, Name: "backup-default-2025-01-17", Namespace: "default",
			Status: "completed", CreatedAt: now.Add(-2 * time.Hour),
			SizeBytes: 15728640, ResourceCount: 12,
		},
	}
}
