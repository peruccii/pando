package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetPullRequestCommitsRESTPagination(t *testing.T) {
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: got=%s want=%s", r.Method, http.MethodGet)
		}
		if r.URL.Path != "/repos/acme/orch/pulls/42/commits" {
			t.Fatalf("unexpected path: got=%s", r.URL.Path)
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
			t.Fatalf("unexpected accept header: got=%q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != githubRESTAPIVersion {
			t.Fatalf("unexpected api version header: got=%q", got)
		}

		w.Header().Set("Link", `<https://api.github.com/repos/acme/orch/pulls/42/commits?page=3&per_page=50>; rel="next", <https://api.github.com/repos/acme/orch/pulls/42/commits?page=4&per_page=50>; rel="last"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[
			{
				"sha":"abc123",
				"html_url":"https://github.com/acme/orch/commit/abc123",
				"commit":{
					"message":"feat: add endpoint",
					"author":{"name":"Ana","email":"ana@example.com","date":"2026-02-20T10:00:00Z"},
					"committer":{"name":"Ana","email":"ana@example.com","date":"2026-02-20T10:00:01Z"}
				},
				"author":{"login":"ana","avatar_url":"https://avatars.githubusercontent.com/u/1"},
				"committer":{"login":"ana","avatar_url":"https://avatars.githubusercontent.com/u/1"},
				"parents":[{"sha":"def456"}]
			}
		]`)
	})

	page, err := service.GetPullRequestCommits("acme", "orch", 42, 2, 50)
	if err != nil {
		t.Fatalf("GetPullRequestCommits() error: %v", err)
	}
	if page == nil {
		t.Fatalf("expected page result")
	}
	if page.Page != 2 || page.PerPage != 50 {
		t.Fatalf("unexpected pagination: page=%d perPage=%d", page.Page, page.PerPage)
	}
	if !page.HasNextPage || page.NextPage != 3 {
		t.Fatalf("unexpected next page info: hasNext=%v next=%d", page.HasNextPage, page.NextPage)
	}
	if len(page.Items) != 1 {
		t.Fatalf("unexpected item count: got=%d want=1", len(page.Items))
	}

	commit := page.Items[0]
	if commit.SHA != "abc123" {
		t.Fatalf("unexpected commit sha: got=%q", commit.SHA)
	}
	if commit.Message != "feat: add endpoint" {
		t.Fatalf("unexpected commit message: got=%q", commit.Message)
	}
	if commit.Author == nil || commit.Author.Login != "ana" {
		t.Fatalf("expected github author login")
	}
	if len(commit.ParentSHAs) != 1 || commit.ParentSHAs[0] != "def456" {
		t.Fatalf("unexpected parent SHAs: %+v", commit.ParentSHAs)
	}
}

func TestGetPullRequestCommitsRESTUsesReadThroughCacheByPage(t *testing.T) {
	requestCount := 0
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[
			{
				"sha":"abc123",
				"html_url":"https://github.com/acme/orch/commit/abc123",
				"commit":{
					"message":"feat: cache commits",
					"author":{"name":"Ana","email":"ana@example.com","date":"2026-02-20T10:00:00Z"},
					"committer":{"name":"Ana","email":"ana@example.com","date":"2026-02-20T10:00:01Z"}
				},
				"parents":[{"sha":"def456"}]
			}
		]`)
	})

	_, err := service.GetPullRequestCommits("acme", "orch", 42, 1, 25)
	if err != nil {
		t.Fatalf("first GetPullRequestCommits() error: %v", err)
	}
	_, err = service.GetPullRequestCommits("acme", "orch", 42, 1, 25)
	if err != nil {
		t.Fatalf("second GetPullRequestCommits() error: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected cached commits response for same page, requests=%d", requestCount)
	}

	_, err = service.GetPullRequestCommits("acme", "orch", 42, 2, 25)
	if err != nil {
		t.Fatalf("GetPullRequestCommits() page 2 error: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected separate cache entry for page 2, requests=%d", requestCount)
	}
}

func TestGetPullRequestCommitsRESTUsesETagConditionalRequestOnExpiredCache(t *testing.T) {
	requestCount := 0
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("ETag", `"commits-etag"`)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[
				{
					"sha":"abc123",
					"commit":{
						"message":"feat: cache commits with etag",
						"author":{"name":"Ana","email":"ana@example.com","date":"2026-02-20T10:00:00Z"},
						"committer":{"name":"Ana","email":"ana@example.com","date":"2026-02-20T10:00:01Z"}
					}
				}
			]`)
			return
		}

		if got := r.Header.Get("If-None-Match"); got != `"commits-etag"` {
			t.Fatalf("expected If-None-Match header on conditional request, got=%q", got)
		}
		w.Header().Set("ETag", `"commits-etag"`)
		w.WriteHeader(http.StatusNotModified)
	})

	first, err := service.GetPullRequestCommits("acme", "orch", 42, 1, 25)
	if err != nil {
		t.Fatalf("first GetPullRequestCommits() error: %v", err)
	}
	if first == nil || len(first.Items) != 1 {
		t.Fatalf("unexpected first response: %+v", first)
	}

	cacheKey := prCommitsKey("acme", "orch", 42, 1, 25)
	service.cache.mu.Lock()
	service.cache.updatedAt[cacheKey] = time.Now().Add(-time.Minute)
	service.cache.mu.Unlock()

	second, err := service.GetPullRequestCommits("acme", "orch", 42, 1, 25)
	if err != nil {
		t.Fatalf("second GetPullRequestCommits() error: %v", err)
	}
	if second == nil || len(second.Items) != 1 {
		t.Fatalf("unexpected second response: %+v", second)
	}
	if requestCount != 2 {
		t.Fatalf("expected conditional request after TTL expiry, requests=%d", requestCount)
	}
}

func TestGetPullRequestFilesPatchStates(t *testing.T) {
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: got=%s want=%s", r.Method, http.MethodGet)
		}
		if r.URL.Path != "/repos/acme/orch/pulls/7/files" {
			t.Fatalf("unexpected path: got=%s", r.URL.Path)
		}
		if got := r.URL.Query().Get("page"); got != "1" {
			t.Fatalf("unexpected page query: got=%s want=1", got)
		}
		if got := r.URL.Query().Get("per_page"); got != "25" {
			t.Fatalf("unexpected per_page query: got=%s want=25", got)
		}
		if got := r.Header.Get("Accept"); got != githubRESTAcceptJSON {
			t.Fatalf("unexpected accept header: got=%q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[
			{
				"filename":"src/main.go",
				"status":"modified",
				"additions":3,
				"deletions":1,
				"changes":4,
				"patch":"@@ -1 +1 @@\n-old\n+new"
			},
			{
				"filename":"assets/logo.png",
				"status":"modified",
				"additions":0,
				"deletions":0,
				"changes":0
			},
			{
				"filename":"bin/blob.bin",
				"status":"modified",
				"additions":0,
				"deletions":0,
				"changes":0,
				"patch":"Binary files a/bin/blob.bin and b/bin/blob.bin differ"
			},
			{
				"filename":"src/huge.ts",
				"status":"modified",
				"additions":2000,
				"deletions":100,
				"changes":2100,
				"patch":"@@ -1 +1 @@\n-old\n+new\n..."
			}
		]`)
	})

	page, err := service.GetPullRequestFiles("acme", "orch", 7, 1, 25)
	if err != nil {
		t.Fatalf("GetPullRequestFiles() error: %v", err)
	}
	if page == nil {
		t.Fatalf("expected page result")
	}
	if len(page.Items) != 4 {
		t.Fatalf("unexpected item count: got=%d want=4", len(page.Items))
	}

	available := page.Items[0]
	if !available.HasPatch || available.PatchState != PRFilePatchStateAvailable {
		t.Fatalf("unexpected available patch state: hasPatch=%v state=%s", available.HasPatch, available.PatchState)
	}

	missingBinary := page.Items[1]
	if missingBinary.HasPatch {
		t.Fatalf("expected missing patch for binary candidate file")
	}
	if !missingBinary.IsBinary || missingBinary.PatchState != PRFilePatchStateBinary {
		t.Fatalf("unexpected binary-missing patch state: isBinary=%v state=%s", missingBinary.IsBinary, missingBinary.PatchState)
	}

	binaryMarker := page.Items[2]
	if binaryMarker.HasPatch {
		t.Fatalf("expected marker binary patch to be treated as non-renderable patch")
	}
	if !binaryMarker.IsBinary || binaryMarker.PatchState != PRFilePatchStateBinary {
		t.Fatalf("unexpected binary marker state: isBinary=%v state=%s", binaryMarker.IsBinary, binaryMarker.PatchState)
	}

	truncated := page.Items[3]
	if !truncated.HasPatch {
		t.Fatalf("expected truncated patch to preserve partial patch data")
	}
	if !truncated.IsPatchTruncated || truncated.PatchState != PRFilePatchStateTruncated {
		t.Fatalf("unexpected truncated state: truncated=%v state=%s", truncated.IsPatchTruncated, truncated.PatchState)
	}
}

func TestGetPullRequestFilesCapsLargePatchPayload(t *testing.T) {
	largePatch := strings.Repeat("+large-line\n", (maxPRFilePatchBytes/11)+2_000)
	encodedPatch, err := json.Marshal(largePatch)
	if err != nil {
		t.Fatalf("failed to encode patch payload: %v", err)
	}

	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `[
			{
				"filename":"src/huge.ts",
				"status":"modified",
				"additions":9000,
				"deletions":0,
				"changes":9000,
				"patch":%s
			}
		]`, encodedPatch)
	})

	page, err := service.GetPullRequestFiles("acme", "orch", 99, 1, 25)
	if err != nil {
		t.Fatalf("GetPullRequestFiles() error: %v", err)
	}
	if page == nil || len(page.Items) != 1 {
		t.Fatalf("unexpected files page result: %+v", page)
	}

	item := page.Items[0]
	if !item.HasPatch {
		t.Fatalf("expected preserved partial patch after truncation")
	}
	if !item.IsPatchTruncated || item.PatchState != PRFilePatchStateTruncated {
		t.Fatalf("expected truncated patch state, got truncated=%v state=%s", item.IsPatchTruncated, item.PatchState)
	}
	if len(item.Patch) > maxPRFilePatchBytes+4 {
		t.Fatalf("expected bounded patch payload, got len=%d", len(item.Patch))
	}
	if !strings.HasSuffix(strings.TrimSpace(item.Patch), "...") {
		t.Fatalf("expected bounded patch to end with ellipsis marker")
	}
}

func TestGetPullRequestFilesRESTUsesReadThroughCacheByPage(t *testing.T) {
	requestCount := 0
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[
			{
				"filename":"src/main.go",
				"status":"modified",
				"additions":1,
				"deletions":0,
				"changes":1,
				"patch":"@@ -1 +1 @@\n-old\n+new"
			}
		]`)
	})

	_, err := service.GetPullRequestFiles("acme", "orch", 7, 1, 25)
	if err != nil {
		t.Fatalf("first GetPullRequestFiles() error: %v", err)
	}
	_, err = service.GetPullRequestFiles("acme", "orch", 7, 1, 25)
	if err != nil {
		t.Fatalf("second GetPullRequestFiles() error: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected cached files response for same page, requests=%d", requestCount)
	}

	_, err = service.GetPullRequestFiles("acme", "orch", 7, 2, 25)
	if err != nil {
		t.Fatalf("GetPullRequestFiles() page 2 error: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected separate cache entry for page 2, requests=%d", requestCount)
	}
}

func TestGetPullRequestFilesRESTUsesETagConditionalRequestOnExpiredCache(t *testing.T) {
	requestCount := 0
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("ETag", `"files-etag"`)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `[
				{
					"filename":"src/main.go",
					"status":"modified",
					"additions":1,
					"deletions":0,
					"changes":1,
					"patch":"@@ -1 +1 @@\n-old\n+new"
				}
			]`)
			return
		}

		if got := r.Header.Get("If-None-Match"); got != `"files-etag"` {
			t.Fatalf("expected If-None-Match header on conditional request, got=%q", got)
		}
		w.Header().Set("ETag", `"files-etag"`)
		w.WriteHeader(http.StatusNotModified)
	})

	first, err := service.GetPullRequestFiles("acme", "orch", 7, 1, 25)
	if err != nil {
		t.Fatalf("first GetPullRequestFiles() error: %v", err)
	}
	if first == nil || len(first.Items) != 1 {
		t.Fatalf("unexpected first response: %+v", first)
	}

	cacheKey := prFilesKey("acme", "orch", 7, 1, 25)
	service.cache.mu.Lock()
	service.cache.updatedAt[cacheKey] = time.Now().Add(-time.Minute)
	service.cache.mu.Unlock()

	second, err := service.GetPullRequestFiles("acme", "orch", 7, 1, 25)
	if err != nil {
		t.Fatalf("second GetPullRequestFiles() error: %v", err)
	}
	if second == nil || len(second.Items) != 1 {
		t.Fatalf("unexpected second response: %+v", second)
	}
	if requestCount != 2 {
		t.Fatalf("expected conditional request after TTL expiry, requests=%d", requestCount)
	}
}

func TestGetPullRequestRawDiffUsesDiffAcceptHeader(t *testing.T) {
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: got=%s want=%s", r.Method, http.MethodGet)
		}
		if r.URL.Path != "/repos/acme/orch/pulls/77" {
			t.Fatalf("unexpected path: got=%s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != githubRESTAcceptDiff {
			t.Fatalf("unexpected accept header: got=%q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != githubRESTAPIVersion {
			t.Fatalf("unexpected api version header: got=%q", got)
		}

		_, _ = io.WriteString(w, "diff --git a/file.txt b/file.txt\nindex 111..222 100644\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new\n")
	})

	rawDiff, err := service.GetPullRequestRawDiff("acme", "orch", 77)
	if err != nil {
		t.Fatalf("GetPullRequestRawDiff() error: %v", err)
	}
	if rawDiff == "" {
		t.Fatalf("expected raw diff response")
	}
	if rawDiff[:10] != "diff --git" {
		t.Fatalf("unexpected diff prefix: %q", rawDiff[:10])
	}
}

func TestGetPullRequestRawDiffUsesReadThroughCache(t *testing.T) {
	requestCount := 0
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_, _ = io.WriteString(w, "diff --git a/file.txt b/file.txt\n@@ -1 +1 @@\n-old\n+new\n")
	})

	_, err := service.GetPullRequestRawDiff("acme", "orch", 77)
	if err != nil {
		t.Fatalf("first GetPullRequestRawDiff() error: %v", err)
	}
	_, err = service.GetPullRequestRawDiff("acme", "orch", 77)
	if err != nil {
		t.Fatalf("second GetPullRequestRawDiff() error: %v", err)
	}
	if requestCount != 1 {
		t.Fatalf("expected cached raw diff response, requests=%d", requestCount)
	}
}

func TestGetPullRequestRawDiffUsesETagConditionalRequestOnExpiredCache(t *testing.T) {
	requestCount := 0
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("ETag", `"raw-diff-etag"`)
			_, _ = io.WriteString(w, "diff --git a/file.txt b/file.txt\n@@ -1 +1 @@\n-old\n+new\n")
			return
		}

		if got := r.Header.Get("If-None-Match"); got != `"raw-diff-etag"` {
			t.Fatalf("expected If-None-Match header on conditional request, got=%q", got)
		}
		w.Header().Set("ETag", `"raw-diff-etag"`)
		w.WriteHeader(http.StatusNotModified)
	})

	first, err := service.GetPullRequestRawDiff("acme", "orch", 77)
	if err != nil {
		t.Fatalf("first GetPullRequestRawDiff() error: %v", err)
	}
	if first == "" {
		t.Fatalf("expected first raw diff payload")
	}

	cacheKey := prRawDiffKey("acme", "orch", 77)
	service.cache.mu.Lock()
	service.cache.updatedAt[cacheKey] = time.Now().Add(-time.Minute)
	service.cache.mu.Unlock()

	second, err := service.GetPullRequestRawDiff("acme", "orch", 77)
	if err != nil {
		t.Fatalf("second GetPullRequestRawDiff() error: %v", err)
	}
	if second == "" {
		t.Fatalf("expected second raw diff payload")
	}
	if requestCount != 2 {
		t.Fatalf("expected conditional request after TTL expiry, requests=%d", requestCount)
	}
}

func TestCheckPullRequestMergedRESTInterprets204And404(t *testing.T) {
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: got=%s want=%s", r.Method, http.MethodGet)
		}
		if got := r.Header.Get("Accept"); got != githubRESTAcceptJSON {
			t.Fatalf("unexpected accept header: got=%q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != githubRESTAPIVersion {
			t.Fatalf("unexpected api version header: got=%q", got)
		}

		switch r.URL.Path {
		case "/repos/acme/orch/pulls/88/merge":
			w.WriteHeader(http.StatusNoContent)
		case "/repos/acme/orch/pulls/89/merge":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
		default:
			t.Fatalf("unexpected path: got=%s", r.URL.Path)
		}
	})

	merged, err := service.CheckPullRequestMerged("acme", "orch", 88)
	if err != nil {
		t.Fatalf("CheckPullRequestMerged() merged path error: %v", err)
	}
	if !merged {
		t.Fatalf("expected merged=true for 204 response")
	}

	notMerged, err := service.CheckPullRequestMerged("acme", "orch", 89)
	if err != nil {
		t.Fatalf("CheckPullRequestMerged() not merged path error: %v", err)
	}
	if notMerged {
		t.Fatalf("expected merged=false for 404 response")
	}
}

func TestCheckPullRequestMergedRESTUsesReadThroughCache(t *testing.T) {
	requestCount := 0
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusNoContent)
	})

	merged, err := service.CheckPullRequestMerged("acme", "orch", 88)
	if err != nil {
		t.Fatalf("first CheckPullRequestMerged() error: %v", err)
	}
	if !merged {
		t.Fatalf("expected merged=true on first request")
	}

	merged, err = service.CheckPullRequestMerged("acme", "orch", 88)
	if err != nil {
		t.Fatalf("second CheckPullRequestMerged() error: %v", err)
	}
	if !merged {
		t.Fatalf("expected merged=true on cached request")
	}
	if requestCount != 1 {
		t.Fatalf("expected cache hit for repeated merge check, requests=%d", requestCount)
	}
}

func TestCheckPullRequestMergedRESTUsesETagConditionalRequestOnExpiredCache(t *testing.T) {
	requestCount := 0
	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("ETag", `"merge-check-etag"`)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if got := r.Header.Get("If-None-Match"); got != `"merge-check-etag"` {
			t.Fatalf("expected If-None-Match header on conditional request, got=%q", got)
		}
		w.Header().Set("ETag", `"merge-check-etag"`)
		w.WriteHeader(http.StatusNotModified)
	})

	merged, err := service.CheckPullRequestMerged("acme", "orch", 88)
	if err != nil {
		t.Fatalf("first CheckPullRequestMerged() error: %v", err)
	}
	if !merged {
		t.Fatalf("expected merged=true for initial 204 response")
	}

	cacheKey := prMergeCheckKey("acme", "orch", 88)
	service.cache.mu.Lock()
	service.cache.updatedAt[cacheKey] = time.Now().Add(-time.Minute)
	service.cache.mu.Unlock()

	merged, err = service.CheckPullRequestMerged("acme", "orch", 88)
	if err != nil {
		t.Fatalf("second CheckPullRequestMerged() error: %v", err)
	}
	if !merged {
		t.Fatalf("expected merged=true when 304 reuses stale cache")
	}
	if requestCount != 2 {
		t.Fatalf("expected conditional request after TTL expiry, requests=%d", requestCount)
	}
}

func TestCreateLabelRESTSuccess(t *testing.T) {
	requestCount := 0
	labelDescription := "Prioridade critica"

	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: got=%s want=%s", r.Method, http.MethodPost)
		}
		if r.URL.Path != "/repos/acme/orch/labels" {
			t.Fatalf("unexpected path: got=%s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != githubRESTAcceptJSON {
			t.Fatalf("unexpected accept header: got=%q", got)
		}

		var payload map[string]any
		if decodeErr := json.NewDecoder(r.Body).Decode(&payload); decodeErr != nil {
			t.Fatalf("failed to decode request payload: %v", decodeErr)
		}
		if payload["name"] != "P0" {
			t.Fatalf("unexpected label name payload: %#v", payload["name"])
		}
		if payload["color"] != "d73a4a" {
			t.Fatalf("unexpected label color payload: %#v", payload["color"])
		}
		if payload["description"] != labelDescription {
			t.Fatalf("unexpected label description payload: %#v", payload["description"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"P0","color":"d73a4a","description":"Prioridade critica"}`)
	})

	created, err := service.CreateLabel(CreateLabelInput{
		Owner:       "acme",
		Repo:        "orch",
		Name:        "P0",
		Color:       "#D73A4A",
		Description: &labelDescription,
	})
	if err != nil {
		t.Fatalf("CreateLabel() error: %v", err)
	}
	if created == nil {
		t.Fatalf("expected created label payload")
	}
	if created.Name != "P0" || created.Color != "d73a4a" || created.Description != labelDescription {
		t.Fatalf("unexpected created label payload: %#v", created)
	}
	if requestCount != 1 {
		t.Fatalf("expected a single HTTP request, got=%d", requestCount)
	}
}

func TestCreateLabelRESTValidation(t *testing.T) {
	service := NewService(func() (string, error) {
		return "test-token", nil
	})

	assertValidationError := func(err error, expectedMessage string) {
		t.Helper()
		if err == nil {
			t.Fatalf("expected validation error")
		}
		var githubErr *GitHubError
		if !errors.As(err, &githubErr) || githubErr == nil {
			t.Fatalf("expected GitHubError, got=%T (%v)", err, err)
		}
		if githubErr.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("unexpected status code: got=%d want=%d", githubErr.StatusCode, http.StatusUnprocessableEntity)
		}
		if githubErr.Type != "validation" {
			t.Fatalf("unexpected error type: got=%q want=%q", githubErr.Type, "validation")
		}
		if !strings.Contains(githubErr.Message, expectedMessage) {
			t.Fatalf("expected message to contain %q, got=%q", expectedMessage, githubErr.Message)
		}
	}

	_, missingNameErr := service.CreateLabel(CreateLabelInput{
		Owner: "acme",
		Repo:  "orch",
		Name:  "",
		Color: "d73a4a",
	})
	assertValidationError(missingNameErr, "label name")

	_, missingColorErr := service.CreateLabel(CreateLabelInput{
		Owner: "acme",
		Repo:  "orch",
		Name:  "bug",
		Color: "",
	})
	assertValidationError(missingColorErr, "label color")

	_, invalidColorErr := service.CreateLabel(CreateLabelInput{
		Owner: "acme",
		Repo:  "orch",
		Name:  "bug",
		Color: "#zzzzzz",
	})
	assertValidationError(invalidColorErr, "label color")
}

func newPRRESTTestService(t *testing.T, handler http.HandlerFunc) *Service {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	service := NewService(func() (string, error) {
		return "test-token", nil
	})
	service.client = server.Client()
	service.restEndpoint = server.URL

	return service
}
