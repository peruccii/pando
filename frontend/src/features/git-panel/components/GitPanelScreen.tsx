import { useEffect, useCallback } from 'react'
import { ArrowLeft, GitBranch, GitCommitHorizontal, GitMerge } from 'lucide-react'
import './GitPanelScreen.css'

interface GitPanelScreenProps {
  onBack: () => void
}

/**
 * GitPanelScreen — tela dedicada para Source Control local.
 * Mantém os terminais no background sem perder estado.
 */
export function GitPanelScreen({ onBack }: GitPanelScreenProps) {
  const handleEsc = useCallback((event: KeyboardEvent) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      onBack()
    }
  }, [onBack])

  useEffect(() => {
    document.addEventListener('keydown', handleEsc, true)
    return () => document.removeEventListener('keydown', handleEsc, true)
  }, [handleEsc])

  return (
    <section className="git-panel-screen" id="git-panel-screen" aria-label="Git Panel">
      <header className="git-panel-screen__header">
        <button
          className="btn btn--ghost git-panel-screen__back"
          onClick={onBack}
          aria-label="Voltar para o Command Center"
          title="Voltar (Esc)"
        >
          <ArrowLeft size={14} />
          Voltar
        </button>
        <div className="git-panel-screen__title-wrap">
          <h1 className="git-panel-screen__title">Git Panel</h1>
          <p className="git-panel-screen__subtitle">Source Control local em tela dedicada</p>
        </div>
      </header>

      <div className="git-panel-screen__content">
        <div className="git-panel-screen__card">
          <h2>Em implementação</h2>
          <p>
            Esta tela será o host oficial do fluxo Git local (status, histórico, diff,
            staging parcial e conflitos), separado dos terminais.
          </p>
          <div className="git-panel-screen__chips">
            <span className="badge badge--accent"><GitBranch size={12} /> Status</span>
            <span className="badge badge--accent"><GitCommitHorizontal size={12} /> History</span>
            <span className="badge badge--accent"><GitMerge size={12} /> Conflicts</span>
          </div>
        </div>
      </div>
    </section>
  )
}
