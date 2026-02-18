import { useState } from 'react'
import { useSession } from '../hooks/useSession'
import './JoinSessionDialog.css'

interface JoinSessionDialogProps {
  isOpen: boolean
  onClose: () => void
}

/**
 * JoinSessionDialog — Modal para Guest entrar em uma sessão usando o código
 */
export function JoinSessionDialog({ isOpen, onClose }: JoinSessionDialogProps) {
  const { joinSession, cancelJoin, isWaitingApproval, joinResult, isLoading, error } = useSession()

  const [code, setCode] = useState('')
  const [name, setName] = useState('')

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!code.trim()) return

    try {
      await joinSession(code.trim(), name.trim() || undefined)
    } catch {
      // error is handled in store
    }
  }

  const handleClose = () => {
    if (isWaitingApproval) {
      cancelJoin()
    }
    setCode('')
    setName('')
    onClose()
  }

  // Formatação automática do código (XXX-YY)
  const handleCodeChange = (value: string) => {
    // Remover caracteres inválidos e formatar
    const cleaned = value.toUpperCase().replace(/[^A-Z0-9-]/g, '')

    // Auto-inserir hífen após 3 caracteres
    if (cleaned.length === 3 && !cleaned.includes('-') && code.length < cleaned.length) {
      setCode(cleaned + '-')
    } else if (cleaned.length <= 6) {
      setCode(cleaned)
    }
  }

  if (!isOpen) return null

  return (
    <div className="join-dialog__overlay" onClick={handleClose}>
      <div className="join-dialog animate-fade-in-up" onClick={(e) => e.stopPropagation()}>
        {/* Header */}
        <div className="join-dialog__header">
          <h2 className="join-dialog__title">Join Session</h2>
          <button className="btn btn--ghost btn--icon" onClick={handleClose} aria-label="Close dialog">
            ✕
          </button>
        </div>

        {/* Aguardando aprovação */}
        {isWaitingApproval && joinResult ? (
          <div className="join-dialog__waiting animate-fade-in">
            <div className="join-dialog__waiting-spinner" />
            <h3 className="join-dialog__waiting-title">Waiting for approval...</h3>
            <p className="join-dialog__waiting-text">
              Session: <strong>{code}</strong>
            </p>
            <p className="join-dialog__waiting-text">
              Host: <strong>{joinResult.hostName}</strong>
            </p>
            <button className="btn btn--ghost" onClick={handleClose}>
              Cancel
            </button>
          </div>
        ) : (
          /* Formulário de Join */
          <form className="join-dialog__form" onSubmit={handleSubmit}>
            <div className="join-dialog__form-group">
              <label className="join-dialog__label" htmlFor="session-code">
                Session Code
              </label>
              <input
                id="session-code"
                className="input input--mono join-dialog__code-input"
                type="text"
                value={code}
                onChange={(e) => handleCodeChange(e.target.value)}
                placeholder="XXX-YY"
                maxLength={6}
                autoFocus
                autoComplete="off"
                spellCheck={false}
              />
              <p className="join-dialog__hint">
                Ask the host for the 5-character session code
              </p>
            </div>

            <div className="join-dialog__form-group">
              <label className="join-dialog__label" htmlFor="guest-name">
                Your Name <span className="join-dialog__optional">(optional)</span>
              </label>
              <input
                id="guest-name"
                className="input"
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="How the host will see you"
                autoComplete="name"
              />
            </div>

            {error && <div className="join-dialog__error">{error}</div>}

            <div className="join-dialog__actions">
              <button type="button" className="btn btn--ghost" onClick={handleClose}>
                Cancel
              </button>
              <button
                type="submit"
                className="btn btn--primary"
                disabled={isLoading || code.length < 5}
              >
                {isLoading ? 'Joining...' : 'Join Session'}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  )
}
