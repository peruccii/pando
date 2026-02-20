package main

import (
	"path/filepath"
	"testing"

	"orch/internal/database"
)

func newAppWithIsolatedDB(t *testing.T) (*App, *database.Service) {
	t.Helper()

	tempRoot := t.TempDir()
	t.Setenv("HOME", tempRoot)
	t.Setenv("ORCH_DB_PATH", filepath.Join(tempRoot, "orch-sync-workspace.db"))

	db, err := database.NewService()
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}

	app := NewApp()
	app.db = db
	return app, db
}

func TestSyncGuestWorkspaceCreatesAndReusesWorkspace(t *testing.T) {
	app, db := newAppWithIsolatedDB(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	before, err := db.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces() before sync failed: %v", err)
	}

	first, err := app.SyncGuestWorkspace("Team")
	if err != nil {
		t.Fatalf("SyncGuestWorkspace() first call failed: %v", err)
	}
	if first == nil || first.ID == 0 {
		t.Fatalf("first sync returned invalid workspace: %+v", first)
	}
	if first.Name != "Team" {
		t.Fatalf("workspace name = %q, want Team", first.Name)
	}

	mid, err := db.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces() after first sync failed: %v", err)
	}
	if len(mid) != len(before)+1 {
		t.Fatalf("workspace count after first sync = %d, want %d", len(mid), len(before)+1)
	}

	second, err := app.SyncGuestWorkspace("Team")
	if err != nil {
		t.Fatalf("SyncGuestWorkspace() second call failed: %v", err)
	}
	if second == nil {
		t.Fatalf("second sync returned nil workspace")
	}
	if second.ID != first.ID {
		t.Fatalf("second sync workspace id = %d, want %d", second.ID, first.ID)
	}

	after, err := db.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces() after second sync failed: %v", err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("workspace count after second sync = %d, want %d", len(after), len(before)+1)
	}
}

func TestSyncGuestWorkspaceIsCaseInsensitive(t *testing.T) {
	app, db := newAppWithIsolatedDB(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	first, err := app.SyncGuestWorkspace("Team Shared")
	if err != nil {
		t.Fatalf("SyncGuestWorkspace() first call failed: %v", err)
	}
	second, err := app.SyncGuestWorkspace("team shared")
	if err != nil {
		t.Fatalf("SyncGuestWorkspace() second call failed: %v", err)
	}

	if second.ID != first.ID {
		t.Fatalf("case-insensitive sync created duplicate workspace id=%d want=%d", second.ID, first.ID)
	}
}
