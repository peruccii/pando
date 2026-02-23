package gitpanel

// PreflightResult descreve o estado de runtime para operar Git no painel.
type PreflightResult struct {
	GitAvailable bool   `json:"gitAvailable"`
	RepoPath     string `json:"repoPath"`
	RepoRoot     string `json:"repoRoot"`
	Branch       string `json:"branch,omitempty"`
	MergeActive  bool   `json:"mergeActive"`
}

// FileChangeDTO representa alteração de arquivo no status Git.
type FileChangeDTO struct {
	Path         string `json:"path"`
	OriginalPath string `json:"originalPath,omitempty"`
	Status       string `json:"status"`
	Added        int    `json:"added"`
	Removed      int    `json:"removed"`
}

// ConflictFileDTO representa arquivo em conflito.
type ConflictFileDTO struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// StatusDTO representa snapshot de status para o painel.
type StatusDTO struct {
	Branch     string            `json:"branch"`
	Ahead      int               `json:"ahead"`
	Behind     int               `json:"behind"`
	Staged     []FileChangeDTO   `json:"staged"`
	Unstaged   []FileChangeDTO   `json:"unstaged"`
	Conflicted []ConflictFileDTO `json:"conflicted"`
}

// HistoryItemDTO representa item do histórico linear.
type HistoryItemDTO struct {
	Hash       string `json:"hash"`
	ShortHash  string `json:"shortHash"`
	Author     string `json:"author"`
	AuthoredAt string `json:"authoredAt"`
	Subject    string `json:"subject"`
}

// HistoryPageDTO representa página de histórico paginado.
type HistoryPageDTO struct {
	Items      []HistoryItemDTO `json:"items"`
	NextCursor string           `json:"nextCursor"`
	HasMore    bool             `json:"hasMore"`
}

// DiffLineDTO representa linha individual no diff estruturado.
type DiffLineDTO struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	OldLine *int   `json:"oldLine,omitempty"`
	NewLine *int   `json:"newLine,omitempty"`
}

// DiffHunkDTO representa bloco de alterações de um arquivo.
type DiffHunkDTO struct {
	Header   string        `json:"header"`
	OldStart int           `json:"oldStart"`
	OldLines int           `json:"oldLines"`
	NewStart int           `json:"newStart"`
	NewLines int           `json:"newLines"`
	Lines    []DiffLineDTO `json:"lines"`
}

// DiffFileDTO representa um arquivo alterado no diff estruturado.
type DiffFileDTO struct {
	Path      string        `json:"path"`
	OldPath   string        `json:"oldPath,omitempty"`
	Status    string        `json:"status"`
	Additions int           `json:"additions"`
	Deletions int           `json:"deletions"`
	IsBinary  bool          `json:"isBinary"`
	Hunks     []DiffHunkDTO `json:"hunks"`
}

// DiffDTO representa payload base de diff para o frontend.
type DiffDTO struct {
	Mode        string        `json:"mode"`
	FilePath    string        `json:"filePath,omitempty"`
	Raw         string        `json:"raw"`
	Files       []DiffFileDTO `json:"files"`
	IsBinary    bool          `json:"isBinary"`
	IsTruncated bool          `json:"isTruncated"`
}

// CommandResultDTO representa resultado de comando emitido por evento runtime.
type CommandResultDTO struct {
	CommandID       string   `json:"commandId"`
	RepoPath        string   `json:"repoPath"`
	Action          string   `json:"action"`
	Args            []string `json:"args,omitempty"`
	DurationMs      int64    `json:"durationMs"`
	ExitCode        int      `json:"exitCode"`
	StderrSanitized string   `json:"stderrSanitized,omitempty"`
	Status          string   `json:"status"`
	Attempt         int      `json:"attempt,omitempty"`
	Error           string   `json:"error,omitempty"`
}
