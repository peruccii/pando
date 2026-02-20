import { useCallback, useEffect, useMemo, useState } from 'react'
import { useSessionStore } from '../stores/sessionStore'
import { DATA_CHANNELS, P2PConnection } from '../services/P2PConnection'
import type { Session, GuestRequest, SessionRole } from '../stores/sessionStore'
import type { ScrollSyncEvent } from '../../scroll-sync/types'
import { useAuthStore } from '../../../stores/authStore'
import { useLayoutStore } from '../../command-center/stores/layoutStore'
import {
  useWorkspaceStore,
  type AgentSessionNode,
  type WorkspaceNode,
} from '../../../stores/workspaceStore'

interface AuditLog {
  id: number
  sessionID: string
  userID: string
  action: string
  details: string
  createdAt: string
}

interface SessionRuntimeState {
  listenersInitialized: boolean
  listenerConsumers: number
  p2p: P2PConnection | null
  p2pConnectInFlight: boolean
  p2pSessionID: string
  p2pUserID: string
  p2pIsHost: boolean
  joinPreviousRole: SessionRole
  activeSessionHydrated: boolean
}

const runtimeState: SessionRuntimeState = {
  listenersInitialized: false,
  listenerConsumers: 0,
  p2p: null,
  p2pConnectInFlight: false,
  p2pSessionID: '',
  p2pUserID: '',
  p2pIsHost: false,
  joinPreviousRole: 'none',
  activeSessionHydrated: false,
}

const CURSOR_PALETTE = ['#22c55e', '#3b82f6', '#f59e0b', '#ef4444', '#a855f7', '#06b6d4'] as const
const APPROVAL_TIMEOUT_MS = 5 * 60 * 1000
const APPROVAL_TIMEOUT_ERROR = 'Tempo de aprovação expirou. Solicite um novo código ao host.'
const SESSION_ENDED_ERROR = 'A sessão foi encerrada pelo host.'
const SESSION_REMOVED_ERROR = 'Você foi removido da sessão pelo host.'

function pickCursorColor(key: string): string {
  let hash = 0
  for (let i = 0; i < key.length; i++) {
    hash = (hash << 5) - hash + key.charCodeAt(i)
    hash |= 0
  }
  return CURSOR_PALETTE[Math.abs(hash) % CURSOR_PALETTE.length]
}

function extractPermission(payload: unknown): string | null {
  if (typeof payload === 'string') {
    return payload
  }
  if (!payload || typeof payload !== 'object') {
    return null
  }
  const maybePermission = (payload as { permission?: unknown }).permission
  return typeof maybePermission === 'string' ? maybePermission : null
}

function extractPermissionTargetUserID(payload: unknown): string | null {
  if (!payload || typeof payload !== 'object') {
    return null
  }
  const value = payload as { guestUserID?: unknown; userID?: unknown }
  const maybeTarget = typeof value.guestUserID === 'string' ? value.guestUserID : value.userID
  return typeof maybeTarget === 'string' ? maybeTarget : null
}

interface TerminalPeerPayload {
  data: string
  paneID?: string
  paneTitle?: string
  agentDBID?: number
  sessionID?: string
}

interface WorkspaceScopeSyncPayload {
  workspaceID: number
  workspace: WorkspaceNode
}

function decodeBase64UTF8(raw: string): string {
  try {
    const binaryString = atob(raw)
    const bytes = new Uint8Array(binaryString.length)
    for (let i = 0; i < binaryString.length; i++) {
      bytes[i] = binaryString.charCodeAt(i)
    }
    return new TextDecoder().decode(bytes)
  } catch {
    return raw
  }
}

function parseTerminalPeerPayload(payload: unknown): TerminalPeerPayload | null {
  if (!payload || typeof payload !== 'object') return null
  const value = payload as Record<string, unknown>
  if (typeof value.data !== 'string' || value.data.length === 0) {
    return null
  }

  const parsed: TerminalPeerPayload = { data: value.data }
  if (typeof value.paneID === 'string') parsed.paneID = value.paneID
  if (typeof value.paneTitle === 'string') parsed.paneTitle = value.paneTitle
  if (typeof value.agentDBID === 'number' && Number.isFinite(value.agentDBID)) {
    parsed.agentDBID = value.agentDBID
  }
  if (typeof value.sessionID === 'string') parsed.sessionID = value.sessionID
  return parsed
}

function normalizeSharedWorkspaceAgents(rawAgents: unknown, fallbackWorkspaceID: number): AgentSessionNode[] {
  if (!Array.isArray(rawAgents)) {
    return []
  }

  const agents: AgentSessionNode[] = []
  for (const item of rawAgents) {
    if (!item || typeof item !== 'object') {
      continue
    }

    const value = item as Record<string, unknown>
    const parsedAgentID = Number(value.id)
    if (!Number.isInteger(parsedAgentID) || parsedAgentID <= 0) {
      continue
    }

    const parsedWorkspaceID = Number(value.workspaceId)
    const parsedSortOrder = Number(value.sortOrder)
    const rawType = typeof value.type === 'string' ? value.type.trim() : ''
    const rawStatus = typeof value.status === 'string' ? value.status.trim() : ''

    agents.push({
      id: parsedAgentID,
      workspaceId: Number.isInteger(parsedWorkspaceID) && parsedWorkspaceID > 0 ? parsedWorkspaceID : fallbackWorkspaceID,
      name: typeof value.name === 'string' && value.name.trim().length > 0
        ? value.name
        : `Terminal ${parsedAgentID}`,
      type: rawType.length > 0 ? rawType : 'terminal',
      shell: typeof value.shell === 'string' ? value.shell : '',
      cwd: typeof value.cwd === 'string' ? value.cwd : '',
      useDocker: Boolean(value.useDocker),
      sessionId: typeof value.sessionId === 'string' && value.sessionId.length > 0
        ? value.sessionId
        : undefined,
      status: rawStatus.length > 0 ? rawStatus : 'idle',
      sortOrder: Number.isInteger(parsedSortOrder) ? parsedSortOrder : agents.length,
      isMinimized: Boolean(value.isMinimized),
    })
  }

  return agents.sort((a, b) => {
    if (a.sortOrder !== b.sortOrder) {
      return a.sortOrder - b.sortOrder
    }
    return a.id - b.id
  })
}

function parseWorkspaceScopeSyncPayload(payload: unknown): WorkspaceScopeSyncPayload | null {
  if (!payload || typeof payload !== 'object') {
    return null
  }

  const value = payload as Record<string, unknown>
  const rawWorkspace = value.workspace
  if (!rawWorkspace || typeof rawWorkspace !== 'object') {
    return null
  }

  const workspaceValue = rawWorkspace as Record<string, unknown>
  const parsedWorkspaceID = Number(
    value.workspaceID ?? workspaceValue.id,
  )
  if (!Number.isInteger(parsedWorkspaceID) || parsedWorkspaceID <= 0) {
    return null
  }

  const normalizedWorkspace: WorkspaceNode = {
    id: parsedWorkspaceID,
    userId: typeof workspaceValue.userId === 'string' ? workspaceValue.userId : 'host',
    name: typeof workspaceValue.name === 'string' && workspaceValue.name.trim().length > 0
      ? workspaceValue.name
      : `Workspace ${parsedWorkspaceID}`,
    path: typeof workspaceValue.path === 'string' ? workspaceValue.path : '',
    gitRemote: typeof workspaceValue.gitRemote === 'string' ? workspaceValue.gitRemote : undefined,
    owner: typeof workspaceValue.owner === 'string' ? workspaceValue.owner : undefined,
    repo: typeof workspaceValue.repo === 'string' ? workspaceValue.repo : undefined,
    color: typeof workspaceValue.color === 'string' ? workspaceValue.color : undefined,
    isActive: true,
    lastOpenedAt: typeof workspaceValue.lastOpenedAt === 'string' ? workspaceValue.lastOpenedAt : undefined,
    agents: normalizeSharedWorkspaceAgents(workspaceValue.agents, parsedWorkspaceID),
  }

  return {
    workspaceID: parsedWorkspaceID,
    workspace: normalizedWorkspace,
  }
}

function buildScopedHostWorkspaceSnapshot(): WorkspaceScopeSyncPayload | null {
  const sessionState = useSessionStore.getState()
  if (sessionState.role !== 'host' || !sessionState.session) {
    return null
  }

  const scopedWorkspaceID = Number(sessionState.session.config?.workspaceID || 0)
  if (!Number.isInteger(scopedWorkspaceID) || scopedWorkspaceID <= 0) {
    return null
  }

  const workspaceState = useWorkspaceStore.getState()
  const scopedWorkspace = workspaceState.workspaces.find((workspace) => workspace.id === scopedWorkspaceID)
  if (!scopedWorkspace) {
    return null
  }

  const layoutState = useLayoutStore.getState()
  const liveAgentsFromLayout: AgentSessionNode[] = []
  const paneIDsInOrder = layoutState.paneOrder.length > 0
    ? layoutState.paneOrder
    : Object.keys(layoutState.panes)
  paneIDsInOrder.forEach((paneID, index) => {
    const pane = layoutState.panes[paneID]
    if (!pane || pane.workspaceID !== scopedWorkspaceID || pane.type !== 'terminal') {
      return
    }

    const agentID = Number(pane.agentDBID)
    if (!Number.isInteger(agentID) || agentID <= 0) {
      return
    }

    const config = (pane.config || {}) as Record<string, unknown>
    liveAgentsFromLayout.push({
      id: agentID,
      workspaceId: scopedWorkspaceID,
      name: typeof pane.title === 'string' && pane.title.trim().length > 0
        ? pane.title
        : `Terminal ${agentID}`,
      type: 'terminal',
      shell: typeof config.shell === 'string' ? config.shell : '',
      cwd: typeof config.cwd === 'string' ? config.cwd : '',
      useDocker: Boolean(config.useDocker),
      sessionId: typeof pane.sessionID === 'string' && pane.sessionID.length > 0
        ? pane.sessionID
        : undefined,
      status: typeof pane.status === 'string' ? pane.status : 'idle',
      sortOrder: index,
      isMinimized: Boolean(pane.isMinimized),
    })
  })

  const persistedAgents = normalizeSharedWorkspaceAgents(scopedWorkspace.agents, scopedWorkspaceID)
  const mergedByID = new Map<number, AgentSessionNode>()
  for (const agent of persistedAgents) {
    mergedByID.set(agent.id, agent)
  }
  for (const liveAgent of liveAgentsFromLayout) {
    const existing = mergedByID.get(liveAgent.id)
    if (!existing) {
      mergedByID.set(liveAgent.id, liveAgent)
      continue
    }
    mergedByID.set(liveAgent.id, {
      ...existing,
      ...liveAgent,
      // Mantém dados persistidos quando o pane não fornece valor útil.
      shell: liveAgent.shell || existing.shell,
      cwd: liveAgent.cwd || existing.cwd,
      sessionId: liveAgent.sessionId || existing.sessionId,
    })
  }
  const mergedAgents = Array.from(mergedByID.values()).sort((a, b) => {
    if (a.sortOrder !== b.sortOrder) {
      return a.sortOrder - b.sortOrder
    }
    return a.id - b.id
  })

  const workspaceName = scopedWorkspace.name?.trim() || sessionState.session.config?.workspaceName || `Workspace ${scopedWorkspaceID}`
  return {
    workspaceID: scopedWorkspaceID,
    workspace: {
      id: scopedWorkspaceID,
      userId: scopedWorkspace.userId || 'host',
      name: workspaceName,
      path: scopedWorkspace.path || '',
      gitRemote: scopedWorkspace.gitRemote,
      owner: scopedWorkspace.owner,
      repo: scopedWorkspace.repo,
      color: scopedWorkspace.color,
      isActive: true,
      lastOpenedAt: scopedWorkspace.lastOpenedAt,
      agents: mergedAgents,
    },
  }
}

function syncScopedWorkspaceToGuests(targetUserID?: string) {
  if (!runtimeState.p2p || !runtimeState.p2pIsHost || !runtimeState.p2p.isConnected) {
    return
  }

  const snapshot = buildScopedHostWorkspaceSnapshot()
  if (!snapshot) {
    return
  }

  runtimeState.p2p.sendControlMessage('workspace_scope_sync', snapshot, targetUserID)
}

interface GuestWorkspaceBinding {
  sessionID: string
  hostWorkspaceID: number
  localWorkspaceID: number
  workspaceName: string
}

const guestWorkspaceBindingsBySession = new Map<string, GuestWorkspaceBinding>()
const guestWorkspaceSyncInFlight = new Set<string>()
const guestWorkspaceResyncRequested = new Set<string>()

function normalizeWorkspaceName(name: string | undefined, fallbackWorkspaceID: number): string {
  const trimmed = typeof name === 'string' ? name.trim() : ''
  if (trimmed.length > 0) {
    return trimmed
  }
  return `Workspace ${fallbackWorkspaceID}`
}

function resolveGuestWorkspaceBinding(sessionID: string, hostWorkspaceID: number): GuestWorkspaceBinding | null {
  const binding = guestWorkspaceBindingsBySession.get(sessionID)
  if (!binding) {
    return null
  }
  if (binding.hostWorkspaceID !== hostWorkspaceID) {
    return null
  }
  return binding
}

function pickWorkspaceSnapshot(
  primary?: WorkspaceNode,
  secondary?: WorkspaceNode,
): WorkspaceNode | undefined {
  if (primary && primary.agents.length > 0) {
    return primary
  }
  if (secondary && secondary.agents.length > 0) {
    return secondary
  }
  return primary ?? secondary
}

function applyGuestWorkspaceScopeSnapshot(
  sessionID: string,
  hostWorkspaceID: number,
  workspaceName: string,
  incomingWorkspace?: WorkspaceNode,
) {
  const binding = resolveGuestWorkspaceBinding(sessionID, hostWorkspaceID)
  const scopedWorkspaceID = binding?.localWorkspaceID || hostWorkspaceID
  const workspaceState = useWorkspaceStore.getState()
  const localWorkspace = workspaceState.workspaces.find((workspace) => workspace.id === scopedWorkspaceID)
  const hostWorkspace = scopedWorkspaceID !== hostWorkspaceID
    ? workspaceState.workspaces.find((workspace) => workspace.id === hostWorkspaceID)
    : undefined
  const existingWorkspace = pickWorkspaceSnapshot(hostWorkspace, localWorkspace)
  const snapshotWorkspace = pickWorkspaceSnapshot(incomingWorkspace, existingWorkspace)

  const mappedAgents = (snapshotWorkspace?.agents ?? []).map((agent) => ({
    ...agent,
    workspaceId: scopedWorkspaceID,
    // Colaboração: não propagar minimização visual do host para o guest.
    isMinimized: false,
  }))

  useWorkspaceStore.getState().applySessionWorkspaceSnapshot({
    id: scopedWorkspaceID,
    userId: snapshotWorkspace?.userId ?? 'host',
    name: workspaceName,
    path: snapshotWorkspace?.path ?? '',
    gitRemote: snapshotWorkspace?.gitRemote,
    owner: snapshotWorkspace?.owner,
    repo: snapshotWorkspace?.repo,
    color: snapshotWorkspace?.color,
    isActive: true,
    lastOpenedAt: snapshotWorkspace?.lastOpenedAt,
    agents: mappedAgents,
  })
}

function syncGuestWorkspaceRecord(sessionID: string, hostWorkspaceID: number, workspaceName: string) {
  if (!window.go?.main?.App?.SyncGuestWorkspace) {
    return
  }

  const existingBinding = resolveGuestWorkspaceBinding(sessionID, hostWorkspaceID)
  if (existingBinding && existingBinding.workspaceName === workspaceName) {
    return
  }

  const syncKey = `${sessionID}:${hostWorkspaceID}:${workspaceName.toLowerCase()}`
  if (guestWorkspaceSyncInFlight.has(syncKey)) {
    return
  }

  guestWorkspaceSyncInFlight.add(syncKey)
  window.go.main.App.SyncGuestWorkspace(workspaceName)
    .then((workspace) => {
      const localWorkspaceID = Number(workspace?.id || 0)
      if (!Number.isInteger(localWorkspaceID) || localWorkspaceID <= 0) {
        return
      }

      guestWorkspaceBindingsBySession.set(sessionID, {
        sessionID,
        hostWorkspaceID,
        localWorkspaceID,
        workspaceName,
      })

      const workspaceState = useWorkspaceStore.getState()
      const hostSnapshot = workspaceState.workspaces.find((workspace) => workspace.id === hostWorkspaceID)
      const localSnapshot = workspaceState.workspaces.find((workspace) => workspace.id === localWorkspaceID)
      const preservedSnapshot = pickWorkspaceSnapshot(hostSnapshot, localSnapshot)
      if (preservedSnapshot && preservedSnapshot.agents.length > 0) {
        applyGuestWorkspaceScopeSnapshot(sessionID, hostWorkspaceID, workspaceName, preservedSnapshot)
      }
    })
    .catch((err) => {
      console.warn('[Session] Failed to sync shared workspace to local db:', err)
    })
    .finally(() => {
      guestWorkspaceSyncInFlight.delete(syncKey)
    })
}

function ensureGuestWorkspaceScope(sessionData: Session | null | undefined) {
  if (!sessionData?.id || !sessionData.config) {
    return
  }

  const hostWorkspaceID = Number(sessionData.config.workspaceID || 0)
  if (!Number.isInteger(hostWorkspaceID) || hostWorkspaceID <= 0) {
    return
  }

  const workspaceName = normalizeWorkspaceName(sessionData.config.workspaceName, hostWorkspaceID)
  const binding = resolveGuestWorkspaceBinding(sessionData.id, hostWorkspaceID)
  const workspaceState = useWorkspaceStore.getState()
  const localSnapshot = binding
    ? workspaceState.workspaces.find((workspace) => workspace.id === binding.localWorkspaceID)
    : undefined
  const hostSnapshot = workspaceState.workspaces.find((workspace) => workspace.id === hostWorkspaceID)
  const preservedSnapshot = pickWorkspaceSnapshot(localSnapshot, hostSnapshot)
  if (preservedSnapshot && preservedSnapshot.agents.length > 0) {
    applyGuestWorkspaceScopeSnapshot(sessionData.id, hostWorkspaceID, workspaceName, preservedSnapshot)
  }
  syncGuestWorkspaceRecord(sessionData.id, hostWorkspaceID, workspaceName)
}

function requestGuestWorkspaceScopeSync(sessionID: string, workspaceID: number) {
  const hasHydratedScope = () => {
    const workspaceState = useWorkspaceStore.getState()
    const binding = resolveGuestWorkspaceBinding(sessionID, workspaceID)
    const hydratedLocal = binding
      ? workspaceState.workspaces.find((workspace) => workspace.id === binding.localWorkspaceID)
      : undefined
    const hydratedHost = workspaceState.workspaces.find((workspace) => workspace.id === workspaceID)

    return Boolean(
      workspaceState.isSessionWorkspaceScoped &&
      (
        (hydratedLocal && hydratedLocal.agents.length > 0) ||
        (hydratedHost && hydratedHost.agents.length > 0)
      ),
    )
  }

  const sendRequest = () => {
    if (
      !runtimeState.p2p ||
      !runtimeState.p2p.isConnected ||
      runtimeState.p2pIsHost ||
      runtimeState.p2pSessionID !== sessionID
    ) {
      return
    }

    runtimeState.p2p.sendControlMessage('workspace_scope_request', { workspaceID })
  }

  sendRequest()

  // Retries defensivos para casos em que o canal de controle abre após o estado "connected".
  const retryDelaysMs = [300, 700, 1200, 2000, 3200, 5000]
  retryDelaysMs.forEach((delay) => {
    window.setTimeout(() => {
      if (hasHydratedScope()) {
        return
      }
      sendRequest()
    }, delay)
  })
}

function applyGuestWorkspaceScopeSync(sessionID: string, payload: WorkspaceScopeSyncPayload) {
  if (!sessionID) {
    return
  }
  const workspaceName = normalizeWorkspaceName(payload.workspace.name, payload.workspaceID)
  applyGuestWorkspaceScopeSnapshot(sessionID, payload.workspaceID, workspaceName, payload.workspace)
  syncGuestWorkspaceRecord(sessionID, payload.workspaceID, workspaceName)

  if (
    !guestWorkspaceResyncRequested.has(sessionID) &&
    runtimeState.p2p &&
    runtimeState.p2p.isConnected &&
    !runtimeState.p2pIsHost &&
    runtimeState.p2pSessionID === sessionID
  ) {
    guestWorkspaceResyncRequested.add(sessionID)
    window.setTimeout(() => {
      if (
        !runtimeState.p2p ||
        !runtimeState.p2p.isConnected ||
        runtimeState.p2pIsHost ||
        runtimeState.p2pSessionID !== sessionID
      ) {
        return
      }
      const workspaceID = Number(useSessionStore.getState().session?.config?.workspaceID || payload.workspaceID || 0)
      runtimeState.p2p.sendControlMessage('workspace_scope_request', { workspaceID })
    }, 350)
  }
}

function clearGuestWorkspaceScope(sessionID?: string) {
  if (sessionID) {
    guestWorkspaceBindingsBySession.delete(sessionID)
    guestWorkspaceResyncRequested.delete(sessionID)
    for (const key of Array.from(guestWorkspaceSyncInFlight)) {
      if (key.startsWith(`${sessionID}:`)) {
        guestWorkspaceSyncInFlight.delete(key)
      }
    }
  }
  useWorkspaceStore.getState()
    .clearSessionWorkspaceSnapshot()
    .catch((err) => {
      console.warn('[Session] Failed to restore local workspace view:', err)
    })
}

function resolvePaneMetaBySessionID(sessionID: string): { id: string; title: string; agentDBID?: number } | null {
  if (!sessionID) return null
  const panes = useLayoutStore.getState().panes
  const pane = Object.values(panes).find((item) => item.type === 'terminal' && item.sessionID === sessionID)
  if (!pane) return null
  return {
    id: pane.id,
    title: pane.title,
    agentDBID: pane.agentDBID,
  }
}

function resolveHostTargetSessionID(payload: TerminalPeerPayload): string | null {
  const panes = useLayoutStore.getState().panes
  const terminalPanes = Object.values(panes).filter((pane) => pane.type === 'terminal')

  if (payload.sessionID) {
    const bySession = terminalPanes.find((pane) => pane.sessionID === payload.sessionID)
    if (bySession?.sessionID) return bySession.sessionID
  }

  if (typeof payload.agentDBID === 'number') {
    const byAgent = terminalPanes.find((pane) => pane.agentDBID === payload.agentDBID)
    if (byAgent?.sessionID) return byAgent.sessionID
  }

  if (payload.paneID) {
    const byID = panes[payload.paneID]
    if (byID?.type === 'terminal' && byID.sessionID) return byID.sessionID
  }

  if (payload.paneTitle) {
    const byTitle = terminalPanes.find((pane) => pane.title === payload.paneTitle)
    if (byTitle?.sessionID) return byTitle.sessionID
  }

  const activePaneID = useLayoutStore.getState().activePaneId
  if (activePaneID) {
    const activePane = panes[activePaneID]
    if (activePane?.type === 'terminal' && activePane.sessionID) {
      return activePane.sessionID
    }
  }

  return null
}

function isTerminalSessionInWorkspaceScope(sessionID: string, workspaceID: number): boolean {
  if (!sessionID) {
    return false
  }
  if (!Number.isInteger(workspaceID) || workspaceID <= 0) {
    return true
  }

  const panes = useLayoutStore.getState().panes
  const paneMatch = Object.values(panes).find(
    (pane) => pane.type === 'terminal' && pane.sessionID === sessionID,
  )
  if (paneMatch?.workspaceID) {
    return paneMatch.workspaceID === workspaceID
  }

  const workspaceState = useWorkspaceStore.getState()
  const scopedWorkspace = workspaceState.workspaces.find((workspace) => workspace.id === workspaceID)
  if (!scopedWorkspace) {
    return false
  }

  return scopedWorkspace.agents.some((agent) => agent.sessionId === sessionID)
}

function normalizeSessionCode(code: string): string {
  const cleaned = code.trim().toUpperCase().replace(/[^A-Z0-9]/g, '')
  if (cleaned.length <= 3) {
    return cleaned
  }
  return `${cleaned.slice(0, 3)}-${cleaned.slice(3, 5)}`
}

function resolveApprovalDeadline(approvalExpiresAt?: string): number {
  if (!approvalExpiresAt) {
    return Date.now() + APPROVAL_TIMEOUT_MS
  }
  const parsed = Date.parse(approvalExpiresAt)
  if (Number.isNaN(parsed)) {
    return Date.now() + APPROVAL_TIMEOUT_MS
  }
  return parsed
}

function resetGuestWaitingState(reason: string | null) {
  const state = useSessionStore.getState()
  state.setWaitingApproval(false)
  state.setJoinResult(null)
  state.setActiveGuestUserID(null)
  if (runtimeState.joinPreviousRole === 'host') {
    state.setRole('host')
  } else {
    state.setRole('none')
  }
  runtimeState.joinPreviousRole = 'none'
  if (reason) {
    state.setError(reason)
  } else {
    state.setError(null)
  }
  destroyRuntimeP2P()
}

function resetGuestActiveSession(reason: string | null) {
  const state = useSessionStore.getState()
  const activeSessionID = state.session?.id
  clearGuestWorkspaceScope(activeSessionID)
  state.setSession(null)
  state.setPendingGuests([])
  resetGuestWaitingState(reason)
}

function getCurrentIdentity() {
  const authUser = useAuthStore.getState().user
  const store = useSessionStore.getState()
  const guestUserID = store.activeGuestUserID || store.joinResult?.guestUserID || ''
  const userID = authUser?.id ||
    (store.role === 'guest' ? guestUserID : '') ||
    (store.role === 'host' && store.session?.hostUserID ? store.session.hostUserID : 'anonymous')
  const userName = authUser?.name || (store.role === 'host' ? 'Host' : 'Guest')
  const userColor = pickCursorColor(userID)
  return { userID, userName, userColor }
}

function destroyRuntimeP2P() {
  if (runtimeState.p2p) {
    runtimeState.p2p.destroy()
    runtimeState.p2p = null
  }
  runtimeState.p2pSessionID = ''
  runtimeState.p2pUserID = ''
  runtimeState.p2pIsHost = false
  useSessionStore.getState().setP2PConnected(false)
}

function setupRuntimeListenersOnce() {
  if (runtimeState.listenersInitialized) {
    return
  }
  runtimeState.listenersInitialized = true

  if (window.runtime) {
    window.runtime.EventsOn('session:guest_request', (data: unknown) => {
      useSessionStore.getState().addPendingGuest(data as GuestRequest)
    })

    window.runtime.EventsOn('session:guest_approved', (data: unknown) => {
      const payload = data as { sessionID: string; guestUserID: string; session: Session }
      const state = useSessionStore.getState()
      if (state.role === 'host') {
        state.setSession(payload.session)
        return
      }
      if (state.role !== 'guest' || !state.isWaitingApproval) {
        return
      }
      const expectedGuestID = state.activeGuestUserID || state.joinResult?.guestUserID
      if (expectedGuestID && payload.guestUserID !== expectedGuestID) {
        return
      }
      state.setSession(payload.session)
      state.setWaitingApproval(false)
      state.setActiveGuestUserID(payload.guestUserID)
      state.setJoinResult(null)
      state.setError(null)
      runtimeState.joinPreviousRole = 'none'
    })

    window.runtime.EventsOn('session:guest_rejected', (data: unknown) => {
      const payload = data as { guestUserID?: string }
      const state = useSessionStore.getState()
      if (state.role !== 'guest' || !state.isWaitingApproval) {
        return
      }
      const expectedGuestID = state.activeGuestUserID || state.joinResult?.guestUserID
      if (expectedGuestID && payload?.guestUserID && payload.guestUserID !== expectedGuestID) {
        return
      }
      resetGuestWaitingState('Sua solicitação foi rejeitada pelo host.')
    })

    window.runtime.EventsOn('session:guest_expired', (data: unknown) => {
      const payload = data as { guestUserID?: string }
      const state = useSessionStore.getState()
      if (state.role !== 'guest' || !state.isWaitingApproval) {
        return
      }
      const expectedGuestID = state.activeGuestUserID || state.joinResult?.guestUserID
      if (expectedGuestID && payload?.guestUserID && payload.guestUserID !== expectedGuestID) {
        return
      }
      resetGuestWaitingState(APPROVAL_TIMEOUT_ERROR)
    })

    window.runtime.EventsOn('session:created', (data: unknown) => {
      const store = useSessionStore.getState()
      store.setSession(data as Session)
      store.setRestoredSession(false)
    })

    window.runtime.EventsOn('session:ended', () => {
      const current = useSessionStore.getState()
      if (current.role === 'guest') {
        resetGuestActiveSession(SESSION_ENDED_ERROR)
        return
      }
      useSessionStore.getState().reset()
      destroyRuntimeP2P()
    })

    window.runtime.EventsOn('session:permission_changed', (data: unknown) => {
      const payload = data as { guestUserID: string; permission: string }
      useSessionStore.getState().updateGuest(payload.guestUserID, {
        permission: payload.permission as 'read_only' | 'read_write',
      })

      const current = useSessionStore.getState()
      if (current.role === 'host' && runtimeState.p2p?.isConnected) {
        runtimeState.p2p.sendControlMessage('permission_change', {
          guestUserID: payload.guestUserID,
          permission: payload.permission,
        }, payload.guestUserID)
      }
    })

    window.runtime.EventsOn('session:permission_revoked', (data: unknown) => {
      const payload = data as { guestUserID: string }
      const identity = getCurrentIdentity()
      if (payload.guestUserID === identity.userID) {
        useSessionStore.getState().setError('Seu acesso de escrita foi revogado pelo host.')
      }
    })

    window.runtime.EventsOn('session:guest_kicked', (data: unknown) => {
      const payload = data as { guestUserID: string }
      const store = useSessionStore.getState()
      store.updateGuest(payload.guestUserID, { status: 'rejected' })

      const identity = getCurrentIdentity()
      if (payload.guestUserID === identity.userID) {
        store.setError('Você foi removido da sessão pelo host.')
      }
    })

    window.runtime.EventsOn('github:prs:updated', (data: unknown) => {
      const current = useSessionStore.getState()
      if (current.role === 'host' && runtimeState.p2p?.isConnected) {
        runtimeState.p2p.broadcastGitHubState(data)
      }
    })

    window.runtime.EventsOn('terminal:output', (msg: { sessionID?: string; data?: string }) => {
      const current = useSessionStore.getState()
      if (current.role !== 'host' || !runtimeState.p2p?.isConnected) {
        return
      }
      if (!msg?.sessionID || typeof msg.data !== 'string' || msg.data.length === 0) {
        return
      }

      const scopedWorkspaceID = Number(current.session?.config?.workspaceID || 0)
      if (!isTerminalSessionInWorkspaceScope(msg.sessionID, scopedWorkspaceID)) {
        return
      }

      const paneMeta = resolvePaneMetaBySessionID(msg.sessionID)
      runtimeState.p2p.send(DATA_CHANNELS.TERMINAL_IO, 'pty_output', {
        sessionID: msg.sessionID,
        paneID: paneMeta?.id,
        paneTitle: paneMeta?.title,
        agentDBID: paneMeta?.agentDBID,
        data: decodeBase64UTF8(msg.data),
      })
    })

    window.runtime.EventsOn('session:docker_fallback', (data: unknown) => {
      const payload = data as { reason?: string }
      useSessionStore.getState().setError(payload.reason ?? 'Docker indisponível. Sessão iniciou em Live Share.')
    })
  }

  window.addEventListener('scrollsync:outgoing', (e: Event) => {
    const event = (e as CustomEvent<ScrollSyncEvent>).detail
    if (runtimeState.p2p?.isConnected) {
      runtimeState.p2p.sendScrollSyncEvent(event)
    }
  })

  window.addEventListener('session:cursor-awareness:local', (e: Event) => {
    if (!runtimeState.p2p?.isConnected) return
    const detail = (e as CustomEvent<{
      column: number
      row: number
      isTyping: boolean
      typingPreview?: string
      paneID?: string
      paneTitle?: string
    }>).detail
    const identity = getCurrentIdentity()
    runtimeState.p2p.sendCursorAwareness({
      userID: identity.userID,
      userName: identity.userName,
      userColor: identity.userColor,
      column: detail.column,
      row: detail.row,
      isTyping: detail.isTyping,
      typingPreview: typeof detail.typingPreview === 'string' ? detail.typingPreview : undefined,
      paneID: typeof detail.paneID === 'string' ? detail.paneID : undefined,
      paneTitle: typeof detail.paneTitle === 'string' ? detail.paneTitle : undefined,
      updatedAt: Date.now(),
    })
  })

  window.addEventListener('session:shared-input:append', (e: Event) => {
    const detail = (e as CustomEvent<{ input: string }>).detail
    if (!runtimeState.p2p?.isConnected || typeof detail.input !== 'string') return
    runtimeState.p2p.appendSharedInput(detail.input)
  })

  window.addEventListener('session:terminal-input:guest', (e: Event) => {
    const current = useSessionStore.getState()
    if (current.role !== 'guest' || !runtimeState.p2p?.isConnected) {
      return
    }

    const detail = (e as CustomEvent<TerminalPeerPayload>).detail
    if (!detail || typeof detail.data !== 'string' || detail.data.length === 0) {
      return
    }

    runtimeState.p2p.send(DATA_CHANNELS.TERMINAL_IO, 'terminal_input', {
      data: detail.data,
      sessionID: detail.sessionID,
      paneID: detail.paneID,
      paneTitle: detail.paneTitle,
      agentDBID: detail.agentDBID,
    })
  })

  window.addEventListener('session:host-workspace:changed', (e: Event) => {
    const current = useSessionStore.getState()
    if (current.role !== 'host' || !runtimeState.p2p?.isConnected) {
      return
    }

    const scopedWorkspaceID = Number(current.session?.config?.workspaceID || 0)
    if (!Number.isInteger(scopedWorkspaceID) || scopedWorkspaceID <= 0) {
      return
    }

    const detail = (e as CustomEvent<{ workspaceID?: number }>).detail
    const changedWorkspaceID = Number(detail?.workspaceID || 0)
    if (changedWorkspaceID !== scopedWorkspaceID) {
      return
    }

    syncScopedWorkspaceToGuests()
  })
}

export function useSession() {
  const store = useSessionStore()
  const authUser = useAuthStore((s) => s.user)

  const [auditLogs, setAuditLogs] = useState<AuditLog[]>([])
  const [isAuditLoading, setIsAuditLoading] = useState(false)

  const currentUserID = useMemo(() => {
    if (authUser?.id) return authUser.id
    if (store.role === 'guest' && store.activeGuestUserID) return store.activeGuestUserID
    if (store.role === 'guest' && store.joinResult?.guestUserID) return store.joinResult.guestUserID
    if (store.role === 'host' && store.session?.hostUserID) return store.session.hostUserID
    return 'anonymous'
  }, [authUser?.id, store.activeGuestUserID, store.joinResult?.guestUserID, store.role, store.session?.hostUserID])

  // Inicializa listeners globais de sessão apenas uma vez por app.
  useEffect(() => {
    runtimeState.listenerConsumers += 1
    setupRuntimeListenersOnce()
    return () => {
      runtimeState.listenerConsumers = Math.max(0, runtimeState.listenerConsumers - 1)
    }
  }, [])

  // Auto-start P2P quando houver sessão ativa
  useEffect(() => {
    if (!store.session || store.role === 'none') {
      return
    }

    const isHost = store.role === 'host'
    const guestUserID = store.activeGuestUserID || store.joinResult?.guestUserID
    const p2pUserID =
      !isHost && guestUserID ? guestUserID : currentUserID

    if (
      runtimeState.p2p &&
      runtimeState.p2pSessionID === store.session.id &&
      runtimeState.p2pUserID === p2pUserID &&
      runtimeState.p2pIsHost === isHost
    ) {
      return
    }
    if (runtimeState.p2pConnectInFlight) {
      return
    }

    runtimeState.p2pConnectInFlight = true
    startP2P(store.session.id, p2pUserID, isHost)
      .catch((err: unknown) => {
        store.setError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        runtimeState.p2pConnectInFlight = false
      })
  }, [currentUserID, store.activeGuestUserID, store.joinResult?.guestUserID, store.role, store.session?.id]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (store.role === 'guest' && store.session && store.isP2PConnected) {
      ensureGuestWorkspaceScope(store.session)
    }
  }, [store.isP2PConnected, store.role, store.session])

  useEffect(() => {
    if (store.role !== 'host' || !store.session?.id || !store.isP2PConnected) {
      return
    }

    const scopedWorkspaceID = Number(store.session.config?.workspaceID || 0)
    if (!Number.isInteger(scopedWorkspaceID) || scopedWorkspaceID <= 0) {
      return
    }

    let lastSnapshotSignature = ''

    const syncIfChanged = () => {
      const snapshot = buildScopedHostWorkspaceSnapshot()
      if (!snapshot || snapshot.workspaceID !== scopedWorkspaceID) {
        return
      }

      const signature = JSON.stringify(snapshot)
      if (signature === lastSnapshotSignature) {
        return
      }

      lastSnapshotSignature = signature
      syncScopedWorkspaceToGuests()
    }

    syncIfChanged()
    const unsubscribe = useWorkspaceStore.subscribe(() => {
      syncIfChanged()
    })

    return () => {
      unsubscribe()
    }
  }, [store.isP2PConnected, store.role, store.session?.config?.workspaceID, store.session?.id])

  // Guest: polling de aprovação para cenários multi-instância
  useEffect(() => {
    if (store.role !== 'guest' || !store.isWaitingApproval || !store.joinResult?.sessionID) {
      return
    }

    const sessionID = store.joinResult.sessionID
    const guestUserID = store.joinResult.guestUserID || store.activeGuestUserID || currentUserID
    const approvalDeadline = resolveApprovalDeadline(store.joinResult.approvalExpiresAt)
    let cancelled = false

    const timeoutIfNeeded = () => {
      if (Date.now() < approvalDeadline) {
        return false
      }
      resetGuestWaitingState(APPROVAL_TIMEOUT_ERROR)
      return true
    }

    const pollApproval = async () => {
      if (timeoutIfNeeded()) {
        return
      }

      try {
        const latestSession = await window.go!.main.App.SessionGetSession(sessionID)
        if (!latestSession || cancelled) {
          return
        }

        const guest = latestSession.guests?.find((g) => g.userID === guestUserID)
        if (!guest) {
          timeoutIfNeeded()
          return
        }

        if (guest.status === 'approved' || guest.status === 'connected') {
          store.setSession(latestSession)
          store.setWaitingApproval(false)
          store.setActiveGuestUserID(guestUserID)
          store.setJoinResult(null)
          store.setError(null)
          runtimeState.joinPreviousRole = 'none'
          return
        }

        if (guest.status === 'rejected') {
          resetGuestWaitingState('Sua solicitação foi rejeitada pelo host.')
          return
        }

        if (guest.status === 'expired') {
          resetGuestWaitingState(APPROVAL_TIMEOUT_ERROR)
          return
        }

        if (guest.status === 'pending') {
          const joinedAt = Date.parse(guest.joinedAt)
          const refreshedDeadline = Number.isNaN(joinedAt)
            ? new Date(approvalDeadline).toISOString()
            : new Date(joinedAt + APPROVAL_TIMEOUT_MS).toISOString()
          const latestJoinResult = useSessionStore.getState().joinResult
          if (latestJoinResult?.sessionID === sessionID) {
            useSessionStore.getState().setJoinResult({
              ...latestJoinResult,
              sessionCode: latestSession.code || latestJoinResult.sessionCode,
              approvalExpiresAt: refreshedDeadline,
            })
          }
        }
      } catch {
        // Ignorar erros transitórios de rede/gateway durante polling.
      }
    }

    void pollApproval()
    const timer = window.setInterval(() => {
      void pollApproval()
    }, 1500)

    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [
    currentUserID,
    store.activeGuestUserID,
    store,
    store.isWaitingApproval,
    store.joinResult?.approvalExpiresAt,
    store.joinResult?.guestUserID,
    store.joinResult?.sessionID,
    store.role,
  ])

  // Guest: expira localmente a espera mesmo sem retorno do host.
  useEffect(() => {
    if (store.role !== 'guest' || !store.isWaitingApproval) {
      return
    }

    const approvalDeadline = resolveApprovalDeadline(store.joinResult?.approvalExpiresAt)
    const enforceTimeout = () => {
      if (Date.now() < approvalDeadline) {
        return
      }
      const latest = useSessionStore.getState()
      if (latest.role === 'guest' && latest.isWaitingApproval) {
        resetGuestWaitingState(APPROVAL_TIMEOUT_ERROR)
      }
    }

    enforceTimeout()
    const timer = window.setInterval(enforceTimeout, 1000)

    return () => {
      window.clearInterval(timer)
    }
  }, [store.isWaitingApproval, store.joinResult?.approvalExpiresAt, store.role])

  // === Host Actions ===

  /** Cria uma nova sessão como Host */
  const createSession = useCallback(
    async (opts?: { maxGuests?: number; mode?: string; allowAnonymous?: boolean; workspaceID?: number }) => {
      store.setLoading(true)
      store.setError(null)

      try {
        const session = await window.go!.main.App.SessionCreate(
          opts?.maxGuests ?? 10,
          opts?.mode ?? 'liveshare',
          opts?.allowAnonymous ?? false,
          opts?.workspaceID ?? 0,
        )

        store.setSession(session)
        store.setRole('host')
        store.setRestoredSession(false)

        // Buscar ICE servers
        const iceServers = await window.go!.main.App.SessionGetICEServers()
        store.setICEServers(iceServers)

        await loadAuditLogs(session.id)
        return session
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err)
        if (message.includes('host already has an active session')) {
          try {
            const activeSession = await window.go!.main.App.SessionGetActive()
            if (activeSession) {
              store.setSession(activeSession)
              store.setRole('host')
              store.setRestoredSession(true)
              store.setError(null)
              if (activeSession.status === 'waiting') {
                const pending = await window.go!.main.App.SessionListPendingGuests(activeSession.id)
                store.setPendingGuests(Array.isArray(pending) ? pending : [])
              }
              await loadAuditLogs(activeSession.id)
              return activeSession
            }
          } catch {
            // fallback para exibir erro original abaixo
          }
        }
        store.setError(message)
        throw err
      } finally {
        store.setLoading(false)
      }
    },
    [store]
  )

  /** Aprova um guest na sessão */
  const approveGuest = useCallback(
    async (guestUserID: string) => {
      if (!store.session) return

      try {
        await window.go!.main.App.SessionApproveGuest(store.session.id, guestUserID)
        store.removePendingGuest(guestUserID)
        store.updateGuest(guestUserID, { status: 'approved' })

        // Criar SDP Offer para o guest aprovado
        if (runtimeState.p2p) {
          await runtimeState.p2p.createOffer(guestUserID)
        }

        await loadAuditLogs(store.session.id)
      } catch (err) {
        store.setError(err instanceof Error ? err.message : String(err))
      }
    },
    [store]
  )

  /** Rejeita um guest */
  const rejectGuest = useCallback(
    async (guestUserID: string) => {
      if (!store.session) return

      try {
        await window.go!.main.App.SessionRejectGuest(store.session.id, guestUserID)
        store.removePendingGuest(guestUserID)
        await loadAuditLogs(store.session.id)
      } catch (err) {
        store.setError(err instanceof Error ? err.message : String(err))
      }
    },
    [store]
  )

  /** Encerra a sessão */
  const endSession = useCallback(async () => {
    if (!store.session) return

    try {
      await window.go!.main.App.SessionEnd(store.session.id)
      destroyRuntimeP2P()
      store.reset()
      setAuditLogs([])
    } catch (err) {
      store.setError(err instanceof Error ? err.message : String(err))
    }
  }, [store])

  /** Altera permissão de um guest */
  const setGuestPermission = useCallback(
    async (guestUserID: string, permission: 'read_only' | 'read_write') => {
      if (!store.session) return

      try {
        await window.go!.main.App.SessionSetGuestPermission(
          store.session.id,
          guestUserID,
          permission
        )
        await loadAuditLogs(store.session.id)
      } catch (err) {
        store.setError(err instanceof Error ? err.message : String(err))
      }
    },
    [store]
  )

  /** Remove um guest da sessão */
  const kickGuest = useCallback(
    async (guestUserID: string) => {
      if (!store.session) return

      try {
        await window.go!.main.App.SessionKickGuest(store.session.id, guestUserID)
        await loadAuditLogs(store.session.id)
      } catch (err) {
        store.setError(err instanceof Error ? err.message : String(err))
      }
    },
    [store]
  )

  /** Reinicia ambiente Docker da sessão */
  const restartEnvironment = useCallback(async () => {
    if (!store.session) return

    try {
      await window.go!.main.App.SessionRestartEnvironment(store.session.id)
      await loadAuditLogs(store.session.id)
    } catch (err) {
      store.setError(err instanceof Error ? err.message : String(err))
    }
  }, [store])

  /** Carrega eventos de auditoria da sessão */
  const loadAuditLogs = useCallback(async (sessionID?: string) => {
    const targetSessionID = sessionID ?? store.session?.id
    if (!targetSessionID) {
      setAuditLogs([])
      return
    }

    setIsAuditLoading(true)
    try {
      const logs = await window.go!.main.App.SessionGetAuditLogs(targetSessionID, 100)
      setAuditLogs(logs ?? [])
    } catch (err) {
      store.setError(err instanceof Error ? err.message : String(err))
    } finally {
      setIsAuditLoading(false)
    }
  }, [store])

  // Restaura sessão ativa no frontend após restart (TTL + restore backend).
  useEffect(() => {
    if (runtimeState.activeSessionHydrated) {
      return
    }
    runtimeState.activeSessionHydrated = true

    let cancelled = false

    const hydrateActiveSession = async () => {
      const current = useSessionStore.getState()
      if (current.session || current.role !== 'none' || current.isWaitingApproval) {
        return
      }

      try {
        const activeSession = await window.go!.main.App.SessionGetActive()
        if (!activeSession || cancelled) {
          return
        }

        const latest = useSessionStore.getState()
        if (latest.session || latest.isWaitingApproval) {
          return
        }

        latest.setSession(activeSession)
        latest.setRole('host')
        latest.setRestoredSession(true)
        latest.setError(null)

        if (activeSession.status === 'waiting') {
          const pending = await window.go!.main.App.SessionListPendingGuests(activeSession.id)
          if (!cancelled) {
            useSessionStore.getState().setPendingGuests(Array.isArray(pending) ? pending : [])
          }
        }
      } catch {
        // Sem sessão ativa é comportamento esperado.
      }
    }

    void hydrateActiveSession()

    return () => {
      cancelled = true
    }
  }, [])

  // === Guest Actions ===

  /** Entra numa sessão como Guest usando o código */
  const joinSession = useCallback(
    async (code: string, name?: string, email?: string) => {
      store.setLoading(true)
      store.setError(null)

      try {
        runtimeState.joinPreviousRole = store.role
        const result = await window.go!.main.App.SessionJoin(code, name ?? '', email ?? '')
        const normalizedCode = normalizeSessionCode(result.sessionCode || code)
        const normalizedJoinResult = {
          ...result,
          sessionCode: normalizedCode,
          approvalExpiresAt: new Date(resolveApprovalDeadline(result.approvalExpiresAt)).toISOString(),
        }
        store.setJoinResult(normalizedJoinResult)
        store.setActiveGuestUserID(normalizedJoinResult.guestUserID || null)
        // Join sempre coloca a instância no contexto de guest.
        store.setRole('guest')
        store.setWaitingApproval(true)

        // Buscar ICE servers
        const iceServers = await window.go!.main.App.SessionGetICEServers()
        store.setICEServers(iceServers)

        return normalizedJoinResult
      } catch (err) {
        store.setError(err instanceof Error ? err.message : String(err))
        throw err
      } finally {
        store.setLoading(false)
      }
    },
    [store]
  )

  /** Cancela uma tentativa de join */
  const cancelJoin = useCallback(() => {
    resetGuestWaitingState(null)
  }, [])

  // === P2P Connection ===

  /** Inicia a conexão WebRTC */
  const startP2P = useCallback(
    async (sessionID: string, userID: string, isHost: boolean) => {
      if (
        runtimeState.p2p &&
        runtimeState.p2pSessionID === sessionID &&
        runtimeState.p2pUserID === userID &&
        runtimeState.p2pIsHost === isHost
      ) {
        return
      }

      destroyRuntimeP2P()

      const iceServers =
        store.iceServers.length > 0
          ? store.iceServers
          : await window.go!.main.App.SessionGetICEServers()
      const signalingApp = window.go?.main?.App as { SessionGetSignalingURL?: () => Promise<string> } | undefined
      const signalingURL = signalingApp?.SessionGetSignalingURL
        ? await signalingApp.SessionGetSignalingURL()
        : ''

      const p2p = new P2PConnection({
        sessionID,
        userID,
        isHost,
        iceServers,
        signalingURL,
        signalingPort: store.signalingPort,
      })

      runtimeState.p2p = p2p
      runtimeState.p2pSessionID = sessionID
      runtimeState.p2pUserID = userID
      runtimeState.p2pIsHost = isHost

      // Monitorar estado
      p2p.onStateChange((state) => {
        store.setP2PConnected(state === 'connected')
        if (state !== 'connected') {
          return
        }

        if (isHost) {
          syncScopedWorkspaceToGuests()
          return
        }

        const activeSession = useSessionStore.getState().session
        const scopedWorkspaceID = Number(activeSession?.config?.workspaceID || 0)
        requestGuestWorkspaceScopeSync(sessionID, scopedWorkspaceID)
      })

      // Listener para mensagens de Scroll Sync e permissões
      p2p.onMessage('control', (msg) => {
        if (msg.type === 'scroll_sync') {
          // Disparar evento global para ser capturado pelo PRDiffViewer
          window.dispatchEvent(new CustomEvent('scrollsync:incoming', { detail: msg.payload }))
          return
        }

        if (msg.type === 'permission_change') {
          const permission = extractPermission(msg.payload)
          const targetGuestUserID = extractPermissionTargetUserID(msg.payload)
          const currentUserID = getCurrentIdentity().userID
          const isTargetedToCurrentUser = !targetGuestUserID || targetGuestUserID === currentUserID

          if (permission === 'read_only' && isTargetedToCurrentUser) {
            store.setError('Seu acesso de escrita foi revogado pelo host.')
          }
          return
        }

        if (msg.type === 'workspace_scope_request' && isHost) {
          const requesterUserID = typeof msg.fromUserID === 'string' ? msg.fromUserID.trim() : ''
          if (!requesterUserID) {
            return
          }
          syncScopedWorkspaceToGuests(requesterUserID)
          return
        }

        if (msg.type === 'workspace_scope_sync' && !isHost) {
          const parsed = parseWorkspaceScopeSyncPayload(msg.payload)
          if (!parsed) {
            return
          }
          const activeSessionID = useSessionStore.getState().session?.id || runtimeState.p2pSessionID
          if (!activeSessionID) {
            return
          }
          applyGuestWorkspaceScopeSync(activeSessionID, parsed)
        }
      })

      // Broadcast P2P de estado de GitHub
      p2p.onMessage('github-state', (msg) => {
        if (msg.type === 'prs_updated') {
          window.dispatchEvent(new CustomEvent('github:p2p_state_updated', { detail: msg.payload }))
        }
      })

      // Terminal compartilhado real: input do guest executa no PTY do host; output do host é espelhado nos guests.
      p2p.onMessage('terminal-io', (msg) => {
        if (msg.type === 'terminal_input' && isHost) {
          const payload = parseTerminalPeerPayload(msg.payload)
          const fromUserID = typeof msg.fromUserID === 'string' ? msg.fromUserID.trim() : ''
          if (!payload || !fromUserID) {
            return
          }

          const latest = useSessionStore.getState()
          const guestPermission = latest.session?.guests?.find((guest) => guest.userID === fromUserID)?.permission
          if (guestPermission !== 'read_write') {
            console.warn(`[Session] terminal_input dropped: guest ${fromUserID} has no write permission`)
            return
          }

          const targetSessionID = resolveHostTargetSessionID(payload)
          if (!targetSessionID) {
            console.warn('[Session] terminal_input dropped: could not resolve host terminal target', payload)
            return
          }

          const scopedWorkspaceID = Number(latest.session?.config?.workspaceID || 0)
          if (!isTerminalSessionInWorkspaceScope(targetSessionID, scopedWorkspaceID)) {
            console.warn('[Session] terminal_input dropped: target outside scoped workspace', payload)
            return
          }

          const appBindings = window.go?.main?.App as {
            WriteTerminalAsGuest?: (sessionID: string, userID: string, data: string) => Promise<void>
          } | undefined
          appBindings?.WriteTerminalAsGuest?.(targetSessionID, fromUserID, payload.data).catch((err: unknown) => {
            console.error('[Session] WriteTerminalAsGuest failed for guest input:', err)
          })
          return
        }

        if (msg.type === 'pty_output' && !isHost) {
          const payload = parseTerminalPeerPayload(msg.payload)
          if (!payload) {
            return
          }
          window.dispatchEvent(new CustomEvent('session:terminal-output:remote', { detail: payload }))
        }
      })

      // Cursor awareness remoto
      p2p.onCursorAwareness((payload) => {
        if (payload.userID === getCurrentIdentity().userID) {
          return
        }
        window.dispatchEvent(new CustomEvent('session:cursor-awareness:remote', { detail: payload }))
      })

      // Yjs input compartilhado
      p2p.onSharedInputChange((value) => {
        window.dispatchEvent(new CustomEvent('session:shared-input:remote', { detail: { input: value } }))
      })

      // Revogação imediata por signaling fallback
      p2p.onPermissionChange((permission) => {
        if (permission === 'read_only') {
          store.setError('Seu acesso de escrita foi revogado pelo host.')
        }
      })

      try {
        await p2p.connect()
      } catch (err) {
        if (runtimeState.p2p === p2p) {
          runtimeState.p2p = null
          runtimeState.p2pSessionID = ''
          runtimeState.p2pUserID = ''
          runtimeState.p2pIsHost = false
        }
        throw err
      }
    },
    [store]
  )

  // Host: sincroniza pendências e estado da sessão para cenários multi-instância.
  useEffect(() => {
    if (store.role !== 'host' || !store.session?.id) {
      return
    }

    const sessionID = store.session.id
    let cancelled = false

    const syncHostState = async () => {
      try {
        const [latestSession, pending] = await Promise.all([
          window.go!.main.App.SessionGetSession(sessionID),
          window.go!.main.App.SessionListPendingGuests(sessionID),
        ])

        if (cancelled) {
          return
        }

        if (latestSession?.id === sessionID) {
          store.setSession(latestSession)
        }
        store.setPendingGuests(Array.isArray(pending) ? pending : [])
      } catch {
        // Ignorar erro transitório; próxima iteração tenta novamente.
      }
    }

    void syncHostState()
    const timer = window.setInterval(() => {
      void syncHostState()
    }, 1200)

    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [store, store.role, store.session?.id])

  // Guest ativo: monitora encerramento/kick remoto para evitar estado preso.
  useEffect(() => {
    if (store.role !== 'guest' || !store.session?.id) {
      return
    }

    const sessionID = store.session.id
    const guestUserID = store.activeGuestUserID || store.joinResult?.guestUserID || currentUserID
    let cancelled = false

    const syncGuestSession = async () => {
      try {
        const latestSession = await window.go!.main.App.SessionGetSession(sessionID)
        if (cancelled) {
          return
        }

        if (!latestSession || latestSession.status === 'ended') {
          resetGuestActiveSession(SESSION_ENDED_ERROR)
          return
        }

        const guest = latestSession.guests?.find((g) => g.userID === guestUserID)
        if (!guest) {
          resetGuestActiveSession(SESSION_REMOVED_ERROR)
          return
        }

        if (guest.status === 'rejected' || guest.status === 'expired') {
          resetGuestActiveSession(SESSION_REMOVED_ERROR)
          return
        }

        store.setSession(latestSession)
      } catch (err) {
        if (cancelled) {
          return
        }
        const message = err instanceof Error ? err.message : String(err)
        if (message.includes('session not found')) {
          resetGuestActiveSession(SESSION_ENDED_ERROR)
        }
      }
    }

    void syncGuestSession()
    const timer = window.setInterval(() => {
      void syncGuestSession()
    }, 1200)

    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [currentUserID, store, store.activeGuestUserID, store.joinResult?.guestUserID, store.role, store.session?.id])

  return {
    // State
    session: store.session,
    role: store.role,
    pendingGuests: store.pendingGuests,
    isLoading: store.isLoading,
    error: store.error,
    isWaitingApproval: store.isWaitingApproval,
    wasRestoredSession: store.wasRestoredSession,
    isP2PConnected: store.isP2PConnected,
    joinResult: store.joinResult,
    isSessionActive: store.session?.status === 'active' || store.session?.status === 'waiting',
    auditLogs,
    isAuditLoading,

    // Host Actions
    createSession,
    approveGuest,
    rejectGuest,
    endSession,
    setGuestPermission,
    kickGuest,
    restartEnvironment,
    loadAuditLogs,

    // Guest Actions
    joinSession,
    cancelJoin,

    // P2P
    startP2P,
  }
}
