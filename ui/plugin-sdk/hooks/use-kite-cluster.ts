import { useContext } from 'react'
import { ClusterContext } from '../../src/contexts/cluster-context'

/**
 * Hook to access the current Kite cluster from a plugin component.
 *
 * @example
 * ```tsx
 * const { currentCluster, clusters } = useKiteCluster()
 * ```
 */
export function useKiteCluster() {
  const ctx = useContext(ClusterContext)
  if (!ctx) {
    throw new Error('useKiteCluster must be used within a Kite plugin context')
  }
  return {
    /** Name of the currently selected cluster */
    currentCluster: ctx.currentCluster,
    /** All available clusters */
    clusters: ctx.clusters,
    /** Whether cluster data is loading */
    isLoading: ctx.isLoading,
  }
}
