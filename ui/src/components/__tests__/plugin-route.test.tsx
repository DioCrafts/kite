import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'

import { PluginErrorBoundary } from '../plugin-error-boundary'
import { PluginProvider, usePlugins } from '../../contexts/plugin-context'
import type { PluginManifestWithName } from '../../lib/plugin-loader'
import * as pluginLoader from '../../lib/plugin-loader'

vi.mock('../../lib/plugin-loader', async () => {
  const actual = await vi.importActual<typeof import('../../lib/plugin-loader')>(
    '../../lib/plugin-loader'
  )
  return {
    ...actual,
    fetchPluginManifests: vi.fn(),
  }
})

// --- PluginErrorBoundary Tests ---

describe('PluginErrorBoundary', () => {
  // Silence React's console.error for expected errors in error-boundary tests
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
  })

  it('renders children when no error', () => {
    render(
      <PluginErrorBoundary pluginName="test-plugin">
        <div data-testid="child">Hello</div>
      </PluginErrorBoundary>
    )
    expect(screen.getByTestId('child')).toBeInTheDocument()
  })

  it('renders fallback UI when child throws', () => {
    const ThrowingComponent = () => {
      throw new Error('Plugin crashed!')
    }

    render(
      <PluginErrorBoundary pluginName="crash-plugin">
        <ThrowingComponent />
      </PluginErrorBoundary>
    )

    expect(screen.getByText(/Plugin Error: crash-plugin/i)).toBeInTheDocument()
    expect(screen.getByText(/Plugin crashed!/i)).toBeInTheDocument()
  })

  it('shows Retry button when error occurs', () => {
    const ThrowingComponent = () => {
      throw new Error('boom')
    }

    render(
      <PluginErrorBoundary pluginName="boom-plugin">
        <ThrowingComponent />
      </PluginErrorBoundary>
    )

    expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument()
  })

  it('recovers after clicking Retry', async () => {
    let shouldThrow = true

    const ConditionalThrower = () => {
      if (shouldThrow) throw new Error('temporary error')
      return <div data-testid="recovered">Recovered</div>
    }

    const { rerender } = render(
      <PluginErrorBoundary pluginName="recovery-plugin">
        <ConditionalThrower />
      </PluginErrorBoundary>
    )

    // Error state
    expect(screen.getByText(/Plugin Error/i)).toBeInTheDocument()

    // Simulate fix + click Retry
    shouldThrow = false
    screen.getByRole('button', { name: /retry/i }).click()

    rerender(
      <PluginErrorBoundary pluginName="recovery-plugin">
        <ConditionalThrower />
      </PluginErrorBoundary>
    )

    expect(screen.getByTestId('recovered')).toBeInTheDocument()
  })

  it('displays generic message when error has no message', () => {
    const ThrowingNoMsg = () => {
      const err = new Error('')
      err.message = ''
      throw err
    }

    render(
      <PluginErrorBoundary pluginName="silent-plugin">
        <ThrowingNoMsg />
      </PluginErrorBoundary>
    )

    expect(screen.getByText(/An unexpected error occurred/i)).toBeInTheDocument()
  })
})

// --- PluginProvider / usePlugins Tests ---

describe('PluginProvider', () => {
  beforeEach(() => {
    vi.spyOn(pluginLoader, 'fetchPluginManifests')
  })

  it('provides loading state initially', async () => {
    // fetchPluginManifests never resolves in this test (stays loading)
    vi.mocked(pluginLoader.fetchPluginManifests).mockReturnValue(new Promise(() => {}))

    const StatusComponent = () => {
      const { isLoading } = usePlugins()
      return <div data-testid="status">{isLoading ? 'loading' : 'done'}</div>
    }

    render(
      <PluginProvider>
        <StatusComponent />
      </PluginProvider>
    )

    expect(screen.getByTestId('status')).toHaveTextContent('loading')
  })

  it('provides plugins after fetch resolves', async () => {
    const mockPlugins: PluginManifestWithName[] = [
      {
        pluginName: 'cost-analyzer',
        frontend: {
          remoteEntry: '/plugins/cost-analyzer/remoteEntry.js',
          routes: [{ path: '/cost', module: './CostDashboard' }],
        },
      },
    ]
    vi.mocked(pluginLoader.fetchPluginManifests).mockResolvedValue(mockPlugins)

    const PluginList = () => {
      const { plugins, isLoading } = usePlugins()
      if (isLoading) return <div>Loading…</div>
      return (
        <ul>
          {plugins.map((p) => (
            <li key={p.pluginName} data-testid="plugin-item">
              {p.pluginName}
            </li>
          ))}
        </ul>
      )
    }

    render(
      <PluginProvider>
        <PluginList />
      </PluginProvider>
    )

    await waitFor(() => screen.getByTestId('plugin-item'))
    expect(screen.getByText('cost-analyzer')).toBeInTheDocument()
  })

  it('shows no plugins when fetch returns empty array', async () => {
    vi.mocked(pluginLoader.fetchPluginManifests).mockResolvedValue([])

    const PluginCount = () => {
      const { plugins, isLoading } = usePlugins()
      if (isLoading) return <div>Loading…</div>
      return <div data-testid="count">{plugins.length}</div>
    }

    render(
      <PluginProvider>
        <PluginCount />
      </PluginProvider>
    )

    await waitFor(() => screen.getByTestId('count'))
    expect(screen.getByTestId('count')).toHaveTextContent('0')
  })

  it('transitions isLoading to false after error', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.mocked(pluginLoader.fetchPluginManifests).mockRejectedValue(new Error('fetch failed'))

    const StatusComponent = () => {
      const { isLoading, plugins } = usePlugins()
      return (
        <div>
          <span data-testid="loading">{String(isLoading)}</span>
          <span data-testid="count">{plugins.length}</span>
        </div>
      )
    }

    render(
      <PluginProvider>
        <StatusComponent />
      </PluginProvider>
    )

    await waitFor(() =>
      expect(screen.getByTestId('loading')).toHaveTextContent('false')
    )
    expect(screen.getByTestId('count')).toHaveTextContent('0')
  })
})

// --- PluginPage: not-found state ---

describe('PluginPage — not found', () => {
  it('renders PluginNotFound when plugin is not in context', async () => {
    vi.mocked(pluginLoader.fetchPluginManifests).mockResolvedValue([])

    // Dynamically import PluginPage to avoid hoisting issues with vi.mock
    const { PluginPage } = await import('../plugin-page')

    render(
      <PluginProvider>
        <MemoryRouter initialEntries={['/plugin/missing-plugin/']}>
          <Routes>
            <Route path="/plugin/:pluginName/*" element={<PluginPage />} />
          </Routes>
        </MemoryRouter>
      </PluginProvider>
    )

    await waitFor(() => screen.getByText(/Plugin Not Found/i))
    expect(screen.getByText(/missing-plugin/i)).toBeInTheDocument()
  })

  it('renders PluginNotFound when pluginName param is missing', async () => {
    vi.mocked(pluginLoader.fetchPluginManifests).mockResolvedValue([])

    const { PluginPage } = await import('../plugin-page')

    render(
      <PluginProvider>
        <MemoryRouter initialEntries={['/plugin/']}>
          <Routes>
            <Route path="/plugin/*" element={<PluginPage />} />
          </Routes>
        </MemoryRouter>
      </PluginProvider>
    )

    await waitFor(() => screen.getByText(/Plugin Not Found/i))
  })
})
