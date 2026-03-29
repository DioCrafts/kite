import type { ComponentType } from 'react'

/**
 * Props passed to every component injected into a detail-page slot.
 * `resource` is the raw Kubernetes object (typed as unknown so plugins
 * can cast it to whatever they expect without coupling to host types).
 */
export interface SlotComponentProps {
  resource: unknown
  cluster: string
  namespace?: string
}

/**
 * A column definition contributed by a plugin into a resource-list table.
 * Mirrors the shape expected by @tanstack/react-table ColumnDef.
 */
export interface PluginColumn<T = unknown> {
  /** Unique column id within the plugin */
  id: string
  /** Header label */
  header: string
  /** Cell renderer function — receives the row's original data */
  cell: (row: T) => React.ReactNode
  /** Lower values appear earlier; default 100 */
  priority?: number
}

interface RegisteredComponent {
  pluginName: string
  component: ComponentType<SlotComponentProps>
  priority: number
}

interface RegisteredColumns<T = unknown> {
  pluginName: string
  columns: PluginColumn<T>[]
  priority: number
}

// ── Singletons ──────────────────────────────────────────────────────────────

const componentSlots = new Map<string, RegisteredComponent[]>()
const columnSlots = new Map<string, RegisteredColumns[]>()
const changeListeners = new Set<() => void>()

// ── Notification helpers ─────────────────────────────────────────────────────

function notify() {
  changeListeners.forEach((fn) => fn())
}

/**
 * Subscribe to registry changes (used by useSlotComponents / usePluginTableColumns).
 * Returns an unsubscribe function.
 */
export function subscribeToRegistry(fn: () => void): () => void {
  changeListeners.add(fn)
  return () => changeListeners.delete(fn)
}

// ── Component slot API ────────────────────────────────────────────────────────

/**
 * Register a React component into a named slot on a Kite detail page.
 *
 * Called as a side-effect when a plugin's injection module is loaded via
 * Module Federation.
 *
 * @param slot      Slot name, e.g. "pod-detail", "deployment-detail"
 * @param pluginName Unique plugin identifier
 * @param component  React component that receives { resource, cluster, namespace }
 * @param priority   Lower values render first (default 100)
 */
export function registerSlotComponent(
  slot: string,
  pluginName: string,
  component: ComponentType<SlotComponentProps>,
  priority = 100
): void {
  const existing = componentSlots.get(slot) ?? []
  // Avoid double-registration across HMR / React StrictMode
  const alreadyRegistered = existing.some((r) => r.pluginName === pluginName)
  if (alreadyRegistered) return

  const updated = [...existing, { pluginName, component, priority }].sort(
    (a, b) => a.priority - b.priority
  )
  componentSlots.set(slot, updated)
  notify()
}

const EMPTY_COMPONENTS: RegisteredComponent[] = []

/**
 * Return all components registered for a slot, sorted by priority ascending.
 */
export function getSlotComponents(slot: string): RegisteredComponent[] {
  return componentSlots.get(slot) ?? EMPTY_COMPONENTS
}

// ── Table column slot API ─────────────────────────────────────────────────────

/**
 * Register extra table columns for a resource-list page.
 *
 * @param slot       Slot name, e.g. "pods-table", "deployments-table"
 * @param pluginName Unique plugin identifier
 * @param columns    Array of PluginColumn definitions
 * @param priority   Lower values appear earlier in column order (default 100)
 */
export function registerTableColumns<T = unknown>(
  slot: string,
  pluginName: string,
  columns: PluginColumn<T>[],
  priority = 100
): void {
  const existing = columnSlots.get(slot) ?? []
  const alreadyRegistered = existing.some((r) => r.pluginName === pluginName)
  if (alreadyRegistered) return

  const updated = [...existing, { pluginName, columns: columns as PluginColumn[], priority }].sort(
    (a, b) => a.priority - b.priority
  )
  columnSlots.set(slot, updated)
  notify()
}

const EMPTY_COLUMNS: PluginColumn[] = []

/**
 * Return all plugin columns registered for a table slot, sorted by priority.
 */
export function getTableColumns<T = unknown>(slot: string): PluginColumn<T>[] {
  const registrations = columnSlots.get(slot)
  if (!registrations || registrations.length === 0) return EMPTY_COLUMNS as PluginColumn<T>[]
  return registrations.flatMap((r) => r.columns as PluginColumn<T>[])
}

// ── Cleanup (used on plugin uninstall / hot-reload) ──────────────────────────

/**
 * Remove all registrations for a given plugin.
 * Called by PluginProvider when a plugin is uninstalled or reloaded.
 */
export function unregisterPlugin(pluginName: string): void {
  for (const [slot, entries] of componentSlots) {
    componentSlots.set(
      slot,
      entries.filter((r) => r.pluginName !== pluginName)
    )
  }
  for (const [slot, entries] of columnSlots) {
    columnSlots.set(
      slot,
      entries.filter((r) => r.pluginName !== pluginName)
    )
  }
  notify()
}

// ── Reset (for tests) ─────────────────────────────────────────────────────────

/**
 * Clear all registrations. Used in unit tests to ensure isolation between cases.
 */
export function resetRegistry(): void {
  componentSlots.clear()
  columnSlots.clear()
  // Don't clear listeners — subscribers should still work after a reset
  notify()
}
