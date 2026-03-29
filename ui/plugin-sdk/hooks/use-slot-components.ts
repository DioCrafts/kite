import { useSyncExternalStore } from 'react'
import {
  getSlotComponents,
  getTableColumns,
  subscribeToRegistry,
} from '../../src/lib/plugin-registry'
import type {
  SlotComponentProps,
  PluginColumn,
} from '../../src/lib/plugin-registry'

/**
 * Returns the list of registered components for a detail-page slot.
 * Re-renders automatically when the registry changes (plugin install/uninstall).
 *
 * @param slot  e.g. "pod-detail", "deployment-detail", "node-detail"
 */
export function useSlotComponents(slot: string) {
  return useSyncExternalStore(
    subscribeToRegistry,
    () => getSlotComponents(slot),
    () => getSlotComponents(slot)
  )
}

/**
 * Returns the merged list of ColumnDef-compatible columns from all plugins
 * registered for a table slot.
 *
 * @param slot  e.g. "pods-table", "deployments-table"
 */
export function usePluginTableColumns<T = unknown>(slot: string): PluginColumn<T>[] {
  return useSyncExternalStore(
    subscribeToRegistry,
    () => getTableColumns<T>(slot),
    () => getTableColumns<T>(slot)
  )
}

export type { SlotComponentProps, PluginColumn }
