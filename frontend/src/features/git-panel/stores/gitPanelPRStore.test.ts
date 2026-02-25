import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as AppBindings from '../../../../wailsjs/go/main/App'
import { PR_ERROR_CODES } from '../../github/types/prBindingError'
import type {
  GitPanelPRFile,
  GitPanelPRFilePage,
  GitPanelPullRequest,
} from '../types/pullRequests'
import { useGitPanelPRStore } from './gitPanelPRStore'

vi.mock('../../../../wailsjs/go/main/App', () => ({
  GitPanelPRGet: vi.fn(),
  GitPanelPRGetCommits: vi.fn(),
  GitPanelPRGetCommitRawDiff: vi.fn(),
  GitPanelPRGetFiles: vi.fn(),
  GitPanelPRGetRawDiff: vi.fn(),
  GitPanelPRList: vi.fn(),
}))

function createDeferred<T>() {
  let resolve: (value: T | PromiseLike<T>) => void = () => { }
  let reject: (reason?: unknown) => void = () => { }
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function createPullRequest(number: number, state = 'open'): GitPanelPullRequest {
  const now = '2026-02-24T11:00:00.000Z'
  return {
    id: `pr-${number}`,
    number,
    title: `PR ${number}`,
    body: 'Descrição da PR',
    state,
    author: { login: 'alice', avatarUrl: '' },
    reviewers: [],
    labels: [],
    createdAt: now,
    updatedAt: now,
    headBranch: `feature/pr-${number}`,
    baseBranch: 'main',
    additions: 10,
    deletions: 2,
    changedFiles: 1,
    isDraft: false,
    maintainerCanModify: true,
  }
}

function createFile(filename: string): GitPanelPRFile {
  return {
    filename,
    status: 'modified',
    additions: 4,
    deletions: 1,
    changes: 5,
    patch: '@@ -1 +1 @@\n-old\n+new',
    hasPatch: true,
    patchState: 'ready',
    isBinary: false,
    isPatchTruncated: false,
  }
}

describe('gitPanelPRStore', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useGitPanelPRStore.getState().reset()
  })

  it('retorna erro de validação quando fetchList é chamado sem repoPath', async () => {
    await useGitPanelPRStore.getState().fetchList()

    const state = useGitPanelPRStore.getState()
    expect(state.list.status).toBe('error')
    expect(state.list.error).toMatchObject({
      code: PR_ERROR_CODES.repoPathRequired,
      message: 'repoPath obrigatorio para carregar Pull Requests.',
    })
  })

  it('marca loading e depois success ao carregar listagem de PRs', async () => {
    const deferred = createDeferred<GitPanelPullRequest[]>()
    vi.mocked(AppBindings.GitPanelPRList).mockReturnValueOnce(deferred.promise as Promise<never>)

    const store = useGitPanelPRStore.getState()
    store.setRepoPath('/tmp/repo')
    const request = store.fetchList({ state: 'open', page: 1, perPage: 10 })

    expect(useGitPanelPRStore.getState().list.status).toBe('loading')

    deferred.resolve([createPullRequest(101)])
    await request

    const state = useGitPanelPRStore.getState()
    expect(state.list.status).toBe('success')
    expect(state.list.data.items).toHaveLength(1)
    expect(state.list.data.items[0]?.number).toBe(101)
  })

  it('preserva estado e mensagem quando listagem falha', async () => {
    vi.mocked(AppBindings.GitPanelPRList).mockRejectedValueOnce({
      code: PR_ERROR_CODES.forbidden,
      message: 'Permissão insuficiente.',
      details: 'secondary rate limit',
    })

    const store = useGitPanelPRStore.getState()
    store.setRepoPath('/tmp/repo')
    await store.fetchList({ state: 'closed', page: 2, perPage: 30 })

    const state = useGitPanelPRStore.getState()
    expect(state.list.status).toBe('error')
    expect(state.list.data.state).toBe('closed')
    expect(state.list.data.page).toBe(2)
    expect(state.list.data.perPage).toBe(30)
    expect(state.list.error).toMatchObject({
      code: PR_ERROR_CODES.forbidden,
      message: 'Permissão insuficiente.',
      details: 'secondary rate limit',
    })
  })

  it('faz append de páginas de arquivos sem perder o conteúdo já carregado', async () => {
    const firstPage: GitPanelPRFilePage = {
      items: [createFile('src/first.ts')],
      page: 1,
      perPage: 1,
      hasNextPage: true,
      nextPage: 2,
    }
    const secondPage: GitPanelPRFilePage = {
      items: [createFile('src/second.ts')],
      page: 2,
      perPage: 1,
      hasNextPage: false,
    }

    vi.mocked(AppBindings.GitPanelPRGetFiles)
      .mockResolvedValueOnce(firstPage as never)
      .mockResolvedValueOnce(secondPage as never)

    const store = useGitPanelPRStore.getState()
    store.setRepoPath('/tmp/repo')
    await store.fetchFiles({ prNumber: 42, page: 1, perPage: 1, append: false })
    await store.fetchFiles({ prNumber: 42, page: 2, perPage: 1, append: true })

    const state = useGitPanelPRStore.getState()
    expect(state.files.status).toBe('success')
    expect(state.files.data?.items.map((file) => file.filename)).toEqual(['src/first.ts', 'src/second.ts'])
    expect(state.files.data?.page).toBe(2)
    expect(state.files.data?.hasNextPage).toBe(false)
  })
})
