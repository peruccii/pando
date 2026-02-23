package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

const (
	gitPanelWatcherBurstSamples       = 5
	gitPanelWatcherBurstEvents        = 2000
	gitPanelWatcherEnqueueBudget      = 120 * time.Millisecond
	gitPanelWatcherFlushBudget        = 320 * time.Millisecond
	gitPanelWatcherFlushTimeout       = 900 * time.Millisecond
	gitPanelWatcherFlushPollInterval  = 5 * time.Millisecond
)

func TestGitPanelWatcherBurstBudgets(t *testing.T) {
	app := NewApp()
	t.Cleanup(app.stopGitPanelEventBridge)

	repoPath := filepath.Clean("/tmp/gitpanel-burst-repo")
	plan := gitPanelInvalidationPlan{
		Status:    true,
		History:   true,
		Conflicts: true,
	}

	enqueueSamples := make([]time.Duration, 0, gitPanelWatcherBurstSamples)
	flushSamples := make([]time.Duration, 0, gitPanelWatcherBurstSamples)

	for sample := 0; sample < gitPanelWatcherBurstSamples; sample++ {
		enqueueStartedAt := time.Now()
		for eventIndex := 0; eventIndex < gitPanelWatcherBurstEvents; eventIndex++ {
			app.queueGitPanelInvalidation(repoPath, "git:index", "filewatcher_bridge", plan)
		}
		enqueueDuration := time.Since(enqueueStartedAt)
		enqueueSamples = append(enqueueSamples, enqueueDuration)

		pendingCount, hasRepoEntry := snapshotPendingInvalidations(app, repoPath)
		if pendingCount != 1 || !hasRepoEntry {
			t.Fatalf("burst should coalesce into a single pending entry (count=%d, hasRepoEntry=%v)", pendingCount, hasRepoEntry)
		}

		flushStartedAt := time.Now()
		if err := waitForPendingInvalidationDrain(app, repoPath, gitPanelWatcherFlushTimeout); err != nil {
			t.Fatalf("pending invalidation did not flush within timeout: %v", err)
		}
		flushSamples = append(flushSamples, time.Since(flushStartedAt))
	}

	enqueueMedian, enqueueP95 := summarizeDurationSamples(enqueueSamples)
	flushMedian, flushP95 := summarizeDurationSamples(flushSamples)

	t.Logf(
		"[baseline] watcher_burst_enqueue events=%d median=%s p95=%s budget=%s samples=%s",
		gitPanelWatcherBurstEvents,
		enqueueMedian,
		enqueueP95,
		gitPanelWatcherEnqueueBudget,
		formatDurationSamples(enqueueSamples),
	)
	t.Logf(
		"[baseline] watcher_burst_flush median=%s p95=%s budget=%s samples=%s",
		flushMedian,
		flushP95,
		gitPanelWatcherFlushBudget,
		formatDurationSamples(flushSamples),
	)

	if enqueueMedian > gitPanelWatcherEnqueueBudget {
		t.Fatalf("watcher burst enqueue median above budget: got=%s budget=%s", enqueueMedian, gitPanelWatcherEnqueueBudget)
	}
	if flushMedian > gitPanelWatcherFlushBudget {
		t.Fatalf("watcher burst flush median above budget: got=%s budget=%s", flushMedian, gitPanelWatcherFlushBudget)
	}
}

func snapshotPendingInvalidations(app *App, repoPath string) (int, bool) {
	key, _ := normalizeGitPanelInvalidationRepo(repoPath)

	app.gitPanelEventsMu.Lock()
	defer app.gitPanelEventsMu.Unlock()

	_, hasRepoEntry := app.gitPanelPendingEvents[key]
	return len(app.gitPanelPendingEvents), hasRepoEntry
}

func waitForPendingInvalidationDrain(app *App, repoPath string, timeout time.Duration) error {
	key, _ := normalizeGitPanelInvalidationRepo(repoPath)
	deadline := time.Now().Add(timeout)

	for {
		app.gitPanelEventsMu.Lock()
		_, exists := app.gitPanelPendingEvents[key]
		app.gitPanelEventsMu.Unlock()

		if !exists {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("repo key %q still pending after %s", key, timeout)
		}
		time.Sleep(gitPanelWatcherFlushPollInterval)
	}
}

func summarizeDurationSamples(samples []time.Duration) (time.Duration, time.Duration) {
	if len(samples) == 0 {
		return 0, 0
	}

	sorted := append(make([]time.Duration, 0, len(samples)), samples...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	median := sorted[len(sorted)/2]
	p95Index := int(float64(len(sorted)-1) * 0.95)
	if p95Index < 0 {
		p95Index = 0
	}
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}

	return median, sorted[p95Index]
}

func formatDurationSamples(samples []time.Duration) string {
	if len(samples) == 0 {
		return "[]"
	}

	formatted := make([]string, 0, len(samples))
	for _, sample := range samples {
		formatted = append(formatted, sample.String())
	}

	joined := formatted[0]
	for index := 1; index < len(formatted); index++ {
		joined += ", " + formatted[index]
	}

	return "[" + joined + "]"
}
