import { useEffect } from 'react'
import { useAppStore, HydrationPayload } from '../stores/appStore'
import { useAuthStore } from '../stores/authStore'
import { useGitHubStore } from '../features/github/stores/githubStore'

// Tipo para eventos Wails
declare global {
  interface Window {
    runtime: {
      EventsOn: (event: string, callback: (...args: any[]) => void) => () => void
      EventsOff: (event: string) => void
      EventsEmit: (event: string, ...args: any[]) => void
    }
  }
}

/**
 * Hook que escuta eventos Wails e atualiza os stores.
 * Deve ser chamado uma vez no componente raiz (App).
 */
export function useWailsEvents() {
  const hydrate = useAppStore((s) => s.hydrate)
  const setAuth = useAuthStore((s) => s.setAuth)
  const setLoading = useAuthStore((s) => s.setLoading)
  
  // GitHub actions
  const setCurrentBranch = useGitHubStore(s => s.setCurrentBranch)
  const fetchPullRequests = useGitHubStore(s => s.fetchPullRequests)
  const fetchBranches = useGitHubStore(s => s.fetchBranches)
  const invalidateCache = useGitHubStore(s => s.invalidateCache)

  useEffect(() => {
    // Verificar se estamos rodando dentro do Wails
    if (!window.runtime) {
      console.warn('[ORCH] Not running inside Wails — using dev mode')
      // Em dev mode, emitir hydration fake
      hydrate({
        isAuthenticated: false,
        theme: 'dark',
        language: 'pt-BR',
        onboardingCompleted: false,
        version: '0.1.0-dev',
      })
      setLoading(false)
      return
    }

    const handleHydration = (payload: HydrationPayload) => {
      console.log('[ORCH] Hydration received:', payload)

      // Atualizar app store
      hydrate(payload)

      // Atualizar auth store
      setAuth({
        isAuthenticated: payload.isAuthenticated,
        user: payload.user ?? null,
        provider: payload.user?.provider ?? null,
        hasGitHubToken: payload.user?.provider === 'github',
        isLoading: false,
      })
    }

    // 1. Escutar evento de hydration do backend
    window.runtime.EventsOn('app:hydrated', handleHydration)

    // 2. Fallback proativo: Buscar dados de hydration imediatamente
    if (window.go?.main?.App?.GetHydrationData) {
      window.go.main.App.GetHydrationData()
        .then((data: any) => {
          console.log('[ORCH] Proactive hydration fetch success')
          handleHydration(data as HydrationPayload)
        })
        .catch((err: Error) => {
          console.error('[ORCH] Failed to fetch hydration data:', err)
        })
    }

    // Escutar mudanças de auth
    const handleAuthChanged = (authState: any) => {
      console.log('[ORCH] Auth changed:', authState)
      setAuth({
        isAuthenticated: authState.isAuthenticated,
        user: authState.user ?? null,
        provider: authState.provider ?? null,
        hasGitHubToken: authState.hasGitHubToken ?? false,
        isLoading: false,
      })
    }

    window.runtime.EventsOn('auth:changed', handleAuthChanged)

    // Escutar mudanças de contexto do terminal (Sync automático de Git)
    const handleTerminalContext = (data: { sessionID: string, path: string, cwd?: string, branch?: string, isGit: string }) => {
      if (data.isGit === 'true' && data.path) {
        console.log(`[ORCH] Terminal ${data.sessionID} is in a Git repo: ${data.path}`)

        if (data.branch) {
          setCurrentBranch(data.branch)
        }
      }
    }

    const handleGitBranchChanged = (event: { details?: { branch?: string } }) => {
      console.log('[ORCH][GitEvents] git:branch_changed', event)
      const branch = event?.details?.branch
      if (branch) {
        setCurrentBranch(branch)
      }
      invalidateCache()
      fetchPullRequests()
      fetchBranches()
    }

    const handleGitCommit = () => {
      console.log('[ORCH][GitEvents] git:commit')
      invalidateCache()
      fetchPullRequests()
    }

    const handleGitFetch = () => {
      console.log('[ORCH][GitEvents] git:fetch')
      fetchBranches()
    }

    const handleGitMerge = () => {
      console.log('[ORCH][GitEvents] git:merge')
      invalidateCache()
      fetchPullRequests()
    }

    const handlePRsUpdated = (event: unknown) => {
      console.log('[ORCH][GitEvents] github:prs:updated', event)
      fetchPullRequests()
    }

    const offContext = window.runtime.EventsOn('terminal:context_changed', handleTerminalContext)
    const offBranchChanged = window.runtime.EventsOn('git:branch_changed', handleGitBranchChanged)
    const offCommit = window.runtime.EventsOn('git:commit', handleGitCommit)
    const offFetch = window.runtime.EventsOn('git:fetch', handleGitFetch)
    const offMerge = window.runtime.EventsOn('git:merge', handleGitMerge)
    const offPRsUpdated = window.runtime.EventsOn('github:prs:updated', handlePRsUpdated)

    // Cleanup
    return () => {
      window.runtime.EventsOff('app:hydrated')
      window.runtime.EventsOff('auth:changed')
      offContext()
      offBranchChanged()
      offCommit()
      offFetch()
      offMerge()
      offPRsUpdated()
    }
  }, [hydrate, setAuth, setLoading, setCurrentBranch, fetchPullRequests, fetchBranches, invalidateCache])
}
