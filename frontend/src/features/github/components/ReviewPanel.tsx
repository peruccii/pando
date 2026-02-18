import React, { useState } from 'react'
import type { Review, Comment } from '../types/github'
import { usePullRequests } from '../hooks/usePullRequests'
import { useOptimisticAction } from '../../../hooks/useOptimisticAction'
import { ConversationThread } from './ConversationThread'
import { OptimisticList } from '../../../components/OptimisticFeedback'
import { AuthGuard } from '../../../components/AuthGuard'

interface Props {
  prNumber: number
  reviews: Review[]
  comments: Comment[]
}

const REVIEW_EVENTS = [
  { label: 'Comment', value: 'COMMENT' },
  { label: 'Approve', value: 'APPROVE' },
  { label: 'Request Changes', value: 'REQUEST_CHANGES' },
]

export const ReviewPanel: React.FC<Props> = ({ prNumber, reviews, comments }) => {
  const { createReview, createComment, fetchReviews, fetchComments } = usePullRequests()
  const [reviewBody, setReviewBody] = useState('')
  const [reviewEvent, setReviewEvent] = useState('COMMENT')
  const [commentBody, setCommentBody] = useState('')

  // Optimistic UI para comentários
  const commentOptimistic = useOptimisticAction<{ body: string }, void>(
    async (data) => {
      await createComment(prNumber, data.body)
    },
    {
      maxRetries: 3,
      broadcastChannel: 'github:comments',
      onSuccess: () => {
        fetchComments(prNumber)
      },
      onError: (err) => {
        console.error('[ReviewPanel] Comment failed:', err.message)
      },
    }
  )

  // Optimistic UI para reviews
  const reviewOptimistic = useOptimisticAction<{ body: string; event: string }, void>(
    async (data) => {
      await createReview(prNumber, data.body, data.event)
    },
    {
      maxRetries: 3,
      broadcastChannel: 'github:reviews',
      onSuccess: () => {
        fetchReviews(prNumber)
      },
      onError: (err) => {
        console.error('[ReviewPanel] Review failed:', err.message)
      },
    }
  )

  const handleSubmitComment = (e: React.FormEvent) => {
    e.preventDefault()
    if (!commentBody.trim()) return
    commentOptimistic.execute({ body: commentBody })
    setCommentBody('') // Limpar input imediatamente (optimistic)
  }

  const handleSubmitReview = (e: React.FormEvent) => {
    e.preventDefault()
    if (!reviewBody.trim()) return
    reviewOptimistic.execute({ body: reviewBody, event: reviewEvent })
    setReviewBody('') // Limpar input imediatamente (optimistic)
  }

  return (
    <div className="gh-review-panel">
      <h3 className="gh-review-panel__title">Reviews & Discussion</h3>

      {/* Reviews */}
      {reviews.length > 0 && (
        <div className="gh-review-panel__reviews">
          {reviews.map(review => (
            <div key={review.id} className={`gh-review gh-review--${review.state.toLowerCase()}`}>
              <div className="gh-review__header">
                <img src={review.author.avatarUrl} alt="" className="gh-avatar" />
                <span className="gh-review__author">{review.author.login}</span>
                <span className={`gh-review__state gh-review__state--${review.state.toLowerCase()}`}>
                  {formatReviewState(review.state)}
                </span>
              </div>
              {review.body && <div className="gh-review__body">{review.body}</div>}
            </div>
          ))}
        </div>
      )}

      {/* Comments Thread */}
      <ConversationThread comments={comments} />

      {/* Optimistic feedback — comentários pendentes/erro */}
      <OptimisticList
        items={commentOptimistic.items}
        label="Comment"
        onRetry={commentOptimistic.retry}
        onRollback={commentOptimistic.rollback}
        onDismiss={commentOptimistic.dismiss}
        renderContent={(data) => <span>{data.body.slice(0, 80)}...</span>}
      />

      {/* Add Comment */}
      <AuthGuard action="Comment" requireGitHub>
        <form className="gh-comment-form" onSubmit={handleSubmitComment}>
          <textarea
            className="gh-comment-form__input"
            placeholder="Leave a comment..."
            value={commentBody}
            onChange={e => setCommentBody(e.target.value)}
            rows={3}
          />
          <div className="gh-comment-form__footer">
            <button
              className="gh-btn gh-btn--primary"
              type="submit"
              disabled={commentOptimistic.hasPending || !commentBody.trim()}
            >
              {commentOptimistic.hasPending ? 'Sending...' : 'Comment'}
            </button>
          </div>
        </form>
      </AuthGuard>

      {/* Optimistic feedback — reviews pendentes/erro */}
      <OptimisticList
        items={reviewOptimistic.items}
        label="Review"
        onRetry={reviewOptimistic.retry}
        onRollback={reviewOptimistic.rollback}
        onDismiss={reviewOptimistic.dismiss}
        renderContent={(data) => (
          <span>{data.event}: {data.body.slice(0, 60)}...</span>
        )}
      />

      {/* Submit Review */}
      <AuthGuard action="Submit Review" requireGitHub>
        <form className="gh-review-form" onSubmit={handleSubmitReview}>
          <h4>Submit Review</h4>
          <textarea
            className="gh-review-form__input"
            placeholder="Write your review..."
            value={reviewBody}
            onChange={e => setReviewBody(e.target.value)}
            rows={4}
          />
          <div className="gh-review-form__actions">
            <select
              className="gh-review-form__select"
              value={reviewEvent}
              onChange={e => setReviewEvent(e.target.value)}
            >
              {REVIEW_EVENTS.map(({ label, value }) => (
                <option key={value} value={value}>{label}</option>
              ))}
            </select>
            <button
              className="gh-btn gh-btn--primary"
              type="submit"
              disabled={reviewOptimistic.hasPending || !reviewBody.trim()}
            >
              {reviewOptimistic.hasPending ? 'Submitting...' : 'Submit Review'}
            </button>
          </div>
        </form>
      </AuthGuard>
    </div>
  )
}

function formatReviewState(state: string): string {
  switch (state) {
    case 'APPROVED': return '✅ Approved'
    case 'CHANGES_REQUESTED': return '🔴 Changes Requested'
    case 'COMMENTED': return '💬 Commented'
    case 'PENDING': return '⏳ Pending'
    default: return state
  }
}
