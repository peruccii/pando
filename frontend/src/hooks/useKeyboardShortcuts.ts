import { useEffect, useCallback } from 'react'
import { useLayoutStore } from '../features/command-center/stores/layoutStore'
import { useAppStore } from '../stores/appStore'
import { useWorkspaceStore } from '../stores/workspaceStore'
import { useBroadcastStore } from '../features/broadcast/stores/broadcastStore'
import { useSessionStore } from '../features/session/stores/sessionStore'
import type { ShortcutCategory, ShortcutId } from '../features/shortcuts/shortcuts'
import {
  SHORTCUT_DEFINITIONS,
  eventMatchesShortcutBinding,
  formatShortcutBinding,
  resolveShortcutBindings,
} from '../features/shortcuts/shortcuts'

interface RuntimeShortcut {
  id: ShortcutId
  key: string
  meta: boolean
  shift: boolean
  alt: boolean
  action: () => void
  description: string
  category: ShortcutCategory
}

/** Emitir evento customizado para toggles globais */
const emitEvent = (name: string, detail?: unknown) => {
  window.dispatchEvent(new CustomEvent(name, { detail }))
}

/** Estado do command palette (emitido como evento customizado) */
let commandPaletteOpen = false
const toggleCommandPalette = () => {
  commandPaletteOpen = !commandPaletteOpen
  window.dispatchEvent(new CustomEvent('command-palette:toggle', { detail: commandPaletteOpen }))
}

/**
 * useKeyboardShortcuts — hook global para atalhos de teclado.
 * Respeita foco do terminal: quando terminal está focado, Cmd+C/V/etc passam pro terminal.
 */
export function useKeyboardShortcuts() {
  // Layout actions
  const activePaneId = useLayoutStore((s) => s.activePaneId)
  const focusNextPane = useLayoutStore((s) => s.focusNextPane)
  const focusPrevPane = useLayoutStore((s) => s.focusPrevPane)
  const focusPaneByIndex = useLayoutStore((s) => s.focusPaneByIndex)
  const toggleZenMode = useLayoutStore((s) => s.toggleZenMode)
  const closePane = useWorkspaceStore((s) => s.closePane)

  // App actions
  const theme = useAppStore((s) => s.theme)
  const setTheme = useAppStore((s) => s.setTheme)
  const zoomInTerminal = useAppStore((s) => s.zoomInTerminal)
  const zoomOutTerminal = useAppStore((s) => s.zoomOutTerminal)
  const resetTerminalZoom = useAppStore((s) => s.resetTerminalZoom)
  const shortcutOverrides = useAppStore((s) => s.shortcutBindings)

  /** Verificar se o foco está no terminal (xterm) */
  const isTerminalFocused = useCallback(() => {
    const active = document.activeElement
    if (!active) return false
    return active.closest('.xterm') !== null || active.classList.contains('xterm-helper-textarea')
  }, [])

  /** Definição de todos os atalhos com bindings resolvidos */
  const getShortcuts = useCallback((): RuntimeShortcut[] => {
    const bindings = resolveShortcutBindings(shortcutOverrides)

    const actionByID: Record<ShortcutId, () => void> = {
      newTerminal: () => window.dispatchEvent(new CustomEvent('new-terminal:toggle')),
      closePane: () => {
        const sessionState = useSessionStore.getState()
        if (sessionState.role === 'guest' && sessionState.session?.id) {
          return
        }
        if (activePaneId) {
          closePane(activePaneId).catch((err) => {
            console.error('[Shortcuts] Failed to close pane:', err)
          })
        }
      },
      zenMode: () => toggleZenMode(),
      focusNextPane: () => focusNextPane(),
      focusPrevPane: () => focusPrevPane(),
      focusPane1: () => focusPaneByIndex(0),
      focusPane2: () => focusPaneByIndex(1),
      focusPane3: () => focusPaneByIndex(2),
      focusPane4: () => focusPaneByIndex(3),
      focusPane5: () => focusPaneByIndex(4),
      focusPane6: () => focusPaneByIndex(5),
      focusPane7: () => focusPaneByIndex(6),
      focusPane8: () => focusPaneByIndex(7),
      focusPane9: () => focusPaneByIndex(8),
      splitVertical: () => window.dispatchEvent(new CustomEvent('new-terminal:toggle')),
      splitHorizontal: () => window.dispatchEvent(new CustomEvent('new-terminal:toggle')),
      commandPalette: () => toggleCommandPalette(),
      toggleTheme: () => {
        const themes = ['dark', 'light', 'hacker', 'nvim', 'min-dark'] as const
        const idx = themes.indexOf(theme)
        setTheme(themes[(idx + 1) % themes.length])
      },
      toggleSidebar: () => emitEvent('sidebar:toggle'),
      toggleGitActivity: () => emitEvent('git-activity:toggle'),
      toggleBroadcast: () => useBroadcastStore.getState().toggle(),
      openSettings: () => emitEvent('settings:toggle'),
    }

    return SHORTCUT_DEFINITIONS.map((def) => {
      const binding = bindings[def.id]
      return {
        id: def.id,
        key: binding.key,
        meta: binding.meta,
        shift: binding.shift,
        alt: binding.alt,
        action: actionByID[def.id],
        description: def.description,
        category: def.category,
      }
    })
  }, [
    activePaneId,
    closePane,
    focusNextPane,
    focusPaneByIndex,
    focusPrevPane,
    setTheme,
    shortcutOverrides,
    theme,
    toggleZenMode,
  ])

  /** Handler global de keydown */
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Não interceptar se não tiver modifier key
      if (!e.metaKey && !e.ctrlKey) return

      // Zoom do terminal (Cmd/Ctrl +/-/0) é reservado.
      if (!e.altKey) {
        const isZoomIn = e.key === '+' || e.key === '=' || e.code === 'NumpadAdd'
        if (isZoomIn) {
          e.preventDefault()
          e.stopPropagation()
          zoomInTerminal()
          return
        }

        const isZoomOut = e.key === '-' || e.key === '_' || e.code === 'NumpadSubtract'
        if (isZoomOut) {
          e.preventDefault()
          e.stopPropagation()
          zoomOutTerminal()
          return
        }

        const isZoomReset = (e.key === '0' || e.code === 'Numpad0') && !e.shiftKey
        if (isZoomReset) {
          e.preventDefault()
          e.stopPropagation()
          resetTerminalZoom()
          return
        }
      }

      const shortcuts = getShortcuts()

      // Quando terminal está focado, permitir shortcuts padrão do terminal
      if (isTerminalFocused()) {
        if (['c', 'v', 'a', 'z'].includes(e.key.toLowerCase()) && (e.metaKey || e.ctrlKey) && !e.shiftKey) {
          return
        }
      }

      for (const shortcut of shortcuts) {
        if (eventMatchesShortcutBinding(e, shortcut)) {
          e.preventDefault()
          e.stopPropagation()
          shortcut.action()
          return
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown, true)
    return () => window.removeEventListener('keydown', handleKeyDown, true)
  }, [getShortcuts, isTerminalFocused, resetTerminalZoom, zoomInTerminal, zoomOutTerminal])

  return { getShortcuts }
}

/** Exportar lista de atalhos (bindings já resolvidos) */
export function getAllShortcuts() {
  const bindings = resolveShortcutBindings(useAppStore.getState().shortcutBindings)
  return SHORTCUT_DEFINITIONS.map((def) => ({
    id: def.id,
    key: formatShortcutBinding(bindings[def.id]),
    description: def.description,
    category: def.category,
    binding: bindings[def.id],
  }))
}

export function getShortcutLabel(id: ShortcutId): string {
  const bindings = resolveShortcutBindings(useAppStore.getState().shortcutBindings)
  return formatShortcutBinding(bindings[id])
}
