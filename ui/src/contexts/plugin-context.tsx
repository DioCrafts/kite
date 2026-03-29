import { createContext, useContext, useEffect, useState, useCallback } from 'react'
import type { ReactNode } from 'react'

import {
  fetchPluginManifests,
  loadPluginModule,
  PluginManifestWithName,
} from '@/lib/plugin-loader'
import { unregisterPlugin } from '@/lib/plugin-registry'

interface PluginContextType {
  plugins: PluginManifestWithName[]
  isLoading: boolean
  /** Re-fetch manifests and reload injection modules (called after install/uninstall/reload) */
  refreshPlugins: () => Promise<void>
}

const PluginContext = createContext<PluginContextType>({
  plugins: [],
  isLoading: true,
  refreshPlugins: async () => {},
})

async function loadInjections(manifests: PluginManifestWithName[]) {
  for (const plugin of manifests) {
    const injections = plugin.frontend.injections ?? []
    for (const injection of injections) {
      try {
        // Loading the module is enough — the module self-registers as a side-effect
        await loadPluginModule(
          plugin.pluginName,
          plugin.frontend.remoteEntry,
          injection.module
        )
      } catch (err) {
        console.error(
          `Failed to load injection module "${injection.module}" from plugin "${plugin.pluginName}":`,
          err
        )
      }
    }
  }
}

export function PluginProvider({ children }: { children: ReactNode }) {
  const [plugins, setPlugins] = useState<PluginManifestWithName[]>([])
  const [isLoading, setIsLoading] = useState(true)

  const refreshPlugins = useCallback(async () => {
    // Clear slot registrations for all current plugins before reloading
    // so stale injection components don't persist after a reload/uninstall
    setPlugins((prev) => {
      prev.forEach((p) => unregisterPlugin(p.pluginName))
      return prev
    })
    try {
      const manifests = await fetchPluginManifests()
      await loadInjections(manifests)
      setPlugins(manifests)
    } catch (err) {
      console.error('Failed to refresh plugin manifests:', err)
    }
  }, [])

  useEffect(() => {
    fetchPluginManifests()
      .then(async (manifests) => {
        await loadInjections(manifests)
        setPlugins(manifests)
      })
      .catch((err) => {
        console.error('Failed to load plugin manifests:', err)
      })
      .finally(() => setIsLoading(false))
  }, [])

  return (
    <PluginContext.Provider value={{ plugins, isLoading, refreshPlugins }}>
      {children}
    </PluginContext.Provider>
  )
}

export function usePlugins() {
  return useContext(PluginContext)
}
