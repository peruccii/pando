package main

import (
	"testing"

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
