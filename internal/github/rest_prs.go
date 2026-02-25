package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type restPullRequest struct {
	NodeID              string     `json:"node_id"`
	Number              int        `json:"number"`
	Title               string     `json:"title"`
	Body                string     `json:"body"`
	State               string     `json:"state"`
	Draft               bool       `json:"draft"`
	MaintainerCanModify *bool      `json:"maintainer_can_modify"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	MergedAt            *time.Time `json:"merged_at"`
	MergeCommitSHA      *string    `json:"merge_commit_sha"`
	Additions           int        `json:"additions"`
	Deletions           int        `json:"deletions"`
	ChangedFiles        int        `json:"changed_files"`
	User                struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
	RequestedReviewers []struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"requested_reviewers"`
	Labels []struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	} `json:"labels"`
	Head struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

// ListPullRequests lista PRs de um repositório via GitHub REST API v3.
func (s *Service) ListPullRequests(owner, repo string, filters PRFilters) ([]PullRequest, error) {
	normalizedOwner, normalizedRepo, normalizeErr := normalizeOwnerRepoForPR(owner, repo)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	endpointPath := fmt.Sprintf(
		"/repos/%s/%s/pulls",
		url.PathEscape(normalizedOwner),
		url.PathEscape(normalizedRepo),
	)
	requestStartedAt := time.Now()

	state, mergedOnly, err := normalizePRRESTState(filters.State)
	if err != nil {
		return nil, &GitHubError{
			StatusCode: 422,
			Message:    err.Error(),
			Type:       "validation",
		}
	}

	page := filters.Page
	if page <= 0 {
		page = 1
	}

	perPage := filters.PerPage
	if perPage <= 0 {
		perPage = filters.First
	}
	if perPage <= 0 {
		perPage = 25
	}
	if perPage > 100 {
		perPage = 100
	}

	cacheState := state
	if mergedOnly {
		cacheState = "merged"
	}

	cacheKey := prListKey(normalizedOwner, normalizedRepo, cacheState, page, perPage)
	if prs, ok := s.cache.GetPRs(normalizedOwner, normalizedRepo, cacheState, page, perPage); ok {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusOK, requestStartedAt, "hit")
		return prs, nil
	}
	stalePRs, hasStale := s.cache.GetPRsStale(normalizedOwner, normalizedRepo, cacheState, page, perPage)
	ifNoneMatch := ""
	if hasStale {
		if etag, ok := s.cache.GetETag(cacheKey); ok {
			ifNoneMatch = etag
		}
	}

	query := url.Values{}
	query.Set("state", state)
	query.Set("page", strconv.Itoa(page))
	query.Set("per_page", strconv.Itoa(perPage))

	respBody, headers, statusCode, err := s.executeRESTRequestConditional(
		http.MethodGet,
		endpointPath,
		query,
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
			return stalePRs, nil
		}
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusNotModified, requestStartedAt, "miss")
		return nil, &GitHubError{
			StatusCode: http.StatusNotModified,
			Message:    "received 304 without cached pull requests",
			Type:       "unknown",
		}
	}

	var response []restPullRequest
	if err := json.Unmarshal(respBody, &response); err != nil {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCode, requestStartedAt, "miss")
		return nil, fmt.Errorf("failed to parse GitHub REST response: %w", err)
	}

	prs := make([]PullRequest, 0, len(response))
	for _, prItem := range response {
		pr := parseRESTPullRequest(prItem)
		if mergedOnly && pr.State != "MERGED" {
			continue
		}
		prs = append(prs, pr)

		// Reaproveita detalhe no cache para otimizar abertura imediata.
		prCopy := pr
		s.cache.SetPR(normalizedOwner, normalizedRepo, pr.Number, &prCopy)
	}

	s.cache.SetPRs(normalizedOwner, normalizedRepo, cacheState, page, perPage, prs)
	s.cache.SetETag(cacheKey, headers.Get("ETag"))
	s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCode, requestStartedAt, "miss")

	return prs, nil
}

// GetPullRequest busca detalhes de um PR via GitHub REST API v3.
func (s *Service) GetPullRequest(owner, repo string, number int) (*PullRequest, error) {
	normalizedOwner, normalizedRepo, normalizeErr := normalizeOwnerRepoForPR(owner, repo)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	if number <= 0 {
		return nil, &GitHubError{StatusCode: 422, Message: "pull request number must be > 0", Type: "validation"}
	}
	endpointPath := fmt.Sprintf(
		"/repos/%s/%s/pulls/%d",
		url.PathEscape(normalizedOwner),
		url.PathEscape(normalizedRepo),
		number,
	)
	requestStartedAt := time.Now()

	cacheKey := prDetailKey(normalizedOwner, normalizedRepo, number)
	if pr, ok := s.cache.GetPR(normalizedOwner, normalizedRepo, number); ok {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusOK, requestStartedAt, "hit")
		return pr, nil
	}
	stalePR, hasStale := s.cache.GetPRStale(normalizedOwner, normalizedRepo, number)
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
			return stalePR, nil
		}
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, http.StatusNotModified, requestStartedAt, "miss")
		return nil, &GitHubError{
			StatusCode: http.StatusNotModified,
			Message:    "received 304 without cached pull request detail",
			Type:       "unknown",
		}
	}

	var response restPullRequest
	if err := json.Unmarshal(respBody, &response); err != nil {
		s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCode, requestStartedAt, "miss")
		return nil, err
	}

	pr := parseRESTPullRequest(response)
	s.cache.SetPR(normalizedOwner, normalizedRepo, number, &pr)
	s.cache.SetETag(cacheKey, headers.Get("ETag"))
	s.emitPRReadTelemetry(http.MethodGet, endpointPath, statusCode, requestStartedAt, "miss")
	return &pr, nil
}

func (s *Service) executePRRESTJSON(action, method, path string, query url.Values, payload interface{}, result interface{}) error {
	requestStartedAt := time.Now()

	var requestBody io.Reader
	if payload != nil {
		rawPayload, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			wrappedErr := fmt.Errorf("failed to marshal rest payload: %w", marshalErr)
			s.emitPRActionResultTelemetry(action, method, path, 0, requestStartedAt, wrappedErr)
			return wrappedErr
		}
		requestBody = bytes.NewReader(rawPayload)
	}

	responseBody, _, statusCode, err := s.executeRESTRequestConditional(
		method,
		path,
		query,
		githubRESTAcceptJSON,
		requestBody,
		"",
	)
	if err != nil {
		s.emitPRActionResultTelemetry(action, method, path, statusCode, requestStartedAt, err)
		return err
	}

	if result == nil || len(responseBody) == 0 {
		s.emitPRActionResultTelemetry(action, method, path, statusCode, requestStartedAt, nil)
		return nil
	}

	if err := json.Unmarshal(responseBody, result); err != nil {
		wrappedErr := fmt.Errorf("failed to parse GitHub REST response: %w", err)
		s.emitPRActionResultTelemetry(action, method, path, statusCode, requestStartedAt, wrappedErr)
		return wrappedErr
	}

	s.emitPRActionResultTelemetry(action, method, path, statusCode, requestStartedAt, nil)
	return nil
}

func parseRESTPullRequest(raw restPullRequest) PullRequest {
	state := strings.ToUpper(strings.TrimSpace(raw.State))
	if raw.MergedAt != nil {
		state = "MERGED"
	}
	if state == "" {
		state = "OPEN"
	}

	labels := make([]Label, 0, len(raw.Labels))
	for _, label := range raw.Labels {
		labels = append(labels, Label{
			Name:  strings.TrimSpace(label.Name),
			Color: strings.TrimSpace(label.Color),
		})
	}

	reviewers := make([]User, 0, len(raw.RequestedReviewers))
	for _, reviewer := range raw.RequestedReviewers {
		reviewers = append(reviewers, User{
			Login:     strings.TrimSpace(reviewer.Login),
			AvatarURL: strings.TrimSpace(reviewer.AvatarURL),
		})
	}

	var mergeCommit *string
	if raw.MergeCommitSHA != nil {
		trimmed := strings.TrimSpace(*raw.MergeCommitSHA)
		if trimmed != "" {
			mergeCommit = &trimmed
		}
	}

	var maintainerCanModify *bool
	if raw.MaintainerCanModify != nil {
		flag := *raw.MaintainerCanModify
		maintainerCanModify = &flag
	}

	return PullRequest{
		ID:                  strings.TrimSpace(raw.NodeID),
		Number:              raw.Number,
		Title:               strings.TrimSpace(raw.Title),
		Body:                raw.Body,
		State:               state,
		Author:              User{Login: strings.TrimSpace(raw.User.Login), AvatarURL: strings.TrimSpace(raw.User.AvatarURL)},
		Reviewers:           reviewers,
		Labels:              labels,
		CreatedAt:           raw.CreatedAt,
		UpdatedAt:           raw.UpdatedAt,
		MergeCommit:         mergeCommit,
		HeadBranch:          strings.TrimSpace(raw.Head.Ref),
		BaseBranch:          strings.TrimSpace(raw.Base.Ref),
		Additions:           raw.Additions,
		Deletions:           raw.Deletions,
		ChangedFiles:        raw.ChangedFiles,
		IsDraft:             raw.Draft,
		MaintainerCanModify: maintainerCanModify,
	}
}

func normalizePRRESTState(rawState string) (state string, mergedOnly bool, err error) {
	normalized := strings.TrimSpace(rawState)
	if normalized == "" {
		return "open", false, nil
	}

	switch strings.ToUpper(normalized) {
	case "OPEN":
		return "open", false, nil
	case "CLOSED":
		return "closed", false, nil
	case "ALL":
		return "all", false, nil
	case "MERGED":
		return "closed", true, nil
	}

	switch strings.ToLower(normalized) {
	case "open":
		return "open", false, nil
	case "closed":
		return "closed", false, nil
	case "all":
		return "all", false, nil
	}

	return "", false, fmt.Errorf("invalid pull request state %q (use open, closed, all)", rawState)
}
