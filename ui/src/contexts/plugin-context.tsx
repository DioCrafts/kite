import { createContext, useContext, useEffect, useState } from 'react'
import type { ReactNode } from 'react'

import {
  fetchPluginManifests,
  PluginManifestWithName,
} from '@/lib/plugin-loader'

interface PluginContextType {
  plugins: PluginManifestWithName[]
  isLoading: boolean
}

const PluginContext = createContext<PluginContextType>({
  plugins: [],
  isLoading: true,
})

export function PluginProvider({ children }: { children: ReactNode }) {
  const [plugins, setPlugins] = useState<PluginManifestWithName[]>([])
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    fetchPluginManifests()
      .then(setPlugins)
      .catch((err) => {
        console.error('Failed to load plugin manifests:', err)
      })
      .finally(() => setIsLoading(false))
  }, [])

  return (
    <PluginContext.Provider value={{ plugins, isLoading }}>
      {children}
    </PluginContext.Provider>
  )
}

export function usePlugins() {
  return useContext(PluginContext)
}
