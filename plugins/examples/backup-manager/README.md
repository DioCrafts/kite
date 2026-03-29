# Backup Manager Plugin

Example Kite plugin that demonstrates **resource handlers** (custom CRUD for a simulated CRD), **multiple AI tools**, and a full frontend with backup list and settings.

## Features

- Simulated `backups.kite.io/v1` CRD with full CRUD operations
- In-memory backup store with seed data
- 3 AI tools: `create_backup`, `list_backups`, `restore_backup`
- Frontend with backup list table, create dialog, and settings panel

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/backups` | List all backups (optional `?namespace=` filter) |
| GET | `/backups/:id` | Get backup by ID or name |
| POST | `/backups` | Create a new backup |
| DELETE | `/backups/:id` | Delete a backup |
| POST | `/backups/:id/restore` | Restore a backup |
| PUT | `/settings` | Update plugin settings |

## AI Tools

| Tool | Example Prompt |
|------|---------------|
| `create_backup` | "Create a backup of the staging namespace" |
| `list_backups` | "Show me the recent backups" |
| `restore_backup` | "Restore the backup backup-production-2025-01-15" |

## Resource Handler

Registers a custom `backups` resource handler implementing the full `ResourceHandler` interface, enabling Kite's built-in resource management to work with plugin-defined types.

## Settings

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `retentionDays` | number | 30 | Backup retention period in days |
| `maxBackups` | number | 50 | Maximum stored backups |
| `defaultNamespace` | text | (empty) | Default namespace for new backups |

## Build

```bash
make build
```
