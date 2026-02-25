import { memo, useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type FormEvent, type KeyboardEvent, type PointerEvent } from 'react'
import { AlertTriangle, Clock3, FileCode2, GitBranch, GitCommitHorizontal, GitPullRequest, Loader2, RefreshCcw, Search } from 'lucide-react'
import type { BundledTheme } from 'shiki'
import { shallow } from 'zustand/shallow'
import {
  AuthLogin,
  GHListBranches,
  GitPanelGetStatus,
  GitPanelPRCreate,
  GitPanelPRCreateLabel,
  GitPanelPRPushLocalBranch,
  GitPanelPRUpdate,
  GitPanelPRResolveRepository,
  SetPollingContext,
  StartPolling,
  StopPolling,
} from '../../../../wailsjs/go/main/App'
import { PR_ERROR_CODES, parsePRBindingError, type PRBindingError } from '../../github/types/prBindingError'
import { useGitPanelPRStore, type GitPanelPRStoreState } from '../stores/gitPanelPRStore'
import type {
  GitPanelPRCommit,
  GitPanelPRFile,
  GitPanelPRListState,
  GitPanelPullRequest,
} from '../types/pullRequests'
import { ReactAICodeBlock } from './ReactAICodeBlock'
import './GitPanelPRView.css'

interface GitPanelPRViewProps {
  repoPath: string
}

type PRRightSection = 'conversation' | 'files' | 'commits'
type PollingContextType = 'pr_detail' | 'pr_list' | 'minimized'
type CreatePRRequiredField = 'title' | 'head' | 'base' | 'targetOwner' | 'targetRepo'
type CreatePRValidationErrors = Partial<Record<CreatePRRequiredField, string>>
type CreateLabelRequiredField = 'name' | 'color'
type CreateLabelValidationField = CreateLabelRequiredField | 'description'
type CreateLabelValidationErrors = Partial<Record<CreateLabelValidationField, string>>
type EditPRWritableState = 'open' | 'closed'
type EditPRRequiredField = 'title' | 'base'
type EditPRValidationErrors = Partial<Record<EditPRRequiredField, string>>

interface CreatePRFormState {
  title: string
  head: string
  base: string
  body: string
  draft: boolean
  maintainerCanModify: boolean
  advancedMode: boolean
  targetOwner: string
  targetRepo: string
  confirmTargetOverride: boolean
}

interface CreatePRResolvedTarget {
  owner: string
  repo: string
}

interface CreateLabelFormState {
  name: string
  color: string
  description: string
}

interface EditPRFormState {
  title: string
  body: string
  base: string
  state: EditPRWritableState
  maintainerCanModify: boolean
}

const FILTERS: Array<{ label: string; value: GitPanelPRListState }> = [
  { label: 'Open', value: 'open' },
  { label: 'Closed', value: 'closed' },
  { label: 'All', value: 'all' },
]

const SECTION_PER_PAGE = 25
const FILES_RENDER_BATCH_SIZE = 40
const PATCH_PREVIEW_MAX_LINES = 220
const PATCH_PREVIEW_MAX_CHARS = 18_000
const PATCH_FULL_RENDER_GUARD_LINES = 1_000
const PATCH_FULL_RENDER_GUARD_CHARS = 80_000
const DEFAULT_LIST_PANEL_WIDTH = 30
const MIN_LIST_PANEL_WIDTH = 16
const MAX_LIST_PANEL_WIDTH = 58
const PR_UPDATE_REFRESH_DEBOUNCE_MS = 220
const CREATE_PR_BRANCH_CACHE_TTL_MS = 20_000
const CREATE_PR_DEFAULT_BASE = 'main'
const CREATE_LABEL_DEFAULT_COLOR = '#0e8a16'
const CREATE_LABEL_COLOR_REGEX = /^#?[0-9a-fA-F]{6}$/
const CREATE_LABEL_MAX_DESCRIPTION_CHARS = 100
const PR_DIFF_SHIKI_THEME: BundledTheme = 'github-dark-default'
type PatchViewMode = 'split' | 'unified'

export function GitPanelPRView({ repoPath }: GitPanelPRViewProps) {
  const normalizedRepoPath = repoPath.trim()
  const [activeSection, setActiveSection] = useState<PRRightSection>('conversation')
  const [listPanelWidth, setListPanelWidth] = useState(DEFAULT_LIST_PANEL_WIDTH)
  const [isResizingColumns, setIsResizingColumns] = useState(false)
  const [selectedCommitSha, setSelectedCommitSha] = useState('')
  const [commitMetricsBySha, setCommitMetricsBySha] = useState<Record<string, { additions: number; deletions: number; changedFiles: number }>>({})
  const [listQuery, setListQuery] = useState('')
  const [isCreateFormOpen, setIsCreateFormOpen] = useState(false)
  const [isPreparingCreatePRContext, setIsPreparingCreatePRContext] = useState(false)
  const [isCreatingPR, setIsCreatingPR] = useState(false)
  const [createPRSubmitMode, setCreatePRSubmitMode] = useState<'create' | 'push_and_create'>('create')
  const [createPRForm, setCreatePRForm] = useState<CreatePRFormState>(() => createDefaultPRFormState())
  const [createPRValidationErrors, setCreatePRValidationErrors] = useState<CreatePRValidationErrors>({})
  const [createPRError, setCreatePRError] = useState<PRBindingError | null>(null)
  const [createPRCurrentBranch, setCreatePRCurrentBranch] = useState('')
  const [createPRAhead, setCreatePRAhead] = useState<number | null>(null)
  const [createPRTarget, setCreatePRTarget] = useState<CreatePRResolvedTarget | null>(null)
  const [createPRBaseOptions, setCreatePRBaseOptions] = useState<string[]>([CREATE_PR_DEFAULT_BASE])
  const [isCreateLabelFormOpen, setIsCreateLabelFormOpen] = useState(false)
  const [isCreatingLabel, setIsCreatingLabel] = useState(false)
  const [createLabelForm, setCreateLabelForm] = useState<CreateLabelFormState>(() => createDefaultLabelFormState())
  const [createLabelValidationErrors, setCreateLabelValidationErrors] = useState<CreateLabelValidationErrors>({})
  const [createLabelError, setCreateLabelError] = useState<PRBindingError | null>(null)
  const [createLabelSuccess, setCreateLabelSuccess] = useState('')
  const [isEditFormOpen, setIsEditFormOpen] = useState(false)
  const [isUpdatingPR, setIsUpdatingPR] = useState(false)
  const [editPRForm, setEditPRForm] = useState<EditPRFormState | null>(null)
  const [editPRValidationErrors, setEditPRValidationErrors] = useState<EditPRValidationErrors>({})
  const [editPRError, setEditPRError] = useState<PRBindingError | null>(null)
  const [pollingTarget, setPollingTarget] = useState<{ owner: string; repo: string } | null>(null)
  const prsUpdatedRefreshTimerRef = useRef<number | null>(null)
  const createPRBranchCacheRef = useRef<{ key: string; expiresAt: number; names: Set<string> } | null>(null)
  const splitResizeContainerRef = useRef<HTMLElement | null>(null)
  const splitResizeSessionRef = useRef<{
    startX: number
    startWidthPx: number
    containerWidthPx: number
  } | null>(null)
  const [isWindowVisible, setIsWindowVisible] = useState(() => {
    if (typeof document === 'undefined') {
      return true
    }
    return document.visibilityState === 'visible'
  })

  const {
    listState,
    selectedPRNumber,
    list,
    detail,
    files,
    commits,
    rawDiff,
    setRepoPath,
    setListState,
    selectPR,
    fetchList,
    fetchDetail,
    fetchFiles,
    fetchCommits,
    fetchRawDiff,
    syncMutationResult,
  } = useGitPanelPRStore(
    (state) => ({
      listState: state.listState,
      selectedPRNumber: state.selectedPRNumber,
      list: state.list,
      detail: state.detail,
      files: state.files,
      commits: state.commits,
      rawDiff: state.rawDiff,
      setRepoPath: state.setRepoPath,
      setListState: state.setListState,
      selectPR: state.selectPR,
      fetchList: state.fetchList,
      fetchDetail: state.fetchDetail,
      fetchFiles: state.fetchFiles,
      fetchCommits: state.fetchCommits,
      fetchRawDiff: state.fetchRawDiff,
      syncMutationResult: state.syncMutationResult,
    }),
    shallow,
  )

  const refreshListQueryRef = useRef<{ state: GitPanelPRListState; page: number; perPage: number }>({
    state: 'open',
    page: 1,
    perPage: 25,
  })
  const selectedPRNumberRef = useRef<number | null>(null)

  const hasRateLimitSignal = useMemo(() => {
    return [
      list.error,
      detail.error,
      files.error,
      commits.error,
      rawDiff.error,
    ].some((error) => isRateLimitedBindingError(error))
  }, [commits.error, detail.error, files.error, list.error, rawDiff.error])

  useEffect(() => {
    refreshListQueryRef.current = {
      state: listState,
      page: list.data.page,
      perPage: list.data.perPage,
    }
  }, [list.data.page, list.data.perPage, listState])

  useEffect(() => {
    selectedPRNumberRef.current = selectedPRNumber
  }, [selectedPRNumber])

  useEffect(() => {
    setRepoPath(normalizedRepoPath)
    setActiveSection('conversation')
    setCommitMetricsBySha({})
    setListQuery('')
    setIsCreateFormOpen(false)
    setIsPreparingCreatePRContext(false)
    setIsCreatingPR(false)
    setCreatePRSubmitMode('create')
    setCreatePRForm(createDefaultPRFormState())
    setCreatePRValidationErrors({})
    setCreatePRError(null)
    setCreatePRCurrentBranch('')
    setCreatePRAhead(null)
    setCreatePRTarget(null)
    setCreatePRBaseOptions([CREATE_PR_DEFAULT_BASE])
    setIsCreateLabelFormOpen(false)
    setIsCreatingLabel(false)
    setCreateLabelForm(createDefaultLabelFormState())
    setCreateLabelValidationErrors({})
    setCreateLabelError(null)
    setCreateLabelSuccess('')
    setIsEditFormOpen(false)
    setIsUpdatingPR(false)
    setEditPRForm(null)
    setEditPRValidationErrors({})
    setEditPRError(null)
  }, [normalizedRepoPath, setRepoPath])

  useEffect(() => {
    if (selectedPRNumber !== null) {
      setActiveSection('conversation')
    }
    setCommitMetricsBySha({})
    setIsEditFormOpen(false)
    setIsUpdatingPR(false)
    setEditPRForm(null)
    setEditPRValidationErrors({})
    setEditPRError(null)
    setSelectedCommitSha('')
  }, [selectedPRNumber])

  useEffect(() => {
    if (!normalizedRepoPath) {
      return
    }
    void fetchList({ state: listState, page: 1, perPage: list.data.perPage })
  }, [normalizedRepoPath, fetchList, list.data.perPage, listState])

  useEffect(() => {
    if (list.status !== 'success') {
      return
    }
    if (list.data.items.length === 0) {
      if (selectedPRNumber !== null) {
        selectPR(null)
      }
      return
    }

    const hasSelected = selectedPRNumber !== null
      && list.data.items.some((item) => item.number === selectedPRNumber)
    if (hasSelected) {
      return
    }

    const fallback = list.data.items[0]
    selectPR(fallback.number)
    void fetchDetail(fallback.number)
  }, [fetchDetail, list.data.items, list.status, selectPR, selectedPRNumber])

  useEffect(() => {
    if (!selectedPRNumber) {
      return
    }
    if (detail.status !== 'idle') {
      return
    }
    void fetchDetail(selectedPRNumber)
  }, [detail.status, fetchDetail, selectedPRNumber])

  useEffect(() => {
    if (!selectedPRNumber) {
      return
    }
    if (files.status === 'idle') {
      void fetchFiles({ prNumber: selectedPRNumber, page: 1, perPage: SECTION_PER_PAGE, append: false })
    }
    if (commits.status === 'idle') {
      void fetchCommits({ prNumber: selectedPRNumber, page: 1, perPage: SECTION_PER_PAGE, append: false })
    }
  }, [commits.status, fetchCommits, fetchFiles, files.status, selectedPRNumber])

  useEffect(() => {
    if (typeof document === 'undefined') {
      return
    }

    const syncVisibility = () => {
      const docVisible = document.visibilityState === 'visible'
      const hasFocus = typeof document.hasFocus === 'function' ? document.hasFocus() : true
      setIsWindowVisible(docVisible && hasFocus)
    }

    syncVisibility()
    document.addEventListener('visibilitychange', syncVisibility)
    window.addEventListener('focus', syncVisibility)
    window.addEventListener('blur', syncVisibility)

    return () => {
      document.removeEventListener('visibilitychange', syncVisibility)
      window.removeEventListener('focus', syncVisibility)
      window.removeEventListener('blur', syncVisibility)
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    if (!normalizedRepoPath) {
      setPollingTarget(null)
      return () => {
        cancelled = true
      }
    }

    void GitPanelPRResolveRepository(normalizedRepoPath, '', '', false)
      .then((target) => {
        if (cancelled) {
          return
        }
        const owner = (target?.owner || '').trim()
        const repo = (target?.repo || '').trim()
        if (!owner || !repo) {
          setPollingTarget(null)
          return
        }
        setPollingTarget({ owner, repo })
      })
      .catch(() => {
        if (!cancelled) {
          setPollingTarget(null)
        }
      })

    return () => {
      cancelled = true
    }
  }, [normalizedRepoPath])

  useEffect(() => {
    const owner = (pollingTarget?.owner || '').trim()
    const repo = (pollingTarget?.repo || '').trim()
    if (!owner || !repo) {
      void StopPolling().catch(() => { })
      return
    }

    if (!isWindowVisible) {
      void SetPollingContext('minimized').catch(() => { })
      void StopPolling().catch(() => { })
      return
    }

    void StartPolling(owner, repo).catch((err: unknown) => {
      console.error('[GitPanelPRView] Failed to start polling:', err)
    })

    return () => {
      void StopPolling().catch(() => { })
    }
  }, [isWindowVisible, pollingTarget?.owner, pollingTarget?.repo])

  useEffect(() => {
    if (!isWindowVisible || !pollingTarget?.owner || !pollingTarget?.repo) {
      return
    }

    const context: PollingContextType = selectedPRNumber ? 'pr_detail' : 'pr_list'
    void SetPollingContext(context).catch((err: unknown) => {
      console.error('[GitPanelPRView] Failed to update polling context:', err)
    })
  }, [isWindowVisible, pollingTarget?.owner, pollingTarget?.repo, selectedPRNumber])

  useEffect(() => {
    if (!hasRateLimitSignal) {
      return
    }

    void StopPolling().catch(() => { })
    const timer = window.setTimeout(() => {
      const owner = (pollingTarget?.owner || '').trim()
      const repo = (pollingTarget?.repo || '').trim()
      if (!isWindowVisible || !owner || !repo) {
        return
      }

      const context: PollingContextType = selectedPRNumber ? 'pr_detail' : 'pr_list'
      void StartPolling(owner, repo)
        .then(() => SetPollingContext(context))
        .catch((err: unknown) => {
          console.error('[GitPanelPRView] Failed to resume polling after rate limit:', err)
        })
    }, 60_000)

    return () => {
      window.clearTimeout(timer)
    }
  }, [hasRateLimitSignal, isWindowVisible, pollingTarget?.owner, pollingTarget?.repo, selectedPRNumber])

  useEffect(() => {
    if (!window.runtime) {
      return
    }

    const scheduleRefresh = () => {
      if (prsUpdatedRefreshTimerRef.current !== null) {
        return
      }
      prsUpdatedRefreshTimerRef.current = window.setTimeout(() => {
        prsUpdatedRefreshTimerRef.current = null
        const query = refreshListQueryRef.current
        const currentPRNumber = selectedPRNumberRef.current
        void fetchList({ state: query.state, page: query.page, perPage: query.perPage })
        if (currentPRNumber) {
          void fetchDetail(currentPRNumber)
        }
      }, PR_UPDATE_REFRESH_DEBOUNCE_MS)
    }

    const off = window.runtime.EventsOn('github:prs:updated', (eventPayload: unknown) => {
      if (!isWindowVisible || !normalizedRepoPath) {
        return
      }

      const eventTarget = resolvePRUpdateEventTarget(eventPayload)
      const currentOwner = (pollingTarget?.owner || '').trim().toLowerCase()
      const currentRepo = (pollingTarget?.repo || '').trim().toLowerCase()
      if (eventTarget && currentOwner && currentRepo) {
        const eventOwner = eventTarget.owner.toLowerCase()
        const eventRepo = eventTarget.repo.toLowerCase()
        if (eventOwner !== currentOwner || eventRepo !== currentRepo) {
          return
        }
      }

      scheduleRefresh()
    })

    return () => {
      if (prsUpdatedRefreshTimerRef.current !== null) {
        window.clearTimeout(prsUpdatedRefreshTimerRef.current)
        prsUpdatedRefreshTimerRef.current = null
      }
      off()
    }
  }, [
    fetchDetail,
    fetchList,
    isWindowVisible,
    normalizedRepoPath,
    pollingTarget?.owner,
    pollingTarget?.repo,
  ])

  useEffect(() => {
    const commitSha = selectedCommitSha.trim()
    if (!commitSha || rawDiff.status !== 'success' || !rawDiff.data) {
      return
    }

    const metrics = buildDiffMetrics(rawDiff.data)
    setCommitMetricsBySha((current) => {
      const existing = current[commitSha]
      if (
        existing
        && existing.additions === metrics.additions
        && existing.deletions === metrics.deletions
        && existing.changedFiles === metrics.changedFiles
      ) {
        return current
      }
      return {
        ...current,
        [commitSha]: metrics,
      }
    })
  }, [rawDiff.data, rawDiff.status, selectedCommitSha])

  const selectedListItem = useMemo(() => {
    if (!selectedPRNumber) {
      return null
    }
    return list.data.items.find((item) => item.number === selectedPRNumber) ?? null
  }, [list.data.items, selectedPRNumber])

  const normalizedListQuery = listQuery.trim()
  const filteredListItems = useMemo(() => {
    if (!normalizedListQuery) {
      return list.data.items
    }
    return list.data.items.filter((pr) => matchesPRListQuery(pr, normalizedListQuery))
  }, [list.data.items, normalizedListQuery])

  const listStatusSummary = useMemo(() => {
    const summary = {
      open: 0,
      closed: 0,
      merged: 0,
      draft: 0,
    }
    for (const item of filteredListItems) {
      const normalizedState = normalizeStateForClass(item.state)
      if (normalizedState === 'open') {
        summary.open++
      } else if (normalizedState === 'closed') {
        summary.closed++
      } else {
        summary.merged++
      }
      if (item.isDraft) {
        summary.draft++
      }
    }
    return summary
  }, [filteredListItems])

  const detailData = detail.data || selectedListItem
  const hasRepoPath = normalizedRepoPath !== ''
  const canOpenEditForm = selectedPRNumber !== null && detail.status === 'success' && detail.data !== null

  const ensureCreatedPRVisible = async (createdPR: GitPanelPullRequest) => {
    const createdPRNumber = normalizePRNumber(createdPR.number)
    if (!createdPRNumber) {
      return
    }

    const isPRVisible = () => {
      const current = useGitPanelPRStore.getState()
      return current.list.data.items.some((item) => item.number === createdPRNumber)
    }

    if (isPRVisible()) {
      return
    }

    const refreshDelays = [400, 1200]
    for (const delayMs of refreshDelays) {
      syncMutationResult(createdPR, { forceListState: 'open' })
      await wait(delayMs)
      await fetchList({ state: 'open', page: 1, perPage: list.data.perPage })
      if (isPRVisible()) {
        return
      }
    }

    syncMutationResult(createdPR, { forceListState: 'open' })
  }

  const handleFilterChange = (value: GitPanelPRListState) => {
    setListState(value)
  }

  const resolveCreatePRTarget = useCallback(async (form: CreatePRFormState): Promise<CreatePRResolvedTarget> => {
    const manualOwner = form.advancedMode ? form.targetOwner.trim() : ''
    const manualRepo = form.advancedMode ? form.targetRepo.trim() : ''
    const allowTargetOverride = form.advancedMode && form.confirmTargetOverride

    const target = await GitPanelPRResolveRepository(
      normalizedRepoPath,
      manualOwner,
      manualRepo,
      allowTargetOverride,
    )

    const owner = (target?.owner || '').trim()
    const repo = (target?.repo || '').trim()
    if (!owner || !repo) {
      throw {
        code: PR_ERROR_CODES.repoResolveFailed,
        message: 'Nao foi possivel resolver o repositorio destino da Pull Request.',
        details: 'Informe owner/repo validos no modo avancado ou valide o origin local.',
      } as PRBindingError
    }

    return {
      owner,
      repo,
    }
  }, [normalizedRepoPath])

  const getCreatePRTargetBranchNames = useCallback(async (owner: string, repo: string): Promise<Set<string>> => {
    const normalizedOwner = owner.trim()
    const normalizedRepo = repo.trim()
    if (!normalizedOwner || !normalizedRepo) {
      return new Set<string>()
    }

    const cacheKey = `${normalizedOwner.toLowerCase()}/${normalizedRepo.toLowerCase()}`
    const now = Date.now()
    const cached = createPRBranchCacheRef.current
    if (cached && cached.key === cacheKey && cached.expiresAt > now) {
      return cached.names
    }

    const response = await GHListBranches(normalizedOwner, normalizedRepo)
    const names = new Set<string>()
    for (const item of Array.isArray(response) ? response : []) {
      if (!item || typeof item !== 'object') {
        continue
      }
      const branchName = typeof (item as { name?: unknown }).name === 'string'
        ? ((item as { name: string }).name || '').trim()
        : ''
      if (branchName) {
        names.add(branchName)
      }
    }

    createPRBranchCacheRef.current = {
      key: cacheKey,
      names,
      expiresAt: now + CREATE_PR_BRANCH_CACHE_TTL_MS,
    }

    return names
  }, [])

  const refreshCreatePRContext = useCallback(async (formCandidate?: CreatePRFormState) => {
    if (!normalizedRepoPath) {
      return
    }

    const sourceForm = formCandidate ?? createPRForm
    setIsPreparingCreatePRContext(true)
    setCreatePRError(null)

    try {
      const [status, target] = await Promise.all([
        GitPanelGetStatus(normalizedRepoPath),
        resolveCreatePRTarget(sourceForm),
      ])

      const currentBranch = normalizeCreatePRBranchName(status?.branch)
      const ahead = normalizeCreatePRAhead(status?.ahead)
      const targetBranches = await getCreatePRTargetBranchNames(target.owner, target.repo)
      const baseOptions = buildCreatePRBaseOptions(targetBranches)

      setCreatePRCurrentBranch(currentBranch)
      setCreatePRAhead(ahead)
      setCreatePRTarget(target)
      setCreatePRBaseOptions(baseOptions)

      setCreatePRForm((current) => {
        const baseSource = formCandidate ? sourceForm : current
        const next = { ...baseSource }
        if (!next.advancedMode) {
          next.head = currentBranch
        }
        next.base = selectCreatePRBase(next.base, baseOptions)
        return next
      })
    } catch (err) {
      setCreatePRError(parsePRBindingError(err))
    } finally {
      setIsPreparingCreatePRContext(false)
    }
  }, [createPRForm, getCreatePRTargetBranchNames, normalizedRepoPath, resolveCreatePRTarget])

  const openCreateForm = () => {
    if (isCreatingLabel) {
      return
    }
    const initialForm = createDefaultPRFormState()
    setCreatePRForm(initialForm)
    setCreatePRValidationErrors({})
    setCreatePRError(null)
    setCreatePRSubmitMode('create')
    setCreatePRCurrentBranch('')
    setCreatePRAhead(null)
    setCreatePRTarget(null)
    setCreatePRBaseOptions([CREATE_PR_DEFAULT_BASE])
    setIsCreateLabelFormOpen(false)
    setIsCreatingLabel(false)
    setCreateLabelForm(createDefaultLabelFormState())
    setCreateLabelValidationErrors({})
    setCreateLabelError(null)
    setCreateLabelSuccess('')
    setIsCreateFormOpen(true)
    void refreshCreatePRContext(initialForm)
  }

  const closeCreateForm = () => {
    if (isCreatingPR || isPreparingCreatePRContext) {
      return
    }
    setIsCreateFormOpen(false)
    setIsPreparingCreatePRContext(false)
    setCreatePRForm(createDefaultPRFormState())
    setCreatePRValidationErrors({})
    setCreatePRError(null)
    setCreatePRSubmitMode('create')
    setCreatePRCurrentBranch('')
    setCreatePRAhead(null)
    setCreatePRTarget(null)
    setCreatePRBaseOptions([CREATE_PR_DEFAULT_BASE])
  }

  const openCreateLabelForm = () => {
    if (!hasRepoPath || isCreatingPR || isPreparingCreatePRContext) {
      return
    }

    setIsCreateFormOpen(false)
    setIsPreparingCreatePRContext(false)
    setCreatePRSubmitMode('create')
    setCreatePRError(null)
    setCreatePRValidationErrors({})
    setCreatePRCurrentBranch('')
    setCreatePRAhead(null)
    setCreatePRTarget(null)
    setCreatePRBaseOptions([CREATE_PR_DEFAULT_BASE])

    setCreateLabelForm(createDefaultLabelFormState())
    setCreateLabelValidationErrors({})
    setCreateLabelError(null)
    setCreateLabelSuccess('')
    setIsCreateLabelFormOpen(true)
  }

  const closeCreateLabelForm = () => {
    if (isCreatingLabel) {
      return
    }
    setIsCreateLabelFormOpen(false)
    setCreateLabelForm(createDefaultLabelFormState())
    setCreateLabelValidationErrors({})
    setCreateLabelError(null)
    setCreateLabelSuccess('')
  }

  const handleCreateLabelFieldChange = (field: CreateLabelValidationField, value: string) => {
    setCreateLabelForm((current) => ({
      ...current,
      [field]: value,
    }))
    setCreateLabelValidationErrors((current) => {
      if (!current[field]) {
        return current
      }
      const next = { ...current }
      delete next[field]
      return next
    })
    if (createLabelError) {
      setCreateLabelError(null)
    }
    if (createLabelSuccess) {
      setCreateLabelSuccess('')
    }
  }

  const handleCreateLabelSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!hasRepoPath || isCreatingLabel) {
      return
    }

    const validationErrors = validateCreateLabelForm(createLabelForm)
    setCreateLabelValidationErrors(validationErrors)
    if (Object.keys(validationErrors).length > 0) {
      return
    }

    const normalizedColor = normalizeCreateLabelColor(createLabelForm.color)
    const normalizedDescription = createLabelForm.description.trim()

    setIsCreatingLabel(true)
    setCreateLabelError(null)
    setCreateLabelSuccess('')
    try {
      const createdLabel = await GitPanelPRCreateLabel(normalizedRepoPath, {
        name: createLabelForm.name.trim(),
        color: normalizedColor,
        description: normalizedDescription || undefined,
      })
      const createdLabelName = (createdLabel?.name || '').trim() || createLabelForm.name.trim()
      setCreateLabelForm(createDefaultLabelFormState())
      setCreateLabelValidationErrors({})
      setCreateLabelSuccess(`Etiqueta "${createdLabelName}" criada.`)
    } catch (err) {
      const parsedError = parsePRBindingError(err)
      console.error('[GitPanelPRView] Falha ao criar etiqueta:', {
        code: parsedError.code,
        message: parsedError.message,
        details: parsedError.details,
        raw: err,
      })
      setCreateLabelError(parsedError)
    } finally {
      setIsCreatingLabel(false)
    }
  }

  const handleCreatePRFieldChange = (field: CreatePRRequiredField, value: string) => {
    setCreatePRForm((current) => ({
      ...current,
      [field]: value,
    }))
    setCreatePRValidationErrors((current) => {
      if (!current[field]) {
        return current
      }
      const next = { ...current }
      delete next[field]
      return next
    })
    if (createPRError) {
      setCreatePRError(null)
    }
  }

  const handleCreatePRAdvancedModeChange = (enabled: boolean) => {
    const nextForm = enabled
      ? {
        ...createPRForm,
        advancedMode: true,
        head: createPRForm.head || createPRCurrentBranch,
      }
      : {
        ...createPRForm,
        advancedMode: false,
        head: createPRCurrentBranch || createPRForm.head,
        targetOwner: '',
        targetRepo: '',
        confirmTargetOverride: false,
      }

    setCreatePRForm(nextForm)
    setCreatePRValidationErrors((current) => {
      const next = { ...current }
      delete next.targetOwner
      delete next.targetRepo
      return next
    })
    if (createPRError) {
      setCreatePRError(null)
    }
    void refreshCreatePRContext(nextForm)
  }

  const handleRefreshCreatePRContext = () => {
    if (!isCreateFormOpen || isCreatingPR) {
      return
    }
    void refreshCreatePRContext()
  }

  const handleCreatePRSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!hasRepoPath || isCreatingPR || isPreparingCreatePRContext) {
      return
    }

    const initialValidationErrors = validateCreatePRForm(createPRForm)
    setCreatePRValidationErrors(initialValidationErrors)
    if (Object.keys(initialValidationErrors).length > 0) {
      return
    }

    setIsCreatingPR(true)
    setCreatePRSubmitMode('create')
    setCreatePRError(null)
    try {
      const [status, target] = await Promise.all([
        GitPanelGetStatus(normalizedRepoPath),
        resolveCreatePRTarget(createPRForm),
      ])

      const currentBranch = normalizeCreatePRBranchName(status?.branch)
      const localAhead = normalizeCreatePRAhead(status?.ahead)
      const normalizedHead = createPRForm.advancedMode ? createPRForm.head.trim() : currentBranch
      const normalizedBase = createPRForm.base.trim()
      const parsedHead = parseCreatePRHeadReference(normalizedHead)
      const remoteBranches = await getCreatePRTargetBranchNames(target.owner, target.repo)
      const baseOptions = buildCreatePRBaseOptions(remoteBranches)

      setCreatePRCurrentBranch(currentBranch)
      setCreatePRAhead(localAhead)
      setCreatePRTarget(target)
      setCreatePRBaseOptions(baseOptions)
      if (!createPRForm.advancedMode) {
        setCreatePRForm((current) => ({ ...current, head: currentBranch }))
      }

      const preflightErrors: CreatePRValidationErrors = {}
      if (!parsedHead.valid) {
        preflightErrors.head = parsedHead.errorMessage || 'Head obrigatoria.'
      }
      if (!normalizedBase) {
        preflightErrors.base = 'Base obrigatoria.'
      } else if (!remoteBranches.has(normalizedBase)) {
        preflightErrors.base = `Base "${normalizedBase}" nao encontrada em ${target.owner}/${target.repo}.`
      }

      const shouldValidateHeadAgainstTarget = parsedHead.valid
        && (!parsedHead.hasOwner || parsedHead.owner.toLowerCase() === target.owner.toLowerCase())
      if (shouldValidateHeadAgainstTarget) {
        if (localAhead <= 0) {
          preflightErrors.head = 'sem commits para abrir PR'
        }
        if (normalizedBase && parsedHead.branch.toLowerCase() === normalizedBase.toLowerCase()) {
          preflightErrors.head = 'Head e base nao podem ser iguais.'
        }
      }

      if (Object.keys(preflightErrors).length > 0) {
        setCreatePRValidationErrors((current) => ({ ...current, ...preflightErrors }))
        return
      }

      const shouldPushHeadBranch = shouldValidateHeadAgainstTarget
        && parsedHead.valid
        && !remoteBranches.has(parsedHead.branch)

      if (shouldPushHeadBranch) {
        setCreatePRSubmitMode('push_and_create')
        await GitPanelPRPushLocalBranch(
          normalizedRepoPath,
          parsedHead.branch,
        )
      }

      const created = await GitPanelPRCreate(normalizedRepoPath, {
        title: createPRForm.title.trim(),
        head: normalizedHead,
        base: normalizedBase,
        body: createPRForm.body.trim() || undefined,
        draft: createPRForm.draft,
        maintainerCanModify: createPRForm.maintainerCanModify,
        manualOwner: createPRForm.advancedMode ? (createPRForm.targetOwner.trim() || undefined) : undefined,
        manualRepo: createPRForm.advancedMode ? (createPRForm.targetRepo.trim() || undefined) : undefined,
        allowTargetOverride: createPRForm.advancedMode ? createPRForm.confirmTargetOverride : undefined,
      })

      const createdPR = created as GitPanelPullRequest
      const createdPRNumber = normalizePRNumber(createdPR?.number)
      setActiveSection('conversation')

      if (createdPRNumber) {
        syncMutationResult(createdPR, {
          select: true,
          forceListState: 'open',
          resetSections: true,
        })

        await Promise.all([
          fetchList({ state: 'open', page: 1, perPage: list.data.perPage }),
          fetchDetail(createdPRNumber),
        ])
        await ensureCreatedPRVisible(createdPR)
      } else {
        setListState('open')
        await fetchList({ state: 'open', page: 1, perPage: list.data.perPage })
      }

      setIsCreateFormOpen(false)
      setCreatePRForm(createDefaultPRFormState())
      setCreatePRValidationErrors({})
      setCreatePRCurrentBranch('')
      setCreatePRAhead(null)
      setCreatePRTarget(null)
      setCreatePRBaseOptions([CREATE_PR_DEFAULT_BASE])
    } catch (err) {
      const parsedError = parsePRBindingError(err)
      console.error('[GitPanelPRView] Falha ao criar PR:', {
        code: parsedError.code,
        message: parsedError.message,
        details: parsedError.details,
        raw: err,
      })
      setCreatePRError(parsedError)
    } finally {
      setIsCreatingPR(false)
      setCreatePRSubmitMode('create')
    }
  }

  const openEditForm = () => {
    if (!canOpenEditForm || !detail.data) {
      return
    }
    setEditPRForm(createEditPRFormState(detail.data))
    setEditPRValidationErrors({})
    setEditPRError(null)
    setIsEditFormOpen(true)
  }

  const closeEditForm = () => {
    if (isUpdatingPR) {
      return
    }
    setIsEditFormOpen(false)
    setIsUpdatingPR(false)
    setEditPRForm(null)
    setEditPRValidationErrors({})
    setEditPRError(null)
  }

  const handleEditPRRequiredFieldChange = (field: EditPRRequiredField, value: string) => {
    setEditPRForm((current) => {
      if (!current) {
        return current
      }
      return {
        ...current,
        [field]: value,
      }
    })

    setEditPRValidationErrors((current) => {
      if (!current[field]) {
        return current
      }
      const next = { ...current }
      delete next[field]
      return next
    })

    if (editPRError) {
      setEditPRError(null)
    }
  }

  const handleEditPRBodyChange = (value: string) => {
    setEditPRForm((current) => (current ? { ...current, body: value } : current))
    if (editPRError) {
      setEditPRError(null)
    }
  }

  const handleEditPRStateChange = (value: EditPRWritableState) => {
    setEditPRForm((current) => (current ? { ...current, state: value } : current))
    if (editPRError) {
      setEditPRError(null)
    }
  }

  const handleEditPRMaintainerCanModifyChange = (value: boolean) => {
    setEditPRForm((current) => (current ? { ...current, maintainerCanModify: value } : current))
    if (editPRError) {
      setEditPRError(null)
    }
  }

  const handleEditPRSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (!hasRepoPath || !selectedPRNumber || isUpdatingPR || !editPRForm) {
      return
    }

    const validationErrors = validateEditPRForm(editPRForm)
    setEditPRValidationErrors(validationErrors)
    if (Object.keys(validationErrors).length > 0) {
      return
    }

    const currentPRNumber = selectedPRNumber
    setIsUpdatingPR(true)
    setEditPRError(null)

    try {
      const updated = await GitPanelPRUpdate(normalizedRepoPath, currentPRNumber, {
        title: editPRForm.title.trim(),
        body: editPRForm.body,
        base: editPRForm.base.trim(),
        state: editPRForm.state,
        maintainerCanModify: editPRForm.maintainerCanModify,
      })

      const updatedPR = updated as GitPanelPullRequest
      const updatedPRNumber = normalizePRNumber(updatedPR.number)
      if (updatedPRNumber) {
        syncMutationResult(updatedPR, { select: updatedPRNumber === currentPRNumber })
      }

      await Promise.all([
        fetchList({ state: listState, page: list.data.page, perPage: list.data.perPage }),
        fetchDetail(currentPRNumber),
      ])

      setIsEditFormOpen(false)
      setEditPRForm(null)
      setEditPRValidationErrors({})
      setEditPRError(null)
    } catch (err) {
      const parsedError = parsePRBindingError(err)
      console.error('[GitPanelPRView] Falha ao atualizar PR:', {
        code: parsedError.code,
        message: parsedError.message,
        details: parsedError.details,
        raw: err,
      })
      setEditPRError(parsedError)
    } finally {
      setIsUpdatingPR(false)
    }
  }

  const handleSectionChange = useCallback((section: PRRightSection) => {
    setActiveSection(section)
    setListPanelWidth((current) => Math.min(current, DEFAULT_LIST_PANEL_WIDTH))
  }, [])

  const handleSelectPR = useCallback((prNumber: number) => {
    handleSectionChange('conversation')
    selectPR(prNumber)
    void fetchDetail(prNumber)
  }, [fetchDetail, handleSectionChange, selectPR])

  const handleReloadList = () => {
    void fetchList({ state: listState, page: list.data.page, perPage: list.data.perPage })
  }

  const handlePrevListPage = () => {
    if (list.data.page <= 1) {
      return
    }
    void fetchList({ state: listState, page: list.data.page - 1, perPage: list.data.perPage })
  }

  const handleNextListPage = () => {
    if (list.data.items.length < list.data.perPage) {
      return
    }
    void fetchList({ state: listState, page: list.data.page + 1, perPage: list.data.perPage })
  }

  const handleRefreshDetail = () => {
    if (!selectedPRNumber) {
      return
    }
    void fetchDetail(selectedPRNumber)
  }

  const handleLoadMoreFiles = () => {
    if (!selectedPRNumber || !files.data?.hasNextPage || !files.data?.nextPage) {
      return
    }
    void fetchFiles({
      prNumber: selectedPRNumber,
      page: files.data.nextPage,
      perPage: files.data.perPage,
      append: true,
    })
  }

  const handleLoadMoreCommits = () => {
    if (!selectedPRNumber || !commits.data?.hasNextPage || !commits.data?.nextPage) {
      return
    }
    void fetchCommits({
      prNumber: selectedPRNumber,
      page: commits.data.nextPage,
      perPage: commits.data.perPage,
      append: true,
    })
  }

  const handleLoadRawDiff = () => {
    if (!selectedPRNumber) {
      return
    }
    setSelectedCommitSha('')
    void fetchRawDiff(selectedPRNumber)
  }

  const handleSelectCommitDiff = (commit: GitPanelPRCommit) => {
    if (!selectedPRNumber) {
      return
    }
    const normalizedSHA = (commit.sha || '').trim()
    if (!normalizedSHA) {
      return
    }
    setSelectedCommitSha(normalizedSHA)
    handleSectionChange('commits')
    void fetchRawDiff(selectedPRNumber, normalizedSHA)
  }

  const handleColumnResizeStart = useCallback((event: PointerEvent<HTMLDivElement>) => {
    const container = splitResizeContainerRef.current
    if (!container) {
      return
    }

    const containerRect = container.getBoundingClientRect()
    splitResizeSessionRef.current = {
      startX: event.clientX,
      startWidthPx: (listPanelWidth / 100) * containerRect.width,
      containerWidthPx: containerRect.width,
    }
    setIsResizingColumns(true)
    event.currentTarget.setPointerCapture(event.pointerId)
    event.preventDefault()
  }, [listPanelWidth])

  useEffect(() => {
    if (!isResizingColumns) {
      return
    }

    const handlePointerMove = (event: globalThis.PointerEvent) => {
      const session = splitResizeSessionRef.current
      if (!session || session.containerWidthPx <= 0) {
        return
      }

      const deltaX = event.clientX - session.startX
      const nextWidthPx = session.startWidthPx + deltaX
      const nextPercent = (nextWidthPx / session.containerWidthPx) * 100
      setListPanelWidth(clamp(nextPercent, MIN_LIST_PANEL_WIDTH, MAX_LIST_PANEL_WIDTH))
    }

    const stopResize = () => {
      splitResizeSessionRef.current = null
      setIsResizingColumns(false)
    }

    window.addEventListener('pointermove', handlePointerMove)
    window.addEventListener('pointerup', stopResize)
    window.addEventListener('pointercancel', stopResize)

    return () => {
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', stopResize)
      window.removeEventListener('pointercancel', stopResize)
    }
  }, [isResizingColumns])

  const handleColumnResizeKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === 'ArrowLeft') {
      event.preventDefault()
      setListPanelWidth((current) => clamp(current - 2, MIN_LIST_PANEL_WIDTH, MAX_LIST_PANEL_WIDTH))
      return
    }
    if (event.key === 'ArrowRight') {
      event.preventDefault()
      setListPanelWidth((current) => clamp(current + 2, MIN_LIST_PANEL_WIDTH, MAX_LIST_PANEL_WIDTH))
    }
  }

  const layoutStyle = useMemo(() => {
    return { '--pr-list-width': `${listPanelWidth}%` } as CSSProperties
  }, [listPanelWidth])

  return (
    <section
      ref={splitResizeContainerRef}
      className={`git-panel-prs ${isResizingColumns ? 'git-panel-prs--resizing' : ''}`}
      style={layoutStyle}
      aria-label="Git Panel Pull Requests"
    >
      <aside className="git-panel-prs__column git-panel-prs__column--list" aria-label="Lista de Pull Requests">
        <header className="git-panel-prs__column-header">
          <h2>
            <GitPullRequest size={14} />
            Pull Requests
            <span className="badge badge--info">{filteredListItems.length}</span>
          </h2>
          <span className="git-panel-prs__header-caption">Página {list.data.page}</span>
        </header>

        <div className="git-panel-prs__list-toolbar">
          <label className="git-panel-prs__search" htmlFor="git-panel-pr-list-search">
            <Search size={14} className="git-panel-prs__search-icon" />
            <input
              id="git-panel-pr-list-search"
              type="search"
              className="git-panel-prs__search-input"
              value={listQuery}
              onChange={(event) => setListQuery(event.target.value)}
              placeholder="Search pull requests..."
              autoComplete="off"
              spellCheck={false}
            />
          </label>
          <button type="button" className="btn btn--ghost git-panel-prs__refresh-btn" onClick={handleReloadList}>
            <RefreshCcw size={12} />
            Atualizar
          </button>
        </div>

        <div className="git-panel-prs__filters">
          {FILTERS.map((filter) => (
            <button
              key={filter.value}
              type="button"
              className={`git-panel-prs__filter-btn ${listState === filter.value ? 'git-panel-prs__filter-btn--active' : ''}`}
              onClick={() => handleFilterChange(filter.value)}
            >
              {filter.label}
            </button>
          ))}
        </div>

        {hasRepoPath && (
          <div className="git-panel-prs__list-summary">
            <span>{filteredListItems.length} visível(is)</span>
            <span>{listStatusSummary.open} open</span>
            <span>{listStatusSummary.closed} closed</span>
            <span>{listStatusSummary.merged} merged</span>
            {listStatusSummary.draft > 0 && <span>{listStatusSummary.draft} draft</span>}
          </div>
        )}

        <div className="git-panel-prs__create">
          <div className="git-panel-prs__quick-actions">
            <button
              type="button"
              className="btn btn--primary git-panel-prs__create-toggle"
              onClick={isCreateFormOpen ? closeCreateForm : openCreateForm}
              disabled={!hasRepoPath || isCreatingPR || isPreparingCreatePRContext || isCreatingLabel}
            >
              {isCreateFormOpen ? 'Fechar PR' : 'Nova PR'}
            </button>
            <button
              type="button"
              className="btn btn--ghost git-panel-prs__create-toggle"
              onClick={isCreateLabelFormOpen ? closeCreateLabelForm : openCreateLabelForm}
              disabled={!hasRepoPath || isCreatingPR || isPreparingCreatePRContext || isCreatingLabel}
            >
              {isCreateLabelFormOpen ? 'Fechar etiqueta' : 'Nova etiqueta'}
            </button>
          </div>

          {isCreateFormOpen && (
            <form className="git-panel-prs__create-form" onSubmit={handleCreatePRSubmit} noValidate>
              <div className="git-panel-prs__form-field">
                <label htmlFor="git-panel-pr-create-title">Titulo *</label>
                <input
                  id="git-panel-pr-create-title"
                  type="text"
                  className={`input ${createPRValidationErrors.title ? 'git-panel-prs__form-input--error' : ''}`}
                  value={createPRForm.title}
                  onChange={(event) => handleCreatePRFieldChange('title', event.target.value)}
                  placeholder="Ex: feat: adicionar fluxo de pagamento"
                  aria-invalid={Boolean(createPRValidationErrors.title)}
                />
                {createPRValidationErrors.title && (
                  <small className="git-panel-prs__form-error">{createPRValidationErrors.title}</small>
                )}
              </div>

              <div className="git-panel-prs__create-toolbar">
                <label className="git-panel-prs__checkbox">
                  <input
                    type="checkbox"
                    checked={createPRForm.advancedMode}
                    onChange={(event) => handleCreatePRAdvancedModeChange(event.target.checked)}
                    disabled={isCreatingPR || isPreparingCreatePRContext}
                  />
                  Modo avancado (fork + override manual)
                </label>
                <button
                  type="button"
                  className="btn btn--ghost git-panel-prs__refresh-btn"
                  onClick={handleRefreshCreatePRContext}
                  disabled={isCreatingPR || isPreparingCreatePRContext}
                >
                  {isPreparingCreatePRContext ? 'Atualizando...' : 'Atualizar contexto'}
                </button>
              </div>

              <div className="git-panel-prs__create-context" role="status">
                <span>Branch atual: <code>{createPRCurrentBranch || '-'}</code></span>
                <span>Ahead: {createPRAhead ?? '-'}</span>
                <span>Destino: {createPRTarget ? `${createPRTarget.owner}/${createPRTarget.repo}` : '-'}</span>
              </div>

              <div className="git-panel-prs__form-field">
                <label htmlFor="git-panel-pr-create-head">
                  {createPRForm.advancedMode ? 'Head *' : 'Head (branch atual) *'}
                </label>
                <input
                  id="git-panel-pr-create-head"
                  type="text"
                  className={`input ${createPRValidationErrors.head ? 'git-panel-prs__form-input--error' : ''}`}
                  value={createPRForm.head}
                  onChange={(event) => handleCreatePRFieldChange('head', event.target.value)}
                  placeholder={createPRForm.advancedMode ? 'Ex: feature/pagamentos ou owner:feature/pagamentos' : 'Branch atual'}
                  readOnly={!createPRForm.advancedMode}
                  aria-invalid={Boolean(createPRValidationErrors.head)}
                  disabled={isCreatingPR || isPreparingCreatePRContext}
                />
                {createPRValidationErrors.head && (
                  <small className="git-panel-prs__form-error">{createPRValidationErrors.head}</small>
                )}
                <small className="git-panel-prs__form-hint">
                  {createPRForm.advancedMode
                    ? 'Modo avancado: use "branch" ou "owner:branch" para fork.'
                    : 'Head automatico da branch atual (read-only).'}
                </small>
              </div>

              <div className="git-panel-prs__form-field">
                <label htmlFor="git-panel-pr-create-base">Base *</label>
                <select
                  id="git-panel-pr-create-base"
                  className={`input ${createPRValidationErrors.base ? 'git-panel-prs__form-input--error' : ''}`}
                  value={createPRForm.base}
                  onChange={(event) => handleCreatePRFieldChange('base', event.target.value)}
                  aria-invalid={Boolean(createPRValidationErrors.base)}
                  disabled={isCreatingPR || isPreparingCreatePRContext}
                >
                  {createPRBaseOptions.map((branch) => (
                    <option key={branch} value={branch}>
                      {branch}
                    </option>
                  ))}
                </select>
                {createPRValidationErrors.base && (
                  <small className="git-panel-prs__form-error">{createPRValidationErrors.base}</small>
                )}
                <small className="git-panel-prs__form-hint">
                  Base default em <code>{CREATE_PR_DEFAULT_BASE}</code>, carregada via dropdown do repositório alvo.
                </small>
              </div>

              {createPRForm.advancedMode && (
                <>
                  <div className="git-panel-prs__form-field">
                    <label htmlFor="git-panel-pr-create-target-owner">Owner destino (override opcional)</label>
                    <input
                      id="git-panel-pr-create-target-owner"
                      type="text"
                      className={`input ${createPRValidationErrors.targetOwner ? 'git-panel-prs__form-input--error' : ''}`}
                      value={createPRForm.targetOwner}
                      onChange={(event) => handleCreatePRFieldChange('targetOwner', event.target.value)}
                      placeholder="Ex: acme"
                      aria-invalid={Boolean(createPRValidationErrors.targetOwner)}
                      disabled={isCreatingPR || isPreparingCreatePRContext}
                    />
                    {createPRValidationErrors.targetOwner && (
                      <small className="git-panel-prs__form-error">{createPRValidationErrors.targetOwner}</small>
                    )}
                  </div>

                  <div className="git-panel-prs__form-field">
                    <label htmlFor="git-panel-pr-create-target-repo">Repo destino (override opcional)</label>
                    <input
                      id="git-panel-pr-create-target-repo"
                      type="text"
                      className={`input ${createPRValidationErrors.targetRepo ? 'git-panel-prs__form-input--error' : ''}`}
                      value={createPRForm.targetRepo}
                      onChange={(event) => handleCreatePRFieldChange('targetRepo', event.target.value)}
                      placeholder="Ex: orch"
                      aria-invalid={Boolean(createPRValidationErrors.targetRepo)}
                      disabled={isCreatingPR || isPreparingCreatePRContext}
                    />
                    {createPRValidationErrors.targetRepo && (
                      <small className="git-panel-prs__form-error">{createPRValidationErrors.targetRepo}</small>
                    )}
                  </div>

                  <label className="git-panel-prs__checkbox">
                    <input
                      type="checkbox"
                      checked={createPRForm.confirmTargetOverride}
                      onChange={(event) => setCreatePRForm((current) => ({
                        ...current,
                        confirmTargetOverride: event.target.checked,
                      }))}
                      disabled={isCreatingPR || isPreparingCreatePRContext}
                    />
                    Confirmar override manual de owner/repo
                  </label>
                </>
              )}

              <div className="git-panel-prs__form-field">
                <label htmlFor="git-panel-pr-create-body">Descricao (opcional)</label>
                <textarea
                  id="git-panel-pr-create-body"
                  className="git-panel-prs__textarea"
                  value={createPRForm.body}
                  onChange={(event) => setCreatePRForm((current) => ({ ...current, body: event.target.value }))}
                  placeholder="Contexto da PR, escopo, riscos e validacoes."
                  rows={4}
                  disabled={isCreatingPR}
                />
              </div>

              <label className="git-panel-prs__checkbox">
                <input
                  type="checkbox"
                  checked={createPRForm.draft}
                  onChange={(event) => setCreatePRForm((current) => ({ ...current, draft: event.target.checked }))}
                  disabled={isCreatingPR}
                />
                Criar como draft
              </label>

              <label className="git-panel-prs__checkbox">
                <input
                  type="checkbox"
                  checked={createPRForm.maintainerCanModify}
                  onChange={(event) => setCreatePRForm((current) => ({ ...current, maintainerCanModify: event.target.checked }))}
                  disabled={isCreatingPR}
                />
                Permitir alteracoes por mantenedores
              </label>

              {createPRError && (
                <div className="git-panel-prs__create-error" role="alert">
                  <strong>Falha ao criar PR</strong>
                  <p>{createPRError.message}</p>
                  {createPRError.details && <small>{createPRError.details}</small>}
                </div>
              )}

              <div className="git-panel-prs__create-actions">
                <button type="button" className="btn btn--ghost" onClick={closeCreateForm} disabled={isCreatingPR || isPreparingCreatePRContext}>
                  Cancelar
                </button>
                <button type="submit" className="btn btn--primary" disabled={isCreatingPR || isPreparingCreatePRContext}>
                  {isCreatingPR
                    ? (createPRSubmitMode === 'push_and_create' ? 'Publicando branch e criando PR...' : 'Criando PR...')
                    : 'Criar PR'}
                </button>
              </div>
            </form>
          )}

          {isCreateLabelFormOpen && (
            <form className="git-panel-prs__label-form" onSubmit={handleCreateLabelSubmit} noValidate>
              <div className="git-panel-prs__form-field">
                <label htmlFor="git-panel-pr-create-label-name">Nome *</label>
                <input
                  id="git-panel-pr-create-label-name"
                  type="text"
                  className={`input ${createLabelValidationErrors.name ? 'git-panel-prs__form-input--error' : ''}`}
                  value={createLabelForm.name}
                  onChange={(event) => handleCreateLabelFieldChange('name', event.target.value)}
                  placeholder="Ex: prioridade/P0"
                  aria-invalid={Boolean(createLabelValidationErrors.name)}
                  disabled={isCreatingLabel}
                />
                {createLabelValidationErrors.name && (
                  <small className="git-panel-prs__form-error">{createLabelValidationErrors.name}</small>
                )}
              </div>

              <div className="git-panel-prs__form-field">
                <label htmlFor="git-panel-pr-create-label-color-value">Cor *</label>
                <div className="git-panel-prs__label-color-row">
                  <input
                    id="git-panel-pr-create-label-color-picker"
                    type="color"
                    className="git-panel-prs__color-picker"
                    aria-label="Seletor de cor da etiqueta"
                    value={resolveCreateLabelPickerColor(createLabelForm.color)}
                    onChange={(event) => handleCreateLabelFieldChange('color', event.target.value)}
                    disabled={isCreatingLabel}
                  />
                  <input
                    id="git-panel-pr-create-label-color-value"
                    type="text"
                    className={`input ${createLabelValidationErrors.color ? 'git-panel-prs__form-input--error' : ''}`}
                    value={createLabelForm.color}
                    onChange={(event) => handleCreateLabelFieldChange('color', event.target.value)}
                    placeholder="#0e8a16"
                    autoComplete="off"
                    spellCheck={false}
                    aria-invalid={Boolean(createLabelValidationErrors.color)}
                    disabled={isCreatingLabel}
                  />
                </div>
                {createLabelValidationErrors.color && (
                  <small className="git-panel-prs__form-error">{createLabelValidationErrors.color}</small>
                )}
                <small className="git-panel-prs__form-hint">Formato hexadecimal de 6 caracteres (ex: #d73a4a).</small>
              </div>

              <div className="git-panel-prs__form-field">
                <label htmlFor="git-panel-pr-create-label-description">Descricao (opcional)</label>
                <textarea
                  id="git-panel-pr-create-label-description"
                  className={`git-panel-prs__textarea ${createLabelValidationErrors.description ? 'git-panel-prs__form-input--error' : ''}`}
                  value={createLabelForm.description}
                  onChange={(event) => handleCreateLabelFieldChange('description', event.target.value)}
                  placeholder="Contexto curto para facilitar uso em PRs e issues."
                  rows={3}
                  disabled={isCreatingLabel}
                  maxLength={CREATE_LABEL_MAX_DESCRIPTION_CHARS}
                />
                {createLabelValidationErrors.description && (
                  <small className="git-panel-prs__form-error">{createLabelValidationErrors.description}</small>
                )}
                <small className="git-panel-prs__form-hint">
                  {createLabelForm.description.trim().length}/{CREATE_LABEL_MAX_DESCRIPTION_CHARS}
                </small>
              </div>

              {createLabelError && (
                <div className="git-panel-prs__create-error" role="alert">
                  <strong>Falha ao criar etiqueta</strong>
                  <p>{createLabelError.message}</p>
                  {createLabelError.details && <small>{createLabelError.details}</small>}
                </div>
              )}

              {createLabelSuccess && (
                <div className="git-panel-prs__create-success" role="status">
                  <strong>Etiqueta criada</strong>
                  <p>{createLabelSuccess}</p>
                </div>
              )}

              <div className="git-panel-prs__create-actions">
                <button type="button" className="btn btn--ghost" onClick={closeCreateLabelForm} disabled={isCreatingLabel}>
                  Cancelar
                </button>
                <button type="submit" className="btn btn--primary" disabled={isCreatingLabel}>
                  {isCreatingLabel ? 'Criando etiqueta...' : 'Criar etiqueta'}
                </button>
              </div>
            </form>
          )}
        </div>

        {!hasRepoPath && (
          <div className="git-panel-prs__empty-state">
            <AlertTriangle size={14} />
            <p>Selecione um repositório Git para listar Pull Requests.</p>
          </div>
        )}

        {hasRepoPath && list.status === 'error' && (
          <BlockError
            title="Falha ao carregar lista de PRs"
            error={list.error}
            onRetry={handleReloadList}
          />
        )}

        {hasRepoPath && list.status === 'loading' && list.data.items.length === 0 && (
          <div className="git-panel-prs__loading">
            <Loader2 size={14} className="git-panel-prs__spinner" />
            <span>Carregando Pull Requests...</span>
          </div>
        )}

        {hasRepoPath && list.data.items.length === 0 && list.status !== 'loading' && list.status !== 'error' && (
          <div className="git-panel-prs__empty-state">
            <GitPullRequest size={14} />
            <p>Nenhuma Pull Request encontrada neste filtro.</p>
          </div>
        )}

        {hasRepoPath && list.data.items.length > 0 && filteredListItems.length === 0 && (
          <div className="git-panel-prs__empty-state">
            <GitPullRequest size={14} />
            <p>Nenhuma PR corresponde à busca atual.</p>
          </div>
        )}

        {hasRepoPath && filteredListItems.length > 0 && (
          <ul className="git-panel-prs__list" role="listbox" aria-label="Pull Requests">
            {filteredListItems.map((pr) => (
              <PRListItem
                key={pr.number}
                pr={pr}
                selected={selectedPRNumber === pr.number}
                onSelect={handleSelectPR}
              />
            ))}
          </ul>
        )}

        {hasRepoPath && (
          <footer className="git-panel-prs__list-footer">
            <button type="button" className="btn btn--ghost" onClick={handlePrevListPage} disabled={list.data.page <= 1}>
              Anterior
            </button>
            <span>Página {list.data.page}</span>
            <button
              type="button"
              className="btn btn--ghost"
              onClick={handleNextListPage}
              disabled={list.data.items.length < list.data.perPage}
            >
              Próxima
            </button>
          </footer>
        )}
      </aside>

      <div
        className="git-panel-prs__column-resizer"
        role="separator"
        aria-orientation="vertical"
        aria-label="Redimensionar largura da lista de Pull Requests"
        aria-valuemin={MIN_LIST_PANEL_WIDTH}
        aria-valuemax={MAX_LIST_PANEL_WIDTH}
        aria-valuenow={Math.round(listPanelWidth)}
        tabIndex={0}
        onPointerDown={handleColumnResizeStart}
        onKeyDown={handleColumnResizeKeyDown}
      />

      <section className="git-panel-prs__column git-panel-prs__column--main" aria-label="Detalhe da Pull Request">
        <header className="git-panel-prs__column-header">
          <h2>Pull Request</h2>
          <div className="git-panel-prs__detail-header-actions">
            <button
              type="button"
              className="btn btn--ghost git-panel-prs__refresh-btn"
              onClick={openEditForm}
              disabled={!canOpenEditForm || isEditFormOpen || isUpdatingPR}
            >
              Editar
            </button>
            <button
              type="button"
              className="btn btn--ghost git-panel-prs__refresh-btn"
              onClick={handleRefreshDetail}
              disabled={!selectedPRNumber || isUpdatingPR}
            >
              <RefreshCcw size={12} />
              Atualizar
            </button>
          </div>
        </header>

        <div className="git-panel-prs__main-scroll">
          {!selectedPRNumber && (
            <div className="git-panel-prs__empty-state">
              <GitPullRequest size={14} />
              <p>Selecione uma PR para abrir o detalhe.</p>
            </div>
          )}

          {selectedPRNumber !== null && detail.status === 'loading' && (
            <div className="git-panel-prs__loading">
              <Loader2 size={14} className="git-panel-prs__spinner" />
              <span>Carregando detalhe da PR...</span>
            </div>
          )}

          {selectedPRNumber !== null && detail.status === 'error' && (
            <BlockError
              title="Falha ao carregar detalhe da PR"
              error={detail.error}
              onRetry={handleRefreshDetail}
            />
          )}

          {selectedPRNumber !== null && detailData && detail.status !== 'error' && isEditFormOpen && editPRForm && (
            <PREditForm
              form={editPRForm}
              validationErrors={editPRValidationErrors}
              error={editPRError}
              isSubmitting={isUpdatingPR}
              isMergedPR={isMergedPRState(detailData.state)}
              onRequiredFieldChange={handleEditPRRequiredFieldChange}
              onBodyChange={handleEditPRBodyChange}
              onStateChange={handleEditPRStateChange}
              onMaintainerCanModifyChange={handleEditPRMaintainerCanModifyChange}
              onCancel={closeEditForm}
              onSubmit={handleEditPRSubmit}
            />
          )}

          {selectedPRNumber !== null && detailData && detail.status !== 'error' && !isEditFormOpen && (
            <section className="git-panel-prs__main-sections" aria-label="Conteúdo principal da Pull Request">
              <nav className="git-panel-prs__section-tabs" aria-label="Seções de Pull Request">
                <button
                  type="button"
                  className={`git-panel-prs__section-tab ${activeSection === 'conversation' ? 'git-panel-prs__section-tab--active' : ''}`}
                  onClick={() => handleSectionChange('conversation')}
                >
                  CONVERSATION
                </button>
                <button
                  type="button"
                  className={`git-panel-prs__section-tab ${activeSection === 'commits' ? 'git-panel-prs__section-tab--active' : ''}`}
                  onClick={() => handleSectionChange('commits')}
                >
                  COMMITS
                </button>
                <button
                  type="button"
                  className={`git-panel-prs__section-tab ${activeSection === 'files' ? 'git-panel-prs__section-tab--active' : ''}`}
                  onClick={() => handleSectionChange('files')}
                >
                  FILES CHANGED
                </button>
              </nav>

              {activeSection === 'conversation' && (
                <PRDetailBlock pr={detailData} />
              )}

              {activeSection === 'commits' && (
                <CommitsWorkspace
                  commits={commits}
                  rawDiff={rawDiff}
                  selectedCommitSha={selectedCommitSha}
                  commitMetricsBySha={commitMetricsBySha}
                  branchName={detailData.headBranch || ''}
                  onSelectCommitDiff={handleSelectCommitDiff}
                  onRetryCommits={() => void fetchCommits({ prNumber: selectedPRNumber, page: 1, perPage: SECTION_PER_PAGE, append: false })}
                  onLoadMoreCommits={handleLoadMoreCommits}
                  onLoad={handleLoadRawDiff}
                />
              )}

              {activeSection === 'files' && (
                <FilesSection
                  files={files}
                  onRetry={() => void fetchFiles({ prNumber: selectedPRNumber, page: 1, perPage: SECTION_PER_PAGE, append: false })}
                  onLoadMore={handleLoadMoreFiles}
                />
              )}
            </section>
          )}
        </div>
      </section>
    </section>
  )
}

interface BlockErrorProps {
  title: string
  error: PRBindingError | null
  onRetry: () => void
}

function BlockError({ title, error, onRetry }: BlockErrorProps) {
  const [isReconnecting, setIsReconnecting] = useState(false)
  const guidance = resolvePRActionableGuidance(error)

  const handleReconnect = () => {
    setIsReconnecting(true)
    void AuthLogin('github')
      .catch((err: unknown) => {
        console.error('[GitPanelPRView] GitHub reconnect failed:', err)
      })
      .finally(() => {
        setIsReconnecting(false)
      })
  }

  return (
    <div className="git-panel-prs__error" role="alert">
      <AlertTriangle size={14} />
      <div>
        <strong>{title}</strong>
        <p>{error?.message || 'Falha desconhecida.'}</p>
        {guidance && <p className="git-panel-prs__error-guidance">{guidance}</p>}
        {error?.details && <small>{error.details}</small>}
        <div className="git-panel-prs__error-actions">
          <button type="button" className="btn btn--ghost git-panel-prs__retry-btn" onClick={onRetry}>
            Tentar novamente
          </button>
          {error?.code === PR_ERROR_CODES.unauthorized && (
            <button
              type="button"
              className="btn btn--ghost git-panel-prs__retry-btn"
              onClick={handleReconnect}
              disabled={isReconnecting}
            >
              {isReconnecting ? 'Abrindo login...' : 'Reconectar GitHub'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

interface PRDetailBlockProps {
  pr: GitPanelPullRequest
}

interface PRListItemProps {
  pr: GitPanelPullRequest
  selected: boolean
  onSelect: (prNumber: number) => void
}

const PRListItem = memo(function PRListItem({ pr, selected, onSelect }: PRListItemProps) {
  const normalizedState = normalizeStateForClass(pr.state)
  return (
    <li>
      <button
        type="button"
        className={`git-panel-prs__item ${selected ? 'git-panel-prs__item--selected' : ''}`}
        onClick={() => onSelect(pr.number)}
        aria-selected={selected}
      >
        <div className="git-panel-prs__item-row">
          <strong>#{pr.number}</strong>
          <span className={`git-panel-prs__state-badge git-panel-prs__state-badge--${normalizedState}`}>
            {pr.state}
          </span>
        </div>
        <div className="git-panel-prs__item-title">
          <GitPullRequest size={13} />
          <span>{pr.title || '(sem título)'}</span>
        </div>
        <div className="git-panel-prs__item-meta">
          <span>@{pr.author?.login || 'unknown'}</span>
          <span>{formatRelativeTime(pr.updatedAt)}</span>
          {pr.isDraft && <span className="badge badge--warning">Draft</span>}
        </div>
        <div className="git-panel-prs__item-foot">
          <span className="git-panel-prs__item-delta git-panel-prs__item-delta--add">+{pr.additions}</span>
          <span className="git-panel-prs__item-delta git-panel-prs__item-delta--del">-{pr.deletions}</span>
          <span className="git-panel-prs__item-files">{pr.changedFiles} arquivo(s)</span>
        </div>
        <div className="git-panel-prs__item-branches">
          <code>{pr.headBranch}</code>
          <span aria-hidden>→</span>
          <code>{pr.baseBranch}</code>
        </div>
      </button>
    </li>
  )
})

function PRDetailBlock({ pr }: PRDetailBlockProps) {
  const authorLogin = pr.author?.login || 'unknown'
  const authorInitial = resolveActorInitial(authorLogin)
  const safeLabels = Array.isArray(pr.labels)
    ? pr.labels.filter((label): label is { name: string; color: string } => {
      if (!label || typeof label !== 'object') {
        return false
      }
      const candidate = label as { name?: unknown; color?: unknown }
      return typeof candidate.name === 'string' && typeof candidate.color === 'string'
    })
    : []

  return (
    <article className="git-panel-prs__detail-card">
      <header className="git-panel-prs__detail-header">
        <div className="git-panel-prs__detail-title-wrap">
          <p className="git-panel-prs__detail-kicker">Pull Request</p>
          <h3>{pr.title || '(sem título)'}</h3>
        </div>
        <div className="git-panel-prs__detail-header-meta">
          <span className={`git-panel-prs__state-badge git-panel-prs__state-badge--${normalizeStateForClass(pr.state)}`}>
            {pr.state}
          </span>
          {pr.isDraft && <span className="badge badge--warning">Draft</span>}
        </div>
      </header>

      <section className="git-panel-prs__detail-body">
        <h4>Descrição</h4>
        <p>{pr.body?.trim() ? pr.body : 'Sem descrição informada para esta PR.'}</p>
      </section>

      <p className="git-panel-prs__detail-number">#{pr.number}</p>
      <div className="git-panel-prs__detail-actor">
        <span className="git-panel-prs__detail-avatar" aria-hidden>{authorInitial}</span>
        <div className="git-panel-prs__detail-actor-meta">
          <p className="git-panel-prs__detail-author">Autor: @{authorLogin}</p>
          <p className="git-panel-prs__detail-updated">Atualizado {formatRelativeTime(pr.updatedAt)}</p>
        </div>
      </div>
      <p className="git-panel-prs__detail-branches">
        <code>{pr.headBranch}</code>
        <span>→</span>
        <code>{pr.baseBranch}</code>
      </p>
      <p className="git-panel-prs__detail-maintainer">
        Maintainer pode modificar: {pr.maintainerCanModify === false ? 'Nao' : 'Sim'}
      </p>

      <div className="git-panel-prs__stats">
        <span className="git-panel-prs__stat git-panel-prs__stat--add">+{pr.additions}</span>
        <span className="git-panel-prs__stat git-panel-prs__stat--del">-{pr.deletions}</span>
        <span className="git-panel-prs__stat">{pr.changedFiles} arquivos</span>
      </div>

      {safeLabels.length > 0 && (
        <div className="git-panel-prs__labels">
          {safeLabels.map((label, index) => (
            <span key={`${label.name}:${index}`} className="git-panel-prs__label-chip">
              {label.name}
            </span>
          ))}
        </div>
      )}
    </article>
  )
}

interface PREditFormProps {
  form: EditPRFormState
  validationErrors: EditPRValidationErrors
  error: PRBindingError | null
  isSubmitting: boolean
  isMergedPR: boolean
  onRequiredFieldChange: (field: EditPRRequiredField, value: string) => void
  onBodyChange: (value: string) => void
  onStateChange: (value: EditPRWritableState) => void
  onMaintainerCanModifyChange: (value: boolean) => void
  onCancel: () => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
}

function PREditForm({
  form,
  validationErrors,
  error,
  isSubmitting,
  isMergedPR,
  onRequiredFieldChange,
  onBodyChange,
  onStateChange,
  onMaintainerCanModifyChange,
  onCancel,
  onSubmit,
}: PREditFormProps) {
  return (
    <form className="git-panel-prs__edit-form" onSubmit={onSubmit} noValidate>
      <div className="git-panel-prs__form-field">
        <label htmlFor="git-panel-pr-edit-title">Titulo *</label>
        <input
          id="git-panel-pr-edit-title"
          type="text"
          className={`input ${validationErrors.title ? 'git-panel-prs__form-input--error' : ''}`}
          value={form.title}
          onChange={(event) => onRequiredFieldChange('title', event.target.value)}
          placeholder="Ex: feat: adicionar fluxo de pagamento"
          aria-invalid={Boolean(validationErrors.title)}
          disabled={isSubmitting}
        />
        {validationErrors.title && (
          <small className="git-panel-prs__form-error">{validationErrors.title}</small>
        )}
      </div>

      <div className="git-panel-prs__form-field">
        <label htmlFor="git-panel-pr-edit-body">Descricao</label>
        <textarea
          id="git-panel-pr-edit-body"
          className="git-panel-prs__textarea"
          value={form.body}
          onChange={(event) => onBodyChange(event.target.value)}
          placeholder="Contexto da PR, escopo, riscos e validacoes."
          rows={6}
          disabled={isSubmitting}
        />
      </div>

      <div className="git-panel-prs__form-field">
        <label htmlFor="git-panel-pr-edit-base">Base *</label>
        <input
          id="git-panel-pr-edit-base"
          type="text"
          className={`input ${validationErrors.base ? 'git-panel-prs__form-input--error' : ''}`}
          value={form.base}
          onChange={(event) => onRequiredFieldChange('base', event.target.value)}
          placeholder="Ex: main"
          aria-invalid={Boolean(validationErrors.base)}
          disabled={isSubmitting}
        />
        {validationErrors.base && (
          <small className="git-panel-prs__form-error">{validationErrors.base}</small>
        )}
      </div>

      <div className="git-panel-prs__form-field">
        <label htmlFor="git-panel-pr-edit-state">Estado</label>
        <select
          id="git-panel-pr-edit-state"
          className="input"
          value={form.state}
          onChange={(event) => onStateChange(event.target.value === 'open' ? 'open' : 'closed')}
          disabled={isSubmitting || isMergedPR}
        >
          <option value="open">Open</option>
          <option value="closed">Closed</option>
        </select>
        {isMergedPR && (
          <small className="git-panel-prs__form-hint">PR mergeada permanece com state fechado.</small>
        )}
      </div>

      <label className="git-panel-prs__checkbox">
        <input
          type="checkbox"
          checked={form.maintainerCanModify}
          onChange={(event) => onMaintainerCanModifyChange(event.target.checked)}
          disabled={isSubmitting}
        />
        Permitir alteracoes por mantenedores
      </label>

      {error && (
        <div className="git-panel-prs__edit-error" role="alert">
          <strong>Falha ao atualizar PR</strong>
          <p>{error.message}</p>
          {error.details && <small>{error.details}</small>}
        </div>
      )}

      <div className="git-panel-prs__create-actions">
        <button type="button" className="btn btn--ghost" onClick={onCancel} disabled={isSubmitting}>
          Cancelar
        </button>
        <button type="submit" className="btn btn--primary" disabled={isSubmitting}>
          {isSubmitting ? 'Salvando...' : 'Salvar alteracoes'}
        </button>
      </div>
    </form>
  )
}

interface FilesSectionProps {
  files: GitPanelPRStoreState['files']
  onRetry: () => void
  onLoadMore: () => void
}

function FilesSection({ files, onRetry, onLoadMore }: FilesSectionProps) {
  const [visibleLoadedCount, setVisibleLoadedCount] = useState(FILES_RENDER_BATCH_SIZE)
  const items = files.data?.items || []
  const currentPage = files.data?.page ?? 0
  const hasMoreOnServer = Boolean(files.data?.hasNextPage)

  useEffect(() => {
    if (currentPage <= 1) {
      if (items.length === 0) {
        setVisibleLoadedCount(FILES_RENDER_BATCH_SIZE)
        return
      }
      setVisibleLoadedCount(Math.min(FILES_RENDER_BATCH_SIZE, items.length))
      return
    }

    setVisibleLoadedCount((current) => {
      if (items.length === 0) {
        return FILES_RENDER_BATCH_SIZE
      }
      if (current <= 0) {
        return Math.min(FILES_RENDER_BATCH_SIZE, items.length)
      }
      if (current > items.length) {
        return items.length
      }
      return current
    })
  }, [currentPage, items.length])

  const fileStats = useMemo(() => {
    let binary = 0
    let truncated = 0
    let missingPatch = 0

    for (const item of items) {
      if (item.isBinary) {
        binary++
      }
      if (item.isPatchTruncated) {
        truncated++
      }
      if (!item.hasPatch) {
        missingPatch++
      }
    }

    return { binary, truncated, missingPatch }
  }, [items])

  const visibleItems = items.slice(0, visibleLoadedCount)
  const hiddenLoadedCount = Math.max(0, items.length - visibleItems.length)

  const handleShowMoreLoaded = () => {
    if (hiddenLoadedCount <= 0) {
      return
    }
    setVisibleLoadedCount((current) => Math.min(items.length, current + FILES_RENDER_BATCH_SIZE))
  }

  if (files.status === 'loading' && !files.data) {
    return (
      <div className="git-panel-prs__loading">
        <Loader2 size={14} className="git-panel-prs__spinner" />
        <span>Carregando arquivos da PR...</span>
      </div>
    )
  }

  if (files.status === 'error') {
    return (
      <BlockError title="Falha ao carregar arquivos da PR" error={files.error} onRetry={onRetry} />
    )
  }

  if (items.length === 0 && files.status !== 'loading') {
    return (
      <div className="git-panel-prs__empty-state">
        <FileCode2 size={14} />
        <p>Sem arquivos retornados para esta PR.</p>
      </div>
    )
  }

  return (
    <div className="git-panel-prs__files">
      <div className="git-panel-prs__files-summary">
        <span>{formatLoadedFilesLabel(items.length)}</span>
        {fileStats.binary > 0 && <span>{fileStats.binary} binário(s)</span>}
        {fileStats.truncated > 0 && <span>{fileStats.truncated} truncado(s)</span>}
        {fileStats.missingPatch > 0 && <span>{fileStats.missingPatch} sem patch renderizável</span>}
        {hasMoreOnServer && <span>há mais arquivos no GitHub</span>}
      </div>

      {visibleItems.map((file, index) => (
        <FileItem key={`${file.filename}:${file.status}:${index}`} file={file} />
      ))}

      {hiddenLoadedCount > 0 && (
        <button
          type="button"
          className="btn btn--ghost git-panel-prs__load-more"
          onClick={handleShowMoreLoaded}
        >
          Mostrar mais {Math.min(hiddenLoadedCount, FILES_RENDER_BATCH_SIZE)} arquivo(s) carregado(s)
        </button>
      )}

      {hasMoreOnServer && (
        <button
          type="button"
          className="btn btn--ghost git-panel-prs__load-more"
          onClick={onLoadMore}
          disabled={files.status === 'loading'}
        >
          {files.status === 'loading' ? 'Carregando mais arquivos...' : 'Carregar mais arquivos do GitHub'}
        </button>
      )}
    </div>
  )
}

interface FileItemProps {
  file: GitPanelPRFile
}

const FileItem = memo(function FileItem({ file }: FileItemProps) {
  const [isPatchExpanded, setIsPatchExpanded] = useState(false)
  const [forceFullPatchRender, setForceFullPatchRender] = useState(false)
  const [patchViewMode, setPatchViewMode] = useState<PatchViewMode>('split')

  const patchText = file.patch || ''
  const patchLineCount = useMemo(() => {
    if (!file.hasPatch || !isPatchExpanded) {
      return 0
    }
    return countPatchLines(patchText)
  }, [file.hasPatch, isPatchExpanded, patchText])

  const shouldGuardFullPatch = file.hasPatch && (
    file.isPatchTruncated
    || patchText.length > PATCH_FULL_RENDER_GUARD_CHARS
    || (patchLineCount > 0 && patchLineCount > PATCH_FULL_RENDER_GUARD_LINES)
  )

  const patchPreview = useMemo(() => {
    if (!file.hasPatch || !isPatchExpanded || !shouldGuardFullPatch || forceFullPatchRender) {
      return null
    }
    return buildPatchPreview(patchText, PATCH_PREVIEW_MAX_LINES, PATCH_PREVIEW_MAX_CHARS)
  }, [file.hasPatch, forceFullPatchRender, isPatchExpanded, patchText, shouldGuardFullPatch])

  const patchToRender = patchPreview?.patch || patchText

  const handlePatchToggle = () => {
    setIsPatchExpanded((current) => {
      const next = !current
      if (!next) {
        setForceFullPatchRender(false)
        setPatchViewMode('split')
      }
      return next
    })
  }

  return (
    <article className="git-panel-prs__file-card">
      <header className="git-panel-prs__file-header">
        <strong>{file.filename}</strong>
        <span className="git-panel-prs__file-status">{file.status}</span>
      </header>
      <p className="git-panel-prs__file-stats">
        +{file.additions} / -{file.deletions} ({file.changes} mudanças)
      </p>

      {file.previousFilename && (
        <p className="git-panel-prs__file-rename">Renamed from: {file.previousFilename}</p>
      )}

      {file.hasPatch ? (
        <>
          <div className="git-panel-prs__patch-actions">
            <button type="button" className="btn btn--ghost git-panel-prs__patch-toggle" onClick={handlePatchToggle}>
              {isPatchExpanded ? 'Ocultar patch' : 'Exibir patch'}
            </button>
            <span className="git-panel-prs__patch-meta">{formatPatchSummary(patchText.length, patchLineCount)}</span>
          </div>

          {isPatchExpanded && (
            <>
              {file.isPatchTruncated && (
                <p className="git-panel-prs__patch-warning">Patch truncado pelo GitHub (exibindo parcial).</p>
              )}
              {patchPreview && (
                <p className="git-panel-prs__patch-warning">
                  Patch grande detectado. Exibindo preview para preservar responsividade.
                </p>
              )}
              <div className="git-panel-prs__patch-view-tabs" role="tablist" aria-label="Modo de visualização do diff">
                <button
                  type="button"
                  role="tab"
                  aria-selected={patchViewMode === 'split'}
                  className={`btn btn--ghost git-panel-prs__patch-view-tab ${patchViewMode === 'split' ? 'git-panel-prs__patch-view-tab--active' : ''}`}
                  onClick={() => setPatchViewMode('split')}
                >
                  Side by side
                </button>
                <button
                  type="button"
                  role="tab"
                  aria-selected={patchViewMode === 'unified'}
                  className={`btn btn--ghost git-panel-prs__patch-view-tab ${patchViewMode === 'unified' ? 'git-panel-prs__patch-view-tab--active' : ''}`}
                  onClick={() => setPatchViewMode('unified')}
                >
                  Unified
                </button>
              </div>
              <div className="git-panel-prs__patch">
                {patchViewMode === 'split' ? (
                  <PatchSplitDiffView patch={patchToRender} />
                ) : (
                  <PatchUnifiedDiffView patch={patchToRender} />
                )}
              </div>
              {patchPreview?.truncated && (
                <p className="git-panel-prs__patch-warning">
                  Preview limitado a {PATCH_PREVIEW_MAX_LINES} linhas ou {formatPatchByteSize(PATCH_PREVIEW_MAX_CHARS)}.
                </p>
              )}
              {patchPreview && (
                <button
                  type="button"
                  className="btn btn--ghost git-panel-prs__patch-toggle"
                  onClick={() => setForceFullPatchRender(true)}
                >
                  Renderizar patch completo
                </button>
              )}
            </>
          )}
        </>
      ) : (
        <p className="git-panel-prs__patch-warning">
          {file.isBinary ? 'Arquivo binário sem patch renderizável.' : 'Patch indisponível para este arquivo.'}
        </p>
      )}
    </article>
  )
})

type PRPatchLineType = 'context' | 'add' | 'delete' | 'meta'

interface PRPatchLine {
  type: PRPatchLineType
  content: string
  oldLine: number | null
  newLine: number | null
}

interface PRPatchHunk {
  header: string
  lines: PRPatchLine[]
}

interface PRPatchSplitPair {
  left: PRPatchLine | null
  right: PRPatchLine | null
  meta?: string
}

interface PatchUnifiedDiffViewProps {
  patch: string
}

function PatchUnifiedDiffView({ patch }: PatchUnifiedDiffViewProps) {
  const hunks = useMemo(() => parsePatchHunks(patch), [patch])

  if (hunks.length === 0) {
    return (
      <pre className="git-panel-prs__split-fallback">
        {patch || '(sem conteúdo de diff)'}
      </pre>
    )
  }

  return (
    <div className="git-panel-diff-hunk__unified">
      {hunks.map((hunk, hunkIndex) => (
        <div key={`${hunk.header}-${hunkIndex}`}>
          <div className="git-panel-diff-hunk__header">
            <code>{hunk.header}</code>
          </div>
          {hunk.lines.map((line, lineIndex) => (
            <div
              key={`${hunkIndex}-${lineIndex}`}
              className={`git-panel-diff-line git-panel-diff-line--${line.type}`}
            >
              <span className="git-panel-diff-line__checkbox-spacer" />
              <span className="git-panel-diff-line__gutter">{line.oldLine ?? ''}</span>
              <span className="git-panel-diff-line__gutter">{line.newLine ?? ''}</span>
              <span className="git-panel-diff-line__prefix">
                {line.type === 'add' ? '+' : line.type === 'delete' ? '-' : line.type === 'meta' ? '\\' : ' '}
              </span>
              <span className="git-panel-diff-line__content">{line.content}</span>
            </div>
          ))}
        </div>
      ))}
    </div>
  )
}

interface PatchSplitDiffViewProps {
  patch: string
}

function PatchSplitDiffView({ patch }: PatchSplitDiffViewProps) {
  const hunks = useMemo(() => parsePatchHunks(patch), [patch])

  if (hunks.length === 0) {
    return (
      <pre className="git-panel-prs__split-fallback">
        {patch || '(sem conteúdo de diff)'}
      </pre>
    )
  }

  return (
    <div className="git-panel-prs__split-root">
      {hunks.map((hunk, hunkIndex) => (
        <PatchSplitHunk key={`${hunk.header}-${hunkIndex}`} hunk={hunk} hunkIndex={hunkIndex} />
      ))}
    </div>
  )
}

interface PatchSplitHunkProps {
  hunk: PRPatchHunk
  hunkIndex: number
}

function PatchSplitHunk({ hunk, hunkIndex }: PatchSplitHunkProps) {
  const splitPairs = useMemo(() => buildPatchSplitPairs(hunk.lines), [hunk.lines])
  const splitLeftRef = useRef<HTMLDivElement | null>(null)
  const splitRightRef = useRef<HTMLDivElement | null>(null)
  const scrollSyncSourceRef = useRef<'left' | 'right' | null>(null)

  const handleSplitScroll = useCallback((source: 'left' | 'right') => {
    if (scrollSyncSourceRef.current && scrollSyncSourceRef.current !== source) {
      return
    }
    scrollSyncSourceRef.current = source
    const sourceEl = source === 'left' ? splitLeftRef.current : splitRightRef.current
    const targetEl = source === 'left' ? splitRightRef.current : splitLeftRef.current
    if (!sourceEl || !targetEl) {
      scrollSyncSourceRef.current = null
      return
    }

    const maxScrollSource = sourceEl.scrollHeight - sourceEl.clientHeight
    const maxScrollTarget = targetEl.scrollHeight - targetEl.clientHeight
    if (maxScrollSource > 0 && maxScrollTarget > 0) {
      const ratio = sourceEl.scrollTop / maxScrollSource
      targetEl.scrollTop = ratio * maxScrollTarget
    } else {
      targetEl.scrollTop = sourceEl.scrollTop
    }

    window.requestAnimationFrame(() => {
      scrollSyncSourceRef.current = null
    })
  }, [])

  return (
    <section className="git-panel-prs__split-hunk">
      <header className="git-panel-prs__split-hunk-header">
        <code>{hunk.header || `Hunk ${hunkIndex + 1}`}</code>
      </header>
      <div className="git-panel-prs__split-container">
        <div
          className="git-panel-prs__split-pane"
          ref={splitLeftRef}
          onScroll={() => handleSplitScroll('left')}
        >
          {splitPairs.map((pair, pairIndex) => {
            if (pair.meta) {
              return (
                <div key={`left-meta-${pairIndex}`} className="git-panel-prs__split-meta-row">
                  {pair.meta}
                </div>
              )
            }
            const line = pair.left
            const rowType = line?.type ?? 'context'
            return (
              <div key={`left-row-${pairIndex}`} className={`git-panel-prs__split-row git-panel-prs__split-row--${rowType}`}>
                <span className="git-panel-prs__split-line-number">{line?.oldLine ?? ''}</span>
                <span className="git-panel-prs__split-code">{line?.content || ' '}</span>
              </div>
            )
          })}
        </div>
        <div className="git-panel-prs__split-divider" />
        <div
          className="git-panel-prs__split-pane"
          ref={splitRightRef}
          onScroll={() => handleSplitScroll('right')}
        >
          {splitPairs.map((pair, pairIndex) => {
            if (pair.meta) {
              return (
                <div key={`right-meta-${pairIndex}`} className="git-panel-prs__split-meta-row">
                  {pair.meta}
                </div>
              )
            }
            const line = pair.right
            const rowType = line?.type ?? 'context'
            return (
              <div key={`right-row-${pairIndex}`} className={`git-panel-prs__split-row git-panel-prs__split-row--${rowType}`}>
                <span className="git-panel-prs__split-line-number">{line?.newLine ?? ''}</span>
                <span className="git-panel-prs__split-code">{line?.content || ' '}</span>
              </div>
            )
          })}
        </div>
      </div>
    </section>
  )
}

function buildPatchSplitPairs(lines: PRPatchLine[]): PRPatchSplitPair[] {
  const pairs: PRPatchSplitPair[] = []
  const pendingDeletes: PRPatchLine[] = []
  const pendingAdds: PRPatchLine[] = []

  const flushPending = () => {
    const maxRows = Math.max(pendingDeletes.length, pendingAdds.length)
    for (let index = 0; index < maxRows; index += 1) {
      pairs.push({
        left: pendingDeletes[index] ?? null,
        right: pendingAdds[index] ?? null,
      })
    }
    pendingDeletes.length = 0
    pendingAdds.length = 0
  }

  for (const line of lines) {
    if (line.type === 'context') {
      flushPending()
      pairs.push({ left: line, right: line })
      continue
    }
    if (line.type === 'delete') {
      pendingDeletes.push(line)
      continue
    }
    if (line.type === 'add') {
      pendingAdds.push(line)
      continue
    }

    flushPending()
    pairs.push({
      left: null,
      right: null,
      meta: line.content,
    })
  }

  flushPending()
  return pairs
}

function parsePatchHunks(patch: string): PRPatchHunk[] {
  if (!patch) {
    return []
  }

  const normalizedPatch = patch.replace(/\r/g, '')
  const rawLines = normalizedPatch.split('\n')
  const hunks: PRPatchHunk[] = []
  let currentHunk: PRPatchHunk | null = null
  let oldLine = 0
  let newLine = 0

  for (const rawLine of rawLines) {
    if (rawLine.startsWith('@@')) {
      const parsedHeader = parsePatchHunkHeader(rawLine)
      currentHunk = {
        header: rawLine,
        lines: [],
      }
      hunks.push(currentHunk)
      oldLine = parsedHeader.oldStart
      newLine = parsedHeader.newStart
      continue
    }

    if (
      rawLine.startsWith('diff --git ')
      || rawLine.startsWith('index ')
      || rawLine.startsWith('--- ')
      || rawLine.startsWith('+++ ')
      || rawLine.startsWith('Binary files ')
    ) {
      continue
    }

    const marker = rawLine.charAt(0)
    const isPatchLine = marker === '+' || marker === '-' || marker === ' ' || marker === '\\'
    if (!currentHunk) {
      if (!isPatchLine) {
        continue
      }
      currentHunk = {
        header: '',
        lines: [],
      }
      hunks.push(currentHunk)
    }

    if (marker === '+' && !rawLine.startsWith('+++')) {
      currentHunk.lines.push({
        type: 'add',
        content: rawLine.slice(1),
        oldLine: null,
        newLine,
      })
      newLine += 1
      continue
    }

    if (marker === '-' && !rawLine.startsWith('---')) {
      currentHunk.lines.push({
        type: 'delete',
        content: rawLine.slice(1),
        oldLine,
        newLine: null,
      })
      oldLine += 1
      continue
    }

    if (marker === ' ') {
      currentHunk.lines.push({
        type: 'context',
        content: rawLine.slice(1),
        oldLine,
        newLine,
      })
      oldLine += 1
      newLine += 1
      continue
    }

    if (marker === '\\') {
      currentHunk.lines.push({
        type: 'meta',
        content: rawLine,
        oldLine: null,
        newLine: null,
      })
    }
  }

  return hunks.filter((hunk) => hunk.lines.length > 0)
}

function parsePatchHunkHeader(header: string): { oldStart: number; newStart: number } {
  const match = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(header)
  if (!match) {
    return { oldStart: 0, newStart: 0 }
  }
  return {
    oldStart: Number.parseInt(match[1], 10) || 0,
    newStart: Number.parseInt(match[2], 10) || 0,
  }
}

interface CommitsWorkspaceProps {
  commits: GitPanelPRStoreState['commits']
  rawDiff: GitPanelPRStoreState['rawDiff']
  selectedCommitSha: string
  commitMetricsBySha: Record<string, { additions: number; deletions: number; changedFiles: number }>
  branchName: string
  onSelectCommitDiff: (commit: GitPanelPRCommit) => void
  onRetryCommits: () => void
  onLoadMoreCommits: () => void
  onLoad: () => void
}

function CommitsWorkspace({
  commits,
  rawDiff,
  selectedCommitSha,
  commitMetricsBySha,
  branchName,
  onSelectCommitDiff,
  onRetryCommits,
  onLoadMoreCommits,
  onLoad,
}: CommitsWorkspaceProps) {
  return (
    <div className="git-panel-prs__commits-workspace">
      <div className="git-panel-prs__commits-pane git-panel-prs__commits-pane--list">
        <CommitsSection
          commits={commits}
          selectedCommitSha={selectedCommitSha}
          commitMetricsBySha={commitMetricsBySha}
          branchName={branchName}
          onSelectCommitDiff={onSelectCommitDiff}
          onRetry={onRetryCommits}
          onLoadMore={onLoadMoreCommits}
        />
      </div>
      <div className="git-panel-prs__commits-pane git-panel-prs__commits-pane--diff">
        <RawDiffSection
          rawDiff={rawDiff}
          onLoad={onLoad}
          selectedCommitSha={selectedCommitSha}
        />
      </div>
    </div>
  )
}

interface CommitsSectionProps {
  commits: GitPanelPRStoreState['commits']
  selectedCommitSha: string
  commitMetricsBySha: Record<string, { additions: number; deletions: number; changedFiles: number }>
  branchName: string
  onSelectCommitDiff: (commit: GitPanelPRCommit) => void
  onRetry: () => void
  onLoadMore: () => void
}

function CommitsSection({
  commits,
  selectedCommitSha,
  commitMetricsBySha,
  branchName,
  onSelectCommitDiff,
  onRetry,
  onLoadMore,
}: CommitsSectionProps) {
  if (commits.status === 'loading' && !commits.data) {
    return (
      <div className="git-panel-prs__loading">
        <Loader2 size={14} className="git-panel-prs__spinner" />
        <span>Carregando commits da PR...</span>
      </div>
    )
  }

  if (commits.status === 'error') {
    return (
      <BlockError title="Falha ao carregar commits da PR" error={commits.error} onRetry={onRetry} />
    )
  }

  const items = commits.data?.items || []
  if (items.length === 0 && commits.status !== 'loading') {
    return (
      <div className="git-panel-prs__empty-state">
        <GitCommitHorizontal size={14} />
        <p>Sem commits retornados para esta PR.</p>
      </div>
    )
  }

  return (
    <div className="git-panel-prs__commits">
      {items.map((commit) => (
        <CommitItem
          key={commit.sha}
          commit={commit}
          metrics={resolveCommitMetrics(commit, commitMetricsBySha)}
          branchName={branchName}
          selected={(commit.sha || '').trim() === selectedCommitSha}
          onSelectCommitDiff={onSelectCommitDiff}
        />
      ))}

      {commits.data?.hasNextPage && (
        <button type="button" className="btn btn--ghost git-panel-prs__load-more" onClick={onLoadMore}>
          Carregar mais commits
        </button>
      )}
    </div>
  )
}

interface CommitItemProps {
  commit: GitPanelPRCommit
  metrics: { additions: number; deletions: number; changedFiles: number }
  branchName: string
  selected: boolean
  onSelectCommitDiff: (commit: GitPanelPRCommit) => void
}

const CommitItem = memo(function CommitItem({ commit, metrics, branchName, selected, onSelectCommitDiff }: CommitItemProps) {
  const actorLabel = commit.authorName || commit.author?.login || commit.committerName || 'autor desconhecido'
  const avatarUrl = commit.author?.avatarUrl || commit.committer?.avatarUrl || ''
  return (
    <button
      type="button"
      className={`git-panel-prs__commit-card git-panel-log__row ${selected ? 'git-panel-log__row--selected' : ''}`}
      onClick={() => onSelectCommitDiff(commit)}
    >
      <div className="git-panel-log__row-main">
        <div className="git-panel-log__row-title-wrap">
          <span className="git-panel-log__commit-icon git-panel-log__commit-icon--title" aria-hidden="true">
            <GitCommitHorizontal size={14} />
          </span>
          <div className="git-panel-log__row-title" title={firstLine(commit.message) || '(sem mensagem)'}>
            {firstLine(commit.message) || '(sem mensagem)'}
          </div>
        </div>
        <div className="git-panel-log__row-tags" aria-hidden="true">
          <span className="git-panel-log__chip">local</span>
          <span className="git-panel-log__chip git-panel-log__chip--hash">{shortSHA(commit.sha)}</span>
        </div>
      </div>

      <div className="git-panel-log__row-author">
        {avatarUrl ? (
          <img
            src={avatarUrl}
            alt={`Avatar de ${actorLabel}`}
            className="git-panel-log__avatar"
            loading="lazy"
          />
        ) : (
          <span className="git-panel-log__avatar git-panel-log__avatar--fallback" aria-hidden="true">
            {buildActorInitials(actorLabel)}
          </span>
        )}
        <span className="git-panel-log__author-name">{actorLabel}</span>
        {branchName ? (
          <span className="git-panel-log__author-branch" title={`Branch atual: ${branchName}`}>
            <GitBranch size={11} aria-hidden="true" />
            {branchName}
          </span>
        ) : null}
      </div>

      <div className="git-panel-prs__commit-metrics git-panel-log__row-meta">
        <span className="git-panel-log__metric">
          <Clock3 size={12} aria-hidden="true" />
          {formatRelativeTime(commit.authoredAt || commit.committedAt || '')}
        </span>
        <span className="git-panel-log__metric git-panel-log__metric--positive">+{formatMetricCount(metrics.additions)}</span>
        <span className="git-panel-log__metric git-panel-log__metric--negative">-{formatMetricCount(metrics.deletions)}</span>
        <span className="git-panel-log__metric">
          <FileCode2 size={12} aria-hidden="true" />
          {formatFilesChangedCompact(metrics.changedFiles)}
        </span>
      </div>
    </button>
  )
})

interface RawDiffSectionProps {
  rawDiff: GitPanelPRStoreState['rawDiff']
  onLoad: () => void
  selectedCommitSha: string
}

function RawDiffSection({ rawDiff, onLoad, selectedCommitSha }: RawDiffSectionProps) {
  if (rawDiff.status === 'idle') {
    return (
      <div className="git-panel-prs__empty-state">
        <FileCode2 size={14} />
        <p>O diff completo não é carregado no boot.</p>
        <button type="button" className="btn btn--primary" onClick={onLoad}>
          Ver diff completo
        </button>
      </div>
    )
  }

  if (rawDiff.status === 'loading') {
    return (
      <div className="git-panel-prs__loading">
        <Loader2 size={14} className="git-panel-prs__spinner" />
        <span>Carregando diff completo...</span>
      </div>
    )
  }

  if (rawDiff.status === 'error') {
    return (
      <BlockError title="Falha ao carregar diff completo" error={rawDiff.error} onRetry={onLoad} />
    )
  }

  return (
    <div className="git-panel-prs__raw-diff">
      <p className="git-panel-prs__raw-diff-caption">
        {selectedCommitSha ? `Diff do commit ${shortSHA(selectedCommitSha)}` : 'Diff completo da Pull Request'}
      </p>
      {rawDiff.data ? (
        <ReactAICodeBlock
          code={rawDiff.data}
          language="diff"
          theme={PR_DIFF_SHIKI_THEME}
          className="git-panel-prs__raw-diff-block"
        />
      ) : (
        <p>Diff completo retornou vazio.</p>
      )}
    </div>
  )
}

interface PatchPreview {
  patch: string
  truncated: boolean
}

function buildPatchPreview(patch: string, maxLines: number, maxChars: number): PatchPreview {
  const normalizedPatch = patch || ''
  if (!normalizedPatch) {
    return { patch: '', truncated: false }
  }

  const safeMaxLines = Math.max(1, Math.floor(maxLines))
  const safeMaxChars = Math.max(1, Math.floor(maxChars))

  let lines = 1
  let endIndex = 0
  for (; endIndex < normalizedPatch.length; endIndex++) {
    if (endIndex >= safeMaxChars) {
      break
    }
    if (normalizedPatch.charCodeAt(endIndex) === 10) {
      if (lines >= safeMaxLines) {
        break
      }
      lines++
    }
  }

  const didTruncate = endIndex < normalizedPatch.length
  if (!didTruncate) {
    return { patch: normalizedPatch, truncated: false }
  }

  const preview = normalizedPatch.slice(0, endIndex).trimEnd()
  return {
    patch: preview ? `${preview}\n...` : '...',
    truncated: true,
  }
}

function countPatchLines(patch: string): number {
  if (!patch) {
    return 0
  }

  let lines = 1
  for (let index = 0; index < patch.length; index++) {
    if (patch.charCodeAt(index) === 10) {
      lines++
    }
  }
  return lines
}

function formatPatchSummary(charCount: number, lineCount: number): string {
  const sizeLabel = formatPatchByteSize(charCount)
  if (lineCount <= 0) {
    return sizeLabel
  }
  return `${lineCount} linhas • ${sizeLabel}`
}

function formatPatchByteSize(charCount: number): string {
  const safeBytes = Math.max(0, Math.floor(charCount))
  if (safeBytes < 1024) {
    return `${safeBytes} B`
  }
  if (safeBytes < 1024 * 1024) {
    return `${(safeBytes / 1024).toFixed(1)} KB`
  }
  return `${(safeBytes / (1024 * 1024)).toFixed(2)} MB`
}

function formatLoadedFilesLabel(count: number): string {
  const safeCount = Math.max(0, Math.floor(count))
  if (safeCount === 1) {
    return '1 arquivo carregado'
  }
  return `${safeCount} arquivos carregados`
}

function resolveCommitMetrics(
  commit: GitPanelPRCommit,
  commitMetricsBySha: Record<string, { additions: number; deletions: number; changedFiles: number }>,
): { additions: number; deletions: number; changedFiles: number } {
  const commitSha = (commit.sha || '').trim()
  const cached = commitSha ? commitMetricsBySha[commitSha] : undefined
  if (cached) {
    return cached
  }

  return {
    additions: parseCommitMetricValue((commit as unknown as { additions?: unknown }).additions),
    deletions: parseCommitMetricValue((commit as unknown as { deletions?: unknown }).deletions),
    changedFiles: parseCommitMetricValue((commit as unknown as { changedFiles?: unknown }).changedFiles),
  }
}

function parseCommitMetricValue(value: unknown): number {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return Math.max(0, Math.floor(value))
  }
  if (typeof value === 'string') {
    const parsed = Number(value.trim())
    if (Number.isFinite(parsed)) {
      return Math.max(0, Math.floor(parsed))
    }
  }
  return 0
}

function buildDiffMetrics(diffText: string): { additions: number; deletions: number; changedFiles: number } {
  if (!diffText) {
    return { additions: 0, deletions: 0, changedFiles: 0 }
  }

  let additions = 0
  let deletions = 0
  let changedFiles = 0

  const lines = diffText.replace(/\r/g, '').split('\n')
  for (const line of lines) {
    if (line.startsWith('diff --git ')) {
      changedFiles++
      continue
    }
    if (line.startsWith('+') && !line.startsWith('+++')) {
      additions++
      continue
    }
    if (line.startsWith('-') && !line.startsWith('---')) {
      deletions++
    }
  }

  return {
    additions,
    deletions,
    changedFiles,
  }
}

function formatMetricCount(value: number): string {
  const safeValue = Number.isFinite(value) ? Math.max(0, Math.trunc(value)) : 0
  return safeValue.toLocaleString()
}

function formatFilesChangedCompact(value: number): string {
  const safeValue = Number.isFinite(value) ? Math.max(0, Math.trunc(value)) : 0
  return safeValue === 1 ? '1 file' : `${safeValue} files`
}

function clamp(value: number, min: number, max: number): number {
  if (value < min) {
    return min
  }
  if (value > max) {
    return max
  }
  return value
}

function resolvePRActionableGuidance(error: PRBindingError | null): string {
  const code = (error?.code || '').trim()
  switch (code) {
    case PR_ERROR_CODES.unauthorized:
      return 'Sessao GitHub expirada. Reconecte a conta para retomar as operacoes de PR.'
    case PR_ERROR_CODES.forbidden:
      return 'Permissao insuficiente ou limite secundario ativo. Aguarde alguns segundos e tente novamente com menor frequencia.'
    case PR_ERROR_CODES.notFound:
      return 'Recurso nao encontrado. Valide o repositorio selecionado e confirme se a PR ainda existe no GitHub.'
    case PR_ERROR_CODES.conflict:
      return 'Conflito detectado. Atualize a branch da PR e recarregue antes de repetir a operacao.'
    case PR_ERROR_CODES.validationFailed:
      return 'Dados invalidos enviados ao backend. Revise numero da PR, filtros e campos obrigatorios.'
    case PR_ERROR_CODES.rateLimited:
      return 'Rate limit excedido. Aguarde o reset do GitHub e evite refresh manual em sequencia.'
    default:
      return ''
  }
}

function isRateLimitedBindingError(error: PRBindingError | null): boolean {
  if (!error) {
    return false
  }
  if (error.code === PR_ERROR_CODES.rateLimited) {
    return true
  }
  if (error.code !== PR_ERROR_CODES.forbidden) {
    return false
  }
  const payload = `${error.message} ${error.details || ''}`.toLowerCase()
  return payload.includes('rate limit') || payload.includes('secondary limit')
}

function normalizeStateForClass(state: string): string {
  const normalized = (state || '').trim().toLowerCase()
  if (normalized === 'merged') {
    return 'merged'
  }
  if (normalized === 'closed') {
    return 'closed'
  }
  return 'open'
}

function matchesPRListQuery(pr: GitPanelPullRequest, rawQuery: string): boolean {
  const normalizedQuery = rawQuery.trim().toLowerCase()
  if (!normalizedQuery) {
    return true
  }
  const searchableFields = [
    `${pr.number}`,
    pr.title,
    pr.state,
    pr.author?.login || '',
    pr.headBranch,
    pr.baseBranch,
    ...(Array.isArray(pr.labels) ? pr.labels.map((label) => label.name) : []),
  ]
    .join(' ')
    .toLowerCase()

  return searchableFields.includes(normalizedQuery)
}

function formatRelativeTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return 'data invalida'
  }

  const diffMs = date.getTime() - Date.now()
  const absDiffMs = Math.abs(diffMs)
  const minuteMs = 60 * 1000
  const hourMs = 60 * minuteMs
  const dayMs = 24 * hourMs

  if (absDiffMs < hourMs) {
    const minutes = Math.max(1, Math.round(diffMs / minuteMs))
    return new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' }).format(minutes, 'minute')
  }

  if (absDiffMs < dayMs) {
    const hours = Math.round(diffMs / hourMs)
    return new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' }).format(hours, 'hour')
  }

  const days = Math.round(diffMs / dayMs)
  return new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' }).format(days, 'day')
}

function resolveActorInitial(login: string): string {
  const normalized = (login || '').trim()
  if (!normalized) {
    return '?'
  }
  const first = normalized[0]
  return first ? first.toUpperCase() : '?'
}

function buildActorInitials(value: string): string {
  const parts = (value || '')
    .trim()
    .split(/\s+/)
    .filter(Boolean)
  if (parts.length === 0) {
    return '?'
  }
  if (parts.length === 1) {
    return parts[0].slice(0, 1).toUpperCase()
  }
  return `${parts[0].slice(0, 1)}${parts[1].slice(0, 1)}`.toUpperCase()
}

function shortSHA(sha: string): string {
  const normalized = (sha || '').trim()
  if (normalized.length <= 7) {
    return normalized
  }
  return normalized.slice(0, 7)
}

function firstLine(value: string): string {
  const normalized = (value || '').trim()
  if (!normalized) {
    return ''
  }
  const lineBreak = normalized.indexOf('\n')
  if (lineBreak < 0) {
    return normalized
  }
  return normalized.slice(0, lineBreak)
}

function createDefaultPRFormState(): CreatePRFormState {
  return {
    title: '',
    head: '',
    base: CREATE_PR_DEFAULT_BASE,
    body: '',
    draft: false,
    maintainerCanModify: true,
    advancedMode: false,
    targetOwner: '',
    targetRepo: '',
    confirmTargetOverride: false,
  }
}

function createDefaultLabelFormState(): CreateLabelFormState {
  return {
    name: '',
    color: CREATE_LABEL_DEFAULT_COLOR,
    description: '',
  }
}

function normalizeCreateLabelColor(value: string): string {
  const normalized = value.trim()
  if (!normalized) {
    return ''
  }
  const withPrefix = normalized.startsWith('#') ? normalized : `#${normalized}`
  return withPrefix.toLowerCase()
}

function resolveCreateLabelPickerColor(value: string): string {
  const normalized = normalizeCreateLabelColor(value)
  if (CREATE_LABEL_COLOR_REGEX.test(normalized)) {
    return normalized
  }
  return CREATE_LABEL_DEFAULT_COLOR
}

function validateCreateLabelForm(form: CreateLabelFormState): CreateLabelValidationErrors {
  const errors: CreateLabelValidationErrors = {}
  if (!form.name.trim()) {
    errors.name = 'Nome obrigatorio.'
  }

  const normalizedColor = normalizeCreateLabelColor(form.color)
  if (!normalizedColor) {
    errors.color = 'Cor obrigatoria.'
  } else if (!CREATE_LABEL_COLOR_REGEX.test(normalizedColor)) {
    errors.color = 'Cor invalida. Use formato hexadecimal de 6 caracteres.'
  }

  const descriptionLength = form.description.trim().length
  if (descriptionLength > CREATE_LABEL_MAX_DESCRIPTION_CHARS) {
    errors.description = `Descricao limitada a ${CREATE_LABEL_MAX_DESCRIPTION_CHARS} caracteres.`
  }

  return errors
}

function validateCreatePRForm(form: CreatePRFormState): CreatePRValidationErrors {
  const errors: CreatePRValidationErrors = {}
  if (!form.title.trim()) {
    errors.title = 'Titulo obrigatorio.'
  }
  const parsedHead = parseCreatePRHeadReference(form.head)
  if (!parsedHead.valid) {
    errors.head = parsedHead.errorMessage || 'Head obrigatoria.'
  }
  const normalizedBase = form.base.trim()
  if (!normalizedBase) {
    errors.base = 'Base obrigatoria.'
  } else if (/\s/.test(normalizedBase)) {
    errors.base = 'Base invalida. Remova espacos.'
  }

  if (form.advancedMode) {
    const normalizedTargetOwner = form.targetOwner.trim()
    const normalizedTargetRepo = form.targetRepo.trim()
    const hasOwner = normalizedTargetOwner !== ''
    const hasRepo = normalizedTargetRepo !== ''
    if (hasOwner !== hasRepo) {
      const message = 'Preencha owner e repo para override manual.'
      errors.targetOwner = message
      errors.targetRepo = message
    }
  }

  return errors
}

interface ParsedHeadReference {
  owner: string
  branch: string
  hasOwner: boolean
  valid: boolean
  errorMessage?: string
}

function parseCreatePRHeadReference(value: string): ParsedHeadReference {
  const normalized = value.trim()
  if (!normalized) {
    return {
      owner: '',
      branch: '',
      hasOwner: false,
      valid: false,
      errorMessage: 'Head obrigatoria.',
    }
  }

  if (/\s/.test(normalized)) {
    return {
      owner: '',
      branch: '',
      hasOwner: false,
      valid: false,
      errorMessage: 'Head invalida. Use "branch" ou "owner:branch", sem espacos.',
    }
  }

  const separatorCount = (normalized.match(/:/g) || []).length
  if (separatorCount > 1) {
    return {
      owner: '',
      branch: '',
      hasOwner: false,
      valid: false,
      errorMessage: 'Head invalida. Use apenas um separador ":" (formato owner:branch).',
    }
  }

  if (separatorCount === 1) {
    const [rawOwner, rawBranch] = normalized.split(':')
    const owner = (rawOwner || '').trim()
    const branch = (rawBranch || '').trim()
    if (!owner || !branch) {
      return {
        owner: '',
        branch: '',
        hasOwner: false,
        valid: false,
        errorMessage: 'Head invalida. Use formato owner:branch com ambos os valores.',
      }
    }
    return {
      owner,
      branch,
      hasOwner: true,
      valid: true,
    }
  }

  return {
    owner: '',
    branch: normalized,
    hasOwner: false,
    valid: true,
  }
}

function normalizeCreatePRBranchName(value: unknown): string {
  if (typeof value !== 'string') {
    return ''
  }
  return value.trim()
}

function normalizeCreatePRAhead(value: unknown): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return 0
  }
  const normalized = Math.floor(value)
  if (normalized < 0) {
    return 0
  }
  return normalized
}

function buildCreatePRBaseOptions(branchNames: Set<string>): string[] {
  const options = new Set<string>()
  for (const name of branchNames) {
    const normalized = name.trim()
    if (!normalized) {
      continue
    }
    options.add(normalized)
  }
  options.add(CREATE_PR_DEFAULT_BASE)

  return Array.from(options).sort((left, right) => {
    if (left === CREATE_PR_DEFAULT_BASE && right !== CREATE_PR_DEFAULT_BASE) {
      return -1
    }
    if (right === CREATE_PR_DEFAULT_BASE && left !== CREATE_PR_DEFAULT_BASE) {
      return 1
    }
    return left.localeCompare(right)
  })
}

function selectCreatePRBase(currentBase: string, options: string[]): string {
  const normalizedCurrentBase = currentBase.trim()
  if (normalizedCurrentBase && options.includes(normalizedCurrentBase)) {
    return normalizedCurrentBase
  }
  if (options.includes(CREATE_PR_DEFAULT_BASE)) {
    return CREATE_PR_DEFAULT_BASE
  }
  if (options.length > 0) {
    return options[0]
  }
  return CREATE_PR_DEFAULT_BASE
}

function createEditPRFormState(pr: GitPanelPullRequest): EditPRFormState {
  return {
    title: pr.title || '',
    body: pr.body || '',
    base: pr.baseBranch || '',
    state: normalizeWritablePRState(pr.state),
    maintainerCanModify: normalizeMaintainerCanModify(pr.maintainerCanModify),
  }
}

function validateEditPRForm(form: EditPRFormState): EditPRValidationErrors {
  const errors: EditPRValidationErrors = {}
  if (!form.title.trim()) {
    errors.title = 'Titulo obrigatorio.'
  }
  if (!form.base.trim()) {
    errors.base = 'Base obrigatoria.'
  }
  return errors
}

function normalizeWritablePRState(state: string): EditPRWritableState {
  const normalized = normalizeStateForClass(state)
  if (normalized === 'open') {
    return 'open'
  }
  return 'closed'
}

function normalizeMaintainerCanModify(value: unknown): boolean {
  if (typeof value === 'boolean') {
    return value
  }
  return true
}

function isMergedPRState(state: string): boolean {
  return normalizeStateForClass(state) === 'merged'
}

function normalizePRNumber(value: unknown): number | null {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return null
  }
  const rounded = Math.floor(value)
  if (rounded <= 0) {
    return null
  }
  return rounded
}

function resolvePRUpdateEventTarget(payload: unknown): { owner: string; repo: string } | null {
  if (!payload || typeof payload !== 'object') {
    return null
  }

  const candidate = payload as { owner?: unknown; repo?: unknown }
  const owner = typeof candidate.owner === 'string' ? candidate.owner.trim() : ''
  const repo = typeof candidate.repo === 'string' ? candidate.repo.trim() : ''
  if (!owner || !repo) {
    return null
  }

  return { owner, repo }
}

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms)
  })
}
