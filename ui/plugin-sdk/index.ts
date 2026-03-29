/**
 * @kite-dashboard/plugin-sdk
 *
 * TypeScript SDK for Kite plugin frontend development.
 * Provides hooks, components, and build helpers for plugin authors.
 */

export { useKiteCluster } from './hooks/use-kite-cluster'
export { useKiteApi } from './hooks/use-kite-api'
export { usePluginApi } from './hooks/use-plugin-api'
export { useSlotComponents, usePluginTableColumns } from './hooks/use-slot-components'
export { KitePluginPage } from './components/kite-plugin-page'
export { definePluginFederation } from './vite/define-plugin-federation'

// Registry functions used as side-effects in injection modules
export {
  registerSlotComponent,
  registerTableColumns,
} from '../src/lib/plugin-registry'

// Re-export types that plugin authors commonly need
export type { PluginFrontendManifest, PluginFrontendRoute, PluginManifestWithName, PluginInjection } from '../src/lib/plugin-loader'
export type { SlotComponentProps, PluginColumn } from './hooks/use-slot-components'
