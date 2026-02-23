package github

import "time"

// === Core Types ===

// User representa um usuário do GitHub
type User struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatarUrl"`
}

// Label representa uma label do GitHub
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Repository representa um repositório do GitHub
type Repository struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"fullName"`
	Owner         string    `json:"owner"`
	Description   string    `json:"description"`
	IsPrivate     bool      `json:"isPrivate"`
	DefaultBranch string    `json:"defaultBranch"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// === Pull Requests ===

// PullRequest representa um PR do GitHub
type PullRequest struct {
	ID           string    `json:"id"`
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	State        string    `json:"state"` // "OPEN", "CLOSED", "MERGED"
	Author       User      `json:"author"`
	Reviewers    []User    `json:"reviewers"`
	Labels       []Label   `json:"labels"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	MergeCommit  *string   `json:"mergeCommit,omitempty"`
	HeadBranch   string    `json:"headBranch"`
	BaseBranch   string    `json:"baseBranch"`
	Additions    int       `json:"additions"`
	Deletions    int       `json:"deletions"`
	ChangedFiles int       `json:"changedFiles"`
	IsDraft      bool      `json:"isDraft"`
}

// PRFilters define os filtros para listagem de PRs
type PRFilters struct {
	State     string   `json:"state"` // "OPEN", "CLOSED", "MERGED", "ALL"
	Author    *string  `json:"author"`
	Labels    []string `json:"labels"`
	OrderBy   string   `json:"orderBy"`   // "CREATED_AT", "UPDATED_AT"
	Direction string   `json:"direction"` // "ASC", "DESC"
	First     int      `json:"first"`
	After     *string  `json:"after"` // Cursor para paginação
}

// MergeMethod define o método de merge
type MergeMethod string

const (
	MergeMethodMerge  MergeMethod = "MERGE"
	MergeMethodSquash MergeMethod = "SQUASH"
	MergeMethodRebase MergeMethod = "REBASE"
)

// CreatePRInput define os campos para criação de um PR
type CreatePRInput struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	HeadBranch string `json:"headBranch"`
	BaseBranch string `json:"baseBranch"`
	IsDraft    bool   `json:"isDraft"`
}

// === Diff ===

// Diff representa o diff completo de um PR
type Diff struct {
	Files      []DiffFile     `json:"files"`
	TotalFiles int            `json:"totalFiles"`
	Pagination DiffPagination `json:"pagination"`
}

// DiffFile representa um arquivo individual no diff
type DiffFile struct {
	Filename  string     `json:"filename"`
	Status    string     `json:"status"` // "added", "modified", "deleted", "renamed"
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	Patch     string     `json:"patch,omitempty"` // Raw patch
	Hunks     []DiffHunk `json:"hunks"`
}

// DiffHunk representa um bloco de mudanças dentro de um arquivo
type DiffHunk struct {
	OldStart int        `json:"oldStart"`
	OldLines int        `json:"oldLines"`
	NewStart int        `json:"newStart"`
	NewLines int        `json:"newLines"`
	Header   string     `json:"header"`
	Lines    []DiffLine `json:"lines"`
}

// DiffLine representa uma linha individual no diff
type DiffLine struct {
	Type    string `json:"type"` // "add", "delete", "context"
	Content string `json:"content"`
	OldLine *int   `json:"oldLine,omitempty"`
	NewLine *int   `json:"newLine,omitempty"`
}

// DiffPagination controla a paginação de diffs
type DiffPagination struct {
	First       int     `json:"first"`
	After       *string `json:"after"`
	HasNextPage bool    `json:"hasNextPage"`
	EndCursor   *string `json:"endCursor"`
}

// === Reviews & Comentários ===

// Review representa um review de PR
type Review struct {
	ID        string    `json:"id"`
	Author    User      `json:"author"`
	State     string    `json:"state"` // "APPROVED", "CHANGES_REQUESTED", "COMMENTED", "PENDING"
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// Comment representa um comentário
type Comment struct {
	ID        string    `json:"id"`
	Author    User      `json:"author"`
	Body      string    `json:"body"`
	Path      *string   `json:"path,omitempty"` // Para inline comments
	Line      *int      `json:"line,omitempty"` // Linha no diff
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CreateReviewInput define os campos para criação de um review
type CreateReviewInput struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"prNumber"`
	Body     string `json:"body"`
	Event    string `json:"event"` // "APPROVE", "REQUEST_CHANGES", "COMMENT"
}

// CreateCommentInput define os campos para criação de um comentário
type CreateCommentInput struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"prNumber"`
	Body     string `json:"body"`
}

// InlineCommentInput define os campos para um comentário inline
type InlineCommentInput struct {
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	PRNumber int    `json:"prNumber"`
	Body     string `json:"body"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Side     string `json:"side"` // "LEFT" ou "RIGHT"
}

// === Issues ===

// Issue representa uma issue do GitHub
type Issue struct {
	ID        string    `json:"id"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"` // "OPEN", "CLOSED"
	Author    User      `json:"author"`
	Assignees []User    `json:"assignees"`
	Labels    []Label   `json:"labels"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// IssueFilters define os filtros para listagem de issues
type IssueFilters struct {
	State    string   `json:"state"`
	Labels   []string `json:"labels"`
	Assignee *string  `json:"assignee"`
	First    int      `json:"first"`
	After    *string  `json:"after"`
}

// CreateIssueInput define os campos para criação de uma issue
type CreateIssueInput struct {
	Owner     string   `json:"owner"`
	Repo      string   `json:"repo"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Labels    []string `json:"labels,omitempty"`
	Assignees []string `json:"assignees,omitempty"`
}

// UpdateIssueInput define os campos para atualização de uma issue
type UpdateIssueInput struct {
	Title     *string  `json:"title,omitempty"`
	Body      *string  `json:"body,omitempty"`
	State     *string  `json:"state,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	Assignees []string `json:"assignees,omitempty"`
}

// === Branches ===

// Branch representa uma branch do repositório
type Branch struct {
	Name   string `json:"name"`
	Prefix string `json:"prefix"` // "refs/heads/"
	Commit string `json:"commit"` // SHA do último commit
}

// === Pagination ===

// PageInfo contém informações de paginação GraphQL
type PageInfo struct {
	HasNextPage bool    `json:"hasNextPage"`
	EndCursor   *string `json:"endCursor"`
	TotalCount  int     `json:"totalCount"`
}

// === Error Types ===

// GitHubError representa um erro da API do GitHub
type GitHubError struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
	Type       string `json:"type"` // "auth", "ratelimit", "notfound", "permission", "conflict", "network"
}

func (e *GitHubError) Error() string {
	return e.Message
}

// IGitHubService define a interface do serviço GitHub
type IGitHubService interface {
	// Repositórios
	ListRepositories() ([]Repository, error)

	// Pull Requests
	ListPullRequests(owner, repo string, filters PRFilters) ([]PullRequest, error)
	GetPullRequest(owner, repo string, number int) (*PullRequest, error)
	GetPullRequestDiff(owner, repo string, number int, pagination DiffPagination) (*Diff, error)
	CreatePullRequest(input CreatePRInput) (*PullRequest, error)
	MergePullRequest(owner, repo string, number int, method MergeMethod) error
	ClosePullRequest(owner, repo string, number int) error

	// Reviews & Comentários
	ListReviews(owner, repo string, prNumber int) ([]Review, error)
	CreateReview(input CreateReviewInput) (*Review, error)
	ListComments(owner, repo string, prNumber int) ([]Comment, error)
	CreateComment(input CreateCommentInput) (*Comment, error)
	CreateInlineComment(input InlineCommentInput) (*Comment, error)

	// Issues
	ListIssues(owner, repo string, filters IssueFilters) ([]Issue, error)
	CreateIssue(input CreateIssueInput) (*Issue, error)
	UpdateIssue(owner, repo string, number int, input UpdateIssueInput) error

	// Branches
	ListBranches(owner, repo string) ([]Branch, error)
	CreateBranch(owner, repo, name, sourceBranch string) (*Branch, error)

	// Cache & Polling
	InvalidateCache(owner, repo string)
	ResolveCommitAuthors(owner, repo string, hashes []string) (map[string]User, error)
}
