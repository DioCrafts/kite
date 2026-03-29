import { apiClient } from '../../src/lib/api-client'

/**
 * Hook to access the authenticated Kite API client from a plugin component.
 * The returned client automatically includes cluster headers and auth tokens.
 *
 * @example
 * ```tsx
 * const api = useKiteApi()
 * const pods = await api.get('/pods')
 * ```
 */
export function useKiteApi() {
  return apiClient
}
