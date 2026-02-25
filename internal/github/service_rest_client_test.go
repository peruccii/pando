package github

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestExecuteRESTRequestConditionalBuildsOfficialHeadersAndQuery(t *testing.T) {
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: got=%s want=%s", r.Method, http.MethodGet)
		}
		if r.URL.Path != "/repos/acme/orch/pulls" {
			t.Fatalf("unexpected path: got=%s", r.URL.Path)
		}
		if got := r.URL.Query().Get("state"); got != "open" {
			t.Fatalf("unexpected state query: got=%s want=open", got)
		}
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Fatalf("unexpected page query: got=%s want=2", got)
		}
		if got := r.URL.Query().Get("per_page"); got != "50" {
			t.Fatalf("unexpected per_page query: got=%s want=50", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("unexpected auth header: got=%q", got)
		}
		if got := r.Header.Get("Accept"); got != githubRESTAcceptJSON {
			t.Fatalf("unexpected accept header: got=%q want=%q", got, githubRESTAcceptJSON)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != githubRESTAPIVersion {
			t.Fatalf("unexpected api version header: got=%q want=%q", got, githubRESTAPIVersion)
		}
		if got := r.Header.Get("If-None-Match"); got != `"list-etag"` {
			t.Fatalf("unexpected If-None-Match header: got=%q want=%q", got, `"list-etag"`)
		}
		if got := r.Header.Get("Content-Type"); got != "" {
			t.Fatalf("expected no Content-Type for request without body, got=%q", got)
		}

		w.Header().Set("ETag", `"list-etag-v2"`)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `[]`)
	})

	query := url.Values{}
	query.Set("state", "open")
	query.Set("page", "2")
	query.Set("per_page", "50")

	body, headers, statusCode, err := service.executeRESTRequestConditional(
		http.MethodGet,
		"repos/acme/orch/pulls",
		query,
		"",
		nil,
		`"list-etag"`,
	)
	if err != nil {
		t.Fatalf("executeRESTRequestConditional() error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got=%d want=%d", statusCode, http.StatusOK)
	}
	if got := string(body); got != "[]" {
		t.Fatalf("unexpected body: got=%q want=%q", got, "[]")
	}
	if got := headers.Get("ETag"); got != `"list-etag-v2"` {
		t.Fatalf("unexpected response etag: got=%q want=%q", got, `"list-etag-v2"`)
	}
}

func TestExecuteRESTRequestConditionalReturnsNotModifiedWithoutError(t *testing.T) {
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("If-None-Match"); got != `"detail-etag"` {
			t.Fatalf("unexpected If-None-Match header: got=%q want=%q", got, `"detail-etag"`)
		}
		w.Header().Set("ETag", `"detail-etag"`)
		w.WriteHeader(http.StatusNotModified)
	})

	body, headers, statusCode, err := service.executeRESTRequestConditional(
		http.MethodGet,
		"/repos/acme/orch/pulls/42",
		nil,
		githubRESTAcceptJSON,
		nil,
		`"detail-etag"`,
	)
	if err != nil {
		t.Fatalf("executeRESTRequestConditional() 304 error: %v", err)
	}
	if statusCode != http.StatusNotModified {
		t.Fatalf("unexpected status code: got=%d want=%d", statusCode, http.StatusNotModified)
	}
	if len(body) != 0 {
		t.Fatalf("expected empty body on 304 response, got=%q", string(body))
	}
	if got := headers.Get("ETag"); got != `"detail-etag"` {
		t.Fatalf("unexpected response etag: got=%q want=%q", got, `"detail-etag"`)
	}
}

func TestExecuteRESTRequestConditionalMapsHTTPErrors(t *testing.T) {
	type testCase struct {
		name            string
		statusCode      int
		responseBody    string
		setup           func(*Service)
		wantType        string
		wantMsgContains string
	}

	cases := []testCase{
		{
			name:            "401 unauthorized",
			statusCode:      http.StatusUnauthorized,
			responseBody:    `{"message":"Bad credentials"}`,
			wantType:        "auth",
			wantMsgContains: "GitHub token expired or invalid",
		},
		{
			name:            "403 permission denied",
			statusCode:      http.StatusForbidden,
			responseBody:    `{"message":"Forbidden"}`,
			wantType:        "permission",
			wantMsgContains: "Permission denied: Forbidden",
		},
		{
			name:         "403 secondary rate limit",
			statusCode:   http.StatusForbidden,
			responseBody: `{"message":"You have exceeded a secondary rate limit."}`,
			setup: func(s *Service) {
				s.rateLeft = 5000
				s.rateReset = time.Date(2026, time.February, 24, 15, 4, 0, 0, time.UTC)
			},
			wantType:        "ratelimit",
			wantMsgContains: "Rate limit exceeded. Resets at",
		},
		{
			name:            "404 not found",
			statusCode:      http.StatusNotFound,
			responseBody:    `{"message":"Not Found"}`,
			wantType:        "notfound",
			wantMsgContains: "Resource not found",
		},
		{
			name:            "409 conflict",
			statusCode:      http.StatusConflict,
			responseBody:    `{"message":"Conflict"}`,
			wantType:        "conflict",
			wantMsgContains: "Merge conflict detected",
		},
		{
			name:            "422 validation failed",
			statusCode:      http.StatusUnprocessableEntity,
			responseBody:    `{"message":"Validation Failed","errors":[{"field":"base","message":"invalid"}]}`,
			wantType:        "validation",
			wantMsgContains: "Validation failed: Validation Failed",
		},
		{
			name:            "429 too many requests",
			statusCode:      http.StatusTooManyRequests,
			responseBody:    `{"message":"Too many requests"}`,
			wantType:        "ratelimit",
			wantMsgContains: "Too many requests",
		},
		{
			name:            "500 unknown",
			statusCode:      http.StatusInternalServerError,
			responseBody:    `{"message":"internal error"}`,
			wantType:        "unknown",
			wantMsgContains: "GitHub API error 500: internal error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newPRRESTTestService(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = io.WriteString(w, tc.responseBody)
			})
			if tc.setup != nil {
				tc.setup(service)
			}

			_, _, statusCode, err := service.executeRESTRequestConditional(
				http.MethodGet,
				"/repos/acme/orch/pulls",
				nil,
				githubRESTAcceptJSON,
				nil,
				"",
			)
			if err == nil {
				t.Fatalf("expected error for status=%d", tc.statusCode)
			}
			if statusCode != tc.statusCode {
				t.Fatalf("unexpected status code: got=%d want=%d", statusCode, tc.statusCode)
			}

			ghErr, ok := err.(*GitHubError)
			if !ok {
				t.Fatalf("expected GitHubError, got=%T err=%v", err, err)
			}
			if ghErr.StatusCode != tc.statusCode {
				t.Fatalf("unexpected GitHubError status: got=%d want=%d", ghErr.StatusCode, tc.statusCode)
			}
			if ghErr.Type != tc.wantType {
				t.Fatalf("unexpected GitHubError type: got=%q want=%q", ghErr.Type, tc.wantType)
			}
			if !strings.Contains(ghErr.Message, tc.wantMsgContains) {
				t.Fatalf("unexpected GitHubError message: got=%q wantContains=%q", ghErr.Message, tc.wantMsgContains)
			}
		})
	}
}

func TestExecuteRESTRequestConditionalPermissionErrorIncludesScopeHint(t *testing.T) {
	service := newPRRESTTestService(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Accepted-GitHub-Permissions", "pull_requests=write,contents=write")
		w.Header().Set("X-OAuth-Scopes", "repo, read:org")
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"Resource not accessible by integration"}`)
	})

	_, _, statusCode, err := service.executeRESTRequestConditional(
		http.MethodPut,
		"/repos/acme/orch/pulls/42/merge",
		nil,
		githubRESTAcceptJSON,
		nil,
		"",
	)
	if err == nil {
		t.Fatalf("expected error for forbidden response")
	}
	if statusCode != http.StatusForbidden {
		t.Fatalf("unexpected status code: got=%d want=%d", statusCode, http.StatusForbidden)
	}

	ghErr, ok := err.(*GitHubError)
	if !ok {
		t.Fatalf("expected GitHubError, got=%T err=%v", err, err)
	}
	if ghErr.Type != "permission" {
		t.Fatalf("unexpected GitHubError type: got=%q want=%q", ghErr.Type, "permission")
	}
	if !strings.Contains(ghErr.Message, "required=pull_requests:write,contents:write") {
		t.Fatalf("expected required permissions hint, got=%q", ghErr.Message)
	}
	if !strings.Contains(ghErr.Message, "accepted_permissions=pull_requests=write,contents=write") {
		t.Fatalf("expected accepted permissions hint, got=%q", ghErr.Message)
	}
	if !strings.Contains(ghErr.Message, "token_scopes=repo, read:org") {
		t.Fatalf("expected token scopes hint, got=%q", ghErr.Message)
	}
}

func TestParseNextPageFromLinkHeader(t *testing.T) {
	type testCase struct {
		name        string
		linkHeader  string
		wantNext    int
		wantHasNext bool
	}

	cases := []testCase{
		{
			name:        "empty header",
			linkHeader:  "",
			wantNext:    0,
			wantHasNext: false,
		},
		{
			name: "next and last links",
			linkHeader: `<https://api.github.com/repos/acme/orch/pulls/42/commits?page=3&per_page=50>; rel="next", ` +
				`<https://api.github.com/repos/acme/orch/pulls/42/commits?page=4&per_page=50>; rel="last"`,
			wantNext:    3,
			wantHasNext: true,
		},
		{
			name:        "no next relation",
			linkHeader:  `<https://api.github.com/repos/acme/orch/pulls/42/commits?page=4&per_page=50>; rel="last"`,
			wantNext:    0,
			wantHasNext: false,
		},
		{
			name:        "next without page parameter",
			linkHeader:  `<https://api.github.com/repos/acme/orch/pulls/42/commits?per_page=50>; rel="next"`,
			wantNext:    0,
			wantHasNext: true,
		},
		{
			name:        "next with invalid page",
			linkHeader:  `<https://api.github.com/repos/acme/orch/pulls/42/commits?page=abc&per_page=50>; rel="next"`,
			wantNext:    0,
			wantHasNext: true,
		},
		{
			name:        "next with non-positive page",
			linkHeader:  `<https://api.github.com/repos/acme/orch/pulls/42/commits?page=0&per_page=50>; rel="next"`,
			wantNext:    0,
			wantHasNext: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nextPage, hasNextPage := parseNextPageFromLinkHeader(tc.linkHeader)
			if nextPage != tc.wantNext || hasNextPage != tc.wantHasNext {
				t.Fatalf(
					"unexpected parse result: next=%d hasNext=%v wantNext=%d wantHasNext=%v",
					nextPage,
					hasNextPage,
					tc.wantNext,
					tc.wantHasNext,
				)
			}
		})
	}
}
