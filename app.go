package main

import (
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/internal"
	"github.com/zxh326/kite/pkg/cluster"
	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/handlers"
	"github.com/zxh326/kite/pkg/middleware"
	"github.com/zxh326/kite/pkg/model"
	"github.com/zxh326/kite/pkg/plugin"
	"github.com/zxh326/kite/pkg/rbac"
	"k8s.io/klog/v2"
)

func initializeApp() (*cluster.ClusterManager, *plugin.PluginManager, error) {
	common.LoadEnvs()
	if klog.V(1).Enabled() {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	model.InitDB()
	if _, err := model.GetGeneralSetting(); err != nil {
		klog.Warningf("Failed to load general setting: %v", err)
	}

	rbac.InitRBAC()
	handlers.InitTemplates()
	internal.LoadConfigFromEnv()

	cm, err := cluster.NewClusterManager()
	if err != nil {
		return nil, nil, err
	}

	pm := plugin.NewPluginManager(common.PluginDir)
	if err := pm.LoadPlugins(); err != nil {
		klog.Warningf("Failed to load plugins: %v", err)
	}

	return cm, pm, nil
}

func buildEngine(cm *cluster.ClusterManager, pm *plugin.PluginManager) *gin.Engine {
	r := gin.New()
	r.Use(middleware.Metrics())
	if !common.DisableGZIP {
		klog.Info("GZIP compression is enabled")
		r.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/metrics"})))
	}
	r.Use(gin.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.DevCORS(common.CORSAllowedOrigins))
	r.Use(pm.PluginMiddleware())

	base := r.Group(common.Base)
	setupAPIRouter(base, cm, pm)
	setupStatic(r)

	return r
}
