package main

import (
	"path/filepath"
	"testing"

	fw "orch/internal/filewatcher"
	ga "orch/internal/gitactivity"
)

func TestBuildIndexFingerprint_SortedStable(t *testing.T) {
	filesA := []ga.EventFile{
		{Path: "b.txt", Status: "M", Added: 1, Removed: 0},
		{Path: "a.txt", Status: "A", Added: 2, Removed: 0},
	}
	filesB := []ga.EventFile{
		{Path: "a.txt", Status: "A", Added: 2, Removed: 0},
		{Path: "b.txt", Status: "M", Added: 1, Removed: 0},
	}

	fpA := buildIndexFingerprint(filesA)
	fpB := buildIndexFingerprint(filesB)
	if fpA != fpB {
		t.Fatalf("fingerprint must be order-independent. a=%q b=%q", fpA, fpB)
	}
}

func TestShouldRecordIndexActivity(t *testing.T) {
	app := NewApp()
	repo := "/tmp/repo"

	initial := []ga.EventFile{
		{Path: "README.md", Status: "M", Added: 1, Removed: 0},
	}
	same := []ga.EventFile{
		{Path: "README.md", Status: "M", Added: 1, Removed: 0},
	}
	changed := []ga.EventFile{
		{Path: "README.md", Status: "M", Added: 2, Removed: 0},
	}

	if app.shouldRecordIndexActivity(repo, initial) {
		t.Fatalf("first snapshot should initialize baseline, not emit")
	}
	if app.shouldRecordIndexActivity(repo, same) {
		t.Fatalf("same staged snapshot should not emit")
	}
	if !app.shouldRecordIndexActivity(repo, changed) {
		t.Fatalf("changed staged snapshot should emit")
	}
	if app.shouldRecordIndexActivity(repo, changed) {
		t.Fatalf("same changed snapshot repeated should not emit")
	}
}

func TestMapLegacyGitEventToGitPanelInvalidation(t *testing.T) {
	tests := []struct {
		eventName     string
		wantStatus    bool
		wantHistory   bool
		wantConflicts bool
	}{
		{eventName: "git:index", wantStatus: true},
		{eventName: "git:merge", wantStatus: true, wantConflicts: true},
		{eventName: "git:branch_changed", wantStatus: true, wantHistory: true},
		{eventName: "git:commit", wantStatus: true, wantHistory: true},
		{eventName: "git:fetch", wantStatus: true},
		{eventName: "git:commit_preparing"},
		{eventName: "unknown:event"},
	}

	for _, tt := range tests {
		got := mapLegacyGitEventToGitPanelInvalidation(tt.eventName)
		if got.Status != tt.wantStatus || got.History != tt.wantHistory || got.Conflicts != tt.wantConflicts {
			t.Fatalf(
				"event=%s unexpected mapping got={status:%v history:%v conflicts:%v} want={status:%v history:%v conflicts:%v}",
				tt.eventName,
				got.Status,
				got.History,
				got.Conflicts,
				tt.wantStatus,
				tt.wantHistory,
				tt.wantConflicts,
			)
		}
	}
}

func TestExtractRepoPathFromGitEventData(t *testing.T) {
	repo := filepath.Clean("/tmp/repo")
	event := fw.FileEvent{
		Type: "index",
		Path: filepath.Join(repo, ".git", "index"),
	}

	fromEvent := extractRepoPathFromGitEventData(event)
	if fromEvent != repo {
		t.Fatalf("unexpected repo from file event: got=%q want=%q", fromEvent, repo)
	}

	fromMap := extractRepoPathFromGitEventData(map[string]string{
		"repoPath": repo,
	})
	if fromMap != repo {
		t.Fatalf("unexpected repo from map payload: got=%q want=%q", fromMap, repo)
	}

	fromPathMap := extractRepoPathFromGitEventData(map[string]interface{}{
		"path": filepath.Join(repo, ".git", "HEAD"),
	})
	if fromPathMap != repo {
		t.Fatalf("unexpected repo from path payload: got=%q want=%q", fromPathMap, repo)
	}
}
