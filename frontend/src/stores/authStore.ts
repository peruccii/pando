import { create } from 'zustand'

export interface User {
  id: string
  email: string
  name: string
  avatarUrl?: string
  provider: string // "github" | "google"
}

export interface AuthState {
  isAuthenticated: boolean
  user: User | null
  provider: string | null
  hasGitHubToken: boolean
  isLoading: boolean
  error: string | null
}

interface AuthActions {
  setAuth: (state: Partial<AuthState>) => void
  setUser: (user: User | null) => void
  setLoading: (loading: boolean) => void
  setError: (error: string | null) => void
  logout: () => void
}

export const useAuthStore = create<AuthState & AuthActions>((set) => ({
  // State
  isAuthenticated: false,
  user: null,
  provider: null,
  hasGitHubToken: false,
  isLoading: true,
  error: null,

  // Actions
  setAuth: (state) => set((prev) => ({ ...prev, ...state })),

  setUser: (user) =>
    set({
      user,
      isAuthenticated: !!user,
      provider: user?.provider ?? null,
      hasGitHubToken: user?.provider === 'github',
    }),

  setLoading: (isLoading) => set({ isLoading }),

  setError: (error) => set({ error }),

  logout: () =>
    set({
      isAuthenticated: false,
      user: null,
      provider: null,
      hasGitHubToken: false,
      error: null,
    }),
}))
