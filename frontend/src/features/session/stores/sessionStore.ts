import { create } from 'zustand'

// === Types ===

export type SessionStatus = 'idle' | 'waiting' | 'active' | 'ended'
export type GuestStatus = 'pending' | 'approved' | 'rejected' | 'connected'
export type Permission = 'read_only' | 'read_write'
export type SessionMode = 'docker' | 'liveshare'
export type SessionRole = 'host' | 'guest' | 'none'

export interface SessionGuest {
  userID: string
  name: string
  avatarUrl?: string
  permission: Permission
  joinedAt: string
  status: GuestStatus
}

export interface SessionConfig {
  maxGuests: number
  defaultPerm: Permission
  allowAnonymous: boolean
  mode: SessionMode
  dockerImage?: string
  projectPath?: string
  codeTTLMinutes: number
}

export interface Session {
  id: string
  code: string
  hostUserID: string
  hostName: string
  status: SessionStatus
  mode: SessionMode
  guests: SessionGuest[]
  createdAt: string
  expiresAt: string
  config: SessionConfig
}

export interface GuestRequest {
  userID: string
  name: string
  email?: string
  avatarUrl?: string
  requestAt: string
}

export interface JoinResult {
  sessionID: string
  hostName: string
  status: string
}

export interface ICEServerConfig {
  urls: string[]
  username?: string
  credential?: string
}

// === Store State ===

interface SessionState {
  // Estado da sessão
  role: SessionRole
  session: Session | null
  pendingGuests: GuestRequest[]
  isLoading: boolean
  error: string | null

  // Guest-side (aguardando aprovação)
  joinResult: JoinResult | null
  isWaitingApproval: boolean

  // WebRTC
  isP2PConnected: boolean
  iceServers: ICEServerConfig[]

  // Signaling WebSocket
  signalingPort: number
}

interface SessionActions {
  // Host actions
  setSession: (session: Session | null) => void
  setRole: (role: SessionRole) => void
  addPendingGuest: (guest: GuestRequest) => void
  removePendingGuest: (userID: string) => void
  updateGuest: (userID: string, updates: Partial<SessionGuest>) => void
  setPendingGuests: (guests: GuestRequest[]) => void

  // Guest actions
  setJoinResult: (result: JoinResult | null) => void
  setWaitingApproval: (waiting: boolean) => void

  // P2P
  setP2PConnected: (connected: boolean) => void
  setICEServers: (servers: ICEServerConfig[]) => void

  // General
  setLoading: (loading: boolean) => void
  setError: (error: string | null) => void
  reset: () => void
}

const initialState: SessionState = {
  role: 'none',
  session: null,
  pendingGuests: [],
  isLoading: false,
  error: null,
  joinResult: null,
  isWaitingApproval: false,
  isP2PConnected: false,
  iceServers: [],
  signalingPort: 9876,
}

export const useSessionStore = create<SessionState & SessionActions>((set) => ({
  ...initialState,

  setSession: (session) =>
    set({
      session,
      error: null,
    }),

  setRole: (role) => set({ role }),

  addPendingGuest: (guest) =>
    set((state) => ({
      pendingGuests: [...state.pendingGuests, guest],
    })),

  removePendingGuest: (userID) =>
    set((state) => ({
      pendingGuests: state.pendingGuests.filter((g) => g.userID !== userID),
    })),

  updateGuest: (userID, updates) =>
    set((state) => {
      if (!state.session) return state
      return {
        session: {
          ...state.session,
          guests: state.session.guests.map((g) =>
            g.userID === userID ? { ...g, ...updates } : g
          ),
        },
      }
    }),

  setPendingGuests: (guests) => set({ pendingGuests: guests }),

  setJoinResult: (result) => set({ joinResult: result }),

  setWaitingApproval: (waiting) => set({ isWaitingApproval: waiting }),

  setP2PConnected: (connected) => set({ isP2PConnected: connected }),

  setICEServers: (servers) => set({ iceServers: servers }),

  setLoading: (isLoading) => set({ isLoading }),

  setError: (error) => set({ error }),

  reset: () => set(initialState),
}))
