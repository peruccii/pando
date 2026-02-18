package database

import "time"

// UserConfig armazena configurações do usuário
type UserConfig struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	UserID              string    `gorm:"uniqueIndex;not null" json:"userId"`
	Theme               string    `gorm:"default:dark" json:"theme"`
	Language            string    `gorm:"default:pt-BR" json:"language"`
	OnboardingCompleted bool      `gorm:"default:false" json:"onboardingCompleted"`
	AIModel             string    `gorm:"default:gemini-2.0-flash" json:"aiModel"`
	AIAPIKey            string    `json:"-"` // Criptografado com AES-256
	DefaultShell        string    `json:"defaultShell"`
	FontSize            int       `gorm:"default:14" json:"fontSize"`
	ShortcutBindings    string    `gorm:"type:text" json:"shortcutBindings,omitempty"` // JSON de atalhos customizados
	LayoutState         string    `gorm:"type:text" json:"layoutState,omitempty"`      // Serialized Command Center layout
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

// Workspace representa um projeto/workspace do usuário
type Workspace struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	UserID       string         `gorm:"index;not null" json:"userId"`
	Name         string         `gorm:"not null" json:"name"`
	Path         string         `gorm:"not null" json:"path"`
	GitRemote    string         `json:"gitRemote,omitempty"`
	Owner        string         `json:"owner,omitempty"`
	Repo         string         `json:"repo,omitempty"`
	Color        string         `gorm:"default:''" json:"color,omitempty"`
	IsActive     bool           `gorm:"default:false" json:"isActive"`
	Agents       []AgentSession `gorm:"foreignKey:WorkspaceID;constraint:OnDelete:CASCADE" json:"agents,omitempty"`
	LastOpenedAt *time.Time     `json:"lastOpenedAt,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
}

// AgentSession representa uma sessão de agente/terminal vinculada a um workspace.
type AgentSession struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	WorkspaceID uint      `gorm:"index;not null" json:"workspaceId"`
	Name        string    `gorm:"not null" json:"name"`
	Type        string    `gorm:"default:terminal" json:"type"` // "terminal" | "ai_agent" | "github"
	Shell       string    `gorm:"default:/bin/zsh" json:"shell"`
	Cwd         string    `gorm:"default:''" json:"cwd"`
	UseDocker   bool      `gorm:"default:false" json:"useDocker"`
	SessionID   string    `gorm:"index;default:''" json:"sessionId,omitempty"`
	Status      string    `gorm:"default:idle" json:"status"`            // "idle" | "running" | "error"
	LayoutJSON  string    `gorm:"type:text" json:"layoutJson,omitempty"` // MosaicNode coords serializado
	SortOrder   int       `gorm:"default:0" json:"sortOrder"`
	IsMinimized bool      `gorm:"default:false" json:"isMinimized"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// TableName mantém compatibilidade com o schema legado.
func (AgentSession) TableName() string {
	return "agent_instances"
}

// AgentInstance é mantido como alias por compatibilidade com código legado.
type AgentInstance = AgentSession

// ChatHistory armazena mensagens de chat com IA
type ChatHistory struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	AgentID    uint      `gorm:"index;not null" json:"agentId"`
	Role       string    `gorm:"not null" json:"role"` // "user" | "assistant" | "system"
	Content    string    `gorm:"type:text;not null" json:"content"`
	TokenCount int       `json:"tokenCount,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// SessionHistory armazena histórico de sessões P2P
type SessionHistory struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	SessionCode string     `gorm:"not null" json:"sessionCode"`
	HostUserID  string     `gorm:"index;not null" json:"hostUserId"`
	GuestCount  int        `gorm:"default:0" json:"guestCount"`
	StartedAt   time.Time  `json:"startedAt"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// AuditLog armazena eventos de auditoria por sessão.
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	SessionID string    `gorm:"index;not null" json:"sessionID"`
	UserID    string    `gorm:"index;not null" json:"userID"`
	Action    string    `gorm:"index;not null" json:"action"`
	Details   string    `gorm:"type:text" json:"details"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
}

// TerminalSnapshot persiste o estado de um terminal/CLI para restauração após restart.
type TerminalSnapshot struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PaneID    string    `gorm:"not null;index" json:"paneId"`
	CLIType   string    `gorm:"default:''" json:"cliType"` // "gemini" | "claude" | "codex" | "opencode" | "" (terminal simples)
	Shell     string    `gorm:"default:/bin/zsh" json:"shell"`
	Cwd       string    `json:"cwd"`
	UseDocker bool      `gorm:"default:false" json:"useDocker"`
	PaneTitle string    `json:"paneTitle"`
	PaneType  string    `gorm:"default:terminal" json:"paneType"` // "terminal" | "ai_agent"
	Config    string    `gorm:"type:text" json:"config,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
