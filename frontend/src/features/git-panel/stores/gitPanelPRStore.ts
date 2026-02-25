import { create } from 'zustand'
import {
  GitPanelPRGetCommitRawDiff,
  GitPanelPRGet,
  GitPanelPRGetCommits,
  GitPanelPRGetFiles,
  GitPanelPRGetRawDiff,
  GitPanelPRList,
} from '../../../../wailsjs/go/main/App'
import {
  PR_ERROR_CODES,
  parsePRBindingError,
  type PRBindingError,
} from '../../github/types/prBindingError'
import type {
  GitPanelPRCommit,
  GitPanelPRCommitPage,
  GitPanelPRFile,
  GitPanelPRFilePage,
  GitPanelPRListState,
  GitPanelPullRequest,
} from '../types/pullRequests'

export type GitPanelPRBlockStatus = 'idle' | 'loading' | 'success' | 'error'
export type GitPanelPRBlockKey = 'list' | 'detail' | 'files' | 'commits' | 'rawDiff'

export interface GitPanelPRAsyncBlock<T> {
  status: GitPanelPRBlockStatus
  data: T
  error: PRBindingError | null
}

export interface GitPanelPRListData {
  items: GitPanelPullRequest[]
  state: GitPanelPRListState
  page: number
  perPage: number
}

export interface GitPanelPRListQuery {
  state?: GitPanelPRListState
  page?: number
  perPage?: number
}

export interface GitPanelPRPageQuery {
  prNumber?: number
  page?: number
  perPage?: number
  append?: boolean
}

export interface GitPanelPRMutationOptions {
  select?: boolean
  forceListState?: GitPanelPRListState
  resetSections?: boolean
}

export interface GitPanelPRStoreState {
  repoPath: string
  listState: GitPanelPRListState
  selectedPRNumber: number | null

  list: GitPanelPRAsyncBlock<GitPanelPRListData>
  detail: GitPanelPRAsyncBlock<GitPanelPullRequest | null>
  files: GitPanelPRAsyncBlock<GitPanelPRFilePage | null>
  commits: GitPanelPRAsyncBlock<GitPanelPRCommitPage | null>
  rawDiff: GitPanelPRAsyncBlock<string>

  setRepoPath: (repoPath: string) => void
  setListState: (state: GitPanelPRListState) => void
  selectPR: (prNumber: number | null) => void

  fetchList: (query?: GitPanelPRListQuery) => Promise<void>
  fetchDetail: (prNumber?: number) => Promise<void>
  fetchFiles: (query?: GitPanelPRPageQuery) => Promise<void>
  fetchCommits: (query?: GitPanelPRPageQuery) => Promise<void>
  fetchRawDiff: (prNumber?: number, commitSHA?: string) => Promise<void>
  syncMutationResult: (pr: GitPanelPullRequest, options?: GitPanelPRMutationOptions) => void

  clearBlockError: (block: GitPanelPRBlockKey) => void
  reset: () => void
}

const DEFAULT_LIST_PAGE = 1
const DEFAULT_LIST_PER_PAGE = 25
const DEFAULT_SECTION_PAGE = 1
const DEFAULT_SECTION_PER_PAGE = 50

function createAsyncBlock<T>(data: T): GitPanelPRAsyncBlock<T> {
  return {
    status: 'idle',
    data,
    error: null,
  }
}

function buildListData(state: GitPanelPRListState, page: number, perPage: number): GitPanelPRListData {
  return {
    items: [],
    state,
    page,
    perPage,
  }
}

function normalizeListState(value: string | null | undefined): GitPanelPRListState {
  const normalized = (value || '').trim().toLowerCase()
  switch (normalized) {
    case 'closed':
      return 'closed'
    case 'all':
      return 'all'
    default:
      return 'open'
  }
}

function normalizePositiveInt(value: number | undefined, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return fallback
  }
  const rounded = Math.floor(value)
  if (rounded <= 0) {
    return fallback
  }
  return rounded
}

function normalizePRNumber(value: number | null | undefined): number | null {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return null
  }
  const rounded = Math.floor(value)
  if (rounded <= 0) {
    return null
  }
  return rounded
}

function normalizePRNumberFromUnknown(value: unknown): number | null {
  if (typeof value === 'number') {
    return normalizePRNumber(value)
  }
  if (typeof value === 'string') {
    const normalized = value.trim()
    if (normalized === '') {
      return null
    }
    return normalizePRNumber(Number(normalized))
  }
  return null
}

function normalizeTrimmedString(value: unknown): string {
  if (typeof value !== 'string') {
    return ''
  }
  return value.trim()
}

function normalizeLooseString(value: unknown): string {
  if (typeof value !== 'string') {
    return ''
  }
  return value
}

function normalizeTimestampString(value: unknown): string {
  if (typeof value === 'string') {
    return value
  }
  if (value instanceof Date && !Number.isNaN(value.getTime())) {
    return value.toISOString()
  }
  return ''
}

function normalizeNonNegativeInt(value: unknown): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return 0
  }
  const rounded = Math.floor(value)
  if (rounded < 0) {
    return 0
  }
  return rounded
}

function normalizeOptionalString(value: unknown): string | undefined {
  if (typeof value !== 'string') {
    return undefined
  }
  const normalized = value.trim()
  if (!normalized) {
    return undefined
  }
  return normalized
}

function normalizePRUser(value: unknown): GitPanelPullRequest['author'] {
  if (!value || typeof value !== 'object') {
    return { login: '', avatarUrl: '' }
  }

  const candidate = value as { login?: unknown; avatarUrl?: unknown }
  return {
    login: normalizeTrimmedString(candidate.login),
    avatarUrl: normalizeTrimmedString(candidate.avatarUrl),
  }
}

function normalizePRUsers(value: unknown): GitPanelPullRequest['reviewers'] {
  if (!Array.isArray(value)) {
    return []
  }

  const users: GitPanelPullRequest['reviewers'] = []
  for (const item of value) {
    if (!item || typeof item !== 'object') {
      continue
    }
    const candidate = item as { login?: unknown; avatarUrl?: unknown }
    users.push({
      login: normalizeTrimmedString(candidate.login),
      avatarUrl: normalizeTrimmedString(candidate.avatarUrl),
    })
  }
  return users
}

function normalizePRLabels(value: unknown): GitPanelPullRequest['labels'] {
  if (!Array.isArray(value)) {
    return []
  }

  const labels: GitPanelPullRequest['labels'] = []
  for (const item of value) {
    if (!item || typeof item !== 'object') {
      continue
    }
    const candidate = item as { name?: unknown; color?: unknown; description?: unknown }
    labels.push({
      name: normalizeTrimmedString(candidate.name),
      color: normalizeTrimmedString(candidate.color),
      description: normalizeTrimmedString(candidate.description),
    })
  }
  return labels
}

function normalizePullRequest(payload: unknown): GitPanelPullRequest | null {
  if (!payload || typeof payload !== 'object') {
    return null
  }

  const candidate = payload as Partial<GitPanelPullRequest> & Record<string, unknown>
  const number = normalizePRNumberFromUnknown(candidate.number)
  if (!number) {
    return null
  }

  const state = normalizeTrimmedString(candidate.state)

  return {
    id: normalizeTrimmedString(candidate.id),
    number,
    title: normalizeTrimmedString(candidate.title),
    body: normalizeLooseString(candidate.body),
    state: state || 'OPEN',
    author: normalizePRUser(candidate.author),
    reviewers: normalizePRUsers(candidate.reviewers),
    labels: normalizePRLabels(candidate.labels),
    createdAt: normalizeTimestampString(candidate.createdAt),
    updatedAt: normalizeTimestampString(candidate.updatedAt),
    mergeCommit: normalizeOptionalString(candidate.mergeCommit),
    headBranch: normalizeTrimmedString(candidate.headBranch),
    baseBranch: normalizeTrimmedString(candidate.baseBranch),
    additions: normalizeNonNegativeInt(candidate.additions),
    deletions: normalizeNonNegativeInt(candidate.deletions),
    changedFiles: normalizeNonNegativeInt(candidate.changedFiles),
    isDraft: candidate.isDraft === true,
    maintainerCanModify: typeof candidate.maintainerCanModify === 'boolean'
      ? candidate.maintainerCanModify
      : undefined,
  }
}

function normalizePullRequestList(payload: unknown): GitPanelPullRequest[] {
  if (!Array.isArray(payload)) {
    return []
  }

  const items: GitPanelPullRequest[] = []
  for (const item of payload) {
    const normalized = normalizePullRequest(item)
    if (normalized) {
      items.push(normalized)
    }
  }
  return items
}

function trimRepoPath(repoPath: string): string {
  return repoPath.trim()
}

function buildStoreError(code: string, message: string, details?: string): PRBindingError {
  return {
    code,
    message,
    details,
  }
}

function resolvePRNumberOrError(
  candidate: number | null | undefined,
  selectedPRNumber: number | null,
): { prNumber: number | null; error: PRBindingError | null } {
  const explicit = normalizePRNumber(candidate)
  if (explicit) {
    return { prNumber: explicit, error: null }
  }

  const selected = normalizePRNumber(selectedPRNumber)
  if (selected) {
    return { prNumber: selected, error: null }
  }

  return {
    prNumber: null,
    error: buildStoreError(
      PR_ERROR_CODES.validationFailed,
      'Numero de Pull Request invalido.',
      'Selecione uma PR valida antes de carregar este bloco.',
    ),
  }
}

function normalizeCommitPage(
  payload: unknown,
  fallbackPage: number,
  fallbackPerPage: number,
): GitPanelPRCommitPage {
  const page = (payload && typeof payload === 'object') ? (payload as Partial<GitPanelPRCommitPage>) : {}
  return {
    items: Array.isArray(page.items) ? page.items as GitPanelPRCommit[] : [],
    page: normalizePositiveInt(page.page, fallbackPage),
    perPage: normalizePositiveInt(page.perPage, fallbackPerPage),
    hasNextPage: Boolean(page.hasNextPage),
    nextPage: normalizePRNumber(page.nextPage ?? null) ?? undefined,
  }
}

function normalizeFilePage(
  payload: unknown,
  fallbackPage: number,
  fallbackPerPage: number,
): GitPanelPRFilePage {
  const page = (payload && typeof payload === 'object') ? (payload as Partial<GitPanelPRFilePage>) : {}
  return {
    items: Array.isArray(page.items) ? page.items as GitPanelPRFile[] : [],
    page: normalizePositiveInt(page.page, fallbackPage),
    perPage: normalizePositiveInt(page.perPage, fallbackPerPage),
    hasNextPage: Boolean(page.hasNextPage),
    nextPage: normalizePRNumber(page.nextPage ?? null) ?? undefined,
  }
}

function mergeCommitPages(
  current: GitPanelPRCommitPage | null,
  incoming: GitPanelPRCommitPage,
  append: boolean,
): GitPanelPRCommitPage {
  if (!append || !current) {
    return incoming
  }

  return {
    ...incoming,
    items: [...current.items, ...incoming.items],
  }
}

function mergeFilePages(
  current: GitPanelPRFilePage | null,
  incoming: GitPanelPRFilePage,
  append: boolean,
): GitPanelPRFilePage {
  if (!append || !current) {
    return incoming
  }

  return {
    ...incoming,
    items: [...current.items, ...incoming.items],
  }
}

function normalizePullRequestStateForListFilter(state: string): 'open' | 'closed' {
  const normalized = state.trim().toLowerCase()
  if (normalized === 'closed' || normalized === 'merged') {
    return 'closed'
  }
  return 'open'
}

function matchesPullRequestListFilter(pr: GitPanelPullRequest, listState: GitPanelPRListState): boolean {
  if (listState === 'all') {
    return true
  }
  return normalizePullRequestStateForListFilter(pr.state) === listState
}

function upsertPullRequestInList(
  items: GitPanelPullRequest[],
  pr: GitPanelPullRequest,
  listState: GitPanelPRListState,
  page: number,
  perPage: number,
): GitPanelPullRequest[] {
  const nextItems = [...items]
  const existingIndex = nextItems.findIndex((item) => item.number === pr.number)
  const shouldInclude = matchesPullRequestListFilter(pr, listState)

  if (!shouldInclude) {
    if (existingIndex >= 0) {
      nextItems.splice(existingIndex, 1)
    }
    return nextItems
  }

  if (existingIndex >= 0) {
    nextItems[existingIndex] = pr
    return nextItems
  }

  if (page === DEFAULT_LIST_PAGE) {
    nextItems.unshift(pr)
    if (nextItems.length > perPage) {
      nextItems.length = perPage
    }
  }

  return nextItems
}

export const useGitPanelPRStore = create<GitPanelPRStoreState>((set, get) => {
  let listRequestSeq = 0
  let detailRequestSeq = 0
  let filesRequestSeq = 0
  let commitsRequestSeq = 0
  let rawDiffRequestSeq = 0
  let listInFlightKey = ''
  let detailInFlightKey = ''
  let filesInFlightKey = ''
  let commitsInFlightKey = ''
  let rawDiffInFlightKey = ''

  const invalidateListRequest = () => {
    listRequestSeq++
    listInFlightKey = ''
  }

  const invalidatePRDetailRequests = () => {
    detailRequestSeq++
    filesRequestSeq++
    commitsRequestSeq++
    rawDiffRequestSeq++
    detailInFlightKey = ''
    filesInFlightKey = ''
    commitsInFlightKey = ''
    rawDiffInFlightKey = ''
  }

  const invalidateAllRequests = () => {
    invalidateListRequest()
    invalidatePRDetailRequests()
  }

  return {
    repoPath: '',
    listState: 'open',
    selectedPRNumber: null,

    list: createAsyncBlock(buildListData('open', DEFAULT_LIST_PAGE, DEFAULT_LIST_PER_PAGE)),
    detail: createAsyncBlock<GitPanelPullRequest | null>(null),
    files: createAsyncBlock<GitPanelPRFilePage | null>(null),
    commits: createAsyncBlock<GitPanelPRCommitPage | null>(null),
    rawDiff: createAsyncBlock(''),

    setRepoPath: (repoPath) => {
      const normalizedRepoPath = trimRepoPath(repoPath)
      const current = get()
      if (normalizedRepoPath === current.repoPath) {
        return
      }

      invalidateAllRequests()
      set({
        repoPath: normalizedRepoPath,
        selectedPRNumber: null,
        list: createAsyncBlock(buildListData(current.listState, DEFAULT_LIST_PAGE, current.list.data.perPage)),
        detail: createAsyncBlock<GitPanelPullRequest | null>(null),
        files: createAsyncBlock<GitPanelPRFilePage | null>(null),
        commits: createAsyncBlock<GitPanelPRCommitPage | null>(null),
        rawDiff: createAsyncBlock(''),
      })
    },

    setListState: (state) => {
      const normalizedState = normalizeListState(state)
      invalidateListRequest()
      set((current) => ({
        listState: normalizedState,
        list: createAsyncBlock(buildListData(normalizedState, DEFAULT_LIST_PAGE, current.list.data.perPage)),
      }))
    },

    selectPR: (prNumber) => {
      invalidatePRDetailRequests()
      set({
        selectedPRNumber: normalizePRNumber(prNumber),
        detail: createAsyncBlock<GitPanelPullRequest | null>(null),
        files: createAsyncBlock<GitPanelPRFilePage | null>(null),
        commits: createAsyncBlock<GitPanelPRCommitPage | null>(null),
        rawDiff: createAsyncBlock(''),
      })
    },

    fetchList: async (query) => {
      const repoPath = trimRepoPath(get().repoPath)
      const requestedState = normalizeListState(query?.state ?? get().listState)
      const requestedPage = normalizePositiveInt(query?.page, DEFAULT_LIST_PAGE)
      const requestedPerPage = normalizePositiveInt(query?.perPage, DEFAULT_LIST_PER_PAGE)
      const requestKey = `${repoPath}|${requestedState}|${requestedPage}|${requestedPerPage}`

      if (!repoPath) {
        set((state) => ({
          list: {
            ...state.list,
            status: 'error',
            error: buildStoreError(
              PR_ERROR_CODES.repoPathRequired,
              'repoPath obrigatorio para carregar Pull Requests.',
              'Abra um repositorio Git valido antes de carregar a lista de PRs.',
            ),
          },
        }))
        return
      }
      if (listInFlightKey === requestKey) {
        return
      }

      listInFlightKey = requestKey
      const requestSeq = ++listRequestSeq
      set((state) => ({
        listState: requestedState,
        list: {
          ...state.list,
          status: 'loading',
          error: null,
          data: {
            ...state.list.data,
            state: requestedState,
            page: requestedPage,
            perPage: requestedPerPage,
          },
        },
      }))

      try {
        const items = await GitPanelPRList(repoPath, requestedState, requestedPage, requestedPerPage)
        if (requestSeq !== listRequestSeq) {
          return
        }

        const normalizedItems = normalizePullRequestList(items)
        set({
          listState: requestedState,
          list: {
            status: 'success',
            error: null,
            data: {
              items: normalizedItems,
              state: requestedState,
              page: requestedPage,
              perPage: requestedPerPage,
            },
          },
        })
      } catch (err) {
        if (requestSeq !== listRequestSeq) {
          return
        }

        set((state) => ({
          listState: requestedState,
          list: {
            ...state.list,
            status: 'error',
            error: parsePRBindingError(err),
            data: {
              ...state.list.data,
              state: requestedState,
              page: requestedPage,
              perPage: requestedPerPage,
            },
          },
        }))
      } finally {
        if (listInFlightKey === requestKey) {
          listInFlightKey = ''
        }
      }
    },

    fetchDetail: async (prNumber) => {
      const repoPath = trimRepoPath(get().repoPath)
      const resolved = resolvePRNumberOrError(prNumber ?? null, get().selectedPRNumber)
      const requestKey = `${repoPath}|${resolved.prNumber ?? 0}`
      if (resolved.error) {
        set((state) => ({
          detail: {
            ...state.detail,
            status: 'error',
            error: resolved.error,
          },
        }))
        return
      }

      if (!repoPath) {
        set((state) => ({
          detail: {
            ...state.detail,
            status: 'error',
            error: buildStoreError(
              PR_ERROR_CODES.repoPathRequired,
              'repoPath obrigatorio para carregar detalhe da PR.',
              'Abra um repositorio Git valido antes de carregar este bloco.',
            ),
          },
        }))
        return
      }
      if (detailInFlightKey === requestKey) {
        return
      }

      detailInFlightKey = requestKey
      const requestSeq = ++detailRequestSeq
      set((state) => ({
        selectedPRNumber: resolved.prNumber,
        detail: {
          ...state.detail,
          status: 'loading',
          error: null,
        },
      }))

      try {
        const detail = await GitPanelPRGet(repoPath, resolved.prNumber as number)
        if (requestSeq !== detailRequestSeq) {
          return
        }

        const normalizedDetail = normalizePullRequest(detail)
        if (!normalizedDetail) {
          set((state) => ({
            selectedPRNumber: resolved.prNumber,
            detail: {
              ...state.detail,
              status: 'error',
              error: buildStoreError(
                PR_ERROR_CODES.validationFailed,
                'Payload invalido para detalhe da Pull Request.',
                'O backend retornou um payload incompleto. Atualize a lista e tente novamente.',
              ),
            },
          }))
          return
        }

        set({
          selectedPRNumber: normalizedDetail.number,
          detail: {
            status: 'success',
            error: null,
            data: normalizedDetail,
          },
        })
      } catch (err) {
        if (requestSeq !== detailRequestSeq) {
          return
        }

        set((state) => ({
          selectedPRNumber: resolved.prNumber,
          detail: {
            ...state.detail,
            status: 'error',
            error: parsePRBindingError(err),
          },
        }))
      } finally {
        if (detailInFlightKey === requestKey) {
          detailInFlightKey = ''
        }
      }
    },

    fetchFiles: async (query) => {
      const repoPath = trimRepoPath(get().repoPath)
      const resolved = resolvePRNumberOrError(query?.prNumber ?? null, get().selectedPRNumber)
      const requestedPage = normalizePositiveInt(query?.page, DEFAULT_SECTION_PAGE)
      const requestedPerPage = normalizePositiveInt(query?.perPage, DEFAULT_SECTION_PER_PAGE)
      const append = query?.append === true
      const requestKey = `${repoPath}|${resolved.prNumber ?? 0}|${requestedPage}|${requestedPerPage}|${append ? 'append' : 'replace'}`

      if (resolved.error) {
        set((state) => ({
          files: {
            ...state.files,
            status: 'error',
            error: resolved.error,
          },
        }))
        return
      }

      if (!repoPath) {
        set((state) => ({
          files: {
            ...state.files,
            status: 'error',
            error: buildStoreError(
              PR_ERROR_CODES.repoPathRequired,
              'repoPath obrigatorio para carregar arquivos da PR.',
              'Abra um repositorio Git valido antes de carregar este bloco.',
            ),
          },
        }))
        return
      }
      if (filesInFlightKey === requestKey) {
        return
      }

      filesInFlightKey = requestKey
      const requestSeq = ++filesRequestSeq
      set((state) => ({
        selectedPRNumber: resolved.prNumber,
        files: {
          ...state.files,
          status: 'loading',
          error: null,
        },
      }))

      try {
        const response = await GitPanelPRGetFiles(
          repoPath,
          resolved.prNumber as number,
          requestedPage,
          requestedPerPage,
        )
        if (requestSeq !== filesRequestSeq) {
          return
        }

        const normalizedPage = normalizeFilePage(response, requestedPage, requestedPerPage)
        set((state) => ({
          selectedPRNumber: resolved.prNumber,
          files: {
            status: 'success',
            error: null,
            data: mergeFilePages(state.files.data, normalizedPage, append),
          },
        }))
      } catch (err) {
        if (requestSeq !== filesRequestSeq) {
          return
        }

        set((state) => ({
          selectedPRNumber: resolved.prNumber,
          files: {
            ...state.files,
            status: 'error',
            error: parsePRBindingError(err),
          },
        }))
      } finally {
        if (filesInFlightKey === requestKey) {
          filesInFlightKey = ''
        }
      }
    },

    fetchCommits: async (query) => {
      const repoPath = trimRepoPath(get().repoPath)
      const resolved = resolvePRNumberOrError(query?.prNumber ?? null, get().selectedPRNumber)
      const requestedPage = normalizePositiveInt(query?.page, DEFAULT_SECTION_PAGE)
      const requestedPerPage = normalizePositiveInt(query?.perPage, DEFAULT_SECTION_PER_PAGE)
      const append = query?.append === true
      const requestKey = `${repoPath}|${resolved.prNumber ?? 0}|${requestedPage}|${requestedPerPage}|${append ? 'append' : 'replace'}`

      if (resolved.error) {
        set((state) => ({
          commits: {
            ...state.commits,
            status: 'error',
            error: resolved.error,
          },
        }))
        return
      }

      if (!repoPath) {
        set((state) => ({
          commits: {
            ...state.commits,
            status: 'error',
            error: buildStoreError(
              PR_ERROR_CODES.repoPathRequired,
              'repoPath obrigatorio para carregar commits da PR.',
              'Abra um repositorio Git valido antes de carregar este bloco.',
            ),
          },
        }))
        return
      }
      if (commitsInFlightKey === requestKey) {
        return
      }

      commitsInFlightKey = requestKey
      const requestSeq = ++commitsRequestSeq
      set((state) => ({
        selectedPRNumber: resolved.prNumber,
        commits: {
          ...state.commits,
          status: 'loading',
          error: null,
        },
      }))

      try {
        const response = await GitPanelPRGetCommits(
          repoPath,
          resolved.prNumber as number,
          requestedPage,
          requestedPerPage,
        )
        if (requestSeq !== commitsRequestSeq) {
          return
        }

        const normalizedPage = normalizeCommitPage(response, requestedPage, requestedPerPage)
        set((state) => ({
          selectedPRNumber: resolved.prNumber,
          commits: {
            status: 'success',
            error: null,
            data: mergeCommitPages(state.commits.data, normalizedPage, append),
          },
        }))
      } catch (err) {
        if (requestSeq !== commitsRequestSeq) {
          return
        }

        set((state) => ({
          selectedPRNumber: resolved.prNumber,
          commits: {
            ...state.commits,
            status: 'error',
            error: parsePRBindingError(err),
          },
        }))
      } finally {
        if (commitsInFlightKey === requestKey) {
          commitsInFlightKey = ''
        }
      }
    },

    fetchRawDiff: async (prNumber, commitSHA) => {
      const repoPath = trimRepoPath(get().repoPath)
      const resolved = resolvePRNumberOrError(prNumber ?? null, get().selectedPRNumber)
      const normalizedCommitSHA = normalizeTrimmedString(commitSHA)
      const requestKey = `${repoPath}|${resolved.prNumber ?? 0}|${normalizedCommitSHA || 'pr'}`
      if (resolved.error) {
        set((state) => ({
          rawDiff: {
            ...state.rawDiff,
            status: 'error',
            error: resolved.error,
          },
        }))
        return
      }

      if (!repoPath) {
        set((state) => ({
          rawDiff: {
            ...state.rawDiff,
            status: 'error',
            error: buildStoreError(
              PR_ERROR_CODES.repoPathRequired,
              'repoPath obrigatorio para carregar diff completo da PR.',
              'Abra um repositorio Git valido antes de carregar este bloco.',
            ),
          },
        }))
        return
      }
      if (rawDiffInFlightKey === requestKey) {
        return
      }

      rawDiffInFlightKey = requestKey
      const requestSeq = ++rawDiffRequestSeq
      set((state) => ({
        selectedPRNumber: resolved.prNumber,
        rawDiff: {
          ...state.rawDiff,
          status: 'loading',
          error: null,
        },
      }))

      try {
        const rawDiff = normalizedCommitSHA
          ? await GitPanelPRGetCommitRawDiff(repoPath, resolved.prNumber as number, normalizedCommitSHA)
          : await GitPanelPRGetRawDiff(repoPath, resolved.prNumber as number)
        if (requestSeq !== rawDiffRequestSeq) {
          return
        }

        set({
          selectedPRNumber: resolved.prNumber,
          rawDiff: {
            status: 'success',
            error: null,
            data: typeof rawDiff === 'string' ? rawDiff : '',
          },
        })
      } catch (err) {
        if (requestSeq !== rawDiffRequestSeq) {
          return
        }

        set((state) => ({
          selectedPRNumber: resolved.prNumber,
          rawDiff: {
            ...state.rawDiff,
            status: 'error',
            error: parsePRBindingError(err),
          },
        }))
      } finally {
        if (rawDiffInFlightKey === requestKey) {
          rawDiffInFlightKey = ''
        }
      }
    },

    syncMutationResult: (pr, options) => {
      const nextPR = normalizePullRequest(pr)
      if (!nextPR) {
        return
      }
      const normalizedNumber = nextPR.number

      const forcedListState = options?.forceListState ? normalizeListState(options.forceListState) : undefined
      const select = options?.select === true
      const resetSections = options?.resetSections === true
      const shouldTouchDetail = select || get().selectedPRNumber === normalizedNumber

      invalidateListRequest()
      if (shouldTouchDetail) {
        detailRequestSeq++
        detailInFlightKey = ''
      }
      if (shouldTouchDetail && (select || resetSections)) {
        filesRequestSeq++
        commitsRequestSeq++
        rawDiffRequestSeq++
        filesInFlightKey = ''
        commitsInFlightKey = ''
        rawDiffInFlightKey = ''
      }

      set((state) => {
        const listStateChanged = forcedListState !== undefined && forcedListState !== state.listState
        const nextListState = forcedListState ?? state.listState
        const nextListPage = listStateChanged ? DEFAULT_LIST_PAGE : state.list.data.page
        const baseItems = listStateChanged ? [] : state.list.data.items
        const nextItems = upsertPullRequestInList(
          baseItems,
          nextPR,
          nextListState,
          nextListPage,
          state.list.data.perPage,
        )

        const shouldSelect = select || state.selectedPRNumber === normalizedNumber
        const shouldResetDetailSections = shouldSelect && (select || resetSections)

        return {
          listState: nextListState,
          selectedPRNumber: shouldSelect ? normalizedNumber : state.selectedPRNumber,
          list: {
            status: 'success',
            error: null,
            data: {
              items: nextItems,
              state: nextListState,
              page: nextListPage,
              perPage: state.list.data.perPage,
            },
          },
          detail: shouldSelect
            ? {
              status: 'success',
              error: null,
              data: nextPR,
            }
            : state.detail,
          files: shouldResetDetailSections
            ? createAsyncBlock<GitPanelPRFilePage | null>(null)
            : state.files,
          commits: shouldResetDetailSections
            ? createAsyncBlock<GitPanelPRCommitPage | null>(null)
            : state.commits,
          rawDiff: shouldResetDetailSections
            ? createAsyncBlock('')
            : state.rawDiff,
        }
      })
    },

    clearBlockError: (block) => {
      switch (block) {
        case 'list':
          set((state) => ({
            list: {
              ...state.list,
              status: state.list.status === 'error' ? 'idle' : state.list.status,
              error: null,
            },
          }))
          return
        case 'detail':
          set((state) => ({
            detail: {
              ...state.detail,
              status: state.detail.status === 'error' ? 'idle' : state.detail.status,
              error: null,
            },
          }))
          return
        case 'files':
          set((state) => ({
            files: {
              ...state.files,
              status: state.files.status === 'error' ? 'idle' : state.files.status,
              error: null,
            },
          }))
          return
        case 'commits':
          set((state) => ({
            commits: {
              ...state.commits,
              status: state.commits.status === 'error' ? 'idle' : state.commits.status,
              error: null,
            },
          }))
          return
        case 'rawDiff':
          set((state) => ({
            rawDiff: {
              ...state.rawDiff,
              status: state.rawDiff.status === 'error' ? 'idle' : state.rawDiff.status,
              error: null,
            },
          }))
      }
    },

    reset: () => {
      invalidateAllRequests()
      set({
        repoPath: '',
        listState: 'open',
        selectedPRNumber: null,
        list: createAsyncBlock(buildListData('open', DEFAULT_LIST_PAGE, DEFAULT_LIST_PER_PAGE)),
        detail: createAsyncBlock<GitPanelPullRequest | null>(null),
        files: createAsyncBlock<GitPanelPRFilePage | null>(null),
        commits: createAsyncBlock<GitPanelPRCommitPage | null>(null),
        rawDiff: createAsyncBlock(''),
      })
    },
  }
})
