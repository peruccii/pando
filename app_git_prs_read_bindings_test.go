package main

import (
	"strings"
	"testing"

	gh "orch/internal/github"
	gp "orch/internal/gitpanel"
	gpr "orch/internal/gitprs"
)

func TestGitPanelPRReadBindingsRequireGitHubService(t *testing.T) {
	app := NewApp()
	app.gitPanel = gp.NewService(nil)
	updateTitle := "Update via ORCH"

	_, listErr := app.GitPanelPRList("/tmp/repo", "open", 1, 20)
	_, createErr := app.GitPanelPRCreate("/tmp/repo", GitPanelPRCreatePayloadDTO{
		Title: "Create from ORCH",
		Head:  "feature/rest-create",
		Base:  "main",
	})
	_, createLabelErr := app.GitPanelPRCreateLabel("/tmp/repo", GitPanelPRCreateLabelPayloadDTO{
		Name:  "bug",
		Color: "#d73a4a",
	})
	_, updateErr := app.GitPanelPRUpdate("/tmp/repo", 1, GitPanelPRUpdatePayloadDTO{
		Title: &updateTitle,
	})
	_, checkMergedErr := app.GitPanelPRCheckMerged("/tmp/repo", 1)
	_, mergeErr := app.GitPanelPRMerge("/tmp/repo", 1, GitPanelPRMergePayloadDTO{MergeMethod: "merge"})
	_, updateBranchErr := app.GitPanelPRUpdateBranch("/tmp/repo", 1, GitPanelPRUpdateBranchPayloadDTO{})
	_, getErr := app.GitPanelPRGet("/tmp/repo", 1)
	_, commitsErr := app.GitPanelPRGetCommits("/tmp/repo", 1, 1, 20)

	for _, err := range []error{
		listErr,
		createErr,
		createLabelErr,
		updateErr,
		checkMergedErr,
		mergeErr,
		updateBranchErr,
		getErr,
		commitsErr,
	} {
		if err == nil {
			t.Fatalf("expected service error when github service is not initialized")
		}

		bindingErr := gpr.AsBindingError(err)
		if bindingErr == nil {
			t.Fatalf("expected PR binding error, got=%v", err)
		}
		if bindingErr.Code != gpr.CodeServiceUnavailable {
			t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, gpr.CodeServiceUnavailable)
		}
	}
}

func TestGitPanelPRReadBindingsValidateInput(t *testing.T) {
	app := NewApp()

	_, createTitleErr := app.GitPanelPRCreate("/tmp/repo", GitPanelPRCreatePayloadDTO{
		Title: "",
		Head:  "feature/rest-create",
		Base:  "main",
	})
	if createTitleErr == nil {
		t.Fatalf("expected validation error for missing PR title")
	}

	_, createHeadErr := app.GitPanelPRCreate("/tmp/repo", GitPanelPRCreatePayloadDTO{
		Title: "Create from ORCH",
		Head:  "",
		Base:  "main",
	})
	if createHeadErr == nil {
		t.Fatalf("expected validation error for missing PR head")
	}

	_, createBaseErr := app.GitPanelPRCreate("/tmp/repo", GitPanelPRCreatePayloadDTO{
		Title: "Create from ORCH",
		Head:  "feature/rest-create",
		Base:  "",
	})
	if createBaseErr == nil {
		t.Fatalf("expected validation error for missing PR base")
	}

	_, createLabelNameErr := app.GitPanelPRCreateLabel("/tmp/repo", GitPanelPRCreateLabelPayloadDTO{
		Name:  "",
		Color: "#0e8a16",
	})
	if createLabelNameErr == nil {
		t.Fatalf("expected validation error for missing label name")
	}

	_, createLabelColorErr := app.GitPanelPRCreateLabel("/tmp/repo", GitPanelPRCreateLabelPayloadDTO{
		Name:  "bug",
		Color: "",
	})
	if createLabelColorErr == nil {
		t.Fatalf("expected validation error for missing label color")
	}

	_, createLabelInvalidColorErr := app.GitPanelPRCreateLabel("/tmp/repo", GitPanelPRCreateLabelPayloadDTO{
		Name:  "bug",
		Color: "#zz99zz",
	})
	if createLabelInvalidColorErr == nil {
		t.Fatalf("expected validation error for invalid label color")
	}

	_, createManualMissingRepoErr := app.GitPanelPRCreate("/tmp/repo", GitPanelPRCreatePayloadDTO{
		Title:       "Create from ORCH",
		Head:        "feature/rest-create",
		Base:        "main",
		ManualOwner: "orch-labs",
	})
	if createManualMissingRepoErr == nil {
		t.Fatalf("expected validation error for incomplete manual owner/repo override")
	}

	_, createManualInvalidOwnerErr := app.GitPanelPRCreate("/tmp/repo", GitPanelPRCreatePayloadDTO{
		Title:       "Create from ORCH",
		Head:        "feature/rest-create",
		Base:        "main",
		ManualOwner: "orch labs",
		ManualRepo:  "orch",
	})
	if createManualInvalidOwnerErr == nil {
		t.Fatalf("expected validation error for invalid manual owner")
	}
	createManualInvalidOwnerBinding := gpr.AsBindingError(createManualInvalidOwnerErr)
	if createManualInvalidOwnerBinding == nil {
		t.Fatalf("expected PR binding error for invalid manual owner, got=%v", createManualInvalidOwnerErr)
	}
	if createManualInvalidOwnerBinding.Code != gpr.CodeManualRepoInvalid {
		t.Fatalf("unexpected error code for invalid manual owner: got=%s want=%s", createManualInvalidOwnerBinding.Code, gpr.CodeManualRepoInvalid)
	}

	blank := "   "
	invalidState := "invalid"
	_, updatePayloadErr := app.GitPanelPRUpdate("/tmp/repo", 1, GitPanelPRUpdatePayloadDTO{})
	if updatePayloadErr == nil {
		t.Fatalf("expected validation error for empty update payload")
	}

	_, updateTitleErr := app.GitPanelPRUpdate("/tmp/repo", 1, GitPanelPRUpdatePayloadDTO{
		Title: &blank,
	})
	if updateTitleErr == nil {
		t.Fatalf("expected validation error for empty update title")
	}

	_, updateStateErr := app.GitPanelPRUpdate("/tmp/repo", 1, GitPanelPRUpdatePayloadDTO{
		State: &invalidState,
	})
	if updateStateErr == nil {
		t.Fatalf("expected validation error for invalid update state")
	}

	_, updateBaseErr := app.GitPanelPRUpdate("/tmp/repo", 1, GitPanelPRUpdatePayloadDTO{
		Base: &blank,
	})
	if updateBaseErr == nil {
		t.Fatalf("expected validation error for empty update base")
	}

	updateBody := "update-body"
	_, updateNumberErr := app.GitPanelPRUpdate("/tmp/repo", 0, GitPanelPRUpdatePayloadDTO{
		Body: &updateBody,
	})
	if updateNumberErr == nil {
		t.Fatalf("expected validation error for invalid PR number on update")
	}

	_, checkMergedErr := app.GitPanelPRCheckMerged("/tmp/repo", 0)
	if checkMergedErr == nil {
		t.Fatalf("expected validation error for invalid PR number on merge check")
	}

	invalidMergeMethod := "fast-forward"
	_, mergeMethodErr := app.GitPanelPRMerge("/tmp/repo", 1, GitPanelPRMergePayloadDTO{
		MergeMethod: invalidMergeMethod,
	})
	if mergeMethodErr == nil {
		t.Fatalf("expected validation error for invalid merge method")
	}

	shortSHA := "abc123"
	_, mergeSHAErr := app.GitPanelPRMerge("/tmp/repo", 1, GitPanelPRMergePayloadDTO{
		MergeMethod: "merge",
		SHA:         &shortSHA,
	})
	if mergeSHAErr == nil {
		t.Fatalf("expected validation error for invalid merge sha")
	}

	_, mergeNumberErr := app.GitPanelPRMerge("/tmp/repo", 0, GitPanelPRMergePayloadDTO{
		MergeMethod: "merge",
	})
	if mergeNumberErr == nil {
		t.Fatalf("expected validation error for invalid PR number on merge")
	}

	_, updateBranchNumberErr := app.GitPanelPRUpdateBranch("/tmp/repo", 0, GitPanelPRUpdateBranchPayloadDTO{})
	if updateBranchNumberErr == nil {
		t.Fatalf("expected validation error for invalid PR number on update-branch")
	}

	invalidExpectedHeadSHA := "def456"
	_, updateBranchSHAErr := app.GitPanelPRUpdateBranch("/tmp/repo", 1, GitPanelPRUpdateBranchPayloadDTO{
		ExpectedHeadSHA: &invalidExpectedHeadSHA,
	})
	if updateBranchSHAErr == nil {
		t.Fatalf("expected validation error for invalid expectedHeadSha")
	}

	_, getErr := app.GitPanelPRGet("/tmp/repo", 0)
	if getErr == nil {
		t.Fatalf("expected validation error for invalid PR number")
	}

	_, commitsErr := app.GitPanelPRGetCommits("/tmp/repo", 0, 1, 20)
	if commitsErr == nil {
		t.Fatalf("expected validation error for invalid PR number")
	}

	_, filesErr := app.GitPanelPRGetFiles("/tmp/repo", -1, 1, 20)
	if filesErr == nil {
		t.Fatalf("expected validation error for invalid PR number")
	}

	_, rawDiffErr := app.GitPanelPRGetRawDiff("/tmp/repo", 0)
	if rawDiffErr == nil {
		t.Fatalf("expected validation error for invalid PR number")
	}

	_, listStateErr := app.GitPanelPRList("/tmp/repo", "invalid-state", 1, 20)
	if listStateErr == nil {
		t.Fatalf("expected validation error for invalid PR state")
	}

	for _, err := range []error{
		createTitleErr,
		createHeadErr,
		createBaseErr,
		createLabelNameErr,
		createLabelColorErr,
		createLabelInvalidColorErr,
		createManualMissingRepoErr,
		updatePayloadErr,
		updateTitleErr,
		updateStateErr,
		updateBaseErr,
		updateNumberErr,
		checkMergedErr,
		mergeMethodErr,
		mergeSHAErr,
		mergeNumberErr,
		updateBranchNumberErr,
		updateBranchSHAErr,
		getErr,
		commitsErr,
		filesErr,
		rawDiffErr,
		listStateErr,
	} {
		bindingErr := gpr.AsBindingError(err)
		if bindingErr == nil {
			t.Fatalf("expected PR binding error, got=%v", err)
		}
		if bindingErr.Code != gpr.CodeValidationFailed {
			t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, gpr.CodeValidationFailed)
		}
	}
}

func TestNormalizeGitPanelPRErrorMapsGitHubHTTPStatus(t *testing.T) {
	app := NewApp()

	cases := []struct {
		name       string
		statusCode int
		detail     string
		ghType     string
		wantCode   string
	}{
		{
			name:       "unauthorized",
			statusCode: 401,
			detail:     "GitHub token expired or invalid",
			ghType:     "auth",
			wantCode:   gpr.CodeUnauthorized,
		},
		{
			name:       "forbidden",
			statusCode: 403,
			detail:     "Permission denied",
			ghType:     "permission",
			wantCode:   gpr.CodeForbidden,
		},
		{
			name:       "not-found",
			statusCode: 404,
			detail:     "Resource not found",
			ghType:     "notfound",
			wantCode:   gpr.CodeNotFound,
		},
		{
			name:       "conflict",
			statusCode: 409,
			detail:     "Merge conflict detected",
			ghType:     "conflict",
			wantCode:   gpr.CodeConflict,
		},
		{
			name:       "validation",
			statusCode: 422,
			detail:     "Validation failed",
			ghType:     "validation",
			wantCode:   gpr.CodeValidationFailed,
		},
		{
			name:       "rate-limit",
			statusCode: 429,
			detail:     "Too many requests",
			ghType:     "ratelimit",
			wantCode:   gpr.CodeRateLimited,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := app.normalizeGitPanelPRError(&gh.GitHubError{
				StatusCode: tc.statusCode,
				Message:    tc.detail,
				Type:       tc.ghType,
			})
			if err == nil {
				t.Fatalf("expected normalized error")
			}

			bindingErr := gpr.AsBindingError(err)
			if bindingErr == nil {
				t.Fatalf("expected binding error, got=%v", err)
			}
			if bindingErr.Code != tc.wantCode {
				t.Fatalf("unexpected code: got=%s want=%s", bindingErr.Code, tc.wantCode)
			}
			if !strings.Contains(bindingErr.Details, tc.detail) {
				t.Fatalf("expected details to include upstream message, got=%q", bindingErr.Details)
			}
		})
	}
}
