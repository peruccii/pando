import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSessionStore } from '../stores/sessionStore'
import { P2PConnection } from '../services/P2PConnection'
import type { Session, GuestRequest } from '../stores/sessionStore'
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

const CURSOR_PALETTE = ['#22c55e', '#3b82f6', '#f59e0b', '#ef4444', '#a855f7', '#06b6d4'] as const

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

export function useSession() {
  const store = useSessionStore()
  const authUser = useAuthStore((s) => s.user)
  const p2pRef = useRef<P2PConnection | null>(null)

  const [auditLogs, setAuditLogs] = useState<AuditLog[]>([])
  const [isAuditLoading, setIsAuditLoading] = useState(false)

  const currentUserID = useMemo(() => {
    if (authUser?.id) return authUser.id
    if (store.role === 'host' && store.session?.hostUserID) return store.session.hostUserID
    return 'anonymous'
  }, [authUser?.id, store.role, store.session?.hostUserID])

  const currentUserName = useMemo(() => {
    if (authUser?.name) return authUser.name
    return store.role === 'host' ? 'Host' : 'Guest'
  }, [authUser?.name, store.role])

  const currentUserColor = useMemo(() => pickCursorColor(currentUserID), [currentUserID])

  // === Escutar eventos Wails do backend ===
  useEffect(() => {
    const cleanups: (() => void)[] = []

    if (window.runtime) {
      // Guest quer entrar
      cleanups.push(
        window.runtime.EventsOn('session:guest_request', (data: unknown) => {
          const guest = data as GuestRequest
          store.addPendingGuest(guest)
        })
      )

      // Guest aprovado
      cleanups.push(
        window.runtime.EventsOn('session:guest_approved', (data: unknown) => {
          const payload = data as { sessionID: string; guestUserID: string; session: Session }
          store.setSession(payload.session)
          store.setWaitingApproval(false)
        })
      )

      // Guest rejeitado
      cleanups.push(
        window.runtime.EventsOn('session:guest_rejected', () => {
          store.setWaitingApproval(false)
          store.setError('Sua solicitação foi rejeitada pelo host.')
          store.setRole('none')
        })
      )

      // Sessão criada
      cleanups.push(
        window.runtime.EventsOn('session:created', (data: unknown) => {
          const session = data as Session
          store.setSession(session)
        })
      )

      // Sessão encerrada
      cleanups.push(
        window.runtime.EventsOn('session:ended', () => {
          store.reset()
          setAuditLogs([])
          if (p2pRef.current) {
            p2pRef.current.destroy()
            p2pRef.current = null
          }
        })
      )

      // Permissão alterada
      cleanups.push(
        window.runtime.EventsOn('session:permission_changed', (data: unknown) => {
          const payload = data as { guestUserID: string; permission: string }
          store.updateGuest(payload.guestUserID, {
            permission: payload.permission as 'read_only' | 'read_write',
          })

          const current = useSessionStore.getState()
          if (current.role === 'host' && p2pRef.current?.isConnected) {
            p2pRef.current.sendControlMessage('permission_change', {
              guestUserID: payload.guestUserID,
              permission: payload.permission,
            })
          }
        })
      )

      // Revogação imediata de escrita
      cleanups.push(
        window.runtime.EventsOn('session:permission_revoked', (data: unknown) => {
          const payload = data as { guestUserID: string }
          if (payload.guestUserID === currentUserID) {
            store.setError('Seu acesso de escrita foi revogado pelo host.')
          }
        })
      )

      // Guest kickado
      cleanups.push(
        window.runtime.EventsOn('session:guest_kicked', (data: unknown) => {
          const payload = data as { guestUserID: string }
          store.updateGuest(payload.guestUserID, { status: 'rejected' })

          if (payload.guestUserID === currentUserID) {
            store.setError('Você foi removido da sessão pelo host.')
          }
        })
      )

      // Broadcast P2P de mudanças GitHub durante colaboração
      cleanups.push(
        window.runtime.EventsOn('github:prs:updated', (data: unknown) => {
          const current = useSessionStore.getState()
          if (current.role === 'host' && p2pRef.current?.isConnected) {
            p2pRef.current.broadcastGitHubState(data)
          }
        })
      )

      cleanups.push(
        window.runtime.EventsOn('session:docker_fallback', (data: unknown) => {
          const payload = data as { reason?: string }
          store.setError(payload.reason ?? 'Docker indisponível. Sessão iniciou em Live Share.')
        })
      )
    }

    // Listener para eventos de Scroll Sync (envio)
    const handleScrollSyncOutgoing = (e: Event) => {
      const event = (e as CustomEvent<ScrollSyncEvent>).detail
      if (p2pRef.current?.isConnected) {
        p2pRef.current.sendScrollSyncEvent(event)
      }
    }
    window.addEventListener('scrollsync:outgoing', handleScrollSyncOutgoing)
    cleanups.push(() => window.removeEventListener('scrollsync:outgoing', handleScrollSyncOutgoing))

    // Cursor awareness local -> enviar via WebRTC
    const handleLocalCursor = (e: Event) => {
      if (!p2pRef.current?.isConnected) return
      const detail = (e as CustomEvent<{ column: number; row: number; isTyping: boolean }>).detail
      p2pRef.current.sendCursorAwareness({
        userID: currentUserID,
        userName: currentUserName,
        userColor: currentUserColor,
        column: detail.column,
        row: detail.row,
        isTyping: detail.isTyping,
        updatedAt: Date.now(),
      })
    }
    window.addEventListener('session:cursor-awareness:local', handleLocalCursor)
    cleanups.push(() => window.removeEventListener('session:cursor-awareness:local', handleLocalCursor))

    // Input local -> Yjs CRDT
    const handleSharedInput = (e: Event) => {
      const detail = (e as CustomEvent<{ input: string }>).detail
      if (!p2pRef.current?.isConnected || typeof detail.input !== 'string') return
      p2pRef.current.appendSharedInput(detail.input)
    }
    window.addEventListener('session:shared-input:append', handleSharedInput)
    cleanups.push(() => window.removeEventListener('session:shared-input:append', handleSharedInput))

    return () => {
      cleanups.forEach((fn) => fn())
    }
  }, [currentUserColor, currentUserID, currentUserName, store])

  // Auto-start P2P quando houver sessão ativa
  useEffect(() => {
    if (!store.session || store.role === 'none') {
      return
    }
    if (p2pRef.current) {
      return
    }

    const isHost = store.role === 'host'
    startP2P(store.session.id, currentUserID, isHost).catch((err: unknown) => {
      store.setError(err instanceof Error ? err.message : String(err))
    })
  }, [currentUserID, store.role, store.session?.id]) // eslint-disable-line react-hooks/exhaustive-deps

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

        // Buscar ICE servers
        const iceServers = await window.go!.main.App.SessionGetICEServers()
        store.setICEServers(iceServers)

        await loadAuditLogs(session.id)
        return session
      } catch (err) {
        store.setError(err instanceof Error ? err.message : String(err))
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
        if (p2pRef.current) {
          await p2pRef.current.createOffer(guestUserID)
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
      if (p2pRef.current) {
        p2pRef.current.destroy()
        p2pRef.current = null
      }
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

  // === Guest Actions ===

  /** Entra numa sessão como Guest usando o código */
  const joinSession = useCallback(
    async (code: string, name?: string, email?: string) => {
      store.setLoading(true)
      store.setError(null)

      try {
        const result = await window.go!.main.App.SessionJoin(code, name ?? '', email ?? '')
        store.setJoinResult(result)
        store.setRole('guest')
        store.setWaitingApproval(true)

        // Buscar ICE servers
        const iceServers = await window.go!.main.App.SessionGetICEServers()
        store.setICEServers(iceServers)

        return result
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
    store.setWaitingApproval(false)
    store.setJoinResult(null)
    store.setRole('none')

    if (p2pRef.current) {
      p2pRef.current.destroy()
      p2pRef.current = null
    }
  }, [store])

  // === P2P Connection ===

  /** Inicia a conexão WebRTC */
  const startP2P = useCallback(
    async (sessionID: string, userID: string, isHost: boolean) => {
      if (p2pRef.current) {
        p2pRef.current.destroy()
      }

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
          if (permission === 'read_only') {
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
        if (payload.userID === currentUserID) {
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

      p2pRef.current = p2p
      await p2p.connect()
    },
    [currentUserID, store]
  )

  return {
    // State
    session: store.session,
    role: store.role,
    pendingGuests: store.pendingGuests,
    isLoading: store.isLoading,
    error: store.error,
    isWaitingApproval: store.isWaitingApproval,
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
