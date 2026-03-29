import { withSubPath } from './subpath'

// --- Types matching backend FrontendManifestWithPlugin ---

export interface PluginSidebarEntry {
  title: string
  icon: string
  section?: string
  priority?: number
}

export interface PluginFrontendRoute {
  path: string
  module: string
  sidebarEntry?: PluginSidebarEntry
}

export interface PluginFrontendManifest {
  remoteEntry: string
  exposedModules?: Record<string, string>
  routes?: PluginFrontendRoute[]
  settingsPanel?: string
}

export interface PluginManifestWithName {
  pluginName: string
  frontend: PluginFrontendManifest
}

// --- Module Federation Container Interface (standard protocol) ---

interface MFContainer {
  init(shareScope: Record<string, unknown>): Promise<void>
  get(module: string): Promise<() => Record<string, unknown>>
}

// --- Shared scope management (Task 3.6) ---
// Exposes host modules (React, ReactDOM, etc.) so plugins don't bundle them.

type SharedModule = {
  get: () => Promise<() => unknown>
  loaded: number
  from: string
}

let sharedScopeReady = false
const sharedScope: Record<string, Record<string, SharedModule>> = {}

function ensureSharedScope(): Record<string, Record<string, SharedModule>> {
  if (sharedScopeReady) return sharedScope

  const register = (
    name: string,
    version: string,
    getter: () => Promise<unknown>
  ) => {
    if (!sharedScope[name]) sharedScope[name] = {}
    sharedScope[name][version] = {
      get: () => getter().then((m) => () => m),
      loaded: 1,
      from: 'kite-host',
    }
  }

  // Core singletons — plugins share these with the host
  register('react', '19.0.0', () => import('react'))
  register('react-dom', '19.0.0', () => import('react-dom'))
  register('react-router-dom', '7.0.0', () => import('react-router-dom'))
  register('@tanstack/react-query', '5.0.0', () =>
    import('@tanstack/react-query')
  )

  sharedScopeReady = true
  return sharedScope
}

// --- Remote entry loading ---

const containers = new Map<string, MFContainer>()
const pendingLoads = new Map<string, Promise<MFContainer>>()

function containerScope(pluginName: string): string {
  return `plugin_${pluginName.replace(/-/g, '_')}`
}

function loadRemoteEntry(
  url: string,
  scope: string
): Promise<MFContainer> {
  const cached = containers.get(scope)
  if (cached) return Promise.resolve(cached)

  const pending = pendingLoads.get(scope)
  if (pending) return pending

  const win = window as unknown as Record<string, unknown>
  const existing = win[scope] as MFContainer | undefined
  if (existing) {
    containers.set(scope, existing)
    return Promise.resolve(existing)
  }

  const promise = new Promise<MFContainer>((resolve, reject) => {
    const script = document.createElement('script')
    script.src = url
    script.type = 'text/javascript'
    script.async = true
    script.onload = () => {
      const container = win[scope] as MFContainer | undefined
      if (!container) {
        reject(
          new Error(
            `Module Federation container "${scope}" not found after loading ${url}`
          )
        )
        return
      }
      containers.set(scope, container)
      pendingLoads.delete(scope)
      resolve(container)
    }
    script.onerror = () => {
      pendingLoads.delete(scope)
      reject(new Error(`Failed to load remote entry: ${url}`))
    }
    document.head.appendChild(script)
  })

  pendingLoads.set(scope, promise)
  return promise
}

// --- Public API ---

/**
 * Fetch plugin frontend manifests from the backend.
 */
export async function fetchPluginManifests(): Promise<
  PluginManifestWithName[]
> {
  try {
    const res = await fetch(withSubPath('/api/v1/plugins/frontends'), {
      credentials: 'include',
    })
    if (!res.ok) {
      console.warn('Failed to fetch plugin manifests:', res.status)
      return []
    }
    const data: PluginManifestWithName[] = await res.json()
    return Array.isArray(data) ? data : []
  } catch (err) {
    console.warn('Failed to fetch plugin manifests:', err)
    return []
  }
}

/**
 * Load a Module Federation module from a plugin's remote entry.
 *
 * @param pluginName - Unique plugin identifier (e.g. "cost-analyzer")
 * @param remoteEntryUrl - URL to the plugin's remoteEntry.js
 * @param moduleName - Exposed module name (e.g. "./CostDashboard")
 * @returns The module's exports
 */
export async function loadPluginModule<T = Record<string, unknown>>(
  pluginName: string,
  remoteEntryUrl: string,
  moduleName: string
): Promise<T> {
  const scope = containerScope(pluginName)
  const container = await loadRemoteEntry(remoteEntryUrl, scope)

  await container.init(
    ensureSharedScope() as unknown as Record<string, unknown>
  )

  const factory = await container.get(moduleName)
  return factory() as T
}

/**
 * Convert a kebab-case icon name to a Tabler icon class name.
 * Example: "currency-dollar" → "IconCurrencyDollar"
 */
export function toTablerIconName(name: string): string {
  return (
    'Icon' +
    name
      .split('-')
      .map((s) => s.charAt(0).toUpperCase() + s.slice(1))
      .join('')
  )
}
