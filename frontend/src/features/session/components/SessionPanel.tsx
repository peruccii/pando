import { useEffect, useMemo, useState } from 'react'
import { useSession } from '../hooks/useSession'
import './SessionPanel.css'

interface PermissionPromptState {
  guestUserID: string
  guestName: string
  nextPermission: 'read_only' | 'read_write'
}

/**
 * SessionPanel — Painel principal de gerenciamento de sessão
 * Mostra o código de convite, guests conectados, logs de auditoria e controles do Host
 */
export function SessionPanel() {
  const {
    session,
    role,
    pendingGuests,
    isLoading,
    error,
    isSessionActive,
    isP2PConnected,
    auditLogs,
    isAuditLoading,
    createSession,
    endSession,
    approveGuest,
    rejectGuest,
    setGuestPermission,
    kickGuest,
    restartEnvironment,
    loadAuditLogs,
  } = useSession()

  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [showAuditLogs, setShowAuditLogs] = useState(false)
  const [permissionPrompt, setPermissionPrompt] = useState<PermissionPromptState | null>(null)

  const [createOpts, setCreateOpts] = useState({
    maxGuests: 10,
    mode: 'liveshare' as 'liveshare' | 'docker',
    allowAnonymous: false,
  })

  useEffect(() => {
    if (session?.id && role === 'host') {
      loadAuditLogs(session.id)
    }
  }, [loadAuditLogs, role, session?.id])

  const canRestartEnv = useMemo(() => {
    return role === 'host' && session?.mode === 'docker'
  }, [role, session?.mode])

  const handleCreate = async () => {
    try {
      await createSession(createOpts)
      setShowCreateDialog(false)
    } catch {
      // error is set in store
    }
  }

  const handleCopyCode = () => {
    if (session?.code) {
      navigator.clipboard.writeText(session.code)
    }
  }

  const handleTogglePermission = async (
    guestUserID: string,
    guestName: string,
    currentPermission: 'read_only' | 'read_write'
  ) => {
    const nextPermission = currentPermission === 'read_only' ? 'read_write' : 'read_only'

    if (nextPermission === 'read_write') {
      setPermissionPrompt({
        guestUserID,
        guestName,
        nextPermission,
      })
      return
    }

    await setGuestPermission(guestUserID, nextPermission)
  }

  const handleConfirmPermissionGrant = async () => {
    if (!permissionPrompt) return
    await setGuestPermission(permissionPrompt.guestUserID, permissionPrompt.nextPermission)
    setPermissionPrompt(null)
  }

  // Se não tem sessão — mostrar botão para criar
  if (!isSessionActive && role !== 'guest') {
    return (
      <div className="session-panel session-panel--empty">
        <div className="session-panel__icon">🤝</div>
        <h3 className="session-panel__title">Collaboration Session</h3>
        <p className="session-panel__subtitle">
          Start a session to collaborate in real-time with other developers
        </p>

        {!showCreateDialog ? (
          <button
            className="btn btn--primary session-panel__create-btn"
            onClick={() => setShowCreateDialog(true)}
            aria-label="Start collaboration session (Cmd+Shift+S)"
          >
            <span className="session-panel__btn-icon">🚀</span>
            Start Session
          </button>
        ) : (
          <div className="session-panel__create-form animate-fade-in-up">
            <div className="session-panel__form-group">
              <label className="session-panel__label">Max Guests</label>
              <input
                type="number"
                className="input"
                value={createOpts.maxGuests}
                onChange={(e) =>
                  setCreateOpts((prev) => ({ ...prev, maxGuests: parseInt(e.target.value, 10) || 10 }))
                }
                min={1}
                max={10}
              />
            </div>

            <div className="session-panel__form-group">
              <label className="session-panel__label">Mode</label>
              <select
                className="input"
                value={createOpts.mode}
                onChange={(e) =>
                  setCreateOpts((prev) => ({
                    ...prev,
                    mode: e.target.value as 'liveshare' | 'docker',
                  }))
                }
              >
                <option value="liveshare">Live Share</option>
                <option value="docker">Docker (Sandboxed)</option>
              </select>
            </div>

            <div className="session-panel__form-group session-panel__form-group--row">
              <label className="session-panel__label">
                <input
                  type="checkbox"
                  checked={createOpts.allowAnonymous}
                  onChange={(e) =>
                    setCreateOpts((prev) => ({ ...prev, allowAnonymous: e.target.checked }))
                  }
                />
                Allow anonymous guests
              </label>
            </div>

            <div className="session-panel__form-actions">
              <button className="btn btn--ghost" onClick={() => setShowCreateDialog(false)}>
                Cancel
              </button>
              <button
                className="btn btn--primary"
                onClick={handleCreate}
                disabled={isLoading}
              >
                {isLoading ? 'Creating...' : 'Create Session'}
              </button>
            </div>
          </div>
        )}

        {error && <div className="session-panel__error">{error}</div>}
      </div>
    )
  }

  // Sessão ativa — mostrar painel de Host
  return (
    <div className="session-panel session-panel--active">
      {/* Header */}
      <div className="session-panel__header">
        <div className="session-panel__header-left">
          <span className={`status-dot ${isP2PConnected ? 'status-dot--idle' : 'status-dot--running'}`} />
          <h3 className="session-panel__title">
            {role === 'host' ? 'Hosting Session' : 'Connected'}
          </h3>
        </div>
        {role === 'host' && (
          <div className="session-panel__header-actions">
            <button
              className="btn btn--ghost btn--icon"
              onClick={() => setShowAuditLogs((prev) => !prev)}
              title="Show audit logs"
              aria-label="Show audit logs"
            >
              📜
            </button>
            {canRestartEnv && (
              <button
                className="btn btn--ghost btn--icon"
                onClick={restartEnvironment}
                title="Restart Docker environment"
                aria-label="Restart Docker environment"
              >
                ♻️
              </button>
            )}
            <button
              className="btn btn--danger btn--icon"
              onClick={endSession}
              title="End Session"
              aria-label="End collaboration session"
            >
              ✕
            </button>
          </div>
        )}
      </div>

      {/* Código de convite */}
      {session && session.status === 'waiting' && (
        <div className="session-panel__code-section animate-fade-in-up">
          <p className="session-panel__code-label">Share this code with your collaborators:</p>
          <div className="session-panel__code-display" onClick={handleCopyCode} title="Click to copy">
            <span className="session-panel__code">{session.code}</span>
            <span className="session-panel__code-copy">📋</span>
          </div>
          <p className="session-panel__code-expiry">
            Expires in {Math.max(0, Math.ceil((new Date(session.expiresAt).getTime() - Date.now()) / 60000))} min
          </p>
        </div>
      )}

      {/* Waiting Room — Pedidos pendentes */}
      {pendingGuests.length > 0 && role === 'host' && (
        <div className="session-panel__waiting-room animate-fade-in-up">
          <h4 className="session-panel__section-title">
            🔔 Pending Requests ({pendingGuests.length})
          </h4>
          {pendingGuests.map((guest) => (
            <div key={guest.userID} className="session-panel__guest-request animate-fade-in-up">
              <div className="session-panel__guest-info">
                {guest.avatarUrl ? (
                  <img
                    src={guest.avatarUrl}
                    alt={guest.name}
                    className="session-panel__guest-avatar"
                  />
                ) : (
                  <div className="session-panel__guest-avatar session-panel__guest-avatar--placeholder">
                    👤
                  </div>
                )}
                <div className="session-panel__guest-details">
                  <span className="session-panel__guest-name">{guest.name}</span>
                  {guest.email && (
                    <span className="session-panel__guest-email">{guest.email}</span>
                  )}
                </div>
              </div>
              <div className="session-panel__guest-actions">
                <button
                  className="btn btn--primary btn--sm"
                  onClick={() => approveGuest(guest.userID)}
                  aria-label={`Approve ${guest.name}`}
                >
                  ✅ Approve
                </button>
                <button
                  className="btn btn--danger btn--sm"
                  onClick={() => rejectGuest(guest.userID)}
                  aria-label={`Reject ${guest.name}`}
                >
                  ❌ Reject
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Lista de Guests conectados */}
      {session && session.guests && session.guests.length > 0 && (
        <div className="session-panel__guests animate-fade-in">
          <h4 className="session-panel__section-title">
            Connected ({session.guests.filter((g) => g.status === 'connected' || g.status === 'approved').length})
          </h4>
          {session.guests
            .filter((g) => g.status === 'approved' || g.status === 'connected')
            .map((guest) => (
              <div key={guest.userID} className="session-panel__guest-item">
                <div className="session-panel__guest-info">
                  {guest.avatarUrl ? (
                    <img
                      src={guest.avatarUrl}
                      alt={guest.name}
                      className="session-panel__guest-avatar"
                    />
                  ) : (
                    <div className="session-panel__guest-avatar session-panel__guest-avatar--placeholder">
                      👤
                    </div>
                  )}
                  <div className="session-panel__guest-details">
                    <span className="session-panel__guest-name">{guest.name}</span>
                    <span className={`badge badge--${guest.permission === 'read_write' ? 'warning' : 'info'}`}>
                      {guest.permission === 'read_write' ? '✏️ Read/Write' : '👁️ Read Only'}
                    </span>
                  </div>
                </div>
                {role === 'host' && (
                  <div className="session-panel__guest-controls">
                    <button
                      className="btn btn--ghost btn--icon"
                      onClick={() => handleTogglePermission(guest.userID, guest.name, guest.permission)}
                      title={`Toggle permission for ${guest.name}`}
                    >
                      🔄
                    </button>
                    <button
                      className="btn btn--ghost btn--icon"
                      onClick={() => kickGuest(guest.userID)}
                      title={`Kick ${guest.name}`}
                    >
                      🚫
                    </button>
                  </div>
                )}
              </div>
            ))}
        </div>
      )}

      {showAuditLogs && role === 'host' && (
        <div className="session-panel__audit animate-fade-in-up">
          <div className="session-panel__audit-header">
            <h4 className="session-panel__section-title">Audit Logs</h4>
            <button className="btn btn--ghost btn--sm" onClick={() => loadAuditLogs()}>
              Refresh
            </button>
          </div>
          {isAuditLoading ? (
            <p className="session-panel__audit-empty">Loading logs...</p>
          ) : auditLogs.length === 0 ? (
            <p className="session-panel__audit-empty">No audit events yet.</p>
          ) : (
            <div className="session-panel__audit-list">
              {auditLogs.slice(0, 40).map((log) => (
                <div key={log.id} className="session-panel__audit-item">
                  <div className="session-panel__audit-main">
                    <code>{log.action}</code>
                    <span>{log.details || '-'}</span>
                  </div>
                  <div className="session-panel__audit-meta">
                    <span>{log.userID}</span>
                    <span>{new Date(log.createdAt).toLocaleString()}</span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {error && <div className="session-panel__error animate-fade-in">{error}</div>}

      {permissionPrompt && (
        <div className="session-panel__modal-overlay" role="presentation" onClick={() => setPermissionPrompt(null)}>
          <div className="session-panel__modal" role="dialog" aria-modal="true" onClick={(e) => e.stopPropagation()}>
            <h4>Security Confirmation</h4>
            <p>
              Granting <strong>Read/Write</strong> lets <strong>{permissionPrompt.guestName}</strong> execute commands on your environment.
            </p>
            <p className="session-panel__modal-warning">Only proceed if you trust this participant.</p>
            <div className="session-panel__modal-actions">
              <button className="btn btn--ghost" onClick={() => setPermissionPrompt(null)}>
                Cancel
              </button>
              <button className="btn btn--danger" onClick={handleConfirmPermissionGrant}>
                Grant Write Access
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
