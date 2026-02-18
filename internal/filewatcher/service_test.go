package filewatcher

import (
	"testing"
	"time"
)

func TestSemanticEventKey(t *testing.T) {
	event := FileEvent{
		Type: "branch_changed",
		Path: "/tmp/repo/.git/HEAD.lock",
		Details: map[string]string{
			"branch": "feature/login",
		},
	}

	key := semanticEventKey(event)
	want := "branch_changed|/tmp/repo/.git/HEAD|branch=feature/login"
	if key != want {
		t.Fatalf("unexpected semantic key. got=%q want=%q", key, want)
	}
}

func TestShouldEmitDedupesWithinWindow(t *testing.T) {
	svc := &Service{
		recent: make(map[string]time.Time),
		window: 80 * time.Millisecond,
	}

	event := FileEvent{
		Type: "index",
		Path: "/tmp/repo/.git/index.lock",
	}

	if !svc.shouldEmit(event) {
		t.Fatalf("first event should be emitted")
	}
	if svc.shouldEmit(event) {
		t.Fatalf("second event inside dedupe window should be ignored")
	}

	time.Sleep(100 * time.Millisecond)

	if !svc.shouldEmit(event) {
		t.Fatalf("event should be emitted again after dedupe window")
	}
}
