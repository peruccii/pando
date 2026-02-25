package github

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestListPullRequestsRESTRejectsUnsafeOwnerRepoBeforeHTTP(t *testing.T) {
	requestCount := 0
	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("[]")),
			}, nil
		}),
	}

	_, err := service.ListPullRequests("../bad-owner", "orch", PRFilters{State: "open"})
	if err == nil {
		t.Fatalf("expected validation error for invalid owner")
	}

	var githubErr *GitHubError
	if !errors.As(err, &githubErr) {
		t.Fatalf("expected GitHubError, got=%T err=%v", err, err)
	}
	if githubErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unexpected status code: got=%d want=%d", githubErr.StatusCode, http.StatusUnprocessableEntity)
	}
	if githubErr.Type != "validation" {
		t.Fatalf("unexpected error type: got=%q want=%q", githubErr.Type, "validation")
	}
	if requestCount != 0 {
		t.Fatalf("expected no HTTP calls for invalid owner/repo, got=%d", requestCount)
	}
}

func TestListPullRequestsRESTEmitsRequestTelemetryWithCacheState(t *testing.T) {
	requestCount := 0
	requestEvents := make([]PRRequestTelemetry, 0, 2)
	cacheEvents := make([]PRCacheTelemetry, 0, 2)

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.SetTelemetryEmitter(func(eventName string, payload interface{}) {
		switch eventName {
		case "gitpanel:prs_request":
			event, ok := payload.(PRRequestTelemetry)
			if !ok {
				t.Fatalf("unexpected request telemetry payload type: %T", payload)
			}
			requestEvents = append(requestEvents, event)
		case "gitpanel:prs_cache":
			event, ok := payload.(PRCacheTelemetry)
			if !ok {
				t.Fatalf("unexpected cache telemetry payload type: %T", payload)
			}
			cacheEvents = append(cacheEvents, event)
		}
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++
			headers := make(http.Header)
			headers.Set("X-RateLimit-Remaining", "4991")

			body := `[{
				"node_id":"PR_kwDOAAX",
				"number":42,
				"title":"Telemetry baseline",
				"state":"open",
				"created_at":"2026-02-20T10:00:00Z",
				"updated_at":"2026-02-20T10:00:00Z",
				"user":{"login":"dev","avatar_url":""},
				"head":{"ref":"feature/telemetry"},
				"base":{"ref":"main"}
			}]`

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     headers,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	_, err := service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("first ListPullRequests() error: %v", err)
	}
	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("second ListPullRequests() error: %v", err)
	}

	if requestCount != 1 {
		t.Fatalf("expected second request served from cache, requestCount=%d", requestCount)
	}
	if len(requestEvents) != 2 {
		t.Fatalf("expected 2 request telemetry events, got=%d", len(requestEvents))
	}
	if len(cacheEvents) != 2 {
		t.Fatalf("expected 2 cache telemetry events, got=%d", len(cacheEvents))
	}

	first := requestEvents[0]
	if first.Method != http.MethodGet {
		t.Fatalf("unexpected method on first request event: %q", first.Method)
	}
	if first.Endpoint != "/repos/orch-labs/orch/pulls" {
		t.Fatalf("unexpected endpoint on first request event: %q", first.Endpoint)
	}
	if first.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status on first request event: %d", first.StatusCode)
	}
	if first.Cache != "miss" {
		t.Fatalf("unexpected cache state on first request event: %q", first.Cache)
	}
	if first.RateRemaining != 4991 {
		t.Fatalf("unexpected rate remaining on first request event: %d", first.RateRemaining)
	}
	if first.DurationMs < 0 {
		t.Fatalf("duration should be non-negative, got=%d", first.DurationMs)
	}

	second := requestEvents[1]
	if second.Cache != "hit" {
		t.Fatalf("unexpected cache state on second request event: %q", second.Cache)
	}
	if second.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status on second request event: %d", second.StatusCode)
	}

	if cacheEvents[0].Cache != "miss" {
		t.Fatalf("unexpected first cache event state: %q", cacheEvents[0].Cache)
	}
	if cacheEvents[1].Cache != "hit" {
		t.Fatalf("unexpected second cache event state: %q", cacheEvents[1].Cache)
	}
}

func TestCreatePullRequestEmitsActionResultTelemetryOnSuccess(t *testing.T) {
	actionEvents := make([]PRActionResultTelemetry, 0, 1)

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.SetTelemetryEmitter(func(eventName string, payload interface{}) {
		if eventName != "gitpanel:prs_action_result" {
			return
		}
		event, ok := payload.(PRActionResultTelemetry)
		if !ok {
			t.Fatalf("unexpected action telemetry payload type: %T", payload)
		}
		actionEvents = append(actionEvents, event)
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("unexpected method: got=%s want=%s", req.Method, http.MethodPost)
			}
			if req.URL.Path != "/repos/orch-labs/orch/pulls" {
				t.Fatalf("unexpected endpoint path: %q", req.URL.Path)
			}

			headers := make(http.Header)
			headers.Set("X-RateLimit-Remaining", "4984")

			body := `{
				"node_id":"PR_kwDOAAX",
				"number":77,
				"title":"Telemetry action",
				"state":"open",
				"created_at":"2026-02-24T10:00:00Z",
				"updated_at":"2026-02-24T10:00:00Z",
				"user":{"login":"lead","avatar_url":""},
				"head":{"ref":"feature/telemetry"},
				"base":{"ref":"main"}
			}`

			return &http.Response{
				StatusCode: http.StatusCreated,
				Header:     headers,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	created, err := service.CreatePullRequest(CreatePRInput{
		Owner:      "orch-labs",
		Repo:       "orch",
		Title:      "Telemetry action",
		HeadBranch: "feature/telemetry",
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest() error: %v", err)
	}
	if created == nil || created.Number != 77 {
		t.Fatalf("unexpected created PR payload: %#v", created)
	}

	if len(actionEvents) != 1 {
		t.Fatalf("expected 1 action telemetry event, got=%d", len(actionEvents))
	}

	event := actionEvents[0]
	if event.Action != prActionCreate {
		t.Fatalf("unexpected action telemetry action: %q", event.Action)
	}
	if event.Method != http.MethodPost {
		t.Fatalf("unexpected action telemetry method: %q", event.Method)
	}
	if event.Endpoint != "/repos/orch-labs/orch/pulls" {
		t.Fatalf("unexpected action telemetry endpoint: %q", event.Endpoint)
	}
	if event.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected action telemetry status: %d", event.StatusCode)
	}
	if !event.Success {
		t.Fatalf("expected successful action telemetry event")
	}
	if event.RateRemaining != 4984 {
		t.Fatalf("unexpected rate remaining on action telemetry: %d", event.RateRemaining)
	}
	if event.ErrorType != "" {
		t.Fatalf("unexpected error type on success action telemetry: %q", event.ErrorType)
	}
	if event.DurationMs < 0 {
		t.Fatalf("duration should be non-negative, got=%d", event.DurationMs)
	}
}

func TestMergePullRequestRESTEmitsActionResultTelemetryOnConflict(t *testing.T) {
	actionEvents := make([]PRActionResultTelemetry, 0, 1)

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.SetTelemetryEmitter(func(eventName string, payload interface{}) {
		if eventName != "gitpanel:prs_action_result" {
			return
		}
		event, ok := payload.(PRActionResultTelemetry)
		if !ok {
			t.Fatalf("unexpected action telemetry payload type: %T", payload)
		}
		actionEvents = append(actionEvents, event)
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPut {
				t.Fatalf("unexpected method: got=%s want=%s", req.Method, http.MethodPut)
			}
			if req.URL.Path != "/repos/orch-labs/orch/pulls/42/merge" {
				t.Fatalf("unexpected endpoint path: %q", req.URL.Path)
			}

			headers := make(http.Header)
			headers.Set("X-RateLimit-Remaining", "4977")

			return &http.Response{
				StatusCode: http.StatusConflict,
				Header:     headers,
				Body:       io.NopCloser(strings.NewReader(`{"message":"merge conflict"}`)),
			}, nil
		}),
	}

	result, err := service.MergePullRequestREST(MergePRInput{
		Owner:       "orch-labs",
		Repo:        "orch",
		Number:      42,
		MergeMethod: PRMergeMethodMerge,
	})
	if err == nil {
		t.Fatalf("expected merge conflict error")
	}
	if result != nil {
		t.Fatalf("expected nil merge result on conflict, got=%#v", result)
	}

	if len(actionEvents) != 1 {
		t.Fatalf("expected 1 action telemetry event, got=%d", len(actionEvents))
	}

	event := actionEvents[0]
	if event.Action != prActionMerge {
		t.Fatalf("unexpected action telemetry action: %q", event.Action)
	}
	if event.Method != http.MethodPut {
		t.Fatalf("unexpected action telemetry method: %q", event.Method)
	}
	if event.Endpoint != "/repos/orch-labs/orch/pulls/42/merge" {
		t.Fatalf("unexpected action telemetry endpoint: %q", event.Endpoint)
	}
	if event.StatusCode != http.StatusConflict {
		t.Fatalf("unexpected action telemetry status: %d", event.StatusCode)
	}
	if event.Success {
		t.Fatalf("expected failed action telemetry event")
	}
	if event.ErrorType != "conflict" {
		t.Fatalf("unexpected action telemetry error type: %q", event.ErrorType)
	}
	if event.RateRemaining != 4977 {
		t.Fatalf("unexpected rate remaining on action telemetry: %d", event.RateRemaining)
	}
	if event.DurationMs < 0 {
		t.Fatalf("duration should be non-negative, got=%d", event.DurationMs)
	}
}
