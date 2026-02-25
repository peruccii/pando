package github

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type integrationPRRecord struct {
	Number              int
	Title               string
	Body                string
	State               string
	Draft               bool
	Head                string
	Base                string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	Merged              bool
	MergeSHA            string
	MaintainerCanModify *bool
}

func (pr integrationPRRecord) restPayload() map[string]interface{} {
	payload := map[string]interface{}{
		"node_id":    "PR_" + strconv.Itoa(pr.Number),
		"number":     pr.Number,
		"title":      pr.Title,
		"body":       pr.Body,
		"state":      pr.State,
		"draft":      pr.Draft,
		"created_at": pr.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": pr.UpdatedAt.UTC().Format(time.RFC3339),
		"user": map[string]interface{}{
			"login":      "integration-bot",
			"avatar_url": "https://avatars.example.com/u/1",
		},
		"head": map[string]interface{}{
			"ref": pr.Head,
		},
		"base": map[string]interface{}{
			"ref": pr.Base,
		},
	}
	if pr.MaintainerCanModify != nil {
		payload["maintainer_can_modify"] = *pr.MaintainerCanModify
	}
	if pr.MergeSHA != "" {
		payload["merge_commit_sha"] = pr.MergeSHA
	}
	if pr.Merged {
		payload["merged_at"] = pr.UpdatedAt.UTC().Format(time.RFC3339)
	}

	return payload
}

func TestPRRESTIntegrationReadFlowWithMockGitHubAPI(t *testing.T) {
	var (
		mu              sync.Mutex
		listCalls       int
		detailCalls     int
		diffCalls       int
		commitsCalls    int
		filesCalls      int
		mergeCheckCalls int
	)

	readPR := integrationPRRecord{
		Number:    42,
		Title:     "Read flow integration",
		Body:      "details from mock GitHub",
		State:     "open",
		Draft:     false,
		Head:      "feature/read-flow",
		Base:      "main",
		CreatedAt: time.Date(2026, time.February, 24, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, time.February, 24, 10, 1, 0, 0, time.UTC),
	}

	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/orch/pulls" {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected list method: got=%s want=%s", r.Method, http.MethodGet)
			}
			assertPRRESTStandardHeaders(t, r, githubRESTAcceptJSON, false)
			if got := r.URL.Query().Get("state"); got != "open" {
				t.Fatalf("unexpected list state query: got=%s want=open", got)
			}
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Fatalf("unexpected list page query: got=%s want=1", got)
			}
			if got := r.URL.Query().Get("per_page"); got != "25" {
				t.Fatalf("unexpected list per_page query: got=%s want=25", got)
			}

			mu.Lock()
			listCalls++
			currentCall := listCalls
			mu.Unlock()

			switch currentCall {
			case 1:
				w.Header().Set("ETag", `"prs-read-flow-v1"`)
				writeJSONResponse(t, w, http.StatusOK, []map[string]interface{}{readPR.restPayload()})
				return
			case 2:
				if got := r.Header.Get("If-None-Match"); got != `"prs-read-flow-v1"` {
					t.Fatalf("expected If-None-Match for list revalidation, got=%q", got)
				}
				w.Header().Set("ETag", `"prs-read-flow-v1"`)
				w.WriteHeader(http.StatusNotModified)
				return
			default:
				t.Fatalf("unexpected list call count=%d", currentCall)
			}
		}

		if r.URL.Path == "/repos/acme/orch/pulls/42" {
			accept := strings.TrimSpace(r.Header.Get("Accept"))
			switch accept {
			case githubRESTAcceptJSON:
				if r.Method != http.MethodGet {
					t.Fatalf("unexpected detail method: got=%s want=%s", r.Method, http.MethodGet)
				}
				assertPRRESTStandardHeaders(t, r, githubRESTAcceptJSON, false)
				mu.Lock()
				detailCalls++
				mu.Unlock()
				writeJSONResponse(t, w, http.StatusOK, readPR.restPayload())
				return
			case githubRESTAcceptDiff:
				if r.Method != http.MethodGet {
					t.Fatalf("unexpected raw diff method: got=%s want=%s", r.Method, http.MethodGet)
				}
				assertPRRESTStandardHeaders(t, r, githubRESTAcceptDiff, false)
				mu.Lock()
				diffCalls++
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("diff --git a/README.md b/README.md\n+integration\n"))
				return
			default:
				t.Fatalf("unexpected Accept header for /pulls/42: %q", accept)
			}
		}

		if r.URL.Path == "/repos/acme/orch/pulls/42/commits" {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected commits method: got=%s want=%s", r.Method, http.MethodGet)
			}
			assertPRRESTStandardHeaders(t, r, githubRESTAcceptJSON, false)
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Fatalf("unexpected commits page query: got=%s want=1", got)
			}
			if got := r.URL.Query().Get("per_page"); got != "25" {
				t.Fatalf("unexpected commits per_page query: got=%s want=25", got)
			}
			mu.Lock()
			commitsCalls++
			mu.Unlock()
			writeJSONResponse(t, w, http.StatusOK, []map[string]interface{}{
				{
					"sha":      "a145a1f1ce6aee16de4f9517d2f41295bb12f34",
					"html_url": "https://github.com/acme/orch/commit/a145a1f1ce6aee16de4f9517d2f41295bb12f34",
					"commit": map[string]interface{}{
						"message": "feat: integration commit",
						"author": map[string]interface{}{
							"name":  "ORCH Bot",
							"email": "bot@orch.dev",
							"date":  "2026-02-24T10:00:00Z",
						},
						"committer": map[string]interface{}{
							"name":  "ORCH Bot",
							"email": "bot@orch.dev",
							"date":  "2026-02-24T10:00:01Z",
						},
					},
					"parents": []map[string]interface{}{
						{"sha": "b145a1f1ce6aee16de4f9517d2f41295bb12f35"},
					},
				},
			})
			return
		}

		if r.URL.Path == "/repos/acme/orch/pulls/42/files" {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected files method: got=%s want=%s", r.Method, http.MethodGet)
			}
			assertPRRESTStandardHeaders(t, r, githubRESTAcceptJSON, false)
			if got := r.URL.Query().Get("page"); got != "1" {
				t.Fatalf("unexpected files page query: got=%s want=1", got)
			}
			if got := r.URL.Query().Get("per_page"); got != "25" {
				t.Fatalf("unexpected files per_page query: got=%s want=25", got)
			}
			mu.Lock()
			filesCalls++
			mu.Unlock()
			writeJSONResponse(t, w, http.StatusOK, []map[string]interface{}{
				{
					"filename":  "README.md",
					"status":    "modified",
					"additions": 1,
					"deletions": 0,
					"changes":   1,
					"patch":     "@@ -1 +1 @@\n-old\n+integration",
				},
			})
			return
		}

		if r.URL.Path == "/repos/acme/orch/pulls/42/merge" {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected merge-check method: got=%s want=%s", r.Method, http.MethodGet)
			}
			assertPRRESTStandardHeaders(t, r, githubRESTAcceptJSON, false)
			mu.Lock()
			mergeCheckCalls++
			mu.Unlock()
			writeJSONResponse(t, w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
			return
		}

		t.Fatalf("unexpected request path in read integration test: %s", r.URL.Path)
	})

	firstList, err := service.ListPullRequests("acme", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("first ListPullRequests() error: %v", err)
	}
	if len(firstList) != 1 || firstList[0].Number != 42 {
		t.Fatalf("unexpected first list payload: %+v", firstList)
	}

	listCacheKey := prListKey("acme", "orch", "open", 1, 25)
	service.cache.mu.Lock()
	service.cache.updatedAt[listCacheKey] = time.Now().Add(-time.Minute)
	service.cache.mu.Unlock()

	secondList, err := service.ListPullRequests("acme", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("second ListPullRequests() error: %v", err)
	}
	if len(secondList) != 1 || secondList[0].Number != 42 {
		t.Fatalf("unexpected second list payload after 304 reuse: %+v", secondList)
	}

	// A listagem reaproveita detalhes no cache; invalidamos para forçar GET /pulls/{number}.
	service.cache.InvalidatePRDetail("acme", "orch", 42)

	detail, err := service.GetPullRequest("acme", "orch", 42)
	if err != nil {
		t.Fatalf("GetPullRequest() error: %v", err)
	}
	if detail == nil || detail.Number != 42 || detail.Title != "Read flow integration" {
		t.Fatalf("unexpected detail payload: %+v", detail)
	}

	commitPage, err := service.GetPullRequestCommits("acme", "orch", 42, 1, 25)
	if err != nil {
		t.Fatalf("GetPullRequestCommits() error: %v", err)
	}
	if commitPage == nil || len(commitPage.Items) != 1 {
		t.Fatalf("unexpected commits payload: %+v", commitPage)
	}

	filePage, err := service.GetPullRequestFiles("acme", "orch", 42, 1, 25)
	if err != nil {
		t.Fatalf("GetPullRequestFiles() error: %v", err)
	}
	if filePage == nil || len(filePage.Items) != 1 || !filePage.Items[0].HasPatch {
		t.Fatalf("unexpected files payload: %+v", filePage)
	}

	rawDiff, err := service.GetPullRequestRawDiff("acme", "orch", 42)
	if err != nil {
		t.Fatalf("GetPullRequestRawDiff() error: %v", err)
	}
	if !strings.Contains(rawDiff, "diff --git") {
		t.Fatalf("unexpected raw diff payload: %q", rawDiff)
	}

	merged, err := service.CheckPullRequestMerged("acme", "orch", 42)
	if err != nil {
		t.Fatalf("CheckPullRequestMerged() error: %v", err)
	}
	if merged {
		t.Fatalf("expected merged=false for 404 merge check")
	}

	mu.Lock()
	defer mu.Unlock()
	if listCalls != 2 {
		t.Fatalf("expected list calls=2 (200 + 304), got=%d", listCalls)
	}
	if detailCalls != 1 {
		t.Fatalf("expected detail calls=1, got=%d", detailCalls)
	}
	if commitsCalls != 1 {
		t.Fatalf("expected commits calls=1, got=%d", commitsCalls)
	}
	if filesCalls != 1 {
		t.Fatalf("expected files calls=1, got=%d", filesCalls)
	}
	if diffCalls != 1 {
		t.Fatalf("expected diff calls=1, got=%d", diffCalls)
	}
	if mergeCheckCalls != 1 {
		t.Fatalf("expected merge-check calls=1, got=%d", mergeCheckCalls)
	}
}

func TestPRRESTIntegrationWriteFlowWithMockGitHubAPI(t *testing.T) {
	var (
		mu                      sync.Mutex
		nextPRNumber            = 102
		prs                     = map[int]integrationPRRecord{}
		createPayloadCapture    map[string]interface{}
		updatePayloadCapture    map[string]interface{}
		updateBranchPayload     map[string]interface{}
		mergePayloadCapture     map[string]interface{}
		lastMergedCheckPRNumber int
	)

	initialCanModify := true
	prs[41] = integrationPRRecord{
		Number:              41,
		Title:               "Initial open PR",
		Body:                "baseline",
		State:               "open",
		Draft:               false,
		Head:                "feature/baseline",
		Base:                "main",
		CreatedAt:           time.Date(2026, time.February, 24, 9, 0, 0, 0, time.UTC),
		UpdatedAt:           time.Date(2026, time.February, 24, 9, 30, 0, 0, time.UTC),
		MaintainerCanModify: &initialCanModify,
	}

	service := newPRRESTTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/orch/pulls" {
			switch r.Method {
			case http.MethodGet:
				assertPRRESTStandardHeaders(t, r, githubRESTAcceptJSON, false)
				state := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("state")))
				if state == "" {
					state = "open"
				}
				if got := r.URL.Query().Get("page"); got != "1" {
					t.Fatalf("unexpected list page query: got=%s want=1", got)
				}
				if got := r.URL.Query().Get("per_page"); got != "25" {
					t.Fatalf("unexpected list per_page query: got=%s want=25", got)
				}

				mu.Lock()
				ordered := make([]int, 0, len(prs))
				for number := range prs {
					ordered = append(ordered, number)
				}
				sort.Ints(ordered)
				items := make([]map[string]interface{}, 0, len(ordered))
				for _, number := range ordered {
					pr := prs[number]
					switch state {
					case "open":
						if pr.Merged || strings.EqualFold(pr.State, "closed") {
							continue
						}
					case "closed":
						if !pr.Merged && !strings.EqualFold(pr.State, "closed") {
							continue
						}
					case "all":
					default:
						mu.Unlock()
						t.Fatalf("unexpected list state query received by mock: %q", state)
					}
					items = append(items, pr.restPayload())
				}
				mu.Unlock()
				writeJSONResponse(t, w, http.StatusOK, items)
				return

			case http.MethodPost:
				assertPRRESTStandardHeaders(t, r, githubRESTAcceptJSON, true)
				payload := decodeJSONRequestBodyMap(t, r)

				title, ok := payload["title"].(string)
				if !ok || strings.TrimSpace(title) == "" {
					t.Fatalf("missing create title payload: %+v", payload)
				}
				head, ok := payload["head"].(string)
				if !ok || strings.TrimSpace(head) == "" {
					t.Fatalf("missing create head payload: %+v", payload)
				}
				base, ok := payload["base"].(string)
				if !ok || strings.TrimSpace(base) == "" {
					t.Fatalf("missing create base payload: %+v", payload)
				}
				body, _ := payload["body"].(string)
				draft, _ := payload["draft"].(bool)

				var canModify *bool
				if raw, exists := payload["maintainer_can_modify"]; exists {
					flag, isBool := raw.(bool)
					if !isBool {
						t.Fatalf("invalid maintainer_can_modify payload: %+v", payload)
					}
					canModify = &flag
				}

				mu.Lock()
				createPayloadCapture = payload
				createdAt := time.Date(2026, time.February, 24, 11, 0, 0, 0, time.UTC)
				pr := integrationPRRecord{
					Number:              nextPRNumber,
					Title:               title,
					Body:                body,
					State:               "open",
					Draft:               draft,
					Head:                head,
					Base:                base,
					CreatedAt:           createdAt,
					UpdatedAt:           createdAt,
					MaintainerCanModify: canModify,
				}
				prs[nextPRNumber] = pr
				nextPRNumber++
				mu.Unlock()

				writeJSONResponse(t, w, http.StatusCreated, pr.restPayload())
				return
			}
		}

		if strings.HasPrefix(r.URL.Path, "/repos/acme/orch/pulls/") {
			tail := strings.TrimPrefix(r.URL.Path, "/repos/acme/orch/pulls/")
			parts := strings.Split(tail, "/")
			if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
				t.Fatalf("invalid pull request path tail: %q", tail)
			}
			number, err := strconv.Atoi(parts[0])
			if err != nil {
				t.Fatalf("invalid pull request number in path %q: %v", r.URL.Path, err)
			}
			subPath := ""
			if len(parts) > 1 {
				subPath = strings.Join(parts[1:], "/")
			}

			switch subPath {
			case "":
				switch r.Method {
				case http.MethodGet:
					accept := strings.TrimSpace(r.Header.Get("Accept"))
					if accept == githubRESTAcceptDiff {
						assertPRRESTStandardHeaders(t, r, githubRESTAcceptDiff, false)
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte("diff --git a/file.txt b/file.txt\n+merged\n"))
						return
					}
					assertPRRESTStandardHeaders(t, r, githubRESTAcceptJSON, false)
					mu.Lock()
					pr, exists := prs[number]
					mu.Unlock()
					if !exists {
						writeJSONResponse(t, w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
						return
					}
					writeJSONResponse(t, w, http.StatusOK, pr.restPayload())
					return

				case http.MethodPatch:
					assertPRRESTStandardHeaders(t, r, githubRESTAcceptJSON, true)
					payload := decodeJSONRequestBodyMap(t, r)

					mu.Lock()
					pr, exists := prs[number]
					if !exists {
						mu.Unlock()
						writeJSONResponse(t, w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
						return
					}
					updatePayloadCapture = payload
					if rawTitle, hasTitle := payload["title"]; hasTitle {
						pr.Title = strings.TrimSpace(rawTitle.(string))
					}
					if rawBody, hasBody := payload["body"]; hasBody {
						pr.Body = rawBody.(string)
					}
					if rawState, hasState := payload["state"]; hasState {
						pr.State = strings.TrimSpace(rawState.(string))
					}
					if rawBase, hasBase := payload["base"]; hasBase {
						pr.Base = strings.TrimSpace(rawBase.(string))
					}
					if rawCanModify, hasCanModify := payload["maintainer_can_modify"]; hasCanModify {
						flag := rawCanModify.(bool)
						pr.MaintainerCanModify = &flag
					}
					pr.UpdatedAt = time.Date(2026, time.February, 24, 11, 15, 0, 0, time.UTC)
					prs[number] = pr
					mu.Unlock()

					writeJSONResponse(t, w, http.StatusOK, pr.restPayload())
					return
				}

			case "merge":
				switch r.Method {
				case http.MethodGet:
					assertPRRESTStandardHeaders(t, r, githubRESTAcceptJSON, false)
					mu.Lock()
					pr, exists := prs[number]
					lastMergedCheckPRNumber = number
					mu.Unlock()
					if !exists {
						writeJSONResponse(t, w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
						return
					}
					if pr.Merged {
						w.WriteHeader(http.StatusNoContent)
						return
					}
					writeJSONResponse(t, w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
					return

				case http.MethodPut:
					assertPRRESTStandardHeaders(t, r, githubRESTAcceptJSON, true)
					payload := decodeJSONRequestBodyMap(t, r)

					mu.Lock()
					pr, exists := prs[number]
					if !exists {
						mu.Unlock()
						writeJSONResponse(t, w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
						return
					}
					mergePayloadCapture = payload
					if rawSHA, hasSHA := payload["sha"]; hasSHA {
						pr.MergeSHA = strings.TrimSpace(rawSHA.(string))
					}
					pr.Merged = true
					pr.State = "closed"
					pr.UpdatedAt = time.Date(2026, time.February, 24, 11, 30, 0, 0, time.UTC)
					prs[number] = pr
					mu.Unlock()

					writeJSONResponse(t, w, http.StatusOK, map[string]interface{}{
						"sha":     pr.MergeSHA,
						"merged":  true,
						"message": "Pull Request successfully merged",
					})
					return
				}

			case "update-branch":
				if r.Method != http.MethodPut {
					t.Fatalf("unexpected update-branch method: got=%s want=%s", r.Method, http.MethodPut)
				}
				assertPRRESTStandardHeaders(t, r, githubRESTAcceptJSON, true)
				payload := decodeJSONRequestBodyMap(t, r)

				mu.Lock()
				_, exists := prs[number]
				updateBranchPayload = payload
				mu.Unlock()
				if !exists {
					writeJSONResponse(t, w, http.StatusNotFound, map[string]interface{}{"message": "Not Found"})
					return
				}
				writeJSONResponse(t, w, http.StatusAccepted, map[string]interface{}{"message": "Updating pull request branch."})
				return
			}
		}

		t.Fatalf("unexpected request in write integration test: method=%s path=%s", r.Method, r.URL.Path)
	})

	openBeforeCreate, err := service.ListPullRequests("acme", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("initial ListPullRequests() error: %v", err)
	}
	if len(openBeforeCreate) != 1 || openBeforeCreate[0].Number != 41 {
		t.Fatalf("unexpected initial open list: %+v", openBeforeCreate)
	}

	canModifyOnCreate := true
	created, err := service.CreatePullRequest(CreatePRInput{
		Owner:               "acme",
		Repo:                "orch",
		Title:               "Create integration PR",
		Body:                "created in integration flow",
		HeadBranch:          "feature/integration",
		BaseBranch:          "main",
		IsDraft:             true,
		MaintainerCanModify: &canModifyOnCreate,
	})
	if err != nil {
		t.Fatalf("CreatePullRequest() error: %v", err)
	}
	if created == nil || created.Number != 102 || !created.IsDraft {
		t.Fatalf("unexpected created PR payload: %+v", created)
	}

	openAfterCreate, err := service.ListPullRequests("acme", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("ListPullRequests() after create error: %v", err)
	}
	if len(openAfterCreate) != 2 || !prListHasNumber(openAfterCreate, 102) {
		t.Fatalf("expected created PR in open list, got=%+v", openAfterCreate)
	}

	updateTitle := "Create integration PR v2"
	updateBody := "updated body"
	updateState := "OPEN"
	updateBase := "release/v2"
	canModifyOnUpdate := false
	updated, err := service.UpdatePullRequest(UpdatePRInput{
		Owner:               "acme",
		Repo:                "orch",
		Number:              102,
		Title:               &updateTitle,
		Body:                &updateBody,
		State:               &updateState,
		BaseBranch:          &updateBase,
		MaintainerCanModify: &canModifyOnUpdate,
	})
	if err != nil {
		t.Fatalf("UpdatePullRequest() error: %v", err)
	}
	if updated == nil || updated.Title != "Create integration PR v2" || updated.BaseBranch != "release/v2" {
		t.Fatalf("unexpected updated PR payload: %+v", updated)
	}

	mergedBeforeMerge, err := service.CheckPullRequestMerged("acme", "orch", 102)
	if err != nil {
		t.Fatalf("CheckPullRequestMerged() before merge error: %v", err)
	}
	if mergedBeforeMerge {
		t.Fatalf("expected merged=false before merge")
	}

	expectedHeadSHA := "A145A1F1CE6AEE16DE4F9517D2F41295BB12F341"
	updateBranchResult, err := service.UpdatePullRequestBranch(UpdatePRBranchInput{
		Owner:           "acme",
		Repo:            "orch",
		Number:          102,
		ExpectedHeadSHA: &expectedHeadSHA,
	})
	if err != nil {
		t.Fatalf("UpdatePullRequestBranch() error: %v", err)
	}
	if updateBranchResult == nil || updateBranchResult.Message != "Updating pull request branch." {
		t.Fatalf("unexpected update-branch payload: %+v", updateBranchResult)
	}

	mergeSHA := "B145A1F1CE6AEE16DE4F9517D2F41295BB12F342"
	mergeResult, err := service.MergePullRequestREST(MergePRInput{
		Owner:       "acme",
		Repo:        "orch",
		Number:      102,
		MergeMethod: PRMergeMethodSquash,
		SHA:         &mergeSHA,
	})
	if err != nil {
		t.Fatalf("MergePullRequestREST() error: %v", err)
	}
	if mergeResult == nil || !mergeResult.Merged {
		t.Fatalf("unexpected merge result payload: %+v", mergeResult)
	}

	mergedAfterMerge, err := service.CheckPullRequestMerged("acme", "orch", 102)
	if err != nil {
		t.Fatalf("CheckPullRequestMerged() after merge error: %v", err)
	}
	if !mergedAfterMerge {
		t.Fatalf("expected merged=true after merge")
	}

	openAfterMerge, err := service.ListPullRequests("acme", "orch", PRFilters{
		State:   "open",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("ListPullRequests() after merge error: %v", err)
	}
	if prListHasNumber(openAfterMerge, 102) {
		t.Fatalf("merged PR should not remain in open list: %+v", openAfterMerge)
	}

	closedAfterMerge, err := service.ListPullRequests("acme", "orch", PRFilters{
		State:   "closed",
		Page:    1,
		PerPage: 25,
	})
	if err != nil {
		t.Fatalf("ListPullRequests(closed) after merge error: %v", err)
	}
	if !prListHasNumber(closedAfterMerge, 102) {
		t.Fatalf("expected merged PR in closed list: %+v", closedAfterMerge)
	}
	mergedClosedPR := prListFindByNumber(closedAfterMerge, 102)
	if mergedClosedPR == nil || mergedClosedPR.State != "MERGED" {
		t.Fatalf("expected merged closed PR state=MERGED, got=%+v", mergedClosedPR)
	}

	mu.Lock()
	defer mu.Unlock()
	if createPayloadCapture == nil {
		t.Fatalf("expected captured create payload")
	}
	if createPayloadCapture["title"] != "Create integration PR" {
		t.Fatalf("unexpected captured create title: %+v", createPayloadCapture["title"])
	}
	if createPayloadCapture["head"] != "feature/integration" {
		t.Fatalf("unexpected captured create head: %+v", createPayloadCapture["head"])
	}
	if createPayloadCapture["base"] != "main" {
		t.Fatalf("unexpected captured create base: %+v", createPayloadCapture["base"])
	}
	if createPayloadCapture["draft"] != true {
		t.Fatalf("unexpected captured create draft: %+v", createPayloadCapture["draft"])
	}
	if createPayloadCapture["maintainer_can_modify"] != true {
		t.Fatalf("unexpected captured create maintainer_can_modify: %+v", createPayloadCapture["maintainer_can_modify"])
	}

	if updatePayloadCapture == nil {
		t.Fatalf("expected captured update payload")
	}
	if updatePayloadCapture["state"] != "open" {
		t.Fatalf("expected normalized update state=open, got=%+v", updatePayloadCapture["state"])
	}
	if updatePayloadCapture["base"] != "release/v2" {
		t.Fatalf("unexpected captured update base: %+v", updatePayloadCapture["base"])
	}
	if updatePayloadCapture["maintainer_can_modify"] != false {
		t.Fatalf("unexpected captured update maintainer_can_modify: %+v", updatePayloadCapture["maintainer_can_modify"])
	}

	if updateBranchPayload == nil {
		t.Fatalf("expected captured update-branch payload")
	}
	if updateBranchPayload["expected_head_sha"] != strings.ToLower(expectedHeadSHA) {
		t.Fatalf("expected normalized expected_head_sha, got=%+v", updateBranchPayload["expected_head_sha"])
	}

	if mergePayloadCapture == nil {
		t.Fatalf("expected captured merge payload")
	}
	if mergePayloadCapture["merge_method"] != "squash" {
		t.Fatalf("unexpected captured merge method: %+v", mergePayloadCapture["merge_method"])
	}
	if mergePayloadCapture["sha"] != strings.ToLower(mergeSHA) {
		t.Fatalf("expected normalized merge sha, got=%+v", mergePayloadCapture["sha"])
	}

	if lastMergedCheckPRNumber != 102 {
		t.Fatalf("expected merge-check to target PR #102, got=%d", lastMergedCheckPRNumber)
	}
}

func assertPRRESTStandardHeaders(t *testing.T, r *http.Request, wantAccept string, expectJSONBody bool) {
	t.Helper()

	if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("unexpected authorization header: got=%q want=%q", got, "Bearer test-token")
	}
	if got := r.Header.Get("X-GitHub-Api-Version"); got != githubRESTAPIVersion {
		t.Fatalf("unexpected api version header: got=%q want=%q", got, githubRESTAPIVersion)
	}
	if got := r.Header.Get("Accept"); got != wantAccept {
		t.Fatalf("unexpected accept header: got=%q want=%q", got, wantAccept)
	}

	gotContentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if expectJSONBody {
		if gotContentType != "application/json" {
			t.Fatalf("unexpected content type header: got=%q want=%q", gotContentType, "application/json")
		}
		return
	}

	if gotContentType != "" {
		t.Fatalf("expected empty content type for request without body, got=%q", gotContentType)
	}
}

func decodeJSONRequestBodyMap(t *testing.T, r *http.Request) map[string]interface{} {
	t.Helper()

	rawBody, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		t.Fatalf("failed to read request body: %v", readErr)
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		t.Fatalf("failed to parse JSON request body: %v raw=%q", err, string(rawBody))
	}
	return payload
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, statusCode int, payload interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("failed to write json response: %v", err)
	}
}

func prListHasNumber(items []PullRequest, number int) bool {
	return prListFindByNumber(items, number) != nil
}

func prListFindByNumber(items []PullRequest, number int) *PullRequest {
	for i := range items {
		if items[i].Number == number {
			return &items[i]
		}
	}
	return nil
}
