import React, { useEffect, useState } from 'react'
import { usePullRequests } from '../hooks/usePullRequests'
import { useDiff } from '../hooks/useDiff'
import { PRDiffViewer } from './PRDiffViewer'
import { ReviewPanel } from './ReviewPanel'
import { MergeDialog } from './MergeDialog'
import { AuthGuard } from '../../../components/AuthGuard'
import './github.css'

interface Props {
  onBack: () => void
}

export const PRDetail: React.FC<Props> = ({ onBack }) => {
  const { selectedPR, reviews, comments, fetchReviews, fetchComments, closePullRequest } = usePullRequests()
  const { loadDiff } = useDiff()
  const [showMergeDialog, setShowMergeDialog] = useState(false)

  useEffect(() => {
    if (selectedPR) {
      loadDiff(selectedPR.number)
      fetchReviews(selectedPR.number)
      fetchComments(selectedPR.number)
    }
  }, [selectedPR?.number]) // eslint-disable-line react-hooks/exhaustive-deps

  if (!selectedPR) return null

  const pr = selectedPR
  const stateClass = pr.state.toLowerCase()

  return (
    <div className="gh-pr-detail">
      {/* Header */}
      <div className="gh-pr-detail__header">
        <button className="gh-back-btn" onClick={onBack}>
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
            <path d="M7.78 12.53a.75.75 0 01-1.06 0L2.47 8.28a.75.75 0 010-1.06l4.25-4.25a.75.75 0 011.06 1.06L4.56 7.25h7.69a.75.75 0 010 1.5H4.56l3.22 3.22a.75.75 0 010 1.06z" />
          </svg>
          Back
        </button>

        <div className="gh-pr-detail__title-row">
          <h2 className="gh-pr-detail__title">{pr.title}</h2>
          <span className={`gh-pr-detail__state gh-pr-detail__state--${stateClass}`}>
            {pr.state}
          </span>
          {pr.isDraft && <span className="gh-pr-detail__draft">Draft</span>}
        </div>

        <div className="gh-pr-detail__meta">
          <span className="gh-pr-detail__number">#{pr.number}</span>
          <span className="gh-pr-detail__branches">
            <code>{pr.headBranch}</code>
            <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor" style={{ margin: '0 4px' }}>
              <path d="M8.22 2.97a.75.75 0 011.06 0l4.25 4.25a.75.75 0 010 1.06l-4.25 4.25a.75.75 0 01-1.06-1.06l2.97-2.97H3.75a.75.75 0 010-1.5h7.44L8.22 4.03a.75.75 0 010-1.06z" />
            </svg>
            <code>{pr.baseBranch}</code>
          </span>
          <span className="gh-pr-detail__author">
            {pr.author.avatarUrl && <img src={pr.author.avatarUrl} alt="" className="gh-avatar" />}
            {pr.author.login}
          </span>
        </div>

        {/* Labels */}
        {pr.labels.length > 0 && (
          <div className="gh-pr-detail__labels">
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

        {/* Stats */}
        <div className="gh-pr-detail__stats">
          <span className="gh-stat gh-stat--add">+{pr.additions}</span>
          <span className="gh-stat gh-stat--del">-{pr.deletions}</span>
          <span className="gh-stat gh-stat--files">{pr.changedFiles} files changed</span>
        </div>

        {/* Actions — Merge usa dialog síncrono (caso especial irreversível) */}
        {pr.state === 'OPEN' && (
          <div className="gh-pr-detail__actions">
            <AuthGuard action="Merge Pull Request" requireGitHub>
              <button
                className="gh-btn gh-btn--merge"
                onClick={() => setShowMergeDialog(true)}
              >
                Merge
              </button>
            </AuthGuard>

            <AuthGuard action="Close Pull Request" requireGitHub>
              <button
                className="gh-btn gh-btn--secondary"
                onClick={() => closePullRequest(pr.number)}
              >
                Close PR
              </button>
            </AuthGuard>
          </div>
        )}
      </div>

      {/* Body */}
      {pr.body && (
        <div className="gh-pr-detail__body">
          <h3>Description</h3>
          <div className="gh-pr-detail__body-content">{pr.body}</div>
        </div>
      )}

      {/* Diff Viewer (read-only — sem guard) */}
      <PRDiffViewer />

      {/* Review Panel (com optimistic UI integrado) */}
      <ReviewPanel
        prNumber={pr.number}
        reviews={reviews}
        comments={comments}
      />

      {/* Merge Confirmation Dialog (síncrono — NÃO optimistic) */}
      <MergeDialog
        isOpen={showMergeDialog}
        prNumber={pr.number}
        prTitle={pr.title}
        onClose={() => setShowMergeDialog(false)}
      />
    </div>
  )
}
