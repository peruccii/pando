import React, { useState } from 'react'
import { useDiff } from '../hooks/useDiff'

interface Props {
  prNumber: number
  path: string
  line: number
  onClose: () => void
}

export const InlineComment: React.FC<Props> = ({ prNumber, path, line, onClose }) => {
  const { createInlineComment } = useDiff()
  const [body, setBody] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!body.trim()) return
    setIsSubmitting(true)
    await createInlineComment(prNumber, body, path, line, 'RIGHT')
    setBody('')
    setIsSubmitting(false)
    onClose()
  }

  return (
    <div className="gh-inline-comment">
      <form onSubmit={handleSubmit}>
        <textarea
          className="gh-inline-comment__input"
          placeholder={`Comment on ${path}:${line}...`}
          value={body}
          onChange={e => setBody(e.target.value)}
          rows={3}
          autoFocus
        />
        <div className="gh-inline-comment__actions">
          <button type="button" className="gh-btn gh-btn--ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            type="submit"
            className="gh-btn gh-btn--primary"
            disabled={isSubmitting || !body.trim()}
          >
            {isSubmitting ? 'Sending...' : 'Add Comment'}
          </button>
        </div>
      </form>
    </div>
  )
}
