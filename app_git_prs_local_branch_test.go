package main

import (
	"os/exec"
	"strings"
	"testing"

	gp "orch/internal/gitpanel"
	gpr "orch/internal/gitprs"
)

func TestGitPanelPRCreateLocalBranchRequiresGitPanelService(t *testing.T) {
	app := NewApp()

	err := app.GitPanelPRCreateLocalBranch("/tmp/repo", "feature/local-head", "")
	if err == nil {
		t.Fatalf("expected service error when git panel service is not initialized")
	}

	bindingErr := gpr.AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected PR binding error, got=%v", err)
	}
	if bindingErr.Code != gpr.CodeServiceUnavailable {
		t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, gpr.CodeServiceUnavailable)
	}
}

func TestGitPanelPRCreateLocalBranchValidatesBranchName(t *testing.T) {
	repoRoot := mustInitPRResolveTestRepo(t, "")
	app := NewApp()
	app.gitPanel = gp.NewService(nil)

	err := app.GitPanelPRCreateLocalBranch(repoRoot, "feature invalid", "")
	if err == nil {
		t.Fatalf("expected validation error for invalid branch name")
	}

	bindingErr := gpr.AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected PR binding error, got=%v", err)
	}
	if bindingErr.Code != gpr.CodeValidationFailed {
		t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, gpr.CodeValidationFailed)
	}
}

func TestGitPanelPRCreateLocalBranchCreatesAndChecksOutBranch(t *testing.T) {
	repoRoot := mustInitPRResolveTestRepo(t, "")
	app := NewApp()
	app.gitPanel = gp.NewService(nil)

	if err := app.GitPanelPRCreateLocalBranch(repoRoot, "feature/local-head", ""); err != nil {
		t.Fatalf("GitPanelPRCreateLocalBranch() error: %v", err)
	}

	currentBranch := runGitOutputOrFailForPRLocalBranch(t, repoRoot, "branch", "--show-current")
	if currentBranch != "feature/local-head" {
		t.Fatalf("unexpected current branch: got=%q want=%q", currentBranch, "feature/local-head")
	}
}

func TestGitPanelPRPushLocalBranchRequiresGitPanelService(t *testing.T) {
	app := NewApp()

	err := app.GitPanelPRPushLocalBranch("/tmp/repo", "feature/local-head")
	if err == nil {
		t.Fatalf("expected service error when git panel service is not initialized")
	}

	bindingErr := gpr.AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected PR binding error, got=%v", err)
	}
	if bindingErr.Code != gpr.CodeServiceUnavailable {
		t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, gpr.CodeServiceUnavailable)
	}
}

func TestGitPanelPRPushLocalBranchPublishesToOrigin(t *testing.T) {
	repoRoot := mustInitPRResolveTestRepo(t, "")
	bareRemote := t.TempDir()
	runGitOrFailForPRResolve(t, bareRemote, "init", "--bare")
	runGitOrFailForPRResolve(t, repoRoot, "remote", "add", "origin", bareRemote)

	app := NewApp()
	app.gitPanel = gp.NewService(nil)

	if err := app.GitPanelPRCreateLocalBranch(repoRoot, "feature/publish", ""); err != nil {
		t.Fatalf("GitPanelPRCreateLocalBranch() error: %v", err)
	}
	if err := app.GitPanelPRPushLocalBranch(repoRoot, "feature/publish"); err != nil {
		t.Fatalf("GitPanelPRPushLocalBranch() error: %v", err)
	}

	remoteBranch := runGitOutputOrFailForPRLocalBranch(
		t,
		repoRoot,
		"ls-remote",
		"--heads",
		"origin",
		"feature/publish",
	)
	if !strings.Contains(remoteBranch, "refs/heads/feature/publish") {
		t.Fatalf("expected branch published to origin, got=%q", remoteBranch)
	}
}

func runGitOutputOrFailForPRLocalBranch(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()

	allArgs := append([]string{"-C", repoRoot}, args...)
	cmd := exec.Command("git", allArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v output=%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output))
}
