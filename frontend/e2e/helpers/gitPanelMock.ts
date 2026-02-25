import type { Page } from '@playwright/test'

type GitPanelMockMode = 'default' | 'conflict' | 'large_binary'

export interface GitPanelMockOptions {
  mode?: GitPanelMockMode
  historyCount?: number
  prCount?: number
  writeDelayMs?: number
  prListDelayMs?: number
  prDetailDelayMs?: number
  prFilesDelayMs?: number
  prCommitsDelayMs?: number
  prRawDiffDelayMs?: number
  failStage?: boolean
}

export async function installGitPanelMock(page: Page, options: GitPanelMockOptions = {}): Promise<void> {
  await page.addInitScript((rawOptions: GitPanelMockOptions) => {
    const opts: GitPanelMockOptions = rawOptions ?? {}
    const mode = opts.mode ?? 'default'
    const writeDelay = Math.max(0, opts.writeDelayMs ?? 40)
    const prListDelay = Math.max(0, opts.prListDelayMs ?? 320)
    const prDetailDelay = Math.max(0, opts.prDetailDelayMs ?? 260)
    const prFilesDelay = Math.max(0, opts.prFilesDelayMs ?? 380)
    const prCommitsDelay = Math.max(0, opts.prCommitsDelayMs ?? 280)
    const prRawDiffDelay = Math.max(0, opts.prRawDiffDelayMs ?? 420)
    const listeners = new Map<string, Set<(...args: unknown[]) => void>>()

    const runtime = {
      EventsOn(event: string, callback: (...args: unknown[]) => void) {
        const set = listeners.get(event) ?? new Set<(...args: unknown[]) => void>()
        set.add(callback)
        listeners.set(event, set)
        return () => {
          const next = listeners.get(event)
          next?.delete(callback)
        }
      },
      EventsOff(event: string) {
        listeners.delete(event)
      },
      EventsEmit(event: string, ...args: unknown[]) {
        const set = listeners.get(event)
        if (!set) return
        for (const callback of set) {
          callback(...args)
        }
      },
    }

    function emitCommand(action: string, status: string, exitCode = 0, stderrSanitized = '') {
      runtime.EventsEmit('gitpanel:command_result', {
        commandId: `mock_${Date.now()}_${Math.random()}`,
        repoPath: '/mock/repo',
        action,
        args: [],
        durationMs: writeDelay,
        exitCode,
        stderrSanitized,
        status,
      })
    }

    function makeHistory(count: number) {
      const out: Array<{
        hash: string
        shortHash: string
        author: string
        authoredAt: string
        subject: string
        additions: number
        deletions: number
        changedFiles: number
        githubLogin?: string
        githubAvatarUrl?: string
      }> = []
      for (let i = 0; i < count; i += 1) {
        const hash = `${(i + 1).toString(16).padStart(40, 'a')}`.slice(0, 40)
        const githubLogin = i % 2 === 0 ? 'alice-dev' : 'bob-dev'
        out.push({
          hash,
          shortHash: hash.slice(0, 8),
          author: i % 2 === 0 ? 'Alice' : 'Bob',
          authoredAt: new Date(Date.now() - i * 120_000).toISOString(),
          subject: `feat(mock): commit ${i + 1}`,
          additions: (i % 9) + 1,
          deletions: i % 4,
          changedFiles: (i % 5) + 1,
          githubLogin,
          githubAvatarUrl: `https://avatars.githubusercontent.com/${githubLogin}`,
        })
      }
      return out
    }

    function wait(ms: number): Promise<void> {
      return new Promise((resolve) => setTimeout(resolve, ms))
    }

    function clone<T>(value: T): T {
      return JSON.parse(JSON.stringify(value)) as T
    }

    function makePullRequests(count: number) {
      const total = Math.max(6, Math.floor(count))
      const now = Date.now()
      const out: Array<Record<string, unknown>> = []
      for (let i = 0; i < total; i += 1) {
        const number = 1200 + i
        const isClosed = i % 6 === 0
        const isMerged = i % 11 === 0
        const state = isMerged ? 'merged' : isClosed ? 'closed' : 'open'
        out.push({
          id: `pr_${number}`,
          number,
          title: `feat(mock): fluxo ${number}`,
          body: `Mock PR #${number} para validar budgets da aba PRs.`,
          state,
          author: {
            login: i % 2 === 0 ? 'alice-dev' : 'bob-dev',
            avatarUrl: 'https://avatars.githubusercontent.com/u/1?v=4',
          },
          reviewers: [],
          labels: [{ name: i % 2 === 0 ? 'backend' : 'frontend', color: i % 2 === 0 ? '0ea5e9' : '10b981' }],
          createdAt: new Date(now - (i + 1) * 60_000).toISOString(),
          updatedAt: new Date(now - i * 30_000).toISOString(),
          headBranch: `feature/mock-${number}`,
          baseBranch: 'main',
          additions: 20 + (i % 9) * 3,
          deletions: 8 + (i % 5) * 2,
          changedFiles: 10 + (i % 6),
          isDraft: i % 7 === 0,
          maintainerCanModify: true,
        })
      }
      return out
    }

    function buildPatch(index: number): string {
      const lines = [
        `@@ -${index + 1},3 +${index + 1},4 @@`,
        `-const oldValue${index} = ${index}`,
        `+const oldValue${index} = ${index + 1}`,
        `+const retryBudget${index} = true`,
        ' export function applyBudget() {',
        `-  return ${index}`,
        `+  return ${index + 1}`,
        ' }',
      ]
      if (index % 13 === 0) {
        lines.push('...')
      }
      return lines.join('\n')
    }

    function makePRFiles(prNumber: number, count: number) {
      const total = Math.max(1, Math.floor(count))
      const out: Array<Record<string, unknown>> = []
      for (let i = 0; i < total; i += 1) {
        const fileIndex = prNumber * 100 + i
        const isBinary = mode === 'large_binary' && i % 17 === 0
        const isTruncated = i % 13 === 0
        const patch = isBinary ? '' : buildPatch(fileIndex)
        out.push({
          filename: `src/mock/file-${prNumber}-${i}.ts`,
          previousFilename: i % 8 === 0 ? `src/mock/legacy-${prNumber}-${i}.ts` : '',
          status: i % 9 === 0 ? 'renamed' : 'modified',
          additions: 4 + (i % 7),
          deletions: 1 + (i % 4),
          changes: 5 + (i % 8),
          blobUrl: '',
          rawUrl: '',
          contentsUrl: '',
          patch,
          hasPatch: !isBinary,
          patchState: isBinary ? 'binary' : isTruncated ? 'truncated' : 'available',
          isBinary,
          isPatchTruncated: isTruncated && !isBinary,
        })
      }
      return out
    }

    function makePRCommits(prNumber: number, count: number) {
      const total = Math.max(1, Math.floor(count))
      const now = Date.now()
      const out: Array<Record<string, unknown>> = []
      for (let i = 0; i < total; i += 1) {
        const seed = `${prNumber}${i}`.padEnd(40, 'a').slice(0, 40)
        out.push({
          sha: seed,
          message: `mock(pr:${prNumber}): ajuste ${i + 1}`,
          htmlUrl: '',
          authorName: i % 2 === 0 ? 'Alice' : 'Bob',
          authoredAt: new Date(now - (i + 1) * 45_000).toISOString(),
          committerName: i % 2 === 0 ? 'Alice' : 'Bob',
          committedAt: new Date(now - (i + 1) * 42_000).toISOString(),
          author: {
            login: i % 2 === 0 ? 'alice-dev' : 'bob-dev',
            avatarUrl: 'https://avatars.githubusercontent.com/u/1?v=4',
          },
          committer: {
            login: i % 2 === 0 ? 'alice-dev' : 'bob-dev',
            avatarUrl: 'https://avatars.githubusercontent.com/u/1?v=4',
          },
          parentShas: [],
        })
      }
      return out
    }

    function paginate<T>(items: T[], page: number, perPage: number): { items: T[]; hasNextPage: boolean; nextPage?: number } {
      const safePage = Math.max(1, Math.floor(page || 1))
      const safePerPage = Math.max(1, Math.floor(perPage || 25))
      const start = (safePage - 1) * safePerPage
      const pagedItems = items.slice(start, start + safePerPage)
      const hasNextPage = start + safePerPage < items.length
      return {
        items: pagedItems,
        hasNextPage,
        nextPage: hasNextPage ? safePage + 1 : undefined,
      }
    }

    const state = {
      writeInFlight: false,
      overlapCount: 0,
      stageCalls: 0,
      unstageCalls: 0,
      stagePatchCalls: 0,
      externalToolCalls: 0,
      preflight: {
        gitAvailable: true,
        repoPath: '/mock/repo',
        repoRoot: '/mock/repo',
        branch: 'main',
        mergeActive: mode === 'conflict',
      },
      status: {
        branch: 'main',
        ahead: 2,
        behind: 1,
        staged: [
          { path: 'src/staged.ts', status: 'M ', added: 2, removed: 1 },
        ],
        unstaged: [
          { path: 'src/changed.ts', status: ' M', added: 4, removed: 2 },
          ...(mode === 'large_binary'
            ? [
                { path: 'src/huge.ts', status: ' M', added: 1200, removed: 0 },
                { path: 'assets/blob.bin', status: ' M', added: 0, removed: 0 },
              ]
            : []),
        ],
        conflicted: mode === 'conflict' ? [{ path: 'conflict.txt', status: 'UU' }] : [],
      },
      history: makeHistory(Math.max(120, opts.historyCount ?? 520)),
      prs: makePullRequests(Math.max(16, opts.prCount ?? 36)),
      prFilesByNumber: new Map<number, Array<Record<string, unknown>>>(),
      prCommitsByNumber: new Map<number, Array<Record<string, unknown>>>(),
      prCalls: {
        list: 0,
        detail: 0,
        files: 0,
        commits: 0,
        rawDiff: 0,
        create: 0,
        update: 0,
        checkMerged: 0,
        merge: 0,
        updateBranch: 0,
      },
      lastPRActions: {
        mergeMethod: '',
        expectedHeadSha: '',
      },
    }

    async function runWrite(action: string, fn: () => void): Promise<void> {
      emitCommand(action, 'queued')
      emitCommand(action, 'started')
      if (state.writeInFlight) {
        state.overlapCount += 1
      }
      state.writeInFlight = true
      await new Promise((resolve) => setTimeout(resolve, writeDelay))
      try {
        fn()
        emitCommand(action, 'succeeded')
      } finally {
        state.writeInFlight = false
      }
    }

    function resolvePRStateForFilter(rawState: unknown): 'open' | 'closed' | 'all' {
      const normalized = typeof rawState === 'string' ? rawState.trim().toLowerCase() : ''
      if (normalized === 'closed') {
        return 'closed'
      }
      if (normalized === 'all') {
        return 'all'
      }
      return 'open'
    }

    function normalizePRState(state: unknown): 'open' | 'closed' {
      const normalized = typeof state === 'string' ? state.trim().toLowerCase() : ''
      if (normalized === 'closed' || normalized === 'merged') {
        return 'closed'
      }
      return 'open'
    }

    function getPRByNumber(prNumber: number): Record<string, unknown> | null {
      const safeNumber = Math.max(1, Math.floor(prNumber || 0))
      return state.prs.find((item) => item.number === safeNumber) ?? null
    }

    function getPRFiles(prNumber: number): Array<Record<string, unknown>> {
      const safeNumber = Math.max(1, Math.floor(prNumber || 0))
      if (!state.prFilesByNumber.has(safeNumber)) {
        state.prFilesByNumber.set(safeNumber, makePRFiles(safeNumber, 58))
      }
      return state.prFilesByNumber.get(safeNumber) ?? []
    }

    function getPRCommits(prNumber: number): Array<Record<string, unknown>> {
      const safeNumber = Math.max(1, Math.floor(prNumber || 0))
      if (!state.prCommitsByNumber.has(safeNumber)) {
        state.prCommitsByNumber.set(safeNumber, makePRCommits(safeNumber, 85))
      }
      return state.prCommitsByNumber.get(safeNumber) ?? []
    }

    function getDiffPayload(filePath: string) {
      if (mode === 'large_binary' && filePath === 'assets/blob.bin') {
        return {
          mode: 'unified',
          filePath,
          raw: '',
          files: [{ path: filePath, status: 'modified', additions: 0, deletions: 0, isBinary: true, hunks: [] }],
          isBinary: true,
          isTruncated: false,
        }
      }

      if (mode === 'large_binary' && filePath === 'src/huge.ts') {
        return {
          mode: 'unified',
          filePath,
          raw: 'Preview desativado automaticamente para arquivo grande.',
          files: [],
          isBinary: false,
          isTruncated: true,
        }
      }

      return {
        mode: 'unified',
        filePath,
        raw: '',
        files: [
          {
            path: filePath,
            status: 'modified',
            additions: 2,
            deletions: 1,
            isBinary: false,
            hunks: [
              {
                header: '',
                oldStart: 10,
                oldLines: 3,
                newStart: 10,
                newLines: 4,
                lines: [
                  { type: 'context', content: 'const before = true', oldLine: 10, newLine: 10 },
                  { type: 'delete', content: 'const oldValue = 1', oldLine: 11 },
                  { type: 'add', content: 'const oldValue = 2', newLine: 11 },
                  { type: 'add', content: 'const extra = true', newLine: 12 },
                  { type: 'context', content: 'return oldValue', oldLine: 12, newLine: 13 },
                ],
              },
            ],
          },
        ],
        isBinary: false,
        isTruncated: false,
      }
    }

    async function stageFile(filePath: string): Promise<void> {
      if (opts.failStage) {
        emitCommand('stage_file', 'failed', 1, 'mock stage failure')
        throw new Error(JSON.stringify({ code: 'E_COMMAND_FAILED', message: 'Falha simulada de stage.' }))
      }

      await runWrite('stage_file', () => {
        state.stageCalls += 1
        const index = state.status.unstaged.findIndex((item) => item.path === filePath)
        if (index >= 0) {
          const [item] = state.status.unstaged.splice(index, 1)
          state.status.staged.push({ ...item, status: 'M ' })
        }
      })
    }

    async function unstageFile(filePath: string): Promise<void> {
      await runWrite('unstage_file', () => {
        state.unstageCalls += 1
        const index = state.status.staged.findIndex((item) => item.path === filePath)
        if (index >= 0) {
          const [item] = state.status.staged.splice(index, 1)
          state.status.unstaged.push({ ...item, status: ' M' })
        }
      })
    }

    const app = {
      GetHydrationData: async () => ({
        isAuthenticated: false,
        theme: 'dark',
        language: 'pt-BR',
        onboardingCompleted: true,
        version: '0.1.0-test',
      }),
      GetWorkspacesWithAgents: async () => ([{
        id: 1,
        userId: 'local',
        name: 'Mock',
        path: '/mock/repo',
        isActive: true,
        agents: [],
      }]),
      GetWorkspaceHistoryBuffer: async () => ({}),
      SetActiveWorkspace: async () => {},

      GitPanelPreflight: async () => ({ ...state.preflight }),
      GitPanelGetStatus: async () => JSON.parse(JSON.stringify(state.status)),
      GitPanelPickRepositoryDirectory: async (_defaultPath?: string) => '/mock/repo',
      GitPanelGetHistory: async (_repoPath: string, cursor: string, limit: number, query: string) => {
        const normalizedQuery = (query ?? '').trim().toLowerCase()
        const filtered = normalizedQuery
          ? state.history.filter((item) => {
              const raw = `${item.hash} ${item.shortHash} ${item.author} ${item.subject}`.toLowerCase()
              return raw.includes(normalizedQuery)
            })
          : state.history

        const startIndex = cursor
          ? Math.max(0, filtered.findIndex((item) => item.hash === cursor) + 1)
          : 0
        const safeLimit = Math.max(1, limit || 200)
        const slice = filtered.slice(startIndex, startIndex + safeLimit)
        const hasMore = startIndex+safeLimit < filtered.length
        const nextCursor = hasMore && slice.length > 0 ? slice[slice.length - 1].hash : ''

        return {
          items: slice,
          nextCursor,
          hasMore,
        }
      },
      GitPanelGetDiff: async (_repoPath: string, filePath: string) => getDiffPayload(filePath),
      AuthLogin: async (_provider: string) => ({}),
      GitPanelPRResolveRepository: async (repoPath: string, owner: string, repo: string) => ({
        repoPath: (repoPath || '').trim(),
        repoRoot: '/mock/repo',
        owner: (owner || 'acme').trim() || 'acme',
        repo: (repo || 'orch').trim() || 'orch',
        source: 'origin',
        originOwner: 'acme',
        originRepo: 'orch',
        overrideConfirmed: true,
      }),
      StartPolling: async () => {},
      StopPolling: async () => {},
      SetPollingContext: async () => {},
      GitPanelPRList: async (_repoPath: string, stateFilter: string, page: number, perPage: number) => {
        state.prCalls.list += 1
        await wait(prListDelay)
        const filter = resolvePRStateForFilter(stateFilter)
        const ordered = [...state.prs].sort((left, right) => {
          const leftTime = new Date(String(left.updatedAt || '')).getTime()
          const rightTime = new Date(String(right.updatedAt || '')).getTime()
          return rightTime - leftTime
        })
        const filtered = filter === 'all'
          ? ordered
          : ordered.filter((item) => normalizePRState(item.state) === filter)
        const pageSlice = paginate(filtered, page, perPage)
        return clone(pageSlice.items)
      },
      GitPanelPRGet: async (_repoPath: string, prNumber: number) => {
        state.prCalls.detail += 1
        await wait(prDetailDelay)
        const found = getPRByNumber(prNumber)
        if (!found) {
          throw new Error(JSON.stringify({
            code: 'E_PR_NOT_FOUND',
            message: `PR #${prNumber} nao encontrada.`,
          }))
        }
        return clone(found)
      },
      GitPanelPRGetFiles: async (_repoPath: string, prNumber: number, page: number, perPage: number) => {
        state.prCalls.files += 1
        await wait(prFilesDelay)
        const found = getPRByNumber(prNumber)
        if (!found) {
          throw new Error(JSON.stringify({
            code: 'E_PR_NOT_FOUND',
            message: `PR #${prNumber} nao encontrada.`,
          }))
        }
        const allFiles = getPRFiles(prNumber)
        const slice = paginate(allFiles, page, perPage)
        const safePage = Math.max(1, Math.floor(page || 1))
        const safePerPage = Math.max(1, Math.floor(perPage || 25))
        return {
          items: clone(slice.items),
          page: safePage,
          perPage: safePerPage,
          hasNextPage: slice.hasNextPage,
          nextPage: slice.nextPage,
        }
      },
      GitPanelPRGetCommits: async (_repoPath: string, prNumber: number, page: number, perPage: number) => {
        state.prCalls.commits += 1
        await wait(prCommitsDelay)
        const found = getPRByNumber(prNumber)
        if (!found) {
          throw new Error(JSON.stringify({
            code: 'E_PR_NOT_FOUND',
            message: `PR #${prNumber} nao encontrada.`,
          }))
        }
        const allCommits = getPRCommits(prNumber)
        const slice = paginate(allCommits, page, perPage)
        const safePage = Math.max(1, Math.floor(page || 1))
        const safePerPage = Math.max(1, Math.floor(perPage || 25))
        return {
          items: clone(slice.items),
          page: safePage,
          perPage: safePerPage,
          hasNextPage: slice.hasNextPage,
          nextPage: slice.nextPage,
        }
      },
      GitPanelPRGetRawDiff: async (_repoPath: string, prNumber: number) => {
        state.prCalls.rawDiff += 1
        await wait(prRawDiffDelay)
        const found = getPRByNumber(prNumber)
        if (!found) {
          throw new Error(JSON.stringify({
            code: 'E_PR_NOT_FOUND',
            message: `PR #${prNumber} nao encontrada.`,
          }))
        }
        const files = getPRFiles(prNumber).slice(0, 12)
        const lines: string[] = []
        for (const item of files) {
          const filename = String(item.filename || 'unknown.txt')
          const patch = String(item.patch || '')
          lines.push(`diff --git a/${filename} b/${filename}`)
          lines.push(`--- a/${filename}`)
          lines.push(`+++ b/${filename}`)
          lines.push(patch || 'Binary files differ')
        }
        return lines.join('\n')
      },
      GitPanelPRGetCommitRawDiff: async (_repoPath: string, prNumber: number, commitSHA: string) => {
        state.prCalls.rawDiff += 1
        await wait(prRawDiffDelay)
        const found = getPRByNumber(prNumber)
        if (!found) {
          throw new Error(JSON.stringify({
            code: 'E_PR_NOT_FOUND',
            message: `PR #${prNumber} nao encontrada.`,
          }))
        }
        const files = getPRFiles(prNumber).slice(0, 6)
        const lines: string[] = [`# commit ${commitSHA}`]
        for (const item of files) {
          const filename = String(item.filename || 'unknown.txt')
          const patch = String(item.patch || '')
          lines.push(`diff --git a/${filename} b/${filename}`)
          lines.push(`--- a/${filename}`)
          lines.push(`+++ b/${filename}`)
          lines.push(patch || 'Binary files differ')
        }
        return lines.join('\n')
      },
      GitPanelPRCreate: async (_repoPath: string, payload: Record<string, unknown>) => {
        state.prCalls.create += 1
        await wait(prDetailDelay)
        const maxNumber = state.prs.reduce((current, item) => Math.max(current, Number(item.number) || 0), 1200)
        const nextNumber = maxNumber + 1
        const nowISO = new Date().toISOString()
        const created = {
          id: `pr_${nextNumber}`,
          number: nextNumber,
          title: String(payload.title || '').trim() || `mock: nova PR ${nextNumber}`,
          body: String(payload.body || ''),
          state: 'open',
          author: { login: 'alice-dev', avatarUrl: 'https://avatars.githubusercontent.com/u/1?v=4' },
          reviewers: [],
          labels: [],
          createdAt: nowISO,
          updatedAt: nowISO,
          headBranch: String(payload.head || `feature/mock-${nextNumber}`),
          baseBranch: String(payload.base || 'main'),
          additions: 15,
          deletions: 4,
          changedFiles: 7,
          isDraft: Boolean(payload.draft),
          maintainerCanModify: payload.maintainerCanModify !== false,
        }
        state.prs.unshift(created)
        runtime.EventsEmit('github:prs:updated', { owner: 'acme', repo: 'orch', count: 1 })
        return clone(created)
      },
      GitPanelPRUpdate: async (_repoPath: string, prNumber: number, payload: Record<string, unknown>) => {
        state.prCalls.update += 1
        await wait(prDetailDelay)
        const index = state.prs.findIndex((item) => item.number === prNumber)
        if (index < 0) {
          throw new Error(JSON.stringify({
            code: 'E_PR_NOT_FOUND',
            message: `PR #${prNumber} nao encontrada.`,
          }))
        }
        const current = state.prs[index]
        const next = {
          ...current,
          title: typeof payload.title === 'string' ? payload.title : current.title,
          body: typeof payload.body === 'string' ? payload.body : current.body,
          state: typeof payload.state === 'string' ? payload.state : current.state,
          baseBranch: typeof payload.base === 'string' ? payload.base : current.baseBranch,
          maintainerCanModify: typeof payload.maintainerCanModify === 'boolean'
            ? payload.maintainerCanModify
            : current.maintainerCanModify,
          updatedAt: new Date().toISOString(),
        }
        state.prs[index] = next
        runtime.EventsEmit('github:prs:updated', { owner: 'acme', repo: 'orch', count: 1 })
        return clone(next)
      },
      GitPanelPRCheckMerged: async (_repoPath: string, prNumber: number) => {
        state.prCalls.checkMerged += 1
        const found = getPRByNumber(prNumber)
        return found ? String(found.state).toLowerCase() === 'merged' : false
      },
      GitPanelPRMerge: async (_repoPath: string, prNumber: number, payload: Record<string, unknown>) => {
        state.prCalls.merge += 1
        const mergeMethod = typeof payload.mergeMethod === 'string' ? payload.mergeMethod.trim().toLowerCase() : ''
        const index = state.prs.findIndex((item) => item.number === prNumber)
        if (index < 0) {
          throw new Error(JSON.stringify({
            code: 'E_PR_NOT_FOUND',
            message: `PR #${prNumber} nao encontrada.`,
          }))
        }
        const current = state.prs[index]
        state.prs[index] = { ...current, state: 'merged', updatedAt: new Date().toISOString() }
        state.lastPRActions.mergeMethod = mergeMethod
        runtime.EventsEmit('github:prs:updated', { owner: 'acme', repo: 'orch', count: 1 })
        return {
          sha: `${prNumber}`.padEnd(40, 'a').slice(0, 40),
          merged: true,
          message: `Pull Request mergeada com sucesso (${mergeMethod || 'merge'}).`,
        }
      },
      GitPanelPRUpdateBranch: async (_repoPath: string, prNumber: number, payload: Record<string, unknown>) => {
        state.prCalls.updateBranch += 1
        await wait(prDetailDelay)

        const index = state.prs.findIndex((item) => item.number === prNumber)
        if (index < 0) {
          throw new Error(JSON.stringify({
            code: 'E_PR_NOT_FOUND',
            message: `PR #${prNumber} nao encontrada.`,
          }))
        }

        const expectedHeadSha = typeof payload.expectedHeadSha === 'string'
          ? payload.expectedHeadSha.trim()
          : ''
        state.lastPRActions.expectedHeadSha = expectedHeadSha
        const current = state.prs[index]
        state.prs[index] = {
          ...current,
          updatedAt: new Date().toISOString(),
        }
        runtime.EventsEmit('github:prs:updated', { owner: 'acme', repo: 'orch', count: 1 })

        return {
          message: 'Branch atualizada com sucesso.',
        }
      },
      GitPanelStageFile: async (_repoPath: string, filePath: string) => stageFile(filePath),
      GitPanelUnstageFile: async (_repoPath: string, filePath: string) => unstageFile(filePath),
      GitPanelStagePatch: async () => {
        await runWrite('stage_patch', () => {
          state.stagePatchCalls += 1
        })
      },
      GitPanelAcceptOurs: async (_repoPath: string, filePath: string, autoStage: boolean) => {
        await runWrite('accept_ours', () => {
          state.status.conflicted = state.status.conflicted.filter((item) => item.path !== filePath)
          if (autoStage) {
            state.status.staged.push({ path: filePath, status: 'M ', added: 0, removed: 0 })
          }
        })
      },
      GitPanelAcceptTheirs: async (_repoPath: string, filePath: string, autoStage: boolean) => {
        await runWrite('accept_theirs', () => {
          state.status.conflicted = state.status.conflicted.filter((item) => item.path !== filePath)
          if (autoStage) {
            state.status.staged.push({ path: filePath, status: 'M ', added: 0, removed: 0 })
          }
        })
      },
      GitPanelOpenExternalMergeTool: async () => {
        await runWrite('open_external_tool', () => {
          state.externalToolCalls += 1
        })
      },
    }

    ;(window as unknown as { runtime: unknown }).runtime = runtime
    ;(window as unknown as { go: unknown }).go = { main: { App: app } }
    ;(window as unknown as { __gitPanelMockState: unknown }).__gitPanelMockState = state
  }, options)
}
