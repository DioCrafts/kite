import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  fetchPluginManifests,
  loadPluginModule,
  toTablerIconName,
  type PluginManifestWithName,
} from '../plugin-loader'

// Reset containers/pending caches between tests by reimporting
// (vitest isolates modules per file, so the module-level state is fresh per test file)

describe('toTablerIconName', () => {
  it('converts single word', () => {
    expect(toTablerIconName('database')).toBe('IconDatabase')
  })

  it('converts kebab-case icon name', () => {
    expect(toTablerIconName('currency-dollar')).toBe('IconCurrencyDollar')
  })

  it('converts multiple hyphens', () => {
    expect(toTablerIconName('arrow-up-right')).toBe('IconArrowUpRight')
  })

  it('converts single uppercase char', () => {
    expect(toTablerIconName('x')).toBe('IconX')
  })
})

describe('fetchPluginManifests', () => {
  beforeEach(() => {
    vi.spyOn(globalThis, 'fetch')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns manifests on success', async () => {
    const mockData: PluginManifestWithName[] = [
      {
        pluginName: 'cost-analyzer',
        frontend: {
          remoteEntry: '/plugins/cost-analyzer/remoteEntry.js',
          routes: [{ path: '/cost', module: './CostDashboard' }],
        },
      },
    ]

    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(mockData),
    } as Response)

    const result = await fetchPluginManifests()
    expect(result).toHaveLength(1)
    expect(result[0].pluginName).toBe('cost-analyzer')
  })

  it('returns empty array on non-ok response', async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: false,
      status: 401,
    } as Response)

    const result = await fetchPluginManifests()
    expect(result).toEqual([])
  })

  it('returns empty array on network error', async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new Error('Network error'))

    const result = await fetchPluginManifests()
    expect(result).toEqual([])
  })

  it('returns empty array when response is not an array', async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ plugins: [] }), // object, not array
    } as Response)

    const result = await fetchPluginManifests()
    expect(result).toEqual([])
  })

  it('returns empty array when response is null', async () => {
    vi.mocked(fetch).mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(null),
    } as Response)

    const result = await fetchPluginManifests()
    expect(result).toEqual([])
  })
})

describe('loadPluginModule', () => {
  it('uses window container when already loaded', async () => {
    const mockComponent = { default: () => null }
    const mockFactory = () => mockComponent
    const mockContainer = {
      init: vi.fn().mockResolvedValue(undefined),
      get: vi.fn().mockResolvedValue(mockFactory),
    }

    // Simulate container already set on window (as if previously loaded)
    const scope = 'plugin_cost_analyzer'
    ;(window as unknown as Record<string, unknown>)[scope] = mockContainer

    const result = await loadPluginModule(
      'cost-analyzer',
      '/plugins/cost-analyzer/remoteEntry.js',
      './CostDashboard'
    )

    expect(mockContainer.init).toHaveBeenCalledOnce()
    expect(mockContainer.get).toHaveBeenCalledWith('./CostDashboard')
    expect(result).toEqual(mockComponent)

    // Cleanup
    delete (window as unknown as Record<string, unknown>)[scope]
  })

  it('rejects when script load fails', async () => {
    const scope = 'plugin_failing_plugin'
    delete (window as unknown as Record<string, unknown>)[scope]

    // Override document.head.appendChild to immediately fire onerror
    const origAppendChild = document.head.appendChild.bind(document.head)
    vi.spyOn(document.head, 'appendChild').mockImplementationOnce((el) => {
      const script = el as HTMLScriptElement
      setTimeout(() => {
        script.onerror?.(new Event('error'))
      }, 0)
      return el
    })

    await expect(
      loadPluginModule(
        'failing-plugin',
        '/plugins/failing/remoteEntry.js',
        './Component'
      )
    ).rejects.toThrow('Failed to load remote entry')

    vi.spyOn(document.head, 'appendChild').mockRestore?.()
    void origAppendChild // ref to suppress unused var warning
  })

  it('rejects when container not found after script loads', async () => {
    const scope = 'plugin_missing_container'
    delete (window as unknown as Record<string, unknown>)[scope]

    vi.spyOn(document.head, 'appendChild').mockImplementationOnce((el) => {
      const script = el as HTMLScriptElement
      // Fire onload but without setting the container
      setTimeout(() => {
        script.onload?.(new Event('load'))
      }, 0)
      return el
    })

    await expect(
      loadPluginModule(
        'missing-container',
        '/plugins/missing/remoteEntry.js',
        './Component'
      )
    ).rejects.toThrow('not found after loading')
  })
})
