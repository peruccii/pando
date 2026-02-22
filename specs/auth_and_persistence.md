# Spec: Autenticação Híbrida & Persistência Local

> **Módulo**: 4 — Auth & Persistence  
> **Status**: Draft  
> **PRD Ref**: Seção 10  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Sistema de autenticação seguro sem custo de infraestrutura (BaaS), acoplado a banco de dados local (SQLite) para persistência de estado. Princípio **"Local-First"** para performance e **"Cloud-Auth"** para identidade.

---

## 2. Autenticação (Identity Layer)

### 2.1 Provedor

- **BaaS**: Supabase Auth
- **Método**: OAuth 2.0 via **PKCE Flow** (padrão para Desktop Apps)
- **Providers**: GitHub, Google

### 2.2 Fluxo OAuth (PKCE)

```
1. Frontend chama AuthService.Login("github")
2. Backend Go gera code_verifier + code_challenge (PKCE)
3. Backend abre Safari na URL de Auth do Supabase
4. Usuário faz login no GitHub via browser
5. Supabase redireciona para orch://auth/callback?code=xxx
6. Wails captura o deep link
7. Backend troca code por access_token + refresh_token
8. Tokens armazenados no macOS Keychain (go-keyring)
9. Frontend recebe evento de "login success"
```

### 2.3 Deep Link

- **Protocolo**: `orch://`
- **Callback**: `orch://auth/callback`
- **Registro**: Info.plist do app macOS (CFBundleURLSchemes)

```xml
<key>CFBundleURLTypes</key>
<array>
    <dict>
        <key>CFBundleURLSchemes</key>
        <array>
            <string>orch</string>
        </array>
        <key>CFBundleURLName</key>
        <string>com.orch.app</string>
    </dict>
</array>
```

### 2.4 AuthService (Go)

```go
type IAuthService interface {
    Login(provider string) error                    // Abre browser
    HandleCallback(code string) (*AuthResult, error) // Processa callback
    Logout() error
    GetCurrentUser() (*User, error)
    IsAuthenticated() bool
    RefreshToken() error
    GetGitHubToken() (string, error)                // Para GitHub API
}

type AuthResult struct {
    AccessToken  string
    RefreshToken string
    ExpiresAt    time.Time
    User         User
}

type User struct {
    ID        string
    Email     string
    Name      string
    AvatarURL string
    Provider  string // "github", "google"
}
```

### 2.5 Token Storage — macOS Keychain

```go
import "github.com/zalando/go-keyring"

const (
    serviceName = "ORCH"
    accessKey   = "access_token"
    refreshKey  = "refresh_token"
    githubKey   = "github_token"
)

func (s *AuthService) storeTokens(result *AuthResult) error {
    if err := keyring.Set(serviceName, accessKey, result.AccessToken); err != nil {
        return err
    }
    if err := keyring.Set(serviceName, refreshKey, result.RefreshToken); err != nil {
        return err
    }
    return nil
}

func (s *AuthService) getAccessToken() (string, error) {
    return keyring.Get(serviceName, accessKey)
}
```

#### Regras de Segurança

| ✅ Obrigatório                    | 🚫 Proibido                           |
| --------------------------------- | -------------------------------------- |
| macOS Keychain via `go-keyring`   | JSON, SQLite, LocalStorage             |
| Token em memória durante execução | Persistir em arquivo de texto           |
| Refresh silencioso no startup     | Expor token em logs                     |
| HTTPS only para todas as requests | HTTP em qualquer endpoint               |

---

## 3. Persistência Local — SQLite

### 3.1 Driver

- **Lib**: `github.com/glebarez/sqlite` (Pure Go) ou `modernc.org/sqlite`
- **Motivo**: Sem dependência de CGO → compilação simplificada
- **ORM**: GORM (`gorm.io/gorm`)

### 3.2 Localização

```
macOS: ~/Library/Application Support/ORCH/orch_data.db
```

```go
func getDBPath() string {
    home, _ := os.UserHomeDir()
    dir := filepath.Join(home, "Library", "Application Support", "ORCH")
    os.MkdirAll(dir, 0700)
    return filepath.Join(dir, "orch_data.db")
}
```

### 3.3 Schema (GORM Models)

```go
type UserConfig struct {
    gorm.Model
    Theme         string `gorm:"default:dark"`
    OpenAIKey     string // Opcional: chave do usuário (criptografada)
    GeminiKey     string
    DefaultShell  string `gorm:"default:/bin/zsh"`
    FontSize      int    `gorm:"default:14"`
    FontFamily    string `gorm:"default:JetBrains Mono"`
    ScrollbackLen int    `gorm:"default:5000"`
    Locale        string `gorm:"default:pt-BR"`
}

type Workspace struct {
    gorm.Model
    Name      string
    IsActive  bool              `gorm:"default:false"`
    RepoPath  string            // Path local do repositório
    RepoURL   string            // URL do GitHub (opcional)
    Agents    []AgentInstance   `gorm:"foreignKey:WorkspaceID"`
}

type AgentInstance struct {
    gorm.Model
    WorkspaceID  uint
    Name         string  // ex: "Refatorador SQL"
    Type         string  // ex: "Gemini Pro", "GPT-4"
    ProviderID   string  // ex: "gemini", "openai"
    Status       string  `gorm:"default:idle"` // "idle", "running", "error"
    SystemPrompt string  // Prompt customizado (opcional)

    // Layout no Grid (Command Center)
    WindowX      int
    WindowY      int
    WindowWidth  int     `gorm:"default:400"`
    WindowHeight int     `gorm:"default:300"`
    IsMinimized  bool    `gorm:"default:false"`
    ZIndex       int     `gorm:"default:0"`
}

type ChatHistory struct {
    gorm.Model
    AgentInstanceID uint
    Role            string // "user", "assistant", "system"
    Content         string
    TokensUsed      int
    Provider        string
    Timestamp       int64
}

type SessionHistory struct {
    gorm.Model
    SessionCode string // "X92B-4K7"
    HostUserID  string
    StartedAt   time.Time
    EndedAt     *time.Time
    GuestCount  int
    Mode        string // "docker", "liveshare"
}
```

### 3.4 Database Service

```go
type IDBService interface {
    // Config
    GetConfig() (*UserConfig, error)
    UpdateConfig(config *UserConfig) error

    // Workspaces
    ListWorkspaces() ([]Workspace, error)
    GetActiveWorkspace() (*Workspace, error)
    CreateWorkspace(ws *Workspace) error
    SetActiveWorkspace(id uint) error
    DeleteWorkspace(id uint) error

    // Agents
    ListAgents(workspaceID uint) ([]AgentInstance, error)
    CreateAgent(agent *AgentInstance) error
    UpdateAgent(agent *AgentInstance) error
    DeleteAgent(id uint) error
    UpdateAgentLayout(id uint, x, y, w, h int) error

    // Chat History
    GetHistory(agentID uint, limit int) ([]ChatHistory, error)
    SaveMessage(msg *ChatHistory) error
    ClearHistory(agentID uint) error
}
```

---

## 4. Rotina de Bootstrap (Startup)

```go
func (app *App) Startup(ctx context.Context) {
    // 1. CHECK AUTH
    token, err := app.auth.getAccessToken()
    if err != nil || token == "" {
        app.state.IsAuthenticated = false
    } else {
        // Validar expiração
        valid, err := app.auth.ValidateToken(token)
        if !valid {
            // Tentar refresh silencioso
            err = app.auth.RefreshToken()
            if err != nil {
                app.state.IsAuthenticated = false
                log.Warn("Token refresh failed, user logged out")
            }
        }
        if app.state.IsAuthenticated {
            user, _ := app.auth.GetCurrentUser()
            app.state.User = user
        }
    }

    // 2. CHECK DB
    dbPath := getDBPath()
    db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
    if err != nil {
        log.Fatal("Failed to open database:", err)
    }

    // AutoMigrate
    db.AutoMigrate(
        &UserConfig{},
        &Workspace{},
        &AgentInstance{},
        &ChatHistory{},
        &SessionHistory{},
    )

    app.db = db

    // 3. RESTORE STATE (Hydration)
    workspace, _ := app.dbService.GetActiveWorkspace()
    if workspace != nil {
        agents, _ := app.dbService.ListAgents(workspace.ID)
        // Enviar para frontend com coordenadas de layout
        runtime.EventsEmit(ctx, "app:hydrated", HydrationPayload{
            User:      app.state.User,
            Workspace: workspace,
            Agents:    agents,
            Config:    app.dbService.GetConfig(),
        })
    }
}
```

---

## 5. Frontend — Estado de Autenticação

```typescript
interface AuthState {
    isAuthenticated: boolean
    user: User | null
    githubToken: string | null  // Em memória apenas
    isLoading: boolean
}

interface User {
    id: string
    email: string
    name: string
    avatarUrl: string
    provider: 'github' | 'google'
}
```

---

## 6. Privacidade & Segurança

| Regra                                    | Implementação                              |
| ---------------------------------------- | ------------------------------------------ |
| Zero telemetria de código                 | Dados nunca saem da máquina (exceto LLM)   |
| SQLite não acessível externamente         | Arquivo com permissão `0600`                |
| API Keys criptografadas no SQLite         | AES-256 com chave derivada do Keychain     |
| Logs não contêm dados sensíveis           | Sanitizer nos logs de debug                 |

---

## 7. Dependências

| Dependência                      | Tipo       |
| --------------------------------- | ---------- |
| Supabase Auth (BaaS)             | Bloqueador |
| `go-keyring` (macOS Keychain)    | Bloqueador |
| `glebarez/sqlite` (Pure Go)     | Bloqueador |
| GORM                              | Bloqueador |
| Wails Deep Links                  | Bloqueador |
