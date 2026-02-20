import { useCallback, useEffect, useMemo, useState } from 'react'
import { useSessionStore } from '../stores/sessionStore'
import { P2PConnection } from '../services/P2PConnection'
import type { Session, GuestRequest, SessionRole } from '../stores/sessionStore'
import type { ScrollSyncEvent } from '../../scroll-sync/types'
import { useAuthStore } from '../../../stores/authStore'

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
  state.setSession(null)
  state.setPendingGuests([])
  resetGuestWaitingState(reason)
}

function getCurrentIdentity() {
  const authUser = useAuthStore.getState().user
  const store = useSessionStore.getState()
  const userID = authUser?.id ||
    (store.role === 'guest' && store.joinResult?.guestUserID ? store.joinResult.guestUserID : '') ||
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
      const expectedGuestID = state.joinResult?.guestUserID
      if (expectedGuestID && payload.guestUserID !== expectedGuestID) {
        return
      }
      state.setSession(payload.session)
      state.setWaitingApproval(false)
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
      const expectedGuestID = state.joinResult?.guestUserID
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
      const expectedGuestID = state.joinResult?.guestUserID
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
    const detail = (e as CustomEvent<{ column: number; row: number; isTyping: boolean }>).detail
    const identity = getCurrentIdentity()
    runtimeState.p2p.sendCursorAwareness({
      userID: identity.userID,
      userName: identity.userName,
      userColor: identity.userColor,
      column: detail.column,
      row: detail.row,
      isTyping: detail.isTyping,
      updatedAt: Date.now(),
    })
  })

  window.addEventListener('session:shared-input:append', (e: Event) => {
    const detail = (e as CustomEvent<{ input: string }>).detail
    if (!runtimeState.p2p?.isConnected || typeof detail.input !== 'string') return
    runtimeState.p2p.appendSharedInput(detail.input)
  })
}

export function useSession() {
  const store = useSessionStore()
  const authUser = useAuthStore((s) => s.user)

  const [auditLogs, setAuditLogs] = useState<AuditLog[]>([])
  const [isAuditLoading, setIsAuditLoading] = useState(false)

  const currentUserID = useMemo(() => {
    if (authUser?.id) return authUser.id
    if (store.role === 'guest' && store.joinResult?.guestUserID) return store.joinResult.guestUserID
    if (store.role === 'host' && store.session?.hostUserID) return store.session.hostUserID
    return 'anonymous'
  }, [authUser?.id, store.joinResult?.guestUserID, store.role, store.session?.hostUserID])

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
    const p2pUserID =
      !isHost && store.joinResult?.guestUserID ? store.joinResult.guestUserID : currentUserID

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
  }, [currentUserID, store.joinResult?.guestUserID, store.role, store.session?.id]) // eslint-disable-line react-hooks/exhaustive-deps

  // Guest: polling de aprovação para cenários multi-instância
  useEffect(() => {
    if (store.role !== 'guest' || !store.isWaitingApproval || !store.joinResult?.sessionID) {
      return
    }

    const sessionID = store.joinResult.sessionID
    const guestUserID = store.joinResult.guestUserID || currentUserID
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
    async (opts?: { maxGuests?: number; mode?: string; allowAnonymous?: boolean }) => {
      store.setLoading(true)
      store.setError(null)

      try {
        const session = await window.go!.main.App.SessionCreate(
          opts?.maxGuests ?? 10,
          opts?.mode ?? 'liveshare',
          opts?.allowAnonymous ?? false
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

      const p2p = new P2PConnection({
        sessionID,
        userID,
        isHost,
        iceServers,
        signalingPort: store.signalingPort,
      })

      runtimeState.p2p = p2p
      runtimeState.p2pSessionID = sessionID
      runtimeState.p2pUserID = userID
      runtimeState.p2pIsHost = isHost

      // Monitorar estado
      p2p.onStateChange((state) => {
        store.setP2PConnected(state === 'connected')
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
        }
      })

      // Broadcast P2P de estado de GitHub
      p2p.onMessage('github-state', (msg) => {
        if (msg.type === 'prs_updated') {
          window.dispatchEvent(new CustomEvent('github:p2p_state_updated', { detail: msg.payload }))
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
    const guestUserID = store.joinResult?.guestUserID || currentUserID
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
  }, [currentUserID, store, store.joinResult?.guestUserID, store.role, store.session?.id])

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
