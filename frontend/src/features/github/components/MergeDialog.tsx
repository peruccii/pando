import React, { useState } from 'react'
import { usePullRequests } from '../hooks/usePullRequests'
import { AlertTriangle, Loader2, CheckCircle, XCircle } from 'lucide-react'
import type { MergeMethod } from '../types/github'

interface Props {
  isOpen: boolean
  prNumber: number
  prTitle?: string
  onClose: () => void
}

type MergeState = 'confirm' | 'merging' | 'success' | 'error'

const MERGE_METHODS: { label: string; value: MergeMethod; description: string }[] = [
  { label: 'Create a merge commit', value: 'MERGE', description: 'All commits will be added to the base branch via a merge commit.' },
  { label: 'Squash and merge', value: 'SQUASH', description: 'All commits will be combined into a single commit on the base branch.' },
  { label: 'Rebase and merge', value: 'REBASE', description: 'All commits will be rebased and added to the base branch.' },
]

/**
 * MergeDialog — Modal de confirmação síncrono para Merge PR.
 *
 * Merge é irreversível, portanto NÃO usa Optimistic UI.
 * Pipeline síncrono:
 *   1. Modal de confirmação com warning
 *   2. Loading spinner durante merge
 *   3. Sucesso → exibe confirmação + fecha
 *   4. Erro → exibe erro inline + opção de retry
 */
export const MergeDialog: React.FC<Props> = ({ isOpen, prNumber, prTitle, onClose }) => {
  const { mergePullRequest } = usePullRequests()
  const [method, setMethod] = useState<MergeMethod>('SQUASH')
  const [mergeState, setMergeState] = useState<MergeState>('confirm')
  const [errorMsg, setErrorMsg] = useState('')

  if (!isOpen) return null

  const handleMerge = async () => {
    setMergeState('merging')
    setErrorMsg('')

    try {
      const success = await mergePullRequest(prNumber, method)
      if (success) {
        setMergeState('success')
        // Auto-fechar após 1.5s
        setTimeout(() => {
          onClose()
          setMergeState('confirm')
        }, 1500)
      } else {
        setMergeState('error')
        setErrorMsg('Merge failed. The PR may have conflicts or require status checks.')
      }
    } catch (err) {
      setMergeState('error')
      setErrorMsg(err instanceof Error ? err.message : 'Unknown error during merge')
    }
  }

  const handleClose = () => {
    if (mergeState === 'merging') return // Não fechar durante merge
    onClose()
    setMergeState('confirm')
    setErrorMsg('')
  }

  return (
    <div className="gh-dialog-overlay" onClick={handleClose}>
      <div className="gh-dialog gh-dialog--small" onClick={e => e.stopPropagation()}>
        {/* Header */}
        <div className="gh-dialog__header">
          <h3>
            {mergeState === 'success'
              ? '✅ Merged!'
              : `Merge Pull Request #${prNumber}`}
          </h3>
          {mergeState !== 'merging' && (
            <button className="gh-dialog__close" onClick={handleClose}>×</button>
          )}
        </div>

        {/* Body */}
        <div className="gh-dialog__body">
          {/* Confirmation state */}
          {mergeState === 'confirm' && (
            <>
              {/* Warning banner */}
              <div className="gh-merge-warning">
                <AlertTriangle size={16} />
                <span>
                  Esta ação é <strong>irreversível</strong>. As alterações serão
                  aplicadas permanentemente à branch base.
                </span>
              </div>

              {prTitle && (
                <p className="gh-merge-pr-title">"{prTitle}"</p>
              )}

              {/* Method selector */}
              <div className="gh-merge-methods">
                {MERGE_METHODS.map(({ label, value, description }) => (
                  <label key={value} className={`gh-merge-method ${method === value ? 'gh-merge-method--selected' : ''}`}>
                    <input
                      type="radio"
                      name="mergeMethod"
                      value={value}
                      checked={method === value}
                      onChange={() => setMethod(value)}
                    />
                    <div>
                      <div className="gh-merge-method__label">{label}</div>
                      <div className="gh-merge-method__desc">{description}</div>
                    </div>
                  </label>
                ))}
              </div>
            </>
          )}

          {/* Merging state */}
          {mergeState === 'merging' && (
            <div className="gh-merge-loading">
              <Loader2 size={32} className="gh-merge-spinner" />
              <p>Merging pull request...</p>
              <p className="gh-merge-loading__sub">This may take a few seconds</p>
            </div>
          )}

          {/* Success state */}
          {mergeState === 'success' && (
            <div className="gh-merge-success">
              <CheckCircle size={32} />
              <p>Pull request #{prNumber} merged successfully!</p>
            </div>
          )}

          {/* Error state */}
          {mergeState === 'error' && (
            <div className="gh-merge-error">
              <XCircle size={24} />
              <p>{errorMsg}</p>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="gh-dialog__footer">
          {mergeState === 'confirm' && (
            <>
              <button className="gh-btn gh-btn--secondary" onClick={handleClose}>
                Cancel
              </button>
              <button className="gh-btn gh-btn--merge" onClick={handleMerge}>
                Confirm {method.toLowerCase()}
              </button>
            </>
          )}

          {mergeState === 'error' && (
            <>
              <button className="gh-btn gh-btn--secondary" onClick={handleClose}>
                Cancel
              </button>
              <button className="gh-btn gh-btn--merge" onClick={handleMerge}>
                Try Again
              </button>
            </>
          )}

          {mergeState === 'merging' && (
            <button className="gh-btn gh-btn--secondary" disabled>
              Merging...
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
