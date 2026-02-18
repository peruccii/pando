import { useCallback } from 'react'
import { useLayoutStore } from '../stores/layoutStore'

/**
 * Hook para gerenciamento de foco entre painéis.
 * Garante hierarquia visual (ativo vs inativo) e navegação.
 */
export function usePaneFocus() {
  const activePaneId = useLayoutStore((s) => s.activePaneId)
  const setActivePaneId = useLayoutStore((s) => s.setActivePaneId)
  const focusNextPane = useLayoutStore((s) => s.focusNextPane)
  const focusPrevPane = useLayoutStore((s) => s.focusPrevPane)
  const focusPaneByIndex = useLayoutStore((s) => s.focusPaneByIndex)
  const paneOrder = useLayoutStore((s) => s.paneOrder)

  /** Verificar se um painel está ativo */
  const isPaneActive = useCallback(
    (paneId: string) => activePaneId === paneId,
    [activePaneId]
  )

  /** Handler de clique para focar um painel */
  const handlePaneClick = useCallback(
    (paneId: string) => {
      setActivePaneId(paneId)
    },
    [setActivePaneId]
  )

  /** Focar painel por índice (1-based para atalhos Cmd+1-9) */
  const focusByNumber = useCallback(
    (num: number) => {
      focusPaneByIndex(num - 1)
    },
    [focusPaneByIndex]
  )

  return {
    activePaneId,
    paneOrder,
    isPaneActive,
    handlePaneClick,
    setActivePaneId,
    focusNextPane,
    focusPrevPane,
    focusByNumber,
  }
}
