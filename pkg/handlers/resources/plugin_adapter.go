package resources

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/plugin"
)

// pluginResourceAdapter wraps a plugin.ResourceHandler to satisfy the
// internal resourceHandler interface. Methods not supported by the plugin
// interface (Search, GetResource, registerCustomRoutes, ListHistory, Describe)
// return sensible defaults or not-implemented responses.
type pluginResourceAdapter struct {
	handler      plugin.ResourceHandler
	clusterScope bool
}

func newPluginResourceAdapter(h plugin.ResourceHandler) *pluginResourceAdapter {
	return &pluginResourceAdapter{
		handler:      h,
		clusterScope: h.IsClusterScoped(),
	}
}

func (a *pluginResourceAdapter) List(c *gin.Context)           { a.handler.List(c) }
func (a *pluginResourceAdapter) Get(c *gin.Context)            { a.handler.Get(c) }
func (a *pluginResourceAdapter) Create(c *gin.Context)         { a.handler.Create(c) }
func (a *pluginResourceAdapter) Update(c *gin.Context)         { a.handler.Update(c) }
func (a *pluginResourceAdapter) Delete(c *gin.Context)         { a.handler.Delete(c) }
func (a *pluginResourceAdapter) Patch(c *gin.Context)          { a.handler.Patch(c) }
func (a *pluginResourceAdapter) IsClusterScoped() bool         { return a.clusterScope }
func (a *pluginResourceAdapter) Searchable() bool              { return false }
func (a *pluginResourceAdapter) registerCustomRoutes(_ *gin.RouterGroup) {}

func (a *pluginResourceAdapter) Search(_ *gin.Context, _ string, _ int64) ([]common.SearchResult, error) {
	return nil, nil
}

func (a *pluginResourceAdapter) GetResource(_ *gin.Context, _, _ string) (interface{}, error) {
	return nil, nil
}

func (a *pluginResourceAdapter) ListHistory(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "history not supported for plugin resources"})
}

func (a *pluginResourceAdapter) Describe(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "describe not supported for plugin resources"})
}
