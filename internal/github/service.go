package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	githubGraphQLEndpoint = "https://api.github.com/graphql"
	defaultCacheTTL       = 30 * time.Second
)

const resolveCommitAuthorsBatchSize = 20

// Service implementa IGitHubService
type Service struct {
	token     func() (string, error) // Função que retorna o token GitHub
	client    *http.Client
	cache     *Cache
	rateLeft  int // Rate limit remaining
	rateReset time.Time
}

// NewService cria um novo serviço GitHub
// tokenFn é uma função que retorna o token de acesso GitHub do usuário autenticado
func NewService(tokenFn func() (string, error)) *Service {
	return &Service{
		token: tokenFn,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache:    NewCache(defaultCacheTTL),
		rateLeft: 5000,
	}
}

// === GraphQL Client ===

// graphqlRequest representa um request GraphQL
type graphqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// graphqlResponse representa um response GraphQL
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"errors,omitempty"`
}

// executeQuery executa uma query/mutation GraphQL
func (s *Service) executeQuery(query string, variables map[string]interface{}) (json.RawMessage, error) {
	token, err := s.token()
	if err != nil {
		return nil, &GitHubError{StatusCode: 401, Message: "Not authenticated with GitHub", Type: "auth"}
	}

	reqBody := graphqlRequest{
		Query:     query,
		Variables: variables,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", githubGraphQLEndpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ORCH-App/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, &GitHubError{StatusCode: 0, Message: "Network error: " + err.Error(), Type: "network"}
	}
	defer resp.Body.Close()

	// Atualizar rate limit info dos headers
	s.updateRateLimit(resp.Header)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Tratar erros HTTP
	if resp.StatusCode != 200 {
		return nil, s.handleHTTPError(resp.StatusCode, respBody)
	}

	var gqlResp graphqlResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return nil, &GitHubError{
			StatusCode: 200,
			Message:    gqlResp.Errors[0].Message,
			Type:       "graphql",
		}
	}

	return gqlResp.Data, nil
}

// updateRateLimit atualiza informações de rate limit dos headers HTTP
func (s *Service) updateRateLimit(headers http.Header) {
	if remaining := headers.Get("X-RateLimit-Remaining"); remaining != "" {
		if n, err := strconv.Atoi(remaining); err == nil {
			s.rateLeft = n
		}
	}
	if reset := headers.Get("X-RateLimit-Reset"); reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			s.rateReset = time.Unix(ts, 0)
		}
	}

	// Avisar se rate limit está baixo
	if s.rateLeft < 100 {
		log.Printf("[GitHub] Rate limit low: %d remaining, resets at %s", s.rateLeft, s.rateReset.Format(time.RFC3339))
	}
}

// handleHTTPError converte erro HTTP em GitHubError tipado
func (s *Service) handleHTTPError(statusCode int, body []byte) *GitHubError {
	msg := string(body)

	switch statusCode {
	case 401:
		return &GitHubError{StatusCode: 401, Message: "GitHub token expired or invalid", Type: "auth"}
	case 403:
		if s.rateLeft <= 0 {
			return &GitHubError{StatusCode: 403, Message: fmt.Sprintf("Rate limit exceeded. Resets at %s", s.rateReset.Format(time.Kitchen)), Type: "ratelimit"}
		}
		return &GitHubError{StatusCode: 403, Message: "Permission denied: " + msg, Type: "permission"}
	case 404:
		return &GitHubError{StatusCode: 404, Message: "Resource not found", Type: "notfound"}
	case 409:
		return &GitHubError{StatusCode: 409, Message: "Merge conflict detected", Type: "conflict"}
	case 429:
		return &GitHubError{StatusCode: 429, Message: "Too many requests", Type: "ratelimit"}
	default:
		return &GitHubError{StatusCode: statusCode, Message: fmt.Sprintf("GitHub API error %d: %s", statusCode, msg), Type: "unknown"}
	}
}

// === Repositories ===

// ListRepositories lista repositórios do usuário autenticado
func (s *Service) ListRepositories() ([]Repository, error) {
	// Checar cache
	if repos, ok := s.cache.GetRepos(); ok {
		return repos, nil
	}

	data, err := s.executeQuery(QueryListRepositories, map[string]interface{}{
		"first": 50,
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		Viewer struct {
			Repositories struct {
				Nodes []struct {
					ID               string                 `json:"id"`
					Name             string                 `json:"name"`
					NameWithOwner    string                 `json:"nameWithOwner"`
					Owner            struct{ Login string } `json:"owner"`
					Description      *string                `json:"description"`
					IsPrivate        bool                   `json:"isPrivate"`
					DefaultBranchRef *struct{ Name string } `json:"defaultBranchRef"`
					UpdatedAt        time.Time              `json:"updatedAt"`
				} `json:"nodes"`
			} `json:"repositories"`
		} `json:"viewer"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse repositories: %w", err)
	}

	repos := make([]Repository, len(result.Viewer.Repositories.Nodes))
	for i, n := range result.Viewer.Repositories.Nodes {
		desc := ""
		if n.Description != nil {
			desc = *n.Description
		}
		defaultBranch := "main"
		if n.DefaultBranchRef != nil {
			defaultBranch = n.DefaultBranchRef.Name
		}
		repos[i] = Repository{
			ID:            n.ID,
			Name:          n.Name,
			FullName:      n.NameWithOwner,
			Owner:         n.Owner.Login,
			Description:   desc,
			IsPrivate:     n.IsPrivate,
			DefaultBranch: defaultBranch,
			UpdatedAt:     n.UpdatedAt,
		}
	}

	s.cache.SetRepos(repos)
	log.Printf("[GitHub] Fetched %d repositories", len(repos))
	return repos, nil
}

// === Pull Requests ===

// ListPullRequests lista PRs de um repositório
func (s *Service) ListPullRequests(owner, repo string, filters PRFilters) ([]PullRequest, error) {
	if filters.First == 0 {
		filters.First = 25
	}

	// Checar cache (sem filtros complexos)
	if filters.Author == nil && len(filters.Labels) == 0 && filters.After == nil {
		if prs, ok := s.cache.GetPRs(owner, repo); ok {
			return filterPRsByState(prs, filters.State), nil
		}
	}

	// Converter estado para GraphQL enum
	var states []string
	switch filters.State {
	case "OPEN":
		states = []string{"OPEN"}
	case "CLOSED":
		states = []string{"CLOSED"}
	case "MERGED":
		states = []string{"MERGED"}
	default:
		states = nil // Todos
	}

	vars := map[string]interface{}{
		"owner": owner,
		"repo":  repo,
		"first": filters.First,
	}
	if states != nil {
		vars["states"] = states
	}
	if filters.After != nil {
		vars["after"] = *filters.After
	}

	data, err := s.executeQuery(QueryListPullRequests, vars)
	if err != nil {
		return nil, err
	}

	var result struct {
		Repository struct {
			PullRequests struct {
				TotalCount int      `json:"totalCount"`
				Nodes      []prNode `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse pull requests: %w", err)
	}

	prs := make([]PullRequest, len(result.Repository.PullRequests.Nodes))
	for i, n := range result.Repository.PullRequests.Nodes {
		prs[i] = parsePRNode(n)
	}

	// Cachear se é query sem filtros
	if filters.Author == nil && len(filters.Labels) == 0 && filters.After == nil {
		s.cache.SetPRs(owner, repo, prs)
	}

	log.Printf("[GitHub] Fetched %d PRs from %s/%s", len(prs), owner, repo)
	return prs, nil
}

// GetPullRequest busca detalhes de um PR
func (s *Service) GetPullRequest(owner, repo string, number int) (*PullRequest, error) {
	if pr, ok := s.cache.GetPR(owner, repo, number); ok {
		return pr, nil
	}

	data, err := s.executeQuery(QueryGetPullRequest, map[string]interface{}{
		"owner":  owner,
		"repo":   repo,
		"number": number,
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		Repository struct {
			PullRequest prNode `json:"pullRequest"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse pull request: %w", err)
	}

	pr := parsePRNode(result.Repository.PullRequest)
	s.cache.SetPR(owner, repo, number, &pr)
	return &pr, nil
}

// GetPullRequestDiff busca o diff de um PR
func (s *Service) GetPullRequestDiff(owner, repo string, number int, pagination DiffPagination) (*Diff, error) {
	if pagination.First == 0 {
		pagination.First = 20 // Chunk de 20 arquivos
	}

	vars := map[string]interface{}{
		"owner":  owner,
		"repo":   repo,
		"number": number,
		"first":  pagination.First,
	}
	if pagination.After != nil {
		vars["after"] = *pagination.After
	}

	data, err := s.executeQuery(QueryGetPRDiff, vars)
	if err != nil {
		return nil, err
	}

	var result struct {
		Repository struct {
			PullRequest struct {
				Files struct {
					TotalCount int `json:"totalCount"`
					PageInfo   struct {
						HasNextPage bool    `json:"hasNextPage"`
						EndCursor   *string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						Path       string  `json:"path"`
						Additions  int     `json:"additions"`
						Deletions  int     `json:"deletions"`
						ChangeType string  `json:"changeType"`
						Patch      *string `json:"patch"`
					} `json:"nodes"`
				} `json:"files"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse diff: %w", err)
	}

	files := make([]DiffFile, len(result.Repository.PullRequest.Files.Nodes))
	for i, n := range result.Repository.PullRequest.Files.Nodes {
		patch := ""
		if n.Patch != nil {
			patch = *n.Patch
		}
		files[i] = DiffFile{
			Filename:  n.Path,
			Status:    strings.ToLower(n.ChangeType),
			Additions: n.Additions,
			Deletions: n.Deletions,
			Patch:     patch,
			Hunks:     parsePatch(patch),
		}
	}

	pageInfo := result.Repository.PullRequest.Files.PageInfo
	diff := &Diff{
		Files:      files,
		TotalFiles: result.Repository.PullRequest.Files.TotalCount,
		Pagination: DiffPagination{
			First:       pagination.First,
			HasNextPage: pageInfo.HasNextPage,
			EndCursor:   pageInfo.EndCursor,
		},
	}

	return diff, nil
}

// CreatePullRequest cria um novo PR
func (s *Service) CreatePullRequest(input CreatePRInput) (*PullRequest, error) {
	// Primeiro, buscar o repositoryId
	repoData, err := s.executeQuery(`query($owner: String!, $repo: String!) { repository(owner: $owner, name: $repo) { id } }`,
		map[string]interface{}{"owner": input.Owner, "repo": input.Repo})
	if err != nil {
		return nil, err
	}

	var repoResult struct {
		Repository struct{ ID string } `json:"repository"`
	}
	if err := json.Unmarshal(repoData, &repoResult); err != nil {
		return nil, err
	}

	data, err := s.executeQuery(MutationCreatePR, map[string]interface{}{
		"input": map[string]interface{}{
			"repositoryId": repoResult.Repository.ID,
			"title":        input.Title,
			"body":         input.Body,
			"headRefName":  input.HeadBranch,
			"baseRefName":  input.BaseBranch,
			"draft":        input.IsDraft,
		},
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		CreatePullRequest struct {
			PullRequest struct {
				ID     string `json:"id"`
				Number int    `json:"number"`
				Title  string `json:"title"`
				State  string `json:"state"`
			} `json:"pullRequest"`
		} `json:"createPullRequest"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	pr := &PullRequest{
		ID:         result.CreatePullRequest.PullRequest.ID,
		Number:     result.CreatePullRequest.PullRequest.Number,
		Title:      result.CreatePullRequest.PullRequest.Title,
		State:      result.CreatePullRequest.PullRequest.State,
		HeadBranch: input.HeadBranch,
		BaseBranch: input.BaseBranch,
		IsDraft:    input.IsDraft,
	}

	// Invalidar cache do repo
	s.cache.Invalidate(input.Owner, input.Repo)

	log.Printf("[GitHub] Created PR #%d: %s", pr.Number, pr.Title)
	return pr, nil
}

// MergePullRequest faz merge de um PR
func (s *Service) MergePullRequest(owner, repo string, number int, method MergeMethod) error {
	pr, err := s.GetPullRequest(owner, repo, number)
	if err != nil {
		return err
	}

	_, err = s.executeQuery(MutationMergePR, map[string]interface{}{
		"input": map[string]interface{}{
			"pullRequestId": pr.ID,
			"mergeMethod":   string(method),
		},
	})
	if err != nil {
		return err
	}

	s.cache.Invalidate(owner, repo)
	log.Printf("[GitHub] Merged PR #%d (%s)", number, method)
	return nil
}

// ClosePullRequest fecha um PR
func (s *Service) ClosePullRequest(owner, repo string, number int) error {
	pr, err := s.GetPullRequest(owner, repo, number)
	if err != nil {
		return err
	}

	_, err = s.executeQuery(MutationClosePR, map[string]interface{}{
		"input": map[string]interface{}{
			"pullRequestId": pr.ID,
		},
	})
	if err != nil {
		return err
	}

	s.cache.Invalidate(owner, repo)
	log.Printf("[GitHub] Closed PR #%d", number)
	return nil
}

// === Reviews ===

// ListReviews lista reviews de um PR
func (s *Service) ListReviews(owner, repo string, prNumber int) ([]Review, error) {
	if reviews, ok := s.cache.GetReviews(owner, repo, prNumber); ok {
		return reviews, nil
	}

	data, err := s.executeQuery(QueryListReviews, map[string]interface{}{
		"owner":  owner,
		"repo":   repo,
		"number": prNumber,
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		Repository struct {
			PullRequest struct {
				Reviews struct {
					Nodes []struct {
						ID        string    `json:"id"`
						State     string    `json:"state"`
						Body      string    `json:"body"`
						CreatedAt time.Time `json:"createdAt"`
						Author    struct {
							Login     string `json:"login"`
							AvatarURL string `json:"avatarUrl"`
						} `json:"author"`
					} `json:"nodes"`
				} `json:"reviews"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	reviews := make([]Review, len(result.Repository.PullRequest.Reviews.Nodes))
	for i, n := range result.Repository.PullRequest.Reviews.Nodes {
		reviews[i] = Review{
			ID:        n.ID,
			Author:    User{Login: n.Author.Login, AvatarURL: n.Author.AvatarURL},
			State:     n.State,
			Body:      n.Body,
			CreatedAt: n.CreatedAt,
		}
	}

	s.cache.SetReviews(owner, repo, prNumber, reviews)
	return reviews, nil
}

// CreateReview cria um review em um PR
func (s *Service) CreateReview(input CreateReviewInput) (*Review, error) {
	pr, err := s.GetPullRequest(input.Owner, input.Repo, input.PRNumber)
	if err != nil {
		return nil, err
	}

	data, err := s.executeQuery(MutationCreateReview, map[string]interface{}{
		"input": map[string]interface{}{
			"pullRequestId": pr.ID,
			"body":          input.Body,
			"event":         input.Event,
		},
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		AddPullRequestReview struct {
			PullRequestReview struct {
				ID        string    `json:"id"`
				State     string    `json:"state"`
				Body      string    `json:"body"`
				CreatedAt time.Time `json:"createdAt"`
				Author    struct {
					Login     string `json:"login"`
					AvatarURL string `json:"avatarUrl"`
				} `json:"author"`
			} `json:"pullRequestReview"`
		} `json:"addPullRequestReview"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	r := result.AddPullRequestReview.PullRequestReview
	review := &Review{
		ID:        r.ID,
		Author:    User{Login: r.Author.Login, AvatarURL: r.Author.AvatarURL},
		State:     r.State,
		Body:      r.Body,
		CreatedAt: r.CreatedAt,
	}

	// Invalidar cache de reviews
	s.cache.Invalidate(input.Owner, input.Repo)
	log.Printf("[GitHub] Created review on PR #%d: %s", input.PRNumber, input.Event)
	return review, nil
}

// === Comments ===

// ListComments lista comentários de um PR
func (s *Service) ListComments(owner, repo string, prNumber int) ([]Comment, error) {
	if comments, ok := s.cache.GetComments(owner, repo, prNumber); ok {
		return comments, nil
	}

	data, err := s.executeQuery(QueryListComments, map[string]interface{}{
		"owner":  owner,
		"repo":   repo,
		"number": prNumber,
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		Repository struct {
			PullRequest struct {
				Comments struct {
					Nodes []commentNode `json:"nodes"`
				} `json:"comments"`
				ReviewThreads struct {
					Nodes []struct {
						Comments struct {
							Nodes []commentNode `json:"nodes"`
						} `json:"comments"`
					} `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	// Combinar comentários gerais e de review
	var comments []Comment

	for _, n := range result.Repository.PullRequest.Comments.Nodes {
		comments = append(comments, parseCommentNode(n))
	}

	for _, thread := range result.Repository.PullRequest.ReviewThreads.Nodes {
		for _, n := range thread.Comments.Nodes {
			comments = append(comments, parseCommentNode(n))
		}
	}

	s.cache.SetComments(owner, repo, prNumber, comments)
	return comments, nil
}

// CreateComment cria um comentário em um PR
func (s *Service) CreateComment(input CreateCommentInput) (*Comment, error) {
	pr, err := s.GetPullRequest(input.Owner, input.Repo, input.PRNumber)
	if err != nil {
		return nil, err
	}

	data, err := s.executeQuery(MutationCreateComment, map[string]interface{}{
		"input": map[string]interface{}{
			"subjectId": pr.ID,
			"body":      input.Body,
		},
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		AddComment struct {
			CommentEdge struct {
				Node commentNode `json:"node"`
			} `json:"commentEdge"`
		} `json:"addComment"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	comment := parseCommentNode(result.AddComment.CommentEdge.Node)
	s.cache.Invalidate(input.Owner, input.Repo)
	log.Printf("[GitHub] Created comment on PR #%d", input.PRNumber)
	return &comment, nil
}

// CreateInlineComment cria um comentário inline no diff (via REST API, pois GraphQL não suporta position facilmente)
func (s *Service) CreateInlineComment(input InlineCommentInput) (*Comment, error) {
	// Para inline comments, usaremos a REST API v3 como fallback
	token, err := s.token()
	if err != nil {
		return nil, &GitHubError{StatusCode: 401, Message: "Not authenticated", Type: "auth"}
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/comments", input.Owner, input.Repo, input.PRNumber)

	payload := map[string]interface{}{
		"body": input.Body,
		"path": input.Path,
		"line": input.Line,
		"side": input.Side,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, &GitHubError{StatusCode: 0, Message: "Network error", Type: "network"}
	}
	defer resp.Body.Close()

	s.updateRateLimit(resp.Header)

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 201 {
		return nil, s.handleHTTPError(resp.StatusCode, respBody)
	}

	var restComment struct {
		ID        int       `json:"id"`
		Body      string    `json:"body"`
		Path      string    `json:"path"`
		Line      int       `json:"line"`
		CreatedAt time.Time `json:"created_at"`
		User      struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"user"`
	}

	if err := json.Unmarshal(respBody, &restComment); err != nil {
		return nil, err
	}

	path := restComment.Path
	line := restComment.Line
	comment := &Comment{
		ID:        strconv.Itoa(restComment.ID),
		Author:    User{Login: restComment.User.Login, AvatarURL: restComment.User.AvatarURL},
		Body:      restComment.Body,
		Path:      &path,
		Line:      &line,
		CreatedAt: restComment.CreatedAt,
		UpdatedAt: restComment.CreatedAt,
	}

	s.cache.Invalidate(input.Owner, input.Repo)
	log.Printf("[GitHub] Created inline comment on %s:%d in PR #%d", input.Path, input.Line, input.PRNumber)
	return comment, nil
}

// === Issues ===

// ListIssues lista issues de um repositório
func (s *Service) ListIssues(owner, repo string, filters IssueFilters) ([]Issue, error) {
	if filters.First == 0 {
		filters.First = 25
	}

	if filters.Assignee == nil && len(filters.Labels) == 0 && filters.After == nil {
		if issues, ok := s.cache.GetIssues(owner, repo); ok {
			return filterIssuesByState(issues, filters.State), nil
		}
	}

	var states []string
	if filters.State != "" && filters.State != "ALL" {
		states = []string{filters.State}
	}

	vars := map[string]interface{}{
		"owner": owner,
		"repo":  repo,
		"first": filters.First,
	}
	if states != nil {
		vars["states"] = states
	}
	if len(filters.Labels) > 0 {
		vars["labels"] = filters.Labels
	}
	if filters.After != nil {
		vars["after"] = *filters.After
	}

	data, err := s.executeQuery(QueryListIssues, vars)
	if err != nil {
		return nil, err
	}

	var result struct {
		Repository struct {
			Issues struct {
				Nodes []issueNode `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	issues := make([]Issue, len(result.Repository.Issues.Nodes))
	for i, n := range result.Repository.Issues.Nodes {
		issues[i] = parseIssueNode(n)
	}

	if filters.Assignee == nil && len(filters.Labels) == 0 && filters.After == nil {
		s.cache.SetIssues(owner, repo, issues)
	}

	log.Printf("[GitHub] Fetched %d issues from %s/%s", len(issues), owner, repo)
	return issues, nil
}

// CreateIssue cria uma nova issue
func (s *Service) CreateIssue(input CreateIssueInput) (*Issue, error) {
	repoData, err := s.executeQuery(`query($owner: String!, $repo: String!) { repository(owner: $owner, name: $repo) { id } }`,
		map[string]interface{}{"owner": input.Owner, "repo": input.Repo})
	if err != nil {
		return nil, err
	}

	var repoResult struct {
		Repository struct{ ID string } `json:"repository"`
	}
	if err := json.Unmarshal(repoData, &repoResult); err != nil {
		return nil, err
	}

	vars := map[string]interface{}{
		"input": map[string]interface{}{
			"repositoryId": repoResult.Repository.ID,
			"title":        input.Title,
			"body":         input.Body,
		},
	}

	data, err := s.executeQuery(MutationCreateIssue, vars)
	if err != nil {
		return nil, err
	}

	var result struct {
		CreateIssue struct {
			Issue struct {
				ID        string    `json:"id"`
				Number    int       `json:"number"`
				Title     string    `json:"title"`
				State     string    `json:"state"`
				CreatedAt time.Time `json:"createdAt"`
			} `json:"issue"`
		} `json:"createIssue"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	issue := &Issue{
		ID:        result.CreateIssue.Issue.ID,
		Number:    result.CreateIssue.Issue.Number,
		Title:     result.CreateIssue.Issue.Title,
		State:     result.CreateIssue.Issue.State,
		CreatedAt: result.CreateIssue.Issue.CreatedAt,
	}

	s.cache.Invalidate(input.Owner, input.Repo)
	log.Printf("[GitHub] Created issue #%d: %s", issue.Number, issue.Title)
	return issue, nil
}

// UpdateIssue atualiza uma issue
func (s *Service) UpdateIssue(owner, repo string, number int, input UpdateIssueInput) error {
	issueData, err := s.executeQuery(`query($owner: String!, $repo: String!, $number: Int!) { repository(owner: $owner, name: $repo) { issue(number: $number) { id } } }`,
		map[string]interface{}{"owner": owner, "repo": repo, "number": number})
	if err != nil {
		return err
	}

	var issueResult struct {
		Repository struct{ Issue struct{ ID string } } `json:"repository"`
	}
	if err := json.Unmarshal(issueData, &issueResult); err != nil {
		return err
	}

	updateInput := map[string]interface{}{
		"id": issueResult.Repository.Issue.ID,
	}
	if input.Title != nil {
		updateInput["title"] = *input.Title
	}
	if input.Body != nil {
		updateInput["body"] = *input.Body
	}
	if input.State != nil {
		updateInput["state"] = *input.State
	}

	_, err = s.executeQuery(MutationUpdateIssue, map[string]interface{}{
		"input": updateInput,
	})
	if err != nil {
		return err
	}

	s.cache.Invalidate(owner, repo)
	log.Printf("[GitHub] Updated issue #%d", number)
	return nil
}

// === Branches ===

// ListBranches lista branches de um repositório
func (s *Service) ListBranches(owner, repo string) ([]Branch, error) {
	if branches, ok := s.cache.GetBranches(owner, repo); ok {
		return branches, nil
	}

	data, err := s.executeQuery(QueryListBranches, map[string]interface{}{
		"owner": owner,
		"repo":  repo,
		"first": 100,
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		Repository struct {
			Refs struct {
				Nodes []struct {
					Name   string `json:"name"`
					Prefix string `json:"prefix"`
					Target struct {
						OID string `json:"oid"`
					} `json:"target"`
				} `json:"nodes"`
			} `json:"refs"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	branches := make([]Branch, len(result.Repository.Refs.Nodes))
	for i, n := range result.Repository.Refs.Nodes {
		branches[i] = Branch{
			Name:   n.Name,
			Prefix: n.Prefix,
			Commit: n.Target.OID,
		}
	}

	s.cache.SetBranches(owner, repo, branches)
	log.Printf("[GitHub] Fetched %d branches from %s/%s", len(branches), owner, repo)
	return branches, nil
}

// CreateBranch cria uma nova branch
func (s *Service) CreateBranch(owner, repo, name, sourceBranch string) (*Branch, error) {
	// Buscar OID da source branch
	branches, err := s.ListBranches(owner, repo)
	if err != nil {
		return nil, err
	}

	var sourceOID string
	for _, b := range branches {
		if b.Name == sourceBranch {
			sourceOID = b.Commit
			break
		}
	}
	if sourceOID == "" {
		return nil, &GitHubError{StatusCode: 404, Message: "Source branch not found: " + sourceBranch, Type: "notfound"}
	}

	// Buscar repositoryId
	repoData, err := s.executeQuery(`query($owner: String!, $repo: String!) { repository(owner: $owner, name: $repo) { id } }`,
		map[string]interface{}{"owner": owner, "repo": repo})
	if err != nil {
		return nil, err
	}

	var repoResult struct {
		Repository struct{ ID string } `json:"repository"`
	}
	if err := json.Unmarshal(repoData, &repoResult); err != nil {
		return nil, err
	}

	data, err := s.executeQuery(MutationCreateBranch, map[string]interface{}{
		"input": map[string]interface{}{
			"repositoryId": repoResult.Repository.ID,
			"name":         "refs/heads/" + name,
			"oid":          sourceOID,
		},
	})
	if err != nil {
		return nil, err
	}

	var result struct {
		CreateRef struct {
			Ref struct {
				Name   string `json:"name"`
				Prefix string `json:"prefix"`
				Target struct {
					OID string `json:"oid"`
				} `json:"target"`
			} `json:"ref"`
		} `json:"createRef"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	branch := &Branch{
		Name:   result.CreateRef.Ref.Name,
		Prefix: result.CreateRef.Ref.Prefix,
		Commit: result.CreateRef.Ref.Target.OID,
	}

	s.cache.Invalidate(owner, repo)
	log.Printf("[GitHub] Created branch %s from %s", name, sourceBranch)
	return branch, nil
}

// === Cache ===

// InvalidateCache invalida o cache de um repositório
func (s *Service) InvalidateCache(owner, repo string) {
	s.cache.Invalidate(owner, repo)
	log.Printf("[GitHub] Cache invalidated for %s/%s", owner, repo)
}

// GetCachedPullRequest retorna um PR do cache sem acessar a API.
func (s *Service) GetCachedPullRequest(owner, repo string, number int) (*PullRequest, bool) {
	return s.cache.GetPR(owner, repo, number)
}

// ResolveCommitAuthors resolve login/avatar de autores de commits por SHA.
// Retorna apenas commits encontrados e vinculados a usuários do GitHub.
func (s *Service) ResolveCommitAuthors(owner, repo string, hashes []string) (map[string]User, error) {
	normalizedOwner := strings.TrimSpace(owner)
	normalizedRepo := strings.TrimSpace(repo)
	if normalizedOwner == "" || normalizedRepo == "" {
		return map[string]User{}, nil
	}

	uniqueHashes := normalizeCommitHashes(hashes)
	if len(uniqueHashes) == 0 {
		return map[string]User{}, nil
	}

	resolved := make(map[string]User, len(uniqueHashes))

	for start := 0; start < len(uniqueHashes); start += resolveCommitAuthorsBatchSize {
		end := start + resolveCommitAuthorsBatchSize
		if end > len(uniqueHashes) {
			end = len(uniqueHashes)
		}
		batch := uniqueHashes[start:end]
		query, aliasToHash := buildResolveCommitAuthorsQuery(batch)
		if len(aliasToHash) == 0 {
			continue
		}

		data, err := s.executeQuery(query, map[string]interface{}{
			"owner": normalizedOwner,
			"repo":  normalizedRepo,
		})
		if err != nil {
			return resolved, err
		}

		parsed, parseErr := parseResolveCommitAuthorsResponse(data, aliasToHash)
		if parseErr != nil {
			return resolved, parseErr
		}

		for hash, user := range parsed {
			resolved[hash] = user
		}
	}

	return resolved, nil
}

// === Internal Helpers ===

var commitHashRegex = regexp.MustCompile(`^[a-f0-9]{7,40}$`)

func normalizeCommitHashes(hashes []string) []string {
	if len(hashes) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(hashes))
	seen := make(map[string]struct{}, len(hashes))

	for _, raw := range hashes {
		hash := strings.ToLower(strings.TrimSpace(raw))
		if hash == "" || !commitHashRegex.MatchString(hash) {
			continue
		}
		if _, exists := seen[hash]; exists {
			continue
		}
		seen[hash] = struct{}{}
		normalized = append(normalized, hash)
	}

	return normalized
}

func buildResolveCommitAuthorsQuery(hashes []string) (string, map[string]string) {
	aliasToHash := make(map[string]string, len(hashes))
	if len(hashes) == 0 {
		return "", aliasToHash
	}

	var builder strings.Builder
	builder.Grow(256 + len(hashes)*128)
	builder.WriteString("query ResolveCommitAuthors($owner: String!, $repo: String!) { repository(owner: $owner, name: $repo) {")

	for index, hash := range hashes {
		alias := fmt.Sprintf("c%d", index)
		aliasToHash[alias] = hash
		builder.WriteString(alias)
		builder.WriteString(`: object(oid: "`)
		builder.WriteString(hash)
		builder.WriteString(`") { ... on Commit { oid author { user { login avatarUrl } } } }`)
	}

	builder.WriteString("} }")
	return builder.String(), aliasToHash
}

func parseResolveCommitAuthorsResponse(data json.RawMessage, aliasToHash map[string]string) (map[string]User, error) {
	resolved := make(map[string]User, len(aliasToHash))
	if len(aliasToHash) == 0 {
		return resolved, nil
	}

	var payload struct {
		Repository map[string]json.RawMessage `json:"repository"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse commit authors payload: %w", err)
	}
	if payload.Repository == nil {
		return resolved, nil
	}

	for alias, hash := range aliasToHash {
		rawNode, ok := payload.Repository[alias]
		if !ok || len(rawNode) == 0 || bytes.Equal(rawNode, []byte("null")) {
			continue
		}

		var node struct {
			OID    string `json:"oid"`
			Author struct {
				User *struct {
					Login     string `json:"login"`
					AvatarURL string `json:"avatarUrl"`
				} `json:"user"`
			} `json:"author"`
		}

		if err := json.Unmarshal(rawNode, &node); err != nil {
			continue
		}
		if node.Author.User == nil {
			continue
		}

		login := strings.TrimSpace(node.Author.User.Login)
		if login == "" {
			continue
		}

		resolved[hash] = User{
			Login:     login,
			AvatarURL: strings.TrimSpace(node.Author.User.AvatarURL),
		}
	}

	return resolved, nil
}

// prNode é o schema de parse para GraphQL PR
type prNode struct {
	ID           string    `json:"id"`
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	Body         string    `json:"body"`
	State        string    `json:"state"`
	IsDraft      bool      `json:"isDraft"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Additions    int       `json:"additions"`
	Deletions    int       `json:"deletions"`
	ChangedFiles int       `json:"changedFiles"`
	HeadRefName  string    `json:"headRefName"`
	BaseRefName  string    `json:"baseRefName"`
	MergeCommit  *struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
	Author struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatarUrl"`
	} `json:"author"`
	Labels struct {
		Nodes []struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"nodes"`
	} `json:"labels"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Login     string `json:"login"`
				AvatarURL string `json:"avatarUrl"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
}

func parsePRNode(n prNode) PullRequest {
	labels := make([]Label, len(n.Labels.Nodes))
	for i, l := range n.Labels.Nodes {
		labels[i] = Label{Name: l.Name, Color: l.Color}
	}

	reviewers := make([]User, len(n.ReviewRequests.Nodes))
	for i, r := range n.ReviewRequests.Nodes {
		reviewers[i] = User{Login: r.RequestedReviewer.Login, AvatarURL: r.RequestedReviewer.AvatarURL}
	}

	var mergeCommit *string
	if n.MergeCommit != nil {
		mergeCommit = &n.MergeCommit.OID
	}

	return PullRequest{
		ID:           n.ID,
		Number:       n.Number,
		Title:        n.Title,
		Body:         n.Body,
		State:        n.State,
		IsDraft:      n.IsDraft,
		Author:       User{Login: n.Author.Login, AvatarURL: n.Author.AvatarURL},
		Reviewers:    reviewers,
		Labels:       labels,
		CreatedAt:    n.CreatedAt,
		UpdatedAt:    n.UpdatedAt,
		MergeCommit:  mergeCommit,
		HeadBranch:   n.HeadRefName,
		BaseBranch:   n.BaseRefName,
		Additions:    n.Additions,
		Deletions:    n.Deletions,
		ChangedFiles: n.ChangedFiles,
	}
}

// commentNode é o schema de parse para GraphQL Comment
type commentNode struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	Path      *string   `json:"path"`
	Line      *int      `json:"line"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Author    struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatarUrl"`
	} `json:"author"`
}

func parseCommentNode(n commentNode) Comment {
	return Comment{
		ID:        n.ID,
		Author:    User{Login: n.Author.Login, AvatarURL: n.Author.AvatarURL},
		Body:      n.Body,
		Path:      n.Path,
		Line:      n.Line,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
	}
}

// issueNode é o schema de parse para GraphQL Issue
type issueNode struct {
	ID        string    `json:"id"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Author    struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatarUrl"`
	} `json:"author"`
	Assignees struct {
		Nodes []struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatarUrl"`
		} `json:"nodes"`
	} `json:"assignees"`
	Labels struct {
		Nodes []struct {
			Name  string `json:"name"`
			Color string `json:"color"`
		} `json:"nodes"`
	} `json:"labels"`
}

func parseIssueNode(n issueNode) Issue {
	assignees := make([]User, len(n.Assignees.Nodes))
	for i, a := range n.Assignees.Nodes {
		assignees[i] = User{Login: a.Login, AvatarURL: a.AvatarURL}
	}

	labels := make([]Label, len(n.Labels.Nodes))
	for i, l := range n.Labels.Nodes {
		labels[i] = Label{Name: l.Name, Color: l.Color}
	}

	return Issue{
		ID:        n.ID,
		Number:    n.Number,
		Title:     n.Title,
		Body:      n.Body,
		State:     n.State,
		Author:    User{Login: n.Author.Login, AvatarURL: n.Author.AvatarURL},
		Assignees: assignees,
		Labels:    labels,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
	}
}

// filterPRsByState filtra PRs por estado
func filterPRsByState(prs []PullRequest, state string) []PullRequest {
	if state == "" || state == "ALL" {
		return prs
	}
	var result []PullRequest
	for _, pr := range prs {
		if pr.State == state {
			result = append(result, pr)
		}
	}
	return result
}

// filterIssuesByState filtra issues por estado
func filterIssuesByState(issues []Issue, state string) []Issue {
	if state == "" || state == "ALL" {
		return issues
	}
	var result []Issue
	for _, issue := range issues {
		if issue.State == state {
			result = append(result, issue)
		}
	}
	return result
}

// parsePatch converte um patch string em DiffHunks estruturados
func parsePatch(patch string) []DiffHunk {
	if patch == "" {
		return nil
	}

	var hunks []DiffHunk

	// Regex para header do hunk: @@ -oldStart,oldLines +newStart,newLines @@
	hunkHeaderRe := regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)`)

	lines := strings.Split(patch, "\n")
	var currentHunk *DiffHunk
	oldLine := 0
	newLine := 0

	for _, line := range lines {
		if matches := hunkHeaderRe.FindStringSubmatch(line); matches != nil {
			// Novo hunk
			if currentHunk != nil {
				hunks = append(hunks, *currentHunk)
			}

			oldStart, _ := strconv.Atoi(matches[1])
			oldLines := 1
			if matches[2] != "" {
				oldLines, _ = strconv.Atoi(matches[2])
			}
			newStart, _ := strconv.Atoi(matches[3])
			newLines := 1
			if matches[4] != "" {
				newLines, _ = strconv.Atoi(matches[4])
			}

			currentHunk = &DiffHunk{
				OldStart: oldStart,
				OldLines: oldLines,
				NewStart: newStart,
				NewLines: newLines,
				Header:   strings.TrimSpace(matches[5]),
				Lines:    []DiffLine{},
			}
			oldLine = oldStart
			newLine = newStart
			continue
		}

		if currentHunk == nil {
			continue
		}

		if len(line) == 0 {
			// Linha vazia — contexto
			o := oldLine
			n := newLine
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    "context",
				Content: "",
				OldLine: &o,
				NewLine: &n,
			})
			oldLine++
			newLine++
		} else if line[0] == '+' {
			n := newLine
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    "add",
				Content: line[1:],
				NewLine: &n,
			})
			newLine++
		} else if line[0] == '-' {
			o := oldLine
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    "delete",
				Content: line[1:],
				OldLine: &o,
			})
			oldLine++
		} else if line[0] == ' ' {
			o := oldLine
			n := newLine
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    "context",
				Content: line[1:],
				OldLine: &o,
				NewLine: &n,
			})
			oldLine++
			newLine++
		}
	}

	if currentHunk != nil {
		hunks = append(hunks, *currentHunk)
	}

	return hunks
}
