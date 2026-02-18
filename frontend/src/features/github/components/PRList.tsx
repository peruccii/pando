import React from 'react'
import { usePullRequests } from '../hooks/usePullRequests'
import { PRListItem } from './PRListItem'
import type { PRState } from '../types/github'
import { AuthGuard } from '../../../components/AuthGuard'
import './github.css'

const PR_STATES: { label: string; value: PRState }[] = [
  { label: 'Open', value: 'OPEN' },
  { label: 'Closed', value: 'CLOSED' },
  { label: 'Merged', value: 'MERGED' },
  { label: 'All', value: 'ALL' },
]

export const PRList: React.FC = () => {
  const { pullRequests, prFilter, isLoading, filterByState, selectPR } = usePullRequests()

  return (
    <div className="gh-pr-list">
      <div className="gh-pr-list__header">
        <h2 className="gh-pr-list__title">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
            <path d="M7.177 3.073L9.573.677A.25.25 0 0110 .854v4.792a.25.25 0 01-.427.177L7.177 3.427a.25.25 0 010-.354zM3.75 2.5a.75.75 0 100 1.5.75.75 0 000-1.5zm-2.25.75a2.25 2.25 0 113 2.122v5.256a2.251 2.251 0 11-1.5 0V5.372A2.25 2.25 0 011.5 3.25zM11 2.5h-1V4h1a1 1 0 011 1v5.628a2.251 2.251 0 101.5 0V5A2.5 2.5 0 0011 2.5zm1 10.25a.75.75 0 111.5 0 .75.75 0 01-1.5 0zM3.75 12a.75.75 0 100 1.5.75.75 0 000-1.5z" />
          </svg>
          Pull Requests
          <span className="gh-pr-list__count">{pullRequests.length}</span>
        </h2>

        <div className="gh-pr-list__actions">
          <AuthGuard action="Create Pull Request" requireGitHub>
            <button
              className="gh-btn gh-btn--sm gh-btn--primary"
              onClick={() => console.log('Open Create PR Dialog')}
            >
              + New
            </button>
          </AuthGuard>
        </div>

        <div className="gh-pr-list__filters">
          {PR_STATES.map(({ label, value }) => (
            <button
              key={value}
              className={`gh-filter-btn ${prFilter === value ? 'gh-filter-btn--active' : ''}`}
              onClick={() => filterByState(value)}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      <div className="gh-pr-list__body">
        {isLoading && pullRequests.length === 0 ? (
          <div className="gh-pr-list__loading">
            <div className="gh-spinner" />
            <span>Loading pull requests...</span>
          </div>
        ) : pullRequests.length === 0 ? (
          <div className="gh-pr-list__empty">
            <svg width="48" height="48" viewBox="0 0 16 16" fill="currentColor" opacity="0.3">
              <path d="M7.177 3.073L9.573.677A.25.25 0 0110 .854v4.792a.25.25 0 01-.427.177L7.177 3.427a.25.25 0 010-.354zM3.75 2.5a.75.75 0 100 1.5.75.75 0 000-1.5zm-2.25.75a2.25 2.25 0 113 2.122v5.256a2.251 2.251 0 11-1.5 0V5.372A2.25 2.25 0 011.5 3.25zM11 2.5h-1V4h1a1 1 0 011 1v5.628a2.251 2.251 0 101.5 0V5A2.5 2.5 0 0011 2.5zm1 10.25a.75.75 0 111.5 0 .75.75 0 01-1.5 0zM3.75 12a.75.75 0 100 1.5.75.75 0 000-1.5z" />
            </svg>
            <p>No pull requests found</p>
          </div>
        ) : (
          pullRequests.map(pr => (
            <PRListItem key={pr.id} pr={pr} onClick={() => selectPR(pr)} />
          ))
        )}
      </div>
    </div>
  )
}
