import { useEffect, useMemo, useState } from 'react'
import { X, Loader2 } from 'lucide-react'
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
  const [remainingSeconds, setRemainingSeconds] = useState(5 * 60)

  const waitingCode = useMemo(() => {
    return joinResult?.sessionCode || code
  }, [code, joinResult?.sessionCode])

  useEffect(() => {
    if (!isWaitingApproval) {
      setRemainingSeconds(5 * 60)
      return
    }

    const deadline = Date.parse(joinResult?.approvalExpiresAt || '')
    const fallbackDeadline = Date.now() + 5 * 60 * 1000
    const target = Number.isNaN(deadline) ? fallbackDeadline : deadline

    const tick = () => {
      const seconds = Math.max(0, Math.ceil((target - Date.now()) / 1000))
      setRemainingSeconds(seconds)
    }

    tick()
    const timer = window.setInterval(tick, 1000)

    return () => {
      window.clearInterval(timer)
    }
  }, [isWaitingApproval, joinResult?.approvalExpiresAt])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!code.trim()) return

    try {
      await joinSession(code.trim(), name.trim() || undefined)
    } catch {
      // error is handled in store
    }
  }

  const resetForm = () => {
    setCode('')
    setName('')
  }

  const handleDismiss = () => {
    if (!isWaitingApproval) {
      resetForm()
    }
    onClose()
  }

  const handleCancelWaiting = () => {
    cancelJoin()
    resetForm()
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
    <div className="join-dialog__overlay" onClick={handleDismiss}>
      <div className="join-dialog animate-fade-in-up" onClick={(e) => e.stopPropagation()}>
        {/* Header */}
        <div className="join-dialog__header">
          <h2 className="join-dialog__title">Join Session</h2>
          <button className="btn btn--ghost btn--icon" onClick={handleDismiss} aria-label="Close dialog">
            <X size={18} />
          </button>
        </div>

        {/* Aguardando aprovação */}
        {isWaitingApproval && joinResult ? (
          <div className="join-dialog__waiting animate-fade-in">
            <Loader2 className="join-dialog__waiting-spinner" size={32} />
            <h3 className="join-dialog__waiting-title">Waiting for approval...</h3>
            <p className="join-dialog__waiting-text">
              Session: <strong>{waitingCode}</strong>
            </p>
            <p className="join-dialog__waiting-text">
              Host: <strong>{joinResult.hostName}</strong>
            </p>
            {joinResult.workspaceName && (
              <div className="join-dialog__waiting-workspace-alert">
                <p><strong>"{joinResult.workspaceName}"</strong> será sincronizada após aprovação e conexão com o host.</p>
              </div>
            )}
            <p className="join-dialog__waiting-deadline">
              Time remaining: <strong>{Math.floor(remainingSeconds / 60)}:{String(remainingSeconds % 60).padStart(2, '0')}</strong>
            </p>
            <p className="join-dialog__waiting-hint">
              You can close this window. The request keeps running in background.
            </p>
            <button className="btn btn--ghost" onClick={handleCancelWaiting}>
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
              <button type="button" className="btn btn--ghost" onClick={handleDismiss}>
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
