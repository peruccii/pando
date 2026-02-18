import './AIAgentPane.css'

interface AIAgentPaneProps {
  paneId: string
  isActive: boolean
}

/**
 * AIAgentPane — painel de agente de IA.
 * Será populado na Fase 4 com o Motor de IA.
 * Por ora, exibe um placeholder informativo.
 */
export function AIAgentPane({ paneId, isActive }: AIAgentPaneProps) {
  return (
    <div
      className={`ai-agent-pane ${isActive ? 'ai-agent-pane--active' : ''}`}
      id={`ai-agent-${paneId}`}
    >
      <div className="ai-agent-pane__placeholder">
        <div className="ai-agent-pane__icon">🤖</div>
        <h3 className="ai-agent-pane__title">AI Agent</h3>
        <p className="ai-agent-pane__subtitle">
          Motor de IA será implementado na Fase 4
        </p>
        <div className="ai-agent-pane__features">
          <span className="badge badge--info">Context-Aware</span>
          <span className="badge badge--info">Streaming</span>
          <span className="badge badge--info">Multi-Provider</span>
        </div>
      </div>
    </div>
  )
}
