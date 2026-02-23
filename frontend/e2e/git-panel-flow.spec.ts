import { expect, test, type Page } from '@playwright/test'
import { installGitPanelMock } from './helpers/gitPanelMock'

const PRIMARY_MOD = process.platform === 'darwin' ? 'Meta' : 'Control'

async function openGitPanel(page: Page) {
  await page.goto('/')
  await expect(page.locator('#empty-state')).toBeVisible()
  await page.locator('#btn-new-github').click()
  await expect(page.locator('#git-panel-screen')).toBeVisible()
}

test.describe('Git Panel E2E Flows', () => {
  test('happy path: seleciona linhas e executa stage patch', async ({ page }) => {
    await installGitPanelMock(page, { mode: 'default' })
    await openGitPanel(page)

    await page.getByRole('button', { name: 'Diff' }).click()
    await expect(page.locator('.git-panel-diff-line__checkbox').first()).toBeVisible()
    await page.locator('.git-panel-diff-line__checkbox').first().click()
    await page.getByRole('button', { name: /Stage Selected Lines/i }).click()

    await expect.poll(async () => {
      return page.evaluate(() => {
        const state = (window as unknown as { __gitPanelMockState?: { stagePatchCalls: number } }).__gitPanelMockState
        return state?.stagePatchCalls ?? 0
      })
    }).toBeGreaterThan(0)
  })

  test('fluxo de conflito: open tool + accept theirs', async ({ page }) => {
    await installGitPanelMock(page, { mode: 'conflict' })
    await openGitPanel(page)

    await page.getByRole('button', { name: 'Conflicts' }).click()
    await expect(page.getByText('Merge in progress')).toBeVisible()

    await expect(page.locator('.git-panel-conflicts__btn--external').first()).toBeVisible()
    await page.locator('.git-panel-conflicts__btn--external').first().click()
    await expect.poll(async () => {
      return page.evaluate(() => {
        const state = (window as unknown as { __gitPanelMockState?: { externalToolCalls: number } }).__gitPanelMockState
        return state?.externalToolCalls ?? 0
      })
    }).toBe(1)

    await expect(page.locator('.git-panel-conflicts__btn--theirs').first()).toBeVisible({ timeout: 10_000 })
    await page.locator('.git-panel-conflicts__btn--theirs').first().click()
    await expect(page.getByText('Nenhum conflito detectado')).toBeVisible()
  })

  test('concorrência de write: múltiplos atalhos não sobrepõem execução', async ({ page }) => {
    await installGitPanelMock(page, { writeDelayMs: 180 })
    await openGitPanel(page)

    for (let i = 0; i < 4; i += 1) {
      await page.keyboard.press(`${PRIMARY_MOD}+s`)
    }

    await expect.poll(async () => {
      return page.evaluate(() => {
        const state = (window as unknown as { __gitPanelMockState?: { stageCalls: number } }).__gitPanelMockState
        return state?.stageCalls ?? 0
      })
    }).toBeGreaterThan(0)

    await expect.poll(async () => {
      return page.evaluate(() => {
        const state = (window as unknown as { __gitPanelMockState?: { overlapCount: number } }).__gitPanelMockState
        return state?.overlapCount ?? 0
      })
    }).toBe(0)
  })

  test('arquivo grande/binário: fallback preserva responsividade', async ({ page }) => {
    await installGitPanelMock(page, { mode: 'large_binary' })
    await openGitPanel(page)

    await page.getByRole('button', { name: 'Diff' }).click()
    await page.selectOption('#git-panel-diff-file', 'src/huge.ts')
    await expect(page.getByText('é muito grande e foi truncado')).toBeVisible()

    await page.getByRole('button', { name: /Visualizar modo truncado/i }).click()
    await expect(page.getByText('Preview desativado automaticamente para arquivo grande')).toBeVisible()

    await page.selectOption('#git-panel-diff-file', 'assets/blob.bin')
    await expect(page.getByText('foi classificado como binário')).toBeVisible()
  })
})
