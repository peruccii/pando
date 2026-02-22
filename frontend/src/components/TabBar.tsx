import { FormEvent, KeyboardEvent, DragEvent, CSSProperties, useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { Plus, X, Pencil, Palette } from 'lucide-react'
import { useWorkspaceStore } from '../stores/workspaceStore'
import { useSessionStore } from '../features/session/stores/sessionStore'
import { useAuthStore } from '../stores/authStore'
import {
  clearTerminalWorkspaceDragPayload,
  hasTerminalWorkspaceDragPayload,
  readTerminalWorkspaceDragPayload,
} from '../features/command-center/utils/terminalWorkspaceDnd'
import './TabBar.css'

const TAB_COLORS = [
  { name: 'Default', value: '' },
  { name: 'Blue', value: '#3b82f6' },
  { name: 'Green', value: '#10b981' },
  { name: 'Red', value: '#ef4444' },
  { name: 'Yellow', value: '#f59e0b' },
  { name: 'Purple', value: '#8b5cf6' },
  { name: 'Pink', value: '#ec4899' },
  { name: 'Orange', value: '#f97316' },
]
const MAX_VISIBLE_COLLAB_AVATARS = 9

interface SessionParticipant {
  userID: string
  name: string
  avatarUrl?: string
}

export function TabBar() {
  const workspaces = useWorkspaceStore((s) => s.workspaces)
  const isSessionWorkspaceScoped = useWorkspaceStore((s) => s.isSessionWorkspaceScoped)
  const activeWorkspaceId = useWorkspaceStore((s) => s.activeWorkspaceId)
  const setActiveWorkspace = useWorkspaceStore((s) => s.setActiveWorkspace)
  const createWorkspace = useWorkspaceStore((s) => s.createWorkspace)
  const renameWorkspace = useWorkspaceStore((s) => s.renameWorkspace)
  const setWorkspaceColor = useWorkspaceStore((s) => s.setWorkspaceColor)
  const deleteWorkspace = useWorkspaceStore((s) => s.deleteWorkspace)
  const moveTerminalToWorkspace = useWorkspaceStore((s) => s.moveTerminalToWorkspace)
  const session = useSessionStore((s) => s.session)
  const sessionRole = useSessionStore((s) => s.role)
  const authUser = useAuthStore((s) => s.user)
  const hasCollaborativeSession = Boolean(session?.id)

  const [editingWorkspaceId, setEditingWorkspaceId] = useState<number | null>(null)
  const [draftName, setDraftName] = useState('')
  const [dropWorkspaceId, setDropWorkspaceId] = useState<number | null>(null)
  const suppressNextTabClickRef = useRef(false)

  const [contextMenu, setContextMenu] = useState<{
    x: number
    y: number
    workspaceId: number
    workspaceName: string
    workspaceColor: string
  } | null>(null)

  const isGuestScoped = sessionRole === 'guest' && hasCollaborativeSession
  const canDeleteWorkspace = !isGuestScoped && workspaces.length > 1

  const scopedWorkspaceID = useMemo(() => {
    if (!session?.id) {
      return null
    }

    if (sessionRole === 'host') {
      const hostWorkspaceID = Number(session.config?.workspaceID || 0)
      return Number.isInteger(hostWorkspaceID) && hostWorkspaceID > 0 ? hostWorkspaceID : null
    }

    if (sessionRole === 'guest' && isSessionWorkspaceScoped && activeWorkspaceId != null) {
      return activeWorkspaceId
    }

    return null
  }, [activeWorkspaceId, isSessionWorkspaceScoped, session?.config?.workspaceID, session?.id, sessionRole])

  const sharedWorkspaceParticipants = useMemo(() => {
    if (!session?.id || !scopedWorkspaceID) {
      return [] as SessionParticipant[]
    }

    const participantsByUserID = new Map<string, SessionParticipant>()
    const upsertParticipant = (userID: string, name: string, avatarUrl?: string) => {
      const normalizedUserID = userID.trim()
      if (!normalizedUserID) {
        return
      }

      const normalizedName = name.trim()
      const normalizedAvatar = avatarUrl?.trim()
      const existing = participantsByUserID.get(normalizedUserID)

      if (existing) {
        participantsByUserID.set(normalizedUserID, {
          userID: existing.userID,
          name: existing.name || normalizedName || normalizedUserID,
          avatarUrl: existing.avatarUrl || normalizedAvatar,
        })
        return
      }

      participantsByUserID.set(normalizedUserID, {
        userID: normalizedUserID,
        name: normalizedName || normalizedUserID,
        avatarUrl: normalizedAvatar,
      })
    }

    const hostUserID = (session.hostUserID || '').trim()
    const hostName = (session.hostName || '').trim()
    const hostAvatar = (session.hostAvatarUrl || '').trim()
    upsertParticipant(hostUserID, hostName || hostUserID || 'Host', hostAvatar)

    for (const guest of session.guests) {
      if (guest.status !== 'approved' && guest.status !== 'connected') {
        continue
      }
      upsertParticipant(guest.userID, guest.name, guest.avatarUrl)
    }

    if (authUser?.id) {
      upsertParticipant(authUser.id, authUser.name || authUser.id, authUser.avatarUrl)
    }

    const participants = Array.from(participantsByUserID.values())
    participants.sort((a, b) => {
      if (a.userID === hostUserID) return -1
      if (b.userID === hostUserID) return 1
      return a.name.localeCompare(b.name)
    })
    return participants
  }, [authUser?.avatarUrl, authUser?.id, authUser?.name, scopedWorkspaceID, session])

  const handleCreateWorkspace = async () => {
    if (isGuestScoped) {
      return
    }
    try {
      await createWorkspace()
    } catch (err) {
      console.error('[TabBar] Failed to create workspace:', err)
    }
  }

  const handleDeleteWorkspace = async (workspaceId: number) => {
    if (isGuestScoped) {
      return
    }
    try {
      await deleteWorkspace(workspaceId)
    } catch (err) {
      console.error('[TabBar] Failed to delete workspace:', err)
    }
  }

  const startRename = (workspaceId: number, currentName: string) => {
    if (isGuestScoped) {
      return
    }
    setEditingWorkspaceId(workspaceId)
    setDraftName(currentName)
    setContextMenu(null)
  }

  const cancelRename = () => {
    setEditingWorkspaceId(null)
    setDraftName('')
  }

  const commitRename = async () => {
    if (editingWorkspaceId == null) {
      return
    }

    const nextName = draftName.trim()
    if (!nextName) {
      cancelRename()
      return
    }

    try {
      await renameWorkspace(editingWorkspaceId, nextName)
    } catch (err) {
      console.error('[TabBar] Failed to rename workspace:', err)
    } finally {
      cancelRename()
    }
  }

  const handleRenameSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    await commitRename()
  }

  const handleRenameKeyDown = async (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      cancelRename()
      return
    }

    if (event.key === 'Enter') {
      event.preventDefault()
      await commitRename()
    }
  }

  const handleContextMenu = (event: React.MouseEvent, workspace: any) => {
    if (isGuestScoped) {
      return
    }
    event.preventDefault()
    setContextMenu({
      x: event.clientX,
      y: event.clientY,
      workspaceId: workspace.id,
      workspaceName: workspace.name,
      workspaceColor: workspace.color || '',
    })
  }

  const closeContextMenu = useCallback(() => {
    setContextMenu(null)
  }, [])

  useEffect(() => {
    if (contextMenu) {
      window.addEventListener('click', closeContextMenu)
      return () => window.removeEventListener('click', closeContextMenu)
    }
  }, [contextMenu, closeContextMenu])

  useEffect(() => {
    const clearDropState = () => {
      setDropWorkspaceId(null)
      clearTerminalWorkspaceDragPayload()
    }
    window.addEventListener('dragend', clearDropState)
    window.addEventListener('drop', clearDropState)

    return () => {
      window.removeEventListener('dragend', clearDropState)
      window.removeEventListener('drop', clearDropState)
    }
  }, [])

  const handleColorSelect = async (workspaceId: number, color: string) => {
    if (isGuestScoped) {
      return
    }
    try {
      await setWorkspaceColor(workspaceId, color)
    } catch (err) {
      console.error('[TabBar] Failed to set workspace color:', err)
    } finally {
      closeContextMenu()
    }
  }

  const activateWorkspaceTab = (workspaceId: number) => {
    if (isGuestScoped) {
      return
    }
    if (suppressNextTabClickRef.current) {
      suppressNextTabClickRef.current = false
      return
    }

    setActiveWorkspace(workspaceId).catch((err) => {
      console.error('[TabBar] Failed to switch workspace:', err)
    })
  }

  const handleTabDragOver = (event: DragEvent<HTMLDivElement>, workspaceId: number) => {
    if (isGuestScoped) {
      return
    }
    if (!hasTerminalWorkspaceDragPayload(event)) {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    event.dataTransfer.dropEffect = 'move'
    setDropWorkspaceId(workspaceId)
  }

  const handleTabDragLeave = (event: DragEvent<HTMLDivElement>, workspaceId: number) => {
    if (isGuestScoped) {
      return
    }
    event.preventDefault()
    event.stopPropagation()
    
    const nextTarget = event.relatedTarget
    if (nextTarget instanceof Node && event.currentTarget.contains(nextTarget)) {
      return
    }
    setDropWorkspaceId((current) => (current === workspaceId ? null : current))
  }

  const handleTabDrop = async (event: DragEvent<HTMLDivElement>, workspaceId: number) => {
    if (isGuestScoped) {
      return
    }
    if (!hasTerminalWorkspaceDragPayload(event)) {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    suppressNextTabClickRef.current = true
    setDropWorkspaceId(null)

    const payload = readTerminalWorkspaceDragPayload(event)
    if (!payload || payload.paneType !== 'terminal') {
      return
    }

    if (payload.sourceWorkspaceId && payload.sourceWorkspaceId === workspaceId) {
      return
    }

    try {
      await moveTerminalToWorkspace(payload.agentId, workspaceId)
    } catch (err) {
      console.error('[TabBar] Failed to move terminal across workspace:', err)
    } finally {
      clearTerminalWorkspaceDragPayload()
      // Garantir que o flag seja resetado após um tempo se o click não disparar
      setTimeout(() => {
        suppressNextTabClickRef.current = false
      }, 100)
    }
  }

  const activeWorkspace = workspaces.find((workspace) => workspace.id === activeWorkspaceId)
  const activeWorkspaceColor =
    activeWorkspace?.color && activeWorkspace.color.trim().length > 0
      ? activeWorkspace.color
      : 'var(--accent)'
  const tabbarStyle = {
    '--active-workspace-border-color': activeWorkspaceColor,
  } as CSSProperties

  return (
    <div className="tabbar" id="workspace-tabbar" style={tabbarStyle}>
      <div className="tabbar__list">
        {workspaces.map((workspace) => {
          const isActive = workspace.id === activeWorkspaceId
          const isEditing = workspace.id === editingWorkspaceId
          const terminalCount = workspace.agents.reduce((count, agent) => {
            const normalizedType = (agent.type || '').trim().toLowerCase()
            if (!normalizedType || normalizedType === 'terminal') {
              return count + 1
            }
            return count
          }, 0)
          const terminalCountLabel = terminalCount > 99 ? '99+' : `${terminalCount}`
          const tabStyle: CSSProperties = {}
          const workspaceColor = workspace.color?.trim()
          const activeBorderColor = workspaceColor && workspaceColor.length > 0
            ? workspaceColor
            : 'var(--accent)'
          const showSessionAvatars =
            scopedWorkspaceID != null &&
            workspace.id === scopedWorkspaceID &&
            sharedWorkspaceParticipants.length > 0
          const visibleParticipants = showSessionAvatars
            ? sharedWorkspaceParticipants.slice(0, MAX_VISIBLE_COLLAB_AVATARS)
            : []
          const hiddenParticipants = showSessionAvatars
            ? Math.max(0, sharedWorkspaceParticipants.length - visibleParticipants.length)
            : 0

          if (workspaceColor) {
            tabStyle.borderTopColor = workspaceColor
          }

          if (isActive) {
            tabStyle.borderTopColor = activeBorderColor
            tabStyle.borderLeftColor = activeBorderColor
            tabStyle.borderRightColor = activeBorderColor
          }

          return (
            <div
              key={workspace.id}
              role="tab"
              tabIndex={0}
              className={`tabbar__tab ${isActive ? 'tabbar__tab--active' : ''} ${dropWorkspaceId === workspace.id ? 'tabbar__tab--drop-target' : ''}`}
              style={tabStyle}
              onClick={() => activateWorkspaceTab(workspace.id)}
              onDragOver={(event) => handleTabDragOver(event, workspace.id)}
              onDragEnter={(event) => handleTabDragOver(event, workspace.id)}
              onDragLeave={(event) => handleTabDragLeave(event, workspace.id)}
              onDrop={(event) => {
                handleTabDrop(event, workspace.id).catch((err) => {
                  console.error('[TabBar] Failed handling workspace drop:', err)
                })
              }}
              onContextMenu={(e) => handleContextMenu(e, workspace)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault()
                  activateWorkspaceTab(workspace.id)
                }
              }}
              onDoubleClick={() => startRename(workspace.id, workspace.name)}
              title={workspace.name}
            >
              {isEditing ? (
                <form className="tabbar__rename" onSubmit={handleRenameSubmit}>
                  <input
                    value={draftName}
                    autoFocus
                    onChange={(event) => setDraftName(event.target.value)}
                    onBlur={() => {
                      commitRename().catch((err) => {
                        console.error('[TabBar] Failed to rename workspace:', err)
                      })
                    }}
                    onKeyDown={handleRenameKeyDown}
                    maxLength={42}
                  />
                </form>
              ) : (
                <>
                  <span className="tabbar__name">{workspace.name}</span>
                  <span
                    className={`tabbar__terminal-count ${terminalCount === 0 ? 'tabbar__terminal-count--empty' : ''}`}
                    aria-label={`${terminalCount} terminais abertos`}
                    title={`${terminalCount} terminais abertos`}
                  >
                    {terminalCountLabel}
                  </span>
                  {showSessionAvatars && (
                    <div className="tabbar__collab-avatars" aria-label={`${sharedWorkspaceParticipants.length} collaborators in this workspace`}>
                      {visibleParticipants.map((participant) => (
                        <span key={participant.userID} className="tabbar__avatar-chip" title={participant.name}>
                          {participant.avatarUrl ? (
                            <img
                              src={participant.avatarUrl}
                              alt={participant.name}
                              className="tabbar__avatar"
                              referrerPolicy="no-referrer"
                            />
                          ) : (
                            <span className="tabbar__avatar tabbar__avatar--fallback">
                              {participant.name.charAt(0).toUpperCase()}
                            </span>
                          )}
                        </span>
                      ))}
                      {hiddenParticipants > 0 && (
                        <span className="tabbar__avatar-overflow" title={`${hiddenParticipants} more collaborators`}>
                          +{hiddenParticipants}
                        </span>
                      )}
                    </div>
                  )}
                </>
              )}
            </div>
          )
        })}
      </div>

      {!isGuestScoped && (
        <button
          type="button"
          className="tabbar__add"
          title="Novo workspace"
          onClick={() => {
            handleCreateWorkspace().catch((err) => {
              console.error('[TabBar] Failed to create workspace:', err)
            })
          }}
        >
          <Plus size={14} />
        </button>
      )}

      {contextMenu && !isGuestScoped && (
        <div
          className="tab-context-menu"
          style={{ top: contextMenu.y, left: contextMenu.x }}
          onClick={(e) => e.stopPropagation()}
        >
          <div
            className="tab-context-menu__item"
            onClick={() => startRename(contextMenu.workspaceId, contextMenu.workspaceName)}
          >
            <Pencil size={14} />
            <span>Renomear</span>
          </div>
          <div className="tab-context-menu__separator" />
          <div className="tab-context-menu__item" style={{ cursor: 'default' }}>
            <Palette size={14} />
            <span>Trocar Cor</span>
          </div>
          <div className="tab-context-menu__colors">
            {TAB_COLORS.map((color) => (
              <button
                key={color.name}
                className={`tab-context-menu__color-btn ${
                  contextMenu.workspaceColor === color.value ? 'tab-context-menu__color-btn--active' : ''
                }`}
                style={{ backgroundColor: color.value || 'var(--accent)' }}
                title={color.name}
                onClick={() => handleColorSelect(contextMenu.workspaceId, color.value)}
              />
            ))}
          </div>
          {canDeleteWorkspace && (
            <>
              <div className="tab-context-menu__separator" />
              <div
                className="tab-context-menu__item"
                style={{ color: 'var(--error)' }}
                onClick={() => {
                  handleDeleteWorkspace(contextMenu.workspaceId)
                  closeContextMenu()
                }}
              >
                <X size={14} />
                <span>Deletar</span>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}
