import { useCallback } from 'react'
import { useLayoutStore } from '../stores/layoutStore'

/**
 * Hook para gerenciamento de layout do Command Center.
 * Expõe ações de alto nível para manipulação de painéis.
 */
export function useLayout() {
  const addPane = useLayoutStore((s) => s.addPane)
  const removePane = useLayoutStore((s) => s.removePane)
  const activePaneId = useLayoutStore((s) => s.activePaneId)
  const panes = useLayoutStore((s) => s.panes)
  const paneOrder = useLayoutStore((s) => s.paneOrder)
  const mosaicNode = useLayoutStore((s) => s.mosaicNode)
  const setMosaicNode = useLayoutStore((s) => s.setMosaicNode)

  /** Criar novo painel de terminal (abre diálogo de seleção) */
  const newTerminal = useCallback(() => {
    window.dispatchEvent(new CustomEvent('new-terminal:toggle'))
    return undefined
  }, [])

  /** Criar novo painel de AI Agent */
  const newAIAgent = useCallback((title?: string) => {
    return addPane('ai_agent', title)
  }, [addPane])

  /** Abrir tela dedicada do Git Panel */
  const newGitHub = useCallback((title?: string) => {
    void title // compatibilidade de assinatura
    window.dispatchEvent(new CustomEvent('git-panel:open'))
    return undefined
  }, [])

  /** Remove o painel ativo */
  const closeActivePane = useCallback(() => {
    if (activePaneId) {
      removePane(activePaneId)
    }
  }, [activePaneId, removePane])

  /** Número total de painéis */
  const paneCount = paneOrder.length

  /** Existe algum painel? */
  const hasPanes = paneCount > 0

  return {
    // State
    panes,
    paneOrder,
    paneCount,
    hasPanes,
    activePaneId,
    mosaicNode,

    // Actions
    newTerminal,
    newAIAgent,
    newGitHub,
    addPane,
    removePane,
    closeActivePane,
    setMosaicNode,
  }
}
