import { useCallback } from 'react'
import { useLayoutStore } from '../stores/layoutStore'

/**
 * Hook para gerenciamento do Zen Mode (maximizar painel).
 */
export function useZenMode() {
  const zenModePane = useLayoutStore((s) => s.zenModePane)
  const enterZenMode = useLayoutStore((s) => s.enterZenMode)
  const exitZenMode = useLayoutStore((s) => s.exitZenMode)
  const toggleZenMode = useLayoutStore((s) => s.toggleZenMode)

  const isZenMode = zenModePane !== null

  /** Verificar se um painel específico está em zen mode */
  const isPaneInZenMode = useCallback(
    (paneId: string) => zenModePane === paneId,
    [zenModePane]
  )

  return {
    isZenMode,
    zenModePane,
    isPaneInZenMode,
    enterZenMode,
    exitZenMode,
    toggleZenMode,
  }
}
