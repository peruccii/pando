import { useEffect, useCallback } from 'react'
import { useLayoutStore } from '../stores/layoutStore'
import { useZenMode } from '../hooks/useZenMode'
import { TerminalPane } from './TerminalPane'
import { AIAgentPane } from './AIAgentPane'
import { GitHubPane } from './GitHubPane'
import { PaneHeader } from './PaneHeader'
import './ZenModeOverlay.css'

interface ZenModeOverlayProps {
  paneId: string
}

/**
 * ZenModeOverlay — overlay de tela cheia para um painel.
 * Usa z-index alto para sobrepor tudo.
 */
export function ZenModeOverlay({ paneId }: ZenModeOverlayProps) {
  const panes = useLayoutStore((s) => s.panes)
  const { exitZenMode } = useZenMode()

  const pane = panes[paneId]

  /** Escape para sair */
  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      exitZenMode()
    }
  }, [exitZenMode])

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown, true)
    return () => document.removeEventListener('keydown', handleKeyDown, true)
  }, [handleKeyDown])

  if (!pane) return null

  return (
    <div className="zen-mode-overlay animate-fade-in" id="zen-mode-overlay">
      <div className="zen-mode-overlay__header">
        <PaneHeader
          paneId={paneId}
          title={pane.title}
          status={pane.status}
          type={pane.type}
          isActive={true}
        />
      </div>
      <div className="zen-mode-overlay__content">
        {pane.type === 'terminal' && (
          <TerminalPane paneId={`${paneId}-zen`} isActive={true} />
        )}
        {pane.type === 'ai_agent' && (
          <AIAgentPane paneId={`${paneId}-zen`} isActive={true} />
        )}
        {pane.type === 'github' && (
          <GitHubPane paneId={`${paneId}-zen`} isActive={true} />
        )}
      </div>
    </div>
  )
}
