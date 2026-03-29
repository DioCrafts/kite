import { describe, it, expect, beforeEach, vi } from 'vitest'
import type { ComponentType } from 'react'

import {
  registerSlotComponent,
  getSlotComponents,
  registerTableColumns,
  getTableColumns,
  subscribeToRegistry,
  unregisterPlugin,
  resetRegistry,
  type SlotComponentProps,
  type PluginColumn,
} from '../../lib/plugin-registry'

const noop = () => {}

const FakeComponent: ComponentType<SlotComponentProps> = () => null
const OtherComponent: ComponentType<SlotComponentProps> = () => null

beforeEach(() => {
  resetRegistry()
})

// ── registerSlotComponent / getSlotComponents ────────────────────────────────

describe('registerSlotComponent', () => {
  it('registers a component and returns it via getSlotComponents', () => {
    registerSlotComponent('pod-detail', 'my-plugin', FakeComponent)
    const slots = getSlotComponents('pod-detail')
    expect(slots).toHaveLength(1)
    expect(slots[0].pluginName).toBe('my-plugin')
    expect(slots[0].component).toBe(FakeComponent)
  })

  it('returns [] for an unknown slot', () => {
    expect(getSlotComponents('unknown-slot')).toEqual([])
  })

  it('sorts components by ascending priority', () => {
    registerSlotComponent('test-slot', 'plugin-b', OtherComponent, 200)
    registerSlotComponent('test-slot', 'plugin-a', FakeComponent, 50)

    const slots = getSlotComponents('test-slot')
    expect(slots[0].pluginName).toBe('plugin-a') // priority 50 first
    expect(slots[1].pluginName).toBe('plugin-b') // priority 200 second
  })

  it('defaults priority to 100', () => {
    registerSlotComponent('test-slot', 'default-prio-plugin', FakeComponent)
    expect(getSlotComponents('test-slot')[0].priority).toBe(100)
  })

  it('does not double-register the same plugin in the same slot (HMR guard)', () => {
    registerSlotComponent('test-slot', 'my-plugin', FakeComponent)
    registerSlotComponent('test-slot', 'my-plugin', FakeComponent) // duplicate
    expect(getSlotComponents('test-slot')).toHaveLength(1)
  })

  it('allows the same plugin to register in different slots', () => {
    registerSlotComponent('slot-1', 'shared-plugin', FakeComponent)
    registerSlotComponent('slot-2', 'shared-plugin', OtherComponent)
    expect(getSlotComponents('slot-1')).toHaveLength(1)
    expect(getSlotComponents('slot-2')).toHaveLength(1)
  })
})

// ── registerTableColumns / getTableColumns ────────────────────────────────────

describe('registerTableColumns', () => {
  const cols: PluginColumn[] = [
    { id: 'col-a', header: 'Column A', cell: () => null },
    { id: 'col-b', header: 'Column B', cell: () => null },
  ]

  it('registers columns and returns them via getTableColumns', () => {
    registerTableColumns('pods-table', 'col-plugin', cols)
    expect(getTableColumns('pods-table')).toHaveLength(2)
  })

  it('returns [] for an unknown table slot', () => {
    expect(getTableColumns('no-such-table')).toEqual([])
  })

  it('merges columns from multiple plugins in priority order', () => {
    const colsA: PluginColumn[] = [{ id: 'a', header: 'A', cell: () => null }]
    const colsB: PluginColumn[] = [{ id: 'b', header: 'B', cell: () => null }]

    registerTableColumns('table-slot', 'plugin-b', colsB, 200)
    registerTableColumns('table-slot', 'plugin-a', colsA, 50)

    const result = getTableColumns('table-slot')
    expect(result[0].id).toBe('a') // plugin-a has lower priority → comes first
    expect(result[1].id).toBe('b')
  })

  it('prevents double-registration for the same plugin+slot', () => {
    registerTableColumns('table-slot', 'dup-plugin', cols)
    registerTableColumns('table-slot', 'dup-plugin', cols)
    expect(getTableColumns('table-slot')).toHaveLength(2) // still just the original 2 columns
  })
})

// ── subscribeToRegistry ───────────────────────────────────────────────────────

describe('subscribeToRegistry', () => {
  it('notifies subscriber when a component is registered', () => {
    const listener = vi.fn()
    const unsub = subscribeToRegistry(listener)

    registerSlotComponent('test-slot', 'notify-plugin', FakeComponent)
    expect(listener).toHaveBeenCalledTimes(1)

    unsub()
  })

  it('stops notifying after unsubscribing', () => {
    const listener = vi.fn()
    const unsub = subscribeToRegistry(listener)
    unsub()

    registerSlotComponent('test-slot', 'after-unsub-plugin', FakeComponent)
    expect(listener).not.toHaveBeenCalled()
  })

  it('notifies multiple subscribers', () => {
    const a = vi.fn()
    const b = vi.fn()
    subscribeToRegistry(a)
    subscribeToRegistry(b)

    registerSlotComponent('test-slot', 'multi-sub-plugin', FakeComponent)
    expect(a).toHaveBeenCalled()
    expect(b).toHaveBeenCalled()
  })
})

// ── unregisterPlugin ──────────────────────────────────────────────────────────

describe('unregisterPlugin', () => {
  it('removes a plugin from all component slots', () => {
    registerSlotComponent('slot-1', 'remove-me', FakeComponent)
    registerSlotComponent('slot-2', 'remove-me', FakeComponent)
    registerSlotComponent('slot-1', 'keep-me', OtherComponent)

    unregisterPlugin('remove-me')

    expect(getSlotComponents('slot-1')).toHaveLength(1)
    expect(getSlotComponents('slot-1')[0].pluginName).toBe('keep-me')
    expect(getSlotComponents('slot-2')).toHaveLength(0)
  })

  it('removes a plugin from all table-column slots', () => {
    const cols: PluginColumn[] = [{ id: 'x', header: 'X', cell: () => null }]
    registerTableColumns('table-slot', 'remove-col-plugin', cols)
    registerTableColumns('table-slot', 'keep-col-plugin', cols)

    unregisterPlugin('remove-col-plugin')

    const remaining = getTableColumns('table-slot')
    remaining.forEach((c) => expect(c.id).toBe('x')) // only keep-col-plugin's col
    expect(remaining).toHaveLength(1)
  })

  it('notifies subscribers when a plugin is unregistered', () => {
    const listener = vi.fn()
    subscribeToRegistry(listener)

    registerSlotComponent('slot', 'unregister-notify-plugin', FakeComponent)
    listener.mockClear()

    unregisterPlugin('unregister-notify-plugin')
    expect(listener).toHaveBeenCalledTimes(1)
  })

  it('is a no-op for unknown plugins', () => {
    // Should not throw
    expect(() => unregisterPlugin('ghost-plugin')).not.toThrow()
  })
})

// ── resetRegistry ─────────────────────────────────────────────────────────────

describe('resetRegistry', () => {
  it('clears all component and column registrations', () => {
    registerSlotComponent('test-slot', 'a', FakeComponent)
    registerTableColumns('table-slot', 'b', [{ id: 'x', header: 'X', cell: noop }])

    resetRegistry()

    expect(getSlotComponents('test-slot')).toEqual([])
    expect(getTableColumns('table-slot')).toEqual([])
  })

  it('notifies existing subscribers on reset', () => {
    const listener = vi.fn()
    subscribeToRegistry(listener)

    registerSlotComponent('slot', 'reset-plugin', FakeComponent)
    listener.mockClear()

    resetRegistry()
    expect(listener).toHaveBeenCalledTimes(1)
  })
})

// ── cross-slot isolation ──────────────────────────────────────────────────────

describe('slot isolation', () => {
  it('registrations in one slot do not affect other slots', () => {
    registerSlotComponent('slot-a', 'isolated-plugin', FakeComponent)
    expect(getSlotComponents('slot-b')).toEqual([])
  })
})
