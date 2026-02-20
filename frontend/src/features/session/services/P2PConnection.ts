import * as Y from 'yjs'
import type { ICEServerConfig } from '../stores/sessionStore'
import type { ScrollSyncEvent } from '../../scroll-sync/types'

// Data channel names — cada canal tem um propósito específico
export const DATA_CHANNELS = {
  TERMINAL_IO: 'terminal-io',
  GITHUB_STATE: 'github-state',
  CURSOR_AWARENESS: 'cursor-awareness',
  CONTROL: 'control',
  CHAT: 'chat',
} as const

export type DataChannelName = (typeof DATA_CHANNELS)[keyof typeof DATA_CHANNELS]

// Tipos de mensagens específicas do canal CONTROL
export type ControlMessageType = 'scroll_sync' | 'permission_change' | 'resize' | 'kick'

export interface ControlMessage {
  type: ControlMessageType
  payload: unknown
  fromUserID: string
  timestamp: number
}

export interface CursorAwarenessPayload {
  userID: string
  userName: string
  userColor: string
  column: number
  row: number
  isTyping: boolean
  updatedAt: number
}

// Tipos de mensagem enviadas nos data channels
export interface DataChannelMessage {
  channel: DataChannelName
  type: string
  payload: unknown
  fromUserID?: string
  timestamp: number
}

interface SendOptions {
  targetUserID?: string
  excludeUserIDs?: string[]
  fromUserID?: string
}

interface PeerContext {
  peerID: string
  pc: RTCPeerConnection
  channels: Map<string, RTCDataChannel>
}

// Callback para mensagens recebidas
type MessageHandler = (msg: DataChannelMessage) => void
type ConnectionStateHandler = (state: RTCPeerConnectionState) => void
type CursorAwarenessHandler = (payload: CursorAwarenessPayload) => void
type SharedInputHandler = (value: string) => void
type PermissionChangeHandler = (permission: string) => void

function encodeUint8Array(data: Uint8Array): string {
  let binary = ''
  for (let i = 0; i < data.length; i++) {
    binary += String.fromCharCode(data[i])
  }
  return btoa(binary)
}

function decodeUint8Array(base64: string): Uint8Array {
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes
}

function isCursorAwarenessPayload(value: unknown): value is CursorAwarenessPayload {
  if (!value || typeof value !== 'object') return false
  const payload = value as Partial<CursorAwarenessPayload>
  return typeof payload.userID === 'string' &&
    typeof payload.userName === 'string' &&
    typeof payload.userColor === 'string' &&
    typeof payload.column === 'number' &&
    typeof payload.row === 'number' &&
    typeof payload.isTyping === 'boolean' &&
    typeof payload.updatedAt === 'number'
}

/**
 * P2PConnection — gerencia conexões WebRTC com reconexão automática
 *
 * Suporta:
 * - Host com múltiplos peers (1 RTCPeerConnection por guest)
 * - Guest com peer único (host)
 * - Múltiplos Data Channels (terminal-io, github-state, etc)
 * - Reconexão automática com backoff exponencial (max 5 retries)
 * - Signaling via WebSocket
 * - CRDT com Yjs para conflitos de input simultâneo
 */
export class P2PConnection {
  private ws: WebSocket | null = null
  private peers = new Map<string, PeerContext>()
  private messageHandlers = new Map<string, Set<MessageHandler>>()
  private stateHandlers = new Set<ConnectionStateHandler>()
  private cursorAwarenessHandlers = new Set<CursorAwarenessHandler>()
  private sharedInputHandlers = new Set<SharedInputHandler>()
  private permissionHandlers = new Set<PermissionChangeHandler>()

  private sessionID: string
  private userID: string
  private isHost: boolean
  private iceServers: ICEServerConfig[]
  private signalingURL: string

  private maxRetries = 5
  private retryDelay = 1000 // ms
  private retryCount = 0
  private isDestroyed = false
  private reconnectInFlight = false
  private aggregateState: RTCPeerConnectionState = 'new'

  // Guest usa sempre 1 peer (host), mas o ID real do host pode variar.
  private guestPeerID = ''

  private yDoc: Y.Doc
  private yInput: Y.Text
  private applyingRemoteYjs = false
  private pendingICEByPeer = new Map<string, RTCIceCandidateInit[]>()

  constructor(opts: {
    sessionID: string
    userID: string
    isHost: boolean
    iceServers: ICEServerConfig[]
    signalingPort?: number
  }) {
    this.sessionID = opts.sessionID
    this.userID = opts.userID
    this.isHost = opts.isHost
    this.iceServers = opts.iceServers
    this.signalingURL = `ws://localhost:${opts.signalingPort || 9876}/ws/signal?session=${opts.sessionID}&user=${opts.userID}&role=${opts.isHost ? 'host' : 'guest'}`

    this.yDoc = new Y.Doc()
    this.yInput = this.yDoc.getText('terminal-input')

    this.yInput.observe(() => {
      if (this.applyingRemoteYjs) {
        return
      }

      const update = Y.encodeStateAsUpdate(this.yDoc)
      this.send(DATA_CHANNELS.TERMINAL_IO, 'yjs_update', encodeUint8Array(update))
      this.emitSharedInput()
    })
  }

  // === Public API ===

  /** Inicia a conexão: abre WebSocket + cria peer inicial no guest */
  async connect(): Promise<void> {
    if (this.isDestroyed) return

    try {
      await this.initializeConnection()
      this.retryCount = 0
    } catch (err) {
      console.error('[P2P] Connection error:', err)
      await this.attemptReconnect()
    }
  }

  private async initializeConnection(): Promise<void> {
    await this.connectSignaling()

    if (!this.isHost && this.peers.size === 0) {
      // Guest mantém um peer único com o host.
      this.guestPeerID = 'host'
      this.ensurePeerConnection(this.guestPeerID, false)
    }

    this.emitAggregateConnectionState(true)
    console.log(`[P2P] Connected as ${this.isHost ? 'host' : 'guest'} to session ${this.sessionID}`)
  }

  /** Cria e envia SDP Offer para um guest específico. */
  async createOffer(targetGuestID: string): Promise<void> {
    if (!this.isHost) {
      return
    }

    if (!targetGuestID) {
      console.warn('[P2P] createOffer ignored: targetGuestID is empty')
      return
    }

    const peer = this.ensurePeerConnection(targetGuestID, true, true)

    try {
      const offer = await peer.pc.createOffer()
      await peer.pc.setLocalDescription(offer)

      this.sendSignal({
        type: 'sdp_offer',
        payload: JSON.stringify(offer),
        targetUserID: targetGuestID,
      })

      console.log(`[P2P] SDP Offer sent to guest ${targetGuestID}`)
    } catch (err) {
      console.error('[P2P] Error creating offer:', err)
    }
  }

  /** Envia dados num data channel específico (broadcast no host por padrão). */
  send(channel: DataChannelName, type: string, payload: unknown, options?: SendOptions): void {
    const msg: DataChannelMessage = {
      channel,
      type,
      payload,
      fromUserID: options?.fromUserID ?? this.userID,
      timestamp: Date.now(),
    }

    // Target explícito (host -> guest específico)
    if (options?.targetUserID) {
      const sent = this.sendToPeer(options.targetUserID, channel, msg)
      if (!sent) {
        console.warn(`[P2P] Channel ${channel} not ready for peer ${options.targetUserID}`)
      }
      return
    }

    // Host: broadcast para todos os guests conectados.
    if (this.isHost) {
      const excluded = new Set(options?.excludeUserIDs || [])
      let sentCount = 0
      for (const [peerID] of this.peers) {
        if (excluded.has(peerID)) {
          continue
        }
        if (this.sendToPeer(peerID, channel, msg)) {
          sentCount++
        }
      }

      if (sentCount === 0) {
        console.warn(`[P2P] Channel ${channel} not ready on any peer`)
      }
      return
    }

    // Guest: envia para o host.
    const guestPeerID = this.resolveGuestPeerID()
    const sent = this.sendToPeer(guestPeerID, channel, msg)
    if (!sent) {
      console.warn(`[P2P] Channel ${channel} not ready (peer: ${guestPeerID})`)
    }
  }

  /** Registra handler para mensagens de um canal específico */
  onMessage(channel: DataChannelName, handler: MessageHandler): () => void {
    if (!this.messageHandlers.has(channel)) {
      this.messageHandlers.set(channel, new Set())
    }
    this.messageHandlers.get(channel)!.add(handler)

    return () => {
      this.messageHandlers.get(channel)?.delete(handler)
    }
  }

  /** Registra handler para mudanças de estado da conexão */
  onStateChange(handler: ConnectionStateHandler): () => void {
    this.stateHandlers.add(handler)
    return () => {
      this.stateHandlers.delete(handler)
    }
  }

  /** Registra handler para cursor awareness remoto */
  onCursorAwareness(handler: CursorAwarenessHandler): () => void {
    this.cursorAwarenessHandlers.add(handler)
    return () => {
      this.cursorAwarenessHandlers.delete(handler)
    }
  }

  /** Registra handler para mudanças do input compartilhado (CRDT). */
  onSharedInputChange(handler: SharedInputHandler): () => void {
    this.sharedInputHandlers.add(handler)
    return () => {
      this.sharedInputHandlers.delete(handler)
    }
  }

  /** Registra handler para mudança de permissão recebida em real-time. */
  onPermissionChange(handler: PermissionChangeHandler): () => void {
    this.permissionHandlers.add(handler)
    return () => {
      this.permissionHandlers.delete(handler)
    }
  }

  /** Verifica se há pelo menos um peer conectado. */
  get isConnected(): boolean {
    return this.aggregateState === 'connected'
  }

  /** Envia evento de Scroll Sync pelo canal CONTROL */
  sendScrollSyncEvent(event: ScrollSyncEvent): void {
    this.send(DATA_CHANNELS.CONTROL, 'scroll_sync', event)
  }

  /** Envia uma mensagem de controle explícita. */
  sendControlMessage(type: ControlMessageType, payload: unknown, targetUserID?: string): void {
    this.send(DATA_CHANNELS.CONTROL, type, payload, targetUserID ? { targetUserID } : undefined)
  }

  /** Broadcast de update GitHub para peers da sessão. */
  broadcastGitHubState(payload: unknown): void {
    this.send(DATA_CHANNELS.GITHUB_STATE, 'prs_updated', payload)
  }

  /** Envia atualização de cursor awareness para os peers. */
  sendCursorAwareness(payload: CursorAwarenessPayload): void {
    this.send(DATA_CHANNELS.CURSOR_AWARENESS, 'cursor_awareness', payload)
  }

  /** Atualiza o valor compartilhado via Yjs (substitui conteúdo completo). */
  updateSharedInput(value: string): void {
    const current = this.yInput.toString()
    this.yDoc.transact(() => {
      if (current.length > 0) {
        this.yInput.delete(0, current.length)
      }
      if (value.length > 0) {
        this.yInput.insert(0, value)
      }
    })
  }

  /** Aplica fragmento de input no fim do texto compartilhado. */
  appendSharedInput(fragment: string): void {
    if (!fragment) {
      return
    }
    this.yDoc.transact(() => {
      this.yInput.insert(this.yInput.length, fragment)
    })
  }

  /** Retorna estado atual do buffer compartilhado. */
  getSharedInput(): string {
    return this.yInput.toString()
  }

  /** Destrói a conexão e limpa recursos */
  destroy(): void {
    this.isDestroyed = true

    this.closeAllPeers()

    if (this.ws) {
      this.ws.close()
      this.ws = null
    }

    this.messageHandlers.clear()
    this.stateHandlers.clear()
    this.cursorAwarenessHandlers.clear()
    this.sharedInputHandlers.clear()
    this.permissionHandlers.clear()
    this.pendingICEByPeer.clear()
    this.yDoc.destroy()

    this.aggregateState = 'closed'
    console.log('[P2P] Connection destroyed')
  }

  // === Private: Signaling ===

  private connectSignaling(): Promise<void> {
    return new Promise((resolve, reject) => {
      this.ws = new WebSocket(this.signalingURL)

      this.ws.onopen = () => {
        console.log('[P2P] Signaling WebSocket connected')
        resolve()
      }

      this.ws.onerror = (err) => {
        console.error('[P2P] Signaling error:', err)
        reject(err)
      }

      this.ws.onclose = () => {
        console.log('[P2P] Signaling WebSocket closed')
        if (!this.isDestroyed) {
          this.attemptReconnect()
        }
      }

      this.ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data)
          this.handleSignalMessage(msg)
        } catch (err) {
          console.error('[P2P] Invalid signal message:', err)
        }
      }
    })
  }

  private sendSignal(msg: { type: string; payload?: string; targetUserID?: string }): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.warn('[P2P] WebSocket not ready for signaling')
      return
    }
    this.ws.send(JSON.stringify(msg))
  }

  private async handleSignalMessage(msg: {
    type: string
    payload?: string
    fromUserID?: string
    targetUserID?: string
  }): Promise<void> {
    switch (msg.type) {
      case 'sdp_offer':
        if (!this.isHost && msg.payload) {
          await this.handleOffer(JSON.parse(msg.payload), msg.fromUserID || '')
        }
        break

      case 'sdp_answer':
        if (this.isHost && msg.payload) {
          await this.handleAnswer(JSON.parse(msg.payload), msg.fromUserID || '')
        }
        break

      case 'ice_candidate':
        if (msg.payload) {
          await this.handleICECandidate(JSON.parse(msg.payload), msg.fromUserID || '')
        }
        break

      case 'guest_approved':
        console.log('[P2P] Guest approved, waiting for SDP offer...')
        break

      case 'guest_rejected':
        console.log('[P2P] Guest was rejected')
        this.destroy()
        break

      case 'session_ended':
        console.log('[P2P] Session ended by host')
        this.destroy()
        break

      case 'permission_change':
        if (msg.payload && typeof msg.payload === 'string') {
          this.permissionHandlers.forEach((handler) => handler(msg.payload!))
        }
        break

      default:
        break
    }
  }

  // === Private: WebRTC ===

  private ensurePeerConnection(peerID: string, createHostDataChannels: boolean, forceRecreate = false): PeerContext {
    if (forceRecreate) {
      this.removePeer(peerID)
    }

    const existing = this.peers.get(peerID)
    if (existing) {
      if (this.isHost && createHostDataChannels && existing.channels.size === 0) {
        this.createDataChannels(peerID, existing.pc)
      }
      return existing
    }

    const config: RTCConfiguration = {
      iceServers: this.iceServers.map((s) => ({
        urls: s.urls,
        username: s.username,
        credential: s.credential,
      })),
    }

    const pc = new RTCPeerConnection(config)
    const peer: PeerContext = {
      peerID,
      pc,
      channels: new Map(),
    }

    this.peers.set(peerID, peer)
    this.bindPeerConnectionHandlers(peer)

    if (this.isHost && createHostDataChannels) {
      this.createDataChannels(peerID, pc)
    }

    this.emitAggregateConnectionState(true)
    return peer
  }

  private bindPeerConnectionHandlers(peer: PeerContext): void {
    const { peerID, pc } = peer

    pc.onicecandidate = (event) => {
      if (!event.candidate) {
        return
      }

      this.sendSignal({
        type: 'ice_candidate',
        payload: JSON.stringify(event.candidate),
        targetUserID: this.isHost ? peerID : undefined,
      })
    }

    pc.onconnectionstatechange = () => {
      const state = pc.connectionState
      console.log(`[P2P] Peer ${peerID} state: ${state}`)
      this.emitAggregateConnectionState(true)

      if (state === 'connected') {
        this.sendSignal({ type: 'peer_connected' })
        return
      }

      if ((state === 'failed' || state === 'disconnected') && !this.isDestroyed) {
        if (this.isHost) {
          // Host mantém os outros peers vivos; remove apenas o peer afetado.
          this.removePeer(peerID)
          this.emitAggregateConnectionState(true)
          return
        }
        this.attemptReconnect()
      }

      if (state === 'closed' && this.isHost) {
        this.removePeer(peerID)
        this.emitAggregateConnectionState(true)
      }
    }

    if (!this.isHost) {
      pc.ondatachannel = (event) => {
        const dc = event.channel
        this.setupDataChannel(peerID, dc)
        console.log(`[P2P] Received data channel ${dc.label} from ${peerID}`)
      }
    }
  }

  private createDataChannels(peerID: string, pc: RTCPeerConnection): void {
    Object.values(DATA_CHANNELS).forEach((channelName) => {
      const dc = pc.createDataChannel(channelName, {
        ordered: channelName === DATA_CHANNELS.TERMINAL_IO,
      })
      this.setupDataChannel(peerID, dc)
    })
  }

  private setupDataChannel(peerID: string, dc: RTCDataChannel): void {
    dc.onopen = () => {
      console.log(`[P2P] Data channel ${dc.label} opened (peer=${peerID})`)
      const peer = this.peers.get(peerID)
      if (!peer) {
        return
      }
      peer.channels.set(dc.label, dc)

      if (dc.label === DATA_CHANNELS.TERMINAL_IO) {
        const update = Y.encodeStateAsUpdate(this.yDoc)
        this.send(DATA_CHANNELS.TERMINAL_IO, 'yjs_update', encodeUint8Array(update), {
          targetUserID: peerID,
        })
      }
    }

    dc.onclose = () => {
      console.log(`[P2P] Data channel ${dc.label} closed (peer=${peerID})`)
      this.peers.get(peerID)?.channels.delete(dc.label)
    }

    dc.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data) as DataChannelMessage

        if (msg.channel === DATA_CHANNELS.TERMINAL_IO && msg.type === 'yjs_update' && typeof msg.payload === 'string') {
          this.applyRemoteYjsUpdate(msg.payload, peerID)
        }

        if (msg.channel === DATA_CHANNELS.CURSOR_AWARENESS && msg.type === 'cursor_awareness' && isCursorAwarenessPayload(msg.payload)) {
          const payload = msg.payload
          this.cursorAwarenessHandlers.forEach((handler) => handler(payload))

          // Host retransmite presença de um guest para os demais guests.
          if (this.isHost) {
            this.send(DATA_CHANNELS.CURSOR_AWARENESS, 'cursor_awareness', payload, {
              excludeUserIDs: [peerID],
            })
          }
        }

        if (msg.channel === DATA_CHANNELS.CONTROL && msg.type === 'permission_change' && typeof msg.payload === 'string') {
          const payload = msg.payload
          this.permissionHandlers.forEach((handler) => handler(payload))
        }

        const handlers = this.messageHandlers.get(msg.channel)
        if (handlers) {
          handlers.forEach((handler) => handler(msg))
        }
      } catch (err) {
        console.error(`[P2P] Error parsing message on ${dc.label} (peer=${peerID}):`, err)
      }
    }

    dc.onerror = (err) => {
      console.error(`[P2P] Data channel ${dc.label} error (peer=${peerID}):`, err)
    }
  }

  private applyRemoteYjsUpdate(encodedUpdate: string, sourcePeerID?: string): void {
    try {
      this.applyingRemoteYjs = true
      const update = decodeUint8Array(encodedUpdate)
      Y.applyUpdate(this.yDoc, update)
      this.emitSharedInput()
    } finally {
      this.applyingRemoteYjs = false
    }

    // Host retransmite update recebido de um guest para os demais peers.
    if (this.isHost) {
      this.send(DATA_CHANNELS.TERMINAL_IO, 'yjs_update', encodedUpdate, {
        excludeUserIDs: sourcePeerID ? [sourcePeerID] : undefined,
      })
    }
  }

  private emitSharedInput(): void {
    const value = this.getSharedInput()
    this.sharedInputHandlers.forEach((handler) => handler(value))
  }

  private async handleOffer(offer: RTCSessionDescriptionInit, fromUserID: string): Promise<void> {
    const peerID = this.resolveGuestPeerID(fromUserID)
    const peer = this.ensurePeerConnection(peerID, false, true)

    try {
      await peer.pc.setRemoteDescription(new RTCSessionDescription(offer))
      await this.flushPendingICE(peer)
      const answer = await peer.pc.createAnswer()
      await peer.pc.setLocalDescription(answer)

      this.sendSignal({
        type: 'sdp_answer',
        payload: JSON.stringify(answer),
        targetUserID: fromUserID || undefined,
      })

      console.log(`[P2P] SDP Answer sent (peer=${peerID})`)
    } catch (err) {
      console.error('[P2P] Error handling offer:', err)
    }
  }

  private async handleAnswer(answer: RTCSessionDescriptionInit, fromUserID: string): Promise<void> {
    const peer = this.resolvePeerForHost(fromUserID)
    if (!peer) {
      console.warn(`[P2P] Received SDP answer for unknown peer ${fromUserID}`)
      return
    }

    try {
      await peer.pc.setRemoteDescription(new RTCSessionDescription(answer))
      await this.flushPendingICE(peer)
      console.log(`[P2P] SDP Answer received and set (peer=${peer.peerID})`)
    } catch (err) {
      console.error('[P2P] Error handling answer:', err)
    }
  }

  private async handleICECandidate(candidate: RTCIceCandidateInit, fromUserID: string): Promise<void> {
    const peer = this.isHost ? this.resolvePeerForHost(fromUserID) : this.resolvePeerForGuest(fromUserID)
    if (!peer) {
      if (fromUserID) {
        this.enqueuePendingICE(fromUserID, candidate)
      }
      console.warn(`[P2P] Received ICE candidate for unknown peer ${fromUserID}`)
      return
    }

    if (!peer.pc.remoteDescription) {
      this.enqueuePendingICE(peer.peerID, candidate)
      return
    }

    try {
      await peer.pc.addIceCandidate(new RTCIceCandidate(candidate))
    } catch (err) {
      console.error('[P2P] Error adding ICE candidate:', err)
    }
  }

  private resolvePeerForHost(fromUserID: string): PeerContext | null {
    if (fromUserID) {
      const exact = this.peers.get(fromUserID)
      if (exact) {
        return exact
      }
    }

    if (this.peers.size === 1) {
      return this.peers.values().next().value || null
    }

    return null
  }

  private resolvePeerForGuest(fromUserID: string): PeerContext | null {
    const peerID = this.resolveGuestPeerID(fromUserID)
    const exact = this.peers.get(peerID)
    if (exact) {
      return exact
    }

    if (this.peers.size === 1) {
      return this.peers.values().next().value || null
    }

    return null
  }

  private resolveGuestPeerID(fromUserID?: string): string {
    const incoming = (fromUserID || '').trim()

    if (this.guestPeerID) {
      return this.guestPeerID
    }

    if (incoming) {
      this.guestPeerID = incoming
    } else {
      this.guestPeerID = 'host'
    }

    return this.guestPeerID
  }

  private sendToPeer(peerID: string, channel: DataChannelName, msg: DataChannelMessage): boolean {
    const peer = this.peers.get(peerID)
    if (!peer) {
      return false
    }

    const dc = peer.channels.get(channel)
    if (!dc || dc.readyState !== 'open') {
      return false
    }

    dc.send(JSON.stringify(msg))
    return true
  }

  private removePeer(peerID: string): void {
    const peer = this.peers.get(peerID)
    if (!peer) {
      return
    }

    peer.channels.forEach((dc) => {
      try {
        dc.close()
      } catch {
        // noop
      }
    })
    peer.channels.clear()

    try {
      peer.pc.close()
    } catch {
      // noop
    }

    this.peers.delete(peerID)
    this.pendingICEByPeer.delete(peerID)
  }

  private closeAllPeers(): void {
    const peerIDs = Array.from(this.peers.keys())
    peerIDs.forEach((peerID) => this.removePeer(peerID))
    this.guestPeerID = ''
  }

  private computeAggregateState(): RTCPeerConnectionState {
    if (this.peers.size === 0) {
      return 'new'
    }

    const states = Array.from(this.peers.values()).map((peer) => peer.pc.connectionState)

    if (states.some((state) => state === 'connected')) {
      return 'connected'
    }
    if (states.some((state) => state === 'connecting')) {
      return 'connecting'
    }
    if (states.some((state) => state === 'disconnected')) {
      return 'disconnected'
    }
    if (states.some((state) => state === 'failed')) {
      return 'failed'
    }
    if (states.some((state) => state === 'closed')) {
      return 'closed'
    }
    return 'new'
  }

  private emitAggregateConnectionState(force = false): void {
    const next = this.computeAggregateState()
    if (!force && next === this.aggregateState) {
      return
    }

    this.aggregateState = next
    this.stateHandlers.forEach((handler) => handler(next))
  }

  private enqueuePendingICE(peerID: string, candidate: RTCIceCandidateInit): void {
    if (!peerID) {
      return
    }
    const pending = this.pendingICEByPeer.get(peerID) || []
    pending.push(candidate)
    this.pendingICEByPeer.set(peerID, pending)
  }

  private async flushPendingICE(peer: PeerContext): Promise<void> {
    const pending = this.pendingICEByPeer.get(peer.peerID)
    if (!pending || pending.length === 0 || !peer.pc.remoteDescription) {
      return
    }

    this.pendingICEByPeer.delete(peer.peerID)
    for (const candidate of pending) {
      try {
        await peer.pc.addIceCandidate(new RTCIceCandidate(candidate))
      } catch (err) {
        console.error(`[P2P] Error adding queued ICE candidate (peer=${peer.peerID}):`, err)
      }
    }
  }

  // === Private: Reconnection ===

  private async attemptReconnect(): Promise<void> {
    if (this.reconnectInFlight) {
      return
    }
    this.reconnectInFlight = true
    try {
      while (!this.isDestroyed && this.retryCount < this.maxRetries) {
        this.retryCount++
        const delay = this.retryDelay * Math.pow(2, this.retryCount - 1)
        console.log(`[P2P] Reconnecting in ${delay}ms (attempt ${this.retryCount}/${this.maxRetries})`)

        await new Promise((resolve) => setTimeout(resolve, delay))

        if (this.isDestroyed) {
          break
        }

        const keepPeers = this.isHost && this.peers.size > 0
        if (!keepPeers) {
          this.closeAllPeers()
        }

        if (this.ws) {
          this.ws.close()
          this.ws = null
        }

        try {
          await this.initializeConnection()
          this.retryCount = 0
          return
        } catch (err) {
          console.error('[P2P] Reconnect attempt failed:', err)
        }
      }

      if (!this.isDestroyed && this.retryCount >= this.maxRetries) {
        console.error(`[P2P] Max retries (${this.maxRetries}) reached, giving up`)
        this.stateHandlers.forEach((handler) => handler('failed'))
      }
    } finally {
      this.reconnectInFlight = false
    }
  }
}
