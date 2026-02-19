package database

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"orch/internal/config"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Service encapsula o acesso ao SQLite via GORM
type Service struct {
	db *gorm.DB
}

var ErrLastWorkspace = errors.New("cannot delete the last workspace")

// NewService cria e inicializa o serviço de banco de dados
func NewService() (*Service, error) {
	dbPath, db, err := openWritableDatabase()
	if err != nil {
		return nil, err
	}

	// Auto-migrate todos os models
	if err := db.AutoMigrate(
		&UserConfig{},
		&Workspace{},
		&AgentSession{},
		&ChatHistory{},
		&SessionHistory{},
		&CollabSessionState{},
		&AuditLog{},
		&TerminalSnapshot{},
	); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate: %w", err)
	}

	svc := &Service{db: db}
	if err := svc.ensureDefaultWorkspace(); err != nil {
		return nil, fmt.Errorf("failed to ensure default workspace: %w", err)
	}

	// Definir permissão 0600 no arquivo do banco
	os.Chmod(dbPath, 0600)

	log.Printf("[DB] Database initialized at %s", dbPath)
	return svc, nil
}

func openWritableDatabase() (string, *gorm.DB, error) {
	candidates := make([]string, 0, 3)
	if override := strings.TrimSpace(os.Getenv("ORCH_DB_PATH")); override != "" {
		candidates = append(candidates, override)
	}
	candidates = append(candidates, config.DBPath())

	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		candidates = append(candidates, filepath.Join(cwd, ".orch", config.DBFileName))
	}
	candidates = append(candidates, filepath.Join(os.TempDir(), "ORCH", config.DBFileName))

	var lastErr error
	for _, candidate := range candidates {
		path := strings.TrimSpace(candidate)
		if path == "" {
			continue
		}

		if !isLikelyWritable(path) {
			lastErr = fmt.Errorf("path not writable: %s", path)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			lastErr = err
			continue
		}

		db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Warn),
		})
		if err != nil {
			lastErr = err
			continue
		}

		sqlDB, err := db.DB()
		if err != nil {
			lastErr = err
			continue
		}

		sqlDB.Exec("PRAGMA journal_mode=WAL")
		sqlDB.Exec("PRAGMA busy_timeout=5000")
		sqlDB.Exec("PRAGMA synchronous=NORMAL")
		sqlDB.Exec("PRAGMA foreign_keys=ON")

		// Probe de escrita para evitar abrir DB readonly em ambientes sandbox.
		probeErr := db.Exec("CREATE TABLE IF NOT EXISTS _orch_write_probe (id INTEGER PRIMARY KEY AUTOINCREMENT)").Error
		if probeErr == nil {
			probeErr = db.Exec("INSERT INTO _orch_write_probe DEFAULT VALUES").Error
		}
		if probeErr == nil {
			_ = db.Exec("DELETE FROM _orch_write_probe WHERE id = (SELECT MAX(id) FROM _orch_write_probe)").Error
		}

		if probeErr != nil {
			lastErr = probeErr
			_ = sqlDB.Close()
			continue
		}

		return path, db, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no database path candidates available")
	}

	return "", nil, fmt.Errorf("failed to open writable database: %w", lastErr)
}

func isLikelyWritable(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// Close fecha a conexão com o banco
func (s *Service) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Service) ensureDefaultWorkspace() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&Workspace{}).Count(&count).Error; err != nil {
			return err
		}

		if count == 0 {
			defaultWorkspace := &Workspace{
				UserID:   "local",
				Name:     "Default",
				Path:     "",
				IsActive: true,
			}
			return tx.Create(defaultWorkspace).Error
		}

		var activeCount int64
		if err := tx.Model(&Workspace{}).Where("is_active = ?", true).Count(&activeCount).Error; err != nil {
			return err
		}

		if activeCount > 0 {
			return nil
		}

		var candidate Workspace
		if err := tx.Order("updated_at DESC, id DESC").First(&candidate).Error; err != nil {
			return err
		}

		return tx.Model(&Workspace{}).Where("id = ?", candidate.ID).Update("is_active", true).Error
	})
}

// === UserConfig CRUD ===

// GetConfig retorna a configuração do usuário (ou cria uma padrão)
func (s *Service) GetConfig() (*UserConfig, error) {
	var cfg UserConfig
	result := s.db.First(&cfg)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Criar config padrão
			cfg = UserConfig{
				UserID:              "local",
				Theme:               "dark",
				Language:            "pt-BR",
				OnboardingCompleted: false,
				DefaultShell:        "",
				FontSize:            14,
				FontFamily:          "JetBrains Mono",
				CursorStyle:         "line",
				ShortcutBindings:    "",
			}
			s.db.Create(&cfg)
			return &cfg, nil
		}
		return nil, result.Error
	}
	return &cfg, nil
}

// GetConfigByUserID retorna a config de um usuário específico
func (s *Service) GetConfigByUserID(userID string) (*UserConfig, error) {
	var cfg UserConfig
	result := s.db.Where("user_id = ?", userID).First(&cfg)
	if result.Error != nil {
		return nil, result.Error
	}
	return &cfg, nil
}

// UpdateConfig atualiza configurações do usuário
func (s *Service) UpdateConfig(cfg *UserConfig) error {
	return s.db.Save(cfg).Error
}

// === Workspace CRUD ===

// ListWorkspaces retorna todos os workspaces
func (s *Service) ListWorkspaces() ([]Workspace, error) {
	var workspaces []Workspace
	result := s.db.Order("is_active DESC, updated_at DESC, id DESC").Find(&workspaces)
	return workspaces, result.Error
}

// CountAgents retorna a quantidade total de agentes persistidos.
func (s *Service) CountAgents() (int64, error) {
	var count int64
	err := s.db.Model(&AgentSession{}).Count(&count).Error
	return count, err
}

// GetWorkspacesWithAgents retorna a árvore hierárquica de workspaces + agentes.
func (s *Service) GetWorkspacesWithAgents() ([]Workspace, error) {
	var workspaces []Workspace
	err := s.db.
		Preload("Agents", func(db *gorm.DB) *gorm.DB {
			return db.Order("sort_order ASC, id ASC")
		}).
		Order("is_active DESC, updated_at DESC, id DESC").
		Find(&workspaces).Error
	return workspaces, err
}

// GetWorkspace retorna um workspace por ID
func (s *Service) GetWorkspace(id uint) (*Workspace, error) {
	var ws Workspace
	result := s.db.First(&ws, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &ws, nil
}

// GetActiveWorkspace retorna o workspace ativo
func (s *Service) GetActiveWorkspace() (*Workspace, error) {
	var ws Workspace
	result := s.db.Where("is_active = ?", true).First(&ws)
	if result.Error != nil {
		return nil, result.Error
	}
	return &ws, nil
}

// CreateWorkspace cria um novo workspace
func (s *Service) CreateWorkspace(ws *Workspace) error {
	ws.Name = strings.TrimSpace(ws.Name)
	if ws.Name == "" {
		ws.Name = "Workspace"
	}

	ws.UserID = strings.TrimSpace(ws.UserID)
	if ws.UserID == "" {
		ws.UserID = "local"
	}

	if ws.Path == "" {
		ws.Path = ""
	}

	return s.db.Create(ws).Error
}

// RenameWorkspace atualiza o nome de um workspace.
func (s *Service) RenameWorkspace(id uint, name string) error {
	newName := strings.TrimSpace(name)
	if newName == "" {
		return fmt.Errorf("workspace name cannot be empty")
	}

	return s.db.Model(&Workspace{}).Where("id = ?", id).Update("name", newName).Error
}

// SetWorkspaceColor atualiza a cor de um workspace.
func (s *Service) SetWorkspaceColor(id uint, color string) error {
	return s.db.Model(&Workspace{}).Where("id = ?", id).Update("color", color).Error
}

// SetActiveWorkspace define qual workspace está ativo (desativa os outros)
func (s *Service) SetActiveWorkspace(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var target Workspace
		if err := tx.First(&target, id).Error; err != nil {
			return err
		}

		// Desativar todos
		if err := tx.Model(&Workspace{}).Where("is_active = ?", true).Update("is_active", false).Error; err != nil {
			return err
		}
		// Ativar o selecionado
		return tx.Model(&Workspace{}).Where("id = ?", id).Update("is_active", true).Error
	})
}

// DeleteWorkspace remove um workspace e seus agentes
func (s *Service) DeleteWorkspace(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var total int64
		if err := tx.Model(&Workspace{}).Count(&total).Error; err != nil {
			return err
		}

		if total <= 1 {
			return ErrLastWorkspace
		}

		var target Workspace
		if err := tx.First(&target, id).Error; err != nil {
			return err
		}

		// Fallback explícito: garante remoção dos agentes mesmo em bancos legados sem FK cascade.
		if err := tx.Where("workspace_id = ?", id).Delete(&AgentSession{}).Error; err != nil {
			return err
		}

		if err := tx.Delete(&Workspace{}, id).Error; err != nil {
			return err
		}

		if !target.IsActive {
			return nil
		}

		var replacement Workspace
		if err := tx.Order("updated_at DESC, id DESC").First(&replacement).Error; err != nil {
			return err
		}

		return tx.Model(&Workspace{}).Where("id = ?", replacement.ID).Update("is_active", true).Error
	})
}

// === AgentSession CRUD ===

// ListAgents retorna todos os agentes de um workspace
func (s *Service) ListAgents(workspaceID uint) ([]AgentSession, error) {
	var agents []AgentSession
	result := s.db.Where("workspace_id = ?", workspaceID).Order("sort_order ASC").Find(&agents)
	return agents, result.Error
}

// GetAgent retorna um agente por ID.
func (s *Service) GetAgent(id uint) (*AgentSession, error) {
	var agent AgentSession
	result := s.db.First(&agent, id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &agent, nil
}

// CreateAgent cria um novo agente
func (s *Service) CreateAgent(agent *AgentSession) error {
	if agent == nil {
		return fmt.Errorf("agent cannot be nil")
	}

	if err := s.db.Create(agent).Error; err != nil {
		return err
	}

	// Contrato de persistência: um agente criado precisa voltar com ID e workspace válidos.
	if agent.ID == 0 {
		return fmt.Errorf("agent persisted with empty id")
	}
	if agent.WorkspaceID == 0 {
		return fmt.Errorf("agent persisted with empty workspace id")
	}

	return nil
}

// UpdateAgent atualiza um agente
func (s *Service) UpdateAgent(agent *AgentSession) error {
	return s.db.Save(agent).Error
}

// MoveAgentToWorkspace move um agente para outro workspace e reindexa a ordenação
// dos painéis de origem/destino para manter sort_order contínuo e sem colisões.
func (s *Service) MoveAgentToWorkspace(agentID uint, targetWorkspaceID uint) (*AgentSession, error) {
	log.Printf("[ORCH][MOVE][DB] begin agentID=%d targetWorkspaceID=%d", agentID, targetWorkspaceID)

	if agentID == 0 {
		log.Printf("[ORCH][MOVE][DB] invalid input: empty agent id")
		return nil, fmt.Errorf("agent id cannot be empty")
	}
	if targetWorkspaceID == 0 {
		log.Printf("[ORCH][MOVE][DB] invalid input: empty target workspace id")
		return nil, fmt.Errorf("target workspace id cannot be empty")
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var target Workspace
		if err := tx.First(&target, targetWorkspaceID).Error; err != nil {
			log.Printf("[ORCH][MOVE][DB] target workspace lookup failed workspaceID=%d err=%v", targetWorkspaceID, err)
			return err
		}

		var agent AgentSession
		if err := tx.First(&agent, agentID).Error; err != nil {
			log.Printf("[ORCH][MOVE][DB] agent lookup failed agentID=%d err=%v", agentID, err)
			return err
		}

		sourceWorkspaceID := agent.WorkspaceID
		log.Printf(
			"[ORCH][MOVE][DB] loaded agentID=%d sourceWorkspaceID=%d targetWorkspaceID=%d sourceSortOrder=%d",
			agentID,
			sourceWorkspaceID,
			targetWorkspaceID,
			agent.SortOrder,
		)

		if sourceWorkspaceID == targetWorkspaceID {
			log.Printf("[ORCH][MOVE][DB] no-op: source and target workspace are identical workspaceID=%d", sourceWorkspaceID)
			return nil
		}

		var targetCount int64
		if err := tx.Model(&AgentSession{}).
			Where("workspace_id = ?", targetWorkspaceID).
			Count(&targetCount).Error; err != nil {
			log.Printf("[ORCH][MOVE][DB] failed to count target workspace agents workspaceID=%d err=%v", targetWorkspaceID, err)
			return err
		}
		log.Printf("[ORCH][MOVE][DB] target workspace current agent count=%d workspaceID=%d", targetCount, targetWorkspaceID)

		if err := tx.Model(&AgentSession{}).
			Where("id = ?", agentID).
			Updates(map[string]interface{}{
				"workspace_id": targetWorkspaceID,
				"sort_order":   int(targetCount),
			}).Error; err != nil {
			log.Printf("[ORCH][MOVE][DB] failed to update agent workspace agentID=%d targetWorkspaceID=%d err=%v", agentID, targetWorkspaceID, err)
			return err
		}

		// Ativação atômica: garante que ao mover, a workspace de destino se torne a ativa
		if err := tx.Model(&Workspace{}).Where("is_active = ?", true).Update("is_active", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&Workspace{}).Where("id = ?", targetWorkspaceID).Update("is_active", true).Error; err != nil {
			return err
		}

		log.Printf(
			"[ORCH][MOVE][DB] agent updated and workspace activated agentID=%d targetWorkspaceID=%d newSortOrder=%d",
			agentID,
			targetWorkspaceID,
			targetCount,
		)

		if err := s.reindexWorkspaceAgentsTx(tx, sourceWorkspaceID); err != nil {
			log.Printf("[ORCH][MOVE][DB] failed reindex source workspace workspaceID=%d err=%v", sourceWorkspaceID, err)
			return err
		}
		if err := s.reindexWorkspaceAgentsTx(tx, targetWorkspaceID); err != nil {
			log.Printf("[ORCH][MOVE][DB] failed reindex target workspace workspaceID=%d err=%v", targetWorkspaceID, err)
			return err
		}

		log.Printf(
			"[ORCH][MOVE][DB] transaction stage completed agentID=%d sourceWorkspaceID=%d targetWorkspaceID=%d",
			agentID,
			sourceWorkspaceID,
			targetWorkspaceID,
		)

		return nil
	})
	if err != nil {
		log.Printf("[ORCH][MOVE][DB] transaction failed agentID=%d targetWorkspaceID=%d err=%v", agentID, targetWorkspaceID, err)
		return nil, err
	}

	moved, err := s.GetAgent(agentID)
	if err != nil {
		log.Printf("[ORCH][MOVE][DB] failed to reload moved agent agentID=%d err=%v", agentID, err)
		return nil, err
	}

	if moved == nil {
		log.Printf("[ORCH][MOVE][DB] reload returned nil agent agentID=%d", agentID)
		return nil, fmt.Errorf("moved agent not found after transaction")
	}

	log.Printf(
		"[ORCH][MOVE][DB] success agentID=%d finalWorkspaceID=%d finalSortOrder=%d sessionID=%q",
		moved.ID,
		moved.WorkspaceID,
		moved.SortOrder,
		moved.SessionID,
	)

	return moved, nil
}

// UpdateAgentLayout atualiza apenas o layout JSON de um agente
func (s *Service) UpdateAgentLayout(id uint, layoutJSON string) error {
	return s.db.Model(&AgentSession{}).Where("id = ?", id).Update("layout_json", layoutJSON).Error
}

// UpdateAgentRuntime atualiza metadados de runtime da sessão de agente.
func (s *Service) UpdateAgentRuntime(id uint, sessionID, shell, cwd string, useDocker bool, status string) error {
	updates := map[string]interface{}{
		"session_id": sessionID,
		"shell":      shell,
		"cwd":        cwd,
		"use_docker": useDocker,
		"status":     status,
	}
	return s.db.Model(&AgentSession{}).Where("id = ?", id).Updates(updates).Error
}

// ClearAgentRuntime limpa o vínculo com sessão de terminal ativa.
func (s *Service) ClearAgentRuntime(id uint) error {
	updates := map[string]interface{}{
		"session_id": "",
		"status":     "idle",
	}
	return s.db.Model(&AgentSession{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteAgent remove um agente
func (s *Service) DeleteAgent(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Remover chat history do agente
		if err := tx.Where("agent_id = ?", id).Delete(&ChatHistory{}).Error; err != nil {
			return err
		}
		return tx.Delete(&AgentSession{}, id).Error
	})
}

func (s *Service) reindexWorkspaceAgentsTx(tx *gorm.DB, workspaceID uint) error {
	if workspaceID == 0 {
		return fmt.Errorf("workspace id cannot be empty")
	}

	var agents []AgentSession
	if err := tx.Where("workspace_id = ?", workspaceID).
		Order("sort_order ASC, id ASC").
		Find(&agents).Error; err != nil {
		return err
	}

	for idx, agent := range agents {
		if agent.SortOrder == idx {
			continue
		}
		if err := tx.Model(&AgentSession{}).
			Where("id = ?", agent.ID).
			Update("sort_order", idx).Error; err != nil {
			return err
		}
		log.Printf(
			"[ORCH][MOVE][DB] reindex workspaceID=%d agentID=%d oldSortOrder=%d newSortOrder=%d",
			workspaceID,
			agent.ID,
			agent.SortOrder,
			idx,
		)
	}

	return nil
}

// === ChatHistory CRUD ===

// GetChatHistory retorna o histórico de chat de um agente
func (s *Service) GetChatHistory(agentID uint, limit int) ([]ChatHistory, error) {
	var history []ChatHistory
	result := s.db.Where("agent_id = ?", agentID).Order("created_at DESC").Limit(limit).Find(&history)
	return history, result.Error
}

// SaveChatMessage salva uma mensagem no histórico
func (s *Service) SaveChatMessage(msg *ChatHistory) error {
	return s.db.Create(msg).Error
}

// ClearChatHistory limpa o histórico de chat de um agente
func (s *Service) ClearChatHistory(agentID uint) error {
	return s.db.Where("agent_id = ?", agentID).Delete(&ChatHistory{}).Error
}

// === AuditLog CRUD ===

// SaveAuditEvent salva um evento auditável e aplica retenção das últimas 1000 entradas por sessão.
func (s *Service) SaveAuditEvent(event *AuditLog) error {
	if event == nil {
		return fmt.Errorf("audit event is nil")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(event).Error; err != nil {
			return err
		}

		// Mantém apenas os 1000 eventos mais recentes por sessão.
		return tx.Exec(`
			DELETE FROM audit_logs
			WHERE session_id = ?
			  AND id NOT IN (
				SELECT id
				FROM audit_logs
				WHERE session_id = ?
				ORDER BY created_at DESC, id DESC
				LIMIT 1000
			  )
		`, event.SessionID, event.SessionID).Error
	})
}

// ListAuditEvents lista eventos de auditoria de uma sessão em ordem decrescente.
func (s *Service) ListAuditEvents(sessionID string, limit int) ([]AuditLog, error) {
	if limit <= 0 {
		limit = 100
	}

	var logs []AuditLog
	err := s.db.Where("session_id = ?", sessionID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

// === CollabSessionState CRUD ===

// UpsertCollabSessionState cria/atualiza o snapshot persistido de uma sessão colaborativa.
func (s *Service) UpsertCollabSessionState(state *CollabSessionState) error {
	if state == nil {
		return fmt.Errorf("collab session state is nil")
	}
	if strings.TrimSpace(state.SessionID) == "" {
		return fmt.Errorf("sessionID is required")
	}

	var existing CollabSessionState
	err := s.db.Where("session_id = ?", state.SessionID).First(&existing).Error
	if err == nil {
		updates := map[string]interface{}{
			"host_user_id":  state.HostUserID,
			"code":          state.Code,
			"status":        state.Status,
			"expires_at":    state.ExpiresAt,
			"persist_until": state.PersistUntil,
			"payload":       state.Payload,
		}
		return s.db.Model(&CollabSessionState{}).Where("session_id = ?", state.SessionID).Updates(updates).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.db.Create(state).Error
}

// DeleteCollabSessionState remove um snapshot persistido por sessionID.
func (s *Service) DeleteCollabSessionState(sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	return s.db.Where("session_id = ?", sessionID).Delete(&CollabSessionState{}).Error
}

// ListRestorableCollabSessionStates lista sessões ainda válidas para restore.
func (s *Service) ListRestorableCollabSessionStates(now time.Time, limit int) ([]CollabSessionState, error) {
	if limit <= 0 {
		limit = 200
	}
	var states []CollabSessionState
	err := s.db.
		Where("persist_until > ?", now).
		Where("status <> ?", "ended").
		Order("updated_at DESC, id DESC").
		Limit(limit).
		Find(&states).Error
	return states, err
}

// CleanupExpiredCollabSessionStates remove snapshots vencidos e/ou finalizados.
func (s *Service) CleanupExpiredCollabSessionStates(now time.Time) error {
	return s.db.
		Where("persist_until <= ? OR status = ?", now, "ended").
		Delete(&CollabSessionState{}).
		Error
}

// === TerminalSnapshot CRUD ===

// SaveTerminalSnapshots apaga snapshots antigos e salva os novos (replace all).
func (s *Service) SaveTerminalSnapshots(snapshots []TerminalSnapshot) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Limpar snapshots anteriores
		if err := tx.Where("1 = 1").Delete(&TerminalSnapshot{}).Error; err != nil {
			return err
		}
		// Inserir novos
		if len(snapshots) == 0 {
			return nil
		}
		return tx.Create(&snapshots).Error
	})
}

// GetTerminalSnapshots retorna todos os snapshots salvos.
func (s *Service) GetTerminalSnapshots() ([]TerminalSnapshot, error) {
	var snapshots []TerminalSnapshot
	err := s.db.Order("id ASC").Find(&snapshots).Error
	return snapshots, err
}

// ClearTerminalSnapshots remove todos os snapshots.
func (s *Service) ClearTerminalSnapshots() error {
	return s.db.Where("1 = 1").Delete(&TerminalSnapshot{}).Error
}
