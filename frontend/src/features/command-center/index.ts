// Barrel exports para features/command-center

// Components
export { CommandCenter } from './components/CommandCenter'
export { PaneHeader } from './components/PaneHeader'
export { TerminalPane } from './components/TerminalPane'
export { AIAgentPane } from './components/AIAgentPane'
export { ZenModeOverlay } from './components/ZenModeOverlay'

// Hooks
export { useLayout } from './hooks/useLayout'
export { usePaneFocus } from './hooks/usePaneFocus'
export { useZenMode } from './hooks/useZenMode'

// Store
export { useLayoutStore } from './stores/layoutStore'

// Types
export type { PaneInfo, PaneStatus, PaneType, LayoutRule, TerminalTheme } from './types/layout'
export { LAYOUT_RULES, TERMINAL_THEMES } from './types/layout'
