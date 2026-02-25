import { expect, test, type Page } from '@playwright/test'
import { installGitPanelMock } from './helpers/gitPanelMock'

const PERF_SAMPLES = 5

const PRS_PERF_BUDGETS = {
  firstPaintMs: 350,
  listVisibleMs: 1200,
  detailVisibleMs: 900,
  filesVisibleMs: 1000,
}

async function openGitPanel(page: Page) {
  await page.goto('/')
  await expect(page.locator('#empty-state')).toBeVisible()
  await page.locator('#btn-new-github').click()
  await expect(page.locator('#git-panel-screen')).toBeVisible()
}

function p50(values: number[]): number {
  if (values.length === 0) {
    return 0
  }
  const ordered = [...values].sort((left, right) => left - right)
  return ordered[Math.floor(ordered.length / 2)]
}

test.describe('Git Panel PRs Performance Budgets', () => {
  test('valida budgets iniciais da secao 6.4', async ({ page }) => {
    test.setTimeout(120_000)

    await installGitPanelMock(page, {
      historyCount: 360,
      prCount: 48,
      prListDelayMs: 320,
      prDetailDelayMs: 260,
      prFilesDelayMs: 360,
      prCommitsDelayMs: 280,
      prRawDiffDelayMs: 420,
    })

    const firstPaintSamples: number[] = []
    const listVisibleSamples: number[] = []
    const detailVisibleSamples: number[] = []
    const filesVisibleSamples: number[] = []

    for (let sample = 0; sample < PERF_SAMPLES; sample += 1) {
      await openGitPanel(page)

      const openStartedAt = Date.now()
      await page.getByRole('button', { name: 'PRs' }).click()
      await expect(page.locator('.git-panel-prs')).toBeVisible()
      firstPaintSamples.push(Date.now() - openStartedAt)

      const listItems = page.locator('.git-panel-prs__list .git-panel-prs__item')
      await expect(listItems.first()).toBeVisible()
      listVisibleSamples.push(Date.now() - openStartedAt)

      const listCount = await listItems.count()
      const targetIndex = Math.min(Math.max(1, sample + 1), Math.max(1, listCount - 1))
      const target = listItems.nth(targetIndex)
      const selectedText = await target.locator('strong').innerText()
      const selectedNumber = Number.parseInt(selectedText.replace(/\D+/g, ''), 10)

      const detailStartedAt = Date.now()
      await target.click()
      await expect(page.locator('.git-panel-prs__detail-number')).toHaveText(`#${selectedNumber}`)
      detailVisibleSamples.push(Date.now() - detailStartedAt)

      await expect(page.locator('.git-panel-prs__file-card').first()).toBeVisible()
      filesVisibleSamples.push(Date.now() - detailStartedAt)
    }

    const firstPaintP50 = p50(firstPaintSamples)
    const listVisibleP50 = p50(listVisibleSamples)
    const detailVisibleP50 = p50(detailVisibleSamples)
    const filesVisibleP50 = p50(filesVisibleSamples)

    console.log(
      `[gitpanel-prs-budget] firstPaint_p50=${firstPaintP50}ms list_p50=${listVisibleP50}ms detail_p50=${detailVisibleP50}ms files_p50=${filesVisibleP50}ms`,
    )

    expect(firstPaintP50).toBeLessThanOrEqual(PRS_PERF_BUDGETS.firstPaintMs)
    expect(listVisibleP50).toBeLessThanOrEqual(PRS_PERF_BUDGETS.listVisibleMs)
    expect(detailVisibleP50).toBeLessThanOrEqual(PRS_PERF_BUDGETS.detailVisibleMs)
    expect(filesVisibleP50).toBeLessThanOrEqual(PRS_PERF_BUDGETS.filesVisibleMs)
  })
})

