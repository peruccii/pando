import { create } from 'zustand'

// === Types ===

export type SessionStatus = 'idle' | 'waiting' | 'active' | 'ended'
export type GuestStatus = 'pending' | 'approved' | 'rejected' | 'connected' | 'expired'
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
  workspaceID: number
  workspaceName?: string
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
  sessionCode?: string
  hostName: string
  status: string
  guestUserID?: string
  approvalExpiresAt?: string
  workspaceName?: string
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
  activeGuestUserID: string | null

  // Guest-side (aguardando aprovação)
  joinResult: JoinResult | null
  isWaitingApproval: boolean
  wasRestoredSession: boolean

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
  setRestoredSession: (restored: boolean) => void
  setActiveGuestUserID: (guestUserID: string | null) => void

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
  activeGuestUserID: null,
  joinResult: null,
  isWaitingApproval: false,
  wasRestoredSession: false,
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
      pendingGuests: state.pendingGuests.some((g) => g.userID === guest.userID)
        ? state.pendingGuests.map((g) => (g.userID === guest.userID ? guest : g))
        : [...state.pendingGuests, guest],
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

  setRestoredSession: (restored) => set({ wasRestoredSession: restored }),

  setActiveGuestUserID: (guestUserID) => set({ activeGuestUserID: guestUserID }),

  setP2PConnected: (connected) => set({ isP2PConnected: connected }),

  setICEServers: (servers) => set({ iceServers: servers }),

  setLoading: (isLoading) => set({ isLoading }),

  setError: (error) => set({ error }),

  reset: () => set(initialState),
}))
