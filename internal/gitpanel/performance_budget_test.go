package gitpanel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

const (
	gitPanelBudgetOpenPanel       = 300 * time.Millisecond
	gitPanelBudgetHistoryFirst    = 200 * time.Millisecond
	gitPanelBudgetStageFile       = 150 * time.Millisecond
	gitPanelBudgetUnstageFile     = 150 * time.Millisecond
	gitPanelPerfCommitCountMedium = 180
	gitPanelPerfSamples           = 5
)

func TestGitPanelLatencyBudgets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping latency budget test in short mode")
	}

	repoRoot := mustInitPerformanceRepo(t, gitPanelPerfCommitCountMedium)
	svc := NewService(nil)
	t.Cleanup(func() { _ = svc.Close(context.Background()) })

	// Warmup para reduzir ruído de primeira execução.
	if _, err := measureGitPanelOpenPipeline(svc, repoRoot); err != nil {
		t.Fatalf("open pipeline warmup failed: %v", err)
	}
	if _, err := svc.GetHistory(repoRoot, "", defaultHistoryLimit, ""); err != nil {
		t.Fatalf("history warmup failed: %v", err)
	}

	openSamples := make([]time.Duration, 0, gitPanelPerfSamples)
	historySamples := make([]time.Duration, 0, gitPanelPerfSamples)
	stageSamples := make([]time.Duration, 0, gitPanelPerfSamples)
	unstageSamples := make([]time.Duration, 0, gitPanelPerfSamples)

	for index := 0; index < gitPanelPerfSamples; index++ {
		svc.InvalidateRepoCache(repoRoot)
		openDuration, err := measureGitPanelOpenPipeline(svc, repoRoot)
		if err != nil {
			t.Fatalf("open pipeline sample %d failed: %v", index+1, err)
		}
		openSamples = append(openSamples, openDuration)

		svc.InvalidateRepoCache(repoRoot)
		historyStartedAt := time.Now()
		if _, err := svc.GetHistory(repoRoot, "", defaultHistoryLimit, ""); err != nil {
			t.Fatalf("history sample %d failed: %v", index+1, err)
		}
		historySamples = append(historySamples, time.Since(historyStartedAt))
	}

	stagePath := "latency-target.txt"
	targetAbs := filepath.Join(repoRoot, stagePath)
	for index := 0; index < gitPanelPerfSamples; index++ {
		content := fmt.Sprintf("latency sample %d at %s\n", index+1, time.Now().Format(time.RFC3339Nano))
		if err := os.WriteFile(targetAbs, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to update stage target file: %v", err)
		}

		stageStartedAt := time.Now()
		if err := svc.StageFile(repoRoot, stagePath); err != nil {
			t.Fatalf("stage sample %d failed: %v", index+1, err)
		}
		stageSamples = append(stageSamples, time.Since(stageStartedAt))

		unstageStartedAt := time.Now()
		if err := svc.UnstageFile(repoRoot, stagePath); err != nil {
			t.Fatalf("unstage sample %d failed: %v", index+1, err)
		}
		unstageSamples = append(unstageSamples, time.Since(unstageStartedAt))
	}

	openMedian, openP95 := summarizeDurations(openSamples)
	historyMedian, historyP95 := summarizeDurations(historySamples)
	stageMedian, stageP95 := summarizeDurations(stageSamples)
	unstageMedian, unstageP95 := summarizeDurations(unstageSamples)

	t.Logf(
		"[baseline] open_panel median=%s p95=%s budget=%s samples=%s",
		openMedian,
		openP95,
		gitPanelBudgetOpenPanel,
		formatDurationSlice(openSamples),
	)
	t.Logf(
		"[baseline] history_first_page median=%s p95=%s budget=%s samples=%s",
		historyMedian,
		historyP95,
		gitPanelBudgetHistoryFirst,
		formatDurationSlice(historySamples),
	)
	t.Logf(
		"[baseline] stage_file median=%s p95=%s budget=%s samples=%s",
		stageMedian,
		stageP95,
		gitPanelBudgetStageFile,
		formatDurationSlice(stageSamples),
	)
	t.Logf(
		"[baseline] unstage_file median=%s p95=%s budget=%s samples=%s",
		unstageMedian,
		unstageP95,
		gitPanelBudgetUnstageFile,
		formatDurationSlice(unstageSamples),
	)

	if openMedian > gitPanelBudgetOpenPanel {
		t.Fatalf("open panel median above budget: got=%s budget=%s", openMedian, gitPanelBudgetOpenPanel)
	}
	if historyMedian > gitPanelBudgetHistoryFirst {
		t.Fatalf("history first page median above budget: got=%s budget=%s", historyMedian, gitPanelBudgetHistoryFirst)
	}
	if stageMedian > gitPanelBudgetStageFile {
		t.Fatalf("stage median above budget: got=%s budget=%s", stageMedian, gitPanelBudgetStageFile)
	}
	if unstageMedian > gitPanelBudgetUnstageFile {
		t.Fatalf("unstage median above budget: got=%s budget=%s", unstageMedian, gitPanelBudgetUnstageFile)
	}
}

func mustInitPerformanceRepo(t *testing.T, additionalCommits int) string {
	t.Helper()

	repoRoot := mustInitTestRepo(t)

	historyFile := filepath.Join(repoRoot, "history.txt")
	for index := 0; index < additionalCommits; index++ {
		content := fmt.Sprintf("commit-%03d @ %s\n", index+1, time.Now().Format(time.RFC3339Nano))
		if err := os.WriteFile(historyFile, []byte(content), 0o644); err != nil {
			t.Fatalf("failed to write history file: %v", err)
		}
		runGitOrFail(t, repoRoot, "add", "--", "history.txt")
		runGitOrFail(t, repoRoot, "commit", "-m", fmt.Sprintf("history commit %03d", index+1))
	}

	stageTarget := filepath.Join(repoRoot, "latency-target.txt")
	if err := os.WriteFile(stageTarget, []byte("baseline\n"), 0o644); err != nil {
		t.Fatalf("failed to create stage target file: %v", err)
	}
	runGitOrFail(t, repoRoot, "add", "--", "latency-target.txt")
	runGitOrFail(t, repoRoot, "commit", "-m", "add latency target")

	return repoRoot
}

func measureGitPanelOpenPipeline(svc *Service, repoPath string) (time.Duration, error) {
	startedAt := time.Now()
	preflight, err := svc.Preflight(repoPath)
	if err != nil {
		return 0, err
	}

	errCh := make(chan error, 2)
	go func() {
		_, statusErr := svc.GetStatus(preflight.RepoRoot)
		errCh <- statusErr
	}()
	go func() {
		_, historyErr := svc.GetHistory(preflight.RepoRoot, "", defaultHistoryLimit, "")
		errCh <- historyErr
	}()

	for index := 0; index < 2; index++ {
		if runErr := <-errCh; runErr != nil {
			return 0, runErr
		}
	}

	return time.Since(startedAt), nil
}

func summarizeDurations(input []time.Duration) (time.Duration, time.Duration) {
	if len(input) == 0 {
		return 0, 0
	}

	values := append(make([]time.Duration, 0, len(input)), input...)
	sort.Slice(values, func(left int, right int) bool {
		return values[left] < values[right]
	})

	median := values[len(values)/2]
	p95Index := int(float64(len(values)-1) * 0.95)
	if p95Index < 0 {
		p95Index = 0
	}
	if p95Index >= len(values) {
		p95Index = len(values) - 1
	}

	return median, values[p95Index]
}

func formatDurationSlice(input []time.Duration) string {
	if len(input) == 0 {
		return "[]"
	}

	formatted := make([]string, 0, len(input))
	for _, value := range input {
		formatted = append(formatted, value.String())
	}
	return "[" + joinComma(formatted) + "]"
}

func joinComma(values []string) string {
	if len(values) == 0 {
		return ""
	}
	joined := values[0]
	for index := 1; index < len(values); index++ {
		joined += ", " + values[index]
	}
	return joined
}
