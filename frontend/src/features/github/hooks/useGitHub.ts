import { useEffect } from 'react'
import { useGitHubStore } from '../stores/githubStore'

/**
 * Hook principal de GitHub — inicializa o contexto e carrega dados
 */
export function useGitHub(owner?: string, repo?: string) {
  const store = useGitHubStore()

  useEffect(() => {
    if (owner && repo) {
      store.setCurrentRepo(owner, repo)
    }
  }, [owner, repo]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (store.currentRepo) {
      store.fetchPullRequests()
      store.fetchIssues()
      store.fetchBranches()
    }
  }, [store.currentRepo]) // eslint-disable-line react-hooks/exhaustive-deps

  return {
    currentRepo: store.currentRepo,
    isLoading: store.isLoading,
    error: store.error,
    clearError: store.clearError,
    currentView: store.currentView,
    setCurrentView: store.setCurrentView,
    invalidateCache: store.invalidateCache,
    repositories: store.repositories,
    fetchRepositories: store.fetchRepositories,
    setCurrentRepo: store.setCurrentRepo,
  }
}
