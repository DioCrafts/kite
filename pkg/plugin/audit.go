package plugin

import (
	"github.com/zxh326/kite/pkg/model"
	"k8s.io/klog/v2"
)

// persistAuditRecord writes a plugin action to the ResourceHistory table.
// This integrates plugin operations into the existing Kite audit log.
func (pm *PluginManager) persistAuditRecord(pluginName, toolName, resource, action, clusterName string, userID uint, success bool, errMsg string) {
	if model.DB == nil {
		return
	}

	resourceType := "plugin"
	resourceName := pluginName
	operationType := "plugin_" + action
	operationSource := "plugin"

	if toolName != "" {
		resourceType = "plugin_tool"
		resourceName = pluginName + "/" + toolName
	}
	if resource != "" {
		resourceType = "plugin_resource"
		resourceName = pluginName + "/" + resource
	}

	record := model.ResourceHistory{
		ClusterName:     clusterName,
		ResourceType:    resourceType,
		ResourceName:    resourceName,
		Namespace:       "",
		OperationType:   operationType,
		OperationSource: operationSource,
		Success:         success,
		ErrorMessage:    errMsg,
		OperatorID:      userID,
	}

	if err := model.DB.Create(&record).Error; err != nil {
		klog.Errorf("Failed to persist plugin audit record: %v", err)
	}
}
