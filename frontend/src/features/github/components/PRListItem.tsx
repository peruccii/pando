import React from 'react'
import type { PullRequest } from '../types/github'

interface Props {
  pr: PullRequest
  onClick: () => void
}

export const PRListItem: React.FC<Props> = ({ pr, onClick }) => {
  const stateClass = pr.state.toLowerCase()
  const timeAgo = getTimeAgo(new Date(pr.updatedAt))

  return (
    <div className="gh-pr-item" onClick={onClick} role="button" tabIndex={0}>
      <div className="gh-pr-item__icon">
        {pr.state === 'MERGED' ? (
          <svg width="16" height="16" viewBox="0 0 16 16" fill="var(--gh-merged)">
            <path d="M5.45 5.154A4.25 4.25 0 004.75 6.5h1.876a2.25 2.25 0 01-1.176-1.346zM1.5 8.5a3.75 3.75 0 017.27-1.346 3.75 3.75 0 010 2.692A3.75 3.75 0 011.5 8.5zM11.329 5.5H9.426a4.25 4.25 0 01-.704 1.346A2.25 2.25 0 0111.329 5.5z" />
          </svg>
        ) : pr.state === 'CLOSED' ? (
          <svg width="16" height="16" viewBox="0 0 16 16" fill="var(--gh-closed)">
            <path d="M3.25 1A2.25 2.25 0 011 3.25v9.5A2.25 2.25 0 013.25 15h9.5A2.25 2.25 0 0115 12.75v-9.5A2.25 2.25 0 0112.75 1h-9.5zm6.03 4.28a.75.75 0 00-1.06-1.06L5.97 6.47 5.28 5.78a.75.75 0 00-1.06 1.06l1.25 1.25a.75.75 0 001.06 0l2.75-2.75z" />
          </svg>
        ) : (
          <svg width="16" height="16" viewBox="0 0 16 16" fill="var(--gh-open)">
            <path d="M7.177 3.073L9.573.677A.25.25 0 0110 .854v4.792a.25.25 0 01-.427.177L7.177 3.427a.25.25 0 010-.354zM3.75 2.5a.75.75 0 100 1.5.75.75 0 000-1.5zm-2.25.75a2.25 2.25 0 113 2.122v5.256a2.251 2.251 0 11-1.5 0V5.372A2.25 2.25 0 011.5 3.25zM11 2.5h-1V4h1a1 1 0 011 1v5.628a2.251 2.251 0 101.5 0V5A2.5 2.5 0 0011 2.5zm1 10.25a.75.75 0 111.5 0 .75.75 0 01-1.5 0zM3.75 12a.75.75 0 100 1.5.75.75 0 000-1.5z" />
          </svg>
        )}
      </div>

      <div className="gh-pr-item__content">
        <div className="gh-pr-item__title">
          <span className="gh-pr-item__name">{pr.title}</span>
          {pr.isDraft && <span className="gh-pr-item__draft">Draft</span>}
        </div>

        <div className="gh-pr-item__meta">
          <span className={`gh-pr-item__state gh-pr-item__state--${stateClass}`}>
            {pr.state}
          </span>
          <span className="gh-pr-item__number">#{pr.number}</span>
          <span className="gh-pr-item__author">
            {pr.author.avatarUrl && (
              <img src={pr.author.avatarUrl} alt="" className="gh-pr-item__avatar" />
            )}
            {pr.author.login}
          </span>
          <span className="gh-pr-item__time">{timeAgo}</span>
        </div>

        {pr.labels.length > 0 && (
          <div className="gh-pr-item__labels">
            {pr.labels.map(label => (
              <span
                key={label.name}
                className="gh-label"
                style={{ backgroundColor: `#${label.color}22`, color: `#${label.color}`, borderColor: `#${label.color}44` }}
              >
                {label.name}
              </span>
            ))}
          </div>
        )}
      </div>

      <div className="gh-pr-item__stats">
        <span className="gh-stat gh-stat--add">+{pr.additions}</span>
        <span className="gh-stat gh-stat--del">-{pr.deletions}</span>
        <span className="gh-stat gh-stat--files">{pr.changedFiles} files</span>
      </div>
    </div>
  )
}

function getTimeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  const months = Math.floor(days / 30)
  return `${months}mo ago`
}
