import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import type { ComponentType } from 'react'

import { usePlugins } from '@/contexts/plugin-context'
import { apiClient } from '@/lib/api-client'
import { loadPluginModule } from '@/lib/plugin-loader'

import { PluginErrorBoundary } from './plugin-error-boundary'

interface PluginSettingsProps {
  pluginConfig: Record<string, unknown>
  onSave: (config: Record<string, unknown>) => Promise<void>
}

interface PluginSettingsPanelProps {
  pluginName: string
  remoteEntry: string
  settingsPanel: string
}

function PanelSkeleton() {
  return (
    <div className="space-y-4 animate-pulse">
      <div className="h-8 w-48 rounded bg-muted" />
      <div className="h-10 w-full rounded bg-muted" />
      <div className="h-10 w-full rounded bg-muted" />
      <div className="h-10 w-32 rounded bg-muted" />
    </div>
  )
}

function PluginSettingsPanelInner({
  pluginName,
  remoteEntry,
  settingsPanel,
}: PluginSettingsPanelProps) {
  const [settings, setSettings] = useState<Record<string, unknown>>({})
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    apiClient
      .get<Record<string, unknown>>(`/admin/plugins/${pluginName}/settings`)
      .then(setSettings)
      .catch(() => setSettings({}))
      .finally(() => setLoading(false))
  }, [pluginName])

  const handleSave = async (config: Record<string, unknown>) => {
    await apiClient.put(`/admin/plugins/${pluginName}/settings`, config)
    setSettings(config)
  }

  const LazyPanel = useMemo(() => {
    return lazy(async () => {
      const mod = await loadPluginModule<{
        default: ComponentType<PluginSettingsProps>
      }>(pluginName, remoteEntry, settingsPanel)
      return { default: mod.default }
    })
  }, [pluginName, remoteEntry, settingsPanel])

  if (loading) return <PanelSkeleton />

  return (
    <Suspense fallback={<PanelSkeleton />}>
      <LazyPanel pluginConfig={settings} onSave={handleSave} />
    </Suspense>
  )
}

/**
 * Returns additional ResponsiveTabs tab items for plugins that declare
 * a settingsPanel in their frontend manifest.
 */
export function usePluginSettingsTabs() {
  const { plugins } = usePlugins()

  return useMemo(
    () =>
      plugins
        .filter((p) => p.frontend.settingsPanel)
        .map((p) => ({
          value: `plugin-${p.pluginName}`,
          label: p.pluginName,
          content: (
            <PluginErrorBoundary pluginName={p.pluginName}>
              <PluginSettingsPanelInner
                pluginName={p.pluginName}
                remoteEntry={p.frontend.remoteEntry}
                settingsPanel={p.frontend.settingsPanel!}
              />
            </PluginErrorBoundary>
          ),
        })),
    [plugins]
  )
}
