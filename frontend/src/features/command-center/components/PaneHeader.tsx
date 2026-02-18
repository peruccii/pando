import { useCallback, useState, type DragEvent } from 'react'
import { Search, RotateCw, Trash2, Maximize2, Minimize2, GripVertical } from 'lucide-react'
import { useLayoutStore } from '../stores/layoutStore'
import { useZenMode } from '../hooks/useZenMode'
import { useBroadcastStore } from '../../broadcast/stores/broadcastStore'
import { useWorkspaceStore } from '../../../stores/workspaceStore'
import {
  clearTerminalWorkspaceDragPayload,
  hasTerminalWorkspaceDragPayload,
  readTerminalWorkspaceDragPayload,
  writeTerminalWorkspaceDragPayload,
} from '../utils/terminalWorkspaceDnd'
import type { PaneStatus, PaneType } from '../types/layout'
import './PaneHeader.css'

interface PaneHeaderProps {
  paneId: string
  title: string
  status: PaneStatus
  type: PaneType
  isActive: boolean
}

/** Ícone do indicador de status */
function StatusIndicator({ status }: { status: PaneStatus }) {
  return (
    <span
      className={`status-dot status-dot--${status}`}
      title={
        status === 'idle' ? 'Pronto' :
          status === 'running' ? 'Executando' :
            'Erro'
      }
    />
  )
}

/**
 * PaneHeader — header de 28px com nome, status e controles rápidos.
 * Exibido no topo de cada painel do mosaic.
 */
export function PaneHeader({ paneId, title, status, type, isActive }: PaneHeaderProps) {
  const closePane = useWorkspaceStore((s) => s.closePane)
  const pane = useLayoutStore((s) => s.panes[paneId])
  const updatePaneStatus = useLayoutStore((s) => s.updatePaneStatus)
  const swapPanePositions = useLayoutStore((s) => s.swapPanePositions)
  const setActivePaneId = useLayoutStore((s) => s.setActivePaneId)
  const { toggleZenMode, isPaneInZenMode } = useZenMode()
  const isBroadcastHighlighted = useBroadcastStore((s) => Boolean(s.highlightedPaneIDs[paneId]))
  const [isSwapDropTarget, setIsSwapDropTarget] = useState(false)

  const isZen = isPaneInZenMode(paneId)
  const canDragToWorkspace = Boolean(
    pane &&
    pane.type === 'terminal' &&
    pane.agentDBID &&
    pane.workspaceID,
  )

  const handleKill = useCallback((e: React.MouseEvent) => {
    e.stopPropagation()
    closePane(paneId).catch((err) => {
      console.error('[PaneHeader] Failed to close pane:', err)
    })
  }, [closePane, paneId])

  const handleRestart = useCallback((e: React.MouseEvent) => {
    e.stopPropagation()
    updatePaneStatus(paneId, 'idle')
    // TODO: Reiniciar o processo PTY
  }, [paneId, updatePaneStatus])

  const handleZen = useCallback((e: React.MouseEvent) => {
    e.stopPropagation()
    toggleZenMode(paneId)
  }, [paneId, toggleZenMode])

  const handleDoubleClick = useCallback(() => {
    toggleZenMode(paneId)
  }, [paneId, toggleZenMode])

  const handleClick = useCallback(() => {
    setActivePaneId(paneId)
  }, [paneId, setActivePaneId])

  const handleDragStart = useCallback((event: DragEvent<HTMLElement>) => {
    if (!pane || !pane.agentDBID || !pane.workspaceID || pane.type !== 'terminal') {
      event.preventDefault()
      return
    }

    // Impede que o Mosaic intercepte este drag
    event.stopPropagation()

    writeTerminalWorkspaceDragPayload(event, {
      paneId,
      agentId: pane.agentDBID,
      sourceWorkspaceId: pane.workspaceID,
      paneType: pane.type,
    })

    // Feedback visual do drag
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move'
      // Criar uma imagem vazia ou transparente se quisermos esconder o ghost default,
      // mas por enquanto vamos deixar o default do browser.
    }
  }, [pane, paneId])

  const handleDragEnd = useCallback(() => {
    clearTerminalWorkspaceDragPayload()
  }, [])

  const canSwapWithPayload = useCallback((payloadPaneId: string, sourceWorkspaceId?: number) => {
    if (!pane || pane.type !== 'terminal' || !pane.workspaceID) {
      return false
    }
    if (payloadPaneId === paneId) {
      return false
    }
    if (!sourceWorkspaceId) {
      return false
    }
    return sourceWorkspaceId === pane.workspaceID
  }, [pane, paneId])

  const handleSwapDragOver = useCallback((event: DragEvent<HTMLDivElement>) => {
    if (!hasTerminalWorkspaceDragPayload(event)) {
      return
    }

    const payload = readTerminalWorkspaceDragPayload(event)
    if (!payload || !canSwapWithPayload(payload.paneId, payload.sourceWorkspaceId)) {
      setIsSwapDropTarget(false)
      return
    }

    event.preventDefault()
    event.stopPropagation()
    event.dataTransfer.dropEffect = 'move'
    setIsSwapDropTarget(true)
  }, [canSwapWithPayload])

  const handleSwapDragLeave = useCallback((event: DragEvent<HTMLDivElement>) => {
    if (!hasTerminalWorkspaceDragPayload(event)) {
      return
    }
    const related = event.relatedTarget
    if (related instanceof Node && event.currentTarget.contains(related)) {
      return
    }
    setIsSwapDropTarget(false)
  }, [])

  const handleSwapDrop = useCallback((event: DragEvent<HTMLDivElement>) => {
    if (!hasTerminalWorkspaceDragPayload(event)) {
      return
    }

    setIsSwapDropTarget(false)

    const payload = readTerminalWorkspaceDragPayload(event)
    if (!payload || !canSwapWithPayload(payload.paneId, payload.sourceWorkspaceId)) {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    swapPanePositions(payload.paneId, paneId)
    setActivePaneId(payload.paneId)
  }, [canSwapWithPayload, paneId, setActivePaneId, swapPanePositions])

  /** Ícone do tipo de painel */
  const typeIcon = type === 'terminal' ? '⌘' : type === 'ai_agent' ? '🤖' : '🐙'

  return (
    <div
      className={`pane-header ${isActive ? 'pane-header--active' : ''} ${isBroadcastHighlighted ? 'pane-header--broadcast' : ''} ${isSwapDropTarget ? 'pane-header--drop-target' : ''}`}
      onDoubleClick={handleDoubleClick}
      onClick={handleClick}
      onDragOver={handleSwapDragOver}
      onDragLeave={handleSwapDragLeave}
      onDrop={handleSwapDrop}
    >
      <div className="pane-header__name">
        {canDragToWorkspace && (
          <span
            className="pane-header__drag-handle"
            draggable
            onMouseDown={(event) => event.stopPropagation()}
            onDragStart={handleDragStart}
            onDragEnd={handleDragEnd}
            title="Arraste para outra workspace"
            role="button"
            aria-label={`Arrastar ${title} para outra workspace`}
          >
            <GripVertical size={13} />
          </span>
        )}
        <StatusIndicator status={status} />
        <span className="pane-header__type-icon">{typeIcon}</span>
        {isBroadcastHighlighted && (
          <span className="pane-header__broadcast-badge" aria-label="Broadcast recebido">⚡</span>
        )}
        <span className="pane-header__title">{title}</span>
      </div>

      <div className="pane-header__controls">
        <button
          className="pane-header__btn"
          onClick={(e) => { e.stopPropagation() }}
          title="Buscar no terminal (Cmd+F)"
          aria-label={`Buscar no terminal ${title}`}
        >
          <Search size={13} />
        </button>

        <button
          className="pane-header__btn"
          onClick={handleRestart}
          title="Reiniciar"
          aria-label={`Reiniciar ${title}`}
        >
          <RotateCw size={13} />
        </button>

        <button
          className="pane-header__btn pane-header__btn--danger"
          onClick={handleKill}
          title="Fechar (Cmd+W)"
          aria-label={`Fechar ${title} (Cmd+W)`}
        >
          <Trash2 size={13} />
        </button>

        <button
          className="pane-header__btn"
          onClick={handleZen}
          title={isZen ? 'Restaurar (Cmd+Enter)' : 'Maximizar (Cmd+Enter)'}
          aria-label={isZen ? `Restaurar ${title}` : `Maximizar ${title}`}
        >
          {isZen ? <Minimize2 size={13} /> : <Maximize2 size={13} />}
        </button>
      </div>
    </div>
  )
}
