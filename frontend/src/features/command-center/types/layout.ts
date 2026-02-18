// Layout types for the Command Center mosaic grid

import type { MosaicNode } from 'react-mosaic-component'

/** Status de um painel/agente */
export type PaneStatus = 'idle' | 'running' | 'error'

/** Tipo de painel */
export type PaneType = 'terminal' | 'ai_agent' | 'github'
export type PaneDropPosition = 'left' | 'right' | 'top' | 'bottom' | 'center'

/** Informações de um painel no grid */
export interface PaneInfo {
  id: string
  title: string
  type: PaneType
  status: PaneStatus
  sessionID?: string       // ID da sessão PTY (para terminais)
  agentDBID?: number       // ID no banco (AgentInstance)
  workspaceID?: number     // ID do workspace (aba) proprietário
  isMinimized: boolean
  config?: Record<string, any> // Configurações específicas do painel (ex: useDocker)
}

/** Regras de layout */
export interface LayoutRule {
  minPaneWidth: number     // 300px
  minPaneHeight: number    // 200px
  gutterSize: number       // 6px
  headerHeight: number     // 28px
  padding: number          // 2px
}

/** Estado completo do layout */
export interface LayoutState {
  panes: Record<string, PaneInfo>
  paneOrder: string[]
  mosaicNode: MosaicNode<string> | null
  activePaneId: string | null
  zenModePane: string | null
}

/** Constantes de layout */
export const LAYOUT_RULES: LayoutRule = {
  minPaneWidth: 300,
  minPaneHeight: 200,
  gutterSize: 6,
  headerHeight: 28,
  padding: 2,
}

/** Terminal theme configs para xterm.js */
export interface TerminalTheme {
  background: string
  foreground: string
  cursor: string
  selectionBackground: string
  black: string
  red: string
  green: string
  yellow: string
  blue: string
  magenta: string
  cyan: string
  white: string
  brightBlack: string
  brightRed: string
  brightGreen: string
  brightYellow: string
  brightBlue: string
  brightMagenta: string
  brightCyan: string
  brightWhite: string
}

const ANSI_NEUTRAL_DARK = {
  black: '#15161e',
  red: '#f7768e',
  green: '#9ece6a',
  yellow: '#e0af68',
  blue: '#7aa2f7',
  magenta: '#bb9af7',
  cyan: '#7dcfff',
  white: '#a9b1d6',
  brightBlack: '#414868',
  brightRed: '#f7768e',
  brightGreen: '#9ece6a',
  brightYellow: '#e0af68',
  brightBlue: '#7aa2f7',
  brightMagenta: '#bb9af7',
  brightCyan: '#7dcfff',
  brightWhite: '#c0caf5',
}

/** Temas de terminal pré-definidos */
export const TERMINAL_THEMES: Record<string, TerminalTheme> = {
  dark: {
    background: '#0f0f14',
    foreground: '#c0caf5',
    cursor: '#c0caf5',
    selectionBackground: 'rgba(122, 162, 247, 0.3)',
    ...ANSI_NEUTRAL_DARK,
  },
  light: {
    background: '#ffffff',
    foreground: '#383a42',
    cursor: '#383a42',
    selectionBackground: 'rgba(74, 108, 247, 0.2)',
    ...ANSI_NEUTRAL_DARK,
  },
  hacker: {
    background: '#0a0a0a',
    foreground: '#00ff41',
    cursor: '#00ff41',
    selectionBackground: 'rgba(0, 255, 65, 0.2)',
    ...ANSI_NEUTRAL_DARK,
  },
  nvim: {
    background: '#121212',
    foreground: '#c0caf5',
    cursor: '#c0caf5',
    selectionBackground: 'rgba(209, 154, 102, 0.25)',
    ...ANSI_NEUTRAL_DARK,
  },
  'min-dark': {
    background: '#0f1115',
    foreground: '#e5e7eb',
    cursor: '#e5e7eb',
    selectionBackground: 'rgba(147, 197, 253, 0.22)',
    ...ANSI_NEUTRAL_DARK,
  },
}
