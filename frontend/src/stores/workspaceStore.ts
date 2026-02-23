import { create } from 'zustand'
import { useLayoutStore } from '../features/command-center/stores/layoutStore'
import type { PaneInfo, PaneStatus, PaneType } from '../features/command-center/types/layout'

export interface AgentSessionNode {
  id: number
  workspaceId: number
  name: string
  type: string
  shell: string
  cwd: string
  useDocker: boolean
  sessionId?: string
  status: string
  sortOrder: number
  isMinimized: boolean
}

export interface WorkspaceNode {
  id: number
  userId: string
  name: string
  path: string
  gitRemote?: string
  owner?: string
  repo?: string
  color?: string
  isActive: boolean
  lastOpenedAt?: string
  agents: AgentSessionNode[]
}

interface WorkspaceState {
  workspaces: WorkspaceNode[]
  activeWorkspaceId: number | null
  historyByAgentId: Record<number, string>
  isLoading: boolean
  isSessionWorkspaceScoped: boolean
}

interface WorkspaceActions {
  loadWorkspaces: () => Promise<void>
  setActiveWorkspace: (workspaceId: number) => Promise<void>
  createWorkspace: (name?: string) => Promise<WorkspaceNode | null>
  renameWorkspace: (workspaceId: number, name: string) => Promise<void>
  setWorkspaceColor: (workspaceId: number, color: string) => Promise<void>
  deleteWorkspace: (workspaceId: number) => Promise<void>
  createTerminalForActiveWorkspace: (useDocker: boolean) => Promise<void>
  moveTerminalToWorkspace: (agentId: number, targetWorkspaceId: number) => Promise<void>
  closePane: (paneId: string) => Promise<void>
  applySessionWorkspaceSnapshot: (workspace: WorkspaceNode) => void
  clearSessionWorkspaceSnapshot: () => Promise<void>
}

interface WorkspaceColorAppAPI {
  SetWorkspaceColor?: (id: number, color: string) => Promise<WorkspaceNode>
}

interface WorkspaceMoveAppAPI {
  MoveAgentSessionToWorkspace?: (
    agentId: number,
    workspaceId: number,
  ) => Promise<AgentSessionNode>
}

const normalizePaneType = (type: string): PaneType => {
  switch (type) {
    case 'ai_agent':
      return 'ai_agent'
    default:
      return 'terminal'
  }
}

const isLegacyGitHubAgent = (type: string): boolean => {
  return type.trim().toLowerCase() === 'github'
}

const normalizePaneStatus = (status: string): PaneStatus => {
  switch (status) {
    case 'running':
      return 'running'
    case 'error':
      return 'error'
    default:
      return 'idle'
  }
}

const toPaneInfo = (workspaceID: number, agent: AgentSessionNode): PaneInfo => {
  return {
    id: `ws-${workspaceID}-agent-${agent.id}`,
    title: agent.name || `Terminal ${agent.id}`,
    type: normalizePaneType(agent.type),
    status: normalizePaneStatus(agent.status),
    sessionID: agent.sessionId || undefined,
    agentDBID: agent.id,
    workspaceID,
    isMinimized: agent.isMinimized,
    config: {
      shell: agent.shell,
      cwd: agent.cwd,
      useDocker: agent.useDocker,
    },
  }
}

const parseHistoryMap = (input: Record<string, string> | null | undefined): Record<number, string> => {
  if (!input) {
    return {}
  }

  const parsed: Record<number, string> = {}
  for (const [key, value] of Object.entries(input)) {
    const agentID = Number(key)
    if (Number.isInteger(agentID) && agentID > 0) {
      parsed[agentID] = value
    }
  }
  return parsed
}

const normalizeWorkspaceNode = (workspace: WorkspaceNode): WorkspaceNode => ({
  ...workspace,
  agents: Array.isArray(workspace.agents) ? workspace.agents : [],
})

const normalizeWorkspaceNodes = (
  workspaces: WorkspaceNode[] | null | undefined,
): WorkspaceNode[] => {
  if (!Array.isArray(workspaces)) {
    return []
  }
  return workspaces.map((workspace) => normalizeWorkspaceNode(workspace))
}

function emitWorkspaceSyncHint(workspaceID: number) {
  if (!Number.isInteger(workspaceID) || workspaceID <= 0) {
    return
  }
  window.dispatchEvent(new CustomEvent('session:host-workspace:changed', {
    detail: { workspaceID },
  }))
}

export const useWorkspaceStore = create<WorkspaceState & WorkspaceActions>((set, get) => {
  let snapshotCacheLoaded = false
  let snapshotCacheLoadPromise: Promise<void> | null = null
  let pendingSnapshotsByPaneID: Record<string, TerminalSnapshotDTO> = {}
  let legacyGitHubMigrationOpened = false

  const ensureSnapshotCache = async () => {
    if (snapshotCacheLoaded) {
      return
    }
    if (snapshotCacheLoadPromise) {
      await snapshotCacheLoadPromise
      return
    }

    snapshotCacheLoadPromise = (async () => {
      if (!window.go?.main?.App?.GetTerminalSnapshots) {
        snapshotCacheLoaded = true
        return
      }

      try {
        const snapshots = await window.go.main.App.GetTerminalSnapshots()
        const map: Record<string, TerminalSnapshotDTO> = {}
        for (const snapshot of snapshots || []) {
          if (!snapshot?.paneId) continue
          map[snapshot.paneId] = snapshot
        }
        pendingSnapshotsByPaneID = map

        if ((snapshots?.length || 0) > 0) {
          await window.go.main.App.ClearTerminalSnapshots?.()
          console.log(`[WorkspaceStore] Loaded ${snapshots.length} terminal snapshots for restore`)
        }
      } catch (err) {
        console.warn('[WorkspaceStore] Failed to load terminal snapshots:', err)
      } finally {
        snapshotCacheLoaded = true
      }
    })()

    try {
      await snapshotCacheLoadPromise
    } finally {
      snapshotCacheLoadPromise = null
    }
  }

  const syncLayoutFromWorkspace = (workspaceID: number | null, workspaces: WorkspaceNode[]) => {
    if (!workspaceID) {
      useLayoutStore.getState().replaceWithPanes([])
      return
    }

    const workspace = normalizeWorkspaceNodes(workspaces).find((ws) => ws.id === workspaceID)
    if (!workspace) {
      useLayoutStore.getState().replaceWithPanes([])
      return
    }

    const legacyGitHubAgents = workspace.agents.filter((agent) => isLegacyGitHubAgent(agent.type))
    if (legacyGitHubAgents.length > 0 && !legacyGitHubMigrationOpened) {
      legacyGitHubMigrationOpened = true
      window.dispatchEvent(new CustomEvent('git-panel:open'))
    }

    const visibleAgents = workspace.agents.filter((agent) => !isLegacyGitHubAgent(agent.type))

    const orderedAgents = [...visibleAgents].sort((a, b) => {
      if (a.sortOrder !== b.sortOrder) {
        return a.sortOrder - b.sortOrder
      }
      return a.id - b.id
    })

    const panes = orderedAgents.map((agent) => toPaneInfo(workspaceID, agent))
    for (const pane of panes) {
      const snapshot = pendingSnapshotsByPaneID[pane.id]
      if (!snapshot) {
        continue
      }

      pane.config = {
        ...(pane.config || {}),
        restoreSnapshot: snapshot,
        shell: snapshot.shell || pane.config?.shell || '',
        cwd: snapshot.cwd || pane.config?.cwd || '',
        useDocker: snapshot.useDocker ?? !!pane.config?.useDocker,
      }

      delete pendingSnapshotsByPaneID[pane.id]
    }

    useLayoutStore.getState().replaceWithPanes(panes)
  }

  const fetchWorkspaceHistory = async (workspaceID: number): Promise<Record<number, string>> => {
    if (!window.go?.main?.App?.GetWorkspaceHistoryBuffer) {
      return {}
    }

    try {
      const raw = await window.go.main.App.GetWorkspaceHistoryBuffer(workspaceID)
      return parseHistoryMap(raw)
    } catch (err) {
      console.warn('[WorkspaceStore] Failed to fetch workspace history:', err)
      return {}
    }
  }

  const activateWorkspace = async (
    workspaceID: number,
    persistBackend: boolean,
    sourceWorkspaces?: WorkspaceNode[],
  ) => {
    // Se já for a workspace ativa, não faz nada para evitar flicker/snap-back
    if (get().activeWorkspaceId === workspaceID && !sourceWorkspaces) {
      return
    }

    const currentWorkspaces = normalizeWorkspaceNodes(sourceWorkspaces ?? get().workspaces)
    const exists = currentWorkspaces.some((ws) => ws.id === workspaceID)
    if (!exists) {
      return
    }

    if (persistBackend && window.go?.main?.App?.SetActiveWorkspace) {
      await window.go.main.App.SetActiveWorkspace(workspaceID)
    }

    await ensureSnapshotCache()

    const historyByAgentId = await fetchWorkspaceHistory(workspaceID)
    const nextWorkspaces = currentWorkspaces.map((ws) => ({
      ...ws,
      isActive: ws.id === workspaceID,
    }))

    set({
      workspaces: nextWorkspaces,
      activeWorkspaceId: workspaceID,
      historyByAgentId,
    })

    syncLayoutFromWorkspace(workspaceID, nextWorkspaces)
  }

  return {
    workspaces: [],
    activeWorkspaceId: null,
    historyByAgentId: {},
    isLoading: false,
    isSessionWorkspaceScoped: false,

    loadWorkspaces: async () => {
      if (get().isSessionWorkspaceScoped) {
        return
      }

      if (!window.go?.main?.App?.GetWorkspacesWithAgents) {
        const fallback: WorkspaceNode = {
          id: 1,
          userId: 'local',
          name: 'Default',
          path: '',
          isActive: true,
          agents: [],
        }
        set({
          workspaces: [fallback],
          activeWorkspaceId: fallback.id,
          historyByAgentId: {},
          isLoading: false,
        })
        syncLayoutFromWorkspace(fallback.id, [fallback])
        return
      }

      set({ isLoading: true })

      try {
        const workspaces = normalizeWorkspaceNodes(await window.go.main.App.GetWorkspacesWithAgents())
        const active = workspaces.find((ws) => ws.isActive) ?? workspaces[0] ?? null

        set({
          workspaces,
          activeWorkspaceId: active?.id ?? null,
          historyByAgentId: {},
          isLoading: false,
        })

        if (active) {
          await activateWorkspace(active.id, false, workspaces)
        } else {
          syncLayoutFromWorkspace(null, [])
        }
      } catch (err) {
        set({ isLoading: false })
        console.error('[WorkspaceStore] Failed to load workspaces:', err)
      }
    },

    setActiveWorkspace: async (workspaceId) => {
      if (get().isSessionWorkspaceScoped && get().activeWorkspaceId !== workspaceId) {
        return
      }
      await activateWorkspace(workspaceId, true)
    },

    createWorkspace: async (name) => {
      if (get().isSessionWorkspaceScoped) {
        throw new Error('workspace changes are disabled for guest collaboration')
      }

      const current = normalizeWorkspaceNodes(get().workspaces)
      const suggestedName = (name || '').trim() || `Workspace ${current.length + 1}`

      if (!window.go?.main?.App?.CreateWorkspace) {
        const nextID = current.length > 0 ? Math.max(...current.map((ws) => ws.id)) + 1 : 1
        const created: WorkspaceNode = {
          id: nextID,
          userId: 'local',
          name: suggestedName,
          path: '',
          isActive: false,
          agents: [],
        }
        const next = [...current, created]
        set({ workspaces: next })
        await activateWorkspace(created.id, false, next)
        return created
      }

      const createdWorkspace = await window.go.main.App.CreateWorkspace(suggestedName)
      const created: WorkspaceNode = {
        ...createdWorkspace,
        agents: createdWorkspace.agents ?? [],
      }

      const next = [...current, created]
      set({ workspaces: next })
      await activateWorkspace(created.id, true, next)
      emitWorkspaceSyncHint(created.id)
      return created
    },

    renameWorkspace: async (workspaceId, name) => {
      if (get().isSessionWorkspaceScoped) {
        throw new Error('workspace changes are disabled for guest collaboration')
      }

      const trimmed = name.trim()
      if (!trimmed) {
        return
      }

      if (window.go?.main?.App?.RenameWorkspace) {
        await window.go.main.App.RenameWorkspace(workspaceId, trimmed)
      }

      set((state) => ({
        workspaces: state.workspaces.map((ws) =>
          ws.id === workspaceId ? { ...ws, name: trimmed } : ws,
        ),
      }))
      emitWorkspaceSyncHint(workspaceId)
    },

    setWorkspaceColor: async (workspaceId, color) => {
      if (get().isSessionWorkspaceScoped) {
        throw new Error('workspace changes are disabled for guest collaboration')
      }

      const appApi = window.go?.main?.App as WorkspaceColorAppAPI | undefined
      if (appApi?.SetWorkspaceColor) {
        await appApi.SetWorkspaceColor(workspaceId, color)
      }

      set((state) => ({
        workspaces: state.workspaces.map((ws) =>
          ws.id === workspaceId ? { ...ws, color } : ws,
        ),
      }))
      emitWorkspaceSyncHint(workspaceId)
    },

    deleteWorkspace: async (workspaceId) => {
      if (get().isSessionWorkspaceScoped) {
        throw new Error('workspace changes are disabled for guest collaboration')
      }

      const state = get()
      const stateWorkspaces = normalizeWorkspaceNodes(state.workspaces)
      if (stateWorkspaces.length <= 1) {
        throw new Error('cannot delete last workspace')
      }

      if (window.go?.main?.App?.DeleteWorkspace) {
        await window.go.main.App.DeleteWorkspace(workspaceId)
      }

      const remaining = stateWorkspaces.filter((ws) => ws.id !== workspaceId)
      const nextActiveId = state.activeWorkspaceId === workspaceId
        ? (remaining[0]?.id ?? null)
        : state.activeWorkspaceId

      set({ workspaces: remaining })

      if (nextActiveId) {
        await activateWorkspace(nextActiveId, true, remaining)
      } else {
        set({ activeWorkspaceId: null, historyByAgentId: {} })
        syncLayoutFromWorkspace(null, remaining)
      }

      emitWorkspaceSyncHint(workspaceId)
    },

    createTerminalForActiveWorkspace: async (useDocker) => {
      if (get().isSessionWorkspaceScoped) {
        throw new Error('only the host can create terminals in this collaboration session')
      }

      let workspaceId = get().activeWorkspaceId
      if (!workspaceId) {
        await get().loadWorkspaces()
        workspaceId = get().activeWorkspaceId
      }

      if (!workspaceId) {
        const createdWorkspace = await get().createWorkspace('Default')
        workspaceId = createdWorkspace?.id ?? null
      }

      if (!workspaceId) {
        throw new Error('no active workspace available')
      }

      const workspaces = normalizeWorkspaceNodes(get().workspaces)
      const targetWorkspace = workspaces.find((ws) => ws.id === workspaceId)
      if (!targetWorkspace) {
        throw new Error('active workspace not found')
      }

      const totalAgents = workspaces.reduce((acc, ws) => acc + ws.agents.length, 0)
      if (totalAgents === 0) {
        const defaultWorkspace = workspaces.find((ws) => ws.name.trim().toLowerCase() === 'default')
        if (defaultWorkspace && defaultWorkspace.id !== workspaceId) {
          await get().setActiveWorkspace(defaultWorkspace.id)
          workspaceId = defaultWorkspace.id
        }
      }

      const refreshedWorkspaces = normalizeWorkspaceNodes(get().workspaces)
      const refreshedTarget = refreshedWorkspaces.find((ws) => ws.id === workspaceId)
      if (!refreshedTarget) {
        throw new Error('target workspace unavailable')
      }

      const defaultName = `Terminal ${refreshedTarget.agents.length + 1}`
      const baselineAgentIDs = new Set(
        refreshedWorkspaces.flatMap((ws) => ws.agents.map((agent) => agent.id)),
      )

      const normalizeCreatedAgent = (
        raw: unknown,
        fallbackWorkspaceID: number,
      ): AgentSessionNode | null => {
        if (!raw || typeof raw !== 'object') {
          return null
        }

        const candidate = raw as Partial<AgentSessionNode> & {
          id?: unknown
          workspaceId?: unknown
          sortOrder?: unknown
        }

        const parsedID = Number(candidate.id)
        if (!Number.isInteger(parsedID) || parsedID <= 0) {
          return null
        }

        const parsedWorkspaceID = Number(candidate.workspaceId)
        const parsedSortOrder = Number(candidate.sortOrder)

        return {
          id: parsedID,
          workspaceId: Number.isInteger(parsedWorkspaceID) && parsedWorkspaceID > 0
            ? parsedWorkspaceID
            : fallbackWorkspaceID,
          name: typeof candidate.name === 'string' && candidate.name.trim().length > 0
            ? candidate.name
            : defaultName,
          type: typeof candidate.type === 'string' && candidate.type.trim().length > 0
            ? candidate.type
            : 'terminal',
          shell: typeof candidate.shell === 'string' ? candidate.shell : '',
          cwd: typeof candidate.cwd === 'string' ? candidate.cwd : '',
          useDocker: typeof candidate.useDocker === 'boolean' ? candidate.useDocker : useDocker,
          sessionId: typeof candidate.sessionId === 'string' && candidate.sessionId.length > 0
            ? candidate.sessionId
            : undefined,
          status: typeof candidate.status === 'string' && candidate.status.length > 0
            ? candidate.status
            : 'idle',
          sortOrder: Number.isInteger(parsedSortOrder) ? parsedSortOrder : refreshedTarget.agents.length,
          isMinimized: Boolean(candidate.isMinimized),
        }
      }

      const createAgentLegacy = async (): Promise<AgentSessionNode | null> => {
        if (!window.go?.main?.App?.CreateAgent) {
          return null
        }
        const legacy = await window.go.main.App.CreateAgent(defaultName, 'terminal')
        return normalizeCreatedAgent(legacy, workspaceId)
      }

      const recoverAgentFromBackend = async (): Promise<{
        createdAgent: AgentSessionNode
        sourceWorkspaces: WorkspaceNode[]
      } | null> => {
        if (!window.go?.main?.App?.GetWorkspacesWithAgents) {
          return null
        }

        const latest = await window.go.main.App.GetWorkspacesWithAgents()
        const normalized: WorkspaceNode[] = latest.map((ws) => ({
          ...ws,
          agents: ws.agents ?? [],
        }))

        for (const ws of normalized) {
          const found = ws.agents.find((agent) => !baselineAgentIDs.has(agent.id))
          if (found) {
            const parsed = normalizeCreatedAgent(found, ws.id)
            if (parsed) {
              return { createdAgent: parsed, sourceWorkspaces: normalized }
            }
          }
        }

        return null
      }

      const createLocalAgent = (): AgentSessionNode => {
        const maxID = refreshedWorkspaces.reduce((acc, ws) => {
          const localMax = ws.agents.reduce((agentAcc, agent) => Math.max(agentAcc, agent.id), 0)
          return Math.max(acc, localMax)
        }, 0)
        return {
          id: maxID + 1,
          workspaceId,
          name: defaultName,
          type: 'terminal',
          shell: '',
          cwd: '',
          useDocker,
          status: 'idle',
          sortOrder: refreshedTarget.agents.length,
          isMinimized: false,
        }
      }

      let created: AgentSessionNode | null = null
      let sourceWorkspaces = normalizeWorkspaceNodes(refreshedWorkspaces)
      const backendUnavailable = !window.go?.main?.App

      if (window.go?.main?.App?.CreateAgentSession) {
        try {
          const createdPayload = await window.go.main.App.CreateAgentSession(workspaceId, defaultName, 'terminal')
          created = normalizeCreatedAgent(createdPayload, workspaceId)

          if (!created) {
            const recovered = await recoverAgentFromBackend()
            if (recovered) {
              created = recovered.createdAgent
              sourceWorkspaces = recovered.sourceWorkspaces
            }
          }

          if (!created) {
            console.warn('[WorkspaceStore] CreateAgentSession returned empty payload, trying legacy CreateAgent')
            created = await createAgentLegacy()
          }

          if (!created) {
            const recovered = await recoverAgentFromBackend()
            if (recovered) {
              created = recovered.createdAgent
              sourceWorkspaces = recovered.sourceWorkspaces
            }
          }
        } catch (err) {
          console.warn('[WorkspaceStore] CreateAgentSession failed, trying backend recovery:', err)

          const recovered = await recoverAgentFromBackend()
          if (recovered) {
            created = recovered.createdAgent
            sourceWorkspaces = recovered.sourceWorkspaces
          }

          if (!created) {
            console.warn('[WorkspaceStore] Backend recovery failed, trying legacy CreateAgent')
            created = await createAgentLegacy()
          }

          if (!created) {
            const recoveredAfterLegacy = await recoverAgentFromBackend()
            if (recoveredAfterLegacy) {
              created = recoveredAfterLegacy.createdAgent
              sourceWorkspaces = recoveredAfterLegacy.sourceWorkspaces
            }
          }
        }
      } else {
        created = await createAgentLegacy()
      }

      if (!created && backendUnavailable) {
        created = createLocalAgent()
      }

      if (!created) {
        throw new Error('failed to create agent session (empty backend payload)')
      }

      const createdAgent: AgentSessionNode = {
        ...created,
        useDocker: created.useDocker ?? useDocker,
      }

      const finalWorkspaceID = createdAgent.workspaceId || workspaceId
      const safeSourceWorkspaces = normalizeWorkspaceNodes(sourceWorkspaces)
      const workspaceExists = safeSourceWorkspaces.some((ws) => ws.id === finalWorkspaceID)
      if (!workspaceExists) {
        throw new Error('target workspace unavailable after terminal creation')
      }

      const next = safeSourceWorkspaces.map((ws) => {
        const isTarget = ws.id === finalWorkspaceID
        if (!isTarget) {
          return { ...ws, isActive: false }
        }

        const alreadyExists = ws.agents.some((agent) => agent.id === createdAgent.id)
        if (alreadyExists) {
          return { ...ws, isActive: true }
        }

        return {
          ...ws,
          isActive: true,
          agents: [...ws.agents, createdAgent],
        }
      })

      set({
        workspaces: next,
        activeWorkspaceId: finalWorkspaceID,
      })
      syncLayoutFromWorkspace(finalWorkspaceID, next)
      useLayoutStore.getState().setActivePaneId(`ws-${finalWorkspaceID}-agent-${createdAgent.id}`)
      emitWorkspaceSyncHint(finalWorkspaceID)
    },

    moveTerminalToWorkspace: async (agentId, targetWorkspaceId) => {
      if (get().isSessionWorkspaceScoped) {
        throw new Error('only the host can move terminals in this collaboration session')
      }

      if (!Number.isInteger(agentId) || agentId <= 0) {
        throw new Error('invalid agent id')
      }
      if (!Number.isInteger(targetWorkspaceId) || targetWorkspaceId <= 0) {
        throw new Error('invalid target workspace id')
      }

      const current = normalizeWorkspaceNodes(get().workspaces)
      const targetWorkspace = current.find((ws) => ws.id === targetWorkspaceId)
      if (!targetWorkspace) {
        throw new Error('target workspace not found')
      }

      const sourceWorkspace = current.find((ws) => ws.agents.some((agent) => agent.id === agentId))
      if (!sourceWorkspace) {
        throw new Error('agent not found')
      }

      if (sourceWorkspace.id === targetWorkspaceId) {
        return
      }

      const agent = sourceWorkspace.agents.find((entry) => entry.id === agentId)
      if (!agent) {
        throw new Error('agent not found in source workspace')
      }

      // 1. Atualização Otimista
      const optimisticWorkspaces = current.map((ws) => {
        if (ws.id === sourceWorkspace.id) {
          return {
            ...ws,
            isActive: false,
            agents: ws.agents.filter((a) => a.id !== agentId),
          }
        }
        if (ws.id === targetWorkspaceId) {
          const movedAgent: AgentSessionNode = {
            ...agent,
            workspaceId: targetWorkspaceId,
            sortOrder: ws.agents.length,
          }
          return {
            ...ws,
            isActive: true,
            agents: [...ws.agents, movedAgent],
          }
        }
        return { ...ws, isActive: false }
      })

      set({
        workspaces: optimisticWorkspaces,
        activeWorkspaceId: targetWorkspaceId,
      })
      syncLayoutFromWorkspace(targetWorkspaceId, optimisticWorkspaces)

      const appApi = window.go?.main?.App as WorkspaceMoveAppAPI | undefined

      try {
        // 2. Persistência no Backend
        if (appApi?.MoveAgentSessionToWorkspace) {
          // O backend agora move e ativa a workspace em uma única transação
          await appApi.MoveAgentSessionToWorkspace(agentId, targetWorkspaceId)
          
          // Sincroniza o estado final real
          await get().loadWorkspaces()

          const movedPaneId = `ws-${targetWorkspaceId}-agent-${agentId}`
          if (useLayoutStore.getState().panes[movedPaneId]) {
            useLayoutStore.getState().setActivePaneId(movedPaneId)
          }
        }
        emitWorkspaceSyncHint(sourceWorkspace.id)
        emitWorkspaceSyncHint(targetWorkspaceId)
      } catch (err) {
        console.error('[WorkspaceStore] Failed to move terminal:', err)
        await get().loadWorkspaces()
      }
    },

    closePane: async (paneId) => {
      if (get().isSessionWorkspaceScoped) {
        throw new Error('only the host can close terminals in this collaboration session')
      }

      const pane = useLayoutStore.getState().panes[paneId]
      if (!pane) {
        return
      }

      if (!pane.agentDBID) {
        useLayoutStore.getState().removePane(paneId)
        return
      }

      if (window.go?.main?.App?.DeleteAgentSession) {
        await window.go.main.App.DeleteAgentSession(pane.agentDBID)
      }

      const workspaceID = get().activeWorkspaceId
      if (!workspaceID) {
        useLayoutStore.getState().removePane(paneId)
        return
      }

      const nextWorkspaces = normalizeWorkspaceNodes(get().workspaces).map((ws) => {
        if (ws.id !== workspaceID) {
          return ws
        }
        return {
          ...ws,
          agents: ws.agents.filter((agent) => agent.id !== pane.agentDBID),
        }
      })

      set({ workspaces: nextWorkspaces })
      syncLayoutFromWorkspace(workspaceID, nextWorkspaces)
      emitWorkspaceSyncHint(workspaceID)
    },

    applySessionWorkspaceSnapshot: (workspace) => {
      const normalized = normalizeWorkspaceNode(workspace)
      const scopedWorkspace: WorkspaceNode = {
        ...normalized,
        isActive: true,
      }

      const currentWorkspaces = normalizeWorkspaceNodes(get().workspaces)
      const scopedIndex = currentWorkspaces.findIndex((item) => item.id === scopedWorkspace.id)
      const scopedSet = scopedIndex >= 0
        ? currentWorkspaces.map((item) => (item.id === scopedWorkspace.id ? scopedWorkspace : item))
        : [...currentWorkspaces, scopedWorkspace]

      set({
        workspaces: scopedSet,
        activeWorkspaceId: scopedWorkspace.id,
        historyByAgentId: {},
        isSessionWorkspaceScoped: true,
        isLoading: false,
      })

      syncLayoutFromWorkspace(scopedWorkspace.id, scopedSet)
    },

    clearSessionWorkspaceSnapshot: async () => {
      if (!get().isSessionWorkspaceScoped) {
        return
      }

      set({
        isSessionWorkspaceScoped: false,
      })

      await get().loadWorkspaces()
    },
  }
})
