import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertTriangle,
  ArrowLeft,
  Check,
  Clock3,
  ExternalLink,
  FileCode2,
  FolderOpen,
  GitBranch,
  GitMerge,
  History,
  Loader2,
  Plus,
  Search,
  ShieldAlert,
  Terminal,
} from 'lucide-react'
import {
  GitPanelAcceptOurs,
  GitPanelAcceptTheirs,
  GitPanelGetDiff,
  GitPanelGetHistory,
  GitPanelOpenExternalMergeTool,
  GitPanelPickRepositoryDirectory,
  GitPanelGetStatus,
  GitPanelPreflight,
  GitPanelStageFile,
  GitPanelStagePatch,
  GitPanelUnstageFile,
} from '../../../../wailsjs/go/main/App'
import { useWorkspaceStore } from '../../../stores/workspaceStore'
import './GitPanelScreen.css'
import commitHorizontalIconSVG from '../assets/svg/git-commit-horizontal.svg?raw'

interface GitPanelScreenProps {
  onBack: () => void
}

interface GitPanelBindingError {
  code: string
  message: string
  details?: string
}

export interface GitPanelCommandResult {
  commandId: string
  repoPath: string
  action: string
  args?: string[]
  durationMs: number
  exitCode: number
  stderrSanitized?: string
  status: string
  error?: string
}

interface GitPanelPreflightResult {
  gitAvailable: boolean
  repoPath: string
  repoRoot: string
  branch?: string
  mergeActive?: boolean
}

interface GitPanelFileChange {
  path: string
  originalPath?: string
  status: string
  added: number
  removed: number
}

interface GitPanelConflictFile {
  path: string
  status: string
}

interface GitPanelStatusDTO {
  branch: string
  ahead: number
  behind: number
  staged: GitPanelFileChange[]
  unstaged: GitPanelFileChange[]
  conflicted: GitPanelConflictFile[]
}

interface GitPanelHistoryItem {
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
}

interface GitPanelHistoryPageDTO {
  items: GitPanelHistoryItem[]
  nextCursor: string
  hasMore: boolean
}

interface GitPanelHistoryAuthorUpdate {
  hash: string
  githubLogin?: string
  githubAvatarUrl?: string
}

interface GitPanelHistoryAuthorsEnrichedEvent {
  repoPath?: string
  items?: GitPanelHistoryAuthorUpdate[]
}

interface GitPanelDiffLine {
  type: 'add' | 'delete' | 'context' | 'meta'
  content: string
  oldLine?: number
  newLine?: number
}

interface GitPanelDiffHunk {
  header: string
  oldStart: number
  oldLines: number
  newStart: number
  newLines: number
  lines: GitPanelDiffLine[]
}

interface GitPanelDiffFile {
  path: string
  oldPath?: string
  status: string
  additions: number
  deletions: number
  isBinary: boolean
  hunks: GitPanelDiffHunk[]
}

interface GitPanelDiffDTO {
  mode: string
  filePath?: string
  raw: string
  files: GitPanelDiffFile[]
  isBinary: boolean
  isTruncated: boolean
}

interface DiffCandidate {
  path: string
  status: string
  bucket: 'staged' | 'unstaged' | 'conflicted'
}

interface QuickRefItem {
  id: string
  label: string
  meta: string
  kind: 'branch' | 'head' | 'commit'
  commitHash?: string
}

type InspectorTab = 'working-tree' | 'commit' | 'diff' | 'conflicts'
type DiffViewMode = 'unified' | 'split'
type GitPanelFocusScope = 'sidebar' | 'log' | 'inspector' | 'actions'

type VirtualHistoryRow =
  | { kind: 'item'; index: number; item: GitPanelHistoryItem }
  | { kind: 'loading'; index: number }

const HISTORY_PAGE_SIZE = 200
const HISTORY_ROW_HEIGHT = 92
const HISTORY_OVERSCAN = 8
const HISTORY_LOAD_MORE_THRESHOLD = HISTORY_ROW_HEIGHT * 10

/**
 * GitPanelScreen — host dedicado do Git Panel, fora do mosaico de terminais.
 * Carrega preflight, status e histórico linear virtualizado com inspector por abas.
 */
export function GitPanelScreen({ onBack }: GitPanelScreenProps) {
  const activeWorkspaceId = useWorkspaceStore((state) => state.activeWorkspaceId)
  const workspaces = useWorkspaceStore((state) => state.workspaces)
  const activeWorkspace = useMemo(() => {
    if (!activeWorkspaceId) {
      return null
    }
    return workspaces.find((workspace) => workspace.id === activeWorkspaceId) ?? null
  }, [activeWorkspaceId, workspaces])
  const workspacePath = activeWorkspace?.path ?? ''

  const [repoPathInput, setRepoPathInput] = useState(workspacePath)
  const [activeRepoPath, setActiveRepoPath] = useState(workspacePath)
  const [resolvedRepoPath, setResolvedRepoPath] = useState('')
  const [preflight, setPreflight] = useState<GitPanelPreflightResult | null>(null)
  const [status, setStatus] = useState<GitPanelStatusDTO | null>(null)
  const [historyItems, setHistoryItems] = useState<GitPanelHistoryItem[]>([])
  const [historyCursor, setHistoryCursor] = useState('')
  const [historyHasMore, setHistoryHasMore] = useState(false)
  const [selectedCommitHash, setSelectedCommitHash] = useState('')
  const [selectedRefID, setSelectedRefID] = useState('')
  const [selectedDiffPath, setSelectedDiffPath] = useState('')
  const [activeInspectorTab, setActiveInspectorTab] = useState<InspectorTab>('working-tree')
  const [diffViewMode, setDiffViewMode] = useState<DiffViewMode>('unified')
  const [diffData, setDiffData] = useState<GitPanelDiffDTO | null>(null)
  const [isDiffLoading, setIsDiffLoading] = useState(false)
  const [diffError, setDiffError] = useState<GitPanelBindingError | null>(null)
  const [isInitialLoading, setIsInitialLoading] = useState(false)
  const [isHistoryLoadingMore, setIsHistoryLoadingMore] = useState(false)
  const [error, setError] = useState<GitPanelBindingError | null>(null)
  const [lastUpdatedAt, setLastUpdatedAt] = useState<number | null>(null)
  const [historyScrollTop, setHistoryScrollTop] = useState(0)
  const [historyViewportHeight, setHistoryViewportHeight] = useState(0)
  const [historySearch, setHistorySearch] = useState('')
  const [debouncedHistorySearch, setDebouncedHistorySearch] = useState('')

  useEffect(() => {
    const handler = setTimeout(() => {
      setDebouncedHistorySearch(historySearch)
    }, 400)
    return () => clearTimeout(handler)
  }, [historySearch])

  // Hunk staging state
  const [stagingHunkIndex, setStagingHunkIndex] = useState<number | null>(null)
  const [hunkStageResult, setHunkStageResult] = useState<{ index: number; status: 'success' | 'error'; message?: string } | null>(null)

  // Line selection state (for partial line staging via Shift+Click)
  const [selectedLines, setSelectedLines] = useState<Set<string>>(new Set())
  const lastClickedLineRef = useRef<string | null>(null)
  const [isStagingLines, setIsStagingLines] = useState(false)
  const [lineStageResult, setLineStageResult] = useState<{ status: 'success' | 'error'; message?: string } | null>(null)

  // File local action state
  const [fileActions, setFileActions] = useState<Record<string, { status: 'running' | 'success' | 'error'; message?: string }>>({})
  const [conflictAutoStage, setConflictAutoStage] = useState(true)

  // Console state
  const [consoleLogs, setConsoleLogs] = useState<GitPanelCommandResult[]>([])
  const [isConsoleOpen, setIsConsoleOpen] = useState(false)

  // Diff degradation state (load-anyway for truncated diffs)
  const [diffForceLoad, setDiffForceLoad] = useState(false)
  const [diffContextLines, setDiffContextLines] = useState(3)
  const [isRepoPickerOpen, setIsRepoPickerOpen] = useState(false)

  const historyViewportRef = useRef<HTMLDivElement | null>(null)
  const sidebarRegionRef = useRef<HTMLElement | null>(null)
  const inspectorRegionRef = useRef<HTMLElement | null>(null)
  const actionsRegionRef = useRef<HTMLElement | null>(null)
  const requestTokenRef = useRef(0)
  const diffRequestTokenRef = useRef(0)

  // --- Wails Diagnostic Events Console ---
  useEffect(() => {
    // @ts-ignore
    const off = window.runtime?.EventsOn('gitpanel:command_result', (result: GitPanelCommandResult) => {
      setConsoleLogs(prev => [result, ...prev].slice(0, 50))
    })
    return () => { off?.() }
  }, [])

  useEffect(() => {
    // @ts-ignore
    const off = window.runtime?.EventsOn('gitpanel:history_authors_enriched', (payload: GitPanelHistoryAuthorsEnrichedEvent) => {
      const payloadRepoPath = (payload?.repoPath || '').trim()
      if (payloadRepoPath === '' || payloadRepoPath !== resolvedRepoPath.trim()) {
        return
      }
      const incomingItems = payload?.items ?? []
      if (incomingItems.length === 0) {
        return
      }

      const updates = new Map<string, GitPanelHistoryAuthorUpdate>()
      for (const item of incomingItems) {
        const hash = (item?.hash || '').trim().toLowerCase()
        if (hash === '') {
          continue
        }
        updates.set(hash, item)
      }
      if (updates.size === 0) {
        return
      }

      setHistoryItems((previous) => {
        let changed = false
        const next = previous.map((item) => {
          const update = updates.get(item.hash.toLowerCase())
          if (!update) {
            return item
          }

          const nextLogin = (update.githubLogin || '').trim() || item.githubLogin
          const nextAvatar = (update.githubAvatarUrl || '').trim() || item.githubAvatarUrl
          if (nextLogin === item.githubLogin && nextAvatar === item.githubAvatarUrl) {
            return item
          }

          changed = true
          return {
            ...item,
            githubLogin: nextLogin,
            githubAvatarUrl: nextAvatar,
          }
        })
        return changed ? next : previous
      })
    })
    return () => { off?.() }
  }, [resolvedRepoPath])


  // Scroll sync refs for side-by-side mode
  const splitLeftRef = useRef<HTMLDivElement | null>(null)
  const splitRightRef = useRef<HTMLDivElement | null>(null)
  const scrollSyncSourceRef = useRef<'left' | 'right' | null>(null)

  const selectedCommit = useMemo(
    () => historyItems.find((item) => item.hash === selectedCommitHash) ?? null,
    [historyItems, selectedCommitHash],
  )

  const diffCandidates = useMemo(() => buildDiffCandidates(status), [status])

  const selectedDiffFile = useMemo(() => {
    if (!diffData?.files || diffData.files.length === 0) {
      return null
    }

    return diffData.files.find((file) => file.path === selectedDiffPath) ?? diffData.files[0]
  }, [diffData, selectedDiffPath])

  const branchName = status?.branch || preflight?.branch || 'Detached/Unknown'

  const quickRefs = useMemo<QuickRefItem[]>(() => {
    const refs: QuickRefItem[] = []

    if (branchName.trim() !== '') {
      refs.push({
        id: `branch:${branchName}`,
        label: branchName,
        meta: 'Current branch',
        kind: 'branch',
      })
    }

    refs.push({
      id: 'HEAD',
      label: 'HEAD',
      meta: 'Current pointer',
      kind: 'head',
    })

    for (const item of historyItems.slice(0, 6)) {
      refs.push({
        id: `commit:${item.hash}`,
        label: item.shortHash,
        meta: item.subject || 'Commit sem subject',
        kind: 'commit',
        commitHash: item.hash,
      })
    }

    return refs
  }, [branchName, historyItems])

  useEffect(() => {
    setRepoPathInput(workspacePath)
    setActiveRepoPath(workspacePath)
  }, [workspacePath])

  useEffect(() => {
    if (quickRefs.length === 0) {
      setSelectedRefID('')
      return
    }

    setSelectedRefID((previous) => {
      if (previous && quickRefs.some((item) => item.id === previous)) {
        return previous
      }
      return quickRefs[0].id
    })
  }, [quickRefs])

  useEffect(() => {
    if (diffCandidates.length === 0) {
      setSelectedDiffPath('')
      setDiffData(null)
      setDiffError(null)
      setIsDiffLoading(false)
      return
    }

    setSelectedDiffPath((previous) => {
      if (previous && diffCandidates.some((item) => item.path === previous)) {
        return previous
      }
      return diffCandidates[0].path
    })
  }, [diffCandidates])

  useEffect(() => {
    if (activeInspectorTab !== 'diff') {
      return
    }
    if (diffCandidates.length > 0) {
      return
    }
    setActiveInspectorTab('working-tree')
  }, [activeInspectorTab, diffCandidates.length])

  useEffect(() => {
    const node = historyViewportRef.current
    if (!node) {
      return
    }

    const updateHeight = () => {
      setHistoryViewportHeight(node.clientHeight)
    }

    updateHeight()
    if (typeof ResizeObserver !== 'undefined') {
      const observer = new ResizeObserver(updateHeight)
      observer.observe(node)
      return () => observer.disconnect()
    }

    window.addEventListener('resize', updateHeight)
    return () => window.removeEventListener('resize', updateHeight)
  }, [])

  const loadSnapshot = useCallback(async (repoCandidate: string) => {
    const repoPath = repoCandidate.trim()
    const requestToken = requestTokenRef.current + 1
    requestTokenRef.current = requestToken

    if (repoPath === '') {
      setPreflight(null)
      setStatus(null)
      setResolvedRepoPath('')
      setHistoryItems([])
      setHistoryCursor('')
      setHistoryHasMore(false)
      setSelectedCommitHash('')
      setSelectedDiffPath('')
      setDiffData(null)
      setDiffError(null)
      setError({
        code: 'E_REPO_NOT_RESOLVED',
        message: 'Repositório não resolvido.',
        details: 'Selecione um workspace Git válido ou informe um caminho manual.',
      })
      return
    }

    setIsInitialLoading(true)
    setError(null)
    setIsHistoryLoadingMore(false)

    try {
      const preflightResultRaw = await GitPanelPreflight(repoPath)
      if (requestTokenRef.current != requestToken) {
        return
      }

      const preflightResult = preflightResultRaw as unknown as GitPanelPreflightResult
      const normalizedRepo = (preflightResult.repoRoot || preflightResult.repoPath || repoPath).trim()

      const [statusRaw, historyRaw] = await Promise.all([
        GitPanelGetStatus(normalizedRepo),
        (GitPanelGetHistory as any)(normalizedRepo, '', HISTORY_PAGE_SIZE, debouncedHistorySearch),
      ])
      if (requestTokenRef.current != requestToken) {
        return
      }

      const statusResult = statusRaw as unknown as GitPanelStatusDTO
      const historyResult = normalizeHistoryPage(historyRaw)

      setPreflight(preflightResult)
      setStatus(statusResult)
      setResolvedRepoPath(normalizedRepo)
      setHistoryItems(historyResult.items)
      setHistoryCursor((historyResult.nextCursor || '').trim())
      setHistoryHasMore(Boolean(historyResult.hasMore))
      setSelectedCommitHash((previous) => {
        const incoming = historyResult.items
        if (previous && incoming.some((item) => item.hash === previous)) {
          return previous
        }
        return incoming[0]?.hash ?? ''
      })
      setLastUpdatedAt(Date.now())
      setHistoryScrollTop(0)
      if (historyViewportRef.current) {
        historyViewportRef.current.scrollTop = 0
      }
    } catch (unknownError) {
      if (requestTokenRef.current != requestToken) {
        return
      }
      setError(parseGitPanelError(unknownError))
      setPreflight(null)
      setStatus(null)
      setResolvedRepoPath('')
      setHistoryItems([])
      setHistoryCursor('')
      setHistoryHasMore(false)
      setSelectedCommitHash('')
      setSelectedDiffPath('')
      setDiffData(null)
      setDiffError(null)
    } finally {
      if (requestTokenRef.current == requestToken) {
        setIsInitialLoading(false)
      }
    }
  }, [debouncedHistorySearch])

  const loadMoreHistory = useCallback(async () => {
    if (isInitialLoading || isHistoryLoadingMore) {
      return
    }
    if (!historyHasMore || historyCursor.trim() === '' || resolvedRepoPath.trim() === '') {
      return
    }

    setIsHistoryLoadingMore(true)
    try {
      const nextPageRaw = await (GitPanelGetHistory as any)(resolvedRepoPath, historyCursor, HISTORY_PAGE_SIZE, debouncedHistorySearch)
      const nextPage = normalizeHistoryPage(nextPageRaw)
      setHistoryItems((previous) => mergeHistoryItems(previous, nextPage.items))
      setHistoryCursor((nextPage.nextCursor || '').trim())
      setHistoryHasMore(Boolean(nextPage.hasMore))
      setLastUpdatedAt(Date.now())
    } catch (unknownError) {
      setError(parseGitPanelError(unknownError))
    } finally {
      setIsHistoryLoadingMore(false)
    }
  }, [historyCursor, historyHasMore, isHistoryLoadingMore, isInitialLoading, resolvedRepoPath, debouncedHistorySearch])

  useEffect(() => {
    void loadSnapshot(activeRepoPath)
  }, [activeRepoPath, loadSnapshot])

  useEffect(() => {
    const repoPath = resolvedRepoPath.trim()
    const filePath = selectedDiffPath.trim()

    if (repoPath === '' || filePath === '') {
      setDiffData(null)
      setDiffError(null)
      setIsDiffLoading(false)
      return
    }

    const requestToken = diffRequestTokenRef.current + 1
    diffRequestTokenRef.current = requestToken
    setIsDiffLoading(true)
    setDiffError(null)

    GitPanelGetDiff(repoPath, filePath, 'unified', diffContextLines)
      .then((rawResult) => {
        if (diffRequestTokenRef.current !== requestToken) {
          return
        }
        setDiffData(rawResult as unknown as GitPanelDiffDTO)
      })
      .catch((unknownError) => {
        if (diffRequestTokenRef.current !== requestToken) {
          return
        }
        setDiffData(null)
        setDiffError(parseGitPanelError(unknownError))
      })
      .finally(() => {
        if (diffRequestTokenRef.current === requestToken) {
          setIsDiffLoading(false)
        }
      })
  }, [resolvedRepoPath, selectedDiffPath])

  const totalVirtualRows = historyItems.length + (isHistoryLoadingMore ? 1 : 0)
  const totalVirtualHeight = totalVirtualRows * HISTORY_ROW_HEIGHT
  const safeViewportHeight = Math.max(historyViewportHeight, HISTORY_ROW_HEIGHT)
  const startIndex = Math.max(0, Math.floor(historyScrollTop / HISTORY_ROW_HEIGHT) - HISTORY_OVERSCAN)
  const visibleRowCount = Math.ceil(safeViewportHeight / HISTORY_ROW_HEIGHT) + HISTORY_OVERSCAN * 2
  const endIndex = Math.min(totalVirtualRows, startIndex + visibleRowCount)

  const virtualRows = useMemo<VirtualHistoryRow[]>(() => {
    const rows: VirtualHistoryRow[] = []
    for (let index = startIndex; index < endIndex; index += 1) {
      if (index >= historyItems.length) {
        rows.push({ kind: 'loading', index })
      } else {
        rows.push({ kind: 'item', index, item: historyItems[index] })
      }
    }
    return rows
  }, [endIndex, historyItems, startIndex])

  const paddingTop = startIndex * HISTORY_ROW_HEIGHT
  const paddingBottom = Math.max(0, totalVirtualHeight - paddingTop - virtualRows.length * HISTORY_ROW_HEIGHT)

  useEffect(() => {
    if (!historyHasMore || isHistoryLoadingMore || isInitialLoading) {
      return
    }
    if (totalVirtualHeight <= 0) {
      return
    }
    const bottomOffset = historyScrollTop + safeViewportHeight
    if (bottomOffset >= totalVirtualHeight - HISTORY_LOAD_MORE_THRESHOLD) {
      void loadMoreHistory()
    }
  }, [
    historyHasMore,
    historyScrollTop,
    isHistoryLoadingMore,
    isInitialLoading,
    loadMoreHistory,
    safeViewportHeight,
    totalVirtualHeight,
  ])

  const handlePickRepositoryDirectory = useCallback(async () => {
    if (isRepoPickerOpen) {
      return
    }

    setIsRepoPickerOpen(true)
    try {
      const defaultPath = repoPathInput.trim() || resolvedRepoPath.trim() || workspacePath.trim()
      const selectedPath = await GitPanelPickRepositoryDirectory(defaultPath)
      const normalizedPath = (selectedPath || '').trim()
      if (normalizedPath === '') {
        return
      }

      setRepoPathInput(normalizedPath)
      setActiveRepoPath(normalizedPath)
      setError(null)
    } catch (err) {
      setError(parseGitPanelError(err))
    } finally {
      setIsRepoPickerOpen(false)
    }
  }, [isRepoPickerOpen, repoPathInput, resolvedRepoPath, workspacePath])

  const handleRefSelection = useCallback((ref: QuickRefItem) => {
    setSelectedRefID(ref.id)
    if (ref.kind === 'commit' && ref.commitHash) {
      setSelectedCommitHash(ref.commitHash)
      setActiveInspectorTab('commit')
    }
  }, [])

  const runStageActionForFile = useCallback(async (filePath: string, action: 'stage' | 'unstage') => {
    const cleanPath = filePath.trim()
    if (!cleanPath || !resolvedRepoPath || fileActions[cleanPath]) {
      return
    }

    setFileActions(prev => ({ ...prev, [cleanPath]: { status: 'running' } }))
    try {
      if (action === 'stage') {
        await GitPanelStageFile(resolvedRepoPath, cleanPath)
      } else {
        await GitPanelUnstageFile(resolvedRepoPath, cleanPath)
      }
      setFileActions(prev => ({ ...prev, [cleanPath]: { status: 'success' } }))
    } catch (err) {
      const parsed = parseGitPanelError(err)
      setFileActions(prev => ({ ...prev, [cleanPath]: { status: 'error', message: parsed.message } }))
    } finally {
      setTimeout(() => {
        setFileActions(prev => {
          const next = { ...prev }
          delete next[cleanPath]
          return next
        })
      }, 900)
    }
  }, [resolvedRepoPath, fileActions])

  const moveDiffSelection = useCallback((delta: -1 | 1) => {
    if (!status) {
      return
    }

    const allFiles = [...status.staged, ...status.unstaged, ...status.conflicted].map((file) => file.path)
    if (allFiles.length === 0) {
      return
    }

    const currentIndex = allFiles.indexOf(selectedDiffPath)
    const base = currentIndex >= 0 ? currentIndex : 0
    const nextIndex = Math.max(0, Math.min(allFiles.length - 1, base + delta))
    const nextPath = allFiles[nextIndex]
    if (!nextPath) {
      return
    }

    setSelectedDiffPath(nextPath)
    setActiveInspectorTab('diff')
  }, [selectedDiffPath, status])

  const moveHistorySelection = useCallback((delta: -1 | 1) => {
    if (historyItems.length === 0) {
      return
    }

    const currentIndex = historyItems.findIndex((item) => item.hash === selectedCommitHash)
    const base = currentIndex >= 0 ? currentIndex : 0
    const nextIndex = Math.max(0, Math.min(historyItems.length - 1, base + delta))
    const nextCommit = historyItems[nextIndex]
    if (!nextCommit) {
      return
    }

    setSelectedCommitHash(nextCommit.hash)
    setActiveInspectorTab('commit')
    requestAnimationFrame(() => {
      const row = document.querySelector<HTMLButtonElement>(`[data-commit-hash="${nextCommit.hash}"]`)
      row?.focus({ preventScroll: true })
      row?.scrollIntoView({ block: 'nearest' })
    })
  }, [historyItems, selectedCommitHash])

  const moveRefSelection = useCallback((delta: -1 | 1) => {
    if (quickRefs.length === 0) {
      return
    }

    const currentIndex = quickRefs.findIndex((item) => item.id === selectedRefID)
    const base = currentIndex >= 0 ? currentIndex : 0
    const nextIndex = Math.max(0, Math.min(quickRefs.length - 1, base + delta))
    const nextRef = quickRefs[nextIndex]
    if (!nextRef) {
      return
    }
    handleRefSelection(nextRef)
  }, [handleRefSelection, quickRefs, selectedRefID])

  const resolveKeyboardScope = useCallback((): GitPanelFocusScope => {
    const active = document.activeElement
    if (active && sidebarRegionRef.current?.contains(active)) {
      return 'sidebar'
    }
    if (active && historyViewportRef.current?.contains(active)) {
      return 'log'
    }
    if (active && inspectorRegionRef.current?.contains(active)) {
      return 'inspector'
    }
    return 'actions'
  }, [])

  const focusScope = useCallback((scope: GitPanelFocusScope) => {
    if (scope === 'sidebar') {
      sidebarRegionRef.current?.focus()
      return
    }
    if (scope === 'log') {
      historyViewportRef.current?.focus()
      return
    }
    if (scope === 'inspector') {
      inspectorRegionRef.current?.focus()
      return
    }
    const defaultAction = actionsRegionRef.current?.querySelector<HTMLElement>(
      '.git-panel-screen__back, .git-panel-screen__repo-picker-btn, button, [tabindex]:not([tabindex="-1"])',
    )
    defaultAction?.focus()
  }, [])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (isEditableElement(event.target)) {
        return
      }

      const key = event.key.toLowerCase()
      const activeFile = selectedDiffPath.trim()
      const hasPrimaryModifier = event.metaKey || event.ctrlKey

      if (event.key === 'Escape') {
        event.preventDefault()
        setIsConsoleOpen(false)
        onBack()
        return
      }

      if (event.altKey && !event.metaKey && !event.ctrlKey && !event.shiftKey) {
        if (key === '1') {
          event.preventDefault()
          focusScope('sidebar')
          return
        }
        if (key === '2') {
          event.preventDefault()
          focusScope('log')
          return
        }
        if (key === '3') {
          event.preventDefault()
          focusScope('inspector')
          return
        }
        if (key === '4') {
          event.preventDefault()
          focusScope('actions')
          return
        }
      }

      if (hasPrimaryModifier && key === 's') {
        event.preventDefault()
        void runStageActionForFile(activeFile, event.shiftKey ? 'unstage' : 'stage')
        return
      }

      const isForward = key === 'j' || event.key === 'ArrowDown'
      const isBackward = key === 'k' || event.key === 'ArrowUp'
      if (!isForward && !isBackward) {
        return
      }
      if (event.metaKey || event.ctrlKey || event.altKey) {
        return
      }

      event.preventDefault()
      const delta: -1 | 1 = isForward ? 1 : -1
      const scope = resolveKeyboardScope()

      if (scope === 'sidebar') {
        moveRefSelection(delta)
        return
      }
      if (scope === 'log') {
        moveHistorySelection(delta)
        return
      }
      moveDiffSelection(delta)
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [
    selectedDiffPath,
    onBack,
    focusScope,
    resolveKeyboardScope,
    moveRefSelection,
    moveHistorySelection,
    moveDiffSelection,
    runStageActionForFile,
  ])

  const stagedCount = status?.staged.length ?? 0
  const unstagedCount = status?.unstaged.length ?? 0
  const conflictedCount = status?.conflicted.length ?? 0
  const selectedRepoPath = (resolvedRepoPath || repoPathInput || activeRepoPath).trim()
  const hasSelectedRepo = selectedRepoPath !== ''

  // --- Clear line selection when diff changes ---
  useEffect(() => {
    setSelectedLines(new Set())
    lastClickedLineRef.current = null
    setLineStageResult(null)
    setHunkStageResult(null)
    setDiffForceLoad(false)
  }, [selectedDiffPath, diffData])

  // --- Patch generation: full hunk ---
  const buildHunkPatch = useCallback((file: GitPanelDiffFile, hunk: GitPanelDiffHunk): string => {
    const filePath = file.path
    const oldPath = file.oldPath || filePath
    const lines: string[] = []
    lines.push(`diff --git a/${oldPath} b/${filePath}`)
    lines.push(`--- a/${oldPath}`)
    lines.push(`+++ b/${filePath}`)
    lines.push(`@@ -${hunk.oldStart},${hunk.oldLines} +${hunk.newStart},${hunk.newLines} @@${hunk.header ? ' ' + hunk.header : ''}`)
    for (const line of hunk.lines) {
      if (line.type === 'add') {
        lines.push('+' + line.content)
      } else if (line.type === 'delete') {
        lines.push('-' + line.content)
      } else if (line.type === 'context') {
        lines.push(' ' + line.content)
      } else if (line.type === 'meta') {
        lines.push(line.content)
      }
    }
    return lines.join('\n') + '\n'
  }, [])

  // --- Patch generation: selected lines only ---
  const buildLinesPatch = useCallback((file: GitPanelDiffFile, selectedKeys: Set<string>): string | null => {
    if (selectedKeys.size === 0) return null

    const filePath = file.path
    const oldPath = file.oldPath || filePath
    const patchLines: string[] = []
    patchLines.push(`diff --git a/${oldPath} b/${filePath}`)
    patchLines.push(`--- a/${oldPath}`)
    patchLines.push(`+++ b/${filePath}`)

    for (let hunkIdx = 0; hunkIdx < file.hunks.length; hunkIdx++) {
      const hunk = file.hunks[hunkIdx]
      const hunkLines: string[] = []
      let oldCount = 0
      let newCount = 0
      let hasSelected = false

      for (let lineIdx = 0; lineIdx < hunk.lines.length; lineIdx++) {
        const line = hunk.lines[lineIdx]
        const key = `${hunkIdx}:${lineIdx}`
        const isSelected = selectedKeys.has(key)

        if (line.type === 'context') {
          hunkLines.push(' ' + line.content)
          oldCount++
          newCount++
        } else if (line.type === 'add') {
          if (isSelected) {
            hunkLines.push('+' + line.content)
            newCount++
            hasSelected = true
          } else {
            // Unselected add: skip (don't include in patch)
          }
        } else if (line.type === 'delete') {
          if (isSelected) {
            hunkLines.push('-' + line.content)
            oldCount++
            hasSelected = true
          } else {
            // Unselected delete: treat as context (keep line in both sides)
            hunkLines.push(' ' + line.content)
            oldCount++
            newCount++
          }
        } else if (line.type === 'meta') {
          hunkLines.push(line.content)
        }
      }

      if (hasSelected) {
        patchLines.push(`@@ -${hunk.oldStart},${oldCount} +${hunk.newStart},${newCount} @@${hunk.header ? ' ' + hunk.header : ''}`)
        patchLines.push(...hunkLines)
      }
    }

    // Check if we actually added any hunk
    if (patchLines.length <= 3) return null

    return patchLines.join('\n') + '\n'
  }, [])

  // --- Handler: Stage a whole hunk ---
  const handleStageHunk = useCallback(async (hunkIndex: number) => {
    if (!selectedDiffFile || !resolvedRepoPath) return
    const hunk = selectedDiffFile.hunks[hunkIndex]
    if (!hunk) return

    setStagingHunkIndex(hunkIndex)
    setHunkStageResult(null)

    try {
      const patchText = buildHunkPatch(selectedDiffFile, hunk)
      await GitPanelStagePatch(resolvedRepoPath, patchText)
      setHunkStageResult({ index: hunkIndex, status: 'success' })
      // Refresh status + diff after successful stage
      const [statusRaw] = await Promise.all([
        GitPanelGetStatus(resolvedRepoPath),
        GitPanelGetDiff(resolvedRepoPath, selectedDiffPath, 'unified', diffContextLines),
      ]).then(([s, d]) => {
        setStatus(s as unknown as GitPanelStatusDTO)
        setDiffData(d as unknown as GitPanelDiffDTO)
        return [s]
      })
      void statusRaw
    } catch (err) {
      const parsed = parseGitPanelError(err)
      setHunkStageResult({ index: hunkIndex, status: 'error', message: parsed.message })
    } finally {
      setStagingHunkIndex(null)
      setTimeout(() => setHunkStageResult(null), 3000)
    }
  }, [selectedDiffFile, resolvedRepoPath, selectedDiffPath, buildHunkPatch])

  // --- Handler: Stage selected lines ---
  const handleStageSelectedLines = useCallback(async () => {
    if (!selectedDiffFile || !resolvedRepoPath || selectedLines.size === 0) return

    setIsStagingLines(true)
    setLineStageResult(null)

    try {
      const patchText = buildLinesPatch(selectedDiffFile, selectedLines)
      if (!patchText) {
        setLineStageResult({ status: 'error', message: 'Nenhuma linha válida selecionada para stage.' })
        setIsStagingLines(false)
        return
      }
      await GitPanelStagePatch(resolvedRepoPath, patchText)
      setLineStageResult({ status: 'success' })
      setSelectedLines(new Set())
      lastClickedLineRef.current = null
      // Refresh
      const [statusRaw, diffRaw] = await Promise.all([
        GitPanelGetStatus(resolvedRepoPath),
        GitPanelGetDiff(resolvedRepoPath, selectedDiffPath, 'unified', diffContextLines),
      ])
      setStatus(statusRaw as unknown as GitPanelStatusDTO)
      setDiffData(diffRaw as unknown as GitPanelDiffDTO)
    } catch (err) {
      const parsed = parseGitPanelError(err)
      setLineStageResult({ status: 'error', message: parsed.message })
    } finally {
      setIsStagingLines(false)
      setTimeout(() => setLineStageResult(null), 3000)
    }
  }, [selectedDiffFile, resolvedRepoPath, selectedDiffPath, selectedLines, buildLinesPatch])

  // --- Handler: Line toggle with Shift+Click support ---
  const handleLineToggle = useCallback((hunkIndex: number, lineIndex: number, shiftKey: boolean) => {
    const key = `${hunkIndex}:${lineIndex}`

    setSelectedLines((prev) => {
      const next = new Set(prev)

      if (shiftKey && lastClickedLineRef.current && selectedDiffFile) {
        // Shift+Click: select range
        const allKeys: string[] = []
        for (let h = 0; h < selectedDiffFile.hunks.length; h++) {
          for (let l = 0; l < selectedDiffFile.hunks[h].lines.length; l++) {
            const line = selectedDiffFile.hunks[h].lines[l]
            if (line.type === 'add' || line.type === 'delete') {
              allKeys.push(`${h}:${l}`)
            }
          }
        }

        const fromIdx = allKeys.indexOf(lastClickedLineRef.current)
        const toIdx = allKeys.indexOf(key)

        if (fromIdx >= 0 && toIdx >= 0) {
          const start = Math.min(fromIdx, toIdx)
          const end = Math.max(fromIdx, toIdx)
          for (let i = start; i <= end; i++) {
            next.add(allKeys[i])
          }
        }
      } else {
        if (next.has(key)) {
          next.delete(key)
        } else {
          next.add(key)
        }
      }

      lastClickedLineRef.current = key
      return next
    })
  }, [selectedDiffFile])

  // --- Scroll sync handler for split view ---
  const handleSplitScroll = useCallback((source: 'left' | 'right') => {
    if (scrollSyncSourceRef.current && scrollSyncSourceRef.current !== source) return

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

    requestAnimationFrame(() => {
      scrollSyncSourceRef.current = null
    })
  }, [])

  // --- Handler: Conflict Resolution (Accept Ours) ---
  const handleAcceptOurs = useCallback(async (filePath: string) => {
    if (!resolvedRepoPath) return
    setFileActions(prev => ({ ...prev, [filePath]: { status: 'running' } }))
    try {
      await GitPanelAcceptOurs(resolvedRepoPath, filePath, conflictAutoStage)
      setFileActions(prev => ({ ...prev, [filePath]: { status: 'success' } }))
      // Update status to reflect resolution
      const newStatus = await GitPanelGetStatus(resolvedRepoPath)
      setStatus(newStatus as unknown as GitPanelStatusDTO)
    } catch (err) {
      const parsed = parseGitPanelError(err)
      setFileActions(prev => ({ ...prev, [filePath]: { status: 'error', message: parsed.message } }))
    } finally {
      setTimeout(() => setFileActions(prev => {
        const next = { ...prev }
        delete next[filePath]
        return next
      }), 3000)
    }
  }, [resolvedRepoPath, conflictAutoStage])

  // --- Handler: Conflict Resolution (Accept Theirs) ---
  const handleAcceptTheirs = useCallback(async (filePath: string) => {
    if (!resolvedRepoPath) return
    setFileActions(prev => ({ ...prev, [filePath]: { status: 'running' } }))
    try {
      await GitPanelAcceptTheirs(resolvedRepoPath, filePath, conflictAutoStage)
      setFileActions(prev => ({ ...prev, [filePath]: { status: 'success' } }))
      // Update status to reflect resolution
      const newStatus = await GitPanelGetStatus(resolvedRepoPath)
      setStatus(newStatus as unknown as GitPanelStatusDTO)
    } catch (err) {
      const parsed = parseGitPanelError(err)
      setFileActions(prev => ({ ...prev, [filePath]: { status: 'error', message: parsed.message } }))
    } finally {
      setTimeout(() => setFileActions(prev => {
        const next = { ...prev }
        delete next[filePath]
        return next
      }), 3000)
    }
  }, [resolvedRepoPath, conflictAutoStage])

  // --- Handler: Open external merge tool ---
  const handleOpenExternalTool = useCallback(async (filePath: string) => {
    if (!resolvedRepoPath) return
    setFileActions(prev => ({ ...prev, [filePath]: { status: 'running' } }))
    try {
      await GitPanelOpenExternalMergeTool(resolvedRepoPath, filePath)
      setFileActions(prev => ({ ...prev, [filePath]: { status: 'success' } }))
      const newStatus = await GitPanelGetStatus(resolvedRepoPath)
      setStatus(newStatus as unknown as GitPanelStatusDTO)
    } catch (err) {
      const parsed = parseGitPanelError(err)
      setFileActions(prev => ({ ...prev, [filePath]: { status: 'error', message: parsed.message } }))
    } finally {
      setTimeout(() => setFileActions(prev => {
        const next = { ...prev }
        delete next[filePath]
        return next
      }), 3000)
    }
  }, [resolvedRepoPath])

  const renderInspectorTabContent = () => {
    if (activeInspectorTab === 'commit') {
      return (
        <div className="git-panel-tab-content">
          <div className="git-panel-card git-panel-card--fill">
            <h2>Commit selecionado</h2>
            {selectedCommit ? (
              <div className="git-panel-inspector__commit">
                <p className="git-panel-inspector__subject">{selectedCommit.subject || '(sem subject)'}</p>
                <p><strong>Hash:</strong> <code>{selectedCommit.hash}</code></p>
                <p><strong>Autor:</strong> {selectedCommit.author}</p>
                <p><strong>Data:</strong> {formatCommitDate(selectedCommit.authoredAt)}</p>
                <p>
                  <strong>Stats:</strong>{' '}
                  {formatFilesChangedLabel(selectedCommit.changedFiles)} • +
                  {formatMetricCount(selectedCommit.additions)} insertions • -
                  {formatMetricCount(selectedCommit.deletions)} deletions
                </p>
              </div>
            ) : (
              <p className="git-panel-inspector__empty">Selecione um commit para ver detalhes.</p>
            )}
          </div>
        </div>
      )
    }

    if (activeInspectorTab === 'diff') {
      return (
        <div className="git-panel-tab-content git-panel-tab-content--diff">
          <div className="git-panel-diff__toolbar">
            <div className="git-panel-diff__field">
              <label htmlFor="git-panel-diff-file">Arquivo</label>
              <select
                id="git-panel-diff-file"
                value={selectedDiffPath}
                onChange={(event) => setSelectedDiffPath(event.target.value)}
                disabled={diffCandidates.length === 0}
              >
                {diffCandidates.map((candidate) => (
                  <option key={`${candidate.bucket}:${candidate.path}`} value={candidate.path}>
                    [{candidate.bucket}] {candidate.path}
                  </option>
                ))}
              </select>
            </div>

            <div className="git-panel-diff__view-toggle" role="tablist" aria-label="Modo do diff">
              <button
                type="button"
                className={`git-panel-diff__view-btn ${diffViewMode === 'unified' ? 'git-panel-diff__view-btn--active' : ''}`}
                onClick={() => setDiffViewMode('unified')}
              >
                Unified
              </button>
              <button
                type="button"
                className={`git-panel-diff__view-btn ${diffViewMode === 'split' ? 'git-panel-diff__view-btn--active' : ''}`}
                onClick={() => setDiffViewMode('split')}
              >
                Split
              </button>
            </div>
          </div>

          {/* Stage Selected Lines action bar */}
          {selectedLines.size > 0 && (
            <div className="git-panel-diff__stage-bar">
              <span className="git-panel-diff__stage-bar-label">
                {selectedLines.size} {selectedLines.size === 1 ? 'linha selecionada' : 'linhas selecionadas'}
              </span>
              <button
                type="button"
                className="git-panel-diff__stage-bar-btn"
                onClick={handleStageSelectedLines}
                disabled={isStagingLines}
              >
                {isStagingLines ? (
                  <><Loader2 size={12} className="git-panel-log__spinner" /> Staging...</>
                ) : (
                  <><Plus size={12} /> Stage Selected Lines</>
                )}
              </button>
              <button
                type="button"
                className="git-panel-diff__stage-bar-clear"
                onClick={() => { setSelectedLines(new Set()); lastClickedLineRef.current = null }}
              >
                Limpar
              </button>
              {lineStageResult && (
                <span className={`git-panel-diff__stage-bar-result git-panel-diff__stage-bar-result--${lineStageResult.status}`}>
                  {lineStageResult.status === 'success' ? <><Check size={12} /> Staged!</> : lineStageResult.message}
                </span>
              )}
            </div>
          )}

          <div className="git-panel-diff__surface">
            {isDiffLoading ? (
              <div className="git-panel-diff__loading">
                <Loader2 size={14} className="git-panel-log__spinner" />
                Carregando diff estruturado...
              </div>
            ) : diffError ? (
              <div className="git-panel-log__error" role="alert">
                <AlertTriangle size={14} />
                <div>
                  <strong>{diffError.message}</strong>
                  {diffError.details && <p>{diffError.details}</p>}
                </div>
              </div>
            ) : !selectedDiffPath ? (
              <div className="git-panel-diff__empty">Selecione um arquivo para visualizar o diff.</div>
            ) : selectedDiffFile?.isBinary && !diffForceLoad ? (
              <div className="git-panel-diff__empty git-panel-diff__degraded">
                <ShieldAlert size={24} className="git-panel-diff__degraded-icon" />
                <p>Arquivo <strong>{selectedDiffFile.path}</strong> foi classificado como binário.</p>
                <button
                  type="button"
                  className="git-panel-diff__degraded-btn"
                  onClick={() => setDiffForceLoad(true)}
                >
                  Tentar visualizar como texto mesmo assim
                </button>
              </div>
            ) : diffData?.isTruncated && !diffForceLoad ? (
              <div className="git-panel-diff__empty git-panel-diff__degraded">
                <AlertTriangle size={24} className="git-panel-diff__degraded-icon warning" />
                <p>O diff do arquivo <strong>{selectedDiffFile?.path || selectedDiffPath}</strong> é muito grande e foi truncado.</p>
                <div className="git-panel-diff__degraded-actions">
                  <button
                    type="button"
                    className="git-panel-diff__degraded-btn"
                    onClick={() => setDiffForceLoad(true)}
                  >
                    Visualizar modo truncado
                  </button>
                </div>
              </div>
            ) : selectedDiffFile && selectedDiffFile.hunks.length > 0 ? (
              <div className="git-panel-diff__files">
                <article className="git-panel-diff-file" key={selectedDiffFile.path}>
                  <header className="git-panel-diff-file__header">
                    <p className="git-panel-diff-file__path" title={selectedDiffFile.path}>{selectedDiffFile.path}</p>
                    <div className="git-panel-diff-file__meta">
                      <span className="badge badge--success">+{selectedDiffFile.additions}</span>
                      <span className="badge badge--error">-{selectedDiffFile.deletions}</span>
                      <span className="badge badge--info">{selectedDiffFile.status}</span>
                      {diffData?.isTruncated && <span className="badge badge--warning" title="Apenas parte do diff foi carregada">Truncado</span>}
                    </div>
                    <div className="git-panel-diff-file__context-actions">
                      <button
                        type="button"
                        onClick={() => setDiffContextLines(prev => prev + 3)}
                        title="Aumentar linhas de contexto"
                        className="git-panel-diff-context-btn"
                      >
                        + Contexto
                      </button>
                    </div>
                  </header>

                  {selectedDiffFile.hunks.map((hunk, hunkIndex) => (
                    <section className="git-panel-diff-hunk" key={`${selectedDiffFile.path}-${hunkIndex}`}>
                      <header className="git-panel-diff-hunk__header">
                        <code>@@ -{hunk.oldStart},{hunk.oldLines} +{hunk.newStart},{hunk.newLines} @@</code>
                        {hunk.header && <span>{hunk.header}</span>}
                        <button
                          type="button"
                          className="git-panel-diff-hunk__stage-btn"
                          onClick={() => handleStageHunk(hunkIndex)}
                          disabled={stagingHunkIndex === hunkIndex}
                          title="Stage este hunk inteiro"
                        >
                          {stagingHunkIndex === hunkIndex ? (
                            <Loader2 size={12} className="git-panel-log__spinner" />
                          ) : hunkStageResult?.index === hunkIndex && hunkStageResult.status === 'success' ? (
                            <Check size={12} />
                          ) : (
                            <Plus size={12} />
                          )}
                          {stagingHunkIndex === hunkIndex ? 'Staging...' : 'Stage Hunk'}
                        </button>
                        {hunkStageResult?.index === hunkIndex && hunkStageResult.status === 'error' && (
                          <span className="git-panel-diff-hunk__stage-error">{hunkStageResult.message}</span>
                        )}
                      </header>

                      {diffViewMode === 'unified' ? (
                        <div className="git-panel-diff-hunk__unified">
                          {hunk.lines.map((line, lineIndex) => {
                            const isSelectable = line.type === 'add' || line.type === 'delete'
                            const lineKey = `${hunkIndex}:${lineIndex}`
                            const isLineSelected = selectedLines.has(lineKey)
                            return (
                              <div
                                key={`${selectedDiffFile.path}-${hunkIndex}-${lineIndex}`}
                                className={`git-panel-diff-line git-panel-diff-line--${line.type}${isLineSelected ? ' git-panel-diff-line--selected' : ''}`}
                              >
                                {isSelectable && (
                                  <span
                                    className="git-panel-diff-line__checkbox"
                                    role="checkbox"
                                    aria-checked={isLineSelected}
                                    tabIndex={0}
                                    onClick={(e) => handleLineToggle(hunkIndex, lineIndex, e.shiftKey)}
                                    onKeyDown={(e) => { if (e.key === ' ' || e.key === 'Enter') { e.preventDefault(); handleLineToggle(hunkIndex, lineIndex, e.shiftKey) } }}
                                  >
                                    <span className={`git-panel-diff-line__check-box${isLineSelected ? ' git-panel-diff-line__check-box--checked' : ''}`}>
                                      {isLineSelected && <Check size={10} />}
                                    </span>
                                  </span>
                                )}
                                {!isSelectable && <span className="git-panel-diff-line__checkbox-spacer" />}
                                <span className="git-panel-diff-line__gutter">{line.oldLine ?? ''}</span>
                                <span className="git-panel-diff-line__gutter">{line.newLine ?? ''}</span>
                                <span className="git-panel-diff-line__prefix">
                                  {line.type === 'add' ? '+' : line.type === 'delete' ? '-' : line.type === 'meta' ? '\\' : ' '}
                                </span>
                                <span className="git-panel-diff-line__content">{line.content || ' '}</span>
                              </div>
                            )
                          })}
                        </div>
                      ) : (
                        /* Split view with scroll-synced panels */
                        <div className="git-panel-diff-split-container">
                          <div
                            className="git-panel-diff-split-pane git-panel-diff-split-pane--left"
                            ref={splitLeftRef}
                            onScroll={() => handleSplitScroll('left')}
                          >
                            {buildSplitPairs(hunk.lines).map((pair, pairIndex) => {
                              if (pair.meta) {
                                return (
                                  <div key={`left-meta-${pairIndex}`} className="git-panel-diff-split__meta-row">
                                    {pair.meta}
                                  </div>
                                )
                              }
                              const leftType = pair.left?.type ?? 'context'
                              return (
                                <div
                                  key={`left-${pairIndex}`}
                                  className={`git-panel-diff-split__row git-panel-diff-split__row--${leftType}`}
                                >
                                  <span className="git-panel-diff-split__line-num">{pair.left?.oldLine ?? ''}</span>
                                  <span className="git-panel-diff-split__code">{pair.left?.content ?? ''}</span>
                                </div>
                              )
                            })}
                          </div>
                          <div className="git-panel-diff-split-divider" />
                          <div
                            className="git-panel-diff-split-pane git-panel-diff-split-pane--right"
                            ref={splitRightRef}
                            onScroll={() => handleSplitScroll('right')}
                          >
                            {buildSplitPairs(hunk.lines).map((pair, pairIndex) => {
                              if (pair.meta) {
                                return (
                                  <div key={`right-meta-${pairIndex}`} className="git-panel-diff-split__meta-row">
                                    {pair.meta}
                                  </div>
                                )
                              }
                              const rightType = pair.right?.type ?? 'context'
                              return (
                                <div
                                  key={`right-${pairIndex}`}
                                  className={`git-panel-diff-split__row git-panel-diff-split__row--${rightType}`}
                                >
                                  <span className="git-panel-diff-split__line-num">{pair.right?.newLine ?? ''}</span>
                                  <span className="git-panel-diff-split__code">{pair.right?.content ?? ''}</span>
                                </div>
                              )
                            })}
                          </div>
                        </div>
                      )}
                    </section>
                  ))}
                </article>
              </div>
            ) : diffData?.raw?.trim() ? (
              <pre className="git-panel-diff__raw">{diffData.raw}</pre>
            ) : (
              <div className="git-panel-diff__empty">Sem diferenças para o arquivo selecionado.</div>
            )}
          </div>
        </div>
      )
    }

    if (activeInspectorTab === 'conflicts') {
      return (
        <div className="git-panel-tab-content git-panel-tab-content--conflicts">
          <div className="git-panel-card git-panel-card--fill">
            <header className="git-panel-conflicts__header">
              <h2>Conflitos de Merge ({conflictedCount})</h2>
              {preflight?.mergeActive && <span className="badge badge--error"><GitMerge size={12} /> Merge in progress</span>}
              <label className="git-panel-conflicts__toggle">
                <input
                  type="checkbox"
                  checked={conflictAutoStage}
                  onChange={(e) => setConflictAutoStage(e.target.checked)}
                />
                Auto-stage após resolução
              </label>
            </header>

            {conflictedCount === 0 ? (
              <div className="git-panel-diff__empty">
                <Check size={24} className="git-panel-diff__degraded-icon success" />
                <p>Nenhum conflito detectado no repositório ativo.</p>
              </div>
            ) : (
              <div className="git-panel-conflicts__list">
                {(status?.conflicted ?? []).map((file) => {
                  const state = fileActions[file.path]
                  return (
                    <div className="git-panel-conflicts__item" key={`conflicted-${file.path}`}>
                      <div className="git-panel-conflicts__item-info">
                        <span className="git-panel-conflicts__item-path" title={file.path}>{file.path}</span>
                        <code className="git-panel-conflicts__item-status">{file.status}</code>
                      </div>

                      <div className="git-panel-conflicts__item-actions">
                        {state?.status === 'running' ? (
                          <div className="git-panel-conflicts__item-status-msg">
                            <Loader2 size={14} className="git-panel-log__spinner" />
                            <span>Resolvendo...</span>
                          </div>
                        ) : state?.status === 'success' ? (
                          <div className="git-panel-conflicts__item-status-msg success">
                            <Check size={14} />
                            <span>Resolvido!</span>
                          </div>
                        ) : state?.status === 'error' ? (
                          <div className="git-panel-conflicts__item-status-msg error" title={state.message}>
                            <AlertTriangle size={14} />
                            <span>Erro</span>
                          </div>
                        ) : (
                          <>
                            <button
                              type="button"
                              className="git-panel-conflicts__btn git-panel-conflicts__btn--mine"
                              onClick={() => handleAcceptOurs(file.path)}
                              aria-label={`Aceitar versão local para ${file.path}`}
                            >
                              Accept Mine
                            </button>
                            <button
                              type="button"
                              className="git-panel-conflicts__btn git-panel-conflicts__btn--theirs"
                              onClick={() => handleAcceptTheirs(file.path)}
                              aria-label={`Aceitar versão remota para ${file.path}`}
                            >
                              Accept Theirs
                            </button>
                            <button
                              type="button"
                              className="git-panel-conflicts__btn git-panel-conflicts__btn--external"
                              onClick={() => handleOpenExternalTool(file.path)}
                              aria-label={`Abrir ferramenta externa de merge para ${file.path}`}
                            >
                              <ExternalLink size={12} />
                              Open Tool
                            </button>
                          </>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        </div>
      )
    }

    return (
      <div className="git-panel-tab-content">
        <div className="git-panel-card git-panel-card--fill">
          <h2>Staged ({stagedCount})</h2>
          <ul className="git-panel-inspector__file-list">
            {(status?.staged ?? []).slice(0, 18).map((file) => {
              const actionState = fileActions[file.path]
              return (
                <li key={`staged-${file.path}`} title={file.path}>
                  <button
                    type="button"
                    className={`git-panel-inspector__file-btn ${file.path === selectedDiffPath ? 'selected-path' : ''}`}
                    onClick={() => {
                      setSelectedDiffPath(file.path)
                      setActiveInspectorTab('diff')
                    }}
                  >
                    <span>{file.path}</span>
                    {actionState?.status === 'running' ? (
                      <Loader2 size={12} className="git-panel-log__spinner" />
                    ) : actionState?.status === 'success' ? (
                      <Check size={12} className="git-panel-diff__stage-bar-result--success" />
                    ) : (
                      <code>{file.status}</code>
                    )}
                  </button>
                </li>
              )
            })}
            {stagedCount === 0 && <li className="git-panel-inspector__file-empty">Sem arquivos staged.</li>}
          </ul>
        </div>

        <div className="git-panel-card git-panel-card--fill">
          <h2>Unstaged ({unstagedCount})</h2>
          <ul className="git-panel-inspector__file-list">
            {(status?.unstaged ?? []).slice(0, 18).map((file) => {
              const actionState = fileActions[file.path]
              return (
                <li key={`unstaged-${file.path}`} title={file.path}>
                  <button
                    type="button"
                    className={`git-panel-inspector__file-btn ${file.path === selectedDiffPath ? 'selected-path' : ''}`}
                    onClick={() => {
                      setSelectedDiffPath(file.path)
                      setActiveInspectorTab('diff')
                    }}
                  >
                    <span>{file.path}</span>
                    {actionState?.status === 'running' ? (
                      <Loader2 size={12} className="git-panel-log__spinner" />
                    ) : actionState?.status === 'success' ? (
                      <Check size={12} className="git-panel-diff__stage-bar-result--success" />
                    ) : (
                      <code>{file.status}</code>
                    )}
                  </button>
                </li>
              )
            })}
            {unstagedCount === 0 && <li className="git-panel-inspector__file-empty">Sem arquivos unstaged.</li>}
          </ul>
        </div>
      </div>
    )
  }

  return (
    <section
      className="git-panel-screen"
      id="git-panel-screen"
      aria-label="Git Panel"
      aria-keyshortcuts="Meta+S Meta+Shift+S Alt+1 Alt+2 Alt+3 Alt+4 J K Esc"
    >
      <header className="git-panel-screen__header" ref={actionsRegionRef}>
        <button
          className="btn btn--ghost git-panel-screen__back"
          onClick={onBack}
          aria-label="Voltar para o Command Center"
          title="Voltar (Esc)"
        >
          <ArrowLeft size={14} />
        </button>

        <div className="git-panel-screen__repo-panel">
          <div className="git-panel-screen__repo-bar" role="group" aria-label="Seleção de repositório">
            <div
              className={`git-panel-screen__repo-chip ${hasSelectedRepo ? '' : 'git-panel-screen__repo-chip--empty'}`}
              title={hasSelectedRepo ? selectedRepoPath : 'Nenhum repositório selecionado'}
            >
              <span className="git-panel-screen__repo-chip-label">
                <FolderOpen size={13} />
                Repositório
              </span>
              <span className="git-panel-screen__repo-chip-path" aria-live="polite">
                {hasSelectedRepo ? selectedRepoPath : 'Nenhum repositório selecionado'}
              </span>
            </div>

            <button
              className={`btn ${hasSelectedRepo ? 'btn--ghost' : 'btn--primary'} git-panel-screen__repo-picker-btn`}
              type="button"
              onClick={() => { void handlePickRepositoryDirectory() }}
              disabled={isRepoPickerOpen}
              aria-label={hasSelectedRepo ? 'Trocar repositório' : 'Selecionar repositório'}
            >
              {isRepoPickerOpen ? <Loader2 size={13} className="git-panel-log__spinner" /> : <FolderOpen size={13} />}
              {isRepoPickerOpen ? 'Abrindo...' : hasSelectedRepo ? 'Trocar' : 'Selecionar'}
            </button>
          </div>
          {!hasSelectedRepo && (
            <p className="git-panel-screen__repo-hint">
              Selecione um repositório Git para carregar histórico, diff e stage.
            </p>
          )}
        </div>
      </header>

      <div className="git-panel-screen__layout">
        <aside
          className="git-panel-pane git-panel-sidebar"
          aria-label="Branches e refs"
          ref={sidebarRegionRef}
          tabIndex={0}
        >
          <div className="git-panel-card">
            <h2>
              <GitBranch size={14} />
              Branch ativa
            </h2>
            <p className="git-panel-sidebar__branch">{branchName}</p>
            <div className="git-panel-sidebar__ahead-behind">
              <span className="badge badge--accent">Ahead {status?.ahead ?? 0}</span>
              <span className="badge badge--info">Behind {status?.behind ?? 0}</span>
            </div>
          </div>

          <div className="git-panel-card">
            <h2>
              <History size={14} />
              Branches / Refs
            </h2>
            <ul className="git-panel-sidebar__ref-list" role="listbox" aria-label="Lista de refs rápidas">
              {quickRefs.map((item) => (
                <li key={item.id}>
                  <button
                    type="button"
                    className={`git-panel-sidebar__ref-item ${selectedRefID === item.id ? 'git-panel-sidebar__ref-item--selected' : ''}`}
                    onClick={() => handleRefSelection(item)}
                    aria-selected={selectedRefID === item.id}
                  >
                    <span className="git-panel-sidebar__ref-label">{item.label}</span>
                    <span className="git-panel-sidebar__ref-meta" title={item.meta}>{item.meta}</span>
                  </button>
                </li>
              ))}
            </ul>
          </div>

          <div className="git-panel-card">
            <h2>
              <FileCode2 size={14} />
              Working tree
            </h2>
            <div className="git-panel-sidebar__status-grid">
              <div>
                <span className="git-panel-sidebar__status-label">Staged</span>
                <strong>{stagedCount}</strong>
              </div>
              <div>
                <span className="git-panel-sidebar__status-label">Unstaged</span>
                <strong>{unstagedCount}</strong>
              </div>
              <div>
                <span className="git-panel-sidebar__status-label">Conflicted</span>
                <strong>{conflictedCount}</strong>
              </div>
            </div>
            <p className="git-panel-sidebar__mono" title={resolvedRepoPath || activeRepoPath}>
              {resolvedRepoPath || activeRepoPath || 'Sem repositório ativo'}
            </p>
            <p className="git-panel-sidebar__updated">
              <Clock3 size={12} /> Última atualização: {formatTimestamp(lastUpdatedAt)}
            </p>
          </div>
        </aside>

        <section className="git-panel-pane git-panel-log" aria-label="Histórico de commits">
          <header className="git-panel-log__header">
            <h2>
              <History size={14} />
              Commit Log
            </h2>
            <div className="git-panel-log__search">
              <Search size={14} className="git-panel-log__search-icon" />
              <input
                type="text"
                placeholder="Buscar (autor, hash, msg)..."
                value={historySearch}
                onChange={(e) => setHistorySearch(e.target.value)}
                className="input git-panel-log__search-input"
              />
            </div>
            <span className="badge badge--accent">{historyItems.length} carregados</span>
          </header>

          {error && (
            <div className="git-panel-log__error" role="alert">
              <AlertTriangle size={14} />
              <div>
                <strong>{error.message}</strong>
                {error.details && <p>{error.details}</p>}
              </div>
            </div>
          )}

          <div
            ref={historyViewportRef}
            className="git-panel-log__viewport"
            onScroll={(event) => setHistoryScrollTop(event.currentTarget.scrollTop)}
            tabIndex={0}
            aria-label="Lista virtualizada de commits"
          >
            {isInitialLoading ? (
              <div className="git-panel-log__skeleton-list">
                {Array.from({ length: 10 }).map((_, index) => (
                  <div key={`git-panel-skeleton-${index}`} className="git-panel-log__skeleton-row" />
                ))}
              </div>
            ) : historyItems.length === 0 ? (
              <div className="git-panel-log__empty">
                <History size={16} />
                <p>Nenhum commit encontrado para o repositório ativo.</p>
              </div>
            ) : (
              <div style={{ height: `${totalVirtualHeight}px` }}>
                <div
                  className="git-panel-log__virtual-track"
                  style={{
                    paddingTop: `${paddingTop}px`,
                    paddingBottom: `${paddingBottom}px`,
                  }}
                >
                  {virtualRows.map((row) => {
                    if (row.kind === 'loading') {
                      return (
                        <div key={`history-loading-${row.index}`} className="git-panel-log__row git-panel-log__row--loading">
                          <Loader2 size={14} className="git-panel-log__spinner" />
                          Carregando próxima página...
                        </div>
                      )
                    }

                    const isSelected = row.item.hash === selectedCommitHash
                    return (
                      <button
                        key={row.item.hash}
                        type="button"
                        className={`git-panel-log__row ${isSelected ? 'git-panel-log__row--selected' : ''}`}
                        data-commit-hash={row.item.hash}
                        aria-selected={isSelected}
                        onClick={() => {
                          setSelectedCommitHash(row.item.hash)
                          setActiveInspectorTab('commit')
                        }}
                      >
                        <div className="git-panel-log__row-title">{row.item.subject || '(sem subject)'}</div>
                        <div className="git-panel-log__row-author">
                          {row.item.githubAvatarUrl ? (
                            <img
                              src={row.item.githubAvatarUrl}
                              alt={`Avatar de ${resolveCommitActorLabel(row.item)}`}
                              className="git-panel-log__avatar"
                              loading="lazy"
                            />
                          ) : (
                            <span className="git-panel-log__avatar git-panel-log__avatar--fallback" aria-hidden="true">
                              {buildCommitInitials(resolveCommitActorLabel(row.item))}
                            </span>
                          )}
                          <span className="git-panel-log__author-name">{resolveCommitActorLabel(row.item)}</span>
                        </div>
                        <div className="git-panel-log__row-meta">
                          <span className="git-panel-log__commit-hash">
                            <span
                              className="git-panel-log__commit-icon"
                              aria-hidden="true"
                              // SVG local estático do projeto (shape oficial de commit horizontal)
                              dangerouslySetInnerHTML={{ __html: commitHorizontalIconSVG }}
                            />
                            <code>{row.item.shortHash}</code>
                          </span>
                          <span>{formatCommitDate(row.item.authoredAt)}</span>
                          <span className="git-panel-log__metric">{formatFilesChangedLabel(row.item.changedFiles)}</span>
                          <span className="git-panel-log__metric git-panel-log__metric--positive">+{formatMetricCount(row.item.additions)} insertions</span>
                          <span className="git-panel-log__metric git-panel-log__metric--negative">-{formatMetricCount(row.item.deletions)} deletions</span>
                        </div>
                      </button>
                    )
                  })}
                </div>
              </div>
            )}
          </div>
        </section>

        <aside
          className="git-panel-pane git-panel-inspector"
          aria-label="Inspector"
          ref={inspectorRegionRef}
          tabIndex={0}
        >
          <nav className="git-panel-inspector__tabs" aria-label="Abas do inspector">
            <button
              type="button"
              className={`git-panel-inspector__tab ${activeInspectorTab === 'working-tree' ? 'git-panel-inspector__tab--active' : ''}`}
              onClick={() => setActiveInspectorTab('working-tree')}
            >
              Working Tree
            </button>
            <button
              type="button"
              className={`git-panel-inspector__tab ${activeInspectorTab === 'commit' ? 'git-panel-inspector__tab--active' : ''}`}
              onClick={() => setActiveInspectorTab('commit')}
            >
              Commit
            </button>
            <button
              type="button"
              className={`git-panel-inspector__tab ${activeInspectorTab === 'diff' ? 'git-panel-inspector__tab--active' : ''}`}
              onClick={() => setActiveInspectorTab('diff')}
            >
              Diff
            </button>
            <button
              type="button"
              className={`git-panel-inspector__tab ${activeInspectorTab === 'conflicts' ? 'git-panel-inspector__tab--active' : ''}`}
              onClick={() => setActiveInspectorTab('conflicts')}
            >
              Conflicts
            </button>
          </nav>

          <div className="git-panel-inspector__body">{renderInspectorTabContent()}</div>
        </aside>
      </div>

      {/* --- Git Command Console output drawer --- */}
      <section className={`git-panel-console-drawer ${isConsoleOpen ? 'open' : ''}`}>
        <header className="git-panel-console__header" onClick={() => setIsConsoleOpen(!isConsoleOpen)}>
          <div className="git-panel-console__title">
            <Terminal size={14} /> Git Console Output
            <span style={{ opacity: 0.6, fontSize: '10px', marginLeft: 8 }}>(Pressione Esc para focar e voltar)</span>
          </div>
          <span className="badge badge--accent">{consoleLogs.length} events</span>
        </header>
        {isConsoleOpen && (
          <div className="git-panel-console__body">
            {consoleLogs.length === 0 ? (
              <div className="git-panel-console__empty">Nenhuma atividade registrada ainda. Execute uma ação.</div>
            ) : (
              <ul className="git-panel-console__list">
                {consoleLogs.map(log => (
                  <li key={log.commandId} className={`git-panel-console__item status-${log.status}`}>
                    <div className="git-panel-console__item-header">
                      <span className="git-panel-console__action">{log.action}</span>
                      <span className="git-panel-console__duration">{log.durationMs}ms</span>
                      <span className="git-panel-console__code">{log.exitCode === 0 ? 'success' : `exit: ${log.exitCode}`}</span>
                    </div>
                    {log.args && log.args.length > 0 && <div className="git-panel-console__args">git {log.args.join(' ')}</div>}
                    {log.stderrSanitized && <div className="git-panel-console__stderr">{log.stderrSanitized}</div>}
                  </li>
                ))}
              </ul>
            )}
          </div>
        )}
      </section>

    </section>
  )
}

interface DiffSplitPair {
  left: GitPanelDiffLine | null
  right: GitPanelDiffLine | null
  meta?: string
}

function buildSplitPairs(lines: GitPanelDiffLine[]): DiffSplitPair[] {
  const pairs: DiffSplitPair[] = []
  const pendingDeletes: GitPanelDiffLine[] = []
  const pendingAdds: GitPanelDiffLine[] = []

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

function isEditableElement(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false
  }

  const tagName = target.tagName
  if (tagName === 'INPUT' || tagName === 'TEXTAREA' || tagName === 'SELECT') {
    return true
  }

  return target.isContentEditable
}

function parseGitPanelError(error: unknown): GitPanelBindingError {
  if (error && typeof error === 'object') {
    const candidate = error as Partial<GitPanelBindingError>
    if (typeof candidate.code === 'string' && typeof candidate.message === 'string') {
      return {
        code: candidate.code,
        message: candidate.message,
        details: typeof candidate.details === 'string' ? candidate.details : undefined,
      }
    }
  }

  const raw = error instanceof Error ? error.message : String(error)
  const trimmed = raw.trim()
  if (trimmed.startsWith('{') && trimmed.endsWith('}')) {
    try {
      const decoded = JSON.parse(trimmed) as Partial<GitPanelBindingError>
      if (typeof decoded.code === 'string' && typeof decoded.message === 'string') {
        return {
          code: decoded.code,
          message: decoded.message,
          details: typeof decoded.details === 'string' ? decoded.details : undefined,
        }
      }
    } catch {
      // fallback abaixo
    }
  }

  return {
    code: 'E_UNKNOWN',
    message: 'Falha ao carregar dados do Git Panel.',
    details: trimmed,
  }
}

function formatCommitDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value || '-'
  }
  return date.toLocaleString()
}

function formatTimestamp(value: number | null): string {
  if (!value) {
    return 'nunca'
  }
  return new Date(value).toLocaleTimeString()
}

function resolveCommitActorLabel(item: GitPanelHistoryItem): string {
  const login = (item.githubLogin || '').trim()
  if (login !== '') {
    return login
  }
  const author = (item.author || '').trim()
  if (author !== '') {
    return author
  }
  return 'autor desconhecido'
}

function buildCommitInitials(value: string): string {
  const parts = value
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

function formatMetricCount(value: number): string {
  const safeValue = Number.isFinite(value) ? Math.max(0, Math.trunc(value)) : 0
  return safeValue.toLocaleString()
}

function formatFilesChangedLabel(value: number): string {
  const safeValue = Number.isFinite(value) ? Math.max(0, Math.trunc(value)) : 0
  return safeValue === 1 ? '1 file changed' : `${safeValue} files changed`
}

function normalizeHistoryPage(raw: unknown): GitPanelHistoryPageDTO {
  if (!raw || typeof raw !== 'object') {
    return {
      items: [],
      nextCursor: '',
      hasMore: false,
    }
  }

  const candidate = raw as Partial<GitPanelHistoryPageDTO>
  const items = Array.isArray(candidate.items)
    ? candidate.items.map((item) => normalizeHistoryItem(item))
    : []

  return {
    items,
    nextCursor: typeof candidate.nextCursor === 'string' ? candidate.nextCursor : '',
    hasMore: Boolean(candidate.hasMore),
  }
}

function normalizeHistoryItem(raw: unknown): GitPanelHistoryItem {
  const item = (raw && typeof raw === 'object') ? (raw as Partial<GitPanelHistoryItem> & { filesChanged?: unknown }) : {}
  const additions = parsePositiveInt(item.additions)
  const deletions = parsePositiveInt(item.deletions)
  const changedFiles = parsePositiveInt(item.changedFiles ?? item.filesChanged)

  return {
    hash: typeof item.hash === 'string' ? item.hash : '',
    shortHash: typeof item.shortHash === 'string' ? item.shortHash : '',
    author: typeof item.author === 'string' ? item.author : '',
    authoredAt: typeof item.authoredAt === 'string' ? item.authoredAt : '',
    subject: typeof item.subject === 'string' ? item.subject : '',
    additions,
    deletions,
    changedFiles,
    githubLogin: typeof item.githubLogin === 'string' ? item.githubLogin : undefined,
    githubAvatarUrl: typeof item.githubAvatarUrl === 'string' ? item.githubAvatarUrl : undefined,
  }
}

function parsePositiveInt(value: unknown): number {
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) {
      return 0
    }
    return Math.max(0, Math.trunc(value))
  }
  if (typeof value === 'string') {
    const parsed = Number.parseInt(value, 10)
    if (!Number.isFinite(parsed)) {
      return 0
    }
    return Math.max(0, Math.trunc(parsed))
  }
  return 0
}

function mergeHistoryItems(current: GitPanelHistoryItem[], incoming: GitPanelHistoryItem[]): GitPanelHistoryItem[] {
  if (incoming.length === 0) {
    return current
  }

  const seen = new Set(current.map((item) => item.hash))
  const merged = [...current]
  for (const item of incoming) {
    if (!seen.has(item.hash)) {
      seen.add(item.hash)
      merged.push(item)
    }
  }
  return merged
}

function buildDiffCandidates(status: GitPanelStatusDTO | null): DiffCandidate[] {
  if (!status) {
    return []
  }

  const seen = new Set<string>()
  const candidates: DiffCandidate[] = []

  const append = (path: string, bucket: DiffCandidate['bucket'], fileStatus: string) => {
    const normalized = path.trim()
    if (normalized === '' || seen.has(normalized)) {
      return
    }
    seen.add(normalized)
    candidates.push({
      path: normalized,
      bucket,
      status: fileStatus,
    })
  }

  for (const file of status.unstaged) {
    append(file.path, 'unstaged', file.status)
  }

  for (const file of status.staged) {
    append(file.path, 'staged', file.status)
  }

  for (const file of status.conflicted) {
    append(file.path, 'conflicted', file.status)
  }

  return candidates
}
