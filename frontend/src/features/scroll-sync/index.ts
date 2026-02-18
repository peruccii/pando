// Scroll Sync Feature — Collaborative Diff Navigation

// Types
export type {
  ScrollSyncEvent,
  ScrollSyncAction,
  ScrollSyncSettings,
  ScrollSyncDebounceState,
} from './types'

export {
  DEFAULT_SCROLL_SYNC_SETTINGS,
  USER_COLORS,
  getUserColor,
} from './types'

// Hooks
export { useScrollSync } from './hooks/useScrollSync'
export { useScrollSyncHandler } from './hooks/useScrollSyncHandler'
export { useScrollSyncIntegration } from './hooks/useScrollSyncIntegration'

// Components
export { ScrollSyncSettingsPanel } from './components/ScrollSyncSettingsPanel'
export { ScrollSyncToastContainer } from './components/ScrollSyncToast'
