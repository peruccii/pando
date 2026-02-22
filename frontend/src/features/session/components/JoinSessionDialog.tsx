import { useEffect, useMemo, useState } from 'react'
import { X, Loader2 } from 'lucide-react'
import { useSession } from '../hooks/useSession'
import {
  isSessionCodeReady,
  normalizeSessionCode,
  sanitizeSessionCodeInput,
} from '../sessionCode'
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
    return joinResult?.sessionCode || normalizeSessionCode(code)
  }, [code, joinResult?.sessionCode])

  const isCodeReady = useMemo(() => {
    return isSessionCodeReady(code)
  }, [code])

  const codePreviewSlots = useMemo(() => {
    const compact = normalizeSessionCode(code).replace(/-/g, '').slice(0, 7)
    return compact.padEnd(7, ' ').split('')
  }, [code])

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
    if (!isSessionCodeReady(code)) return

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

  // Formatação automática do código (XXXX-XXX). Mantém legado XXX-YY quando o usuário digita com hífen.
  const handleCodeChange = (value: string) => {
    const cleaned = sanitizeSessionCodeInput(value)

    if (cleaned.includes('-')) {
      const [leftRaw = '', rightRaw = ''] = cleaned.split('-', 2)
      const legacyStyle = leftRaw.length <= 3
      const leftLen = legacyStyle ? 3 : 4
      const rightLen = legacyStyle ? 2 : 3
      const left = leftRaw.replace(/-/g, '').slice(0, leftLen)
      const right = rightRaw.replace(/-/g, '').slice(0, rightLen)
      setCode(right.length > 0 || cleaned.endsWith('-') ? `${left}-${right}` : left)
      return
    }

    const compact = cleaned.replace(/-/g, '').slice(0, 7)
    if (compact.length === 4 && code.length < compact.length) {
      setCode(`${compact}-`)
      return
    }
    if (compact.length > 4) {
      setCode(`${compact.slice(0, 4)}-${compact.slice(4, 7)}`)
      return
    }
    setCode(compact)
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
              <div className="join-dialog__code-preview" aria-hidden="true">
                <span className={`join-dialog__code-slot ${codePreviewSlots[0].trim() ? 'join-dialog__code-slot--filled' : ''}`}>{codePreviewSlots[0]}</span>
                <span className={`join-dialog__code-slot ${codePreviewSlots[1].trim() ? 'join-dialog__code-slot--filled' : ''}`}>{codePreviewSlots[1]}</span>
                <span className={`join-dialog__code-slot ${codePreviewSlots[2].trim() ? 'join-dialog__code-slot--filled' : ''}`}>{codePreviewSlots[2]}</span>
                <span className={`join-dialog__code-slot ${codePreviewSlots[3].trim() ? 'join-dialog__code-slot--filled' : ''}`}>{codePreviewSlots[3]}</span>
                <span className="join-dialog__code-separator">-</span>
                <span className={`join-dialog__code-slot ${codePreviewSlots[4].trim() ? 'join-dialog__code-slot--filled' : ''}`}>{codePreviewSlots[4]}</span>
                <span className={`join-dialog__code-slot ${codePreviewSlots[5].trim() ? 'join-dialog__code-slot--filled' : ''}`}>{codePreviewSlots[5]}</span>
                <span className={`join-dialog__code-slot ${codePreviewSlots[6].trim() ? 'join-dialog__code-slot--filled' : ''}`}>{codePreviewSlots[6]}</span>
              </div>
              <input
                id="session-code"
                className="input input--mono join-dialog__code-input"
                type="text"
                value={code}
                onChange={(e) => handleCodeChange(e.target.value)}
                placeholder="XXXX-XXX"
                maxLength={8}
                autoFocus
                autoComplete="off"
                spellCheck={false}
              />
              <p className="join-dialog__hint">
                Ask the host for the invite code (`XXXX-XXX` or legacy `XXX-YY`)
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
                disabled={isLoading || !isCodeReady}
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
