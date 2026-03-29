import type { ReactNode } from 'react'

interface KitePluginPageProps {
  /** Page title displayed in the header area */
  title: string
  /** Optional description shown below the title */
  description?: string
  /** Page content */
  children: ReactNode
}

/**
 * Layout wrapper for plugin pages. Provides consistent styling
 * that matches Kite's native page layout.
 *
 * @example
 * ```tsx
 * export default function CostDashboard() {
 *   return (
 *     <KitePluginPage title="Cost Dashboard" description="Cluster cost overview">
 *       <CostTable />
 *     </KitePluginPage>
 *   )
 * }
 * ```
 */
export function KitePluginPage({ title, description, children }: KitePluginPageProps) {
  return (
    <div className="flex flex-1 flex-col gap-4 p-4 pt-0">
      <div className="flex flex-col gap-1">
        <h1 className="text-2xl font-bold tracking-tight">{title}</h1>
        {description && (
          <p className="text-muted-foreground">{description}</p>
        )}
      </div>
      <div className="flex-1">{children}</div>
    </div>
  )
}
