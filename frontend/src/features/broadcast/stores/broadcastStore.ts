import { create } from 'zustand'
import { useLayoutStore } from '../../command-center/stores/layoutStore'
import type { PaneInfo, PaneStatus } from '../../command-center/types/layout'

const MAX_HISTORY_ITEMS = 20
const HIGHLIGHT_DURATION_MS = 500

export type TargetFilter = 'all' | 'running' | 'idle' | 'custom'

export interface BroadcastTarget {
  id: string
  status: PaneStatus
  sessionID?: string
}

interface BroadcastState {
  isActive: boolean
  targetFilter: TargetFilter
  targetAgentIDs: string[]
  history: string[]
  highlightedPaneIDs: Record<string, number>
}

interface BroadcastActions {
  activate: () => void
  deactivate: () => void
  toggle: () => void
  setTargetFilter: (filter: TargetFilter) => void
  setTargets: (ids: string[]) => void
  send: (message: string) => Promise<BroadcastSendResult>
  markHighlighted: (paneIDs: string[]) => void
}

export interface BroadcastSendResult {
  sent: number
  targeted: number
  skipped: number
  failed: number
}

const highlightTimers = new Map<string, ReturnType<typeof setTimeout>>()

function getTerminalTargets(panes: Record<string, PaneInfo>): BroadcastTarget[] {
  return Object.values(panes)
    .filter((pane) => pane.type === 'terminal')
    .map((pane) => ({
      id: pane.id,
      status: pane.status,
      sessionID: pane.sessionID,
    }))
}

export function resolveTargetPaneIDs(
  targets: BroadcastTarget[],
  filter: TargetFilter,
  customIDs: string[]
): string[] {
  switch (filter) {
    case 'all':
      return targets.map((target) => target.id)
    case 'running':
      return targets
        .filter((target) => target.status === 'running')
        .map((target) => target.id)
    case 'idle':
      return targets
        .filter((target) => target.status === 'idle')
        .map((target) => target.id)
    case 'custom': {
      const selected = new Set(customIDs)
      return targets
        .filter((target) => selected.has(target.id))
        .map((target) => target.id)
    }
    default:
      return []
  }
}

function appendHistory(history: string[], message: string): string[] {
  return [...history, message].slice(-MAX_HISTORY_ITEMS)
}

function normalizeBroadcastPayload(message: string): string {
  const normalized = message.trim().toLowerCase()
  if (normalized === 'ctrl+c' || normalized === '^c' || normalized === 'sigint') {
    return '\u0003'
  }
  return message
}

async function getLocalAliveSessionIDs(): Promise<Set<string> | null> {
  const app = window.go?.main?.App
  if (!app?.GetTerminals) {
    return null
  }

  try {
    const sessions = await app.GetTerminals()
    const aliveIDs = new Set<string>()

    for (const rawSession of sessions as Array<{ id?: string; isAlive?: boolean }>) {
      if (rawSession.id && rawSession.isAlive) {
        aliveIDs.add(rawSession.id)
      }
    }

    return aliveIDs
  } catch {
    return null
  }
}

async function broadcastSend(
  message: string,
  targetPaneIDs: string[],
  panes: Record<string, PaneInfo>
): Promise<BroadcastSendResult & { highlightedPaneIDs: string[] }> {
  const localAliveSessions = await getLocalAliveSessionIDs()
  const payload = normalizeBroadcastPayload(message)
  const messageToWrite = payload === '\u0003' ? payload : `${payload}\n`

  let sent = 0
  let skipped = 0
  let failed = 0
  const highlightedPaneIDs: string[] = []

  await Promise.all(
    targetPaneIDs.map(async (paneID) => {
      const pane = panes[paneID]
      if (!pane?.sessionID) {
        skipped += 1
        return
      }

      // Segurança: só envia para sessões PTY locais/alive.
      if (localAliveSessions && !localAliveSessions.has(pane.sessionID)) {
        skipped += 1
        return
      }

      const app = window.go?.main?.App
      if (!app?.WriteTerminal) {
        skipped += 1
        return
      }

      try {
        await app.WriteTerminal(pane.sessionID, messageToWrite)
        sent += 1
        highlightedPaneIDs.push(paneID)
      } catch {
        failed += 1
      }
    })
  )

  return {
    sent,
    targeted: targetPaneIDs.length,
    skipped,
    failed,
    highlightedPaneIDs,
  }
}

export const useBroadcastStore = create<BroadcastState & BroadcastActions>((set, get) => ({
  isActive: false,
  targetFilter: 'all',
  targetAgentIDs: [],
  history: [],
  highlightedPaneIDs: {},

  activate: () => set({ isActive: true }),

  deactivate: () => set({ isActive: false }),

  toggle: () => set((state) => ({ isActive: !state.isActive })),

  setTargetFilter: (filter) => set({ targetFilter: filter }),

  setTargets: (ids) => set({ targetAgentIDs: ids }),

  send: async (message) => {
    const trimmed = message.trim()
    if (!trimmed) {
      return { sent: 0, targeted: 0, skipped: 0, failed: 0 }
    }

    set((state) => ({ history: appendHistory(state.history, trimmed) }))

    const layoutState = useLayoutStore.getState()
    const targets = getTerminalTargets(layoutState.panes)
    const targetPaneIDs = resolveTargetPaneIDs(targets, get().targetFilter, get().targetAgentIDs)

    if (targetPaneIDs.length === 0) {
      return { sent: 0, targeted: 0, skipped: 0, failed: 0 }
    }

    const result = await broadcastSend(trimmed, targetPaneIDs, layoutState.panes)

    if (result.highlightedPaneIDs.length > 0) {
      get().markHighlighted(result.highlightedPaneIDs)
    }

    return {
      sent: result.sent,
      targeted: result.targeted,
      skipped: result.skipped,
      failed: result.failed,
    }
  },

  markHighlighted: (paneIDs) => {
    if (paneIDs.length === 0) return

    const now = Date.now()
    set((state) => {
      const next = { ...state.highlightedPaneIDs }
      for (const paneID of paneIDs) {
        next[paneID] = now
      }
      return { highlightedPaneIDs: next }
    })

    for (const paneID of paneIDs) {
      const existingTimer = highlightTimers.get(paneID)
      if (existingTimer) {
        clearTimeout(existingTimer)
      }

      const timer = setTimeout(() => {
        set((state) => {
          if (!state.highlightedPaneIDs[paneID]) {
            return state
          }

          const next = { ...state.highlightedPaneIDs }
          delete next[paneID]
          return { highlightedPaneIDs: next }
        })
      }, HIGHLIGHT_DURATION_MS)

      highlightTimers.set(paneID, timer)
    }
  },
}))
