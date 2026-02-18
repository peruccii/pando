import { test, expect } from '@playwright/test'

test.describe('Critical Flows', () => {
  test('onboarding skip and create terminal from empty state', async ({ page }) => {
    await page.goto('/')

    await expect(page.locator('.onboarding-card')).toBeVisible()
    await page.getByRole('button', { name: /Pular|Skip/ }).click()

    await expect(page.locator('#empty-state')).toBeVisible()
    await expect(page.locator('#btn-new-terminal')).toBeVisible()

    await page.locator('#btn-new-terminal').click()
    await expect(page.locator('#command-center')).toBeVisible()
  })

  test('theme toggle cycles in titlebar', async ({ page }) => {
    await page.goto('/')

    const skipButton = page.getByRole('button', { name: /Pular|Skip/ })
    if (await skipButton.isVisible()) {
      await skipButton.click()
    }

    const root = page.locator('html')
    const beforeTheme = await root.getAttribute('data-theme')

    await page.locator('#btn-theme-toggle').click()

    await expect(async () => {
      const afterTheme = await root.getAttribute('data-theme')
      expect(afterTheme).not.toBe(beforeTheme)
    }).toPass()
  })
})
