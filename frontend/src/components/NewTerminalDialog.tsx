import { useState, useEffect, useCallback } from 'react'
import { Container, Laptop, AlertTriangle } from 'lucide-react'
import { useWorkspaceStore } from '../stores/workspaceStore'
import './NewTerminalDialog.css'

export function NewTerminalDialog() {
  const [isOpen, setIsOpen] = useState(false)
  const [isDockerAvailable, setIsDockerAvailable] = useState(false)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)
  const createTerminalForActiveWorkspace = useWorkspaceStore((s) => s.createTerminalForActiveWorkspace)

  const checkDocker = useCallback(async () => {
    try {
      if (window.go?.main?.App?.DockerIsAvailable) {
        const available = await window.go.main.App.DockerIsAvailable()
        setIsDockerAvailable(available)
      } else {
        // Fallback for dev without backend
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
      }
    }
    window.addEventListener('new-terminal:toggle', handleToggle)
    return () => window.removeEventListener('new-terminal:toggle', handleToggle)
  }, [isOpen, checkDocker])

  const handleSelect = async (useDocker: boolean) => {
    if (useDocker && !isDockerAvailable) return

    setErrorMessage(null)
    try {
      await createTerminalForActiveWorkspace(useDocker)
      setIsOpen(false)
    } catch (err) {
      console.error('[NewTerminal] Failed to create terminal:', err)
      setErrorMessage(err instanceof Error ? err.message : 'falha ao criar terminal')
    }
  }

  // Close on Escape
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
        <h2 className="new-terminal-dialog__title">Novo Terminal</h2>
        
        <div className="new-terminal-dialog__options">
          {/* Option: Docker */}
          <button 
            className={`new-terminal-option ${!isDockerAvailable ? 'new-terminal-option--disabled' : ''}`}
            onClick={() => handleSelect(true)}
            disabled={!isDockerAvailable}
            title={!isDockerAvailable ? "Docker não detectado ou não iniciado" : "Terminal isolado em container"}
          >
            <div className="new-terminal-option__header">
              <Container size={20} className={isDockerAvailable ? "text-success" : "text-muted"} />
              <span>Docker Container</span>
              {isDockerAvailable && <span className="new-terminal-option__badge new-terminal-option__badge--secure">Seguro</span>}
            </div>
            <p className="new-terminal-option__desc">
              Terminal isolado do seu sistema. Guest tem permissão total (root) dentro do container.
            </p>
            {!isDockerAvailable && (
              <div className="text-xs text-warning flex items-center gap-1 mt-1">
                <AlertTriangle size={12} />
                Docker não disponível. 
                <a 
                  href="https://www.docker.com/products/docker-desktop/" 
                  target="_blank" 
                  rel="noopener noreferrer"
                  className="text-accent hover:underline ml-1"
                  onClick={(e) => e.stopPropagation()}
                >
                  Instalar Docker
                </a>
              </div>
            )}
          </button>

          {/* Option: Local */}
          <button 
            className="new-terminal-option"
            onClick={() => handleSelect(false)}
            title="Terminal direto no seu macOS"
          >
            <div className="new-terminal-option__header">
              <Laptop size={20} className="text-info" />
              <span>Local Terminal</span>
              <span className="new-terminal-option__badge new-terminal-option__badge--local">Live Share</span>
            </div>
            <p className="new-terminal-option__desc">
              Roda direto no seu macOS. Guests entram como Read-Only por segurança.
            </p>
          </button>
        </div>

        {errorMessage && (
          <p className="new-terminal-dialog__error">
            {errorMessage}
          </p>
        )}
      </div>
    </div>
  )
}
