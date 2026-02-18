import { FormEvent, KeyboardEvent, DragEvent, CSSProperties, useState, useEffect, useCallback, useRef } from 'react'
import { Plus, X, Pencil, Palette } from 'lucide-react'
import { useWorkspaceStore } from '../stores/workspaceStore'
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

export function TabBar() {
  const workspaces = useWorkspaceStore((s) => s.workspaces)
  const activeWorkspaceId = useWorkspaceStore((s) => s.activeWorkspaceId)
  const setActiveWorkspace = useWorkspaceStore((s) => s.setActiveWorkspace)
  const createWorkspace = useWorkspaceStore((s) => s.createWorkspace)
  const renameWorkspace = useWorkspaceStore((s) => s.renameWorkspace)
  const setWorkspaceColor = useWorkspaceStore((s) => s.setWorkspaceColor)
  const deleteWorkspace = useWorkspaceStore((s) => s.deleteWorkspace)
  const moveTerminalToWorkspace = useWorkspaceStore((s) => s.moveTerminalToWorkspace)

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

  const canDeleteWorkspace = workspaces.length > 1

  const handleCreateWorkspace = async () => {
    try {
      await createWorkspace()
    } catch (err) {
      console.error('[TabBar] Failed to create workspace:', err)
    }
  }

  const handleDeleteWorkspace = async (workspaceId: number) => {
    try {
      await deleteWorkspace(workspaceId)
    } catch (err) {
      console.error('[TabBar] Failed to delete workspace:', err)
    }
  }

  const startRename = (workspaceId: number, currentName: string) => {
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
    try {
      await setWorkspaceColor(workspaceId, color)
    } catch (err) {
      console.error('[TabBar] Failed to set workspace color:', err)
    } finally {
      closeContextMenu()
    }
  }

  const activateWorkspaceTab = (workspaceId: number) => {
    if (suppressNextTabClickRef.current) {
      suppressNextTabClickRef.current = false
      return
    }

    setActiveWorkspace(workspaceId).catch((err) => {
      console.error('[TabBar] Failed to switch workspace:', err)
    })
  }

  const handleTabDragOver = (event: DragEvent<HTMLDivElement>, workspaceId: number) => {
    if (!hasTerminalWorkspaceDragPayload(event)) {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    event.dataTransfer.dropEffect = 'move'
    setDropWorkspaceId(workspaceId)
  }

  const handleTabDragLeave = (event: DragEvent<HTMLDivElement>, workspaceId: number) => {
    event.preventDefault()
    event.stopPropagation()
    
    const nextTarget = event.relatedTarget
    if (nextTarget instanceof Node && event.currentTarget.contains(nextTarget)) {
      return
    }
    setDropWorkspaceId((current) => (current === workspaceId ? null : current))
  }

  const handleTabDrop = async (event: DragEvent<HTMLDivElement>, workspaceId: number) => {
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
          const tabStyle: CSSProperties = {}
          const workspaceColor = workspace.color?.trim()
          const activeBorderColor = workspaceColor && workspaceColor.length > 0
            ? workspaceColor
            : 'var(--accent)'

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
                  {canDeleteWorkspace && (
                    <span
                      className="tabbar__delete"
                      role="button"
                      tabIndex={0}
                      title="Deletar workspace"
                      onClick={(event) => {
                        event.stopPropagation()
                        handleDeleteWorkspace(workspace.id).catch((err) => {
                          console.error('[TabBar] Failed to delete workspace:', err)
                        })
                      }}
                      onKeyDown={(event) => {
                        if (event.key === 'Enter' || event.key === ' ') {
                          event.preventDefault()
                          event.stopPropagation()
                          handleDeleteWorkspace(workspace.id).catch((err) => {
                            console.error('[TabBar] Failed to delete workspace:', err)
                          })
                        }
                      }}
                    >
                      <X size={12} />
                    </span>
                  )}
                </>
              )}
            </div>
          )
        })}
      </div>

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

      {contextMenu && (
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
