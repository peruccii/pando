import { useEffect, useMemo, useState } from 'react'
import {
  Users,
  Play,
  ScrollText,
  RefreshCw,
  RotateCcw,
  ChevronDown,
  ChevronUp,
  X,
  Copy,
  User,
  Check,
  Shield,
  Eye,
  Trash2,
  AlertCircle
} from 'lucide-react'
import { useSession } from '../hooks/useSession'
import { useWorkspaceStore } from '../../../stores/workspaceStore'
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
    isWaitingApproval,
    wasRestoredSession,
    isP2PConnected,
    joinResult,
    auditLogs,
    isAuditLoading,
    createSession,
    endSession,
    approveGuest,
    rejectGuest,
    setGuestPermission,
    kickGuest,
    regenerateCode,
    revokeCode,
    setAllowNewJoins,
    restartEnvironment,
    loadAuditLogs,
  } = useSession()

  const [showCreateDialog, setShowCreateDialog] = useState(false)
  const [showAdvancedCreate, setShowAdvancedCreate] = useState(false)
  const [showAuditLogs, setShowAuditLogs] = useState(false)
  const [permissionPrompt, setPermissionPrompt] = useState<PermissionPromptState | null>(null)
  const [isCodeActionLoading, setIsCodeActionLoading] = useState(false)
  const workspaces = useWorkspaceStore((s) => s.workspaces)
  const activeWorkspaceId = useWorkspaceStore((s) => s.activeWorkspaceId)

  const [createOpts, setCreateOpts] = useState({
    maxGuests: 10,
    mode: 'liveshare' as 'liveshare' | 'docker',
    workspaceID: activeWorkspaceId ?? 0,
  })

  useEffect(() => {
    if (!showCreateDialog) {
      return
    }

    const fallbackWorkspaceID = activeWorkspaceId ?? workspaces[0]?.id ?? 0
    setCreateOpts((prev) => {
      if (prev.workspaceID > 0) {
        const stillExists = workspaces.some((workspace) => workspace.id === prev.workspaceID)
        if (stillExists) {
          return prev
        }
      }

      return {
        ...prev,
        workspaceID: fallbackWorkspaceID,
      }
    })
  }, [activeWorkspaceId, showCreateDialog, workspaces])

  useEffect(() => {
    if (session?.id && role === 'host') {
      loadAuditLogs(session.id)
    }
  }, [loadAuditLogs, role, session?.id])

  const canRestartEnv = useMemo(() => {
    return role === 'host' && session?.mode === 'docker'
  }, [role, session?.mode])

  const inviteIsOpen = Boolean(session?.allowNewJoins && session?.code)
  const inviteStatusLabel = inviteIsOpen ? 'OPEN' : 'PAUSED'
  const inviteExpiryMinutes = useMemo(() => {
    if (!session?.expiresAt) {
      return 0
    }
    return Math.max(0, Math.ceil((new Date(session.expiresAt).getTime() - Date.now()) / 60000))
  }, [session?.expiresAt])

  const headerTitle = useMemo(() => {
    if (role === 'host') return 'Hosting Session'
    if (isWaitingApproval) return 'Waiting Approval'
    return 'Connected'
  }, [isWaitingApproval, role])

  const handleCreate = async () => {
    try {
      await createSession(createOpts)
      setShowCreateDialog(false)
      setShowAdvancedCreate(false)
    } catch {
      // error is set in store
    }
  }

  const handleCopyCode = () => {
    if (session?.code && session.allowNewJoins) {
      navigator.clipboard.writeText(session.code)
    }
  }

  const handleRegenerateCode = async () => {
    if (!session || isCodeActionLoading) return
    setIsCodeActionLoading(true)
    try {
      await regenerateCode()
    } finally {
      setIsCodeActionLoading(false)
    }
  }

  const handleRevokeCode = async () => {
    if (!session || isCodeActionLoading) return
    setIsCodeActionLoading(true)
    try {
      await revokeCode()
    } finally {
      setIsCodeActionLoading(false)
    }
  }

  const handleToggleAllowNewJoins = async (allow: boolean) => {
    if (!session || isCodeActionLoading) return
    setIsCodeActionLoading(true)
    try {
      await setAllowNewJoins(allow)
    } finally {
      setIsCodeActionLoading(false)
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
        <div className="session-panel__icon">
          <Users size={32} />
        </div>
        <h3 className="session-panel__title">Collaboration Session</h3>
        <p className="session-panel__subtitle">
          Start a session to collaborate in real-time with other developers
        </p>

        {!showCreateDialog ? (
          <button
            className="btn btn--primary session-panel__create-btn"
            onClick={() => {
              setShowAdvancedCreate(false)
              setShowCreateDialog(true)
            }}
            aria-label="Start collaboration session (Cmd+Shift+S)"
          >
            <Play size={14} fill="currentColor" />
            Start Session
          </button>
        ) : (
          <div className="session-panel__create-form animate-fade-in-up">
            <div className="session-panel__form-group">
              <label className="session-panel__label">Workspace</label>
              <select
                className="input"
                value={createOpts.workspaceID || ''}
                onChange={(e) =>
                  setCreateOpts((prev) => ({
                    ...prev,
                    workspaceID: Number(e.target.value) || 0,
                  }))
                }
              >
                {workspaces.map((workspace) => (
                  <option key={workspace.id} value={workspace.id}>
                    {workspace.name}
                  </option>
                ))}
              </select>
            </div>

            <button
              type="button"
              className="btn btn--ghost btn--sm session-panel__advanced-toggle"
              onClick={() => setShowAdvancedCreate((current) => !current)}
            >
              {showAdvancedCreate ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
              {showAdvancedCreate ? 'Hide advanced options' : 'Advanced options'}
            </button>

            {showAdvancedCreate && (
              <div className="session-panel__advanced-grid animate-fade-in-up">
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

              </div>
            )}

            <div className="session-panel__form-actions">
              <button
                className="btn btn--ghost"
                onClick={() => {
                  setShowCreateDialog(false)
                  setShowAdvancedCreate(false)
                }}
              >
                Cancel
              </button>
              <button
                className="btn btn--primary"
                onClick={handleCreate}
                disabled={isLoading || createOpts.workspaceID <= 0}
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
            {headerTitle}
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
              <ScrollText size={16} />
            </button>
            {canRestartEnv && (
              <button
                className="btn btn--ghost btn--icon"
                onClick={restartEnvironment}
                title="Restart Docker environment"
                aria-label="Restart Docker environment"
              >
                <RefreshCw size={16} />
              </button>
            )}
            <button
              className="btn btn--danger btn--icon"
              onClick={endSession}
              title="End Session"
              aria-label="End collaboration session"
            >
              <X size={16} />
            </button>
          </div>
        )}
      </div>

      {role === 'host' && session && wasRestoredSession && (
        <div className="session-panel__restore-banner animate-fade-in-up">
          <div className="session-panel__restore-copy">
            <span className="session-panel__restore-badge">Sessão restaurada</span>
            <p className="session-panel__restore-text">
              Esta sessão foi recuperada após reiniciar o app.
            </p>
          </div>
          <button className="btn btn--danger btn--sm" onClick={endSession}>
            Encerrar sessão anterior
          </button>
        </div>
      )}

      {/* Código de convite */}
      {session && role === 'host' && (
        <div className="session-panel__code-section session-panel__invite-control animate-fade-in-up">
          <div className="session-panel__invite-head">
            <p className="session-panel__code-label">Invite Control</p>
            <span
              className={`session-panel__invite-status ${inviteIsOpen ? 'session-panel__invite-status--open' : 'session-panel__invite-status--paused'}`}
            >
              {inviteStatusLabel}
            </span>
          </div>

          {inviteIsOpen ? (
            <div className="session-panel__code-display" onClick={handleCopyCode} title="Click to copy">
              <span className="session-panel__code">{session.code}</span>
              <Copy size={16} className="session-panel__code-copy" />
            </div>
          ) : (
            <div className="session-panel__invite-paused">
              Invites are currently paused. Connected guests keep access.
            </div>
          )}

          <p className="session-panel__code-expiry">
            {inviteIsOpen ? `Code expires in ${inviteExpiryMinutes} min` : 'No active invite code'}
          </p>

          <div className="session-panel__code-controls session-panel__invite-actions">
            <button
              className="btn btn--primary btn--sm"
              onClick={handleCopyCode}
              disabled={isCodeActionLoading || isLoading || !inviteIsOpen}
            >
              <Copy size={12} /> Copy code
            </button>
            <button
              className="btn btn--ghost btn--sm"
              onClick={handleRegenerateCode}
              disabled={isCodeActionLoading || isLoading}
            >
              <RotateCcw size={12} /> Regenerate
            </button>
            <button
              className="btn btn--ghost btn--sm"
              onClick={() => handleToggleAllowNewJoins(!session.allowNewJoins)}
              disabled={isCodeActionLoading || isLoading}
            >
              {session.allowNewJoins ? 'Pause invites' : 'Enable invites'}
            </button>
            <button
              className="btn btn--ghost btn--sm"
              onClick={handleRevokeCode}
              disabled={isCodeActionLoading || isLoading || !session.code}
            >
              <X size={12} /> Revoke
            </button>
          </div>
        </div>
      )}

      {role === 'guest' && isWaitingApproval && (
        <div className="session-panel__audit animate-fade-in-up">
          <p className="session-panel__audit-empty">
            Waiting for host approval{joinResult?.hostName ? ` from ${joinResult.hostName}` : ''}.
          </p>
          <p className="session-panel__audit-empty">
            Keep this window open until the host accepts your request.
          </p>
        </div>
      )}

      {/* Waiting Room — Pedidos pendentes */}
      {pendingGuests.length > 0 && role === 'host' && (
        <div className="session-panel__waiting-room animate-fade-in-up">
          <h4 className="session-panel__section-title">
            Pending Requests ({pendingGuests.length})
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
                    <User size={14} />
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
                  <Check size={14} /> Approve
                </button>
                <button
                  className="btn btn--danger btn--sm"
                  onClick={() => rejectGuest(guest.userID)}
                  aria-label={`Reject ${guest.name}`}
                >
                  <X size={14} /> Reject
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
                      <User size={14} />
                    </div>
                  )}
                  <div className="session-panel__guest-details">
                    <span className="session-panel__guest-name">{guest.name}</span>
                    <span className={`badge badge--${guest.permission === 'read_write' ? 'warning' : 'info'}`}>
                      {guest.permission === 'read_write' ? (
                        <><Shield size={10} style={{ marginRight: 4 }} /> Read/Write</>
                      ) : (
                        <><Eye size={10} style={{ marginRight: 4 }} /> Read Only</>
                      )}
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
                      <RefreshCw size={14} />
                    </button>
                    <button
                      className="btn btn--ghost btn--icon"
                      onClick={() => kickGuest(guest.userID)}
                      title={`Kick ${guest.name}`}
                    >
                      <Trash2 size={14} />
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
              <RefreshCw size={12} style={{ marginRight: 4 }} /> Refresh
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
            <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
              <AlertCircle size={24} className="text-danger" />
              <h4 style={{ margin: 0 }}>Security Confirmation</h4>
            </div>
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
