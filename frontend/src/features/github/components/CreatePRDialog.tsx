import React, { useState } from 'react'
import { usePullRequests } from '../hooks/usePullRequests'
import { useBranches } from '../hooks/useBranches'

interface Props {
  isOpen: boolean
  onClose: () => void
}

export const CreatePRDialog: React.FC<Props> = ({ isOpen, onClose }) => {
  const { createPullRequest } = usePullRequests()
  const { branches } = useBranches()
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [headBranch, setHeadBranch] = useState('')
  const [baseBranch, setBaseBranch] = useState('main')
  const [isDraft, setIsDraft] = useState(false)
  const [isSubmitting, setIsSubmitting] = useState(false)

  if (!isOpen) return null

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!title.trim() || !headBranch) return

    setIsSubmitting(true)
    const pr = await createPullRequest(title, body, headBranch, baseBranch, isDraft)
    setIsSubmitting(false)

    if (pr) {
      setTitle('')
      setBody('')
      setHeadBranch('')
      setIsDraft(false)
      onClose()
    }
  }

  return (
    <div className="gh-dialog-overlay" onClick={onClose}>
      <div className="gh-dialog" onClick={e => e.stopPropagation()}>
        <div className="gh-dialog__header">
          <h3>Create Pull Request</h3>
          <button className="gh-dialog__close" onClick={onClose}>×</button>
        </div>

        <form onSubmit={handleSubmit}>
          <div className="gh-dialog__body">
            <div className="gh-dialog__branches">
              <div className="gh-form-group">
                <label>Head Branch</label>
                <select value={headBranch} onChange={e => setHeadBranch(e.target.value)} required>
                  <option value="">Select branch...</option>
                  {branches.map(b => (
                    <option key={b.name} value={b.name}>{b.name}</option>
                  ))}
                </select>
              </div>
              <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor" style={{ margin: '24px 8px 0' }}>
                <path d="M8.22 2.97a.75.75 0 011.06 0l4.25 4.25a.75.75 0 010 1.06l-4.25 4.25a.75.75 0 01-1.06-1.06l2.97-2.97H3.75a.75.75 0 010-1.5h7.44L8.22 4.03a.75.75 0 010-1.06z" />
              </svg>
              <div className="gh-form-group">
                <label>Base Branch</label>
                <select value={baseBranch} onChange={e => setBaseBranch(e.target.value)}>
                  {branches.map(b => (
                    <option key={b.name} value={b.name}>{b.name}</option>
                  ))}
                </select>
              </div>
            </div>

            <div className="gh-form-group">
              <label>Title</label>
              <input
                type="text"
                value={title}
                onChange={e => setTitle(e.target.value)}
                placeholder="PR title..."
                required
              />
            </div>

            <div className="gh-form-group">
              <label>Description</label>
              <textarea
                value={body}
                onChange={e => setBody(e.target.value)}
                placeholder="Describe your changes..."
                rows={6}
              />
            </div>

            <label className="gh-checkbox">
              <input
                type="checkbox"
                checked={isDraft}
                onChange={e => setIsDraft(e.target.checked)}
              />
              Create as draft
            </label>
          </div>

          <div className="gh-dialog__footer">
            <button type="button" className="gh-btn gh-btn--secondary" onClick={onClose}>
              Cancel
            </button>
            <button
              type="submit"
              className="gh-btn gh-btn--primary"
              disabled={isSubmitting || !title.trim() || !headBranch}
            >
              {isSubmitting ? 'Creating...' : 'Create Pull Request'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
