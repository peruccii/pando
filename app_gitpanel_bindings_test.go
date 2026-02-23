package main

import (
	"testing"

	gp "orch/internal/gitpanel"
)

func TestGitPanelBindingsRequireService(t *testing.T) {
	app := NewApp()

	_, err := app.GitPanelGetStatus("/tmp")
	if err == nil {
		t.Fatalf("expected error when git panel service is not initialized")
	}

	bindingErr := gp.AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected binding error, got: %v", err)
	}
	if bindingErr.Code != gp.CodeServiceUnavailable {
		t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, gp.CodeServiceUnavailable)
	}
}

func TestGitPanelPreflightNormalizesErrors(t *testing.T) {
	app := NewApp()
	app.gitPanel = gp.NewService(nil)

	_, err := app.GitPanelPreflight("   ")
	if err == nil {
		t.Fatalf("expected error for empty repo path")
	}

	bindingErr := gp.AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected binding error, got: %v", err)
	}
	if bindingErr.Code != gp.CodeRepoNotResolved {
		t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, gp.CodeRepoNotResolved)
	}
}

func TestGitPanelOpenExternalMergeToolRequiresService(t *testing.T) {
	app := NewApp()

	err := app.GitPanelOpenExternalMergeTool("/tmp/repo", "conflict.txt")
	if err == nil {
		t.Fatalf("expected error when git panel service is not initialized")
	}

	bindingErr := gp.AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected binding error, got: %v", err)
	}
	if bindingErr.Code != gp.CodeServiceUnavailable {
		t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, gp.CodeServiceUnavailable)
	}
}
