package database

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newInMemoryDatabaseService(t *testing.T) *Service {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}

	if err := db.AutoMigrate(&Workspace{}, &AgentSession{}); err != nil {
		t.Fatalf("failed to migrate in-memory sqlite: %v", err)
	}

	return &Service{db: db}
}

func TestCreateAgentRejectsNil(t *testing.T) {
	svc := newInMemoryDatabaseService(t)

	err := svc.CreateAgent(nil)
	if err == nil {
		t.Fatalf("expected error for nil agent")
	}
}

func TestCreateAgentReturnsPersistedIdentity(t *testing.T) {
	svc := newInMemoryDatabaseService(t)

	ws := &Workspace{
		UserID:   "local",
		Name:     "Default",
		Path:     "",
		IsActive: true,
	}
	if err := svc.CreateWorkspace(ws); err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}

	agent := &AgentSession{
		WorkspaceID: ws.ID,
		Name:        "Terminal 1",
		Type:        "terminal",
		Status:      "idle",
		SortOrder:   0,
	}
	if err := svc.CreateAgent(agent); err != nil {
		t.Fatalf("CreateAgent returned error: %v", err)
	}

	if agent.ID == 0 {
		t.Fatalf("CreateAgent returned without assigning id")
	}
	if agent.WorkspaceID == 0 {
		t.Fatalf("CreateAgent returned without workspace id")
	}
}

func TestCreateAgentRejectsEmptyWorkspaceID(t *testing.T) {
	svc := newInMemoryDatabaseService(t)

	agent := &AgentSession{
		WorkspaceID: 0,
		Name:        "Terminal invalid",
		Type:        "terminal",
		Status:      "idle",
	}
	err := svc.CreateAgent(agent)
	if err == nil {
		t.Fatalf("expected error for empty workspace id")
	}

	if !strings.Contains(strings.ToLower(err.Error()), "workspace") {
		t.Fatalf("expected workspace-related error, got: %v", err)
	}
}

func TestMoveAgentToWorkspaceReindexesSortOrder(t *testing.T) {
	svc := newInMemoryDatabaseService(t)

	wsA := &Workspace{
		UserID:   "local",
		Name:     "A",
		Path:     "",
		IsActive: true,
	}
	if err := svc.CreateWorkspace(wsA); err != nil {
		t.Fatalf("failed to create workspace A: %v", err)
	}

	wsB := &Workspace{
		UserID:   "local",
		Name:     "B",
		Path:     "",
		IsActive: false,
	}
	if err := svc.CreateWorkspace(wsB); err != nil {
		t.Fatalf("failed to create workspace B: %v", err)
	}

	agentA1 := &AgentSession{
		WorkspaceID: wsA.ID,
		Name:        "Terminal A1",
		Type:        "terminal",
		Status:      "idle",
		SortOrder:   0,
	}
	if err := svc.CreateAgent(agentA1); err != nil {
		t.Fatalf("failed to create agentA1: %v", err)
	}

	agentA2 := &AgentSession{
		WorkspaceID: wsA.ID,
		Name:        "Terminal A2",
		Type:        "terminal",
		Status:      "idle",
		SortOrder:   1,
	}
	if err := svc.CreateAgent(agentA2); err != nil {
		t.Fatalf("failed to create agentA2: %v", err)
	}

	agentB1 := &AgentSession{
		WorkspaceID: wsB.ID,
		Name:        "Terminal B1",
		Type:        "terminal",
		Status:      "idle",
		SortOrder:   0,
	}
	if err := svc.CreateAgent(agentB1); err != nil {
		t.Fatalf("failed to create agentB1: %v", err)
	}

	moved, err := svc.MoveAgentToWorkspace(agentA1.ID, wsB.ID)
	if err != nil {
		t.Fatalf("MoveAgentToWorkspace returned error: %v", err)
	}

	if moved == nil {
		t.Fatalf("MoveAgentToWorkspace returned nil payload")
	}
	if moved.WorkspaceID != wsB.ID {
		t.Fatalf("expected moved agent workspace=%d, got=%d", wsB.ID, moved.WorkspaceID)
	}

	agentsA, err := svc.ListAgents(wsA.ID)
	if err != nil {
		t.Fatalf("failed to list agents from workspace A: %v", err)
	}
	if len(agentsA) != 1 {
		t.Fatalf("expected 1 agent in workspace A after move, got %d", len(agentsA))
	}
	if agentsA[0].ID != agentA2.ID {
		t.Fatalf("expected remaining agent in workspace A to be A2, got id=%d", agentsA[0].ID)
	}
	if agentsA[0].SortOrder != 0 {
		t.Fatalf("expected workspace A sort_order to be reindexed to 0, got %d", agentsA[0].SortOrder)
	}

	agentsB, err := svc.ListAgents(wsB.ID)
	if err != nil {
		t.Fatalf("failed to list agents from workspace B: %v", err)
	}
	if len(agentsB) != 2 {
		t.Fatalf("expected 2 agents in workspace B after move, got %d", len(agentsB))
	}
	if agentsB[0].ID != agentB1.ID || agentsB[0].SortOrder != 0 {
		t.Fatalf("expected first agent in workspace B to be B1 at sort_order=0, got id=%d order=%d", agentsB[0].ID, agentsB[0].SortOrder)
	}
	if agentsB[1].ID != agentA1.ID || agentsB[1].SortOrder != 1 {
		t.Fatalf("expected moved agent in workspace B at sort_order=1, got id=%d order=%d", agentsB[1].ID, agentsB[1].SortOrder)
	}
}
