import { useEffect, useCallback, useRef } from 'react'
import { useGitHubStore } from '../features/github/stores/githubStore'

/**
 * Contextos de polling possíveis — espelham PollingContext do backend
 */
export type PollingContextType =
  | 'pr_detail'
  | 'pr_list'
  | 'background'
  | 'minimized'
  | 'collaborate'

interface RateLimitInfo {
  remaining: number
  limit: number
  resetAt: string
}

interface UsePollingOptions {
  /** Owner do repositório (ex: 'facebook') */
  owner?: string
  /** Nome do repositório (ex: 'react') */
  repo?: string
  /** Contexto atual de polling */
  context?: PollingContextType
  /** Se true, inicia polling automaticamente */
  autoStart?: boolean
}

/**
 * Hook que gerencia o polling inteligente de dados GitHub.
 * 
 * - Inicia/para polling quando owner/repo mudam
 * - Atualiza contexto de polling
 * - Escuta eventos de atualização de PRs e rate limit
 * - Integra com githubStore para invalidar cache
 */
export function usePolling(options: UsePollingOptions = {}) {
  const { owner, repo, context = 'background', autoStart = true } = options
  const prevRepoRef = useRef<string | null>(null)
  const prevContextRef = useRef<PollingContextType | null>(null)

  const invalidateCache = useGitHubStore((s) => s.invalidateCache)
  const fetchPullRequests = useGitHubStore((s) => s.fetchPullRequests)

  const api = window.go?.main?.App

  // Iniciar/parar polling quando o repositório muda
  useEffect(() => {
    if (!api || !autoStart) return

    const repoKey = owner && repo ? `${owner}/${repo}` : null
    if (repoKey === prevRepoRef.current) return

    prevRepoRef.current = repoKey

    if (owner && repo) {
      api.StartPolling(owner, repo).catch((err: Error) => {
        console.error('[usePolling] Failed to start:', err)
      })
    } else {
      api.StopPolling().catch(() => { })
    }

    return () => {
      api.StopPolling().catch(() => { })
    }
  }, [api, owner, repo, autoStart])

  // Atualizar contexto quando muda
  useEffect(() => {
    if (!api || context === prevContextRef.current) return
    prevContextRef.current = context

    api.SetPollingContext(context).catch((err: Error) => {
      console.error('[usePolling] Failed to set context:', err)
    })
  }, [api, context])

  // Escutar evento de PRs atualizados
  useEffect(() => {
    if (!window.runtime) return

    const off = window.runtime.EventsOn('github:prs:updated', () => {
      invalidateCache()
      fetchPullRequests()
    })

    const onP2PState = () => {
      invalidateCache()
      fetchPullRequests()
    }
    window.addEventListener('github:p2p_state_updated', onP2PState)

    return () => {
      off()
      window.removeEventListener('github:p2p_state_updated', onP2PState)
    }
  }, [invalidateCache, fetchPullRequests])

  // Fetch rate limit info sob demanda
  const getRateLimitInfo = useCallback(async (): Promise<RateLimitInfo | null> => {
    if (!api) return null
    try {
      return await api.GetRateLimitInfo()
    } catch {
      return null
    }
  }, [api])

  return {
    getRateLimitInfo,
    startPolling: useCallback((o: string, r: string) => {
      api?.StartPolling(o, r).catch(() => { })
    }, [api]),
    stopPolling: useCallback(() => {
      api?.StopPolling().catch(() => { })
    }, [api]),
    setContext: useCallback((ctx: PollingContextType) => {
      api?.SetPollingContext(ctx).catch(() => { })
    }, [api]),
  }
}
