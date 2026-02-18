package ai

import "context"

// IAIService define a interface principal do motor de IA.
type IAIService interface {
	GenerateResponse(ctx context.Context, userMessage string, sessionID string) (<-chan string, error)
	SetProvider(provider AIProvider) error
	ListProviders() []AIProvider
	Cancel(sessionID string) error
}

// AIProvider representa um provedor/modelo de IA configurável.
type AIProvider struct {
	ID       string `json:"id"`                 // "gemini", "openai", "ollama"
	Name     string `json:"name"`               // "Gemini", "GPT-4.1", "Llama 3"
	Model    string `json:"model"`              // "gpt-4.1-mini", "llama3", etc.
	APIKey   string `json:"apiKey,omitempty"`   // Nunca persistir em plaintext
	Endpoint string `json:"endpoint,omitempty"` // URL base (ex.: Ollama local)
	Enabled  bool   `json:"enabled"`            // Disponível para uso imediato
}

// SessionState representa o contexto da sessão para prompt augmentation.
type SessionState struct {
	ProjectName   string `json:"projectName"`
	ProjectPath   string `json:"projectPath,omitempty"`
	CurrentBranch string `json:"currentBranch"`
	CurrentFile   string `json:"currentFile,omitempty"`

	// GitHub context
	ActivePR    *PRContext    `json:"activePR,omitempty"`
	ActiveIssue *IssueContext `json:"activeIssue,omitempty"`

	// Terminal context
	LastCommand string `json:"lastCommand,omitempty"`
	LastStdout  string `json:"lastStdout,omitempty"`
	LastStderr  string `json:"lastStderr,omitempty"`
	ShellType   string `json:"shellType,omitempty"`
}

// PRContext representa o PR ativo no contexto.
type PRContext struct {
	Owner  string `json:"owner,omitempty"`
	Repo   string `json:"repo,omitempty"`
	Number int    `json:"number"`
	Title  string `json:"title,omitempty"`
	Body   string `json:"body,omitempty"`
	Diff   string `json:"diff,omitempty"`
	Author string `json:"author,omitempty"`
	Branch string `json:"branch,omitempty"`
}

// IssueContext representa a Issue ativa no contexto.
type IssueContext struct {
	Owner  string `json:"owner,omitempty"`
	Repo   string `json:"repo,omitempty"`
	Number int    `json:"number"`
	Title  string `json:"title,omitempty"`
	Body   string `json:"body,omitempty"`
	State  string `json:"state,omitempty"`
}
