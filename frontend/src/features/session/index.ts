// Feature: Session / P2P Collaboration
export { SessionPanel } from './components/SessionPanel'
export { JoinSessionDialog } from './components/JoinSessionDialog'
export { useSession } from './hooks/useSession'
export { useSessionStore } from './stores/sessionStore'
export { P2PConnection, DATA_CHANNELS } from './services/P2PConnection'
export type {
  Session,
  SessionGuest,
  SessionConfig,
  SessionStatus,
  GuestStatus,
  Permission,
  SessionMode,
  SessionRole,
  GuestRequest,
  JoinResult,
  ICEServerConfig,
} from './stores/sessionStore'
export type { DataChannelName, DataChannelMessage } from './services/P2PConnection'
