package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	"orch/internal/database"

	"github.com/zalando/go-keyring"
)

const (
	// Keychain service name
	keychainService = "com.orch.app"

	// Keychain keys
	keychainAccessToken  = "access_token"
	keychainRefreshToken = "refresh_token"
	keychainProvider     = "auth_provider"
	keychainExpiresAt    = "token_expires_at"

	// Supabase config
	supabaseURL     = "https://imlkpvutzzbznxqlhqyn.supabase.co"
	supabaseAnonKey = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6ImltbGtwdnV0enpiem54cWxocXluIiwicm9sZSI6ImFub24iLCJpYXQiOjE3NzEwOTc3MjUsImV4cCI6MjA4NjY3MzcyNX0.zpIeyaP8zEA4GtypQVSGypACygd0C8KtApgNo73wC2E"

	// Local callback server config
	callbackPort    = 9877
	callbackTimeout = 5 * time.Minute
)

// CallbackHandler é uma função chamada quando o callback OAuth é recebido
type CallbackHandler func(result *AuthResult)

// Service gerencia autenticação via Supabase + macOS Keychain
type Service struct {
	db              *database.Service
	currentUser     *User
	currentPKCE     *PKCEChallenge
	callbackServer  *http.Server
	callbackHandler CallbackHandler
	callbackPort    int
}

// NewService cria um novo serviço de autenticação
func NewService(db *database.Service) *Service {
	return &Service{
		db: db,
	}
}

// StartCallbackServer inicia um servidor HTTP local para receber o callback OAuth
func (s *Service) StartCallbackServer(handler CallbackHandler) (string, error) {
	// Verificar se já existe um servidor rodando
	if s.callbackServer != nil {
		return s.getCallbackURL(), nil
	}

	s.callbackHandler = handler

	// Criar mux e rotas
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", s.handleLocalCallback)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ORCH Authentication Server"))
	})

	// Tentar encontrar uma porta disponível
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", callbackPort))
	if err != nil {
		// Tentar porta alternativa
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", fmt.Errorf("failed to start callback server: %w", err)
		}
	}

	port := listener.Addr().(*net.TCPAddr).Port
	s.callbackPort = port

	s.callbackServer = &http.Server{
		Handler:     mux,
		ReadTimeout: 10 * time.Second,
	}

	// Iniciar servidor em goroutine
	go func() {
		if err := s.callbackServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[AUTH] Callback server error: %v", err)
		}
	}()

	// Configurar timeout para desligar servidor
	go func() {
		time.Sleep(callbackTimeout)
		s.StopCallbackServer()
	}()

	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	log.Printf("[AUTH] Callback server started at %s", callbackURL)
	return callbackURL, nil
}

// StopCallbackServer para o servidor de callback
func (s *Service) StopCallbackServer() {
	if s.callbackServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.callbackServer.Shutdown(ctx)
		s.callbackServer = nil
		s.callbackPort = 0
		log.Println("[AUTH] Callback server stopped")
	}
}

// getCallbackURL retorna a URL de callback
func (s *Service) getCallbackURL() string {
	if s.callbackPort > 0 {
		return fmt.Sprintf("http://127.0.0.1:%d/callback", s.callbackPort)
	}
	return fmt.Sprintf("http://127.0.0.1:%d/callback", callbackPort)
}

// handleLocalCallback processa o callback do OAuth no servidor local
func (s *Service) handleLocalCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")

	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}

	// Processar callback
	result, err := s.HandleCallback(code)

	// Enviar resultado para o handler
	if s.callbackHandler != nil {
		s.callbackHandler(result)
	}

	// Responder ao browser
	w.Header().Set("Content-Type", "text/html")
	if result != nil && result.Success {
		w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>ORCH - Login Successful</title></head>
<body style="font-family: system-ui; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #0f0f14; color: white;">
<div style="text-align: center;">
<h1>✓ Login realizado com sucesso!</h1>
<p>Você pode fechar esta janela e voltar para o ORCH.</p>
</div>
</body>
</html>`))
	} else {
		errorMsg := "Erro desconhecido"
		if result != nil {
			errorMsg = result.Error
		}
		if err != nil {
			errorMsg = err.Error()
		}
		w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>ORCH - Login Failed</title></head>
<body style="font-family: system-ui; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #0f0f14; color: white;">
<div style="text-align: center;">
<h1>✗ Falha no login</h1>
<p>%s</p>
<p>Tente novamente no aplicativo ORCH.</p>
</div>
</body>
</html>`, errorMsg)))
	}

	// Parar servidor após receber callback
	go func() {
		time.Sleep(2 * time.Second)
		s.StopCallbackServer()
	}()
}

// GetAuthURL retorna a URL de autenticação do Supabase para o provedor
// Usa PKCE manual conforme documentação do Supabase
func (s *Service) GetAuthURL(provider string) (string, error) {
	// Gerar PKCE challenge
	pkce, err := GeneratePKCE()
	if err != nil {
		return "", fmt.Errorf("failed to generate PKCE: %w", err)
	}
	s.currentPKCE = pkce

	// Obter URL de callback (usa servidor local)
	callbackURL, err := s.StartCallbackServer(nil)
	if err != nil {
		return "", fmt.Errorf("failed to start callback server: %w", err)
	}

	// Construir URL de auth com PKCE conforme documentação Supabase
	params := url.Values{}
	params.Add("provider", provider)
	params.Add("redirect_to", callbackURL)
	params.Add("code_challenge", pkce.CodeChallenge)
	params.Add("code_challenge_method", "S256")

	authURL := fmt.Sprintf("%s/auth/v1/authorize?%s", supabaseURL, params.Encode())
	log.Printf("[AUTH] Auth URL: %s", authURL)

	return authURL, nil
}

// HandleCallback processa o callback de autenticação
// Usa PKCE conforme documentação Supabase
func (s *Service) HandleCallback(code string) (*AuthResult, error) {
	// Trocar code por tokens
	tokenPair, err := s.exchangeCodeForTokens(code)
	if err != nil {
		return &AuthResult{Success: false, Error: err.Error()}, nil
	}

	// Limpar PKCE após uso
	s.currentPKCE = nil

	// Armazenar tokens no Keychain
	if err := s.storeTokens(tokenPair); err != nil {
		return &AuthResult{Success: false, Error: "Failed to store tokens"}, nil
	}

	// Buscar perfil do usuário
	user, err := s.fetchUserProfile(tokenPair.AccessToken)
	if err != nil {
		return &AuthResult{Success: false, Error: "Failed to fetch user profile"}, nil
	}

	s.currentUser = user

	return &AuthResult{Success: true, User: user}, nil
}

// exchangeCodeForTokens troca o authorization code por access_token + refresh_token
// Usa o endpoint /auth/v1/token com formato JSON
// Para OAuth com providers (ex: GitHub), usa grant_type=pkce com auth_code + code_verifier
func (s *Service) exchangeCodeForTokens(code string) (*TokenPair, error) {
	if s.currentPKCE == nil {
		return nil, fmt.Errorf("no PKCE challenge found - authentication flow not initiated")
	}

	reqBodyMap := map[string]string{
		"auth_code":     code,
		"code_verifier": s.currentPKCE.CodeVerifier,
	}

	reqBodyJSON, err := json.Marshal(reqBodyMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", supabaseURL+"/auth/v1/token?grant_type=pkce", bytes.NewReader(reqBodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apikey", supabaseAnonKey)

	// Debug log
	log.Printf("[AUTH] Token exchange request to: %s", supabaseURL+"/auth/v1/token?grant_type=pkce")
	log.Printf("[AUTH] Request body: %s", string(reqBodyJSON))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[AUTH] Token exchange response status: %d", resp.StatusCode)
	log.Printf("[AUTH] Token exchange response body: %s", string(body))

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("token exchange failed: %s", string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		User         struct {
			AppMetadata struct {
				Provider string `json:"provider"`
			} `json:"app_metadata"`
		} `json:"user"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &TokenPair{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Provider:     tokenResp.User.AppMetadata.Provider,
	}, nil
}

// storeTokens armazena tokens no macOS Keychain
func (s *Service) storeTokens(pair *TokenPair) error {
	if err := keyring.Set(keychainService, keychainAccessToken, pair.AccessToken); err != nil {
		return fmt.Errorf("failed to store access token: %w", err)
	}
	if err := keyring.Set(keychainService, keychainRefreshToken, pair.RefreshToken); err != nil {
		return fmt.Errorf("failed to store refresh token: %w", err)
	}
	if err := keyring.Set(keychainService, keychainProvider, pair.Provider); err != nil {
		return fmt.Errorf("failed to store provider: %w", err)
	}
	if err := keyring.Set(keychainService, keychainExpiresAt, pair.ExpiresAt.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("failed to store expiration: %w", err)
	}
	return nil
}

// getAccessToken retorna o access token do Keychain
func (s *Service) getAccessToken() (string, error) {
	return keyring.Get(keychainService, keychainAccessToken)
}

// getRefreshToken retorna o refresh token do Keychain
func (s *Service) getRefreshToken() (string, error) {
	return keyring.Get(keychainService, keychainRefreshToken)
}

// IsAuthenticated verifica se o usuário está autenticado com token válido
func (s *Service) IsAuthenticated() (bool, error) {
	token, err := s.getAccessToken()
	if err != nil || token == "" {
		return false, nil
	}

	// Verificar expiração
	expiresStr, err := keyring.Get(keychainService, keychainExpiresAt)
	if err != nil {
		return false, nil
	}

	expiresAt, err := time.Parse(time.RFC3339, expiresStr)
	if err != nil {
		return false, nil
	}

	// Se expirou, tentar refresh
	if time.Now().After(expiresAt) {
		if err := s.RefreshToken(); err != nil {
			return false, nil
		}
	}

	return true, nil
}

// RefreshToken renova o access token usando o refresh token
func (s *Service) RefreshToken() error {
	refreshToken, err := s.getRefreshToken()
	if err != nil || refreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	reqBody := url.Values{}
	reqBody.Set("grant_type", "refresh_token")
	reqBody.Set("refresh_token", refreshToken)

	req, err := http.NewRequest("POST", supabaseURL+"/auth/v1/token", bytes.NewBufferString(reqBody.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("apikey", supabaseAnonKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("refresh failed: %s", string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("failed to parse refresh response: %w", err)
	}

	// Buscar provider atual
	provider, _ := keyring.Get(keychainService, keychainProvider)

	return s.storeTokens(&TokenPair{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Provider:     provider,
	})
}

// GetCurrentUser retorna o usuário autenticado atual
func (s *Service) GetCurrentUser() (*User, error) {
	if s.currentUser != nil {
		return s.currentUser, nil
	}

	token, err := s.getAccessToken()
	if err != nil || token == "" {
		return nil, fmt.Errorf("not authenticated")
	}

	user, err := s.fetchUserProfile(token)
	if err != nil {
		return nil, err
	}

	s.currentUser = user
	return user, nil
}

// SetCurrentUserForTesting injeta um usuário autenticado em memória para testes.
func (s *Service) SetCurrentUserForTesting(user *User) {
	s.currentUser = user
}

// fetchUserProfile busca o perfil do usuário no Supabase
func (s *Service) fetchUserProfile(accessToken string) (*User, error) {
	req, err := http.NewRequest("GET", supabaseURL+"/auth/v1/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("apikey", supabaseAnonKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user profile request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("user profile fetch failed: %s", string(body))
	}

	var userResp struct {
		ID          string `json:"id"`
		Email       string `json:"email"`
		AppMetadata struct {
			Provider string `json:"provider"`
		} `json:"app_metadata"`
		UserMetadata struct {
			Name      string `json:"full_name"`
			AvatarURL string `json:"avatar_url"`
		} `json:"user_metadata"`
	}

	if err := json.Unmarshal(body, &userResp); err != nil {
		return nil, fmt.Errorf("failed to parse user profile: %w", err)
	}

	return &User{
		ID:        userResp.ID,
		Email:     userResp.Email,
		Name:      userResp.UserMetadata.Name,
		AvatarURL: userResp.UserMetadata.AvatarURL,
		Provider:  userResp.AppMetadata.Provider,
	}, nil
}

// GetGitHubToken retorna o token do GitHub para chamadas API
// O Supabase OAuth retorna o provider_token nos metadados
func (s *Service) GetGitHubToken() (string, error) {
	provider, _ := keyring.Get(keychainService, keychainProvider)
	if provider != "github" {
		return "", fmt.Errorf("not authenticated with GitHub")
	}
	// O access token do Supabase com provider GitHub já contém permissões
	// Para acessar GitHub API, usamos o provider_token armazenado
	return s.getAccessToken()
}

// Logout limpa todos os tokens e dados de auth
func (s *Service) Logout() error {
	s.currentUser = nil
	s.StopCallbackServer()

	keys := []string{keychainAccessToken, keychainRefreshToken, keychainProvider, keychainExpiresAt}
	for _, key := range keys {
		if err := keyring.Delete(keychainService, key); err != nil {
			log.Printf("[AUTH] Warning: failed to delete keychain key %s: %v", key, err)
		}
	}

	return nil
}

// GetAuthState retorna o estado completo da autenticação
func (s *Service) GetAuthState() *AuthState {
	isAuth, _ := s.IsAuthenticated()
	state := &AuthState{
		IsAuthenticated: isAuth,
	}

	if isAuth {
		if user, err := s.GetCurrentUser(); err == nil {
			state.User = user
			state.Provider = user.Provider
			state.HasGitHubToken = user.Provider == "github"
		}
	}

	return state
}
