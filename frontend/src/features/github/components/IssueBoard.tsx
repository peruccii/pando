import React from 'react'
import { useIssues } from '../hooks/useIssues'
import { useOptimisticAction } from '../../../hooks/useOptimisticAction'
import { IssueCard } from './IssueCard'
import { OptimisticList } from '../../../components/OptimisticFeedback'
import type { KanbanColumn } from '../types/github'
import './github.css'

const COLUMNS: { key: KanbanColumn; label: string; color: string }[] = [
  { key: 'backlog', label: 'Backlog', color: 'var(--gh-open)' },
  { key: 'in_progress', label: 'In Progress', color: 'var(--gh-warning)' },
  { key: 'done', label: 'Done', color: 'var(--gh-closed)' },
]

export const IssueBoard: React.FC = () => {
  const { issues, getIssuesByColumn, moveToColumn, isLoading } = useIssues()

  // Optimistic UI para mover issues no kanban
  const moveOptimistic = useOptimisticAction<
    { issueNumber: number; column: KanbanColumn },
    void
  >(
    async (data) => {
      await moveToColumn(data.issueNumber, data.column)
    },
    {
      maxRetries: 3,
      broadcastChannel: 'github:issues',
      onError: (err) => {
        console.error('[IssueBoard] Move failed:', err.message)
      },
    }
  )

  const handleDrop = (e: React.DragEvent, column: KanbanColumn) => {
    e.preventDefault()
    const issueNumber = parseInt(e.dataTransfer.getData('issueNumber'), 10)
    if (!isNaN(issueNumber)) {
      moveOptimistic.execute({ issueNumber, column })
    }
  }

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault()
  }

  if (issues.length === 0 && !isLoading) {
    return (
      <div className="gh-issue-board__empty">
        <svg width="48" height="48" viewBox="0 0 16 16" fill="currentColor" opacity="0.3">
          <path d="M8 1.5a6.5 6.5 0 100 13 6.5 6.5 0 000-13zM0 8a8 8 0 1116 0A8 8 0 010 8zm9 3a1 1 0 11-2 0 1 1 0 012 0zM6.92 6.085c.081-.16.19-.299.34-.398.145-.097.371-.187.74-.187.28 0 .553.087.738.225A.613.613 0 019 6.25c0 .177-.04.264-.077.318a.956.956 0 01-.277.245c-.076.051-.158.1-.258.161l-.007.004a7.71 7.71 0 00-.313.195 2.416 2.416 0 00-.692.661.75.75 0 001.248.832.956.956 0 01.276-.245 6.42 6.42 0 01.252-.157l.01-.006c.083-.054.189-.123.304-.21a2.461 2.461 0 00.57-.517C10.2 7.3 10.5 6.84 10.5 6.25c0-.538-.232-1.04-.618-1.393-.385-.353-.9-.557-1.482-.557-.585 0-1.108.176-1.476.47-.369.294-.624.717-.761 1.18a.75.75 0 001.427.465z" />
        </svg>
        <p>No issues found</p>
      </div>
    )
  }

  return (
    <div className="gh-issue-board">
      <h2 className="gh-issue-board__title">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
          <path d="M8 1.5a6.5 6.5 0 100 13 6.5 6.5 0 000-13zM0 8a8 8 0 1116 0A8 8 0 010 8zm9 3a1 1 0 11-2 0 1 1 0 012 0z" />
        </svg>
        Issue Board
        <span className="gh-issue-board__count">{issues.length}</span>
      </h2>

      {/* Optimistic feedback — moves pendentes/erros */}
      <OptimisticList
        items={moveOptimistic.items}
        label="Move"
        onRetry={moveOptimistic.retry}
        onRollback={moveOptimistic.rollback}
        onDismiss={moveOptimistic.dismiss}
        renderContent={(data) => (
          <span>Issue #{data.issueNumber} → {data.column}</span>
        )}
      />

      <div className="gh-issue-board__columns">
        {COLUMNS.map(({ key, label, color }) => {
          const columnIssues = getIssuesByColumn(key)
          return (
            <div
              key={key}
              className="gh-kanban-column"
              onDrop={e => handleDrop(e, key)}
              onDragOver={handleDragOver}
            >
              <div className="gh-kanban-column__header">
                <span className="gh-kanban-column__dot" style={{ backgroundColor: color }} />
                <span className="gh-kanban-column__label">{label}</span>
                <span className="gh-kanban-column__count">{columnIssues.length}</span>
              </div>

              <div className="gh-kanban-column__body">
                {columnIssues.map(issue => (
                  <IssueCard key={issue.id} issue={issue} />
                ))}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
