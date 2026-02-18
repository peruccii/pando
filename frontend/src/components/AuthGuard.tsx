import React, { useState } from 'react'
import { useAuth } from '../hooks/useAuth'
import { LoginPrompt } from './LoginPrompt'
import './AuthGuard.css'

export interface AuthGuardProps {
  /** Conteúdo protegido — renderizado apenas quando autenticado */
  children: React.ReactNode
  /** Fallback customizado para estado não-autenticado */
  fallback?: React.ReactNode
  /** Nome da ação protegida (ex: "Criar PR", "Comentar") */
  action?: string
  /** Se true, exige que o provider seja GitHub */
  requireGitHub?: boolean
  /** Se true, renderiza inline (span) ao invés de bloco */
  inline?: boolean
}

/**
 * AuthGuard — Barreira de identidade com Progressive Disclosure.
 *
 * Envolve elementos interativos que requerem autenticação.
 * - Usuário autenticado: renderiza children normalmente
 * - Não autenticado: botão disabled com 🔒 + tooltip
 * - Autenticado sem GitHub: botão "Conectar GitHub"
 *
 * Clicar no botão disabled abre um LoginPrompt contextual.
 */
export function AuthGuard({
  children,
  fallback,
  action,
  requireGitHub = false,
  inline = false,
}: AuthGuardProps) {
  const { isAuthenticated, isGitHubUser } = useAuth()
  const [showLoginPrompt, setShowLoginPrompt] = useState(false)

  // Autenticado (e com GitHub se necessário) → renderizar children
  if (isAuthenticated && (!requireGitHub || isGitHubUser)) {
    return <>{children}</>
  }

  // Autenticado mas sem GitHub quando GitHub é requerido
  if (isAuthenticated && requireGitHub && !isGitHubUser) {
    return (
      <button
        className="auth-guard__link-github"
        onClick={() => setShowLoginPrompt(true)}
        title={`Conecte ao GitHub para ${action || 'realizar esta ação'}`}
      >
        <span className="auth-guard__icon">🔗</span>
        Conectar GitHub{action ? ` para ${action}` : ''}
      </button>
    )
  }

  // Não autenticado — fallback customizado
  if (fallback) {
    return <>{fallback}</>
  }

  // Não autenticado — botão disabled com badge 🔒
  const Wrapper = inline ? 'span' : 'div'
  const tooltipText = `Faça login no GitHub para ${action || 'realizar esta ação'}`

  return (
    <>
      <Wrapper className="auth-guard__wrapper">
        <button
          className="auth-guard__button btn--auth-required"
          disabled
          title={tooltipText}
          onClick={() => setShowLoginPrompt(true)}
        >
          <span className="auth-guard__lock">🔒</span>
          {action || 'Login necessário'}
        </button>
        <div className="auth-guard__tooltip">{tooltipText}</div>
      </Wrapper>

      {showLoginPrompt && (
        <LoginPrompt
          action={action}
          onClose={() => setShowLoginPrompt(false)}
        />
      )}
    </>
  )
}
