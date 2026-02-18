import './GitHubPane.css'

interface GitHubPaneProps {
  paneId: string
  isActive: boolean
}

/**
 * GitHubPane — painel de integração GitHub.
 * Será populado na Fase 2 com PRs, Issues, Branches.
 * Por ora, exibe um placeholder informativo.
 */
export function GitHubPane({ paneId, isActive }: GitHubPaneProps) {
  return (
    <div
      className={`github-pane ${isActive ? 'github-pane--active' : ''}`}
      id={`github-${paneId}`}
    >
      <div className="github-pane__placeholder">
        <div className="github-pane__icon">🐙</div>
        <h3 className="github-pane__title">GitHub</h3>
        <p className="github-pane__subtitle">
          Integração GitHub será implementada na Fase 2
        </p>
        <div className="github-pane__features">
          <span className="badge badge--accent">Pull Requests</span>
          <span className="badge badge--accent">Diff Viewer</span>
          <span className="badge badge--accent">Issues</span>
          <span className="badge badge--accent">Branches</span>
        </div>
      </div>
    </div>
  )
}
