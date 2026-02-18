import { create } from 'zustand'
import type { MosaicNode, MosaicDirection } from 'react-mosaic-component'
import type { PaneDropPosition, PaneInfo, PaneStatus, PaneType } from '../types/layout'

interface LayoutState {
  // Panes
  panes: Record<string, PaneInfo>
  paneOrder: string[]
  mosaicNode: MosaicNode<string> | null
  activePaneId: string | null

  // Zen Mode
  zenModePane: string | null
  zenModePreviousNode: MosaicNode<string> | null

  // Counter para IDs únicos
  paneCounter: number
}

interface LayoutActions {
  // Pane management
  addPane: (type: PaneType, title?: string, config?: Record<string, any>) => string
  replaceWithPanes: (panes: PaneInfo[]) => void
  removePane: (id: string) => void
  renamePane: (id: string, title: string) => void
  updatePaneStatus: (id: string, status: PaneStatus) => void
  setPaneSessionID: (id: string, sessionID: string) => void

  // Focus
  setActivePaneId: (id: string | null) => void
  focusNextPane: () => void
  focusPrevPane: () => void
  focusPaneByIndex: (index: number) => void

  // Mosaic
  setMosaicNode: (node: MosaicNode<string> | null) => void
  swapPanePositions: (sourcePaneId: string, targetPaneId: string) => void
  movePaneToPosition: (
    sourcePaneId: string,
    targetPaneId: string,
    position: PaneDropPosition,
  ) => void
  updateLayout: () => void

  // Zen Mode
  enterZenMode: (paneId: string) => void
  exitZenMode: () => void
  toggleZenMode: (paneId?: string) => void

  // Bulk
  reset: () => void

  // Persistence
  saveLayout: () => Promise<void>
  restoreLayout: () => Promise<void>
  getSerializedLayout: () => string
  loadSerializedLayout: (json: string) => void

  // Session Persistence
  captureTerminalSnapshots: () => TerminalSnapshotDTO[]
}

/** Calcula o MosaicNode automaticamente com base no número de painéis */
function calculateLayout(paneIds: string[]): MosaicNode<string> | null {
  const count = paneIds.length

  if (count === 0) return null
  if (count === 1) return paneIds[0]
  if (count === 2) {
    return {
      direction: 'row' as MosaicDirection,
      first: paneIds[0],
      second: paneIds[1],
      splitPercentage: 50,
    }
  }
  if (count === 3) {
    return {
      direction: 'row' as MosaicDirection,
      first: paneIds[0],
      second: {
        direction: 'column' as MosaicDirection,
        first: paneIds[1],
        second: paneIds[2],
        splitPercentage: 50,
      },
      splitPercentage: 50,
    }
  }

  // Para 4+ painéis, criar grid automático
  const cols = Math.ceil(Math.sqrt(count))

  // Dividir painéis em linhas
  const rows: string[][] = []
  for (let i = 0; i < count; i += cols) {
    rows.push(paneIds.slice(i, i + cols))
  }

  // Construir linhas como MosaicNodes
  function buildRow(ids: string[]): MosaicNode<string> {
    if (ids.length === 1) return ids[0]
    return {
      direction: 'row' as MosaicDirection,
      first: ids[0],
      second: buildRow(ids.slice(1)),
      splitPercentage: 100 / ids.length,
    }
  }

  function buildColumn(rowNodes: MosaicNode<string>[]): MosaicNode<string> {
    if (rowNodes.length === 1) return rowNodes[0]
    return {
      direction: 'column' as MosaicDirection,
      first: rowNodes[0],
      second: buildColumn(rowNodes.slice(1)),
      splitPercentage: 100 / rowNodes.length,
    }
  }

  const rowNodes = rows.map(row => buildRow(row))
  return buildColumn(rowNodes)
}

/** Serializar estado do layout para JSON persistível */
function serializeLayout(state: LayoutState): string {
  return JSON.stringify({
    panes: state.panes,
    paneOrder: state.paneOrder,
    mosaicNode: state.mosaicNode,
    activePaneId: state.activePaneId,
    paneCounter: state.paneCounter,
  })
}

function mosaicContainsPane(node: MosaicNode<string> | null, paneId: string): boolean {
  if (!node) return false
  if (typeof node === 'string') return node === paneId
  return mosaicContainsPane(node.first, paneId) || mosaicContainsPane(node.second, paneId)
}

function swapPaneIdsInMosaicNode(
  node: MosaicNode<string>,
  sourcePaneId: string,
  targetPaneId: string,
): MosaicNode<string> {
  if (typeof node === 'string') {
    if (node === sourcePaneId) return targetPaneId
    if (node === targetPaneId) return sourcePaneId
    return node
  }

  return {
    ...node,
    first: swapPaneIdsInMosaicNode(node.first, sourcePaneId, targetPaneId),
    second: swapPaneIdsInMosaicNode(node.second, sourcePaneId, targetPaneId),
  }
}

function removePaneFromMosaicNode(
  node: MosaicNode<string>,
  paneId: string,
): { node: MosaicNode<string> | null; removed: boolean } {
  if (typeof node === 'string') {
    if (node === paneId) {
      return { node: null, removed: true }
    }
    return { node, removed: false }
  }

  const removedFromFirst = removePaneFromMosaicNode(node.first, paneId)
  if (removedFromFirst.removed) {
    if (!removedFromFirst.node) {
      return { node: node.second, removed: true }
    }
    return {
      node: {
        ...node,
        first: removedFromFirst.node,
      },
      removed: true,
    }
  }

  const removedFromSecond = removePaneFromMosaicNode(node.second, paneId)
  if (removedFromSecond.removed) {
    if (!removedFromSecond.node) {
      return { node: node.first, removed: true }
    }
    return {
      node: {
        ...node,
        second: removedFromSecond.node,
      },
      removed: true,
    }
  }

  return { node, removed: false }
}

function insertPaneRelativeToTarget(
  node: MosaicNode<string>,
  targetPaneId: string,
  sourcePaneId: string,
  position: Exclude<PaneDropPosition, 'center'>,
): { node: MosaicNode<string>; inserted: boolean } {
  if (typeof node === 'string') {
    if (node !== targetPaneId) {
      return { node, inserted: false }
    }

    const direction: MosaicDirection = (position === 'left' || position === 'right')
      ? 'row'
      : 'column'
    const sourceFirst = position === 'left' || position === 'top'

    return {
      node: {
        direction,
        first: sourceFirst ? sourcePaneId : node,
        second: sourceFirst ? node : sourcePaneId,
        splitPercentage: 50,
      },
      inserted: true,
    }
  }

  const insertedInFirst = insertPaneRelativeToTarget(
    node.first,
    targetPaneId,
    sourcePaneId,
    position,
  )
  if (insertedInFirst.inserted) {
    return {
      node: {
        ...node,
        first: insertedInFirst.node,
      },
      inserted: true,
    }
  }

  const insertedInSecond = insertPaneRelativeToTarget(
    node.second,
    targetPaneId,
    sourcePaneId,
    position,
  )
  if (insertedInSecond.inserted) {
    return {
      node: {
        ...node,
        second: insertedInSecond.node,
      },
      inserted: true,
    }
  }

  return { node, inserted: false }
}

/** Timer para debounced auto-save */
let saveTimer: ReturnType<typeof setTimeout> | null = null
const SAVE_DEBOUNCE_MS = 2000

function scheduleSave() {
  if (saveTimer) clearTimeout(saveTimer)
  saveTimer = setTimeout(() => {
    const state = useLayoutStore.getState()
    state.saveLayout()
  }, SAVE_DEBOUNCE_MS)
}

export const useLayoutStore = create<LayoutState & LayoutActions>((set, get) => ({
  // Initial state
  panes: {},
  paneOrder: [],
  mosaicNode: null,
  activePaneId: null,
  zenModePane: null,
  zenModePreviousNode: null,
  paneCounter: 0,

  // === Pane Management ===

  addPane: (type, title, config) => {
    const state = get()
    const counter = state.paneCounter + 1
    const id = `pane-${counter}`
    const defaultTitle = title || (type === 'terminal' ? `Terminal ${counter}` : type === 'ai_agent' ? `AI Agent ${counter}` : `GitHub ${counter}`)

    const newPane: PaneInfo = {
      id,
      title: defaultTitle,
      type,
      status: 'idle',
      isMinimized: false,
      config,
    }

    const newOrder = [...state.paneOrder, id]
    const newPanes = { ...state.panes, [id]: newPane }

    set({
      panes: newPanes,
      paneOrder: newOrder,
      paneCounter: counter,
      activePaneId: id,
      mosaicNode: calculateLayout(newOrder),
    })

    scheduleSave()
    return id
  },

  replaceWithPanes: (nextPanes) => {
    const paneMap: Record<string, PaneInfo> = {}
    const paneOrder: string[] = []
    let maxCounter = 0

    nextPanes.forEach((pane, index) => {
      paneMap[pane.id] = pane
      paneOrder.push(pane.id)

      const match = pane.id.match(/(\d+)$/)
      if (match) {
        const parsed = Number(match[1])
        if (Number.isInteger(parsed) && parsed > maxCounter) {
          maxCounter = parsed
        }
      } else if (index+1 > maxCounter) {
        maxCounter = index + 1
      }
    })

    set({
      panes: paneMap,
      paneOrder,
      activePaneId: paneOrder.length > 0 ? paneOrder[0] : null,
      mosaicNode: calculateLayout(paneOrder),
      zenModePane: null,
      zenModePreviousNode: null,
      paneCounter: maxCounter,
    })
  },

  removePane: (id) => {
    const state = get()
    const newPanes = { ...state.panes }
    delete newPanes[id]
    const newOrder = state.paneOrder.filter(pId => pId !== id)

    // Se removeu o painel ativo, focar no último
    let newActive = state.activePaneId
    if (newActive === id) {
      newActive = newOrder.length > 0 ? newOrder[newOrder.length - 1] : null
    }

    set({
      panes: newPanes,
      paneOrder: newOrder,
      activePaneId: newActive,
      mosaicNode: calculateLayout(newOrder),
    })

    scheduleSave()
  },

  renamePane: (id, title) => {
    const state = get()
    if (!state.panes[id]) return
    set({
      panes: {
        ...state.panes,
        [id]: { ...state.panes[id], title },
      },
    })
  },

  updatePaneStatus: (id, status) => {
    const state = get()
    if (!state.panes[id]) return
    set({
      panes: {
        ...state.panes,
        [id]: { ...state.panes[id], status },
      },
    })
  },

  setPaneSessionID: (id, sessionID) => {
    const state = get()
    if (!state.panes[id]) return
    set({
      panes: {
        ...state.panes,
        [id]: { ...state.panes[id], sessionID },
      },
    })
  },

  // === Focus ===

  setActivePaneId: (id) => set({ activePaneId: id }),

  focusNextPane: () => {
    const { paneOrder, activePaneId } = get()
    if (paneOrder.length === 0) return
    const currentIndex = activePaneId ? paneOrder.indexOf(activePaneId) : -1
    const nextIndex = (currentIndex + 1) % paneOrder.length
    set({ activePaneId: paneOrder[nextIndex] })
  },

  focusPrevPane: () => {
    const { paneOrder, activePaneId } = get()
    if (paneOrder.length === 0) return
    const currentIndex = activePaneId ? paneOrder.indexOf(activePaneId) : 0
    const prevIndex = (currentIndex - 1 + paneOrder.length) % paneOrder.length
    set({ activePaneId: paneOrder[prevIndex] })
  },

  focusPaneByIndex: (index) => {
    const { paneOrder } = get()
    if (index >= 0 && index < paneOrder.length) {
      set({ activePaneId: paneOrder[index] })
    }
  },

  // === Mosaic ===

  setMosaicNode: (node) => {
    set({ mosaicNode: node })
    scheduleSave()
  },

  swapPanePositions: (sourcePaneId, targetPaneId) => {
    if (!sourcePaneId || !targetPaneId || sourcePaneId === targetPaneId) {
      return
    }

    const { mosaicNode } = get()
    if (!mosaicNode) {
      return
    }

    if (!mosaicContainsPane(mosaicNode, sourcePaneId) || !mosaicContainsPane(mosaicNode, targetPaneId)) {
      return
    }

    const nextMosaicNode = swapPaneIdsInMosaicNode(mosaicNode, sourcePaneId, targetPaneId)
    set({ mosaicNode: nextMosaicNode, activePaneId: sourcePaneId })
    scheduleSave()
  },

  movePaneToPosition: (sourcePaneId, targetPaneId, position) => {
    if (!sourcePaneId || !targetPaneId || sourcePaneId === targetPaneId) {
      return
    }

    if (position === 'center') {
      get().swapPanePositions(sourcePaneId, targetPaneId)
      return
    }

    const { mosaicNode } = get()
    if (!mosaicNode) {
      return
    }

    if (!mosaicContainsPane(mosaicNode, sourcePaneId) || !mosaicContainsPane(mosaicNode, targetPaneId)) {
      return
    }

    const removed = removePaneFromMosaicNode(mosaicNode, sourcePaneId)
    if (!removed.removed || !removed.node) {
      return
    }

    const inserted = insertPaneRelativeToTarget(removed.node, targetPaneId, sourcePaneId, position)
    if (!inserted.inserted) {
      return
    }

    set({ mosaicNode: inserted.node, activePaneId: sourcePaneId })
    scheduleSave()
  },

  updateLayout: () => {
    const { paneOrder } = get()
    set({ mosaicNode: calculateLayout(paneOrder) })
  },

  // === Zen Mode ===

  enterZenMode: (paneId) => {
    const { mosaicNode } = get()
    set({
      zenModePane: paneId,
      zenModePreviousNode: mosaicNode,
      activePaneId: paneId,
    })
  },

  exitZenMode: () => {
    const { zenModePreviousNode } = get()
    set({
      zenModePane: null,
      mosaicNode: zenModePreviousNode,
      zenModePreviousNode: null,
    })
  },

  toggleZenMode: (paneId) => {
    const state = get()
    if (state.zenModePane) {
      state.exitZenMode()
    } else {
      const target = paneId || state.activePaneId
      if (target) state.enterZenMode(target)
    }
  },

  // === Reset ===

  reset: () => set({
    panes: {},
    paneOrder: [],
    mosaicNode: null,
    activePaneId: null,
    zenModePane: null,
    zenModePreviousNode: null,
    paneCounter: 0,
  }),

  // === Persistence ===

  saveLayout: async () => {
    const state = get()
    const json = serializeLayout(state)
    try {
      // Salvar via Wails binding se disponível
      if (window.go?.main?.App?.SaveLayoutState) {
        await window.go.main.App.SaveLayoutState(json)
      } else {
        // Fallback: localStorage para dev
        localStorage.setItem('orch:layout', json)
      }
    } catch (err) {
      console.warn('[Layout] Failed to persist layout:', err)
    }
  },

  restoreLayout: async () => {
    try {
      let json: string | null = null

      if (window.go?.main?.App?.GetLayoutState) {
        json = await window.go.main.App.GetLayoutState()
      } else {
        // Fallback: localStorage para dev
        json = localStorage.getItem('orch:layout')
      }

      if (json) {
        get().loadSerializedLayout(json)
      }

      // Buscar snapshots de CLIs salvas para restaurar sessões
      try {
        const snapshots = await window.go?.main?.App?.GetTerminalSnapshots()
        if (snapshots && snapshots.length > 0) {
          const panes = { ...get().panes }
          let injected = 0
          for (const snap of snapshots) {
            if (panes[snap.paneId]) {
              panes[snap.paneId] = {
                ...panes[snap.paneId],
                config: {
                  ...panes[snap.paneId].config,
                  restoreSnapshot: snap,
                },
              }
              injected++
            }
          }
          if (injected > 0) {
            set({ panes })
            console.log(`[Layout] Injected ${injected} CLI restore snapshots`)
          }
          // Limpar snapshots do backend para evitar uso acidental no futuro
          await window.go?.main?.App?.ClearTerminalSnapshots()
        }
      } catch (snapErr) {
        console.warn('[Layout] Failed to load terminal snapshots:', snapErr)
      }
    } catch (err) {
      console.warn('[Layout] Failed to restore layout:', err)
    }
  },

  getSerializedLayout: () => {
    return serializeLayout(get())
  },

  loadSerializedLayout: (json: string) => {
    try {
      const data = JSON.parse(json)
      if (data.panes && data.paneOrder) {
        // Validar: filtrar paneOrder apenas para painéis que realmente existem
        const validPaneOrder = data.paneOrder.filter((id: string) => !!data.panes[id])

        // Validar: garantir que todos os painéis em panes estão no paneOrder
        const validPanes: Record<string, PaneInfo> = {}
        for (const id of validPaneOrder) {
          const pane = { ...data.panes[id] }
          // Limpar sessionID — terminais anteriores não estão vivos após restart
          delete pane.sessionID
          // Resetar status para idle
          pane.status = 'idle'
          validPanes[id] = pane
        }

        if (validPaneOrder.length === 0) {
          // Nenhum painel válido — resetar para estado limpo
          console.warn('[Layout] No valid panes in saved layout, resetting')
          return
        }

        // Validar mosaicNode — verificar se referencia apenas painéis válidos
        const validPaneSet = new Set(validPaneOrder)
        const isMosaicNodeValid = (node: MosaicNode<string> | null | undefined): boolean => {
          if (node == null) return false
          if (typeof node === 'string') return validPaneSet.has(node)
          if (typeof node === 'object' && 'first' in node && 'second' in node) {
            return isMosaicNodeValid(node.first) && isMosaicNodeValid(node.second)
          }
          return false
        }

        const mosaicNode = isMosaicNodeValid(data.mosaicNode)
          ? data.mosaicNode
          : calculateLayout(validPaneOrder)

        const activePaneId = data.activePaneId && validPaneSet.has(data.activePaneId)
          ? data.activePaneId
          : (validPaneOrder.length > 0 ? validPaneOrder[0] : null)

        set({
          panes: validPanes,
          paneOrder: validPaneOrder,
          mosaicNode,
          activePaneId,
          paneCounter: data.paneCounter || validPaneOrder.length,
        })
      }
    } catch (err) {
      console.warn('[Layout] Failed to parse layout:', err)
    }
  },

  // === Session Persistence ===

  captureTerminalSnapshots: () => {
    const { panes, paneOrder } = get()
    const snapshots: TerminalSnapshotDTO[] = []

    for (const id of paneOrder) {
      const pane = panes[id]
      if (!pane) continue

      // Apenas panes com sessão ativa
      if (!pane.sessionID) continue
      if (pane.type !== 'terminal' && pane.type !== 'ai_agent') continue

      snapshots.push({
        paneId: id,
        sessionId: pane.sessionID,
        paneTitle: pane.title,
        paneType: pane.type,
        shell: pane.config?.shell || '',
        cwd: pane.config?.cwd || '',
        useDocker: !!pane.config?.useDocker,
        config: pane.config ? JSON.stringify(pane.config) : undefined,
      })
    }

    return snapshots
  },
}))
