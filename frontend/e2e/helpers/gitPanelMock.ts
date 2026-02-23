import type { Page } from '@playwright/test'

type GitPanelMockMode = 'default' | 'conflict' | 'large_binary'

export interface GitPanelMockOptions {
  mode?: GitPanelMockMode
  historyCount?: number
  writeDelayMs?: number
  failStage?: boolean
}

export async function installGitPanelMock(page: Page, options: GitPanelMockOptions = {}): Promise<void> {
  await page.addInitScript((rawOptions: GitPanelMockOptions) => {
    const opts: GitPanelMockOptions = rawOptions ?? {}
    const mode = opts.mode ?? 'default'
    const writeDelay = Math.max(0, opts.writeDelayMs ?? 40)
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
      }> = []
      for (let i = 0; i < count; i += 1) {
        const hash = `${(i + 1).toString(16).padStart(40, 'a')}`.slice(0, 40)
        out.push({
          hash,
          shortHash: hash.slice(0, 8),
          author: i % 2 === 0 ? 'Alice' : 'Bob',
          authoredAt: new Date(Date.now() - i * 120_000).toISOString(),
          subject: `feat(mock): commit ${i + 1}`,
        })
      }
      return out
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
