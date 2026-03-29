/**
 * @kite-dashboard/plugin-sdk
 *
 * TypeScript SDK for Kite plugin frontend development.
 * Provides hooks, components, and build helpers for plugin authors.
 */

export { useKiteCluster } from './hooks/use-kite-cluster'
export { useKiteApi } from './hooks/use-kite-api'
export { usePluginApi } from './hooks/use-plugin-api'
export { KitePluginPage } from './components/kite-plugin-page'
export { definePluginFederation } from './vite/define-plugin-federation'

// Re-export types that plugin authors commonly need
export type { PluginFrontendManifest, PluginFrontendRoute, PluginManifestWithName } from '../src/lib/plugin-loader'
