import { useGitHubStore } from '../stores/githubStore'

/**
 * Hook para operações de Branches
 */
export function useBranches() {
  const {
    branches,
    isLoading,
    fetchBranches,
    createBranch,
  } = useGitHubStore()

  return {
    branches,
    isLoading,
    fetchBranches,
    createBranch,
  }
}
