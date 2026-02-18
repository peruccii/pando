import React from 'react'
import type { Issue } from '../types/github'

interface Props {
  issue: Issue
}

export const IssueCard: React.FC<Props> = ({ issue }) => {
  const handleDragStart = (e: React.DragEvent) => {
    e.dataTransfer.setData('issueNumber', String(issue.number))
    e.dataTransfer.effectAllowed = 'move'
  }

  return (
    <div
      className="gh-issue-card"
      draggable
      onDragStart={handleDragStart}
    >
      <div className="gh-issue-card__header">
        <span className="gh-issue-card__number">#{issue.number}</span>
        <span className={`gh-issue-card__state gh-issue-card__state--${issue.state.toLowerCase()}`}>
          {issue.state === 'OPEN' ? (
            <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
              <path d="M8 1.5a6.5 6.5 0 100 13 6.5 6.5 0 000-13zM0 8a8 8 0 1116 0A8 8 0 010 8z" />
            </svg>
          ) : (
            <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor">
              <path d="M11.28 6.78a.75.75 0 00-1.06-1.06L7.25 8.69 5.78 7.22a.75.75 0 00-1.06 1.06l2 2a.75.75 0 001.06 0l3.5-3.5z" />
              <path d="M16 8A8 8 0 110 8a8 8 0 0116 0zm-1.5 0a6.5 6.5 0 10-13 0 6.5 6.5 0 0013 0z" />
            </svg>
          )}
        </span>
      </div>

      <div className="gh-issue-card__title">{issue.title}</div>

      {issue.labels.length > 0 && (
        <div className="gh-issue-card__labels">
          {issue.labels.slice(0, 3).map(label => (
            <span
              key={label.name}
              className="gh-label gh-label--small"
              style={{ backgroundColor: `#${label.color}22`, color: `#${label.color}`, borderColor: `#${label.color}44` }}
            >
              {label.name}
            </span>
          ))}
          {issue.labels.length > 3 && (
            <span className="gh-label gh-label--more">+{issue.labels.length - 3}</span>
          )}
        </div>
      )}

      {issue.assignees.length > 0 && (
        <div className="gh-issue-card__assignees">
          {issue.assignees.map(a => (
            <img key={a.login} src={a.avatarUrl} alt={a.login} className="gh-avatar gh-avatar--small" title={a.login} />
          ))}
        </div>
      )}
    </div>
  )
}
