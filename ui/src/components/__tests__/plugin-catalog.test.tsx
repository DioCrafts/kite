import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

import { PluginCatalog } from '../settings/plugin-catalog'
import type { AdminPluginInfo } from '../../lib/api'

// ── Module mocks ──────────────────────────────────────────────────────────────

vi.mock('../../lib/api', async () => {
  const actual = await vi.importActual<typeof import('../../lib/api')>('../../lib/api')
  return {
    ...actual,
    useAdminPlugins: vi.fn(),
    installPlugin: vi.fn(),
    uninstallPlugin: vi.fn(),
    reloadPlugin: vi.fn(),
    setPluginEnabled: vi.fn(),
  }
})

// ── Helpers ───────────────────────────────────────────────────────────────────

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const { useAdminPlugins, installPlugin, uninstallPlugin, reloadPlugin, setPluginEnabled } =
  await import('../../lib/api')

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}

function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={makeQueryClient()}>
      {children}
    </QueryClientProvider>
  )
}

function renderCatalog() {
  return render(<PluginCatalog />, { wrapper })
}

const loadedPlugin: AdminPluginInfo = {
  name: 'cost-analyzer',
  version: '1.0.0',
  description: 'Shows cost data',
  author: 'Acme',
  state: 'loaded',
  priority: 100,
  permissions: [],
  settings: [],
}

const failedPlugin: AdminPluginInfo = {
  name: 'broken-plugin',
  version: '0.1.0',
  description: '',
  author: '',
  state: 'failed',
  error: 'binary not found',
  priority: 100,
  permissions: [],
  settings: [],
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('PluginCatalog', () => {
  beforeEach(() => {
    vi.mocked(useAdminPlugins).mockReturnValue({
      data: [],
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useAdminPlugins>)
  })

  it('shows loading state', () => {
    vi.mocked(useAdminPlugins).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
    } as ReturnType<typeof useAdminPlugins>)

    renderCatalog()
    expect(screen.getByText(/loading plugins/i)).toBeInTheDocument()
  })

  it('shows error state', () => {
    vi.mocked(useAdminPlugins).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('connection refused'),
    } as ReturnType<typeof useAdminPlugins>)

    renderCatalog()
    expect(screen.getByText(/connection refused/i)).toBeInTheDocument()
  })

  it('shows empty state when no plugins installed', () => {
    renderCatalog()
    expect(screen.getByText(/no plugins installed/i)).toBeInTheDocument()
  })

  it('renders a plugin card with name, version and state badge', () => {
    vi.mocked(useAdminPlugins).mockReturnValue({
      data: [loadedPlugin],
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useAdminPlugins>)

    renderCatalog()
    expect(screen.getByText('cost-analyzer')).toBeInTheDocument()
    expect(screen.getByText(/v1\.0\.0/)).toBeInTheDocument()
    expect(screen.getByText('Loaded')).toBeInTheDocument()
  })

  it('renders failed state badge with error message', () => {
    vi.mocked(useAdminPlugins).mockReturnValue({
      data: [failedPlugin],
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useAdminPlugins>)

    renderCatalog()
    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(screen.getByText('binary not found')).toBeInTheDocument()
  })

  it('opens install dialog when Install Plugin button is clicked', async () => {
    renderCatalog()
    await userEvent.click(screen.getByRole('button', { name: /install plugin/i }))
    // The dialog title appears when the dialog is open
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /install plugin/i })).toBeInTheDocument()
  })

  it('calls uninstallPlugin after confirming uninstall dialog', async () => {
    vi.mocked(uninstallPlugin).mockResolvedValue(undefined)
    vi.mocked(useAdminPlugins).mockReturnValue({
      data: [loadedPlugin],
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useAdminPlugins>)

    renderCatalog()

    // Click Uninstall button on the plugin card
    await userEvent.click(screen.getByRole('button', { name: /uninstall/i }))

    // The delete confirmation dialog appears — type the plugin name to confirm
    const input = await screen.findByRole('textbox')
    await userEvent.type(input, 'cost-analyzer')

    const confirmBtn = screen.getByRole('button', { name: /delete/i })
    await userEvent.click(confirmBtn)

    await waitFor(() =>
      expect(vi.mocked(uninstallPlugin)).toHaveBeenCalledWith('cost-analyzer')
    )
  })

  it('calls reloadPlugin when Reload button is clicked', async () => {
    vi.mocked(reloadPlugin).mockResolvedValue(undefined)
    vi.mocked(useAdminPlugins).mockReturnValue({
      data: [loadedPlugin],
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useAdminPlugins>)

    renderCatalog()

    await userEvent.click(screen.getByRole('button', { name: /reload/i }))

    await waitFor(() =>
      expect(vi.mocked(reloadPlugin)).toHaveBeenCalledWith('cost-analyzer')
    )
  })

  it('calls setPluginEnabled(false) when Disable is clicked', async () => {
    vi.mocked(setPluginEnabled).mockResolvedValue(undefined)
    vi.mocked(useAdminPlugins).mockReturnValue({
      data: [loadedPlugin],
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useAdminPlugins>)

    renderCatalog()

    await userEvent.click(screen.getByRole('button', { name: /disable/i }))

    await waitFor(() =>
      expect(vi.mocked(setPluginEnabled)).toHaveBeenCalledWith('cost-analyzer', false)
    )
  })

  it('calls setPluginEnabled(true) when Enable is clicked on a disabled plugin', async () => {
    vi.mocked(setPluginEnabled).mockResolvedValue(undefined)
    vi.mocked(useAdminPlugins).mockReturnValue({
      data: [{ ...loadedPlugin, state: 'disabled' as const }],
      isLoading: false,
      isError: false,
      error: null,
    } as ReturnType<typeof useAdminPlugins>)

    renderCatalog()

    await userEvent.click(screen.getByRole('button', { name: /enable/i }))

    await waitFor(() =>
      expect(vi.mocked(setPluginEnabled)).toHaveBeenCalledWith('cost-analyzer', true)
    )
  })
})
