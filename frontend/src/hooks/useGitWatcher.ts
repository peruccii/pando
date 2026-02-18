import { useEffect, useCallback, useRef } from 'react'
import { useGitHubStore } from '../features/github/stores/githubStore'

/**
 * Tipos de eventos emitidos pelo FileWatcher backend
 */
interface GitFileEvent {
  type: 'branch_changed' | 'commit' | 'merge' | 'fetch' | 'index' | 'commit_preparing'
  path: string
  timestamp: string
  details: Record<string, string>
}

/**
 * Estado do Git Watcher no frontend
 */
interface GitWatcherState {
  currentBranch: string | null
  isMerging: boolean
  lastCommitRef: string | null
}

/**
 * Hook que escuta eventos do FileWatcher via Wails Events
 * e atualiza o githubStore automaticamente.
 *
 * Deve ser chamado no componente raiz ou no GitHubPane.
 */
export function useGitWatcher(projectPath?: string) {
  const stateRef = useRef<GitWatcherState>({
    currentBranch: null,
    isMerging: false,
    lastCommitRef: null,
  })

  const invalidateCache = useGitHubStore((s) => s.invalidateCache)
  const fetchBranches = useGitHubStore((s) => s.fetchBranches)
  const fetchPullRequests = useGitHubStore((s) => s.fetchPullRequests)
  const setCurrentBranch = useGitHubStore((s) => s.setCurrentBranch)

  // Handler para eventos de branch_changed
  const handleBranchChanged = useCallback((event: GitFileEvent) => {
    const newBranch = event.details?.branch
    if (newBranch && newBranch !== stateRef.current.currentBranch) {
      stateRef.current.currentBranch = newBranch
      console.log(`[GitWatcher] Branch changed to: ${newBranch}`)

      // Atualizar branch no store global
      setCurrentBranch(newBranch)

      // Invalidar cache e re-fetch dados relacionados à branch
      invalidateCache()
      fetchPullRequests()
      fetchBranches()
    }
  }, [invalidateCache, fetchPullRequests, fetchBranches, setCurrentBranch])

  // Handler para eventos de commit
  const handleCommit = useCallback((event: GitFileEvent) => {
    const ref = event.details?.ref
    stateRef.current.lastCommitRef = ref || null
    console.log(`[GitWatcher] Commit detected on ref: ${ref}`)

    // Invalidar cache (commit pode afetar status do PR)
    invalidateCache()
  }, [invalidateCache])

  // Handler para eventos de merge
  const handleMerge = useCallback((_event: GitFileEvent) => {
    stateRef.current.isMerging = true
    console.log('[GitWatcher] Merge in progress')

    // Invalidar cache após merge
    invalidateCache()
    fetchPullRequests()
  }, [invalidateCache, fetchPullRequests])

  // Handler para eventos de fetch
  const handleFetch = useCallback((_event: GitFileEvent) => {
    console.log('[GitWatcher] Fetch detected')

    // Re-fetch branches (pode ter novas branches remotas)
    fetchBranches()
  }, [fetchBranches])

  // Iniciar/parar watch quando projectPath muda
  useEffect(() => {
    if (!projectPath || !window.runtime) return

    const api = window.go?.main?.App
    if (!api) return

    // Iniciar watch
    api.WatchProject(projectPath).catch((err: Error) => {
      console.error('[GitWatcher] Failed to start watching:', err)
    })

    // Buscar branch atual
    api.GetCurrentBranch(projectPath).then((branch: string) => {
      if (branch) {
        stateRef.current.currentBranch = branch
        setCurrentBranch(branch)
      }
    }).catch(() => {
      // Ignorar erro se não é um repo git
    })

    // Registrar listeners de eventos Wails
    const offBranch = window.runtime.EventsOn('git:branch_changed', (data: GitFileEvent) => {
      handleBranchChanged(data)
    })

    const offCommit = window.runtime.EventsOn('git:commit', (data: GitFileEvent) => {
      handleCommit(data)
    })

    const offMerge = window.runtime.EventsOn('git:merge', (data: GitFileEvent) => {
      handleMerge(data)
    })

    const offFetch = window.runtime.EventsOn('git:fetch', (data: GitFileEvent) => {
      handleFetch(data)
    })

    // Cleanup
    return () => {
      offBranch()
      offCommit()
      offMerge()
      offFetch()

      // Parar watch
      api.UnwatchProject(projectPath).catch(() => {
        // Ignorar erros no cleanup
      })
    }
  }, [projectPath, handleBranchChanged, handleCommit, handleMerge, handleFetch])

  return {
    currentBranch: stateRef.current.currentBranch,
    isMerging: stateRef.current.isMerging,
  }
}
