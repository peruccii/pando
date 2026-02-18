// Scroll Sync Types — Collaborative Diff Navigation

export type ScrollSyncAction = 'comment' | 'navigate' | 'review'

export interface ScrollSyncEvent {
  type: 'scroll_sync'
  file: string          // Path relativo do arquivo
  line: number          // Linha no diff
  userID: string
  userName: string
  userColor: string     // Cor do cursor de awareness
  action: ScrollSyncAction
  timestamp: number
}

// Settings para Scroll Sync
export interface ScrollSyncSettings {
  enabled: boolean
  autoFollow: boolean
  showToast: boolean
}

// Estado do debounce por usuário
export interface ScrollSyncDebounceState {
  [userID: string]: number // timestamp do último evento
}

export const DEFAULT_SCROLL_SYNC_SETTINGS: ScrollSyncSettings = {
  enabled: true,
  autoFollow: true,
  showToast: true,
}

// Cores para usuários (ciclo)
export const USER_COLORS = [
  '#ef4444', // red
  '#f97316', // orange
  '#f59e0b', // amber
  '#84cc16', // lime
  '#10b981', // emerald
  '#06b6d4', // cyan
  '#3b82f6', // blue
  '#8b5cf6', // violet
  '#d946ef', // fuchsia
  '#f43f5e', // rose
]

export function getUserColor(userID: string): string {
  let hash = 0
  for (let i = 0; i < userID.length; i++) {
    hash = ((hash << 5) - hash) + userID.charCodeAt(i)
    hash = hash & hash // Convert to 32bit integer
  }
  const index = Math.abs(hash) % USER_COLORS.length
  return USER_COLORS[index]
}
