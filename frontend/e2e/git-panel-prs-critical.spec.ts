import { expect, test, type Page } from '@playwright/test'
import { installGitPanelMock } from './helpers/gitPanelMock'

interface PRMockStateSnapshot {
  prs: Array<{ number: number; state: string }>
  prCalls: {
    create: number
    merge: number
    updateBranch: number
  }
  lastPRActions: {
    mergeMethod: string
    expectedHeadSha: string
  }
}

function parsePRNumber(raw: string): number {
  const parsed = Number.parseInt(raw.replace(/\D+/g, ''), 10)
  if (!Number.isFinite(parsed) || parsed <= 0) {
    throw new Error(`Nao foi possivel extrair numero de PR de: "${raw}"`)
  }
  return parsed
}

async function openGitPanel(page: Page) {
  await page.goto('/')
  await expect(page.locator('#empty-state')).toBeVisible()
  await page.locator('#btn-new-github').click()
  await expect(page.locator('#git-panel-screen')).toBeVisible()
}

async function openPRTab(page: Page) {
  await openGitPanel(page)
  await page.getByRole('button', { name: 'PRs' }).click()
  await expect(page.locator('.git-panel-prs')).toBeVisible()
  await expect(page.locator('.git-panel-prs__list .git-panel-prs__item').first()).toBeVisible()
}

async function selectFirstOpenPR(page: Page): Promise<number> {
  const openPRItem = page.locator('.git-panel-prs__item').filter({
    has: page.locator('.git-panel-prs__state-badge', { hasText: /^open$/i }),
  }).first()

  await expect(openPRItem).toBeVisible()
  const numberText = await openPRItem.locator('strong').innerText()
  const prNumber = parsePRNumber(numberText)
  await openPRItem.click()
  await expect(page.locator('.git-panel-prs__detail-number')).toHaveText(`#${prNumber}`)
  return prNumber
}

async function readPRMockState(page: Page): Promise<PRMockStateSnapshot> {
  return page.evaluate(() => {
    const snapshot = (window as unknown as { __gitPanelMockState?: PRMockStateSnapshot }).__gitPanelMockState
    if (!snapshot) {
      throw new Error('Git panel mock state indisponivel')
    }
    return JSON.parse(JSON.stringify(snapshot)) as PRMockStateSnapshot
  })
}

test.describe('Git Panel PRs Critical E2E', () => {
  test('listar PRs, abrir detalhe e visualizar arquivos', async ({ page }) => {
    await installGitPanelMock(page, { prCount: 40 })
    await openPRTab(page)

    const listItems = page.locator('.git-panel-prs__list .git-panel-prs__item')
    const listCount = await listItems.count()
    expect(listCount).toBeGreaterThan(0)

    const targetIndex = Math.min(2, listCount - 1)
    const targetItem = listItems.nth(targetIndex)
    const targetPRNumber = parsePRNumber(await targetItem.locator('strong').innerText())
    await targetItem.click()

    await expect(page.locator('.git-panel-prs__detail-number')).toHaveText(`#${targetPRNumber}`)
    const firstFileCard = page.locator('.git-panel-prs__file-card').first()
    await expect(firstFileCard).toBeVisible()

    await firstFileCard.getByRole('button', { name: 'Exibir patch' }).click()
    await expect(firstFileCard.locator('.git-panel-prs__patch')).toBeVisible()
  })

  test('criar PR e refletir imediatamente na lista/detalhe', async ({ page }) => {
    await installGitPanelMock(page, { prCount: 28 })
    await openPRTab(page)

    const before = await readPRMockState(page)
    const createdTitle = 'feat(e2e): fluxo critico de PR'
    const createdHead = 'feature/e2e-pr-flow'
    const createdBase = 'main'

    await page.getByRole('button', { name: 'Nova PR' }).click()
    await page.locator('#git-panel-pr-create-title').fill(createdTitle)
    await page.locator('#git-panel-pr-create-head').fill(createdHead)
    await page.locator('#git-panel-pr-create-base').fill(createdBase)
    await page.locator('#git-panel-pr-create-body').fill('E2E: validar criação com refresh de lista e detalhe.')
    await page.getByRole('button', { name: 'Criar PR' }).click()

    await expect(page.getByRole('button', { name: 'Nova PR' })).toBeVisible()
    await expect(page.locator('.git-panel-prs__detail-header h3')).toHaveText(createdTitle)
    await expect(page.locator('.git-panel-prs__detail-branches')).toContainText(createdHead)
    await expect(page.locator('.git-panel-prs__detail-branches')).toContainText(createdBase)
    await expect(page.locator('.git-panel-prs__list .git-panel-prs__item').filter({ hasText: createdTitle }).first()).toBeVisible()

    await expect.poll(async () => {
      const snapshot = await readPRMockState(page)
      return snapshot.prCalls.create
    }).toBe(before.prCalls.create + 1)
  })

  test('merge da PR atualiza estado para merged e checkMerged retorna true', async ({ page }) => {
    await installGitPanelMock(page, { prCount: 34 })
    await openPRTab(page)

    const before = await readPRMockState(page)
    const prNumber = await selectFirstOpenPR(page)

    const mergeResult = await page.evaluate(async ({ number }) => {
      const app = (window as unknown as { go: { main: { App: { GitPanelPRMerge: (repoPath: string, prNumber: number, payload: Record<string, unknown>) => Promise<{ merged: boolean; message: string }> } } } }).go.main.App
      return app.GitPanelPRMerge('/mock/repo', number, { mergeMethod: 'squash' })
    }, { number: prNumber })

    expect(mergeResult.merged).toBe(true)
    expect(mergeResult.message).toContain('mergeada')

    await expect.poll(async () => {
      const snapshot = await readPRMockState(page)
      return snapshot.prCalls.merge
    }).toBe(before.prCalls.merge + 1)

    await expect.poll(async () => {
      const snapshot = await readPRMockState(page)
      const mergedPR = snapshot.prs.find((item) => item.number === prNumber)
      return mergedPR?.state ?? ''
    }).toBe('merged')

    await expect.poll(async () => {
      return page.evaluate(async ({ number }) => {
        const app = (window as unknown as { go: { main: { App: { GitPanelPRCheckMerged: (repoPath: string, prNumber: number) => Promise<boolean> } } } }).go.main.App
        return app.GitPanelPRCheckMerged('/mock/repo', number)
      }, { number: prNumber })
    }).toBe(true)

    await page.getByRole('button', { name: 'All' }).click()
    const mergedListItem = page.locator('.git-panel-prs__item').filter({ hasText: `#${prNumber}` }).first()
    await expect(mergedListItem).toBeVisible()
    await expect(mergedListItem.locator('.git-panel-prs__state-badge')).toHaveText('merged')
    await mergedListItem.click()

    await expect(page.locator('.git-panel-prs__detail-number')).toHaveText(`#${prNumber}`)
    await expect(page.locator('.git-panel-prs__detail-card .git-panel-prs__state-badge')).toHaveText('merged')
    await expect.poll(async () => {
      const snapshot = await readPRMockState(page)
      return snapshot.lastPRActions.mergeMethod
    }).toBe('squash')
  })

  test('update branch executa com sucesso e registra expectedHeadSha', async ({ page }) => {
    await installGitPanelMock(page, { prCount: 32 })
    await openPRTab(page)

    const before = await readPRMockState(page)
    const prNumber = await selectFirstOpenPR(page)
    const expectedHeadSha = 'deadbeefcafebabe'

    const updateResult = await page.evaluate(async ({ number, sha }) => {
      const app = (window as unknown as { go: { main: { App: { GitPanelPRUpdateBranch: (repoPath: string, prNumber: number, payload: Record<string, unknown>) => Promise<{ message: string }> } } } }).go.main.App
      return app.GitPanelPRUpdateBranch('/mock/repo', number, { expectedHeadSha: sha })
    }, { number: prNumber, sha: expectedHeadSha })

    expect(updateResult.message).toContain('Branch atualizada com sucesso')

    await expect.poll(async () => {
      const snapshot = await readPRMockState(page)
      return snapshot.prCalls.updateBranch
    }).toBe(before.prCalls.updateBranch + 1)

    await expect.poll(async () => {
      const snapshot = await readPRMockState(page)
      return snapshot.lastPRActions.expectedHeadSha
    }).toBe(expectedHeadSha)

    await expect(page.locator('.git-panel-prs__detail-number')).toHaveText(`#${prNumber}`)
  })
})
