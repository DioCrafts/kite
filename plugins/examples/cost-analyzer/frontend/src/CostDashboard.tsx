import { useState, useEffect } from 'react'
import {
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer,
  PieChart, Pie, Cell,
} from 'recharts'

interface NamespaceCost {
  namespace: string
  cpuCores: number
  memoryGB: number
  cpuCostPerHour: number
  memoryCostPerHour: number
  totalCostPerHour: number
  podCount: number
}

interface CostResponse {
  costs: NamespaceCost[]
  settings: { cpuPricePerHour: number; memoryPricePerGBHour: number }
}

const COLORS = ['#2563eb', '#7c3aed', '#db2777', '#ea580c', '#65a30d', '#0891b2']

export default function CostDashboard() {
  const [data, setData] = useState<CostResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    fetch('/api/v1/plugins/cost-analyzer/costs', { credentials: 'include' })
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then(setData)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div style={{ padding: 24 }}>
        <h1 style={{ fontSize: 24, fontWeight: 700, marginBottom: 8 }}>Cost Analysis</h1>
        <p style={{ color: '#6b7280' }}>Loading cost data...</p>
      </div>
    )
  }

  if (error || !data) {
    return (
      <div style={{ padding: 24 }}>
        <h1 style={{ fontSize: 24, fontWeight: 700, marginBottom: 8 }}>Cost Analysis</h1>
        <p style={{ color: '#ef4444' }}>Failed to load cost data: {error}</p>
      </div>
    )
  }

  const { costs, settings } = data
  const totalCost = costs.reduce((sum, c) => sum + c.totalCostPerHour, 0)
  const totalPods = costs.reduce((sum, c) => sum + c.podCount, 0)
  const totalCPU = costs.reduce((sum, c) => sum + c.cpuCores, 0)
  const totalMemory = costs.reduce((sum, c) => sum + c.memoryGB, 0)

  const barData = costs.map((c) => ({
    name: c.namespace,
    'CPU Cost': Number(c.cpuCostPerHour.toFixed(4)),
    'Memory Cost': Number(c.memoryCostPerHour.toFixed(4)),
  }))

  const pieData = costs.map((c) => ({
    name: c.namespace,
    value: Number(c.totalCostPerHour.toFixed(4)),
  }))

  return (
    <div style={{ padding: 24, display: 'flex', flexDirection: 'column', gap: 24 }}>
      <div>
        <h1 style={{ fontSize: 24, fontWeight: 700, marginBottom: 4 }}>Cost Analysis</h1>
        <p style={{ color: '#6b7280', fontSize: 14 }}>
          Estimated hourly costs based on CPU (${settings.cpuPricePerHour}/core/hr)
          and memory (${settings.memoryPricePerGBHour}/GB/hr)
        </p>
      </div>

      {/* Summary Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16 }}>
        <SummaryCard label="Total Cost/Hour" value={`$${totalCost.toFixed(4)}`} />
        <SummaryCard label="Est. Monthly" value={`$${(totalCost * 730).toFixed(2)}`} />
        <SummaryCard label="Total Pods" value={String(totalPods)} />
        <SummaryCard label="Resources" value={`${totalCPU.toFixed(1)} CPU / ${totalMemory.toFixed(1)} GB`} />
      </div>

      {/* Charts */}
      <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: 24 }}>
        <div style={{ background: '#fff', borderRadius: 8, padding: 16, border: '1px solid #e5e7eb' }}>
          <h2 style={{ fontSize: 16, fontWeight: 600, marginBottom: 16 }}>Cost Breakdown by Namespace</h2>
          <ResponsiveContainer width="100%" height={300}>
            <BarChart data={barData}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="name" />
              <YAxis tickFormatter={(v: number) => `$${v}`} />
              <Tooltip formatter={(v: number) => `$${v.toFixed(4)}/hr`} />
              <Legend />
              <Bar dataKey="CPU Cost" fill="#2563eb" />
              <Bar dataKey="Memory Cost" fill="#7c3aed" />
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div style={{ background: '#fff', borderRadius: 8, padding: 16, border: '1px solid #e5e7eb' }}>
          <h2 style={{ fontSize: 16, fontWeight: 600, marginBottom: 16 }}>Cost Distribution</h2>
          <ResponsiveContainer width="100%" height={300}>
            <PieChart>
              <Pie data={pieData} dataKey="value" nameKey="name" cx="50%" cy="50%" outerRadius={100} label>
                {pieData.map((_, i) => (
                  <Cell key={i} fill={COLORS[i % COLORS.length]} />
                ))}
              </Pie>
              <Tooltip formatter={(v: number) => `$${v.toFixed(4)}/hr`} />
            </PieChart>
          </ResponsiveContainer>
        </div>
      </div>

      {/* Namespace Table */}
      <div style={{ background: '#fff', borderRadius: 8, border: '1px solid #e5e7eb', overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
          <thead>
            <tr style={{ background: '#f9fafb', borderBottom: '1px solid #e5e7eb' }}>
              <th style={thStyle}>Namespace</th>
              <th style={thStyle}>Pods</th>
              <th style={thStyle}>CPU (cores)</th>
              <th style={thStyle}>Memory (GB)</th>
              <th style={thStyle}>CPU Cost/hr</th>
              <th style={thStyle}>Memory Cost/hr</th>
              <th style={thStyle}>Total/hr</th>
              <th style={thStyle}>Est. Monthly</th>
            </tr>
          </thead>
          <tbody>
            {costs.map((c) => (
              <tr key={c.namespace} style={{ borderBottom: '1px solid #e5e7eb' }}>
                <td style={tdStyle}><strong>{c.namespace}</strong></td>
                <td style={tdStyleNum}>{c.podCount}</td>
                <td style={tdStyleNum}>{c.cpuCores.toFixed(1)}</td>
                <td style={tdStyleNum}>{c.memoryGB.toFixed(1)}</td>
                <td style={tdStyleNum}>${c.cpuCostPerHour.toFixed(4)}</td>
                <td style={tdStyleNum}>${c.memoryCostPerHour.toFixed(4)}</td>
                <td style={tdStyleNum}><strong>${c.totalCostPerHour.toFixed(4)}</strong></td>
                <td style={tdStyleNum}>${(c.totalCostPerHour * 730).toFixed(2)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ background: '#fff', borderRadius: 8, padding: 16, border: '1px solid #e5e7eb' }}>
      <p style={{ fontSize: 12, color: '#6b7280', marginBottom: 4 }}>{label}</p>
      <p style={{ fontSize: 20, fontWeight: 700 }}>{value}</p>
    </div>
  )
}

const thStyle: React.CSSProperties = { padding: '10px 12px', textAlign: 'left', fontWeight: 600 }
const tdStyle: React.CSSProperties = { padding: '10px 12px' }
const tdStyleNum: React.CSSProperties = { padding: '10px 12px', textAlign: 'right', fontVariantNumeric: 'tabular-nums' }
