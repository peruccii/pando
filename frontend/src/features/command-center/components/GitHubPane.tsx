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
  const openGitPanel = () => {
    window.dispatchEvent(new CustomEvent('git-panel:open'))
  }

  return (
    <div
      className={`github-pane ${isActive ? 'github-pane--active' : ''}`}
      id={`github-${paneId}`}
    >
      <div className="github-pane__placeholder">
        <div className="github-pane__icon">🐙</div>
        <h3 className="github-pane__title">Git Panel</h3>
        <p className="github-pane__subtitle">
          Fluxo Git local migrou para tela dedicada.
        </p>
        <button className="btn btn--primary github-pane__cta" onClick={openGitPanel}>
          Abrir Git Panel
        </button>
        <div className="github-pane__features">
          <span className="badge badge--accent">Working Tree</span>
          <span className="badge badge--accent">Diff Viewer</span>
          <span className="badge badge--accent">History</span>
          <span className="badge badge--accent">Conflicts</span>
        </div>
      </div>
    </div>
  )
}
