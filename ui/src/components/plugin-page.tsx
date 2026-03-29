import { lazy, Suspense, useMemo } from 'react'
import type { ComponentType } from 'react'
import { useParams } from 'react-router-dom'

import { usePlugins } from '@/contexts/plugin-context'
import { loadPluginModule } from '@/lib/plugin-loader'

import { PluginErrorBoundary } from './plugin-error-boundary'

function PluginLoading() {
  return (
    <div className="flex items-center justify-center p-12">
      <div className="h-6 w-6 animate-spin rounded-full border-2 border-muted border-t-primary" />
    </div>
  )
}

function PluginNotFound({ name }: { name: string }) {
  return (
    <div className="flex flex-col items-center justify-center p-12 text-center">
      <h2 className="text-xl font-semibold">Plugin Not Found</h2>
      <p className="mt-2 text-muted-foreground">
        The plugin &ldquo;{name}&rdquo; is not available or has no frontend.
      </p>
    </div>
  )
}

export function PluginPage() {
  const { pluginName, '*': subPath } = useParams()
  const { plugins, isLoading } = usePlugins()

  const manifest = plugins.find((p) => p.pluginName === pluginName)

  // Find the matching route in the plugin's manifest
  const matchedRoute = useMemo(() => {
    if (!manifest?.frontend.routes) return null
    const normalizedPath = '/' + (subPath || '')
    return (
      manifest.frontend.routes.find(
        (r) =>
          normalizedPath === r.path || normalizedPath.startsWith(r.path + '/')
      ) ?? manifest.frontend.routes[0] ?? null
    )
  }, [manifest, subPath])

  // Create a stable lazy component reference for the matched route
  const LazyComponent = useMemo(() => {
    if (!manifest || !matchedRoute) return null
    return lazy(async () => {
      const mod = await loadPluginModule<{
        default: ComponentType
      }>(manifest.pluginName, manifest.frontend.remoteEntry, matchedRoute.module)
      return { default: mod.default }
    })
  }, [manifest, matchedRoute])

  if (isLoading) return <PluginLoading />
  if (!pluginName || !manifest || !LazyComponent)
    return <PluginNotFound name={pluginName || 'unknown'} />

  return (
    <PluginErrorBoundary pluginName={pluginName}>
      <Suspense fallback={<PluginLoading />}>
        <LazyComponent />
      </Suspense>
    </PluginErrorBoundary>
  )
}
