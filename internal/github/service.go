package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	githubGraphQLEndpoint = "https://api.github.com/graphql"
	githubRESTEndpoint    = "https://api.github.com"
	githubRESTAPIVersion  = "2022-11-28"
	githubRESTAcceptJSON  = "application/vnd.github+json"
	githubRESTAcceptDiff  = "application/vnd.github.diff"
	defaultCacheTTL       = 30 * time.Second

	restReadRetryMaxAttempts = 3
	restReadRetryBaseDelay   = 250 * time.Millisecond
	restReadRetryMaxBackoff  = 2 * time.Second
	restReadRetryMaxWait     = 5 * time.Second
)

const (
	resolveCommitAuthorsBatchSize = 20
	defaultRESTPage               = 1
	defaultRESTPerPage            = 30
	maxRESTPerPage                = 100
	maxPRFilePatchBytes           = 128 * 1024
	maxLabelNameLength            = 50
	maxLabelDescriptionLength     = 100
)

// Service implementa IGitHubService
type Service struct {
	token        func() (string, error) // Função que retorna o token GitHub
	client       *http.Client
	cache        *Cache
	restEndpoint string
	telemetry    func(eventName string, payload interface{})
	rateLeft     int // Rate limit remaining
	rateReset    time.Time
	retrySleep   func(time.Duration)
	retryRand    func() float64
}

// NewService cria um novo serviço GitHub
// tokenFn é uma função que retorna o token de acesso GitHub do usuário autenticado
func NewService(tokenFn func() (string, error)) *Service {
	return &Service{
		token: tokenFn,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache:        NewCache(defaultCacheTTL),
		restEndpoint: githubRESTEndpoint,
		rateLeft:     5000,
		retrySleep:   time.Sleep,
		retryRand:    rand.Float64,
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

// executeRESTRequest executa chamadas REST para GitHub com headers oficiais de PR API.
func (s *Service) executeRESTRequest(method, endpointPath string, queryValues url.Values, acceptHeader string, body io.Reader) ([]byte, http.Header, error) {
	respBody, headers, statusCode, err := s.executeRESTRequestConditional(method, endpointPath, queryValues, acceptHeader, body, "")
	if err != nil {
		return nil, headers, err
	}
	if statusCode == http.StatusNotModified {
		return nil, headers, &GitHubError{
			StatusCode: http.StatusNotModified,
			Message:    "Not modified response without conditional request",
			Type:       "unknown",
		}
	}
	return respBody, headers, nil
}

// executeRESTRequestConditional executa request REST opcionalmente condicional via If-None-Match.
func (s *Service) executeRESTRequestConditional(
	method,
	endpointPath string,
	queryValues url.Values,
	acceptHeader string,
	body io.Reader,
	ifNoneMatch string,
) ([]byte, http.Header, int, error) {
	token, err := s.token()
	if err != nil {
		return nil, nil, 0, &GitHubError{StatusCode: 401, Message: "Not authenticated with GitHub", Type: "auth"}
	}

	normalizedPath := strings.TrimSpace(endpointPath)
	if normalizedPath == "" {
		return nil, nil, 0, fmt.Errorf("empty REST endpoint path")
	}
	if !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}

	baseURL := strings.TrimRight(strings.TrimSpace(s.restEndpoint), "/")
	if baseURL == "" {
		baseURL = githubRESTEndpoint
	}

	requestURL, err := url.Parse(baseURL + normalizedPath)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to build REST request URL: %w", err)
	}
	if len(queryValues) > 0 {
		requestURL.RawQuery = queryValues.Encode()
	}

	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	if normalizedMethod == "" {
		normalizedMethod = http.MethodGet
	}

	normalizedAccept := strings.TrimSpace(acceptHeader)
	if normalizedAccept == "" {
		normalizedAccept = githubRESTAcceptJSON
	}

	maxAttempts := 1
	if normalizedMethod == http.MethodGet {
		maxAttempts = restReadRetryMaxAttempts
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest(normalizedMethod, requestURL.String(), body)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("failed to create REST request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "ORCH-App/1.0")
		req.Header.Set("X-GitHub-Api-Version", githubRESTAPIVersion)
		req.Header.Set("Accept", normalizedAccept)
		if normalizedIfNoneMatch := strings.TrimSpace(ifNoneMatch); normalizedIfNoneMatch != "" {
			req.Header.Set("If-None-Match", normalizedIfNoneMatch)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, requestErr := s.client.Do(req)
		if requestErr != nil {
			wrapped := &GitHubError{StatusCode: 0, Message: "Network error: " + requestErr.Error(), Type: "network"}
			if delay, reason, shouldRetry := s.shouldRetryRESTRead(normalizedMethod, attempt, maxAttempts, 0, nil, nil, wrapped); shouldRetry {
				log.Printf("[GitHub][PR-REST] retrying read request method=%s path=%s attempt=%d/%d reason=%s delay=%s status=%d", normalizedMethod, normalizedPath, attempt+1, maxAttempts, reason, delay, 0)
				s.sleepRetryDelay(delay)
				continue
			}
			return nil, nil, 0, wrapped
		}

		s.updateRateLimit(resp.Header)

		if resp.StatusCode == http.StatusNotModified {
			_ = resp.Body.Close()
			return nil, resp.Header, resp.StatusCode, nil
		}

		respBody, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil && readErr == nil {
			readErr = closeErr
		}
		if readErr != nil {
			wrappedReadErr := fmt.Errorf("failed to read REST response: %w", readErr)
			if delay, reason, shouldRetry := s.shouldRetryRESTRead(normalizedMethod, attempt, maxAttempts, resp.StatusCode, resp.Header, nil, wrappedReadErr); shouldRetry {
				log.Printf("[GitHub][PR-REST] retrying read request method=%s path=%s attempt=%d/%d reason=%s delay=%s status=%d", normalizedMethod, normalizedPath, attempt+1, maxAttempts, reason, delay, resp.StatusCode)
				s.sleepRetryDelay(delay)
				continue
			}
			return nil, resp.Header, resp.StatusCode, wrappedReadErr
		}

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			httpErr := s.handleRESTHTTPError(normalizedMethod, normalizedPath, resp.StatusCode, resp.Header, respBody)
			if delay, reason, shouldRetry := s.shouldRetryRESTRead(normalizedMethod, attempt, maxAttempts, resp.StatusCode, resp.Header, respBody, nil); shouldRetry {
				log.Printf("[GitHub][PR-REST] retrying read request method=%s path=%s attempt=%d/%d reason=%s delay=%s status=%d", normalizedMethod, normalizedPath, attempt+1, maxAttempts, reason, delay, resp.StatusCode)
				s.sleepRetryDelay(delay)
				continue
			}
			return nil, resp.Header, resp.StatusCode, httpErr
		}

		return respBody, resp.Header, resp.StatusCode, nil
	}

	return nil, nil, 0, fmt.Errorf("failed to execute REST request")
}

func (s *Service) shouldRetryRESTRead(
	method string,
	attempt,
	maxAttempts,
	statusCode int,
	headers http.Header,
	responseBody []byte,
	requestErr error,
) (time.Duration, string, bool) {
	if strings.ToUpper(strings.TrimSpace(method)) != http.MethodGet {
		return 0, "", false
	}
	if attempt >= maxAttempts {
		return 0, "", false
	}

	if requestErr != nil {
		delay, ok := s.computeRESTReadRetryDelay(attempt, headers)
		if !ok {
			return 0, "", false
		}
		return delay, "read-error", true
	}

	switch statusCode {
	case http.StatusTooManyRequests:
		delay, ok := s.computeRESTReadRetryDelay(attempt, headers)
		if !ok {
			return 0, "", false
		}
		return delay, "rate-limit-429", true
	case http.StatusForbidden:
		if !isSecondaryRateLimitResponse(statusCode, headers, responseBody) {
			return 0, "", false
		}
		delay, ok := s.computeRESTReadRetryDelay(attempt, headers)
		if !ok {
			return 0, "", false
		}
		return delay, "secondary-rate-limit", true
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		delay, ok := s.computeRESTReadRetryDelay(attempt, headers)
		if !ok {
			return 0, "", false
		}
		return delay, fmt.Sprintf("http-%d", statusCode), true
	default:
		return 0, "", false
	}
}

func (s *Service) computeRESTReadRetryDelay(attempt int, headers http.Header) (time.Duration, bool) {
	if attempt <= 0 {
		attempt = 1
	}

	if retryAfter, ok := parseRetryAfterHeader(headers); ok {
		if retryAfter > restReadRetryMaxWait {
			return 0, false
		}
		return retryAfter + s.jitterDuration(retryAfter/4), true
	}

	backoff := restReadRetryBaseDelay
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if backoff >= restReadRetryMaxBackoff {
			backoff = restReadRetryMaxBackoff
			break
		}
	}

	halfBackoff := backoff / 2
	if halfBackoff <= 0 {
		return backoff, true
	}
	delay := halfBackoff + s.jitterDuration(halfBackoff)
	if delay > restReadRetryMaxWait {
		delay = restReadRetryMaxWait
	}
	if delay < 0 {
		delay = 0
	}
	return delay, true
}

func parseRetryAfterHeader(headers http.Header) (time.Duration, bool) {
	if headers == nil {
		return 0, false
	}

	rawRetryAfter := strings.TrimSpace(headers.Get("Retry-After"))
	if rawRetryAfter == "" {
		return 0, false
	}

	if retryAfterSeconds, err := strconv.Atoi(rawRetryAfter); err == nil {
		if retryAfterSeconds < 0 {
			retryAfterSeconds = 0
		}
		return time.Duration(retryAfterSeconds) * time.Second, true
	}

	if retryAt, err := http.ParseTime(rawRetryAfter); err == nil {
		delay := retryAt.Sub(time.Now())
		if delay < 0 {
			delay = 0
		}
		return delay, true
	}

	return 0, false
}

func isSecondaryRateLimitResponse(statusCode int, headers http.Header, body []byte) bool {
	if statusCode != http.StatusForbidden {
		return false
	}

	normalizedBody := strings.ToLower(strings.TrimSpace(string(body)))
	if strings.Contains(normalizedBody, "secondary rate limit") {
		return true
	}
	if strings.Contains(normalizedBody, "secondary limit") {
		return true
	}
	if strings.Contains(normalizedBody, "abuse detection") {
		return true
	}

	if strings.TrimSpace(headers.Get("Retry-After")) != "" {
		return strings.Contains(normalizedBody, "rate limit")
	}

	return false
}

func (s *Service) jitterDuration(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(s.retryRandomFloat64() * float64(max))
}

func (s *Service) retryRandomFloat64() float64 {
	if s == nil || s.retryRand == nil {
		return rand.Float64()
	}

	value := s.retryRand()
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func (s *Service) sleepRetryDelay(delay time.Duration) {
	if delay <= 0 {
		return
	}
	if s != nil && s.retrySleep != nil {
		s.retrySleep(delay)
		return
	}
	time.Sleep(delay)
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
	msg := parseGitHubErrorMessage(body)
	lowerMessage := strings.ToLower(msg)

	switch statusCode {
	case 401:
		return &GitHubError{StatusCode: 401, Message: "GitHub token expired or invalid", Type: "auth"}
	case 403:
		if s.rateLeft <= 0 || strings.Contains(lowerMessage, "rate limit") || strings.Contains(lowerMessage, "secondary limit") {
			if msg == "" {
				msg = "Rate limit exceeded."
			}
			return &GitHubError{StatusCode: 403, Message: fmt.Sprintf("Rate limit exceeded. Resets at %s", s.rateReset.Format(time.Kitchen)), Type: "ratelimit"}
		}
		return &GitHubError{StatusCode: 403, Message: "Permission denied: " + msg, Type: "permission"}
	case 404:
		return &GitHubError{StatusCode: 404, Message: "Resource not found", Type: "notfound"}
	case 409:
		return &GitHubError{StatusCode: 409, Message: "Merge conflict detected", Type: "conflict"}
	case 422:
		return &GitHubError{StatusCode: 422, Message: "Validation failed: " + msg, Type: "validation"}
	case 429:
		return &GitHubError{StatusCode: 429, Message: "Too many requests", Type: "ratelimit"}
	default:
		return &GitHubError{StatusCode: statusCode, Message: fmt.Sprintf("GitHub API error %d: %s", statusCode, msg), Type: "unknown"}
	}
}

func (s *Service) handleRESTHTTPError(method, endpointPath string, statusCode int, headers http.Header, body []byte) *GitHubError {
	githubErr := s.handleHTTPError(statusCode, body)
	if githubErr == nil {
		return nil
	}
	if statusCode != http.StatusForbidden || strings.TrimSpace(githubErr.Type) != "permission" {
		return githubErr
	}

	scopeHint := buildPRPermissionScopeHint(method, endpointPath, headers)
	if scopeHint == "" {
		return githubErr
	}

	if strings.TrimSpace(githubErr.Message) == "" {
		githubErr.Message = scopeHint
		return githubErr
	}

	githubErr.Message = strings.TrimSpace(githubErr.Message) + " | " + scopeHint
	return githubErr
}

func parseGitHubErrorMessage(body []byte) string {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return "empty error response"
	}

	var payload struct {
		Message string          `json:"message"`
		Errors  json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return raw
	}

	message := strings.TrimSpace(payload.Message)
	if message == "" {
		return raw
	}

	errorDetails := strings.TrimSpace(string(payload.Errors))
	switch errorDetails {
	case "", "null", "[]", "{}":
		return message
	default:
		return fmt.Sprintf("%s | errors=%s", message, errorDetails)
	}
}

func buildPRPermissionScopeHint(method, endpointPath string, headers http.Header) string {
	required := requiredPRPermissionsForRequest(method, endpointPath)
	parts := make([]string, 0, 4)
	if required != "" {
		parts = append(parts, "required="+required)
	}

	if headers != nil {
		if acceptedPermissions := sanitizePermissionHeaderValue(headers.Get("X-Accepted-GitHub-Permissions")); acceptedPermissions != "" {
			parts = append(parts, "accepted_permissions="+acceptedPermissions)
		}
		if tokenScopes := sanitizePermissionHeaderValue(headers.Get("X-OAuth-Scopes")); tokenScopes != "" {
			parts = append(parts, "token_scopes="+tokenScopes)
		}
		if acceptedScopes := sanitizePermissionHeaderValue(headers.Get("X-Accepted-OAuth-Scopes")); acceptedScopes != "" {
			parts = append(parts, "accepted_scopes="+acceptedScopes)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return fmt.Sprintf("Insufficient token scope for PR operation (%s)", strings.Join(parts, " | "))
}

func requiredPRPermissionsForRequest(method, endpointPath string) string {
	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	if normalizedMethod == http.MethodGet {
		return "pull_requests:read"
	}

	normalizedPath := strings.ToLower(strings.TrimSpace(endpointPath))
	if strings.HasSuffix(normalizedPath, "/merge") {
		return "pull_requests:write,contents:write"
	}

	return "pull_requests:write"
}

func sanitizePermissionHeaderValue(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return ""
	}

	normalized = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(normalized)
	normalized = strings.Join(strings.Fields(normalized), " ")
	if len(normalized) > 180 {
		normalized = normalized[:180] + "..."
	}
	return normalized
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

// GetPullRequestCommits busca commits de um PR via REST com paginação.
func (s *Service) GetPullRequestCommits(owner, repo string, number int, page, perPage int) (*PRCommitPage, error) {
	normalizedOwner, normalizedRepo, normalizeErr := normalizeOwnerRepoForPR(owner, repo)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	if number <= 0 {
		return nil, &GitHubError{StatusCode: 422, Message: "pull request number must be > 0", Type: "validation"}
	}
	endpointPath := fmt.Sprintf(
		"/repos/%s/%s/pulls/%d/commits",
		url.PathEscape(normalizedOwner),
		url.PathEscape(normalizedRepo),
		number,
	)
	requestStartedAt := time.Now()

	page, perPage = normalizeRESTPagination(page, perPage)
	if cached, ok := s.cache.GetPRCommitPage(normalizedOwner, normalizedRepo, number, page, perPage); ok {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusOK, requestStartedAt, "hit")
		return cached, nil
	}
	cacheKey := prCommitsKey(normalizedOwner, normalizedRepo, number, page, perPage)
	stalePage, hasStale := s.cache.GetPRCommitPageStale(normalizedOwner, normalizedRepo, number, page, perPage)
	ifNoneMatch := ""
	if hasStale {
		if etag, ok := s.cache.GetETag(cacheKey); ok {
			ifNoneMatch = etag
		}
	}

	queryValues := url.Values{}
	queryValues.Set("page", strconv.Itoa(page))
	queryValues.Set("per_page", strconv.Itoa(perPage))

	respBody, headers, statusCode, err := s.executeRESTRequestConditional(
		http.MethodGet,
		endpointPath,
		queryValues,
		githubRESTAcceptJSON,
		nil,
		ifNoneMatch,
	)
	if err != nil {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCodeFromGitHubError(err), requestStartedAt, "miss")
		return nil, err
	}
	if statusCode == http.StatusNotModified {
		if hasStale {
			s.cache.Touch(cacheKey)
			if etag := strings.TrimSpace(headers.Get("ETag")); etag != "" {
				s.cache.SetETag(cacheKey, etag)
			}
			s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusNotModified, requestStartedAt, "hit")
			return stalePage, nil
		}
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusNotModified, requestStartedAt, "miss")
		return nil, &GitHubError{
			StatusCode: http.StatusNotModified,
			Message:    "received 304 without cached pull request commits",
			Type:       "unknown",
		}
	}

	var payload []struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
			Author  struct {
				Name  string    `json:"name"`
				Email string    `json:"email"`
				Date  time.Time `json:"date"`
			} `json:"author"`
			Committer struct {
				Name  string    `json:"name"`
				Email string    `json:"email"`
				Date  time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
		Author *struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"author"`
		Committer *struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"committer"`
		Parents []struct {
			SHA string `json:"sha"`
		} `json:"parents"`
	}

	if err := json.Unmarshal(respBody, &payload); err != nil {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCode, requestStartedAt, "miss")
		return nil, fmt.Errorf("failed to parse pull request commits: %w", err)
	}

	items := make([]PRCommit, len(payload))
	for index, entry := range payload {
		parentSHAs := make([]string, 0, len(entry.Parents))
		for _, parent := range entry.Parents {
			sha := strings.TrimSpace(parent.SHA)
			if sha != "" {
				parentSHAs = append(parentSHAs, sha)
			}
		}

		commit := PRCommit{
			SHA:            strings.TrimSpace(entry.SHA),
			Message:        entry.Commit.Message,
			HTMLURL:        strings.TrimSpace(entry.HTMLURL),
			AuthorName:     strings.TrimSpace(entry.Commit.Author.Name),
			AuthorEmail:    strings.TrimSpace(entry.Commit.Author.Email),
			AuthoredAt:     entry.Commit.Author.Date,
			CommitterName:  strings.TrimSpace(entry.Commit.Committer.Name),
			CommitterEmail: strings.TrimSpace(entry.Commit.Committer.Email),
			CommittedAt:    entry.Commit.Committer.Date,
			ParentSHAs:     parentSHAs,
		}

		if entry.Author != nil {
			login := strings.TrimSpace(entry.Author.Login)
			avatar := strings.TrimSpace(entry.Author.AvatarURL)
			if login != "" || avatar != "" {
				commit.Author = &User{
					Login:     login,
					AvatarURL: avatar,
				}
			}
		}
		if entry.Committer != nil {
			login := strings.TrimSpace(entry.Committer.Login)
			avatar := strings.TrimSpace(entry.Committer.AvatarURL)
			if login != "" || avatar != "" {
				commit.Committer = &User{
					Login:     login,
					AvatarURL: avatar,
				}
			}
		}

		items[index] = commit
	}

	nextPage, hasNextPage := parseNextPageFromLinkHeader(headers.Get("Link"))

	result := PRCommitPage{
		Items:       items,
		Page:        page,
		PerPage:     perPage,
		HasNextPage: hasNextPage,
		NextPage:    nextPage,
	}
	s.cache.SetPRCommitPage(normalizedOwner, normalizedRepo, number, result)
	s.cache.SetETag(cacheKey, headers.Get("ETag"))
	s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCode, requestStartedAt, "miss")
	return &result, nil
}

// GetPullRequestFiles busca arquivos de um PR via REST com paginação.
func (s *Service) GetPullRequestFiles(owner, repo string, number int, page, perPage int) (*PRFilePage, error) {
	normalizedOwner, normalizedRepo, normalizeErr := normalizeOwnerRepoForPR(owner, repo)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	if number <= 0 {
		return nil, &GitHubError{StatusCode: 422, Message: "pull request number must be > 0", Type: "validation"}
	}
	endpointPath := fmt.Sprintf(
		"/repos/%s/%s/pulls/%d/files",
		url.PathEscape(normalizedOwner),
		url.PathEscape(normalizedRepo),
		number,
	)
	requestStartedAt := time.Now()

	page, perPage = normalizeRESTPagination(page, perPage)
	if cached, ok := s.cache.GetPRFilePage(normalizedOwner, normalizedRepo, number, page, perPage); ok {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusOK, requestStartedAt, "hit")
		return cached, nil
	}
	cacheKey := prFilesKey(normalizedOwner, normalizedRepo, number, page, perPage)
	stalePage, hasStale := s.cache.GetPRFilePageStale(normalizedOwner, normalizedRepo, number, page, perPage)
	ifNoneMatch := ""
	if hasStale {
		if etag, ok := s.cache.GetETag(cacheKey); ok {
			ifNoneMatch = etag
		}
	}

	queryValues := url.Values{}
	queryValues.Set("page", strconv.Itoa(page))
	queryValues.Set("per_page", strconv.Itoa(perPage))

	respBody, headers, statusCode, err := s.executeRESTRequestConditional(
		http.MethodGet,
		endpointPath,
		queryValues,
		githubRESTAcceptJSON,
		nil,
		ifNoneMatch,
	)
	if err != nil {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCodeFromGitHubError(err), requestStartedAt, "miss")
		return nil, err
	}
	if statusCode == http.StatusNotModified {
		if hasStale {
			s.cache.Touch(cacheKey)
			if etag := strings.TrimSpace(headers.Get("ETag")); etag != "" {
				s.cache.SetETag(cacheKey, etag)
			}
			s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusNotModified, requestStartedAt, "hit")
			return stalePage, nil
		}
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusNotModified, requestStartedAt, "miss")
		return nil, &GitHubError{
			StatusCode: http.StatusNotModified,
			Message:    "received 304 without cached pull request files",
			Type:       "unknown",
		}
	}

	var payload []struct {
		Filename         string  `json:"filename"`
		PreviousFilename string  `json:"previous_filename"`
		Status           string  `json:"status"`
		Additions        int     `json:"additions"`
		Deletions        int     `json:"deletions"`
		Changes          int     `json:"changes"`
		BlobURL          string  `json:"blob_url"`
		RawURL           string  `json:"raw_url"`
		ContentsURL      string  `json:"contents_url"`
		Patch            *string `json:"patch"`
	}

	if err := json.Unmarshal(respBody, &payload); err != nil {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCode, requestStartedAt, "miss")
		return nil, fmt.Errorf("failed to parse pull request files: %w", err)
	}

	items := make([]PRFile, len(payload))
	for index, entry := range payload {
		patch := ""
		hasPatch := false
		if entry.Patch != nil {
			patch = *entry.Patch
			hasPatch = strings.TrimSpace(patch) != ""
		}

		isBinary := false
		isPatchTruncated := false
		patchState := PRFilePatchStateMissing

		if hasPatch {
			if limitedPatch, limited := truncatePRFilePatch(patch); limited {
				patch = limitedPatch
				isPatchTruncated = true
			}

			if patchLooksBinary(patch) {
				isBinary = true
				hasPatch = false
				patch = ""
				patchState = PRFilePatchStateBinary
			} else if patchLooksTruncated(patch) || isPatchTruncated {
				isPatchTruncated = true
				patchState = PRFilePatchStateTruncated
			} else {
				patchState = PRFilePatchStateAvailable
			}
		} else if filenameLooksBinary(entry.Filename) {
			isBinary = true
			patchState = PRFilePatchStateBinary
		}

		items[index] = PRFile{
			Filename:         strings.TrimSpace(entry.Filename),
			PreviousFilename: strings.TrimSpace(entry.PreviousFilename),
			Status:           strings.TrimSpace(strings.ToLower(entry.Status)),
			Additions:        entry.Additions,
			Deletions:        entry.Deletions,
			Changes:          entry.Changes,
			BlobURL:          strings.TrimSpace(entry.BlobURL),
			RawURL:           strings.TrimSpace(entry.RawURL),
			ContentsURL:      strings.TrimSpace(entry.ContentsURL),
			Patch:            patch,
			HasPatch:         hasPatch,
			PatchState:       patchState,
			IsBinary:         isBinary,
			IsPatchTruncated: isPatchTruncated,
		}
	}

	nextPage, hasNextPage := parseNextPageFromLinkHeader(headers.Get("Link"))

	result := PRFilePage{
		Items:       items,
		Page:        page,
		PerPage:     perPage,
		HasNextPage: hasNextPage,
		NextPage:    nextPage,
	}
	s.cache.SetPRFilePage(normalizedOwner, normalizedRepo, number, result)
	s.cache.SetETag(cacheKey, headers.Get("ETag"))
	s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCode, requestStartedAt, "miss")
	return &result, nil
}

// GetPullRequestRawDiff busca o diff completo bruto de um PR sob demanda.
func (s *Service) GetPullRequestRawDiff(owner, repo string, number int) (string, error) {
	normalizedOwner, normalizedRepo, normalizeErr := normalizeOwnerRepoForPR(owner, repo)
	if normalizeErr != nil {
		return "", normalizeErr
	}
	if number <= 0 {
		return "", &GitHubError{StatusCode: 422, Message: "pull request number must be > 0", Type: "validation"}
	}
	endpointPath := fmt.Sprintf(
		"/repos/%s/%s/pulls/%d",
		url.PathEscape(normalizedOwner),
		url.PathEscape(normalizedRepo),
		number,
	)
	requestStartedAt := time.Now()
	if cached, ok := s.cache.GetPRRawDiff(normalizedOwner, normalizedRepo, number); ok {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusOK, requestStartedAt, "hit")
		return cached, nil
	}
	cacheKey := prRawDiffKey(normalizedOwner, normalizedRepo, number)
	staleDiff, hasStale := s.cache.GetPRRawDiffStale(normalizedOwner, normalizedRepo, number)
	ifNoneMatch := ""
	if hasStale {
		if etag, ok := s.cache.GetETag(cacheKey); ok {
			ifNoneMatch = etag
		}
	}

	respBody, headers, statusCode, err := s.executeRESTRequestConditional(
		http.MethodGet,
		endpointPath,
		nil,
		githubRESTAcceptDiff,
		nil,
		ifNoneMatch,
	)
	if err != nil {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCodeFromGitHubError(err), requestStartedAt, "miss")
		return "", err
	}
	if statusCode == http.StatusNotModified {
		if hasStale {
			s.cache.Touch(cacheKey)
			if etag := strings.TrimSpace(headers.Get("ETag")); etag != "" {
				s.cache.SetETag(cacheKey, etag)
			}
			s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusNotModified, requestStartedAt, "hit")
			return staleDiff, nil
		}
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusNotModified, requestStartedAt, "miss")
		return "", &GitHubError{
			StatusCode: http.StatusNotModified,
			Message:    "received 304 without cached pull request raw diff",
			Type:       "unknown",
		}
	}

	rawDiff := string(respBody)
	s.cache.SetPRRawDiff(normalizedOwner, normalizedRepo, number, rawDiff)
	s.cache.SetETag(cacheKey, headers.Get("ETag"))
	s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCode, requestStartedAt, "miss")
	return rawDiff, nil
}

// GetCommitRawDiff busca o diff bruto de um commit especifico.
func (s *Service) GetCommitRawDiff(owner, repo, sha string) (string, error) {
	normalizedOwner, normalizedRepo, normalizeErr := normalizeOwnerRepoForPR(owner, repo)
	if normalizeErr != nil {
		return "", normalizeErr
	}
	normalizedSHA := strings.TrimSpace(sha)
	if normalizedSHA == "" {
		return "", &GitHubError{StatusCode: 422, Message: "commit sha must not be empty", Type: "validation"}
	}

	endpointPath := fmt.Sprintf(
		"/repos/%s/%s/commits/%s",
		url.PathEscape(normalizedOwner),
		url.PathEscape(normalizedRepo),
		url.PathEscape(normalizedSHA),
	)
	requestStartedAt := time.Now()

	respBody, _, statusCode, err := s.executeRESTRequestConditional(
		http.MethodGet,
		endpointPath,
		nil,
		githubRESTAcceptDiff,
		nil,
		"",
	)
	if err != nil {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCodeFromGitHubError(err), requestStartedAt, "miss")
		return "", err
	}

	rawDiff := string(respBody)
	s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCode, requestStartedAt, "miss")
	return rawDiff, nil
}

// CheckPullRequestMerged verifica se uma PR ja foi mergeada via endpoint REST /merge.
func (s *Service) CheckPullRequestMerged(owner, repo string, number int) (bool, error) {
	normalizedOwner, normalizedRepo, normalizeErr := normalizeOwnerRepoForPR(owner, repo)
	if normalizeErr != nil {
		return false, normalizeErr
	}
	if number <= 0 {
		return false, &GitHubError{StatusCode: 422, Message: "pull request number must be > 0", Type: "validation"}
	}

	endpointPath := fmt.Sprintf(
		"/repos/%s/%s/pulls/%d/merge",
		url.PathEscape(normalizedOwner),
		url.PathEscape(normalizedRepo),
		number,
	)
	requestStartedAt := time.Now()

	if cached, ok := s.cache.GetPRMerged(normalizedOwner, normalizedRepo, number); ok {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusOK, requestStartedAt, "hit")
		return cached, nil
	}

	cacheKey := prMergeCheckKey(normalizedOwner, normalizedRepo, number)
	staleMerged, hasStale := s.cache.GetPRMergedStale(normalizedOwner, normalizedRepo, number)
	ifNoneMatch := ""
	if hasStale {
		if etag, ok := s.cache.GetETag(cacheKey); ok {
			ifNoneMatch = etag
		}
	}

	_, headers, statusCode, err := s.executeRESTRequestConditional(
		http.MethodGet,
		endpointPath,
		nil,
		githubRESTAcceptJSON,
		nil,
		ifNoneMatch,
	)
	if err != nil {
		var githubErr *GitHubError
		if errors.As(err, &githubErr) && githubErr != nil && githubErr.StatusCode == http.StatusNotFound {
			s.cache.SetPRMerged(normalizedOwner, normalizedRepo, number, false)
			s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusNotFound, requestStartedAt, "miss")
			return false, nil
		}
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCodeFromGitHubError(err), requestStartedAt, "miss")
		return false, err
	}

	if statusCode == http.StatusNotModified {
		if hasStale {
			s.cache.Touch(cacheKey)
			if etag := strings.TrimSpace(headers.Get("ETag")); etag != "" {
				s.cache.SetETag(cacheKey, etag)
			}
			s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusNotModified, requestStartedAt, "hit")
			return staleMerged, nil
		}

		s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusNotModified, requestStartedAt, "miss")
		return false, &GitHubError{
			StatusCode: http.StatusNotModified,
			Message:    "received 304 without cached pull request merge status",
			Type:       "unknown",
		}
	}

	merged := statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
	s.cache.SetPRMerged(normalizedOwner, normalizedRepo, number, merged)
	s.cache.SetETag(cacheKey, headers.Get("ETag"))

	s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCode, requestStartedAt, "miss")
	return merged, nil
}

// CreatePullRequest cria um novo PR
func (s *Service) CreatePullRequest(input CreatePRInput) (*PullRequest, error) {
	normalizedOwner, normalizedRepo, normalizeErr := normalizeOwnerRepoForPR(input.Owner, input.Repo)
	if normalizeErr != nil {
		return nil, normalizeErr
	}

	normalizedTitle := strings.TrimSpace(input.Title)
	if normalizedTitle == "" {
		return nil, &GitHubError{
			StatusCode: 422,
			Message:    "title required",
			Type:       "validation",
		}
	}

	normalizedHead := strings.TrimSpace(input.HeadBranch)
	if normalizedHead == "" {
		return nil, &GitHubError{
			StatusCode: 422,
			Message:    "head required",
			Type:       "validation",
		}
	}

	normalizedBase := strings.TrimSpace(input.BaseBranch)
	if normalizedBase == "" {
		return nil, &GitHubError{
			StatusCode: 422,
			Message:    "base required",
			Type:       "validation",
		}
	}

	requestPayload := map[string]interface{}{
		"title": normalizedTitle,
		"head":  normalizedHead,
		"base":  normalizedBase,
	}
	if normalizedBody := strings.TrimSpace(input.Body); normalizedBody != "" {
		requestPayload["body"] = input.Body
	}
	if input.IsDraft {
		requestPayload["draft"] = true
	}
	if input.MaintainerCanModify != nil {
		requestPayload["maintainer_can_modify"] = *input.MaintainerCanModify
	}

	endpointPath := fmt.Sprintf(
		"/repos/%s/%s/pulls",
		url.PathEscape(normalizedOwner),
		url.PathEscape(normalizedRepo),
	)

	var payloadResponse restPullRequest
	if err := s.executePRRESTJSON(prActionCreate, http.MethodPost, endpointPath, nil, requestPayload, &payloadResponse); err != nil {
		return nil, err
	}

	pr := parseRESTPullRequest(payloadResponse)

	s.cache.InvalidatePRLists(normalizedOwner, normalizedRepo)
	s.cache.SetPR(normalizedOwner, normalizedRepo, pr.Number, &pr)

	log.Printf("[GitHub] Created PR #%d: %s", pr.Number, pr.Title)
	return &pr, nil
}

func normalizePRRESTWritableState(rawState string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(rawState))
	switch normalized {
	case "open", "closed":
		return normalized, nil
	default:
		return "", fmt.Errorf(`state must be "open" or "closed"`)
	}
}

func normalizePRRESTMergeMethod(rawMethod string) (PRMergeMethod, error) {
	normalized := strings.ToLower(strings.TrimSpace(rawMethod))
	switch normalized {
	case "", string(PRMergeMethodMerge):
		return PRMergeMethodMerge, nil
	case string(PRMergeMethodSquash):
		return PRMergeMethodSquash, nil
	case string(PRMergeMethodRebase):
		return PRMergeMethodRebase, nil
	default:
		return "", fmt.Errorf(`merge method must be "merge", "squash" or "rebase"`)
	}
}

func normalizePRRESTOptionalSHA(rawSHA string) (string, error) {
	normalized := strings.TrimSpace(rawSHA)
	if normalized == "" {
		return "", nil
	}
	if !fullCommitHashRegex.MatchString(normalized) {
		return "", fmt.Errorf("sha must be a full 40-character hexadecimal commit hash")
	}
	return strings.ToLower(normalized), nil
}

// UpdatePullRequest atualiza campos de um PR via GitHub REST API v3.
func (s *Service) UpdatePullRequest(input UpdatePRInput) (*PullRequest, error) {
	normalizedOwner, normalizedRepo, normalizeErr := normalizeOwnerRepoForPR(input.Owner, input.Repo)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	if input.Number <= 0 {
		return nil, &GitHubError{
			StatusCode: 422,
			Message:    "pull request number must be > 0",
			Type:       "validation",
		}
	}

	requestPayload := map[string]interface{}{}
	if input.Title != nil {
		normalizedTitle := strings.TrimSpace(*input.Title)
		if normalizedTitle == "" {
			return nil, &GitHubError{
				StatusCode: 422,
				Message:    "title must not be empty when provided",
				Type:       "validation",
			}
		}
		requestPayload["title"] = normalizedTitle
	}
	if input.Body != nil {
		requestPayload["body"] = *input.Body
	}
	if input.State != nil {
		normalizedState, err := normalizePRRESTWritableState(*input.State)
		if err != nil {
			return nil, &GitHubError{
				StatusCode: 422,
				Message:    err.Error(),
				Type:       "validation",
			}
		}
		requestPayload["state"] = normalizedState
	}
	if input.BaseBranch != nil {
		normalizedBase := strings.TrimSpace(*input.BaseBranch)
		if normalizedBase == "" {
			return nil, &GitHubError{
				StatusCode: 422,
				Message:    "base must not be empty when provided",
				Type:       "validation",
			}
		}
		requestPayload["base"] = normalizedBase
	}
	if input.MaintainerCanModify != nil {
		requestPayload["maintainer_can_modify"] = *input.MaintainerCanModify
	}
	if len(requestPayload) == 0 {
		return nil, &GitHubError{
			StatusCode: 422,
			Message:    "at least one field must be provided for update",
			Type:       "validation",
		}
	}

	endpointPath := fmt.Sprintf(
		"/repos/%s/%s/pulls/%d",
		url.PathEscape(normalizedOwner),
		url.PathEscape(normalizedRepo),
		input.Number,
	)

	var payloadResponse restPullRequest
	if err := s.executePRRESTJSON(prActionUpdate, http.MethodPatch, endpointPath, nil, requestPayload, &payloadResponse); err != nil {
		return nil, err
	}

	pr := parseRESTPullRequest(payloadResponse)
	s.cache.InvalidatePRMutation(normalizedOwner, normalizedRepo, input.Number)
	s.cache.SetPR(normalizedOwner, normalizedRepo, pr.Number, &pr)

	log.Printf("[GitHub] Updated PR #%d", pr.Number)
	return &pr, nil
}

// MergePullRequestREST executa merge de PR via endpoint REST /pulls/{pull_number}/merge.
func (s *Service) MergePullRequestREST(input MergePRInput) (*PRMergeResult, error) {
	normalizedOwner, normalizedRepo, normalizeErr := normalizeOwnerRepoForPR(input.Owner, input.Repo)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	if input.Number <= 0 {
		return nil, &GitHubError{
			StatusCode: 422,
			Message:    "pull request number must be > 0",
			Type:       "validation",
		}
	}

	normalizedMethod, methodErr := normalizePRRESTMergeMethod(string(input.MergeMethod))
	if methodErr != nil {
		return nil, &GitHubError{
			StatusCode: 422,
			Message:    methodErr.Error(),
			Type:       "validation",
		}
	}

	requestPayload := map[string]interface{}{
		"merge_method": string(normalizedMethod),
	}

	if input.SHA != nil {
		normalizedSHA, shaErr := normalizePRRESTOptionalSHA(*input.SHA)
		if shaErr != nil {
			return nil, &GitHubError{
				StatusCode: 422,
				Message:    shaErr.Error(),
				Type:       "validation",
			}
		}
		if normalizedSHA != "" {
			requestPayload["sha"] = normalizedSHA
		}
	}

	endpointPath := fmt.Sprintf(
		"/repos/%s/%s/pulls/%d/merge",
		url.PathEscape(normalizedOwner),
		url.PathEscape(normalizedRepo),
		input.Number,
	)

	var payloadResponse struct {
		SHA     string `json:"sha"`
		Merged  bool   `json:"merged"`
		Message string `json:"message"`
	}
	if err := s.executePRRESTJSON(prActionMerge, http.MethodPut, endpointPath, nil, requestPayload, &payloadResponse); err != nil {
		return nil, err
	}

	result := PRMergeResult{
		SHA:     strings.TrimSpace(payloadResponse.SHA),
		Merged:  payloadResponse.Merged,
		Message: strings.TrimSpace(payloadResponse.Message),
	}
	if result.Message == "" {
		result.Message = "Pull request merge completed."
	}

	s.cache.Invalidate(normalizedOwner, normalizedRepo)
	log.Printf("[GitHub] Merged PR #%d (%s)", input.Number, normalizedMethod)
	return &result, nil
}

// UpdatePullRequestBranch atualiza branch de PR via endpoint REST /pulls/{pull_number}/update-branch.
func (s *Service) UpdatePullRequestBranch(input UpdatePRBranchInput) (*PRUpdateBranchResult, error) {
	normalizedOwner, normalizedRepo, normalizeErr := normalizeOwnerRepoForPR(input.Owner, input.Repo)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	if input.Number <= 0 {
		return nil, &GitHubError{
			StatusCode: 422,
			Message:    "pull request number must be > 0",
			Type:       "validation",
		}
	}

	requestPayload := map[string]interface{}{}
	if input.ExpectedHeadSHA != nil {
		normalizedSHA, shaErr := normalizePRRESTOptionalSHA(*input.ExpectedHeadSHA)
		if shaErr != nil {
			return nil, &GitHubError{
				StatusCode: 422,
				Message:    shaErr.Error(),
				Type:       "validation",
			}
		}
		if normalizedSHA != "" {
			requestPayload["expected_head_sha"] = normalizedSHA
		}
	}

	endpointPath := fmt.Sprintf(
		"/repos/%s/%s/pulls/%d/update-branch",
		url.PathEscape(normalizedOwner),
		url.PathEscape(normalizedRepo),
		input.Number,
	)

	var payloadResponse struct {
		Message string `json:"message"`
	}
	if err := s.executePRRESTJSON(prActionUpdateBranch, http.MethodPut, endpointPath, nil, requestPayload, &payloadResponse); err != nil {
		return nil, err
	}

	result := PRUpdateBranchResult{
		Message: strings.TrimSpace(payloadResponse.Message),
	}
	if result.Message == "" {
		result.Message = "Pull request branch update requested."
	}

	s.cache.Invalidate(normalizedOwner, normalizedRepo)
	log.Printf("[GitHub] Requested update-branch for PR #%d", input.Number)
	return &result, nil
}

// MergePullRequest faz merge de um PR
func (s *Service) MergePullRequest(owner, repo string, number int, method MergeMethod) error {
	_, err := s.MergePullRequestREST(MergePRInput{
		Owner:       owner,
		Repo:        repo,
		Number:      number,
		MergeMethod: PRMergeMethod(string(method)),
	})
	return err
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
	payload := map[string]interface{}{
		"body": input.Body,
		"path": input.Path,
		"line": input.Line,
		"side": input.Side,
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

	if err := s.executePRRESTJSON(
		prActionInlineCommentCreate,
		http.MethodPost,
		fmt.Sprintf("/repos/%s/%s/pulls/%d/comments", input.Owner, input.Repo, input.PRNumber),
		nil,
		payload,
		&restComment,
	); err != nil {
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

func normalizeLabelName(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", &GitHubError{
			StatusCode: 422,
			Message:    "label name required",
			Type:       "validation",
		}
	}
	if len([]rune(normalized)) > maxLabelNameLength {
		return "", &GitHubError{
			StatusCode: 422,
			Message:    fmt.Sprintf("label name too long (max %d characters)", maxLabelNameLength),
			Type:       "validation",
		}
	}
	return normalized, nil
}

func normalizeLabelColor(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	normalized = strings.TrimPrefix(normalized, "#")
	normalized = strings.ToLower(normalized)
	if !labelColorRegex.MatchString(normalized) {
		return "", &GitHubError{
			StatusCode: 422,
			Message:    "label color must be a 6-character hexadecimal value",
			Type:       "validation",
		}
	}
	return normalized, nil
}

func normalizeLabelDescription(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}

	normalized := strings.TrimSpace(*raw)
	if normalized == "" {
		return nil, nil
	}
	if len([]rune(normalized)) > maxLabelDescriptionLength {
		return nil, &GitHubError{
			StatusCode: 422,
			Message:    fmt.Sprintf("label description too long (max %d characters)", maxLabelDescriptionLength),
			Type:       "validation",
		}
	}
	return &normalized, nil
}

// CreateLabel cria uma label de repositorio via GitHub REST API v3.
func (s *Service) CreateLabel(input CreateLabelInput) (*Label, error) {
	normalizedOwner, normalizedRepo, normalizeErr := normalizeOwnerRepoForPR(input.Owner, input.Repo)
	if normalizeErr != nil {
		return nil, normalizeErr
	}

	normalizedName, nameErr := normalizeLabelName(input.Name)
	if nameErr != nil {
		return nil, nameErr
	}

	normalizedColor, colorErr := normalizeLabelColor(input.Color)
	if colorErr != nil {
		return nil, colorErr
	}

	normalizedDescription, descriptionErr := normalizeLabelDescription(input.Description)
	if descriptionErr != nil {
		return nil, descriptionErr
	}

	requestPayload := map[string]interface{}{
		"name":  normalizedName,
		"color": normalizedColor,
	}
	if normalizedDescription != nil {
		requestPayload["description"] = *normalizedDescription
	}

	endpointPath := fmt.Sprintf(
		"/repos/%s/%s/labels",
		url.PathEscape(normalizedOwner),
		url.PathEscape(normalizedRepo),
	)

	var payloadResponse struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Description string `json:"description"`
	}
	if err := s.executePRRESTJSON(
		prActionLabelCreate,
		http.MethodPost,
		endpointPath,
		nil,
		requestPayload,
		&payloadResponse,
	); err != nil {
		return nil, err
	}

	responseName := strings.TrimSpace(payloadResponse.Name)
	if responseName == "" {
		responseName = normalizedName
	}

	responseColor, responseColorErr := normalizeLabelColor(payloadResponse.Color)
	if responseColorErr != nil {
		responseColor = normalizedColor
	}

	responseDescription := strings.TrimSpace(payloadResponse.Description)
	label := &Label{
		Name:        responseName,
		Color:       responseColor,
		Description: responseDescription,
	}

	s.cache.Invalidate(normalizedOwner, normalizedRepo)
	log.Printf("[GitHub] Created label %q on %s/%s", label.Name, normalizedOwner, normalizedRepo)
	return label, nil
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

var (
	commitHashRegex     = regexp.MustCompile(`^[a-f0-9]{7,40}$`)
	fullCommitHashRegex = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)
	labelColorRegex     = regexp.MustCompile(`^[a-f0-9]{6}$`)
	binaryExtSet        = map[string]struct{}{
		".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".bmp": {}, ".webp": {}, ".ico": {},
		".pdf": {}, ".zip": {}, ".gz": {}, ".tar": {}, ".tgz": {}, ".7z": {}, ".rar": {},
		".mp3": {}, ".wav": {}, ".ogg": {}, ".mp4": {}, ".mov": {}, ".avi": {}, ".mkv": {},
		".ttf": {}, ".otf": {}, ".woff": {}, ".woff2": {}, ".eot": {},
		".exe": {}, ".dll": {}, ".so": {}, ".dylib": {}, ".bin": {}, ".wasm": {},
	}
)

func normalizeRESTPagination(page, perPage int) (int, int) {
	normalizedPage := page
	if normalizedPage < defaultRESTPage {
		normalizedPage = defaultRESTPage
	}

	normalizedPerPage := perPage
	if normalizedPerPage <= 0 {
		normalizedPerPage = defaultRESTPerPage
	}
	if normalizedPerPage > maxRESTPerPage {
		normalizedPerPage = maxRESTPerPage
	}

	return normalizedPage, normalizedPerPage
}

func parseNextPageFromLinkHeader(linkHeader string) (int, bool) {
	normalized := strings.TrimSpace(linkHeader)
	if normalized == "" {
		return 0, false
	}

	parts := strings.Split(normalized, ",")
	for _, part := range parts {
		segment := strings.TrimSpace(part)
		if segment == "" || !strings.Contains(segment, `rel="next"`) {
			continue
		}

		open := strings.Index(segment, "<")
		close := strings.Index(segment, ">")
		if open == -1 || close <= open+1 {
			return 0, true
		}

		nextURL, err := url.Parse(segment[open+1 : close])
		if err != nil {
			return 0, true
		}

		pageRaw := strings.TrimSpace(nextURL.Query().Get("page"))
		if pageRaw == "" {
			return 0, true
		}

		nextPage, atoiErr := strconv.Atoi(pageRaw)
		if atoiErr != nil || nextPage <= 0 {
			return 0, true
		}

		return nextPage, true
	}

	return 0, false
}

func patchLooksBinary(patch string) bool {
	normalizedPatch := strings.TrimSpace(patch)
	return strings.Contains(normalizedPatch, "Binary files ") || strings.Contains(normalizedPatch, "GIT binary patch")
}

func patchLooksTruncated(patch string) bool {
	normalizedPatch := strings.TrimSpace(patch)
	if normalizedPatch == "" {
		return false
	}

	lines := strings.Split(normalizedPatch, "\n")
	lastLine := strings.TrimSpace(lines[len(lines)-1])
	if lastLine == "..." {
		return true
	}

	return strings.Contains(normalizedPatch, "\n...\n")
}

func truncatePRFilePatch(patch string) (string, bool) {
	if len(patch) <= maxPRFilePatchBytes {
		return patch, false
	}

	trimmed := patch[:maxPRFilePatchBytes]
	if lineBreak := strings.LastIndex(trimmed, "\n"); lineBreak > 0 {
		trimmed = trimmed[:lineBreak]
	}

	trimmed = strings.TrimRight(trimmed, "\n")
	if trimmed == "" {
		trimmed = patch[:maxPRFilePatchBytes]
	}

	return trimmed + "\n...", true
}

func filenameLooksBinary(path string) bool {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(path)))
	if ext == "" {
		return false
	}
	_, ok := binaryExtSet[ext]
	return ok
}

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
