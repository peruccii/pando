import { expect, test, type Page } from '@playwright/test'
import { installGitPanelMock } from './helpers/gitPanelMock'

const PRIMARY_MOD = process.platform === 'darwin' ? 'Meta' : 'Control'

async function openGitPanel(page: Page) {
  await page.goto('/')
  await expect(page.locator('#empty-state')).toBeVisible()
  await page.locator('#btn-new-github').click()
  await expect(page.locator('#git-panel-screen')).toBeVisible()
}

async function getLoadedCommits(page: Page): Promise<number> {
  const text = await page.locator('.git-panel-log__header .badge').first().innerText()
  const parsed = Number.parseInt(text.replace(/\D+/g, ''), 10)
  return Number.isNaN(parsed) ? 0 : parsed
}

test.describe('Git Panel UI Suite', () => {
  test('virtualiza histórico e pagina sem renderizar toda a lista', async ({ page }) => {
    await installGitPanelMock(page, { historyCount: 1200 })
    await openGitPanel(page)

    const initialLoaded = await getLoadedCommits(page)
    expect(initialLoaded).toBeGreaterThan(0)

    const visibleRows = await page.locator('.git-panel-log__row').count()
    expect(visibleRows).toBeLessThan(90)

    const viewport = page.locator('.git-panel-log__viewport')
    await viewport.evaluate((node) => {
      node.scrollTop = node.scrollHeight
    })

    await expect.poll(() => getLoadedCommits(page)).toBeGreaterThan(initialLoaded)
  })

  test('atalhos de teclado navegam e executam stage/unstage', async ({ page }) => {
    await installGitPanelMock(page)
    await openGitPanel(page)

    await page.keyboard.press('Alt+2')
    const beforeHash = await page.locator('.git-panel-log__row--selected').first().getAttribute('data-commit-hash')
    await page.keyboard.press('j')
    const afterHash = await page.locator('.git-panel-log__row--selected').first().getAttribute('data-commit-hash')
    expect(afterHash).not.toBe(beforeHash)

    await page.keyboard.press(`${PRIMARY_MOD}+s`)
    await expect.poll(async () => {
      return page.evaluate(() => {
        const state = (window as unknown as { __gitPanelMockState?: { stageCalls: number } }).__gitPanelMockState
        return state?.stageCalls ?? 0
      })
    }).toBeGreaterThan(0)

    await page.waitForTimeout(1100)
    await page.keyboard.press(`${PRIMARY_MOD}+Shift+s`)
    await expect.poll(async () => {
      return page.evaluate(() => {
        const state = (window as unknown as { __gitPanelMockState?: { unstageCalls: number } }).__gitPanelMockState
        return state?.unstageCalls ?? 0
      })
    }).toBeGreaterThan(0)
  })

  test('diff exibe fallbacks para binário e arquivo grande', async ({ page }) => {
    await installGitPanelMock(page, { mode: 'large_binary' })
    await openGitPanel(page)

    await page.getByRole('button', { name: 'Diff' }).click()
    await expect(page.locator('#git-panel-diff-file')).toBeVisible()

    await page.selectOption('#git-panel-diff-file', 'assets/blob.bin')
    await expect(page.getByText('foi classificado como binário')).toBeVisible()

    await page.selectOption('#git-panel-diff-file', 'src/huge.ts')
    await expect(page.getByText('é muito grande e foi truncado')).toBeVisible()
  })
})
