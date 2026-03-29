import { useState, useEffect, useCallback } from 'react'

interface Backup {
  id: number
  name: string
  namespace: string
  status: string
  createdAt: string
  sizeBytes: number
  resourceCount: number
}

export default function BackupList() {
  const [backups, setBackups] = useState<Backup[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [createNs, setCreateNs] = useState('')
  const [createName, setCreateName] = useState('')
  const [creating, setCreating] = useState(false)
  const [filterNs, setFilterNs] = useState('')

  const fetchBackups = useCallback(() => {
    const url = filterNs
      ? `/api/v1/plugins/backup-manager/backups?namespace=${encodeURIComponent(filterNs)}`
      : '/api/v1/plugins/backup-manager/backups'

    fetch(url, { credentials: 'include' })
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then((data) => {
        setBackups(data.backups ?? [])
        setError(null)
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [filterNs])

  useEffect(() => {
    fetchBackups()
  }, [fetchBackups])

  const handleCreate = () => {
    if (!createNs) return
    setCreating(true)
    fetch('/api/v1/plugins/backup-manager/backups', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ namespace: createNs, name: createName || undefined }),
    })
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        setShowCreate(false)
        setCreateNs('')
        setCreateName('')
        fetchBackups()
      })
      .catch(() => alert('Failed to create backup'))
      .finally(() => setCreating(false))
  }

  const handleDelete = (id: number) => {
    if (!confirm('Delete this backup?')) return
    fetch(`/api/v1/plugins/backup-manager/backups/${id}`, {
      method: 'DELETE',
      credentials: 'include',
    })
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        fetchBackups()
      })
      .catch(() => alert('Failed to delete backup'))
  }

  const handleRestore = (id: number) => {
    if (!confirm('Restore this backup? This will re-apply its resources.')) return
    fetch(`/api/v1/plugins/backup-manager/backups/${id}/restore`, {
      method: 'POST',
      credentials: 'include',
    })
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then((data) => alert(`Restored: ${data.resources} resources to namespace ${data.namespace}`))
      .catch(() => alert('Failed to restore backup'))
  }

  const formatSize = (bytes: number) => {
    if (bytes >= 1073741824) return `${(bytes / 1073741824).toFixed(1)} GB`
    if (bytes >= 1048576) return `${(bytes / 1048576).toFixed(1)} MB`
    return `${(bytes / 1024).toFixed(1)} KB`
  }

  const formatDate = (iso: string) => {
    const d = new Date(iso)
    return d.toLocaleString()
  }

  if (loading) {
    return (
      <div style={{ padding: 24 }}>
        <h1 style={{ fontSize: 24, fontWeight: 700 }}>Backups</h1>
        <p style={{ color: '#6b7280' }}>Loading backups...</p>
      </div>
    )
  }

  return (
    <div style={{ padding: 24, display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h1 style={{ fontSize: 24, fontWeight: 700, marginBottom: 4 }}>Backups</h1>
          <p style={{ color: '#6b7280', fontSize: 14, margin: 0 }}>Manage Kubernetes namespace backups</p>
        </div>
        <button onClick={() => setShowCreate(true)} style={primaryButton}>
          + Create Backup
        </button>
      </div>

      {/* Filter */}
      <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
        <input
          placeholder="Filter by namespace..."
          value={filterNs}
          onChange={(e) => setFilterNs(e.target.value)}
          style={inputStyle}
        />
      </div>

      {error && <p style={{ color: '#ef4444' }}>Error: {error}</p>}

      {/* Create Dialog */}
      {showCreate && (
        <div style={{ background: '#f9fafb', borderRadius: 8, padding: 16, border: '1px solid #e5e7eb' }}>
          <h3 style={{ fontSize: 16, fontWeight: 600, marginBottom: 12, marginTop: 0 }}>Create Backup</h3>
          <div style={{ display: 'flex', gap: 8, alignItems: 'flex-end' }}>
            <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <span style={{ fontSize: 13, fontWeight: 500 }}>Namespace *</span>
              <input value={createNs} onChange={(e) => setCreateNs(e.target.value)} style={inputStyle} placeholder="e.g. production" />
            </label>
            <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <span style={{ fontSize: 13, fontWeight: 500 }}>Name (optional)</span>
              <input value={createName} onChange={(e) => setCreateName(e.target.value)} style={inputStyle} placeholder="auto-generated if empty" />
            </label>
            <button onClick={handleCreate} disabled={creating || !createNs} style={primaryButton}>
              {creating ? 'Creating...' : 'Create'}
            </button>
            <button onClick={() => setShowCreate(false)} style={secondaryButton}>Cancel</button>
          </div>
        </div>
      )}

      {/* Table */}
      <div style={{ background: '#fff', borderRadius: 8, border: '1px solid #e5e7eb', overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
          <thead>
            <tr style={{ background: '#f9fafb', borderBottom: '1px solid #e5e7eb' }}>
              <th style={thStyle}>Name</th>
              <th style={thStyle}>Namespace</th>
              <th style={thStyle}>Status</th>
              <th style={thStyle}>Created</th>
              <th style={thStyle}>Size</th>
              <th style={thStyle}>Resources</th>
              <th style={thStyle}>Actions</th>
            </tr>
          </thead>
          <tbody>
            {backups.length === 0 ? (
              <tr>
                <td colSpan={7} style={{ padding: 24, textAlign: 'center', color: '#9ca3af' }}>
                  No backups found
                </td>
              </tr>
            ) : (
              backups.map((b) => (
                <tr key={b.id} style={{ borderBottom: '1px solid #e5e7eb' }}>
                  <td style={tdStyle}><strong>{b.name}</strong></td>
                  <td style={tdStyle}>{b.namespace}</td>
                  <td style={tdStyle}>
                    <span style={{
                      ...statusBadge,
                      background: b.status === 'completed' ? '#dcfce7' : b.status === 'failed' ? '#fee2e2' : '#fef3c7',
                      color: b.status === 'completed' ? '#166534' : b.status === 'failed' ? '#991b1b' : '#92400e',
                    }}>
                      {b.status}
                    </span>
                  </td>
                  <td style={tdStyle}>{formatDate(b.createdAt)}</td>
                  <td style={tdStyleNum}>{formatSize(b.sizeBytes)}</td>
                  <td style={tdStyleNum}>{b.resourceCount}</td>
                  <td style={tdStyle}>
                    <div style={{ display: 'flex', gap: 4 }}>
                      <button onClick={() => handleRestore(b.id)} style={smallButton}>Restore</button>
                      <button onClick={() => handleDelete(b.id)} style={{ ...smallButton, color: '#dc2626' }}>Delete</button>
                    </div>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

const thStyle: React.CSSProperties = { padding: '10px 12px', textAlign: 'left', fontWeight: 600 }
const tdStyle: React.CSSProperties = { padding: '10px 12px' }
const tdStyleNum: React.CSSProperties = { padding: '10px 12px', textAlign: 'right', fontVariantNumeric: 'tabular-nums' }

const inputStyle: React.CSSProperties = {
  padding: '8px 12px', border: '1px solid #d1d5db', borderRadius: 6, fontSize: 14,
}

const primaryButton: React.CSSProperties = {
  padding: '8px 16px', background: '#2563eb', color: '#fff', border: 'none',
  borderRadius: 6, fontSize: 14, fontWeight: 500, cursor: 'pointer',
}

const secondaryButton: React.CSSProperties = {
  padding: '8px 16px', background: '#fff', color: '#374151', border: '1px solid #d1d5db',
  borderRadius: 6, fontSize: 14, fontWeight: 500, cursor: 'pointer',
}

const smallButton: React.CSSProperties = {
  padding: '4px 10px', background: 'transparent', color: '#2563eb', border: '1px solid #d1d5db',
  borderRadius: 4, fontSize: 13, cursor: 'pointer',
}

const statusBadge: React.CSSProperties = {
  display: 'inline-block', padding: '2px 8px', borderRadius: 12, fontSize: 12, fontWeight: 500,
}
