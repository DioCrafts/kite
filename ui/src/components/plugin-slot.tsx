import { useSyncExternalStore } from 'react'
import { getSlotComponents, subscribeToRegistry } from '@/lib/plugin-registry'
import { PluginErrorBoundary } from './plugin-error-boundary'

interface PluginSlotProps {
  /** Slot identifier, e.g. "pod-detail", "deployment-detail" */
  slot: string
  /** The Kubernetes resource object passed to each injected component */
  resource: unknown
  /** Current cluster name */
  cluster: string
  /** Optional namespace */
  namespace?: string
}

/**
 * Renders all plugin components registered for a named slot.
 * Each component is individually wrapped in a PluginErrorBoundary so a crash
 * in one plugin never affects the host page or other plugin slots.
 *
 * Usage in a detail page:
 * ```tsx
 * <PluginSlot slot="pod-detail" resource={pod} cluster={currentCluster} namespace={namespace} />
 * ```
 */
export function PluginSlot({ slot, resource, cluster, namespace }: PluginSlotProps) {
  const components = useSyncExternalStore(
    subscribeToRegistry,
    () => getSlotComponents(slot),
    () => getSlotComponents(slot)
  )

  if (components.length === 0) return null

  return (
    <>
      {components.map(({ pluginName, component: Component }) => (
        <PluginErrorBoundary key={pluginName} pluginName={pluginName}>
          <Component resource={resource} cluster={cluster} namespace={namespace} />
        </PluginErrorBoundary>
      ))}
    </>
  )
}
