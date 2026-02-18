package filewatcher

import "time"

// FileEvent representa um evento detectado no .git
type FileEvent struct {
	Type      string            `json:"type"`      // "branch_changed", "commit", "merge", "fetch", "index"
	Path      string            `json:"path"`      // Caminho do arquivo alterado
	Timestamp time.Time         `json:"timestamp"` // Quando o evento ocorreu
	Details   map[string]string `json:"details"`   // Detalhes extras (nova branch, ref, etc.)
}

// CommitInfo representa informações de um commit
type CommitInfo struct {
	Hash    string    `json:"hash"`
	Message string    `json:"message"`
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
}

// IFileWatcher define a interface do serviço de monitoramento de arquivos .git
type IFileWatcher interface {
	// Watch inicia o monitoramento da pasta .git de um projeto
	Watch(projectPath string) error

	// Unwatch para o monitoramento de um projeto
	Unwatch(projectPath string) error

	// OnChange registra um handler para receber eventos
	OnChange(handler func(event FileEvent))

	// GetCurrentBranch retorna a branch atual do projeto
	GetCurrentBranch(projectPath string) (string, error)

	// GetLastCommit retorna informações do último commit
	GetLastCommit(projectPath string) (*CommitInfo, error)

	// Close encerra todos os watchers
	Close() error
}
