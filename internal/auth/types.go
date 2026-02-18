package auth

import "time"

// User representa um usuário autenticado
type User struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	Provider  string `json:"provider"` // "github" | "google"
}

// AuthState representa o estado de autenticação atual
type AuthState struct {
	IsAuthenticated bool   `json:"isAuthenticated"`
	User            *User  `json:"user,omitempty"`
	Provider        string `json:"provider,omitempty"`
	HasGitHubToken  bool   `json:"hasGitHubToken"`
}

// AuthResult é o resultado de uma operação de autenticação
type AuthResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	User    *User  `json:"user,omitempty"`
}

// TokenPair armazena os tokens de acesso e refresh
type TokenPair struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
	Provider     string    `json:"provider"`
}

// PKCEChallenge representa os dados do PKCE flow
type PKCEChallenge struct {
	CodeVerifier  string `json:"codeVerifier"`
	CodeChallenge string `json:"codeChallenge"`
	State         string `json:"state"`
}
