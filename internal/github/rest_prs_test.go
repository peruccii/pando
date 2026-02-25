package github

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestListPullRequestsRESTRetriesReadOnTransientServerError(t *testing.T) {
	requestCount := 0

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.retrySleep = func(time.Duration) {}
	service.retryRand = func() float64 { return 0 }
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++

			if requestCount < 3 {
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"message":"Service temporarily unavailable"}`)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`[
					{
						"node_id":"PR_retry_ok",
						"number":77,
						"title":"PR after retry",
						"state":"open",
						"created_at":"2026-02-24T10:00:00Z",
						"updated_at":"2026-02-24T10:00:00Z",
						"user":{"login":"dev","avatar_url":""},
						"head":{"ref":"feature/retry"},
						"base":{"ref":"main"}
					}
				]`)),
			}, nil
		}),
	}

	prs, err := service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("ListPullRequests() error: %v", err)
	}
	if requestCount != 3 {
		t.Fatalf("expected retry flow with 3 requests, got=%d", requestCount)
	}
	if len(prs) != 1 || prs[0].Number != 77 {
		t.Fatalf("unexpected PR payload after retries: %+v", prs)
	}
}

func TestListPullRequestsRESTRetriesOnSecondaryRateLimit(t *testing.T) {
	requestCount := 0

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.retrySleep = func(time.Duration) {}
	service.retryRand = func() float64 { return 0 }
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++

			if requestCount == 1 {
				headers := make(http.Header)
				headers.Set("Retry-After", "1")
				return &http.Response{
					StatusCode: http.StatusForbidden,
					Header:     headers,
					Body: io.NopCloser(strings.NewReader(`{
						"message":"You have exceeded a secondary rate limit. Please wait a few minutes before you try again."
					}`)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(`[
					{
						"node_id":"PR_secondary_retry_ok",
						"number":88,
						"title":"PR after secondary limit",
						"state":"open",
						"created_at":"2026-02-24T10:00:00Z",
						"updated_at":"2026-02-24T10:00:00Z",
						"user":{"login":"dev","avatar_url":""},
						"head":{"ref":"feature/secondary-retry"},
						"base":{"ref":"main"}
					}
				]`)),
			}, nil
		}),
	}

	prs, err := service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("ListPullRequests() error: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected one retry for secondary limit, got=%d requests", requestCount)
	}
	if len(prs) != 1 || prs[0].Number != 88 {
		t.Fatalf("unexpected PR payload after secondary retry: %+v", prs)
	}
}

func TestListPullRequestsRESTDoesNotRetryOnPermissionForbidden(t *testing.T) {
	requestCount := 0

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.retrySleep = func(time.Duration) {}
	service.retryRand = func() float64 { return 0 }
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"message":"Resource not accessible by integration"}`)),
			}, nil
		}),
	}

	_, err := service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 1, PerPage: 25})
	if err == nil {
		t.Fatalf("expected permission error")
	}
	if requestCount != 1 {
		t.Fatalf("expected no retry for non-rate-limit 403, got=%d requests", requestCount)
	}

	ghErr, ok := err.(*GitHubError)
	if !ok {
		t.Fatalf("expected GitHubError, got=%T", err)
	}
	if ghErr.Type != "permission" {
		t.Fatalf("expected permission error type, got=%s", ghErr.Type)
	}
}

func TestCreatePullRequestRESTDoesNotRetryOnServerError(t *testing.T) {
	requestCount := 0

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.retrySleep = func(time.Duration) {}
	service.retryRand = func() float64 { return 0 }
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"message":"Service unavailable"}`)),
			}, nil
		}),
	}

	_, err := service.CreatePullRequest(CreatePRInput{
		Owner:      "orch-labs",
		Repo:       "orch",
		Title:      "Create without blind retry",
		HeadBranch: "feature/no-retry",
		BaseBranch: "main",
	})
	if err == nil {
		t.Fatalf("expected create failure on 503")
	}
	if requestCount != 1 {
		t.Fatalf("expected no retry for mutation, got=%d requests", requestCount)
	}

	ghErr, ok := err.(*GitHubError)
	if !ok {
		t.Fatalf("expected GitHubError, got=%T", err)
	}
	if ghErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got=%d", ghErr.StatusCode)
	}
}

func TestListPullRequestsRESTUsesOfficialHeadersAndPagination(t *testing.T) {
	var called bool

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			called = true
			if req.Method != http.MethodGet {
				t.Fatalf("unexpected method: got=%s want=%s", req.Method, http.MethodGet)
			}
			if req.URL.Path != "/repos/orch-labs/orch/pulls" {
				t.Fatalf("unexpected path: got=%s", req.URL.Path)
			}
			if req.URL.Query().Get("state") != "open" {
				t.Fatalf("unexpected state query: got=%s", req.URL.Query().Get("state"))
			}
			if req.URL.Query().Get("page") != "2" {
				t.Fatalf("unexpected page query: got=%s", req.URL.Query().Get("page"))
			}
			if req.URL.Query().Get("per_page") != "50" {
				t.Fatalf("unexpected per_page query: got=%s", req.URL.Query().Get("per_page"))
			}
			if req.Header.Get("Authorization") != "Bearer gh-token" {
				t.Fatalf("unexpected authorization header: %s", req.Header.Get("Authorization"))
			}
			if req.Header.Get("Accept") != githubRESTAcceptJSON {
				t.Fatalf("unexpected accept header: %s", req.Header.Get("Accept"))
			}
			if req.Header.Get("X-GitHub-Api-Version") != githubRESTAPIVersion {
				t.Fatalf("unexpected api version header: %s", req.Header.Get("X-GitHub-Api-Version"))
			}

			body := `[{
					"node_id": "PR_kwDOAA",
					"number": 42,
					"title": "Add REST endpoint",
					"body": "body",
					"state": "open",
					"draft": false,
					"maintainer_can_modify": true,
					"created_at": "2026-02-20T10:00:00Z",
					"updated_at": "2026-02-21T11:00:00Z",
					"merge_commit_sha": null,
					"additions": 10,
				"deletions": 3,
				"changed_files": 2,
				"user": {"login": "perucci", "avatar_url": "https://avatar"},
				"requested_reviewers": [{"login": "orch-reviewer", "avatar_url": "https://avatar/r"}],
				"labels": [{"name": "backend", "color": "ededed"}],
				"head": {"ref": "feature/rest"},
				"base": {"ref": "main"}
			}]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	prs, err := service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    2,
		PerPage: 50,
	})
	if err != nil {
		t.Fatalf("ListPullRequests() error: %v", err)
	}
	if !called {
		t.Fatalf("expected rest client to be called")
	}
	if len(prs) != 1 {
		t.Fatalf("unexpected PR size: got=%d want=1", len(prs))
	}
	if prs[0].Number != 42 {
		t.Fatalf("unexpected PR number: got=%d want=42", prs[0].Number)
	}
	if prs[0].State != "OPEN" {
		t.Fatalf("unexpected PR state: got=%s want=OPEN", prs[0].State)
	}
	if prs[0].HeadBranch != "feature/rest" || prs[0].BaseBranch != "main" {
		t.Fatalf("unexpected branch refs: head=%s base=%s", prs[0].HeadBranch, prs[0].BaseBranch)
	}
	if prs[0].MaintainerCanModify == nil || *prs[0].MaintainerCanModify != true {
		t.Fatalf("unexpected maintainer can modify value: %v", prs[0].MaintainerCanModify)
	}
}

func TestListPullRequestsRESTCachesByEndpointAndParams(t *testing.T) {
	requestCount := 0

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++

			state := req.URL.Query().Get("state")
			page := req.URL.Query().Get("page")
			perPage := req.URL.Query().Get("per_page")
			if perPage != "25" {
				t.Fatalf("unexpected per_page query: got=%s want=25", perPage)
			}

			body := `[{
				"node_id":"PR_open",
				"number":101,
				"title":"Open PR",
				"state":"open",
				"created_at":"2026-02-20T10:00:00Z",
				"updated_at":"2026-02-20T10:01:00Z",
				"user":{"login":"dev","avatar_url":""},
				"head":{"ref":"feature/open"},
				"base":{"ref":"main"}
			}]`
			if state == "closed" {
				body = `[{
					"node_id":"PR_closed",
					"number":202,
					"title":"Closed PR",
					"state":"closed",
					"created_at":"2026-02-20T10:00:00Z",
					"updated_at":"2026-02-20T10:01:00Z",
					"merged_at":"2026-02-20T10:02:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/closed"},
					"base":{"ref":"main"}
				}]`
			}
			if page == "2" {
				body = `[{
					"node_id":"PR_page_2",
					"number":303,
					"title":"Open PR page 2",
					"state":"open",
					"created_at":"2026-02-20T10:00:00Z",
					"updated_at":"2026-02-20T10:01:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/open-2"},
					"base":{"ref":"main"}
				}]`
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	_, err := service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("first open list error: %v", err)
	}
	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("second open list error: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected cache hit for repeated open page=1 list, requests=%d", requestCount)
	}

	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{State: "closed", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("closed list error: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected separate cache key for closed list, requests=%d", requestCount)
	}

	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 2, PerPage: 25})
	if err != nil {
		t.Fatalf("open page=2 list error: %v", err)
	}
	if requestCount != 3 {
		t.Fatalf("expected separate cache key for open page=2, requests=%d", requestCount)
	}

	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{State: "MERGED", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("merged list error: %v", err)
	}
	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{State: "MERGED", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("merged list cache hit error: %v", err)
	}
	if requestCount != 4 {
		t.Fatalf("expected merged query to cache independently, requests=%d", requestCount)
	}
}

func TestListPullRequestsRESTMergedFilter(t *testing.T) {
	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Query().Get("state") != "closed" {
				t.Fatalf("expected state=closed for merged filter, got=%s", req.URL.Query().Get("state"))
			}

			body := `[
				{
					"node_id": "PR_closed",
					"number": 10,
					"title": "Closed no merge",
					"state": "closed",
					"created_at": "2026-02-20T10:00:00Z",
					"updated_at": "2026-02-20T10:01:00Z",
					"merged_at": null,
					"user": {"login": "dev", "avatar_url": ""},
					"head": {"ref": "feature/a"},
					"base": {"ref": "main"}
				},
				{
					"node_id": "PR_merged",
					"number": 11,
					"title": "Merged PR",
					"state": "closed",
					"created_at": "2026-02-20T10:00:00Z",
					"updated_at": "2026-02-20T10:01:00Z",
					"merged_at": "2026-02-20T10:02:00Z",
					"user": {"login": "dev", "avatar_url": ""},
					"head": {"ref": "feature/b"},
					"base": {"ref": "main"}
				}
			]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	prs, err := service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "MERGED",
		Page:    1,
		PerPage: 20,
	})
	if err != nil {
		t.Fatalf("ListPullRequests() error: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected only merged PRs, got=%d", len(prs))
	}
	if prs[0].Number != 11 || prs[0].State != "MERGED" {
		t.Fatalf("unexpected merged PR: number=%d state=%s", prs[0].Number, prs[0].State)
	}
}

func TestGetPullRequestRESTUsesOfficialHeaders(t *testing.T) {
	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodGet {
				t.Fatalf("unexpected method: got=%s", req.Method)
			}
			if req.URL.Path != "/repos/orch-labs/orch/pulls/7" {
				t.Fatalf("unexpected path: got=%s", req.URL.Path)
			}
			if req.Header.Get("Accept") != githubRESTAcceptJSON {
				t.Fatalf("unexpected accept header: %s", req.Header.Get("Accept"))
			}
			if req.Header.Get("X-GitHub-Api-Version") != githubRESTAPIVersion {
				t.Fatalf("unexpected api version header: %s", req.Header.Get("X-GitHub-Api-Version"))
			}
			if req.Header.Get("Authorization") != "Bearer gh-token" {
				t.Fatalf("unexpected authorization header: %s", req.Header.Get("Authorization"))
			}

			body := `{
					"node_id": "PR_node_id",
					"number": 7,
					"title": "PR Detail",
					"body": "detail body",
					"state": "closed",
					"draft": false,
					"maintainer_can_modify": false,
					"created_at": "2026-02-20T10:00:00Z",
					"updated_at": "2026-02-21T10:00:00Z",
					"merged_at": "2026-02-22T10:00:00Z",
					"merge_commit_sha": "abc123",
				"additions": 55,
				"deletions": 13,
				"changed_files": 8,
				"user": {"login": "author", "avatar_url": "https://avatar"},
				"requested_reviewers": [{"login": "reviewer-1", "avatar_url": "https://avatar/r1"}],
				"labels": [{"name": "enhancement", "color": "aabbcc"}],
				"head": {"ref": "feature/pr"},
				"base": {"ref": "main"}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	pr, err := service.GetPullRequest("orch-labs", "orch", 7)
	if err != nil {
		t.Fatalf("GetPullRequest() error: %v", err)
	}
	if pr == nil {
		t.Fatalf("expected PR detail")
	}
	if pr.State != "MERGED" {
		t.Fatalf("expected merged state, got=%s", pr.State)
	}
	if pr.MergeCommit == nil || *pr.MergeCommit != "abc123" {
		t.Fatalf("unexpected merge commit: %v", pr.MergeCommit)
	}
	if pr.ID != "PR_node_id" {
		t.Fatalf("unexpected node id: got=%s", pr.ID)
	}
	if pr.MaintainerCanModify == nil || *pr.MaintainerCanModify != false {
		t.Fatalf("unexpected maintainer can modify value: %v", pr.MaintainerCanModify)
	}
}

func TestCreatePullRequestRESTUsesOfficialHeadersAndPayload(t *testing.T) {
	canModify := true
	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("unexpected method: got=%s want=%s", req.Method, http.MethodPost)
			}
			if req.URL.Path != "/repos/orch-labs/orch/pulls" {
				t.Fatalf("unexpected path: got=%s", req.URL.Path)
			}
			if req.Header.Get("Authorization") != "Bearer gh-token" {
				t.Fatalf("unexpected authorization header: %s", req.Header.Get("Authorization"))
			}
			if req.Header.Get("Accept") != githubRESTAcceptJSON {
				t.Fatalf("unexpected accept header: %s", req.Header.Get("Accept"))
			}
			if req.Header.Get("X-GitHub-Api-Version") != githubRESTAPIVersion {
				t.Fatalf("unexpected api version header: %s", req.Header.Get("X-GitHub-Api-Version"))
			}
			if req.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected content type: %s", req.Header.Get("Content-Type"))
			}

			rawBody, readErr := io.ReadAll(req.Body)
			if readErr != nil {
				t.Fatalf("failed to read request body: %v", readErr)
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(rawBody, &payload); err != nil {
				t.Fatalf("failed to parse payload: %v", err)
			}
			if payload["title"] != "Create via REST" {
				t.Fatalf("unexpected title payload: %v", payload["title"])
			}
			if payload["head"] != "feature/rest-create" {
				t.Fatalf("unexpected head payload: %v", payload["head"])
			}
			if payload["base"] != "main" {
				t.Fatalf("unexpected base payload: %v", payload["base"])
			}
			if payload["body"] != "payload body" {
				t.Fatalf("unexpected body payload: %v", payload["body"])
			}
			if payload["draft"] != true {
				t.Fatalf("unexpected draft payload: %v", payload["draft"])
			}
			if payload["maintainer_can_modify"] != true {
				t.Fatalf("unexpected maintainer_can_modify payload: %v", payload["maintainer_can_modify"])
			}

			body := `{
				"node_id": "PR_new",
				"number": 55,
				"title": "Create via REST",
				"body": "payload body",
				"state": "open",
				"draft": true,
				"created_at": "2026-02-24T10:00:00Z",
				"updated_at": "2026-02-24T10:00:00Z",
				"user": {"login": "perucci", "avatar_url": "https://avatar"},
				"head": {"ref": "feature/rest-create"},
				"base": {"ref": "main"}
			}`
			return &http.Response{
				StatusCode: http.StatusCreated,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	pr, err := service.CreatePullRequest(CreatePRInput{
		Owner:               "orch-labs",
		Repo:                "orch",
		Title:               "Create via REST",
		Body:                "payload body",
		HeadBranch:          "feature/rest-create",
		BaseBranch:          "main",
		IsDraft:             true,
		MaintainerCanModify: &canModify,
	})
	if err != nil {
		t.Fatalf("CreatePullRequest() error: %v", err)
	}
	if pr == nil {
		t.Fatalf("expected created pull request")
	}
	if pr.Number != 55 {
		t.Fatalf("unexpected PR number: got=%d want=55", pr.Number)
	}
	if pr.State != "OPEN" {
		t.Fatalf("unexpected PR state: got=%s want=OPEN", pr.State)
	}
	if !pr.IsDraft {
		t.Fatalf("expected draft=true on created PR")
	}
}

func TestCreatePullRequestRESTInvalidatesRepositoryCache(t *testing.T) {
	var getCalls int
	var postCalls int

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.Method {
			case http.MethodGet:
				getCalls++
				if req.URL.Path != "/repos/orch-labs/orch/pulls" {
					t.Fatalf("unexpected GET path: got=%s", req.URL.Path)
				}

				body := `[{
					"node_id":"PR_cached",
					"number":101,
					"title":"Cached list",
					"state":"open",
					"created_at":"2026-02-20T10:00:00Z",
					"updated_at":"2026-02-20T10:01:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/cached"},
					"base":{"ref":"main"}
				}]`
				if getCalls == 2 {
					body = `[{
						"node_id":"PR_refetched",
						"number":202,
						"title":"Refetched list",
						"state":"open",
						"created_at":"2026-02-20T10:00:00Z",
						"updated_at":"2026-02-20T10:01:00Z",
						"user":{"login":"dev","avatar_url":""},
						"head":{"ref":"feature/refetched"},
						"base":{"ref":"main"}
					}]`
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			case http.MethodPost:
				postCalls++
				if req.URL.Path != "/repos/orch-labs/orch/pulls" {
					t.Fatalf("unexpected POST path: got=%s", req.URL.Path)
				}
				body := `{
					"node_id":"PR_created",
					"number":303,
					"title":"Created PR",
					"state":"open",
					"created_at":"2026-02-24T10:00:00Z",
					"updated_at":"2026-02-24T10:00:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/new-pr"},
					"base":{"ref":"main"}
				}`
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			default:
				t.Fatalf("unexpected method: %s", req.Method)
				return nil, nil
			}
		}),
	}

	_, err := service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("first ListPullRequests() error: %v", err)
	}
	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("second ListPullRequests() error: %v", err)
	}
	if getCalls != 1 {
		t.Fatalf("expected list cache hit before mutation, getCalls=%d", getCalls)
	}

	_, err = service.CreatePullRequest(CreatePRInput{
		Owner:      "orch-labs",
		Repo:       "orch",
		Title:      "Created PR",
		HeadBranch: "feature/new-pr",
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest() error: %v", err)
	}
	if postCalls != 1 {
		t.Fatalf("expected one create request, postCalls=%d", postCalls)
	}

	listAfterCreate, err := service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("third ListPullRequests() error: %v", err)
	}
	if getCalls != 2 {
		t.Fatalf("expected list refetch after mutation invalidation, getCalls=%d", getCalls)
	}
	if len(listAfterCreate) != 1 || listAfterCreate[0].Number != 202 {
		t.Fatalf("unexpected list payload after invalidation: %+v", listAfterCreate)
	}
}

func TestCreatePullRequestRESTInvalidatesOnlyPRListCache(t *testing.T) {
	var listCalls int
	var detailCalls int
	var postCalls int

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodGet && req.URL.Path == "/repos/orch-labs/orch/pulls":
				listCalls++
				body := `[{
					"node_id":"PR_cached_list",
					"number":11,
					"title":"Cached list",
					"state":"open",
					"created_at":"2026-02-20T10:00:00Z",
					"updated_at":"2026-02-20T10:01:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/cached-list"},
					"base":{"ref":"main"}
				}]`
				if listCalls == 2 {
					body = `[{
						"node_id":"PR_refetched_list",
						"number":22,
						"title":"Refetched list",
						"state":"open",
						"created_at":"2026-02-20T10:00:00Z",
						"updated_at":"2026-02-20T10:02:00Z",
						"user":{"login":"dev","avatar_url":""},
						"head":{"ref":"feature/refetched-list"},
						"base":{"ref":"main"}
					}]`
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			case req.Method == http.MethodGet && req.URL.Path == "/repos/orch-labs/orch/pulls/900":
				detailCalls++
				body := `{
					"node_id":"PR_cached_detail",
					"number":900,
					"title":"Cached detail",
					"state":"open",
					"created_at":"2026-02-20T10:00:00Z",
					"updated_at":"2026-02-20T10:01:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/cached-detail"},
					"base":{"ref":"main"}
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			case req.Method == http.MethodPost && req.URL.Path == "/repos/orch-labs/orch/pulls":
				postCalls++
				body := `{
					"node_id":"PR_created",
					"number":303,
					"title":"Created PR",
					"state":"open",
					"created_at":"2026-02-24T10:00:00Z",
					"updated_at":"2026-02-24T10:00:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/new-pr"},
					"base":{"ref":"main"}
				}`
				return &http.Response{
					StatusCode: http.StatusCreated,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			default:
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
				return nil, nil
			}
		}),
	}

	_, err := service.GetPullRequest("orch-labs", "orch", 900)
	if err != nil {
		t.Fatalf("first GetPullRequest() error: %v", err)
	}
	_, err = service.GetPullRequest("orch-labs", "orch", 900)
	if err != nil {
		t.Fatalf("second GetPullRequest() error: %v", err)
	}
	if detailCalls != 1 {
		t.Fatalf("expected PR detail cache hit before mutation, detailCalls=%d", detailCalls)
	}

	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("first ListPullRequests() error: %v", err)
	}
	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("second ListPullRequests() error: %v", err)
	}
	if listCalls != 1 {
		t.Fatalf("expected PR list cache hit before mutation, listCalls=%d", listCalls)
	}

	_, err = service.CreatePullRequest(CreatePRInput{
		Owner:      "orch-labs",
		Repo:       "orch",
		Title:      "Created PR",
		HeadBranch: "feature/new-pr",
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreatePullRequest() error: %v", err)
	}
	if postCalls != 1 {
		t.Fatalf("expected one create call, postCalls=%d", postCalls)
	}

	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("third ListPullRequests() error: %v", err)
	}
	if listCalls != 2 {
		t.Fatalf("expected list cache invalidated after create, listCalls=%d", listCalls)
	}

	_, err = service.GetPullRequest("orch-labs", "orch", 900)
	if err != nil {
		t.Fatalf("third GetPullRequest() error: %v", err)
	}
	if detailCalls != 1 {
		t.Fatalf("expected unrelated PR detail cache preserved after create, detailCalls=%d", detailCalls)
	}
}

func TestUpdatePullRequestRESTUsesOfficialHeadersAndPayload(t *testing.T) {
	title := "Update via REST"
	body := ""
	state := "CLOSED"
	base := "release/v1"
	canModify := false

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPatch {
				t.Fatalf("unexpected method: got=%s want=%s", req.Method, http.MethodPatch)
			}
			if req.URL.Path != "/repos/orch-labs/orch/pulls/55" {
				t.Fatalf("unexpected path: got=%s", req.URL.Path)
			}
			if req.Header.Get("Authorization") != "Bearer gh-token" {
				t.Fatalf("unexpected authorization header: %s", req.Header.Get("Authorization"))
			}
			if req.Header.Get("Accept") != githubRESTAcceptJSON {
				t.Fatalf("unexpected accept header: %s", req.Header.Get("Accept"))
			}
			if req.Header.Get("X-GitHub-Api-Version") != githubRESTAPIVersion {
				t.Fatalf("unexpected api version header: %s", req.Header.Get("X-GitHub-Api-Version"))
			}
			if req.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected content type: %s", req.Header.Get("Content-Type"))
			}

			rawBody, readErr := io.ReadAll(req.Body)
			if readErr != nil {
				t.Fatalf("failed to read request body: %v", readErr)
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(rawBody, &payload); err != nil {
				t.Fatalf("failed to parse payload: %v", err)
			}
			if payload["title"] != "Update via REST" {
				t.Fatalf("unexpected title payload: %v", payload["title"])
			}
			if payload["body"] != "" {
				t.Fatalf("unexpected body payload: %v", payload["body"])
			}
			if payload["state"] != "closed" {
				t.Fatalf("unexpected state payload: %v", payload["state"])
			}
			if payload["base"] != "release/v1" {
				t.Fatalf("unexpected base payload: %v", payload["base"])
			}
			if payload["maintainer_can_modify"] != false {
				t.Fatalf("unexpected maintainer_can_modify payload: %v", payload["maintainer_can_modify"])
			}

			body := `{
				"node_id": "PR_updated",
				"number": 55,
				"title": "Update via REST",
				"body": "",
				"state": "closed",
				"draft": false,
				"created_at": "2026-02-24T10:00:00Z",
				"updated_at": "2026-02-24T10:05:00Z",
				"user": {"login": "perucci", "avatar_url": "https://avatar"},
				"head": {"ref": "feature/rest-create"},
				"base": {"ref": "release/v1"}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	pr, err := service.UpdatePullRequest(UpdatePRInput{
		Owner:               "orch-labs",
		Repo:                "orch",
		Number:              55,
		Title:               &title,
		Body:                &body,
		State:               &state,
		BaseBranch:          &base,
		MaintainerCanModify: &canModify,
	})
	if err != nil {
		t.Fatalf("UpdatePullRequest() error: %v", err)
	}
	if pr == nil {
		t.Fatalf("expected updated pull request")
	}
	if pr.Number != 55 {
		t.Fatalf("unexpected PR number: got=%d want=55", pr.Number)
	}
	if pr.State != "CLOSED" {
		t.Fatalf("unexpected PR state: got=%s want=CLOSED", pr.State)
	}
	if pr.BaseBranch != "release/v1" {
		t.Fatalf("unexpected base branch: got=%s want=release/v1", pr.BaseBranch)
	}
}

func TestUpdatePullRequestRESTInvalidatesRepositoryCache(t *testing.T) {
	var getCalls int
	var patchCalls int

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.Method {
			case http.MethodGet:
				getCalls++
				if req.URL.Path != "/repos/orch-labs/orch/pulls" {
					t.Fatalf("unexpected GET path: got=%s", req.URL.Path)
				}

				body := `[{
					"node_id":"PR_cached",
					"number":77,
					"title":"Cached list",
					"state":"open",
					"created_at":"2026-02-20T10:00:00Z",
					"updated_at":"2026-02-20T10:01:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/cached"},
					"base":{"ref":"main"}
				}]`
				if getCalls == 2 {
					body = `[{
						"node_id":"PR_refetched",
						"number":88,
						"title":"Refetched list",
						"state":"open",
						"created_at":"2026-02-20T10:00:00Z",
						"updated_at":"2026-02-20T10:01:00Z",
						"user":{"login":"dev","avatar_url":""},
						"head":{"ref":"feature/refetched"},
						"base":{"ref":"main"}
					}]`
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			case http.MethodPatch:
				patchCalls++
				if req.URL.Path != "/repos/orch-labs/orch/pulls/77" {
					t.Fatalf("unexpected PATCH path: got=%s", req.URL.Path)
				}
				body := `{
					"node_id":"PR_updated",
					"number":77,
					"title":"Updated PR",
					"state":"open",
					"created_at":"2026-02-24T10:00:00Z",
					"updated_at":"2026-02-24T10:00:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/cached"},
					"base":{"ref":"main"}
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			default:
				t.Fatalf("unexpected method: %s", req.Method)
				return nil, nil
			}
		}),
	}

	_, err := service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("first ListPullRequests() error: %v", err)
	}
	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("second ListPullRequests() error: %v", err)
	}
	if getCalls != 1 {
		t.Fatalf("expected list cache hit before update, getCalls=%d", getCalls)
	}

	title := "Updated PR"
	_, err = service.UpdatePullRequest(UpdatePRInput{
		Owner:  "orch-labs",
		Repo:   "orch",
		Number: 77,
		Title:  &title,
	})
	if err != nil {
		t.Fatalf("UpdatePullRequest() error: %v", err)
	}
	if patchCalls != 1 {
		t.Fatalf("expected one update request, patchCalls=%d", patchCalls)
	}

	listAfterUpdate, err := service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("third ListPullRequests() error: %v", err)
	}
	if getCalls != 2 {
		t.Fatalf("expected list refetch after mutation invalidation, getCalls=%d", getCalls)
	}
	if len(listAfterUpdate) != 1 || listAfterUpdate[0].Number != 88 {
		t.Fatalf("unexpected list payload after invalidation: %+v", listAfterUpdate)
	}
}

func TestUpdatePullRequestRESTRefreshesMutatedDetailAndKeepsUnrelatedDetailCache(t *testing.T) {
	detailCallsByPath := map[string]int{}
	var patchCalls int

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodGet && strings.HasPrefix(req.URL.Path, "/repos/orch-labs/orch/pulls/"):
				detailCallsByPath[req.URL.Path]++
				prNumber := strings.TrimPrefix(req.URL.Path, "/repos/orch-labs/orch/pulls/")
				body := `{
					"node_id":"PR_` + prNumber + `",
					"number":` + prNumber + `,
					"title":"PR ` + prNumber + `",
					"state":"open",
					"created_at":"2026-02-20T10:00:00Z",
					"updated_at":"2026-02-20T10:01:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/pr-` + prNumber + `"},
					"base":{"ref":"main"}
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			case req.Method == http.MethodPatch && req.URL.Path == "/repos/orch-labs/orch/pulls/77":
				patchCalls++
				body := `{
					"node_id":"PR_77_updated",
					"number":77,
					"title":"Updated PR 77",
					"state":"open",
					"created_at":"2026-02-24T10:00:00Z",
					"updated_at":"2026-02-24T10:05:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/pr-77"},
					"base":{"ref":"main"}
				}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			case req.Method == http.MethodGet && req.URL.Path == "/repos/orch-labs/orch/pulls":
				body := `[{
					"node_id":"PR_cached_list",
					"number":1,
					"title":"cached list",
					"state":"open",
					"created_at":"2026-02-20T10:00:00Z",
					"updated_at":"2026-02-20T10:01:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/list"},
					"base":{"ref":"main"}
				}]`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			default:
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
				return nil, nil
			}
		}),
	}

	_, err := service.GetPullRequest("orch-labs", "orch", 77)
	if err != nil {
		t.Fatalf("warm cache PR 77 error: %v", err)
	}
	_, err = service.GetPullRequest("orch-labs", "orch", 99)
	if err != nil {
		t.Fatalf("warm cache PR 99 error: %v", err)
	}
	_, err = service.GetPullRequest("orch-labs", "orch", 77)
	if err != nil {
		t.Fatalf("cache hit PR 77 error: %v", err)
	}
	_, err = service.GetPullRequest("orch-labs", "orch", 99)
	if err != nil {
		t.Fatalf("cache hit PR 99 error: %v", err)
	}
	if detailCallsByPath["/repos/orch-labs/orch/pulls/77"] != 1 {
		t.Fatalf("expected PR 77 cached before update, calls=%d", detailCallsByPath["/repos/orch-labs/orch/pulls/77"])
	}
	if detailCallsByPath["/repos/orch-labs/orch/pulls/99"] != 1 {
		t.Fatalf("expected PR 99 cached before update, calls=%d", detailCallsByPath["/repos/orch-labs/orch/pulls/99"])
	}

	title := "Updated PR 77"
	_, err = service.UpdatePullRequest(UpdatePRInput{
		Owner:  "orch-labs",
		Repo:   "orch",
		Number: 77,
		Title:  &title,
	})
	if err != nil {
		t.Fatalf("UpdatePullRequest() error: %v", err)
	}
	if patchCalls != 1 {
		t.Fatalf("expected one update call, patchCalls=%d", patchCalls)
	}

	updatedPR, err := service.GetPullRequest("orch-labs", "orch", 77)
	if err != nil {
		t.Fatalf("refresh updated PR 77 error: %v", err)
	}
	if updatedPR == nil || updatedPR.Title != "Updated PR 77" {
		t.Fatalf("expected updated PR detail cached after mutation, got=%+v", updatedPR)
	}
	if detailCallsByPath["/repos/orch-labs/orch/pulls/77"] != 1 {
		t.Fatalf("expected updated PR detail served from refreshed cache, calls=%d", detailCallsByPath["/repos/orch-labs/orch/pulls/77"])
	}

	_, err = service.GetPullRequest("orch-labs", "orch", 99)
	if err != nil {
		t.Fatalf("read unrelated PR 99 error: %v", err)
	}
	if detailCallsByPath["/repos/orch-labs/orch/pulls/99"] != 1 {
		t.Fatalf("expected unrelated PR detail cache preserved, calls=%d", detailCallsByPath["/repos/orch-labs/orch/pulls/99"])
	}
}

func TestUpdatePullRequestRESTRejectsEmptyPayload(t *testing.T) {
	service := NewService(func() (string, error) {
		return "gh-token", nil
	})

	_, err := service.UpdatePullRequest(UpdatePRInput{
		Owner:  "orch-labs",
		Repo:   "orch",
		Number: 22,
	})
	if err == nil {
		t.Fatalf("expected validation error for empty update payload")
	}

	ghErr, ok := err.(*GitHubError)
	if !ok {
		t.Fatalf("expected GitHubError, got=%T", err)
	}
	if ghErr.StatusCode != 422 {
		t.Fatalf("unexpected status code: got=%d want=422", ghErr.StatusCode)
	}
	if ghErr.Type != "validation" {
		t.Fatalf("unexpected error type: got=%s want=validation", ghErr.Type)
	}
}

func TestMergePullRequestRESTUsesOfficialHeadersAndPayload(t *testing.T) {
	sha := "9d145a1f1ce6aee16de4f9517d2f41295bb12f34"

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPut {
				t.Fatalf("unexpected method: got=%s want=%s", req.Method, http.MethodPut)
			}
			if req.URL.Path != "/repos/orch-labs/orch/pulls/55/merge" {
				t.Fatalf("unexpected path: got=%s", req.URL.Path)
			}
			if req.Header.Get("Authorization") != "Bearer gh-token" {
				t.Fatalf("unexpected authorization header: %s", req.Header.Get("Authorization"))
			}
			if req.Header.Get("Accept") != githubRESTAcceptJSON {
				t.Fatalf("unexpected accept header: %s", req.Header.Get("Accept"))
			}
			if req.Header.Get("X-GitHub-Api-Version") != githubRESTAPIVersion {
				t.Fatalf("unexpected api version header: %s", req.Header.Get("X-GitHub-Api-Version"))
			}
			if req.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected content type: %s", req.Header.Get("Content-Type"))
			}

			rawBody, readErr := io.ReadAll(req.Body)
			if readErr != nil {
				t.Fatalf("failed to read request body: %v", readErr)
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(rawBody, &payload); err != nil {
				t.Fatalf("failed to parse payload: %v", err)
			}
			if payload["merge_method"] != "squash" {
				t.Fatalf("unexpected merge_method payload: %v", payload["merge_method"])
			}
			if payload["sha"] != sha {
				t.Fatalf("unexpected sha payload: %v", payload["sha"])
			}

			body := `{
				"sha":"abc123abc123abc123abc123abc123abc123abcd",
				"merged":true,
				"message":"Pull Request successfully merged"
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	result, err := service.MergePullRequestREST(MergePRInput{
		Owner:       "orch-labs",
		Repo:        "orch",
		Number:      55,
		MergeMethod: PRMergeMethodSquash,
		SHA:         &sha,
	})
	if err != nil {
		t.Fatalf("MergePullRequestREST() error: %v", err)
	}
	if result == nil {
		t.Fatalf("expected merge result")
	}
	if !result.Merged {
		t.Fatalf("expected merged=true")
	}
	if result.Message != "Pull Request successfully merged" {
		t.Fatalf("unexpected merge message: %q", result.Message)
	}
}

func TestMergePullRequestRESTInvalidatesRepositoryCache(t *testing.T) {
	var getCalls int
	var putCalls int

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.Method {
			case http.MethodGet:
				getCalls++
				if req.URL.Path != "/repos/orch-labs/orch/pulls" {
					t.Fatalf("unexpected GET path: got=%s", req.URL.Path)
				}

				body := `[{
					"node_id":"PR_cached",
					"number":77,
					"title":"Cached list",
					"state":"open",
					"created_at":"2026-02-20T10:00:00Z",
					"updated_at":"2026-02-20T10:01:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/cached"},
					"base":{"ref":"main"}
				}]`
				if getCalls == 2 {
					body = `[{
						"node_id":"PR_refetched",
						"number":88,
						"title":"Refetched list",
						"state":"closed",
						"created_at":"2026-02-20T10:00:00Z",
						"updated_at":"2026-02-20T10:01:00Z",
						"user":{"login":"dev","avatar_url":""},
						"head":{"ref":"feature/refetched"},
						"base":{"ref":"main"}
					}]`
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			case http.MethodPut:
				putCalls++
				if req.URL.Path != "/repos/orch-labs/orch/pulls/77/merge" {
					t.Fatalf("unexpected PUT path: got=%s", req.URL.Path)
				}

				body := `{"sha":"abc","merged":true,"message":"Pull Request successfully merged"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			default:
				t.Fatalf("unexpected method: %s", req.Method)
				return nil, nil
			}
		}),
	}

	_, err := service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("first ListPullRequests() error: %v", err)
	}
	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("second ListPullRequests() error: %v", err)
	}
	if getCalls != 1 {
		t.Fatalf("expected list cache hit before merge, getCalls=%d", getCalls)
	}

	_, err = service.MergePullRequestREST(MergePRInput{
		Owner:       "orch-labs",
		Repo:        "orch",
		Number:      77,
		MergeMethod: PRMergeMethodMerge,
	})
	if err != nil {
		t.Fatalf("MergePullRequestREST() error: %v", err)
	}
	if putCalls != 1 {
		t.Fatalf("expected one merge request, putCalls=%d", putCalls)
	}

	listAfterMerge, err := service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("third ListPullRequests() error: %v", err)
	}
	if getCalls != 2 {
		t.Fatalf("expected list refetch after merge invalidation, getCalls=%d", getCalls)
	}
	if len(listAfterMerge) != 1 || listAfterMerge[0].Number != 88 {
		t.Fatalf("unexpected list payload after invalidation: %+v", listAfterMerge)
	}
}

func TestMergePullRequestRESTRejectsInvalidSHA(t *testing.T) {
	shortSHA := "abc123"

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})

	_, err := service.MergePullRequestREST(MergePRInput{
		Owner:       "orch-labs",
		Repo:        "orch",
		Number:      42,
		MergeMethod: PRMergeMethodMerge,
		SHA:         &shortSHA,
	})
	if err == nil {
		t.Fatalf("expected validation error for invalid merge sha")
	}

	ghErr, ok := err.(*GitHubError)
	if !ok {
		t.Fatalf("expected GitHubError, got=%T", err)
	}
	if ghErr.StatusCode != 422 {
		t.Fatalf("unexpected status code: got=%d want=422", ghErr.StatusCode)
	}
	if ghErr.Type != "validation" {
		t.Fatalf("unexpected error type: got=%s want=validation", ghErr.Type)
	}
}

func TestUpdatePullRequestBranchRESTUsesOfficialHeadersAndPayload(t *testing.T) {
	expectedSHA := "a145a1f1ce6aee16de4f9517d2f41295bb12f341"

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPut {
				t.Fatalf("unexpected method: got=%s want=%s", req.Method, http.MethodPut)
			}
			if req.URL.Path != "/repos/orch-labs/orch/pulls/55/update-branch" {
				t.Fatalf("unexpected path: got=%s", req.URL.Path)
			}
			if req.Header.Get("Authorization") != "Bearer gh-token" {
				t.Fatalf("unexpected authorization header: %s", req.Header.Get("Authorization"))
			}
			if req.Header.Get("Accept") != githubRESTAcceptJSON {
				t.Fatalf("unexpected accept header: %s", req.Header.Get("Accept"))
			}
			if req.Header.Get("X-GitHub-Api-Version") != githubRESTAPIVersion {
				t.Fatalf("unexpected api version header: %s", req.Header.Get("X-GitHub-Api-Version"))
			}
			if req.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected content type: %s", req.Header.Get("Content-Type"))
			}

			rawBody, readErr := io.ReadAll(req.Body)
			if readErr != nil {
				t.Fatalf("failed to read request body: %v", readErr)
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(rawBody, &payload); err != nil {
				t.Fatalf("failed to parse payload: %v", err)
			}
			if payload["expected_head_sha"] != expectedSHA {
				t.Fatalf("unexpected expected_head_sha payload: %v", payload["expected_head_sha"])
			}

			body := `{"message":"Updating pull request branch."}`
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	result, err := service.UpdatePullRequestBranch(UpdatePRBranchInput{
		Owner:           "orch-labs",
		Repo:            "orch",
		Number:          55,
		ExpectedHeadSHA: &expectedSHA,
	})
	if err != nil {
		t.Fatalf("UpdatePullRequestBranch() error: %v", err)
	}
	if result == nil {
		t.Fatalf("expected update-branch result")
	}
	if result.Message != "Updating pull request branch." {
		t.Fatalf("unexpected update-branch message: %q", result.Message)
	}
}

func TestUpdatePullRequestBranchRESTInvalidatesRepositoryCache(t *testing.T) {
	var getCalls int
	var putCalls int

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.Method {
			case http.MethodGet:
				getCalls++
				if req.URL.Path != "/repos/orch-labs/orch/pulls" {
					t.Fatalf("unexpected GET path: got=%s", req.URL.Path)
				}

				body := `[{
					"node_id":"PR_cached",
					"number":77,
					"title":"Cached list",
					"state":"open",
					"created_at":"2026-02-20T10:00:00Z",
					"updated_at":"2026-02-20T10:01:00Z",
					"user":{"login":"dev","avatar_url":""},
					"head":{"ref":"feature/cached"},
					"base":{"ref":"main"}
				}]`
				if getCalls == 2 {
					body = `[{
						"node_id":"PR_refetched",
						"number":88,
						"title":"Refetched list",
						"state":"open",
						"created_at":"2026-02-20T10:00:00Z",
						"updated_at":"2026-02-20T10:01:00Z",
						"user":{"login":"dev","avatar_url":""},
						"head":{"ref":"feature/refetched"},
						"base":{"ref":"main"}
					}]`
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			case http.MethodPut:
				putCalls++
				if req.URL.Path != "/repos/orch-labs/orch/pulls/77/update-branch" {
					t.Fatalf("unexpected PUT path: got=%s", req.URL.Path)
				}
				body := `{"message":"Updating pull request branch."}`
				return &http.Response{
					StatusCode: http.StatusAccepted,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			default:
				t.Fatalf("unexpected method: %s", req.Method)
				return nil, nil
			}
		}),
	}

	_, err := service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("first ListPullRequests() error: %v", err)
	}
	_, err = service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("second ListPullRequests() error: %v", err)
	}
	if getCalls != 1 {
		t.Fatalf("expected list cache hit before update-branch, getCalls=%d", getCalls)
	}

	_, err = service.UpdatePullRequestBranch(UpdatePRBranchInput{
		Owner:  "orch-labs",
		Repo:   "orch",
		Number: 77,
	})
	if err != nil {
		t.Fatalf("UpdatePullRequestBranch() error: %v", err)
	}
	if putCalls != 1 {
		t.Fatalf("expected one update-branch request, putCalls=%d", putCalls)
	}

	listAfterUpdateBranch, err := service.ListPullRequests("orch-labs", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("third ListPullRequests() error: %v", err)
	}
	if getCalls != 2 {
		t.Fatalf("expected list refetch after update-branch invalidation, getCalls=%d", getCalls)
	}
	if len(listAfterUpdateBranch) != 1 || listAfterUpdateBranch[0].Number != 88 {
		t.Fatalf("unexpected list payload after invalidation: %+v", listAfterUpdateBranch)
	}
}

func TestUpdatePullRequestBranchRESTRejectsInvalidExpectedHeadSHA(t *testing.T) {
	shortSHA := "def456"

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})

	_, err := service.UpdatePullRequestBranch(UpdatePRBranchInput{
		Owner:           "orch-labs",
		Repo:            "orch",
		Number:          42,
		ExpectedHeadSHA: &shortSHA,
	})
	if err == nil {
		t.Fatalf("expected validation error for invalid expected_head_sha")
	}

	ghErr, ok := err.(*GitHubError)
	if !ok {
		t.Fatalf("expected GitHubError, got=%T", err)
	}
	if ghErr.StatusCode != 422 {
		t.Fatalf("unexpected status code: got=%d want=422", ghErr.StatusCode)
	}
	if ghErr.Type != "validation" {
		t.Fatalf("unexpected error type: got=%s want=validation", ghErr.Type)
	}
}

func TestListPullRequestsRESTUsesETagConditionalRequestOnExpiredCache(t *testing.T) {
	requestCount := 0

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++
			if requestCount == 1 {
				body := `[{
					"node_id": "PR_first",
					"number": 808,
					"title": "Cached PR",
					"state": "open",
					"created_at": "2026-02-20T10:00:00Z",
					"updated_at": "2026-02-20T10:01:00Z",
					"user": {"login": "dev", "avatar_url": ""},
					"head": {"ref": "feature/cache"},
					"base": {"ref": "main"}
				}]`
				headers := make(http.Header)
				headers.Set("ETag", `"prs-open-etag"`)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     headers,
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}

			if got := req.Header.Get("If-None-Match"); got != `"prs-open-etag"` {
				t.Fatalf("expected If-None-Match with cached etag, got=%q", got)
			}
			headers := make(http.Header)
			headers.Set("ETag", `"prs-open-etag"`)
			return &http.Response{
				StatusCode: http.StatusNotModified,
				Header:     headers,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	first, err := service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("first ListPullRequests() error: %v", err)
	}
	if len(first) != 1 || first[0].Number != 808 {
		t.Fatalf("unexpected first response: %+v", first)
	}

	cacheKey := prListKey("orch-labs", "orch", "open", 1, 25)
	service.cache.mu.Lock()
	service.cache.updatedAt[cacheKey] = time.Now().Add(-time.Minute)
	service.cache.mu.Unlock()

	second, err := service.ListPullRequests("orch-labs", "orch", PRFilters{State: "open", Page: 1, PerPage: 25})
	if err != nil {
		t.Fatalf("second ListPullRequests() error: %v", err)
	}
	if len(second) != 1 || second[0].Number != 808 {
		t.Fatalf("unexpected second response: %+v", second)
	}
	if requestCount != 2 {
		t.Fatalf("expected conditional second request, requests=%d", requestCount)
	}
}

func TestGetPullRequestRESTUsesETagConditionalRequestOnExpiredCache(t *testing.T) {
	requestCount := 0

	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requestCount++
			if requestCount == 1 {
				body := `{
					"node_id": "PR_detail",
					"number": 33,
					"title": "Cached detail",
					"body": "body",
					"state": "open",
					"created_at": "2026-02-20T10:00:00Z",
					"updated_at": "2026-02-20T10:01:00Z",
					"user": {"login": "dev", "avatar_url": ""},
					"head": {"ref": "feature/cache-detail"},
					"base": {"ref": "main"}
				}`
				headers := make(http.Header)
				headers.Set("ETag", `"pr-detail-etag"`)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     headers,
					Body:       io.NopCloser(strings.NewReader(body)),
				}, nil
			}

			if got := req.Header.Get("If-None-Match"); got != `"pr-detail-etag"` {
				t.Fatalf("expected If-None-Match with cached etag, got=%q", got)
			}
			headers := make(http.Header)
			headers.Set("ETag", `"pr-detail-etag"`)
			return &http.Response{
				StatusCode: http.StatusNotModified,
				Header:     headers,
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}),
	}

	first, err := service.GetPullRequest("orch-labs", "orch", 33)
	if err != nil {
		t.Fatalf("first GetPullRequest() error: %v", err)
	}
	if first == nil || first.Number != 33 {
		t.Fatalf("unexpected first PR detail: %+v", first)
	}

	cacheKey := prDetailKey("orch-labs", "orch", 33)
	service.cache.mu.Lock()
	service.cache.updatedAt[cacheKey] = time.Now().Add(-time.Minute)
	service.cache.mu.Unlock()

	second, err := service.GetPullRequest("orch-labs", "orch", 33)
	if err != nil {
		t.Fatalf("second GetPullRequest() error: %v", err)
	}
	if second == nil || second.Number != 33 {
		t.Fatalf("unexpected second PR detail: %+v", second)
	}
	if requestCount != 2 {
		t.Fatalf("expected conditional second request, requests=%d", requestCount)
	}
}

func TestListPullRequestsRESTRejectsInvalidState(t *testing.T) {
	service := NewService(func() (string, error) {
		return "gh-token", nil
	})

	_, err := service.ListPullRequests("orch-labs", "orch", PRFilters{State: "invalid-state"})
	if err == nil {
		t.Fatalf("expected invalid state error")
	}

	ghErr, ok := err.(*GitHubError)
	if !ok {
		t.Fatalf("expected GitHubError, got=%T", err)
	}
	if ghErr.StatusCode != 422 {
		t.Fatalf("unexpected status code: got=%d want=422", ghErr.StatusCode)
	}
	if ghErr.Type != "validation" {
		t.Fatalf("unexpected error type: got=%s want=validation", ghErr.Type)
	}
}

func TestCreateInlineCommentUsesRESTClientHeaders(t *testing.T) {
	service := NewService(func() (string, error) {
		return "gh-token", nil
	})
	service.client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Fatalf("unexpected method: got=%s want=%s", req.Method, http.MethodPost)
			}
			if req.URL.Path != "/repos/orch-labs/orch/pulls/9/comments" {
				t.Fatalf("unexpected path: got=%s", req.URL.Path)
			}
			if req.Header.Get("Authorization") != "Bearer gh-token" {
				t.Fatalf("unexpected authorization header: %s", req.Header.Get("Authorization"))
			}
			if req.Header.Get("Accept") != githubRESTAcceptJSON {
				t.Fatalf("unexpected accept header: %s", req.Header.Get("Accept"))
			}
			if req.Header.Get("X-GitHub-Api-Version") != githubRESTAPIVersion {
				t.Fatalf("unexpected api version header: %s", req.Header.Get("X-GitHub-Api-Version"))
			}
			if req.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected content type: %s", req.Header.Get("Content-Type"))
			}

			rawBody, readErr := io.ReadAll(req.Body)
			if readErr != nil {
				t.Fatalf("failed to read request body: %v", readErr)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(rawBody, &payload); err != nil {
				t.Fatalf("failed to parse payload: %v", err)
			}
			if payload["body"] != "LGTM" {
				t.Fatalf("unexpected body payload: %v", payload["body"])
			}

			body := `{
				"id": 123,
				"body": "LGTM",
				"path": "internal/github/rest_prs.go",
				"line": 10,
				"created_at": "2026-02-23T10:00:00Z",
				"user": {"login": "reviewer", "avatar_url": "https://avatar/reviewer"}
			}`
			return &http.Response{
				StatusCode: http.StatusCreated,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}

	comment, err := service.CreateInlineComment(InlineCommentInput{
		Owner:    "orch-labs",
		Repo:     "orch",
		PRNumber: 9,
		Body:     "LGTM",
		Path:     "internal/github/rest_prs.go",
		Line:     10,
		Side:     "RIGHT",
	})
	if err != nil {
		t.Fatalf("CreateInlineComment() error: %v", err)
	}
	if comment == nil {
		t.Fatalf("expected created comment")
	}
	if comment.ID != "123" {
		t.Fatalf("unexpected comment id: got=%s want=123", comment.ID)
	}
}
