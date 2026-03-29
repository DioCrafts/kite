import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import type { ComponentType } from 'react'

import { PluginSlot } from '../plugin-slot'
import {
  registerSlotComponent,
  resetRegistry,
  type SlotComponentProps,
} from '../../lib/plugin-registry'

// Silence expected React error-boundary console.error calls
beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {})
  resetRegistry()
})

// ── Utility components ────────────────────────────────────────────────────────

const TextWidget: ComponentType<SlotComponentProps> = ({ resource }) => (
  <div data-testid="text-widget">Widget: {JSON.stringify(resource)}</div>
)

const AnotherWidget: ComponentType<SlotComponentProps> = () => (
  <div data-testid="another-widget">Another</div>
)

const CrashingWidget: ComponentType<SlotComponentProps> = () => {
  throw new Error('Plugin crash!')
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('PluginSlot', () => {
  it('renders nothing when no components are registered for the slot', () => {
    const { container } = render(
      <PluginSlot slot="empty-slot" resource={{}} cluster="default" />
    )
    expect(container.firstChild).toBeNull()
  })

  it('renders a registered component for the slot', () => {
    registerSlotComponent('pod-detail', 'test-plugin', TextWidget)

    render(
      <PluginSlot slot="pod-detail" resource={{ name: 'my-pod' }} cluster="prod" />
    )

    expect(screen.getByTestId('text-widget')).toBeInTheDocument()
    expect(screen.getByTestId('text-widget')).toHaveTextContent('my-pod')
  })

  it('passes cluster and namespace to each component', () => {
    const PropsCapture: ComponentType<SlotComponentProps> = ({ cluster, namespace }) => (
      <div data-testid="props-capture">
        {cluster}:{namespace ?? 'none'}
      </div>
    )

    registerSlotComponent('test-slot', 'props-plugin', PropsCapture)

    render(
      <PluginSlot
        slot="test-slot"
        resource={{}}
        cluster="my-cluster"
        namespace="kube-system"
      />
    )

    expect(screen.getByTestId('props-capture')).toHaveTextContent(
      'my-cluster:kube-system'
    )
  })

  it('renders multiple registered components', () => {
    registerSlotComponent('multi-slot', 'plugin-1', TextWidget, 10)
    registerSlotComponent('multi-slot', 'plugin-2', AnotherWidget, 20)

    render(<PluginSlot slot="multi-slot" resource={{}} cluster="c" />)

    expect(screen.getByTestId('text-widget')).toBeInTheDocument()
    expect(screen.getByTestId('another-widget')).toBeInTheDocument()
  })

  it('does not render components registered for a different slot', () => {
    registerSlotComponent('other-slot', 'other-plugin', TextWidget)

    const { container } = render(
      <PluginSlot slot="pod-detail" resource={{}} cluster="c" />
    )
    expect(container.firstChild).toBeNull()
  })

  it('wraps each component in PluginErrorBoundary so one crash does not block others', () => {
    registerSlotComponent('crash-slot', 'crashing-plugin', CrashingWidget, 10)
    registerSlotComponent('crash-slot', 'healthy-plugin', AnotherWidget, 20)

    render(<PluginSlot slot="crash-slot" resource={{}} cluster="c" />)

    // The healthy plugin should still render
    expect(screen.getByTestId('another-widget')).toBeInTheDocument()
    // The crashing plugin should show an error boundary fallback
    expect(screen.getByText(/Plugin Error/i)).toBeInTheDocument()
  })

  it('reactively re-renders when a new component is registered after mount', async () => {
    const { queryByTestId } = render(
      <PluginSlot slot="dynamic-slot" resource={{}} cluster="c" />
    )
    expect(queryByTestId('text-widget')).toBeNull()

    // Register after mount
    act(() => {
      registerSlotComponent('dynamic-slot', 'late-plugin', TextWidget)
    })

    expect(screen.getByTestId('text-widget')).toBeInTheDocument()
  })
})
