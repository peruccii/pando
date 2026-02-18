import React from 'react'
import type { Comment } from '../types/github'

interface Props {
  comments: Comment[]
}

export const ConversationThread: React.FC<Props> = ({ comments }) => {
  if (comments.length === 0) return null

  // Group inline comments by path
  const generalComments = comments.filter(c => !c.path)
  const inlineComments = comments.filter(c => c.path)

  return (
    <div className="gh-conversation">
      {generalComments.length > 0 && (
        <div className="gh-conversation__general">
          {generalComments.map(comment => (
            <CommentItem key={comment.id} comment={comment} />
          ))}
        </div>
      )}

      {inlineComments.length > 0 && (
        <div className="gh-conversation__inline">
          <h4 className="gh-conversation__inline-title">Inline Comments</h4>
          {inlineComments.map(comment => (
            <CommentItem key={comment.id} comment={comment} showPath />
          ))}
        </div>
      )}
    </div>
  )
}

const CommentItem: React.FC<{ comment: Comment; showPath?: boolean }> = ({ comment, showPath }) => {
  const timeAgo = getTimeAgo(new Date(comment.createdAt))

  return (
    <div className="gh-comment">
      <div className="gh-comment__header">
        <img src={comment.author.avatarUrl} alt="" className="gh-avatar" />
        <span className="gh-comment__author">{comment.author.login}</span>
        <span className="gh-comment__time">{timeAgo}</span>
        {showPath && comment.path && (
          <span className="gh-comment__path">
            {comment.path}{comment.line != null ? `:${comment.line}` : ''}
          </span>
        )}
      </div>
      <div className="gh-comment__body">{comment.body}</div>
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
  return `${days}d ago`
}
