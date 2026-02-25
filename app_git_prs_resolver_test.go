package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gp "orch/internal/gitpanel"
	gpr "orch/internal/gitprs"
)

func TestGitPanelPRResolveRepositoryRequiresService(t *testing.T) {
	app := NewApp()

	_, err := app.GitPanelPRResolveRepository("/tmp/repo", "", "", false)
	if err == nil {
		t.Fatalf("expected error when git panel service is not initialized")
	}

	bindingErr := gpr.AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected PR binding error, got: %v", err)
	}
	if bindingErr.Code != gpr.CodeServiceUnavailable {
		t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, gpr.CodeServiceUnavailable)
	}
}

func TestGitPanelPRResolveRepositoryFromOrigin(t *testing.T) {
	repoRoot := mustInitPRResolveTestRepo(t, "git@github.com:orch-labs/orch.git")
	app := NewApp()
	app.gitPanel = gp.NewService(nil)

	target, err := app.GitPanelPRResolveRepository(repoRoot, "", "", false)
	if err != nil {
		t.Fatalf("GitPanelPRResolveRepository() error: %v", err)
	}
	if target.Owner != "orch-labs" || target.Repo != "orch" {
		t.Fatalf("unexpected owner/repo: got=%s/%s", target.Owner, target.Repo)
	}
	if target.Source != "origin" {
		t.Fatalf("expected source=origin, got=%q", target.Source)
	}
}

func TestGitPanelPRResolveRepositoryManualFallback(t *testing.T) {
	repoRoot := mustInitPRResolveTestRepo(t, "")
	app := NewApp()
	app.gitPanel = gp.NewService(nil)

	target, err := app.GitPanelPRResolveRepository(repoRoot, "orch-labs", "orch", false)
	if err != nil {
		t.Fatalf("GitPanelPRResolveRepository() error: %v", err)
	}
	if target.Owner != "orch-labs" || target.Repo != "orch" {
		t.Fatalf("unexpected owner/repo: got=%s/%s", target.Owner, target.Repo)
	}
	if target.Source != "manual" {
		t.Fatalf("expected source=manual, got=%q", target.Source)
	}
}

func TestGitPanelPRResolveRepositoryRejectsDivergentManualWithoutOverride(t *testing.T) {
	repoRoot := mustInitPRResolveTestRepo(t, "https://github.com/orch-labs/orch.git")
	app := NewApp()
	app.gitPanel = gp.NewService(nil)

	_, err := app.GitPanelPRResolveRepository(repoRoot, "another-org", "another-repo", false)
	if err == nil {
		t.Fatalf("expected mismatch error")
	}

	bindingErr := gpr.AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected PR binding error, got=%v", err)
	}
	if bindingErr.Code != gpr.CodeRepoTargetMismatch {
		t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, gpr.CodeRepoTargetMismatch)
	}
}

func TestGitPanelPRResolveRepositoryAllowsManualOverrideWhenConfirmed(t *testing.T) {
	repoRoot := mustInitPRResolveTestRepo(t, "https://github.com/orch-labs/orch.git")
	app := NewApp()
	app.gitPanel = gp.NewService(nil)

	target, err := app.GitPanelPRResolveRepository(repoRoot, "another-org", "another-repo", true)
	if err != nil {
		t.Fatalf("GitPanelPRResolveRepository() error: %v", err)
	}
	if target.Owner != "another-org" || target.Repo != "another-repo" {
		t.Fatalf("expected manual owner/repo, got=%s/%s", target.Owner, target.Repo)
	}
	if target.Source != "manual" {
		t.Fatalf("expected source=manual, got=%q", target.Source)
	}
	if !target.OverrideConfirmed {
		t.Fatalf("expected override confirmed flag")
	}
}

func TestGitPanelPRResolveRepositoryRequiresManualWhenOriginCannotBeResolved(t *testing.T) {
	repoRoot := mustInitPRResolveTestRepo(t, "")
	app := NewApp()
	app.gitPanel = gp.NewService(nil)

	_, err := app.GitPanelPRResolveRepository(repoRoot, "", "", false)
	if err == nil {
		t.Fatalf("expected resolve failure")
	}

	bindingErr := gpr.AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected PR binding error, got=%v", err)
	}
	if bindingErr.Code != gpr.CodeRepoResolveFailed {
		t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, gpr.CodeRepoResolveFailed)
	}
}

func mustInitPRResolveTestRepo(t *testing.T, originURL string) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in test environment")
	}

	repoRoot := t.TempDir()
	runGitOrFailForPRResolve(t, repoRoot, "init")
	runGitOrFailForPRResolve(t, repoRoot, "config", "user.email", "tests@orch.local")
	runGitOrFailForPRResolve(t, repoRoot, "config", "user.name", "ORCH Tests")

	readmePath := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}
	runGitOrFailForPRResolve(t, repoRoot, "add", "--", "README.md")
	runGitOrFailForPRResolve(t, repoRoot, "commit", "-m", "initial commit")

	normalizedOrigin := strings.TrimSpace(originURL)
	if normalizedOrigin != "" {
		runGitOrFailForPRResolve(t, repoRoot, "remote", "add", "origin", normalizedOrigin)
	}

	return repoRoot
}

func runGitOrFailForPRResolve(t *testing.T, repoRoot string, args ...string) {
	t.Helper()

	allArgs := append([]string{"-C", repoRoot}, args...)
	cmd := exec.Command("git", allArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v output=%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
}
