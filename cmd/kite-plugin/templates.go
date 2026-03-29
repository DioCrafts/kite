package main

// Scaffold templates for `kite-plugin init`.
// Each template uses Go text/template syntax with a scaffoldData context.

var mainGoTmpl = `package main

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/zxh326/kite/pkg/plugin"
	"github.com/zxh326/kite/pkg/plugin/sdk"
)

type {{.NameTitle}}Plugin struct {
	sdk.BasePlugin
}

func (p *{{.NameTitle}}Plugin) Manifest() plugin.PluginManifest {
	return plugin.PluginManifest{
		Name:        "{{.Name}}",
		Version:     "0.1.0",
		Description: "{{.NameTitle}} plugin for Kite",
		Author:      "Your Name",
		Permissions: []plugin.Permission{
			{Resource: "pods", Verbs: []string{"get", "list"}},
		},{{if .WithFrontend}}
		Frontend: &plugin.FrontendManifest{
			RemoteEntry: "/plugins/{{.Name}}/static/remoteEntry.js",
			ExposedModules: map[string]string{
				"./Page":     "PluginPage",
				"./Settings": "Settings",
			},
			Routes: []plugin.FrontendRoute{
				{
					Path:   "/",
					Module: "./Page",
					SidebarEntry: &plugin.SidebarEntry{
						Title:   "{{.NameTitle}}",
						Icon:    "box",
						Section: "plugins",
					},
				},
			},
			SettingsPanel: "./Settings",
		},{{end}}
	}
}

func (p *{{.NameTitle}}Plugin) RegisterRoutes(group gin.IRoutes) {
	group.GET("/hello", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "Hello from {{.Name}} plugin!"})
	})
}

func (p *{{.NameTitle}}Plugin) RegisterAITools() []plugin.AIToolDefinition {
	return []plugin.AIToolDefinition{
		sdk.NewAITool(
			"hello",
			"Say hello from the {{.Name}} plugin",
			map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Name to greet",
				},
			},
			[]string{"name"},
		),
	}
}

func (p *{{.NameTitle}}Plugin) Shutdown(ctx context.Context) error {
	sdk.Logger().Info("{{.Name}} plugin shutting down")
	return nil
}

func main() {
	sdk.Serve(&{{.NameTitle}}Plugin{})
}
`

var manifestYamlTmpl = `name: {{.Name}}
version: 0.1.0
description: "{{.NameTitle}} plugin for Kite"
author: "Your Name"
priority: 100
rateLimit: 100

permissions:
  - resource: pods
    verbs: [get, list]
{{if .WithFrontend}}
frontend:
  remoteEntry: "/plugins/{{.Name}}/static/remoteEntry.js"
  exposedModules:
    ./Page: PluginPage
    ./Settings: Settings
  routes:
    - path: "/"
      module: "./Page"
      sidebarEntry:
        title: "{{.NameTitle}}"
        icon: "box"
        section: "plugins"
  settingsPanel: "./Settings"
{{end}}
settings:
  - name: enabled
    label: "Enable {{.NameTitle}}"
    type: boolean
    default: "true"
    description: "Enable or disable this plugin"
`

var goModTmpl = `module {{.Name}}-plugin

go 1.25.0

require (
	github.com/gin-gonic/gin v1.12.0
	github.com/zxh326/kite v0.0.0
)
`

var makefileTmpl = `.PHONY: build dev clean test

PLUGIN_NAME = {{.Name}}
BINARY = $(PLUGIN_NAME)

build:
	go build -o $(BINARY) .
{{if .WithFrontend}}	cd frontend && pnpm build{{end}}

dev:
	go build -o $(BINARY) .
	@echo "Plugin binary built: ./$(BINARY)"

clean:
	rm -f $(BINARY)
{{if .WithFrontend}}	rm -rf frontend/dist{{end}}

test:
	go test ./...
`

var readmeTmpl = `# {{.NameTitle}} Plugin

A Kite plugin that provides ...

## Development

` + "```" + `bash
# Build the plugin
make build

# Run tests
make test
` + "```" + `

## Installation

Copy the built plugin directory to Kite's plugin directory:

` + "```" + `bash
cp -r . $KITE_PLUGIN_DIR/{{.Name}}/
` + "```" + `
`

// --- Frontend templates ---

var frontendPackageJsonTmpl = `{
  "name": "{{.Name}}-plugin-frontend",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
  },
  "devDependencies": {
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0",
    "@vitejs/plugin-react": "^4.0.0",
    "typescript": "^5.7.0",
    "vite": "^6.0.0"
  }
}
`

var frontendViteConfigTmpl = `import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// Module Federation for Kite plugin
// The host (Kite) loads this plugin's remoteEntry.js at runtime.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    lib: {
      entry: {
        PluginPage: './src/PluginPage.tsx',
        Settings: './src/Settings.tsx',
      },
      formats: ['es'],
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'react-router-dom', '@tanstack/react-query'],
      output: {
        entryFileNames: '[name].js',
      },
    },
  },
})
`

var frontendTsconfigTmpl = `{
  "compilerOptions": {
    "target": "ES2020",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true,
    "noEmit": true,
    "skipLibCheck": true
  },
  "include": ["src"]
}
`

var frontendPluginPageTmpl = `import { useState, useEffect } from 'react'

export default function PluginPage() {
  const [message, setMessage] = useState<string>('')

  useEffect(() => {
    fetch('/api/v1/plugins/{{.Name}}/hello', { credentials: 'include' })
      .then((r) => r.json())
      .then((data) => setMessage(data.message))
      .catch(() => setMessage('Failed to load'))
  }, [])

  return (
    <div style={{padding: '24px'}}>
      <h1>{{.NameTitle}} Plugin</h1>
      <p>{message || 'Loading...'}</p>
    </div>
  )
}
`

var frontendSettingsTmpl = `import { useState } from 'react'

interface SettingsProps {
  pluginConfig: Record<string, unknown>
  onSave: (config: Record<string, unknown>) => Promise<void>
}

export default function Settings({ pluginConfig, onSave }: SettingsProps) {
  const [enabled, setEnabled] = useState(pluginConfig.enabled !== false)
  const [saving, setSaving] = useState(false)

  const handleSave = async () => {
    setSaving(true)
    try {
      await onSave({ ...pluginConfig, enabled })
    } finally {
      setSaving(false)
    }
  }

  return (
    <div style={{display: 'flex', flexDirection: 'column', gap: '16px', maxWidth: '400px'}}>
      <label style={{display: 'flex', alignItems: 'center', gap: '8px'}}>
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
        />
        Enable {{.NameTitle}}
      </label>
      <button onClick={handleSave} disabled={saving}>
        {saving ? 'Saving...' : 'Save Settings'}
      </button>
    </div>
  )
}
`
