import { expect, test } from '@playwright/test'

/**
 * Plugin system E2E tests.
 *
 * These tests verify the plugin API surface and frontend behaviour.
 * They do NOT require compiled plugin binaries to pass — the assertions
 * are designed to work whether zero or more plugins are loaded at runtime.
 */

test.describe('plugin system — API', () => {
  test('GET /api/v1/plugins/manifests returns JSON array', async ({
    request,
  }) => {
    const response = await request.get('/api/v1/plugins/manifests')

    expect(response.status()).toBe(200)

    const body = await response.json()
    expect(Array.isArray(body)).toBe(true)
  })

  test('POST /api/v1/plugins/tools/:toolName returns 400 for unknown tool format', async ({
    request,
  }) => {
    // A tool name with no "." separator is always invalid
    const response = await request.post('/api/v1/plugins/tools/not-a-valid-tool', {
      data: { arguments: {} },
      headers: { 'Content-Type': 'application/json' },
    })

    // 400 (bad request) or 404 (unknown plugin)
    expect([400, 404]).toContain(response.status())
  })

  test('GET /api/v1/plugins/:pluginName/* returns 404 for non-existent plugin', async ({
    request,
  }) => {
    const response = await request.get('/api/v1/plugins/nonexistent-plugin/api/data')

    expect(response.status()).toBe(404)
  })
})

test.describe('plugin system — frontend', () => {
  test('navigating to /plugin/unknown shows Plugin Not Found', async ({
    page,
  }) => {
    await page.goto('/plugin/unknown-plugin/')

    await expect(
      page.getByRole('heading', { name: 'Plugin Not Found' })
    ).toBeVisible()

    await expect(page.getByText('unknown-plugin')).toBeVisible()
  })

  test('main app still loads without any plugins', async ({ page }) => {
    await page.goto('/')

    // The overview heading should always be present
    await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  })

  test('sidebar renders without crashing when no plugins are loaded', async ({
    page,
  }) => {
    await page.goto('/')

    // The sidebar nav group labels should be present regardless of plugin state
    await expect(page.getByRole('navigation')).toBeVisible()
  })
})
