/**
 * Vite configuration helper for Kite plugin frontends.
 *
 * Generates a Module Federation-compatible Vite config that lets Kite
 * load the plugin's components at runtime.
 *
 * @example
 * ```ts
 * // vite.config.ts
 * import react from '@vitejs/plugin-react'
 * import { defineConfig } from 'vite'
 * import { definePluginFederation } from '@kite-dashboard/plugin-sdk/vite'
 *
 * export default defineConfig({
 *   plugins: [
 *     react(),
 *     ...definePluginFederation({
 *       name: 'cost-analyzer',
 *       exposes: {
 *         './CostDashboard': './src/CostDashboard.tsx',
 *         './Settings': './src/Settings.tsx',
 *       },
 *     }),
 *   ],
 * })
 * ```
 */
export interface PluginFederationOptions {
  /** Plugin name — must match the name in manifest.yaml */
  name: string
  /** Map of exposed module names to file paths */
  exposes: Record<string, string>
}

/**
 * Returns a Vite plugin array that configures the build output for
 * runtime Module Federation loading by Kite.
 *
 * This creates an ES module library build with external React/Router
 * dependencies (provided by the Kite host at runtime).
 */
export function definePluginFederation(options: PluginFederationOptions) {
  const entries: Record<string, string> = {}
  for (const [key, value] of Object.entries(options.exposes)) {
    // Convert "./CostDashboard" → "CostDashboard"
    const entryName = key.replace(/^\.\//, '')
    entries[entryName] = value
  }

  return {
    build: {
      outDir: 'dist',
      lib: {
        entry: entries,
        formats: ['es'] as const,
        fileName: (_format: string, entryName: string) => `${entryName}.js`,
      },
      rollupOptions: {
        external: [
          'react',
          'react-dom',
          'react-router-dom',
          '@tanstack/react-query',
        ],
        output: {
          globals: {
            react: 'React',
            'react-dom': 'ReactDOM',
            'react-router-dom': 'ReactRouterDOM',
            '@tanstack/react-query': 'ReactQuery',
          },
        },
      },
    },
  }
}
