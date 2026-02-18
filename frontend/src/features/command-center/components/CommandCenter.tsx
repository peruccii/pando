import { useCallback, useEffect, useState, type DragEvent } from 'react'
import { Mosaic, MosaicWindow, type MosaicBranch, type MosaicNode } from 'react-mosaic-component'
import 'react-mosaic-component/react-mosaic-component.css'
import { useLayoutStore } from '../stores/layoutStore'
import { PaneHeader } from './PaneHeader'
import { TerminalPane } from './TerminalPane'
import { AIAgentPane } from './AIAgentPane'
import { GitHubPane } from './GitHubPane'
import { ZenModeOverlay } from './ZenModeOverlay'
import {
  hasTerminalWorkspaceDragPayload,
  readTerminalWorkspaceDragPayload,
  writeTerminalWorkspaceDragPayload,
} from '../utils/terminalWorkspaceDnd'
import type { PaneDropPosition } from '../types/layout'
import './CommandCenter.css'

const DROP_ZONE_ORDER: PaneDropPosition[] = ['top', 'left', 'center', 'right', 'bottom']

/**
 * CommandCenter — container principal do grid de mosaico.
 * Usa react-mosaic-component para layout dinâmico de painéis.
 */
export function CommandCenter() {
  const panes = useLayoutStore((s) => s.panes)
  const mosaicNode = useLayoutStore((s) => s.mosaicNode)
  const setMosaicNode = useLayoutStore((s) => s.setMosaicNode)
  const movePaneToPosition = useLayoutStore((s) => s.movePaneToPosition)
  const activePaneId = useLayoutStore((s) => s.activePaneId)
  const setActivePaneId = useLayoutStore((s) => s.setActivePaneId)
  const zenModePane = useLayoutStore((s) => s.zenModePane)
  const [dropOverlayPaneId, setDropOverlayPaneId] = useState<string | null>(null)
  const [dropOverlayPosition, setDropOverlayPosition] = useState<PaneDropPosition>('center')

  useEffect(() => {
    const clearDropOverlay = () => {
      setDropOverlayPaneId(null)
      setDropOverlayPosition('center')
    }

    window.addEventListener('dragend', clearDropOverlay)
    window.addEventListener('drop', clearDropOverlay)
    return () => {
      window.removeEventListener('dragend', clearDropOverlay)
      window.removeEventListener('drop', clearDropOverlay)
    }
  }, [])

  const handlePaneDragStart = useCallback((event: DragEvent<HTMLDivElement>, paneId: string) => {
    const pane = panes[paneId]
    if (!pane || pane.type !== 'terminal' || !pane.agentDBID || !pane.workspaceID) {
      return
    }

    writeTerminalWorkspaceDragPayload(event, {
      paneId,
      agentId: pane.agentDBID,
      sourceWorkspaceId: pane.workspaceID,
      paneType: pane.type,
    })
  }, [panes])

  const canRepositionTerminals = useCallback((sourcePaneId: string, targetPaneId: string) => {
    const source = panes[sourcePaneId]
    const target = panes[targetPaneId]
    if (!source || !target) {
      return false
    }
    if (sourcePaneId === targetPaneId) {
      return false
    }
    if (source.type !== 'terminal' || target.type !== 'terminal') {
      return false
    }
    if (!source.workspaceID || !target.workspaceID) {
      return false
    }
    return source.workspaceID === target.workspaceID
  }, [panes])

  const handlePaneContentDragEnter = useCallback((event: DragEvent<HTMLDivElement>, targetPaneId: string) => {
    if (!hasTerminalWorkspaceDragPayload(event)) {
      return
    }

    const payload = readTerminalWorkspaceDragPayload(event)
    if (!payload || !canRepositionTerminals(payload.paneId, targetPaneId)) {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    event.dataTransfer.dropEffect = 'move'
    setDropOverlayPaneId(targetPaneId)
    setDropOverlayPosition('center')
  }, [canRepositionTerminals])

  const handlePaneContentDragOver = useCallback((event: DragEvent<HTMLDivElement>, targetPaneId: string) => {
    if (!hasTerminalWorkspaceDragPayload(event)) {
      return
    }

    const payload = readTerminalWorkspaceDragPayload(event)
    if (!payload || !canRepositionTerminals(payload.paneId, targetPaneId)) {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    event.dataTransfer.dropEffect = 'move'
    if (dropOverlayPaneId !== targetPaneId) {
      setDropOverlayPaneId(targetPaneId)
      setDropOverlayPosition('center')
    }
  }, [canRepositionTerminals, dropOverlayPaneId])

  const handlePaneContentDragLeave = useCallback((event: DragEvent<HTMLDivElement>, targetPaneId: string) => {
    if (dropOverlayPaneId !== targetPaneId) {
      return
    }
    const related = event.relatedTarget
    if (related instanceof Node && event.currentTarget.contains(related)) {
      return
    }
    setDropOverlayPaneId(null)
    setDropOverlayPosition('center')
  }, [dropOverlayPaneId])

  const handlePaneContentDrop = useCallback((event: DragEvent<HTMLDivElement>, targetPaneId: string) => {
    if (!hasTerminalWorkspaceDragPayload(event)) {
      return
    }

    const payload = readTerminalWorkspaceDragPayload(event)
    if (!payload || !canRepositionTerminals(payload.paneId, targetPaneId)) {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    movePaneToPosition(payload.paneId, targetPaneId, dropOverlayPosition)
    setActivePaneId(payload.paneId)
    setDropOverlayPaneId(null)
    setDropOverlayPosition('center')
  }, [canRepositionTerminals, dropOverlayPosition, movePaneToPosition, setActivePaneId])

  const handleDropZoneDragOver = useCallback((
    event: DragEvent<HTMLDivElement>,
    targetPaneId: string,
    position: PaneDropPosition,
  ) => {
    if (!hasTerminalWorkspaceDragPayload(event)) {
      return
    }

    const payload = readTerminalWorkspaceDragPayload(event)
    if (!payload || !canRepositionTerminals(payload.paneId, targetPaneId)) {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    event.dataTransfer.dropEffect = 'move'
    setDropOverlayPaneId(targetPaneId)
    setDropOverlayPosition(position)
  }, [canRepositionTerminals])

  const handleDropZoneDrop = useCallback((
    event: DragEvent<HTMLDivElement>,
    targetPaneId: string,
    position: PaneDropPosition,
  ) => {
    if (!hasTerminalWorkspaceDragPayload(event)) {
      return
    }

    const payload = readTerminalWorkspaceDragPayload(event)
    if (!payload || !canRepositionTerminals(payload.paneId, targetPaneId)) {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    movePaneToPosition(payload.paneId, targetPaneId, position)
    setActivePaneId(payload.paneId)
    setDropOverlayPaneId(null)
    setDropOverlayPosition('center')
  }, [canRepositionTerminals, movePaneToPosition, setActivePaneId])

  /** Renderiza o conteúdo de cada painel baseado no tipo */
  const renderTile = useCallback(
    (id: string, path: MosaicBranch[]) => {
      const pane = panes[id]
      if (!pane) return <div className="pane-placeholder">Painel não encontrado</div>

      const isActive = activePaneId === id
      const isDropOverlayVisible = dropOverlayPaneId === id

      return (
        <MosaicWindow<string>
          path={path}
          title={pane.title}
          renderToolbar={() => (
            <div className="pane-header-drag-wrapper" onDragStart={(event) => handlePaneDragStart(event, id)}>
              <PaneHeader
                paneId={id}
                title={pane.title}
                status={pane.status}
                type={pane.type}
                isActive={isActive}
              />
            </div>
          )}
          className={`pane-window ${isActive ? 'pane-window--active' : 'pane-window--inactive'} ${isDropOverlayVisible ? 'pane-window--drop-ready' : ''}`}
        >
          <div
            className="pane-content"
            onMouseDown={() => setActivePaneId(id)}
            onClick={() => setActivePaneId(id)}
            onDragEnter={(event) => handlePaneContentDragEnter(event, id)}
            onDragOver={(event) => handlePaneContentDragOver(event, id)}
            onDragLeave={(event) => handlePaneContentDragLeave(event, id)}
            onDrop={(event) => handlePaneContentDrop(event, id)}
          >
            {pane.type === 'terminal' && (
              <TerminalPane paneId={id} isActive={isActive} />
            )}
            {pane.type === 'ai_agent' && (
              <AIAgentPane paneId={id} isActive={isActive} />
            )}
            {pane.type === 'github' && (
              <GitHubPane paneId={id} isActive={isActive} />
            )}

            {isDropOverlayVisible && (
              <div className="pane-drop-overlay" aria-hidden>
                {DROP_ZONE_ORDER.map((position) => (
                  <div
                    key={`${id}-${position}`}
                    className={`pane-drop-zone pane-drop-zone--${position} ${dropOverlayPosition === position ? 'pane-drop-zone--active' : ''}`}
                    onDragOver={(event) => handleDropZoneDragOver(event, id, position)}
                    onDrop={(event) => handleDropZoneDrop(event, id, position)}
                  />
                ))}
              </div>
            )}
          </div>
        </MosaicWindow>
      )
    },
    [
      panes,
      activePaneId,
      dropOverlayPaneId,
      dropOverlayPosition,
      handleDropZoneDragOver,
      handleDropZoneDrop,
      handlePaneContentDragEnter,
      handlePaneContentDragLeave,
      handlePaneContentDragOver,
      handlePaneContentDrop,
      handlePaneDragStart,
      setActivePaneId,
    ]
  )

  /** Handler de mudança no mosaic (resize, drag) */
  const handleMosaicChange = useCallback(
    (node: MosaicNode<string> | null) => {
      setMosaicNode(node)
    },
    [setMosaicNode]
  )

  if (!mosaicNode) {
    return null
  }

  return (
    <div className="command-center" id="command-center">
      <Mosaic<string>
        renderTile={renderTile}
        value={mosaicNode}
        onChange={handleMosaicChange}
        className="mosaic-theme-orch"
        resize={{ minimumPaneSizePercentage: 10 }}
      />

      {/* Zen Mode Overlay */}
      {zenModePane && (
        <ZenModeOverlay paneId={zenModePane} />
      )}
    </div>
  )
}
