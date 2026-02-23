import type { DragEvent } from 'react'
import type { PaneType } from '../types/layout'

export const TERMINAL_WORKSPACE_DND_MIME = 'application/x-orch-terminal-workspace'

export interface TerminalWorkspaceDragPayload {
  paneId: string
  agentId: number
  sourceWorkspaceId?: number
  paneType?: PaneType
}

let activeTerminalWorkspaceDragPayload: TerminalWorkspaceDragPayload | null = null

export const writeTerminalWorkspaceDragPayload = (
  event: DragEvent<HTMLElement>,
  payload: TerminalWorkspaceDragPayload,
) => {
  activeTerminalWorkspaceDragPayload = payload

  if (!event.dataTransfer) {
    return
  }

  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData(TERMINAL_WORKSPACE_DND_MIME, JSON.stringify(payload))
  event.dataTransfer.setData('text/plain', String(payload.agentId))
}

export const clearTerminalWorkspaceDragPayload = () => {
  activeTerminalWorkspaceDragPayload = null
}

export const getActiveTerminalWorkspaceDragPayload = (): TerminalWorkspaceDragPayload | null => {
  return activeTerminalWorkspaceDragPayload
}

export const hasTerminalWorkspaceDragPayload = (
  event: DragEvent<HTMLElement>,
): boolean => {
  if (activeTerminalWorkspaceDragPayload) {
    return true
  }

  if (!event.dataTransfer) {
    return false
  }

  const hasCustomMime = Array.from(event.dataTransfer.types).includes(TERMINAL_WORKSPACE_DND_MIME)
  if (hasCustomMime) {
    return true
  }

  const plain = event.dataTransfer.getData('text/plain')
  if (!plain) {
    return false
  }

  const parsedID = Number(plain)
  return Number.isInteger(parsedID) && parsedID > 0
}

export const readTerminalWorkspaceDragPayload = (
  event: DragEvent<HTMLElement>,
): TerminalWorkspaceDragPayload | null => {
  if (event.dataTransfer) {
    const raw = event.dataTransfer.getData(TERMINAL_WORKSPACE_DND_MIME)
    if (raw) {
      try {
        const parsed = JSON.parse(raw) as Partial<TerminalWorkspaceDragPayload>
        const paneId = typeof parsed.paneId === 'string' ? parsed.paneId : ''
        const agentId = Number(parsed.agentId)
        const sourceWorkspaceId = Number(parsed.sourceWorkspaceId)
        const paneType = parsed.paneType

        const isPaneTypeValid =
          paneType !== undefined &&
          paneType !== 'terminal' &&
          paneType !== 'ai_agent'

        if (paneId && Number.isInteger(agentId) && agentId > 0 && !isPaneTypeValid) {
          return {
            paneId,
            agentId,
            sourceWorkspaceId:
              Number.isInteger(sourceWorkspaceId) && sourceWorkspaceId > 0
                ? sourceWorkspaceId
                : undefined,
            paneType: paneType ?? 'terminal',
          }
        }
      } catch {
        // segue para fallback em memória/texto
      }
    }
  }

  if (activeTerminalWorkspaceDragPayload) {
    return activeTerminalWorkspaceDragPayload
  }

  if (event.dataTransfer) {
    const plain = event.dataTransfer.getData('text/plain')
    const parsedID = Number(plain)
    if (Number.isInteger(parsedID) && parsedID > 0) {
      return {
        paneId: '',
        agentId: parsedID,
        paneType: 'terminal',
      }
    }
  }

  return null
}
