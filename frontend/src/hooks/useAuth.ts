import { useAuthStore } from '../stores/authStore'

/**
 * Hook simplificado para acessar estado de autenticação.
 * Centraliza lógica comum de auth checks.
 */
export function useAuth() {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const user = useAuthStore((s) => s.user)
  const provider = useAuthStore((s) => s.provider)
  const hasGitHubToken = useAuthStore((s) => s.hasGitHubToken)
  const isLoading = useAuthStore((s) => s.isLoading)

  return {
    isAuthenticated,
    user,
    provider,
    hasGitHubToken,
    isLoading,

    /** Verifica se pode executar ações no GitHub */
    canActOnGitHub: isAuthenticated && hasGitHubToken,

    /** Verifica se o login é do GitHub */
    isGitHubUser: provider === 'github',
  }
}
