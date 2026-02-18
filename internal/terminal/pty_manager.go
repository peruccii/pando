package terminal

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
)

// PTYManager gerencia múltiplas sessões de terminal PTY
type PTYManager struct {
	sessions map[string]*PTYSession
	mu       sync.RWMutex
	seq      atomic.Uint64
}

// NewPTYManager cria um novo gerenciador de terminais
func NewPTYManager() *PTYManager {
	return &PTYManager{
		sessions: make(map[string]*PTYSession),
	}
}

// Create cria uma nova sessão PTY
func (m *PTYManager) Create(config PTYConfig) (string, error) {
	// Defaults
	if config.Shell == "" {
		if config.UseDocker {
			config.Shell = "/bin/sh"
		} else {
			config.Shell = DefaultShell()
		}
	}
	if config.Cols == 0 {
		config.Cols = 80
	}
	if config.Rows == 0 {
		config.Rows = 24
	}
	if config.Cwd == "" {
		home, _ := os.UserHomeDir()
		config.Cwd = home
	}

	// Configurar ambiente de forma robusta
	env := os.Environ()
	envMap := make(map[string]string)
	for _, e := range env {
		kv := strings.SplitN(e, "=", 2)
		if len(kv) == 2 {
			envMap[kv[0]] = kv[1]
		}
	}

	// Aplicar overrides da config do Orch
	for _, e := range config.Env {
		kv := strings.SplitN(e, "=", 2)
		if len(kv) == 2 {
			envMap[kv[0]] = kv[1]
		}
	}

	// Ambiente previsível para TUI dentro do xterm.js:
	// sem "hacks" de emulação e com locale UTF-8 consistente.
	delete(envMap, "TERM_PROGRAM")
	delete(envMap, "TERM_PROGRAM_VERSION")
	delete(envMap, "XTERM_VERSION")
	delete(envMap, "ITERM_SESSION_ID")
	delete(envMap, "ITERM_PROFILE")
	delete(envMap, "LC_TERMINAL")
	delete(envMap, "LC_TERMINAL_VERSION")
	delete(envMap, "VTE_VERSION")

	envMap["TERM"] = "xterm-256color"
	envMap["COLORTERM"] = "truecolor"
	envMap["TERM_PROGRAM"] = "orch"
	envMap["TERM_PROGRAM_VERSION"] = "1"
	envMap["NVIM_TUI_ENABLE_TRUE_COLOR"] = "1"
	if _, ok := envMap["LANG"]; !ok {
		envMap["LANG"] = "en_US.UTF-8"
	}
	if _, ok := envMap["LC_ALL"]; !ok {
		envMap["LC_ALL"] = envMap["LANG"]
	}
	if _, ok := envMap["LC_CTYPE"]; !ok {
		envMap["LC_CTYPE"] = envMap["LANG"]
	}

	var cmd *exec.Cmd
	if config.UseDocker {
		if config.DockerImage == "" {
			config.DockerImage = "alpine:latest"
		}

		args := []string{"run", "--rm", "-it"}

		// Passar variáveis de ambiente para o container via flag -e
		for k, v := range envMap {
			// Evitar passar variáveis do host que não fazem sentido no container
			if k == "PATH" || k == "HOME" || k == "PWD" || k == "USER" || k == "SHELL" {
				continue
			}
			args = append(args, "-e", k+"="+v)
		}

		args = append(args, "--name", "orch-pty-"+uuid.NewString()[:8])

		if mount := strings.TrimSpace(config.DockerMount); mount != "" {
			safeMount := filepath.Clean(mount)
			args = append(args, "-v", fmt.Sprintf("%s:/workspace", safeMount), "-w", "/workspace")
		}

		args = append(args, config.DockerImage, config.Shell)
		cmd = exec.Command("docker", args...)
	} else {
		// Modo Live Share/local: shell local no diretório corrente.
		cmd = exec.Command(config.Shell)
		cmd.Dir = config.Cwd
	}

	// Reconstruir slice de ambiente para o processo (local ou comando docker)
	finalEnv := make([]string, 0, len(envMap))
	for k, v := range envMap {
		finalEnv = append(finalEnv, k+"="+v)
	}
	cmd.Env = finalEnv

	// Tamanho inicial do terminal
	winSize := &pty.Winsize{
		Cols: config.Cols,
		Rows: config.Rows,
	}

	// Iniciar PTY
	ptmx, err := pty.StartWithSize(cmd, winSize)
	if err != nil {
		return "", fmt.Errorf("failed to start PTY: %w", err)
	}

	sessionID := uuid.New().String()[:8]

	session := &PTYSession{
		ID:          sessionID,
		Config:      config,
		Cols:        config.Cols,
		Rows:        config.Rows,
		IsAlive:     true,
		CreatedAt:   time.Now(),
		pty:         ptmx,
		cmd:         cmd.Process,
		output:      make([]func(data []byte), 0),
		done:        make(chan struct{}),
		permissions: make(map[string]TerminalPermission),
	}

	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()

	// Iniciar goroutine de leitura do PTY
	go m.readLoop(session)

	log.Printf("[PTY] Session %s created (shell: %s, cwd: %s, docker: %t, size: %dx%d)",
		sessionID, config.Shell, config.Cwd, config.UseDocker, config.Cols, config.Rows)

	return sessionID, nil
}

// readLoop lê continuamente o output do PTY e notifica os handlers
func (m *PTYManager) readLoop(session *PTYSession) {
	buf := make([]byte, 32*1024) // 32KB buffer

	defer func() {
		session.mu.Lock()
		session.IsAlive = false
		session.mu.Unlock()
		close(session.done)
		log.Printf("[PTY] Session %s read loop ended", session.ID)
	}()

	for {
		n, err := session.pty.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])

			// Notificar todos os handlers registrados
			session.mu.Lock()
			handlers := make([]func(data []byte), len(session.output))
			copy(handlers, session.output)
			session.mu.Unlock()

			for _, handler := range handlers {
				handler(data)
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("[PTY] Session %s read error: %v", session.ID, err)
			}
			return
		}
	}
}

// Destroy encerra e remove uma sessão PTY
func (m *PTYManager) Destroy(sessionID string) error {
	m.mu.Lock()
	session, exists := m.sessions[sessionID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("session %s not found", sessionID)
	}
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	// Fechar o PTY (isso vai encerrar o processo filho)
	if session.pty != nil {
		session.pty.Close()
	}

	// Matar o processo se ainda estiver rodando
	if session.cmd != nil {
		session.cmd.Kill()
		session.cmd.Wait()
	}

	log.Printf("[PTY] Session %s destroyed", sessionID)
	return nil
}

// Resize altera o tamanho do terminal
func (m *PTYManager) Resize(sessionID string, cols, rows uint16) error {
	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if session.pty == nil {
		return fmt.Errorf("session %s PTY is nil", sessionID)
	}

	err := pty.Setsize(session.pty, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
	if err != nil {
		return fmt.Errorf("failed to resize PTY: %w", err)
	}

	session.Cols = cols
	session.Rows = rows

	return nil
}

// Write envia dados para o stdin do PTY
func (m *PTYManager) Write(sessionID string, data []byte) error {
	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if session.pty == nil || !session.IsAlive {
		return fmt.Errorf("session %s is not alive", sessionID)
	}

	_, err := session.pty.Write(data)
	return err
}

// OnOutput registra um handler para o output do PTY
func (m *PTYManager) OnOutput(sessionID string, handler func(data []byte)) {
	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		log.Printf("[PTY] Warning: OnOutput for non-existent session %s", sessionID)
		return
	}

	session.mu.Lock()
	session.output = append(session.output, handler)
	session.mu.Unlock()
}

// GetSessions retorna informações de todas as sessões ativas
func (m *PTYManager) GetSessions() []SessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]SessionInfo, 0, len(m.sessions))
	for _, s := range m.sessions {
		s.mu.Lock()
		infos = append(infos, SessionInfo{
			ID:        s.ID,
			Shell:     s.Config.Shell,
			Cwd:       s.Config.Cwd,
			Cols:      s.Cols,
			Rows:      s.Rows,
			IsAlive:   s.IsAlive,
			CreatedAt: s.CreatedAt,
		})
		s.mu.Unlock()
	}
	return infos
}

// IsAlive verifica se uma sessão está ativa
func (m *PTYManager) IsAlive(sessionID string) bool {
	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	return session.IsAlive
}

// GetProcessPID retorna o PID do processo shell de uma sessão.
func (m *PTYManager) GetProcessPID(sessionID string) (int, error) {
	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return 0, fmt.Errorf("session %s not found", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if session.cmd == nil {
		return 0, fmt.Errorf("session %s has no process", sessionID)
	}
	return session.cmd.Pid, nil
}

// DestroyAll encerra todas as sessões
func (m *PTYManager) DestroyAll() {
	m.mu.Lock()
	sessionIDs := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		sessionIDs = append(sessionIDs, id)
	}
	m.mu.Unlock()

	for _, id := range sessionIDs {
		m.Destroy(id)
	}
	log.Println("[PTY] All sessions destroyed")
}

// SetPermission define a permissão de um usuário em uma sessão
func (m *PTYManager) SetPermission(sessionID, userID string, perm TerminalPermission) error {
	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if session.permissions == nil {
		session.permissions = make(map[string]TerminalPermission)
	}
	session.permissions[userID] = perm
	log.Printf("[PTY] Permission set for user %s on session %s: %s", userID, sessionID, perm)
	return nil
}

// GetPermission retorna a permissão de um usuário em uma sessão
func (m *PTYManager) GetPermission(sessionID, userID string) TerminalPermission {
	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return PermissionNone
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Host (userID vazio) sempre tem read_write
	if userID == "" {
		return PermissionReadWrite
	}

	if perm, ok := session.permissions[userID]; ok {
		return perm
	}
	return PermissionNone
}

// WriteWithPermission envia dados para o PTY apenas se o usuário tiver permissão
func (m *PTYManager) WriteWithPermission(sessionID, userID string, data []byte) error {
	perm := m.GetPermission(sessionID, userID)

	switch perm {
	case PermissionNone:
		return fmt.Errorf("user %s has no permission on session %s", userID, sessionID)
	case PermissionReadOnly:
		return fmt.Errorf("user %s has read-only permission on session %s", userID, sessionID)
	case PermissionReadWrite:
		return m.Write(sessionID, data)
	default:
		return fmt.Errorf("unknown permission %q for user %s", perm, userID)
	}
}
