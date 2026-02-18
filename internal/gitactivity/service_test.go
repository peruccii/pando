package gitactivity

import (
	"testing"
	"time"
)

func TestAppendEventDeduplicatesWithinWindow(t *testing.T) {
	service := NewService(20, 500*time.Millisecond)
	baseTime := time.Now()

	_, ok := service.AppendEvent(Event{
		Type:      EventTypeBranchChanged,
		RepoPath:  "/tmp/repo",
		Branch:    "main",
		Message:   "branch changed",
		Timestamp: baseTime,
		DedupeKey: "branch|/tmp/repo|main",
	})
	if !ok {
		t.Fatalf("expected first event to be accepted")
	}

	_, ok = service.AppendEvent(Event{
		Type:      EventTypeBranchChanged,
		RepoPath:  "/tmp/repo",
		Branch:    "main",
		Message:   "branch changed duplicate",
		Timestamp: baseTime.Add(120 * time.Millisecond),
		DedupeKey: "branch|/tmp/repo|main",
	})
	if ok {
		t.Fatalf("expected duplicate event inside dedupe window to be ignored")
	}

	if got := service.Count(); got != 1 {
		t.Fatalf("expected count=1, got %d", got)
	}
}

func TestListEventsAppliesFiltersAndOrder(t *testing.T) {
	service := NewService(20, 0)
	now := time.Now()

	service.AppendEvent(Event{
		Type:      EventTypeFetch,
		RepoPath:  "/tmp/a",
		Message:   "fetch",
		Timestamp: now.Add(-2 * time.Second),
		DedupeKey: "fetch|/tmp/a",
	})
	service.AppendEvent(Event{
		Type:      EventTypeBranchChanged,
		RepoPath:  "/tmp/b",
		Message:   "branch b",
		Timestamp: now.Add(-1 * time.Second),
		DedupeKey: "branch|/tmp/b",
	})
	service.AppendEvent(Event{
		Type:      EventTypeBranchChanged,
		RepoPath:  "/tmp/a",
		Message:   "branch a",
		Timestamp: now,
		DedupeKey: "branch|/tmp/a",
	})

	items := service.ListEvents(ListOptions{
		Limit:    10,
		Type:     EventTypeBranchChanged,
		RepoPath: "/tmp/a",
	})

	if len(items) != 1 {
		t.Fatalf("expected 1 filtered event, got %d", len(items))
	}
	if items[0].Message != "branch a" {
		t.Fatalf("expected most recent matching event, got %q", items[0].Message)
	}
}

func TestGetAndClearEvent(t *testing.T) {
	service := NewService(5, 0)
	stored, ok := service.AppendEvent(Event{
		Type:      EventTypeIndexUpdated,
		RepoPath:  "/tmp/repo",
		Message:   "index",
		Timestamp: time.Now(),
		DedupeKey: "index|/tmp/repo",
	})
	if !ok {
		t.Fatalf("expected event to be appended")
	}

	got, found := service.GetEvent(stored.ID)
	if !found || got == nil {
		t.Fatalf("expected event to be found by id")
	}
	if got.ID != stored.ID {
		t.Fatalf("expected id %q, got %q", stored.ID, got.ID)
	}

	service.Clear()
	if service.Count() != 0 {
		t.Fatalf("expected count=0 after clear")
	}
	if _, found := service.GetEvent(stored.ID); found {
		t.Fatalf("expected event not found after clear")
	}
}
