import { useMemo } from 'react'
import { apiClient } from '../../src/lib/api-client'

/**
 * Hook to access a plugin-scoped API client. Requests are automatically
 * prefixed with `/plugins/<pluginName>/` (relative to the API base).
 *
 * @example
 * ```tsx
 * const api = usePluginApi('cost-analyzer')
 * // GET /api/v1/plugins/cost-analyzer/summary
 * const data = await api.get('/summary')
 * ```
 */
export function usePluginApi(pluginName: string) {
  return useMemo(() => {
    const prefix = `/plugins/${encodeURIComponent(pluginName)}`
    return {
      get: <T>(url: string, opts?: RequestInit) =>
        apiClient.get<T>(prefix + url, opts),
      post: <T>(url: string, data?: unknown, opts?: RequestInit) =>
        apiClient.post<T>(prefix + url, data, opts),
      put: <T>(url: string, data?: unknown, opts?: RequestInit) =>
        apiClient.put<T>(prefix + url, data, opts),
      patch: <T>(url: string, data?: unknown, opts?: RequestInit) =>
        apiClient.patch<T>(prefix + url, data, opts),
      delete: <T>(url: string, opts?: RequestInit) =>
        apiClient.delete<T>(prefix + url, opts),
    }
  }, [pluginName])
}
