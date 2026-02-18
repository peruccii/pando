import { Github, Chrome, X, Eye } from 'lucide-react'
import './AuthGuard.css'

interface LoginPromptProps {
  /** Ação que o usuário tentou executar */
  action?: string
  /** Fechar o modal */
  onClose: () => void
}

/**
 * LoginPrompt — Modal contextual que aparece quando um
 * usuário não autenticado tenta executar uma ação protegida.
 *
 * Oferece opções de login (GitHub, Google) e "Continuar sem login".
 */
import { AuthLogin } from '../../wailsjs/go/main/App'

// ... (imports remain)

export function LoginPrompt({ action, onClose }: LoginPromptProps) {
  const handleGitHubLogin = async () => {
    try {
      console.log('[Auth] GitHub login requested...')
      await AuthLogin('github')
      onClose()
    } catch (err) {
      console.error('[Auth] Login failed:', err)
    }
  }

  const handleGoogleLogin = async () => {
    try {
      console.log('[Auth] Google login requested...')
      await AuthLogin('google')
      onClose()
    } catch (err) {
      console.error('[Auth] Login failed:', err)
    }
  }

  return (
    <div className="login-prompt__overlay" onClick={onClose}>
      <div
        className="login-prompt__modal"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="login-prompt__header">
          <div className="login-prompt__icon">🔐</div>
          <h3 className="login-prompt__title">Login necessário</h3>
          <button
            className="login-prompt__close"
            onClick={onClose}
            aria-label="Fechar"
          >
            <X size={16} />
          </button>
        </div>

        {/* Body */}
        <div className="login-prompt__body">
          <p className="login-prompt__message">
            {action
              ? `Para ${action.toLowerCase()}, você precisa estar conectado ao GitHub.`
              : 'Para realizar esta ação, você precisa estar autenticado.'}
          </p>

          {/* Login Buttons */}
          <div className="login-prompt__actions">
            <button
              className="login-prompt__btn login-prompt__btn--github"
              onClick={handleGitHubLogin}
            >
              <Github size={18} />
              Login com GitHub
            </button>

            <button
              className="login-prompt__btn login-prompt__btn--google"
              onClick={handleGoogleLogin}
            >
              <Chrome size={18} />
              Login com Google
            </button>
          </div>

          {/* Divider */}
          <div className="login-prompt__divider">
            <span>ou</span>
          </div>

          {/* Continue without login */}
          <button
            className="login-prompt__btn login-prompt__btn--ghost"
            onClick={onClose}
          >
            <Eye size={16} />
            Continuar sem login (apenas leitura)
          </button>
        </div>
      </div>
    </div>
  )
}
