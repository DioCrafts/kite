import { useState } from 'react'

interface BackupSettingsProps {
  pluginConfig: Record<string, string>
  onSave: (config: Record<string, string>) => void
}

export default function BackupSettings({ pluginConfig, onSave }: BackupSettingsProps) {
  const [retentionDays, setRetentionDays] = useState(pluginConfig.retentionDays ?? '30')
  const [maxBackups, setMaxBackups] = useState(pluginConfig.maxBackups ?? '50')
  const [defaultNamespace, setDefaultNamespace] = useState(pluginConfig.defaultNamespace ?? '')
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  const handleSave = () => {
    setSaving(true)
    setSaved(false)

    fetch('/api/v1/plugins/backup-manager/settings', {
      method: 'PUT',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        retentionDays: parseInt(retentionDays, 10),
        maxBackups: parseInt(maxBackups, 10),
        defaultNamespace,
      }),
    })
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        onSave({ retentionDays, maxBackups, defaultNamespace })
        setSaved(true)
      })
      .catch(() => alert('Failed to save settings'))
      .finally(() => setSaving(false))
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <h3 style={{ fontSize: 16, fontWeight: 600, margin: 0 }}>Backup Manager Settings</h3>
      <p style={{ color: '#6b7280', fontSize: 14, margin: 0 }}>
        Configure backup retention, limits, and defaults.
      </p>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
        <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <span style={{ fontSize: 14, fontWeight: 500 }}>Retention Period (days)</span>
          <input
            type="number"
            min="1"
            value={retentionDays}
            onChange={(e) => setRetentionDays(e.target.value)}
            style={inputStyle}
          />
          <span style={{ fontSize: 12, color: '#9ca3af' }}>
            Backups older than this will be automatically cleaned up.
          </span>
        </label>

        <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <span style={{ fontSize: 14, fontWeight: 500 }}>Maximum Backups</span>
          <input
            type="number"
            min="1"
            value={maxBackups}
            onChange={(e) => setMaxBackups(e.target.value)}
            style={inputStyle}
          />
          <span style={{ fontSize: 12, color: '#9ca3af' }}>
            Maximum number of backups to store. New backups will be rejected when limit is reached.
          </span>
        </label>

        <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <span style={{ fontSize: 14, fontWeight: 500 }}>Default Namespace</span>
          <input
            type="text"
            value={defaultNamespace}
            onChange={(e) => setDefaultNamespace(e.target.value)}
            placeholder="Leave empty for no default"
            style={inputStyle}
          />
          <span style={{ fontSize: 12, color: '#9ca3af' }}>
            Pre-fill namespace when creating backups from the UI.
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
