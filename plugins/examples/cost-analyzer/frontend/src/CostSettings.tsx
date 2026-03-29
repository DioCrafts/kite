import { useState } from 'react'

interface CostSettings {
  cpuPricePerHour: number
  memoryPricePerGBHour: number
}

interface CostSettingsProps {
  pluginConfig: Record<string, string>
  onSave: (config: Record<string, string>) => void
}

export default function CostSettings({ pluginConfig, onSave }: CostSettingsProps) {
  const [cpuPrice, setCpuPrice] = useState(pluginConfig.cpuPricePerHour ?? '0.05')
  const [memPrice, setMemPrice] = useState(pluginConfig.memoryPricePerGBHour ?? '0.01')
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  const handleSave = () => {
    setSaving(true)
    setSaved(false)

    fetch('/api/v1/plugins/cost-analyzer/settings', {
      method: 'PUT',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        cpuPricePerHour: parseFloat(cpuPrice),
        memoryPricePerGBHour: parseFloat(memPrice),
      } satisfies CostSettings),
    })
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        onSave({ cpuPricePerHour: cpuPrice, memoryPricePerGBHour: memPrice })
        setSaved(true)
      })
      .catch(() => alert('Failed to save settings'))
      .finally(() => setSaving(false))
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <h3 style={{ fontSize: 16, fontWeight: 600, margin: 0 }}>Cost Analyzer Settings</h3>
      <p style={{ color: '#6b7280', fontSize: 14, margin: 0 }}>
        Configure pricing rates for cost estimation. Costs are calculated as resource usage multiplied
        by the hourly rate.
      </p>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <span style={{ fontSize: 14, fontWeight: 500 }}>CPU Price ($/core/hour)</span>
          <input
            type="number"
            step="0.001"
            min="0"
            value={cpuPrice}
            onChange={(e) => setCpuPrice(e.target.value)}
            style={inputStyle}
          />
          <span style={{ fontSize: 12, color: '#9ca3af' }}>
            Default: $0.05 per core per hour (approx. on-demand pricing)
          </span>
        </label>

        <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <span style={{ fontSize: 14, fontWeight: 500 }}>Memory Price ($/GB/hour)</span>
          <input
            type="number"
            step="0.001"
            min="0"
            value={memPrice}
            onChange={(e) => setMemPrice(e.target.value)}
            style={inputStyle}
          />
          <span style={{ fontSize: 12, color: '#9ca3af' }}>
            Default: $0.01 per GB per hour (approx. on-demand pricing)
          </span>
        </label>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <button onClick={handleSave} disabled={saving} style={buttonStyle}>
          {saving ? 'Saving...' : 'Save Settings'}
        </button>
        {saved && <span style={{ color: '#16a34a', fontSize: 14 }}>Settings saved!</span>}
      </div>
    </div>
  )
}

const inputStyle: React.CSSProperties = {
  padding: '8px 12px',
  border: '1px solid #d1d5db',
  borderRadius: 6,
  fontSize: 14,
  width: 200,
}

const buttonStyle: React.CSSProperties = {
  padding: '8px 16px',
  background: '#2563eb',
  color: '#fff',
  border: 'none',
  borderRadius: 6,
  fontSize: 14,
  fontWeight: 500,
  cursor: 'pointer',
}
