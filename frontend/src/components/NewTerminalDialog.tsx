import { useState, useEffect, useCallback } from 'react'
import { Container, Laptop, AlertTriangle, X, Shield, Users } from 'lucide-react'
import { useWorkspaceStore } from '../stores/workspaceStore'
import { useSessionStore } from '../features/session/stores/sessionStore'
import './NewTerminalDialog.css'

export function NewTerminalDialog() {
  const [isOpen, setIsOpen] = useState(false)
  const [isDockerAvailable, setIsDockerAvailable] = useState(false)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)
  const createTerminalForActiveWorkspace = useWorkspaceStore((s) => s.createTerminalForActiveWorkspace)
  const sessionRole = useSessionStore((s) => s.role)
  const hasCollaborativeSession = useSessionStore((s) => Boolean(s.session?.id))
  const isGuestScoped = sessionRole === 'guest' && hasCollaborativeSession

  const checkDocker = useCallback(async () => {
    try {
      if (window.go?.main?.App?.DockerIsAvailable) {
        const available = await window.go.main.App.DockerIsAvailable()
        setIsDockerAvailable(available)
      } else {
        setIsDockerAvailable(false)
      }
    } catch (err) {
      console.warn('[NewTerminal] Failed to check Docker:', err)
      setIsDockerAvailable(false)
    }
  }, [])

  useEffect(() => {
    const handleToggle = () => {
      setIsOpen((v) => !v)
      if (!isOpen) {
        checkDocker()
        setErrorMessage(null)
      }
    }
    window.addEventListener('new-terminal:toggle', handleToggle)
    return () => window.removeEventListener('new-terminal:toggle', handleToggle)
  }, [isOpen, checkDocker])

  const handleSelect = async (useDocker: boolean) => {
    if (isGuestScoped) {
      setErrorMessage('Somente o host pode criar terminais nesta sessão colaborativa.')
      return
    }
    if (useDocker && !isDockerAvailable) return

    setErrorMessage(null)
    try {
      await createTerminalForActiveWorkspace(useDocker)
      setIsOpen(false)
    } catch (err) {
      console.error('[NewTerminal] Failed to create terminal:', err)
      setErrorMessage(err instanceof Error ? err.message : 'Falha ao criar terminal')
    }
  }

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (isOpen && e.key === 'Escape') {
        setIsOpen(false)
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [isOpen])

  if (!isOpen) return null

  return (
    <div className="new-terminal-dialog-backdrop" onClick={() => setIsOpen(false)}>
      <div className="new-terminal-dialog" onClick={(e) => e.stopPropagation()}>
        <div className="new-terminal-dialog__header">
          <div className="new-terminal-dialog__title-group">
            <h2 className="new-terminal-dialog__title">Novo Terminal</h2>
            <p className="new-terminal-dialog__subtitle">Escolha o ambiente para sua nova sessão</p>
          </div>
          <button className="new-terminal-dialog__close" onClick={() => setIsOpen(false)}>
            <X size={18} />
          </button>
        </div>
        
        <div className="new-terminal-dialog__options">
          {/* Option: Docker */}
          <div 
            className={`new-terminal-card new-terminal-card--recommended ${(!isDockerAvailable || isGuestScoped) ? 'new-terminal-card--disabled' : ''}`}
            onClick={() => isDockerAvailable && !isGuestScoped && handleSelect(true)}
          >
            <div className="new-terminal-card__icon-wrapper new-terminal-card__icon-wrapper--docker">
              <Container size={24} />
            </div>
            <div className="new-terminal-card__content">
              <div className="new-terminal-card__header">
                <span className="new-terminal-card__name">Docker Container</span>
                <div className="flex gap-2">
                  <span className="badge badge--accent">Recomendado</span>
                  {isDockerAvailable && <span className="badge badge--success">Isolado</span>}
                </div>
              </div>
              <p className="new-terminal-card__desc">
                Ambiente sandbox Linux totalmente isolado. Ideal para rodar códigos de terceiros com segurança total.
              </p>
              
              <div className="new-terminal-card__features">
                <div className="new-terminal-card__feature">
                  <Shield size={12} />
                  <span>Acesso Root</span>
                </div>
                <div className="new-terminal-card__feature">
                  <Users size={12} />
                  <span>Colaboração Full</span>
                </div>
              </div>

              {!isDockerAvailable && (
                <div className="new-terminal-card__warning">
                  <AlertTriangle size={14} />
                  <div className="new-terminal-card__warning-text">
                    Docker não detectado. 
                    <a 
                      href="https://www.docker.com/products/docker-desktop/" 
                      target="_blank" 
                      rel="noopener noreferrer"
                      onClick={(e) => e.stopPropagation()}
                    >
                      Instalar
                    </a>
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* Option: Local */}
          <div 
            className={`new-terminal-card ${isGuestScoped ? 'new-terminal-card--disabled' : ''}`}
            onClick={() => !isGuestScoped && handleSelect(false)}
          >
            <div className="new-terminal-card__icon-wrapper new-terminal-card__icon-wrapper--local">
              <Laptop size={24} />
            </div>
            <div className="new-terminal-card__content">
              <div className="new-terminal-card__header">
                <span className="new-terminal-card__name">Terminal Local</span>
                <span className="badge badge--info">Performance</span>
              </div>
              <p className="new-terminal-card__desc">
                Roda nativamente no seu macOS. Acesso direto aos seus arquivos e ferramentas locais.
              </p>
              
              <div className="new-terminal-card__features">
                <div className="new-terminal-card__feature">
                  <Shield size={12} />
                  <span>Read-Only p/ Convidados</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        {errorMessage && (
          <div className="new-terminal-dialog__error-box">
            <AlertTriangle size={16} />
            <span>{errorMessage}</span>
          </div>
        )}

        <div className="new-terminal-dialog__footer">
          <button className="btn btn--ghost" onClick={() => setIsOpen(false)}>
            Cancelar
          </button>
        </div>
      </div>
    </div>
  )
}
