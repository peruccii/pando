import { useCallback } from 'react'
import { useGitHubStore } from '../stores/githubStore'
import type { IssueState, KanbanColumn } from '../types/github'

/**
 * Hook para operações de Issues
 */
export function useIssues() {
  const {
    issues,
    issueFilter,
    isLoading,
    fetchIssues,
    createIssue,
    updateIssue,
    setIssueFilter,
  } = useGitHubStore()

  const filterByState = useCallback((state: IssueState) => {
    setIssueFilter(state)
  }, [setIssueFilter])

  // Kanban helpers
  const getIssuesByColumn = useCallback((column: KanbanColumn) => {
    switch (column) {
      case 'backlog':
        return issues.filter(i => i.state === 'OPEN' && i.assignees.length === 0)
      case 'in_progress':
        return issues.filter(i => i.state === 'OPEN' && i.assignees.length > 0)
      case 'done':
        return issues.filter(i => i.state === 'CLOSED')
    }
  }, [issues])

  const moveToColumn = useCallback(async (issueNumber: number, column: KanbanColumn) => {
    switch (column) {
      case 'done':
        return updateIssue(issueNumber, undefined, undefined, 'CLOSED')
      case 'backlog':
      case 'in_progress':
        return updateIssue(issueNumber, undefined, undefined, 'OPEN')
    }
  }, [updateIssue])

  return {
    issues,
    issueFilter,
    isLoading,
    fetchIssues,
    createIssue,
    updateIssue,
    filterByState,
    getIssuesByColumn,
    moveToColumn,
  }
}
