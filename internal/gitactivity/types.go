package gitactivity

import "time"

// EventType representa o tipo canônico de atividade Git.
type EventType string

const (
	EventTypeBranchCreated   EventType = "branch_created"
	EventTypeBranchChanged   EventType = "branch_changed"
	EventTypeCommitCreated   EventType = "commit_created"
	EventTypeCommitPreparing EventType = "commit_preparing"
	EventTypeIndexUpdated    EventType = "index_updated"
	EventTypeMerge           EventType = "merge"
	EventTypeFetch           EventType = "fetch"
	EventTypeUnknown         EventType = "unknown"
)

// EventFile representa um arquivo relacionado ao evento.
type EventFile struct {
	Path    string `json:"path"`
	Status  string `json:"status,omitempty"`
	Added   int    `json:"added,omitempty"`
	Removed int    `json:"removed,omitempty"`
}

// EventDetails contém metadados extras do evento.
type EventDetails struct {
	Ref         string            `json:"ref,omitempty"`
	CommitHash  string            `json:"commitHash,omitempty"`
	DiffPreview string            `json:"diffPreview,omitempty"`
	Files       []EventFile       `json:"files,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// Event é a unidade persistida no buffer de atividades Git.
type Event struct {
	ID        string       `json:"id"`
	Type      EventType    `json:"type"`
	ActorName string       `json:"actorName"`
	ActorID   string       `json:"actorId,omitempty"`
	RepoPath  string       `json:"repoPath"`
	RepoName  string       `json:"repoName"`
	Branch    string       `json:"branch,omitempty"`
	Message   string       `json:"message"`
	Timestamp time.Time    `json:"timestamp"`
	Source    string       `json:"source,omitempty"`
	DedupeKey string       `json:"dedupeKey,omitempty"`
	Details   EventDetails `json:"details,omitempty"`
}

// ListOptions controla filtros da listagem.
type ListOptions struct {
	Limit    int
	Type     EventType
	RepoPath string
}
