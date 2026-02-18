import { useCallback } from 'react'
import { useGitHubStore } from '../stores/githubStore'
import type { MergeMethod, PRState } from '../types/github'

/**
 * Hook para operações de Pull Request
 */
export function usePullRequests() {
  const {
    pullRequests,
    selectedPR,
    reviews,
    comments,
    prFilter,
    isLoading,
    fetchPullRequests,
    selectPR,
    fetchPRDetail,
    createPullRequest,
    mergePullRequest: merge,
    closePullRequest: close,
    setPRFilter,
    fetchReviews,
    createReview,
    fetchComments,
    createComment,
  } = useGitHubStore()

  const refreshPR = useCallback(async (number: number) => {
    await fetchPRDetail(number)
    await Promise.all([fetchReviews(number), fetchComments(number)])
  }, [fetchPRDetail, fetchReviews, fetchComments])

  const mergePullRequest = useCallback(async (number: number, method: MergeMethod) => {
    return merge(number, method)
  }, [merge])

  const closePullRequest = useCallback(async (number: number) => {
    return close(number)
  }, [close])

  const filterByState = useCallback((state: PRState) => {
    setPRFilter(state)
  }, [setPRFilter])

  return {
    // Data
    pullRequests,
    selectedPR,
    reviews,
    comments,
    prFilter,
    isLoading,

    // Actions
    fetchPullRequests,
    selectPR,
    refreshPR,
    createPullRequest,
    mergePullRequest,
    closePullRequest,
    filterByState,
    fetchReviews,
    createReview,
    fetchComments,
    createComment,
  }
}
