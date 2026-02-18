import { useCallback } from 'react'
import { useGitHubStore } from '../stores/githubStore'
import type { DiffViewMode } from '../types/github'

/**
 * Hook para carregamento e visualização de Diff
 */
export function useDiff() {
  const {
    currentDiff,
    diffViewMode,
    isLoading,
    fetchPRDiff,
    loadMoreDiffFiles,
    setDiffViewMode,
    createInlineComment,
  } = useGitHubStore()

  const loadDiff = useCallback(async (prNumber: number) => {
    await fetchPRDiff(prNumber)
  }, [fetchPRDiff])

  const toggleViewMode = useCallback(() => {
    setDiffViewMode(diffViewMode === 'unified' ? 'side-by-side' : 'unified')
  }, [diffViewMode, setDiffViewMode])

  const setViewMode = useCallback((mode: DiffViewMode) => {
    setDiffViewMode(mode)
  }, [setDiffViewMode])

  return {
    diff: currentDiff,
    files: currentDiff?.files ?? [],
    totalFiles: currentDiff?.totalFiles ?? 0,
    hasMoreFiles: currentDiff?.pagination?.hasNextPage ?? false,
    viewMode: diffViewMode,
    isLoading,
    loadDiff,
    loadMoreFiles: loadMoreDiffFiles,
    toggleViewMode,
    setViewMode,
    createInlineComment,
  }
}
