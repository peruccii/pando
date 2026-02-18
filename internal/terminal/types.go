package terminal

import (
	"os"
	"sync"
	"time"
)

// PTYConfig configura a criação de um novo terminal
type PTYConfig struct {
	Shell       string   `json:"shell"`       // "/bin/zsh" ou "/bin/bash"
	Cwd         string   `json:"cwd"`         // Diretório de trabalho
	Env         []string `json:"env"`         // Variáveis de ambiente extras
	Cols        uint16   `json:"cols"`        // Colunas (default: 80)
	Rows        uint16   `json:"rows"`        // Linhas (default: 24)
	UseDocker   bool     `json:"useDocker"`   // Se deve rodar em container Docker
	DockerImage string   `json:"dockerImage"` // Imagem Docker
	DockerMount string   `json:"dockerMount"` // Ponto de montagem Docker
}

// PTYSession representa uma sessão de terminal ativa
type PTYSession struct {
	ID        string    `json:"id"`
	Config    PTYConfig `json:"config"`
	Cols      uint16    `json:"cols"`
	Rows      uint16    `json:"rows"`
	IsAlive   bool      `json:"isAlive"`
	CreatedAt time.Time `json:"createdAt"`

	// Campos internos (não exportados para JSON)
	pty         *os.File
	cmd         *os.Process
	mu          sync.Mutex
	output      []func(data []byte)
	done        chan struct{}
	permissions map[string]TerminalPermission // userID → permission level
}

// OutputMessage é uma mensagem de output do terminal (PTY → Frontend)
type OutputMessage struct {
	SessionID string `json:"sessionID"`
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
	Sequence  uint64 `json:"sequence"`
}

// InputMessage é uma mensagem de input para o terminal (Frontend → PTY)
type InputMessage struct {
	SessionID string `json:"sessionID"`
	UserID    string `json:"userID,omitempty"`
	Data      string `json:"data"`
}

// ResizeMessage é uma mensagem de resize do terminal
type ResizeMessage struct {
	SessionID string `json:"sessionID"`
	Cols      uint16 `json:"cols"`
	Rows      uint16 `json:"rows"`
}

// SessionInfo retorna informações serializáveis de uma sessão
type SessionInfo struct {
	ID        string    `json:"id"`
	Shell     string    `json:"shell"`
	Cwd       string    `json:"cwd"`
	Cols      uint16    `json:"cols"`
	Rows      uint16    `json:"rows"`
	IsAlive   bool      `json:"isAlive"`
	CreatedAt time.Time `json:"createdAt"`
}

// TerminalPermission define o nível de permissão de um guest
type TerminalPermission string

const (
	PermissionNone      TerminalPermission = "none"
	PermissionReadOnly  TerminalPermission = "read_only"
	PermissionReadWrite TerminalPermission = "read_write"
)

// IPTYManager define a interface do gerenciador de terminais
type IPTYManager interface {
	Create(config PTYConfig) (sessionID string, err error)
	Destroy(sessionID string) error
	Resize(sessionID string, cols, rows uint16) error
	Write(sessionID string, data []byte) error
	OnOutput(sessionID string, handler func(data []byte))
	GetSessions() []SessionInfo
	IsAlive(sessionID string) bool
	DestroyAll()
}
