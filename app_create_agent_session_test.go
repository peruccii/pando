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

