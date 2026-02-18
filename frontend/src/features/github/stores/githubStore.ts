import { create } from 'zustand'
import type {
  Repository,
  PullRequest,
  Diff,
  Issue,
  Branch,
  Review,
  Comment,
  PRState,
  IssueState,
  MergeMethod,
  DiffViewMode,
  GitHubView,
  GitHubError,
} from '../types/github'

function getGHApi() {
  return window.go?.main?.App
}

interface GitHubState {
  // Current context
  currentRepo: { owner: string; repo: string } | null
  currentView: GitHubView
  diffViewMode: DiffViewMode

  // Data
  repositories: Repository[]
  pullRequests: PullRequest[]
  selectedPR: PullRequest | null
  currentDiff: Diff | null
  issues: Issue[]
  branches: Branch[]
  currentBranch: string | null
  reviews: Review[]
  comments: Comment[]

  // UI
  isLoading: boolean
  error: GitHubError | null
  prFilter: PRState
  issueFilter: IssueState

  // Actions — Repo
  setCurrentRepo: (owner: string, repo: string) => void
  fetchRepositories: () => Promise<void>

  // Actions — View
  setCurrentView: (view: GitHubView) => void
  setDiffViewMode: (mode: DiffViewMode) => void
  setPRFilter: (state: PRState) => void
  setIssueFilter: (state: IssueState) => void
  clearError: () => void

  // Actions — PRs
  fetchPullRequests: () => Promise<void>
  selectPR: (pr: PullRequest | null) => void
  fetchPRDetail: (number: number) => Promise<void>
  fetchPRDiff: (number: number) => Promise<void>
  loadMoreDiffFiles: () => Promise<void>
  createPullRequest: (title: string, body: string, head: string, base: string, isDraft: boolean) => Promise<PullRequest | null>
  mergePullRequest: (number: number, method: MergeMethod) => Promise<boolean>
  closePullRequest: (number: number) => Promise<boolean>

  // Actions — Reviews & Comments
  fetchReviews: (prNumber: number) => Promise<void>
  createReview: (prNumber: number, body: string, event: string) => Promise<Review | null>
  fetchComments: (prNumber: number) => Promise<void>
  createComment: (prNumber: number, body: string) => Promise<Comment | null>
  createInlineComment: (prNumber: number, body: string, path: string, line: number, side: string) => Promise<Comment | null>

  // Actions — Issues
  fetchIssues: () => Promise<void>
  createIssue: (title: string, body: string) => Promise<Issue | null>
  updateIssue: (number: number, title?: string, body?: string, state?: string) => Promise<boolean>

  // Actions — Branches
  fetchBranches: () => Promise<void>
  setCurrentBranch: (branch: string | null) => void
  createBranch: (name: string, sourceBranch: string) => Promise<Branch | null>

  // Actions — Cache
  invalidateCache: () => void
}

export const useGitHubStore = create<GitHubState>((set, get) => ({
  // State
  currentRepo: null,
  currentView: 'prs',
  diffViewMode: 'unified',
  repositories: [],
  pullRequests: [],
  selectedPR: null,
  currentDiff: null,
  issues: [],
  branches: [],
  currentBranch: null,
  reviews: [],
  comments: [],
  isLoading: false,
  error: null,
  prFilter: 'OPEN',
  issueFilter: 'OPEN',

  // === Context ===

  setCurrentRepo: (owner, repo) => {
    set({ currentRepo: { owner, repo }, pullRequests: [], issues: [], branches: [], currentBranch: null, selectedPR: null, currentDiff: null, error: null })
  },

  setCurrentView: (view) => set({ currentView: view }),
  setDiffViewMode: (mode) => set({ diffViewMode: mode }),
  setPRFilter: (state) => { set({ prFilter: state }); get().fetchPullRequests() },
  setIssueFilter: (state) => { set({ issueFilter: state }); get().fetchIssues() },
  clearError: () => set({ error: null }),

  // === Repositories ===

  fetchRepositories: async () => {
    const api = getGHApi()
    if (!api) return

    set({ isLoading: true, error: null })
    try {
      const repos = await api.GHListRepositories()
      set({ repositories: repos || [], isLoading: false })
    } catch (err) {
      set({ isLoading: false, error: parseError(err) })
    }
  },

  // === Pull Requests ===

  fetchPullRequests: async () => {
    const api = getGHApi()
    const { currentRepo, prFilter } = get()
    if (!api || !currentRepo) return

    set({ isLoading: true, error: null })
    try {
      const prs = await api.GHListPullRequests(currentRepo.owner, currentRepo.repo, prFilter, 25)
      set({ pullRequests: prs || [], isLoading: false })
    } catch (err) {
      set({ isLoading: false, error: parseError(err) })
    }
  },

  selectPR: (pr) => set({ selectedPR: pr, currentDiff: null, reviews: [], comments: [] }),

  fetchPRDetail: async (number) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return

    set({ isLoading: true, error: null })
    try {
      const pr = await api.GHGetPullRequest(currentRepo.owner, currentRepo.repo, number)
      set({ selectedPR: pr, isLoading: false })
    } catch (err) {
      set({ isLoading: false, error: parseError(err) })
    }
  },

  fetchPRDiff: async (number) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return

    set({ isLoading: true, error: null })
    try {
      const diff = await api.GHGetPullRequestDiff(currentRepo.owner, currentRepo.repo, number, 20, '')
      set({ currentDiff: diff, isLoading: false })
    } catch (err) {
      set({ isLoading: false, error: parseError(err) })
    }
  },

  loadMoreDiffFiles: async () => {
    const api = getGHApi()
    const { currentRepo, selectedPR, currentDiff } = get()
    if (!api || !currentRepo || !selectedPR || !currentDiff?.pagination?.hasNextPage) return

    try {
      const moreDiff = await api.GHGetPullRequestDiff(
        currentRepo.owner, currentRepo.repo, selectedPR.number,
        20, currentDiff.pagination.endCursor || ''
      )
      if (moreDiff) {
        set({
          currentDiff: {
            files: [...currentDiff.files, ...moreDiff.files],
            totalFiles: moreDiff.totalFiles,
            pagination: moreDiff.pagination,
          }
        })
      }
    } catch (err) {
      set({ error: parseError(err) })
    }
  },

  createPullRequest: async (title, body, head, base, isDraft) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return null

    set({ isLoading: true, error: null })
    try {
      const pr = await api.GHCreatePullRequest(currentRepo.owner, currentRepo.repo, title, body, head, base, isDraft)
      set({ isLoading: false })
      get().fetchPullRequests() // Refresh list
      return pr
    } catch (err) {
      set({ isLoading: false, error: parseError(err) })
      return null
    }
  },

  mergePullRequest: async (number, method) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return false

    set({ isLoading: true, error: null })
    try {
      await api.GHMergePullRequest(currentRepo.owner, currentRepo.repo, number, method)
      set({ isLoading: false })
      get().fetchPullRequests()
      return true
    } catch (err) {
      set({ isLoading: false, error: parseError(err) })
      return false
    }
  },

  closePullRequest: async (number) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return false

    set({ isLoading: true, error: null })
    try {
      await api.GHClosePullRequest(currentRepo.owner, currentRepo.repo, number)
      set({ isLoading: false })
      get().fetchPullRequests()
      return true
    } catch (err) {
      set({ isLoading: false, error: parseError(err) })
      return false
    }
  },

  // === Reviews & Comments ===

  fetchReviews: async (prNumber) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return

    try {
      const reviews = await api.GHListReviews(currentRepo.owner, currentRepo.repo, prNumber)
      set({ reviews: reviews || [] })
    } catch (err) {
      set({ error: parseError(err) })
    }
  },

  createReview: async (prNumber, body, event) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return null

    try {
      const review = await api.GHCreateReview(currentRepo.owner, currentRepo.repo, prNumber, body, event)
      get().fetchReviews(prNumber)
      return review
    } catch (err) {
      set({ error: parseError(err) })
      return null
    }
  },

  fetchComments: async (prNumber) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return

    try {
      const comments = await api.GHListComments(currentRepo.owner, currentRepo.repo, prNumber)
      set({ comments: comments || [] })
    } catch (err) {
      set({ error: parseError(err) })
    }
  },

  createComment: async (prNumber, body) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return null

    try {
      const comment = await api.GHCreateComment(currentRepo.owner, currentRepo.repo, prNumber, body)
      get().fetchComments(prNumber)
      return comment
    } catch (err) {
      set({ error: parseError(err) })
      return null
    }
  },

  createInlineComment: async (prNumber, body, path, line, side) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return null

    try {
      const comment = await api.GHCreateInlineComment(
        currentRepo.owner, currentRepo.repo, prNumber, body, path, line, side
      )
      get().fetchComments(prNumber)
      return comment
    } catch (err) {
      set({ error: parseError(err) })
      return null
    }
  },

  // === Issues ===

  fetchIssues: async () => {
    const api = getGHApi()
    const { currentRepo, issueFilter } = get()
    if (!api || !currentRepo) return

    set({ isLoading: true, error: null })
    try {
      const issues = await api.GHListIssues(currentRepo.owner, currentRepo.repo, issueFilter, 50)
      set({ issues: issues || [], isLoading: false })
    } catch (err) {
      set({ isLoading: false, error: parseError(err) })
    }
  },

  createIssue: async (title, body) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return null

    set({ isLoading: true, error: null })
    try {
      const issue = await api.GHCreateIssue(currentRepo.owner, currentRepo.repo, title, body)
      set({ isLoading: false })
      get().fetchIssues()
      return issue
    } catch (err) {
      set({ isLoading: false, error: parseError(err) })
      return null
    }
  },

  updateIssue: async (number, title, body, state) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return false

    try {
      await api.GHUpdateIssue(
        currentRepo.owner, currentRepo.repo, number,
        title ?? null, body ?? null, state ?? null
      )
      get().fetchIssues()
      return true
    } catch (err) {
      set({ error: parseError(err) })
      return false
    }
  },

  // === Branches ===

  fetchBranches: async () => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return

    set({ isLoading: true, error: null })
    try {
      const branches = await api.GHListBranches(currentRepo.owner, currentRepo.repo)
      set({ branches: branches || [], isLoading: false })
    } catch (err) {
      set({ isLoading: false, error: parseError(err) })
    }
  },

  setCurrentBranch: (branch) => set({ currentBranch: branch }),

  createBranch: async (name, sourceBranch) => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return null

    set({ isLoading: true, error: null })
    try {
      const branch = await api.GHCreateBranch(currentRepo.owner, currentRepo.repo, name, sourceBranch)
      set({ isLoading: false })
      get().fetchBranches()
      return branch
    } catch (err) {
      set({ isLoading: false, error: parseError(err) })
      return null
    }
  },

  // === Cache ===

  invalidateCache: () => {
    const api = getGHApi()
    const { currentRepo } = get()
    if (!api || !currentRepo) return
    api.GHInvalidateCache(currentRepo.owner, currentRepo.repo)
  },
}))

// === Helper ===

function parseError(err: unknown): GitHubError {
  if (err && typeof err === 'object' && 'statusCode' in err) {
    return err as GitHubError
  }
  const message = err instanceof Error ? err.message : String(err)

  if (message.includes('401') || message.includes('token')) {
    return { statusCode: 401, message, type: 'auth' }
  }
  if (message.includes('403') || message.includes('rate')) {
    return { statusCode: 403, message, type: 'ratelimit' }
  }
  if (message.includes('404') || message.includes('not found')) {
    return { statusCode: 404, message, type: 'notfound' }
  }
  return { statusCode: 0, message, type: 'unknown' }
}
