package main

import (
	"strings"
	"testing"

	"orch/internal/database"
)

func newTestAppWithDatabase(t *testing.T) *App {
	t.Helper()

	t.Setenv("HOME", t.TempDir())

	db, err := database.NewService()
	if err != nil {
		t.Fatalf("failed to init test database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	app := NewApp()
	app.db = db
	return app
}

func TestCreateAgentSessionReturnsPersistedPayload(t *testing.T) {
	app := newTestAppWithDatabase(t)

	agent, err := app.CreateAgentSession(0, "", "")
	if err != nil {
		t.Fatalf("CreateAgentSession returned error: %v", err)
	}
	if agent == nil {
		t.Fatalf("CreateAgentSession returned nil payload with nil error")
	}
	if agent.ID == 0 {
		t.Fatalf("CreateAgentSession returned payload with empty id")
	}
	if agent.WorkspaceID == 0 {
		t.Fatalf("CreateAgentSession returned payload with empty workspace id")
	}
	if strings.TrimSpace(agent.Name) == "" {
		t.Fatalf("CreateAgentSession returned payload with empty name")
	}
	if strings.TrimSpace(agent.Type) == "" {
		t.Fatalf("CreateAgentSession returned payload with empty type")
	}

	persisted, err := app.db.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("failed to load created agent from db: %v", err)
	}
	if persisted == nil {
		t.Fatalf("db returned nil agent for created id=%d", agent.ID)
	}
	if persisted.ID != agent.ID {
		t.Fatalf("created payload id mismatch: payload=%d db=%d", agent.ID, persisted.ID)
	}
}

func TestCreateAgentSessionDatabaseNotInitialized(t *testing.T) {
	app := NewApp()

	agent, err := app.CreateAgentSession(0, "Terminal", "terminal")
	if err == nil {
		t.Fatalf("expected error when db is nil, got payload=%+v", agent)
	}
}

func TestCreateAgentSessionRejectsLegacyGitHubType(t *testing.T) {
	app := newTestAppWithDatabase(t)

	agent, err := app.CreateAgentSession(0, "Legacy GitHub", "github")
	if err == nil {
		t.Fatalf("expected error for deprecated github agent type, got payload=%+v", agent)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "descontinuado") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestGetWorkspacesWithAgentsMigratesLegacyGitHubAgents(t *testing.T) {
	app := newTestAppWithDatabase(t)

	workspace, err := app.db.GetActiveWorkspace()
	if err != nil {
		t.Fatalf("failed to load active workspace: %v", err)
	}
	if workspace == nil || workspace.ID == 0 {
		t.Fatalf("invalid active workspace payload")
	}

	terminalAgent := &database.AgentSession{
		WorkspaceID: workspace.ID,
		Name:        "Terminal 1",
		Type:        "terminal",
		Shell:       "/bin/zsh",
		Status:      "idle",
		SortOrder:   0,
	}
	if err := app.db.CreateAgent(terminalAgent); err != nil {
		t.Fatalf("failed to create terminal agent: %v", err)
	}

	legacyGitHubAgent := &database.AgentSession{
		WorkspaceID: workspace.ID,
		Name:        "GitHub Legacy",
		Type:        "github",
		Shell:       "/bin/zsh",
		Status:      "idle",
		SortOrder:   1,
	}
	if err := app.db.CreateAgent(legacyGitHubAgent); err != nil {
		t.Fatalf("failed to create github legacy agent: %v", err)
	}

	workspaces, err := app.GetWorkspacesWithAgents()
	if err != nil {
		t.Fatalf("GetWorkspacesWithAgents returned error: %v", err)
	}

	var selected *database.Workspace
	for i := range workspaces {
		if workspaces[i].ID == workspace.ID {
			selected = &workspaces[i]
			break
		}
	}
	if selected == nil {
		t.Fatalf("workspace %d not found in response", workspace.ID)
	}

	for _, agent := range selected.Agents {
		if strings.EqualFold(strings.TrimSpace(agent.Type), "github") {
			t.Fatalf("legacy github agent leaked in response payload")
		}
	}

	persistedAgents, err := app.db.ListAgents(workspace.ID)
	if err != nil {
		t.Fatalf("failed to list persisted agents: %v", err)
	}
	for _, agent := range persistedAgents {
		if strings.EqualFold(strings.TrimSpace(agent.Type), "github") {
			t.Fatalf("legacy github agent was not migrated from database")
		}
	}
}
