package model

import (
	"encoding/json"
	"fmt"
)

// PluginSetting stores per-plugin configuration and enabled state.
type PluginSetting struct {
	Model
	PluginName string `json:"pluginName" gorm:"uniqueIndex;not null"`
	Enabled    bool   `json:"enabled" gorm:"default:true"`
	Config     string `json:"config" gorm:"type:text"` // JSON-encoded settings map
}

// SavePluginSettings persists a plugin's configuration to the database.
func SavePluginSettings(name string, settings map[string]any) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal plugin settings: %w", err)
	}

	var ps PluginSetting
	result := DB.Where("plugin_name = ?", name).First(&ps)
	if result.Error != nil {
		// Create new record
		ps = PluginSetting{
			PluginName: name,
			Enabled:    true,
			Config:     string(data),
		}
		return DB.Create(&ps).Error
	}

	ps.Config = string(data)
	return DB.Save(&ps).Error
}

// GetPluginSettings retrieves a plugin's configuration from the database.
func GetPluginSettings(name string) (map[string]any, error) {
	var ps PluginSetting
	result := DB.Where("plugin_name = ?", name).First(&ps)
	if result.Error != nil {
		return map[string]any{}, nil
	}

	var settings map[string]any
	if ps.Config == "" {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal([]byte(ps.Config), &settings); err != nil {
		return nil, fmt.Errorf("unmarshal plugin settings: %w", err)
	}
	return settings, nil
}

// SetPluginEnabled updates a plugin's enabled state in the database.
func SetPluginEnabled(name string, enabled bool) error {
	var ps PluginSetting
	result := DB.Where("plugin_name = ?", name).First(&ps)
	if result.Error != nil {
		// Create new record with default config
		ps = PluginSetting{
			PluginName: name,
			Enabled:    enabled,
		}
		return DB.Create(&ps).Error
	}

	ps.Enabled = enabled
	return DB.Save(&ps).Error
}
