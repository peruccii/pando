import { create } from 'zustand'
import type { ScrollSyncSettings } from '../features/scroll-sync/types'
import { DEFAULT_SCROLL_SYNC_SETTINGS } from '../features/scroll-sync/types'
import type { ShortcutBinding, ShortcutBindingOverrides, ShortcutId } from '../features/shortcuts/shortcuts'
import {
  parseShortcutBindingsJSON,
  serializeShortcutBindingsJSON,
  toShortcutOverride,
} from '../features/shortcuts/shortcuts'

export type Theme = 'dark' | 'light' | 'hacker' | 'nvim' | 'min-dark'
export type Language = 'pt-BR' | 'en-US'
export type TerminalCursorStyle = 'line' | 'block' | 'underline'
export const DEFAULT_TERMINAL_FONT_SIZE = 14
export const MIN_TERMINAL_FONT_SIZE = 10
export const MAX_TERMINAL_FONT_SIZE = 24
export const DEFAULT_TERMINAL_FONT_FAMILY = 'JetBrains Mono'
export const DEFAULT_TERMINAL_CURSOR_STYLE: TerminalCursorStyle = 'line'

export interface Workspace {
  id: number
  userId: string
  name: string
  path: string
  gitRemote?: string
  owner?: string
  repo?: string
  isActive: boolean
  lastOpenedAt?: string
}

export interface AppState {
  // App info
  version: string
  isReady: boolean

  // Theme
  theme: Theme
  language: Language
  defaultShell: string
  terminalFontSize: number
  terminalFontFamily: string
  terminalCursorStyle: TerminalCursorStyle
  onboardingCompleted: boolean
  shortcutBindings: ShortcutBindingOverrides

  // Workspaces
  workspaces: Workspace[]
  activeWorkspace: Workspace | null

  // Scroll Sync Settings
  scrollSyncSettings: ScrollSyncSettings
}

interface AppActions {
  setReady: (ready: boolean) => void
  setTheme: (theme: Theme) => void
  setLanguage: (language: Language) => void
  setDefaultShell: (shell: string) => void
  setTerminalFontSize: (size: number) => void
  setTerminalFontFamily: (family: string) => void
  setTerminalCursorStyle: (style: TerminalCursorStyle) => void
  zoomInTerminal: () => void
  zoomOutTerminal: () => void
  resetTerminalZoom: () => void
  setShortcutBinding: (id: ShortcutId, binding: ShortcutBinding) => void
  resetShortcutBindings: () => void
  completeOnboarding: () => void
  setWorkspaces: (workspaces: Workspace[]) => void
  setActiveWorkspace: (workspace: Workspace | null) => void
  setScrollSyncSettings: (settings: ScrollSyncSettings) => void
  hydrate: (payload: HydrationPayload) => void
}

export interface HydrationPayload {
  isAuthenticated: boolean
  user?: {
    id: string
    email: string
    name: string
    username?: string
    avatarUrl?: string
    provider: string
  }
  theme: string
  language?: string
  defaultShell?: string
  terminalFontSize?: number
  terminalFontFamily?: string
  terminalCursorStyle?: string
  onboardingCompleted?: boolean
  shortcutBindings?: string
  version: string
  workspaces?: Workspace[]
}

const normalizeTheme = (theme: string | undefined): Theme => {
  if (theme === 'light' || theme === 'hacker' || theme === 'nvim' || theme === 'min-dark') {
    return theme
  }
  return 'dark'
}

const normalizeLanguage = (language: string | undefined): Language => {
  if (language === 'en-US' || language?.toLowerCase() === 'en-us') {
    return 'en-US'
  }
  return 'pt-BR'
}

const normalizeTerminalFontSize = (size: number | undefined): number => {
  if (!Number.isFinite(size)) {
    return DEFAULT_TERMINAL_FONT_SIZE
  }

  const rounded = Math.round(size as number)
  if (rounded < MIN_TERMINAL_FONT_SIZE) return MIN_TERMINAL_FONT_SIZE
  if (rounded > MAX_TERMINAL_FONT_SIZE) return MAX_TERMINAL_FONT_SIZE
  return rounded
}

const normalizeTerminalFontFamily = (family: string | undefined): string => {
  const normalized = (family || '').trim()
  return normalized.length > 0 ? normalized : DEFAULT_TERMINAL_FONT_FAMILY
}

const normalizeTerminalCursorStyle = (style: string | undefined): TerminalCursorStyle => {
  const normalized = (style || '').trim().toLowerCase()
  if (normalized === 'block') return 'block'
  if (normalized === 'underline') return 'underline'
  return DEFAULT_TERMINAL_CURSOR_STYLE
}

export const useAppStore = create<AppState & AppActions>((set, get) => ({
  // State
  version: '1.0.0',
  isReady: false,
  theme: 'dark',
  language: 'pt-BR',
  defaultShell: '',
  terminalFontSize: DEFAULT_TERMINAL_FONT_SIZE,
  terminalFontFamily: DEFAULT_TERMINAL_FONT_FAMILY,
  terminalCursorStyle: DEFAULT_TERMINAL_CURSOR_STYLE,
  onboardingCompleted: false,
  shortcutBindings: {},
  workspaces: [],
  activeWorkspace: null,
  scrollSyncSettings: DEFAULT_SCROLL_SYNC_SETTINGS,

  // Actions
  setReady: (isReady) => set({ isReady }),

  setTheme: (theme) => {
    const normalizedTheme = normalizeTheme(theme)
    document.documentElement.setAttribute('data-theme', normalizedTheme)
    set({ theme: normalizedTheme })

    if (window.go?.main?.App?.SaveTheme) {
      window.go.main.App.SaveTheme(normalizedTheme).catch((err: unknown) => {
        console.warn('[AppStore] Failed to persist theme:', err)
      })
    }
  },

  setLanguage: (language) => {
    const normalizedLanguage = normalizeLanguage(language)
    document.documentElement.lang = normalizedLanguage
    set({ language: normalizedLanguage })

    if (window.go?.main?.App?.SaveLanguage) {
      window.go.main.App.SaveLanguage(normalizedLanguage).catch((err: unknown) => {
        console.warn('[AppStore] Failed to persist language:', err)
      })
    }
  },

  setDefaultShell: (shell) => {
    set({ defaultShell: shell })

    if (window.go?.main?.App?.SaveDefaultShell) {
      window.go.main.App.SaveDefaultShell(shell).catch((err: unknown) => {
        console.warn('[AppStore] Failed to persist default shell:', err)
      })
    }
  },

  setTerminalFontSize: (size) => {
    const next = normalizeTerminalFontSize(size)
    const current = get().terminalFontSize
    if (current === next) {
      return
    }

    set({ terminalFontSize: next })

    const appBindings = window.go?.main?.App as { SaveTerminalFontSize?: (value: number) => Promise<void> } | undefined
    appBindings?.SaveTerminalFontSize?.(next).catch((err: unknown) => {
      console.warn('[AppStore] Failed to persist terminal font size:', err)
    })
  },

  setTerminalFontFamily: (family) => {
    const next = normalizeTerminalFontFamily(family)
    const current = get().terminalFontFamily
    if (current === next) {
      return
    }

    set({ terminalFontFamily: next })

    const appBindings = window.go?.main?.App as { SaveTerminalFontFamily?: (value: string) => Promise<void> } | undefined
    appBindings?.SaveTerminalFontFamily?.(next).catch((err: unknown) => {
      console.warn('[AppStore] Failed to persist terminal font family:', err)
    })
  },

  setTerminalCursorStyle: (style) => {
    const next = normalizeTerminalCursorStyle(style)
    const current = get().terminalCursorStyle
    if (current === next) {
      return
    }

    set({ terminalCursorStyle: next })

    const appBindings = window.go?.main?.App as { SaveTerminalCursorStyle?: (value: TerminalCursorStyle) => Promise<void> } | undefined
    appBindings?.SaveTerminalCursorStyle?.(next).catch((err: unknown) => {
      console.warn('[AppStore] Failed to persist terminal cursor style:', err)
    })
  },

  zoomInTerminal: () => {
    const current = get().terminalFontSize
    get().setTerminalFontSize(current + 1)
  },

  zoomOutTerminal: () => {
    const current = get().terminalFontSize
    get().setTerminalFontSize(current - 1)
  },

  resetTerminalZoom: () => {
    get().setTerminalFontSize(DEFAULT_TERMINAL_FONT_SIZE)
  },

  setShortcutBinding: (id, binding) => {
    const currentOverrides = get().shortcutBindings
    const nextOverrides = toShortcutOverride(currentOverrides, id, binding)
    if (serializeShortcutBindingsJSON(currentOverrides) === serializeShortcutBindingsJSON(nextOverrides)) {
      return
    }

    set({ shortcutBindings: nextOverrides })

    const appBindings = window.go?.main?.App as { SaveShortcutBindings?: (value: string) => Promise<void> } | undefined
    appBindings?.SaveShortcutBindings?.(serializeShortcutBindingsJSON(nextOverrides)).catch((err: unknown) => {
      console.warn('[AppStore] Failed to persist shortcut bindings:', err)
    })
  },

  resetShortcutBindings: () => {
    set({ shortcutBindings: {} })

    const appBindings = window.go?.main?.App as { SaveShortcutBindings?: (value: string) => Promise<void> } | undefined
    appBindings?.SaveShortcutBindings?.('{}').catch((err: unknown) => {
      console.warn('[AppStore] Failed to reset shortcut bindings:', err)
    })
  },

  completeOnboarding: () => {
    set({ onboardingCompleted: true })

    if (window.go?.main?.App?.CompleteOnboarding) {
      window.go.main.App.CompleteOnboarding().catch((err: unknown) => {
        console.warn('[AppStore] Failed to persist onboarding state:', err)
      })
    }
  },

  setWorkspaces: (workspaces) => set({ workspaces }),

  setActiveWorkspace: (workspace) => set({ activeWorkspace: workspace }),

  setScrollSyncSettings: (scrollSyncSettings) => set({ scrollSyncSettings }),

  hydrate: (payload) => {
    // Aplicar tema
    const theme = normalizeTheme(payload.theme)
    const language = normalizeLanguage(payload.language)
    document.documentElement.setAttribute('data-theme', theme)
    document.documentElement.lang = language

    const activeWs = payload.workspaces?.find((w) => w.isActive) ?? null

    set({
      version: payload.version || '1.0.0',
      theme,
      language,
      defaultShell: payload.defaultShell || '',
      terminalFontSize: normalizeTerminalFontSize(payload.terminalFontSize),
      terminalFontFamily: normalizeTerminalFontFamily(payload.terminalFontFamily),
      terminalCursorStyle: normalizeTerminalCursorStyle(payload.terminalCursorStyle),
      onboardingCompleted: payload.onboardingCompleted ?? false,
      shortcutBindings: parseShortcutBindingsJSON(payload.shortcutBindings),
      workspaces: payload.workspaces || [],
      activeWorkspace: activeWs,
      isReady: true,
    })
  },
}))
