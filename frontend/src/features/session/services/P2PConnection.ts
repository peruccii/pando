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
 * P2PConnection — gerencia uma conexão WebRTC com reconexão automática
 *
 * Suporta:
 * - Múltiplos Data Channels (terminal-io, github-state, etc)
 * - Reconexão automática com backoff exponencial (max 5 retries)
 * - Signaling via WebSocket
 * - CRDT com Yjs para conflitos de input simultâneo
 */
export class P2PConnection {
  private pc: RTCPeerConnection | null = null
  private ws: WebSocket | null = null
  private dataChannels = new Map<string, RTCDataChannel>()
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

  private yDoc: Y.Doc
  private yInput: Y.Text
  private applyingRemoteYjs = false

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

  /** Inicia a conexão: abre WebSocket + cria RTCPeerConnection */
  async connect(): Promise<void> {
    if (this.isDestroyed) return

    try {
      // 1. Conectar ao signaling server
      await this.connectSignaling()

      // 2. Criar RTCPeerConnection
      this.createPeerConnection()

      // 3. Se Host, criar data channels e offer
      if (this.isHost) {
        this.createDataChannels()
      }

      this.retryCount = 0
      console.log(`[P2P] Connected as ${this.isHost ? 'host' : 'guest'} to session ${this.sessionID}`)
    } catch (err) {
      console.error('[P2P] Connection error:', err)
      await this.attemptReconnect()
    }
  }

  /** Cria e envia SDP Offer (usado pelo Host após aprovação de um guest) */
  async createOffer(targetGuestID: string): Promise<void> {
    if (!this.pc) return

    try {
      const offer = await this.pc.createOffer()
      await this.pc.setLocalDescription(offer)

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

  /** Envia dados num data channel específico */
  send(channel: DataChannelName, type: string, payload: unknown): void {
    const dc = this.dataChannels.get(channel)
    if (!dc || dc.readyState !== 'open') {
      console.warn(`[P2P] Channel ${channel} not ready (state: ${dc?.readyState})`)
      return
    }

    const msg: DataChannelMessage = {
      channel,
      type,
      payload,
      fromUserID: this.userID,
      timestamp: Date.now(),
    }

    dc.send(JSON.stringify(msg))
  }

  /** Registra handler para mensagens de um canal específico */
  onMessage(channel: DataChannelName, handler: MessageHandler): () => void {
    if (!this.messageHandlers.has(channel)) {
      this.messageHandlers.set(channel, new Set())
    }
    this.messageHandlers.get(channel)!.add(handler)

    // Retorna função de cleanup
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

  /** Verifica se a conexão P2P está ativa */
  get isConnected(): boolean {
    return this.pc?.connectionState === 'connected'
  }

  /** Envia evento de Scroll Sync pelo canal CONTROL */
  sendScrollSyncEvent(event: ScrollSyncEvent): void {
    this.send(DATA_CHANNELS.CONTROL, 'scroll_sync', event)
  }

  /** Envia uma mensagem de controle explícita. */
  sendControlMessage(type: ControlMessageType, payload: unknown): void {
    this.send(DATA_CHANNELS.CONTROL, type, payload)
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

    // Fechar data channels
    this.dataChannels.forEach((dc) => dc.close())
    this.dataChannels.clear()

    // Fechar PeerConnection
    if (this.pc) {
      this.pc.close()
      this.pc = null
    }

    // Fechar WebSocket
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }

    this.messageHandlers.clear()
    this.stateHandlers.clear()
    this.cursorAwarenessHandlers.clear()
    this.sharedInputHandlers.clear()
    this.permissionHandlers.clear()
    this.yDoc.destroy()

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
        // Guest recebe SDP Offer do Host
        if (!this.isHost && msg.payload) {
          await this.handleOffer(JSON.parse(msg.payload))
        }
        break

      case 'sdp_answer':
        // Host recebe SDP Answer de um Guest
        if (this.isHost && msg.payload) {
          await this.handleAnswer(JSON.parse(msg.payload))
        }
        break

      case 'ice_candidate':
        // Receber ICE candidate do peer
        if (msg.payload) {
          await this.handleICECandidate(JSON.parse(msg.payload))
        }
        break

      case 'guest_approved':
        // Guest foi aprovado — conexão P2P pode iniciar
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

  private createPeerConnection(): void {
    const config: RTCConfiguration = {
      iceServers: this.iceServers.map((s) => ({
        urls: s.urls,
        username: s.username,
        credential: s.credential,
      })),
    }

    this.pc = new RTCPeerConnection(config)

    // ICE Candidate handling
    this.pc.onicecandidate = (event) => {
      if (event.candidate) {
        this.sendSignal({
          type: 'ice_candidate',
          payload: JSON.stringify(event.candidate),
        })
      }
    }

    // Connection state changes
    this.pc.onconnectionstatechange = () => {
      const state = this.pc?.connectionState
      console.log(`[P2P] Connection state: ${state}`)

      if (state) {
        this.stateHandlers.forEach((handler) => handler(state))
      }

      if (state === 'connected') {
        this.sendSignal({ type: 'peer_connected' })
      } else if (state === 'failed' || state === 'disconnected') {
        if (!this.isDestroyed) {
          this.attemptReconnect()
        }
      }
    }

    // Guest: receber data channels criados pelo Host
    if (!this.isHost) {
      this.pc.ondatachannel = (event) => {
        const dc = event.channel
        this.setupDataChannel(dc)
        console.log(`[P2P] Received data channel: ${dc.label}`)
      }
    }
  }

  private createDataChannels(): void {
    if (!this.pc) return

    // Criar todos os data channels definidos na spec
    Object.values(DATA_CHANNELS).forEach((channelName) => {
      const dc = this.pc!.createDataChannel(channelName, {
        ordered: channelName === DATA_CHANNELS.TERMINAL_IO,
      })
      this.setupDataChannel(dc)
    })
  }

  private setupDataChannel(dc: RTCDataChannel): void {
    dc.onopen = () => {
      console.log(`[P2P] Data channel ${dc.label} opened`)
      this.dataChannels.set(dc.label, dc)

      if (dc.label === DATA_CHANNELS.TERMINAL_IO) {
        const update = Y.encodeStateAsUpdate(this.yDoc)
        this.send(DATA_CHANNELS.TERMINAL_IO, 'yjs_update', encodeUint8Array(update))
      }
    }

    dc.onclose = () => {
      console.log(`[P2P] Data channel ${dc.label} closed`)
      this.dataChannels.delete(dc.label)
    }

    dc.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data) as DataChannelMessage

        if (msg.channel === DATA_CHANNELS.TERMINAL_IO && msg.type === 'yjs_update' && typeof msg.payload === 'string') {
          this.applyRemoteYjsUpdate(msg.payload)
        }

        if (msg.channel === DATA_CHANNELS.CURSOR_AWARENESS && msg.type === 'cursor_awareness' && isCursorAwarenessPayload(msg.payload)) {
          const payload = msg.payload
          this.cursorAwarenessHandlers.forEach((handler) => handler(payload))
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
        console.error(`[P2P] Error parsing message on ${dc.label}:`, err)
      }
    }

    dc.onerror = (err) => {
      console.error(`[P2P] Data channel ${dc.label} error:`, err)
    }
  }

  private applyRemoteYjsUpdate(encodedUpdate: string): void {
    try {
      this.applyingRemoteYjs = true
      const update = decodeUint8Array(encodedUpdate)
      Y.applyUpdate(this.yDoc, update)
      this.emitSharedInput()
    } finally {
      this.applyingRemoteYjs = false
    }
  }

  private emitSharedInput(): void {
    const value = this.getSharedInput()
    this.sharedInputHandlers.forEach((handler) => handler(value))
  }

  private async handleOffer(offer: RTCSessionDescriptionInit): Promise<void> {
    if (!this.pc) {
      this.createPeerConnection()
    }

    try {
      await this.pc!.setRemoteDescription(new RTCSessionDescription(offer))
      const answer = await this.pc!.createAnswer()
      await this.pc!.setLocalDescription(answer)

      this.sendSignal({
        type: 'sdp_answer',
        payload: JSON.stringify(answer),
      })

      console.log('[P2P] SDP Answer sent')
    } catch (err) {
      console.error('[P2P] Error handling offer:', err)
    }
  }

  private async handleAnswer(answer: RTCSessionDescriptionInit): Promise<void> {
    if (!this.pc) return

    try {
      await this.pc.setRemoteDescription(new RTCSessionDescription(answer))
      console.log('[P2P] SDP Answer received and set')
    } catch (err) {
      console.error('[P2P] Error handling answer:', err)
    }
  }

  private async handleICECandidate(candidate: RTCIceCandidateInit): Promise<void> {
    if (!this.pc) return

    try {
      await this.pc.addIceCandidate(new RTCIceCandidate(candidate))
    } catch (err) {
      console.error('[P2P] Error adding ICE candidate:', err)
    }
  }

  // === Private: Reconnection ===

  private async attemptReconnect(): Promise<void> {
    if (this.isDestroyed || this.retryCount >= this.maxRetries) {
      console.error(`[P2P] Max retries (${this.maxRetries}) reached, giving up`)
      this.stateHandlers.forEach((handler) => handler('failed'))
      return
    }

    this.retryCount++
    const delay = this.retryDelay * Math.pow(2, this.retryCount - 1)
    console.log(`[P2P] Reconnecting in ${delay}ms (attempt ${this.retryCount}/${this.maxRetries})`)

    await new Promise((resolve) => setTimeout(resolve, delay))

    if (!this.isDestroyed) {
      // Limpar estado anterior
      if (this.pc) {
        this.pc.close()
        this.pc = null
      }
      this.dataChannels.clear()

      await this.connect()
    }
  }
}
