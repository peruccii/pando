package gitpanel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPreflightRequiresRepoPath(t *testing.T) {
	svc := NewService(nil)

	_, err := svc.Preflight("   ")
	if err == nil {
		t.Fatalf("expected error for empty repo path")
	}

	bindingErr := AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected binding error, got: %v", err)
	}
	if bindingErr.Code != CodeRepoNotResolved {
		t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, CodeRepoNotResolved)
	}
}

func TestPreflightAndStatus(t *testing.T) {
	repoRoot := mustInitTestRepo(t)
	svc := NewService(nil)

	preflight, err := svc.Preflight(repoRoot)
	if err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
	if !preflight.GitAvailable {
		t.Fatalf("expected git to be available")
	}
	if strings.TrimSpace(preflight.RepoRoot) == "" {
		t.Fatalf("expected repo root to be resolved")
	}

	filePath := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(filePath, []byte("modified\n"), 0o644); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	status, err := svc.GetStatus(repoRoot)
	if err != nil {
		t.Fatalf("GetStatus returned error: %v", err)
	}
	if len(status.Unstaged) == 0 {
		t.Fatalf("expected unstaged changes after file modification")
	}
}

func TestStageFileRejectsTraversalPath(t *testing.T) {
	repoRoot := mustInitTestRepo(t)
	svc := NewService(nil)

	err := svc.StageFile(repoRoot, "../outside.txt")
	if err == nil {
		t.Fatalf("expected error for traversal path")
	}

	bindingErr := AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected binding error, got: %v", err)
	}
	if bindingErr.Code != CodeInvalidPath {
		t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, CodeInvalidPath)
	}
}

func TestEnsurePathWithinRepoRejectsBackslashTraversal(t *testing.T) {
	repoRoot := t.TempDir()

	_, err := ensurePathWithinRepo(repoRoot, "..\\outside.txt")
	if err == nil {
		t.Fatalf("expected backslash traversal path to be rejected")
	}

	bindingErr := AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected binding error, got: %v", err)
	}
	if bindingErr.Code != CodeInvalidPath {
		t.Fatalf("unexpected error code: got=%s want=%s", bindingErr.Code, CodeInvalidPath)
	}
}

func TestParsePorcelainStatusZIncludesBranchAndBuckets(t *testing.T) {
	raw := strings.Join([]string{
		"## main...origin/main [ahead 2, behind 1]",
		"M  staged.txt",
		" M unstaged.txt",
		"UU conflict.txt",
		"?? new file.txt",
		"R  renamed-new.txt",
		"renamed-old.txt",
		"C  copied-new.txt",
		"copied-old.txt",
		"",
	}, "\x00")

	status := parsePorcelainStatus(raw)
	if status.Branch != "main" {
		t.Fatalf("unexpected branch: got=%q want=%q", status.Branch, "main")
	}
	if status.Ahead != 2 || status.Behind != 1 {
		t.Fatalf("unexpected ahead/behind: got=%d/%d want=2/1", status.Ahead, status.Behind)
	}
	if !containsFile(status.Staged, "staged.txt") {
		t.Fatalf("expected staged.txt in staged bucket: %+v", status.Staged)
	}
	if !containsFile(status.Staged, "renamed-new.txt") {
		t.Fatalf("expected renamed-new.txt in staged bucket: %+v", status.Staged)
	}
	if !hasOriginalPath(status.Staged, "renamed-new.txt", "renamed-old.txt") {
		t.Fatalf("expected renamed path pair to be preserved: %+v", status.Staged)
	}
	if !hasOriginalPath(status.Staged, "copied-new.txt", "copied-old.txt") {
		t.Fatalf("expected copied path pair to be preserved: %+v", status.Staged)
	}
	if !containsFile(status.Unstaged, "unstaged.txt") {
		t.Fatalf("expected unstaged.txt in unstaged bucket: %+v", status.Unstaged)
	}
	if !containsFile(status.Unstaged, "new file.txt") {
		t.Fatalf("expected new file.txt in unstaged bucket: %+v", status.Unstaged)
	}
	if !containsConflict(status.Conflicted, "conflict.txt") {
		t.Fatalf("expected conflict.txt in conflicted bucket: %+v", status.Conflicted)
	}
}

func TestWriteQueueSerializesCommandsPerRepository(t *testing.T) {
	var running atomic.Int32
	var maxRunning atomic.Int32

	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		current := running.Add(1)
		for {
			prev := maxRunning.Load()
			if current <= prev || maxRunning.CompareAndSwap(prev, current) {
				break
			}
		}
		defer running.Add(-1)

		select {
		case <-ctx.Done():
			return "", "", 0, ctx.Err()
		case <-time.After(40 * time.Millisecond):
			return "", "", 0, nil
		}
	}

	svc := newServiceWithDeps(nil, runner, sleepWithContext)
	t.Cleanup(func() { _ = svc.Close(context.Background()) })

	repoRoot := "/tmp/orch-test-repo-queue"
	start := make(chan struct{})
	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errCh <- svc.executeWrite(
				repoRoot,
				fmt.Sprintf("cmd_%d", i),
				"stage_file",
				[]string{"add", "--", "README.md"},
				time.Now(),
				2*time.Second,
				func(ctx context.Context, diag *commandDiagnosticState) error {
					_, _, _, runErr := svc.runWriteGitWithRetry(ctx, diag, "", "-C", repoRoot, "add", "--", "README.md")
					return runErr
				},
			)
		}(i)
	}

	close(start)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("expected queued command to succeed, got error: %v", err)
		}
	}
	if maxRunning.Load() != 1 {
		t.Fatalf("expected max concurrency=1 for same repo, got=%d", maxRunning.Load())
	}
}

func TestBasicWriteActionsUseQueueSequentially(t *testing.T) {
	repoRoot := t.TempDir()
	filePath := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(filePath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("failed to prepare file: %v", err)
	}

	var runningWrites atomic.Int32
	var maxRunningWrites atomic.Int32
	var executedWrites atomic.Int32

	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		// Preflight: rev-parse --show-toplevel
		if hasArgSequence(args, "rev-parse", "--show-toplevel") {
			return repoRoot, "", 0, nil
		}
		// Preflight: rev-parse --abbrev-ref HEAD
		if hasArgSequence(args, "rev-parse", "--abbrev-ref", "HEAD") {
			return "main\n", "", 0, nil
		}

		// Simular duração em writes para detectar paralelismo indevido.
		if hasWriteCommand(args) {
			executedWrites.Add(1)
			current := runningWrites.Add(1)
			for {
				prev := maxRunningWrites.Load()
				if current <= prev || maxRunningWrites.CompareAndSwap(prev, current) {
					break
				}
			}
			defer runningWrites.Add(-1)

			select {
			case <-ctx.Done():
				return "", "", 0, ctx.Err()
			case <-time.After(35 * time.Millisecond):
				return "", "", 0, nil
			}
		}

		return "", "", 0, nil
	}

	svc := newServiceWithDeps(nil, runner, sleepWithContext)
	t.Cleanup(func() { _ = svc.Close(context.Background()) })

	errCh := make(chan error, 3)
	go func() { errCh <- svc.StageFile(repoRoot, "README.md") }()
	go func() { errCh <- svc.UnstageFile(repoRoot, "README.md") }()
	go func() { errCh <- svc.DiscardFile(repoRoot, "README.md") }()

	for i := 0; i < 3; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("write operation failed: %v", err)
		}
	}

	if executedWrites.Load() != 3 {
		t.Fatalf("unexpected executed write count: got=%d want=3", executedWrites.Load())
	}
	if maxRunningWrites.Load() != 1 {
		t.Fatalf("expected queue to serialize writes, got maxConcurrency=%d", maxRunningWrites.Load())
	}
}

func TestStageFileEmitsPostWriteReconciliationEvent(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	eventsMu := sync.Mutex{}
	events := make([]struct {
		name string
		data interface{}
	}, 0, 4)

	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		if hasArgSequence(args, "rev-parse", "--show-toplevel") {
			return repoRoot, "", 0, nil
		}
		if hasArgSequence(args, "rev-parse", "--abbrev-ref", "HEAD") {
			return "main\n", "", 0, nil
		}
		return "", "", 0, nil
	}

	svc := newServiceWithDeps(func(eventName string, data interface{}) {
		eventsMu.Lock()
		events = append(events, struct {
			name string
			data interface{}
		}{
			name: eventName,
			data: data,
		})
		eventsMu.Unlock()
	}, runner, sleepWithContext)
	t.Cleanup(func() { _ = svc.Close(context.Background()) })

	if err := svc.StageFile(repoRoot, "README.md"); err != nil {
		t.Fatalf("StageFile failed: %v", err)
	}

	eventsMu.Lock()
	defer eventsMu.Unlock()

	found := false
	for _, event := range events {
		if event.name != "gitpanel:status_changed" {
			continue
		}
		payload, ok := event.data.(map[string]string)
		if !ok {
			continue
		}
		if payload["reason"] == "post_write_reconcile" && payload["sourceEvent"] == "stage_file" {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected post-write reconciliation event, got=%+v", events)
	}
}

func TestStageFileEmitsCommandDiagnosticsLifecycle(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("failed to prepare file: %v", err)
	}

	var attempts atomic.Int32
	eventsMu := sync.Mutex{}
	events := make([]CommandResultDTO, 0, 8)

	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		if hasArgSequence(args, "rev-parse", "--show-toplevel") {
			return repoRoot, "", 0, nil
		}
		if hasArgSequence(args, "rev-parse", "--abbrev-ref", "HEAD") {
			return "main\n", "", 0, nil
		}
		if hasArgSequence(args, "add", "--", "README.md") {
			attempt := attempts.Add(1)
			if attempt == 1 {
				lockPath := filepath.Join(repoRoot, ".git", "index.lock")
				return "", fmt.Sprintf("fatal: Unable to create %q: File exists.", lockPath), 128, errors.New("exit status 128")
			}
			return "", "", 0, nil
		}
		return "", "", 0, nil
	}

	svc := newServiceWithDeps(func(eventName string, data interface{}) {
		if eventName != "gitpanel:command_result" {
			return
		}
		payload, ok := data.(CommandResultDTO)
		if !ok {
			t.Fatalf("unexpected payload type for command_result: %T", data)
		}
		eventsMu.Lock()
		events = append(events, payload)
		eventsMu.Unlock()
	}, runner, sleepWithContext)
	t.Cleanup(func() { _ = svc.Close(context.Background()) })

	if err := svc.StageFile(repoRoot, "README.md"); err != nil {
		t.Fatalf("StageFile failed: %v", err)
	}

	eventsMu.Lock()
	defer eventsMu.Unlock()

	statuses := make([]string, 0, len(events))
	var retriedEvent *CommandResultDTO
	for i := range events {
		event := events[i]
		if event.Action != "stage_file" {
			continue
		}
		statuses = append(statuses, event.Status)
		if event.Status == commandStatusRetried {
			copied := event
			retriedEvent = &copied
		}
	}

	expectedOrder := []string{
		commandStatusQueued,
		commandStatusStarted,
		commandStatusRetried,
		commandStatusSucceeded,
	}
	if len(statuses) != len(expectedOrder) {
		t.Fatalf("unexpected status sequence length: got=%v want=%v", statuses, expectedOrder)
	}
	for i := range expectedOrder {
		if statuses[i] != expectedOrder[i] {
			t.Fatalf("unexpected command diagnostic order: got=%v want=%v", statuses, expectedOrder)
		}
	}

	if retriedEvent == nil {
		t.Fatalf("expected retried event in diagnostics stream: %+v", events)
	}
	if retriedEvent.StderrSanitized == "" {
		t.Fatalf("expected stderrSanitized in retried event")
	}
	if strings.Contains(retriedEvent.StderrSanitized, repoRoot) {
		t.Fatalf("stderr must be sanitized: %q", retriedEvent.StderrSanitized)
	}
	if !strings.Contains(retriedEvent.StderrSanitized, "<repo>") {
		t.Fatalf("stderr must include <repo> token after sanitization: %q", retriedEvent.StderrSanitized)
	}
	if retriedEvent.Attempt != 1 {
		t.Fatalf("expected first retry to report attempt=1, got=%d", retriedEvent.Attempt)
	}
}

func TestStageFileEmitsFailedDiagnosticWithSanitizedStderr(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("failed to prepare file: %v", err)
	}

	var attempts atomic.Int32
	eventsMu := sync.Mutex{}
	events := make([]CommandResultDTO, 0, 8)

	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		if hasArgSequence(args, "rev-parse", "--show-toplevel") {
			return repoRoot, "", 0, nil
		}
		if hasArgSequence(args, "rev-parse", "--abbrev-ref", "HEAD") {
			return "main\n", "", 0, nil
		}
		if hasArgSequence(args, "add", "--", "README.md") {
			attempts.Add(1)
			lockedPath := filepath.Join(repoRoot, ".git", "index.lock")
			return "", fmt.Sprintf("fatal: could not lock %s: File exists", lockedPath), 128, errors.New("exit status 128")
		}
		return "", "", 0, nil
	}

	svc := newServiceWithDeps(func(eventName string, data interface{}) {
		if eventName != "gitpanel:command_result" {
			return
		}
		payload, ok := data.(CommandResultDTO)
		if !ok {
			t.Fatalf("unexpected payload type for command_result: %T", data)
		}
		eventsMu.Lock()
		events = append(events, payload)
		eventsMu.Unlock()
	}, runner, sleepWithContext)
	t.Cleanup(func() { _ = svc.Close(context.Background()) })

	err := svc.StageFile(repoRoot, "README.md")
	if err == nil {
		t.Fatalf("expected StageFile error when all retries fail")
	}

	eventsMu.Lock()
	defer eventsMu.Unlock()

	var failed *CommandResultDTO
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Action == "stage_file" && events[i].Status == commandStatusFailed {
			copied := events[i]
			failed = &copied
			break
		}
	}
	if failed == nil {
		t.Fatalf("expected failed diagnostic event, got=%+v", events)
	}
	if failed.ExitCode != 128 {
		t.Fatalf("expected exitCode=128 on failed diagnostic, got=%d", failed.ExitCode)
	}
	if failed.StderrSanitized == "" {
		t.Fatalf("expected stderrSanitized on failed diagnostic")
	}
	if strings.Contains(failed.StderrSanitized, repoRoot) {
		t.Fatalf("stderr must be sanitized: %q", failed.StderrSanitized)
	}
}

func TestGetStatusUsesShortLivedCache(t *testing.T) {
	repoRoot := t.TempDir()

	var statusCalls atomic.Int32
	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		if hasArgSequence(args, "rev-parse", "--show-toplevel") {
			return repoRoot, "", 0, nil
		}
		if hasArgSequence(args, "rev-parse", "--abbrev-ref", "HEAD") {
			return "main\n", "", 0, nil
		}
		if hasArgSequence(args, "status", "--porcelain=v1", "-z", "--branch") {
			statusCalls.Add(1)
			return "## main...origin/main [ahead 1]\x00 M README.md\x00", "", 0, nil
		}
		return "", "", 0, nil
	}

	svc := newServiceWithDeps(nil, runner, sleepWithContext)

	first, err := svc.GetStatus(repoRoot)
	if err != nil {
		t.Fatalf("GetStatus first call failed: %v", err)
	}
	second, err := svc.GetStatus(repoRoot)
	if err != nil {
		t.Fatalf("GetStatus second call failed: %v", err)
	}

	if statusCalls.Load() != 1 {
		t.Fatalf("expected status command to run once due to cache, got=%d", statusCalls.Load())
	}
	if first.Branch != second.Branch {
		t.Fatalf("cached status mismatch: first=%q second=%q", first.Branch, second.Branch)
	}
}

func TestInvalidateRepoCacheForcesFreshStatusRead(t *testing.T) {
	repoRoot := t.TempDir()

	var statusCalls atomic.Int32
	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		if hasArgSequence(args, "rev-parse", "--show-toplevel") {
			return repoRoot, "", 0, nil
		}
		if hasArgSequence(args, "rev-parse", "--abbrev-ref", "HEAD") {
			return "main\n", "", 0, nil
		}
		if hasArgSequence(args, "status", "--porcelain=v1", "-z", "--branch") {
			call := statusCalls.Add(1)
			return fmt.Sprintf("## main...origin/main [ahead %d]\x00", call), "", 0, nil
		}
		return "", "", 0, nil
	}

	svc := newServiceWithDeps(nil, runner, sleepWithContext)

	if _, err := svc.GetStatus(repoRoot); err != nil {
		t.Fatalf("GetStatus first call failed: %v", err)
	}
	svc.InvalidateRepoCache(repoRoot)
	second, err := svc.GetStatus(repoRoot)
	if err != nil {
		t.Fatalf("GetStatus second call failed: %v", err)
	}

	if statusCalls.Load() != 2 {
		t.Fatalf("expected status command to run twice after invalidation, got=%d", statusCalls.Load())
	}
	if second.Ahead != 2 {
		t.Fatalf("expected fresh status payload after cache invalidation, got ahead=%d", second.Ahead)
	}
}

func TestGetDiffSkipsPreviewForLargeFile(t *testing.T) {
	repoRoot := t.TempDir()
	largePath := filepath.Join(repoRoot, "large.txt")
	if err := os.WriteFile(largePath, bytesRepeat('x', maxDiffPreviewBytes+1024), 0o644); err != nil {
		t.Fatalf("failed to create large file: %v", err)
	}

	var diffCalls atomic.Int32
	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		if hasArgSequence(args, "rev-parse", "--show-toplevel") {
			return repoRoot, "", 0, nil
		}
		if hasArgSequence(args, "rev-parse", "--abbrev-ref", "HEAD") {
			return "main\n", "", 0, nil
		}
		if hasArgSequence(args, "diff") {
			diffCalls.Add(1)
			return "", "", 0, nil
		}
		return "", "", 0, nil
	}

	svc := newServiceWithDeps(nil, runner, sleepWithContext)

	diff, err := svc.GetDiff(repoRoot, "large.txt", "unified", 3)
	if err != nil {
		t.Fatalf("GetDiff returned error for large file fallback: %v", err)
	}
	if !diff.IsTruncated {
		t.Fatalf("expected large file diff to be marked as truncated fallback")
	}
	if !strings.Contains(diff.Raw, "Preview desativado automaticamente") {
		t.Fatalf("expected fallback explanation in raw diff, got=%q", diff.Raw)
	}
	if diffCalls.Load() != 0 {
		t.Fatalf("expected git diff command to be skipped for large file, got calls=%d", diffCalls.Load())
	}
}

func TestGetDiffTimeoutReturnsDegradedPayload(t *testing.T) {
	repoRoot := t.TempDir()

	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		if hasArgSequence(args, "rev-parse", "--show-toplevel") {
			return repoRoot, "", 0, nil
		}
		if hasArgSequence(args, "rev-parse", "--abbrev-ref", "HEAD") {
			return "main\n", "", 0, nil
		}
		if hasArgSequence(args, "diff") {
			return "", "", 0, NewBindingError(CodeTimeout, "timeout", "diff timeout")
		}
		return "", "", 0, nil
	}

	svc := newServiceWithDeps(nil, runner, sleepWithContext)
	diff, err := svc.GetDiff(repoRoot, "README.md", "unified", 3)
	if err != nil {
		t.Fatalf("expected timeout fallback payload instead of error, got: %v", err)
	}
	if !diff.IsTruncated {
		t.Fatalf("expected degraded diff to be flagged as truncated")
	}
	if !strings.Contains(diff.Raw, "excedeu o timeout") {
		t.Fatalf("expected timeout hint in degraded diff message, got=%q", diff.Raw)
	}
}

func TestOpenExternalMergeToolUsesGitMergetool(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "conflict.txt"), []byte("content\n"), 0o644); err != nil {
		t.Fatalf("failed to create conflict file: %v", err)
	}

	var mergetoolCalls atomic.Int32
	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		if hasArgSequence(args, "rev-parse", "--show-toplevel") {
			return repoRoot, "", 0, nil
		}
		if hasArgSequence(args, "rev-parse", "--abbrev-ref", "HEAD") {
			return "main\n", "", 0, nil
		}
		if hasArgSequence(args, "mergetool", "--no-prompt", "--", "conflict.txt") {
			mergetoolCalls.Add(1)
			return "", "", 0, nil
		}
		return "", "", 0, nil
	}

	svc := newServiceWithDeps(nil, runner, sleepWithContext)
	t.Cleanup(func() { _ = svc.Close(context.Background()) })

	if err := svc.OpenExternalMergeTool(repoRoot, "conflict.txt"); err != nil {
		t.Fatalf("OpenExternalMergeTool failed: %v", err)
	}
	if mergetoolCalls.Load() != 1 {
		t.Fatalf("expected mergetool command once, got=%d", mergetoolCalls.Load())
	}
}

func TestParseHistoryItemsPreservesSpecialCharacters(t *testing.T) {
	raw := strings.Join([]string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\x1faaaaaaaa\x1fAlice\x1f2026-02-22T15:04:05Z\x1falice@example.com\x1ffeat: suporte pipe | tab\t e separador \x1f interno\x1e",
		"12\t3\tsrc/main.ts",
		"-\t-\tassets/logo.png",
	}, "\n")

	items := parseHistoryItems(raw)
	if len(items) != 1 {
		t.Fatalf("unexpected history item count: got=%d want=1", len(items))
	}
	if got, want := items[0].Hash, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"; got != want {
		t.Fatalf("unexpected hash: got=%q want=%q", got, want)
	}
	if got, want := items[0].Subject, "feat: suporte pipe | tab\t e separador \x1f interno"; got != want {
		t.Fatalf("unexpected subject parsing: got=%q want=%q", got, want)
	}
	if got, want := items[0].AuthorEmail, "alice@example.com"; got != want {
		t.Fatalf("unexpected author email: got=%q want=%q", got, want)
	}
	if got, want := items[0].Additions, 12; got != want {
		t.Fatalf("unexpected additions: got=%d want=%d", got, want)
	}
	if got, want := items[0].Deletions, 3; got != want {
		t.Fatalf("unexpected deletions: got=%d want=%d", got, want)
	}
	if got, want := items[0].ChangedFiles, 2; got != want {
		t.Fatalf("unexpected changed files: got=%d want=%d", got, want)
	}
}

func TestParseHistoryItemsParsesMultipleCommitsInGitLogNumstatLayout(t *testing.T) {
	raw := strings.Join([]string{
		"1111111111111111111111111111111111111111\x1f11111111\x1fAlice\x1f2026-02-22T15:04:05Z\x1falice@example.com\x1ffeat: first\x1e",
		"10\t2\tsrc/main.ts",
		"-\t-\tassets/logo.png",
		"",
		"2222222222222222222222222222222222222222\x1f22222222\x1fBob\x1f2026-02-21T15:04:05Z\x1fbob@example.com\x1ffix: second\x1e",
		"4\t1\tREADME.md",
	}, "\n")

	items := parseHistoryItems(raw)
	if got, want := len(items), 2; got != want {
		t.Fatalf("unexpected history item count: got=%d want=%d", got, want)
	}

	first := items[0]
	if got, want := first.ChangedFiles, 2; got != want {
		t.Fatalf("unexpected first changed files: got=%d want=%d", got, want)
	}
	if got, want := first.Additions, 10; got != want {
		t.Fatalf("unexpected first additions: got=%d want=%d", got, want)
	}
	if got, want := first.Deletions, 2; got != want {
		t.Fatalf("unexpected first deletions: got=%d want=%d", got, want)
	}

	second := items[1]
	if got, want := second.ChangedFiles, 1; got != want {
		t.Fatalf("unexpected second changed files: got=%d want=%d", got, want)
	}
	if got, want := second.Additions, 4; got != want {
		t.Fatalf("unexpected second additions: got=%d want=%d", got, want)
	}
	if got, want := second.Deletions, 1; got != want {
		t.Fatalf("unexpected second deletions: got=%d want=%d", got, want)
	}
}

func TestParseDiffFilesStructuredModel(t *testing.T) {
	raw := strings.Join([]string{
		`diff --git "a/old name.txt" "b/new name.txt"`,
		"similarity index 97%",
		"rename from old name.txt",
		"rename to new name.txt",
		"index 1111111..2222222 100644",
		"--- a/old name.txt",
		"+++ b/new name.txt",
		"@@ -1,2 +1,2 @@",
		"-old value",
		"+new value",
		" context",
		"diff --git a/assets/logo.png b/assets/logo.png",
		"index 3333333..4444444 100644",
		"Binary files a/assets/logo.png and b/assets/logo.png differ",
	}, "\n")

	files := parseDiffFiles(raw)
	if len(files) != 2 {
		t.Fatalf("unexpected file count: got=%d want=2", len(files))
	}

	renamed := files[0]
	if renamed.Status != "renamed" {
		t.Fatalf("unexpected status for renamed file: got=%q want=%q", renamed.Status, "renamed")
	}
	if renamed.OldPath != "old name.txt" {
		t.Fatalf("unexpected oldPath: got=%q", renamed.OldPath)
	}
	if renamed.Path != "new name.txt" {
		t.Fatalf("unexpected path: got=%q", renamed.Path)
	}
	if renamed.Additions != 1 || renamed.Deletions != 1 {
		t.Fatalf("unexpected additions/deletions: got=%d/%d want=1/1", renamed.Additions, renamed.Deletions)
	}
	if len(renamed.Hunks) != 1 {
		t.Fatalf("expected one hunk in renamed file, got=%d", len(renamed.Hunks))
	}
	if len(renamed.Hunks[0].Lines) != 3 {
		t.Fatalf("unexpected hunk line count: got=%d want=3", len(renamed.Hunks[0].Lines))
	}

	binary := files[1]
	if binary.Path != "assets/logo.png" {
		t.Fatalf("unexpected binary path: got=%q", binary.Path)
	}
	if !binary.IsBinary {
		t.Fatalf("expected binary file to be marked as binary")
	}
	if len(binary.Hunks) != 0 {
		t.Fatalf("expected no hunks for binary file, got=%d", len(binary.Hunks))
	}
}

func TestParseDiffFilesHandlesAddedAndDeletedFiles(t *testing.T) {
	raw := strings.Join([]string{
		"diff --git a/dev/null b/new-file.txt",
		"new file mode 100644",
		"index 0000000..1111111",
		"--- /dev/null",
		"+++ b/new-file.txt",
		"@@ -0,0 +1,2 @@",
		"+line a",
		"+line b",
		"diff --git a/old-file.txt b/dev/null",
		"deleted file mode 100644",
		"index 1111111..0000000",
		"--- a/old-file.txt",
		"+++ /dev/null",
		"@@ -1,2 +0,0 @@",
		"-line x",
		"-line y",
	}, "\n")

	files := parseDiffFiles(raw)
	if len(files) != 2 {
		t.Fatalf("unexpected file count: got=%d want=2", len(files))
	}

	added := files[0]
	if added.Status != "added" {
		t.Fatalf("unexpected status for added file: got=%q want=%q", added.Status, "added")
	}
	if added.Path != "new-file.txt" {
		t.Fatalf("unexpected path for added file: got=%q", added.Path)
	}
	if added.Additions != 2 || added.Deletions != 0 {
		t.Fatalf("unexpected added file counters: got=%d/%d want=2/0", added.Additions, added.Deletions)
	}

	deleted := files[1]
	if deleted.Status != "deleted" {
		t.Fatalf("unexpected status for deleted file: got=%q want=%q", deleted.Status, "deleted")
	}
	if deleted.Path != "old-file.txt" {
		t.Fatalf("unexpected path for deleted file: got=%q", deleted.Path)
	}
	if deleted.Deletions != 2 || deleted.Additions != 0 {
		t.Fatalf("unexpected deleted file counters: got=%d/%d want=2/0", deleted.Deletions, deleted.Additions)
	}
}

func TestGetHistoryUsesHashCursorAndSkipResolution(t *testing.T) {
	repoRoot := t.TempDir()
	const (
		hashC = "cccccccccccccccccccccccccccccccccccccccc"
		hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)

	var revListCalls atomic.Int32

	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		if hasArgSequence(args, "rev-parse", "--show-toplevel") {
			return repoRoot, "", 0, nil
		}
		if hasArgSequence(args, "rev-parse", "--abbrev-ref", "HEAD") {
			return "main\n", "", 0, nil
		}
		if hasArgSequence(args, "rev-list", "--count", hashB+"..HEAD") {
			revListCalls.Add(1)
			return "1\n", "", 0, nil
		}
		if hasArgSequence(args, "log") {
			switch {
			case containsArg(args, "--skip=0"):
				return historyRecord(hashC, "cccccccc", "Alice", "2026-02-22T11:00:00Z", "alice@example.com", "third") +
					historyRecord(hashB, "bbbbbbbb", "Bob", "2026-02-21T11:00:00Z", "bob@example.com", "second") +
					historyRecord(hashA, "aaaaaaaa", "Carol", "2026-02-20T11:00:00Z", "carol@example.com", "first"), "", 0, nil
			case containsArg(args, "--skip=2"):
				return historyRecord(hashA, "aaaaaaaa", "Carol", "2026-02-20T11:00:00Z", "carol@example.com", "first"), "", 0, nil
			default:
				return "", "", 1, fmt.Errorf("unexpected log args: %v", args)
			}
		}
		return "", "", 0, nil
	}

	svc := newServiceWithDeps(nil, runner, sleepWithContext)

	page1, err := svc.GetHistory(repoRoot, "", 2, "")
	if err != nil {
		t.Fatalf("GetHistory first page failed: %v", err)
	}
	if len(page1.Items) != 2 || !page1.HasMore {
		t.Fatalf("unexpected first page shape: %+v", page1)
	}
	if page1.NextCursor != hashB {
		t.Fatalf("expected next cursor to be last hash of page, got=%q want=%q", page1.NextCursor, hashB)
	}

	page2, err := svc.GetHistory(repoRoot, page1.NextCursor, 2, "")
	if err != nil {
		t.Fatalf("GetHistory second page failed: %v", err)
	}
	if len(page2.Items) != 1 || page2.HasMore {
		t.Fatalf("unexpected second page shape: %+v", page2)
	}
	if page2.Items[0].Hash != hashA {
		t.Fatalf("unexpected second page item hash: got=%q want=%q", page2.Items[0].Hash, hashA)
	}
	if page2.NextCursor != "" {
		t.Fatalf("expected empty next cursor for tail page, got=%q", page2.NextCursor)
	}
	if revListCalls.Load() != 1 {
		t.Fatalf("expected rev-list cursor resolution call once, got=%d", revListCalls.Load())
	}
}

func TestRunWriteGitWithRetryForIndexLock(t *testing.T) {
	var attempts atomic.Int32
	var sleepMu sync.Mutex
	sleepDurations := make([]time.Duration, 0, 3)

	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		attempt := attempts.Add(1)
		if attempt < 3 {
			return "", "fatal: Unable to create '/tmp/repo/.git/index.lock': File exists.", 128, errors.New("exit status 128")
		}
		return "", "", 0, nil
	}
	sleeper := func(ctx context.Context, d time.Duration) error {
		sleepMu.Lock()
		sleepDurations = append(sleepDurations, d)
		sleepMu.Unlock()
		return nil
	}

	svc := newServiceWithDeps(nil, runner, sleeper)
	_, _, _, err := svc.runWriteGitWithRetry(context.Background(), nil, "", "-C", "/tmp/repo", "add", "--", "a.txt")
	if err != nil {
		t.Fatalf("expected retry flow to succeed, got error: %v", err)
	}
	if attempts.Load() != 3 {
		t.Fatalf("unexpected attempts count: got=%d want=3", attempts.Load())
	}

	sleepMu.Lock()
	defer sleepMu.Unlock()
	if len(sleepDurations) != 2 {
		t.Fatalf("unexpected backoff count: got=%d want=2", len(sleepDurations))
	}
	if sleepDurations[0] != 80*time.Millisecond || sleepDurations[1] != 160*time.Millisecond {
		t.Fatalf("unexpected backoff sequence: got=%v", sleepDurations)
	}
}

func TestExecuteWriteRespectsTimeout(t *testing.T) {
	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		<-ctx.Done()
		return "", "", 0, ctx.Err()
	}

	svc := newServiceWithDeps(nil, runner, sleepWithContext)
	t.Cleanup(func() { _ = svc.Close(context.Background()) })

	err := svc.executeWrite(
		"/tmp/orch-timeout-repo",
		"cmd_timeout",
		"stage_file",
		[]string{"add", "--", "README.md"},
		time.Now(),
		30*time.Millisecond,
		func(ctx context.Context, diag *commandDiagnosticState) error {
			_, _, _, runErr := svc.runWriteGitWithRetry(ctx, diag, "", "-C", "/tmp/orch-timeout-repo", "add", "--", "README.md")
			return runErr
		},
	)
	if err == nil {
		t.Fatalf("expected timeout error")
	}

	bindingErr := AsBindingError(err)
	if bindingErr == nil {
		t.Fatalf("expected binding error, got: %v", err)
	}
	if bindingErr.Code != CodeTimeout {
		t.Fatalf("unexpected timeout code: got=%s want=%s", bindingErr.Code, CodeTimeout)
	}
}

func TestQueueWorkerShutdownCancelsInFlightCommand(t *testing.T) {
	started := make(chan struct{}, 1)
	runner := func(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return "", "", 0, ctx.Err()
	}

	svc := newServiceWithDeps(nil, runner, sleepWithContext)
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.executeWrite(
			"/tmp/orch-close-repo",
			"cmd_close",
			"stage_file",
			[]string{"add", "--", "README.md"},
			time.Now(),
			3*time.Second,
			func(ctx context.Context, diag *commandDiagnosticState) error {
				_, _, _, runErr := svc.runWriteGitWithRetry(ctx, diag, "", "-C", "/tmp/orch-close-repo", "add", "--", "README.md")
				return runErr
			},
		)
	}()

	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("command never started execution")
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Close(closeCtx); err != nil {
		t.Fatalf("expected close without timeout, got: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatalf("expected canceled command error")
		}
		bindingErr := AsBindingError(err)
		if bindingErr == nil {
			t.Fatalf("expected binding error, got: %v", err)
		}
		if bindingErr.Code != CodeCanceled && bindingErr.Code != CodeServiceUnavailable {
			t.Fatalf("unexpected cancellation code: got=%s", bindingErr.Code)
		}
	case <-time.After(time.Second):
		t.Fatalf("command did not return after shutdown")
	}
}

func containsFile(files []FileChangeDTO, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func containsConflict(files []ConflictFileDTO, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func hasOriginalPath(files []FileChangeDTO, path string, originalPath string) bool {
	for _, file := range files {
		if file.Path == path && file.OriginalPath == originalPath {
			return true
		}
	}
	return false
}

func hasArgSequence(args []string, sequence ...string) bool {
	if len(sequence) == 0 || len(args) < len(sequence) {
		return false
	}
	for i := 0; i <= len(args)-len(sequence); i++ {
		match := true
		for j := 0; j < len(sequence); j++ {
			if args[i+j] != sequence[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func containsArg(args []string, token string) bool {
	for _, arg := range args {
		if arg == token {
			return true
		}
	}
	return false
}

func historyRecord(hash string, shortHash string, author string, authoredAt string, authorEmail string, subject string, numstatLines ...string) string {
	record := strings.Join([]string{
		hash,
		shortHash,
		author,
		authoredAt,
		authorEmail,
		subject,
	}, "\x1f")

	if len(numstatLines) > 0 {
		record += "\n" + strings.Join(numstatLines, "\n")
	}

	return record + "\x1e"
}

func bytesRepeat(char byte, size int) []byte {
	if size <= 0 {
		return nil
	}
	out := make([]byte, size)
	for i := range out {
		out[i] = char
	}
	return out
}

func hasWriteCommand(args []string) bool {
	return hasArgSequence(args, "add") ||
		hasArgSequence(args, "restore", "--staged") ||
		hasArgSequence(args, "checkout")
}

func mustInitTestRepo(t *testing.T) string {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available in test environment")
	}

	repoRoot := t.TempDir()
	runGitOrFail(t, repoRoot, "init")
	runGitOrFail(t, repoRoot, "config", "user.email", "tests@orch.local")
	runGitOrFail(t, repoRoot, "config", "user.name", "ORCH Tests")

	readmePath := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	runGitOrFail(t, repoRoot, "add", "--", "README.md")
	runGitOrFail(t, repoRoot, "commit", "-m", "initial commit")
	return repoRoot
}

func runGitOrFail(t *testing.T, repoRoot string, args ...string) {
	t.Helper()

	allArgs := append([]string{"-C", repoRoot}, args...)
	_, stderr, _, err := runGitWithInput(context.Background(), 5*time.Second, "", allArgs...)
	if err != nil {
		t.Fatalf("git %s failed: %v stderr=%s", strings.Join(args, " "), err, strings.TrimSpace(stderr))
	}
}
