package main

import (
	"path/filepath"
	"strings"
	"testing"

	"orch/internal/database"
	"orch/internal/session"
	"orch/internal/terminal"
)

func newScopedPermissionTestApp(t *testing.T) (*App, *database.Service, *database.Workspace, *database.Workspace) {
	t.Helper()

	app, db := newAppWithIsolatedDB(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	app.session = session.NewService(nil)
	app.sessionGatewayOwner = true

	scopedWorkspace, err := db.GetActiveWorkspace()
	if err != nil {
		t.Fatalf("GetActiveWorkspace() error: %v", err)
	}

	otherWorkspace := &database.Workspace{
		UserID:   "local",
		Name:     "Other Workspace",
		Path:     filepath.Join(t.TempDir(), "other-workspace"),
		IsActive: false,
	}
	if err := db.CreateWorkspace(otherWorkspace); err != nil {
		t.Fatalf("CreateWorkspace(other) error: %v", err)
	}

	return app, db, scopedWorkspace, otherWorkspace
}

func createApprovedGuestInScopedSession(
	t *testing.T,
	app *App,
	workspaceID uint,
	defaultPerm session.Permission,
	guestUserID string,
) *session.Session {
	t.Helper()

	created, err := app.session.CreateSession("local", session.SessionConfig{
		WorkspaceID:    workspaceID,
		DefaultPerm:    defaultPerm,
		AllowAnonymous: true,
	})
	if err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}

	if _, err := app.session.JoinSession(created.Code, guestUserID, session.GuestInfo{Name: "Guest"}); err != nil {
		t.Fatalf("JoinSession() error: %v", err)
	}
	if err := app.session.ApproveGuest(created.ID, guestUserID); err != nil {
		t.Fatalf("ApproveGuest() error: %v", err)
	}

	return created
}

func bindTerminalToWorkspace(t *testing.T, app *App, db *database.Service, workspaceID uint, terminalSessionID string) uint {
	t.Helper()

	agent := &database.AgentSession{
		WorkspaceID: workspaceID,
		Name:        "Scoped Terminal",
		Type:        "terminal",
		Cwd:         "",
		Shell:       "/bin/zsh",
	}
	if err := db.CreateAgent(agent); err != nil {
		t.Fatalf("CreateAgent() error: %v", err)
	}

	app.bindTerminalToAgent(terminalSessionID, agent.ID)
	return agent.ID
}

func TestResolveGuestTerminalPermission_AllowsTerminalInsideScopedWorkspace(t *testing.T) {
	app, db, scopedWorkspace, _ := newScopedPermissionTestApp(t)

	_ = createApprovedGuestInScopedSession(t, app, scopedWorkspace.ID, session.PermReadWrite, "guest-in")
	bindTerminalToWorkspace(t, app, db, scopedWorkspace.ID, "term-inside")

	perm, err := app.resolveGuestTerminalPermission("term-inside", "guest-in")
	if err != nil {
		t.Fatalf("resolveGuestTerminalPermission() error: %v", err)
	}
	if perm != terminal.PermissionReadWrite {
		t.Fatalf("permission = %q, want %q", perm, terminal.PermissionReadWrite)
	}
}

func TestResolveGuestTerminalPermission_RejectsTerminalOutsideScopedWorkspace(t *testing.T) {
	app, db, scopedWorkspace, otherWorkspace := newScopedPermissionTestApp(t)

	_ = createApprovedGuestInScopedSession(t, app, scopedWorkspace.ID, session.PermReadWrite, "guest-out")
	bindTerminalToWorkspace(t, app, db, otherWorkspace.ID, "term-outside")

	perm, err := app.resolveGuestTerminalPermission("term-outside", "guest-out")
	if err == nil {
		t.Fatalf("expected error for terminal outside scoped workspace")
	}
	if !strings.Contains(err.Error(), "outside scoped workspace") {
		t.Fatalf("unexpected error: %v", err)
	}
	if perm != terminal.PermissionNone {
		t.Fatalf("permission = %q, want %q on error", perm, terminal.PermissionNone)
	}
}
