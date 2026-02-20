package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"orch/internal/ai"
	"orch/internal/auth"
	"orch/internal/config"
	"orch/internal/database"
	"orch/internal/docker"
	fw "orch/internal/filewatcher"
	ga "orch/internal/gitactivity"
	gh "orch/internal/github"
	"orch/internal/security"
	"orch/internal/session"
	"orch/internal/terminal"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	defaultSessionGatewayListenAddr   = "127.0.0.1:9888"
	defaultSessionGatewayBaseURL      = "http://127.0.0.1:9888"
	defaultSessionSignalingListenAddr = "127.0.0.1:9876"
	defaultSessionSignalingBaseURL    = "ws://127.0.0.1:9876/ws/signal"
)

// App struct — ponto central do Wails, conecta todos os services
type App struct {
	ctx         context.Context
	db          *database.Service
	auth        *auth.Service
	docker      *docker.Service
	ptyMgr      *terminal.PTYManager
	bridge      *terminal.Bridge
	github      *gh.Service
	fileWatcher *fw.Service
	gitActivity *ga.Service
	poller      *gh.Poller
	session     *session.Service
	signaling   *session.SignalingService
	sessionHTTP *session.GatewayServer
	ai          *ai.Service

	logSanitizer      *security.LogSanitizer
	sessionContainers map[string]string // sessionID -> containerID
	mu                sync.RWMutex

	terminalStateMu sync.RWMutex
	terminalHistory map[string]string // sessionID -> ring buffer textual do terminal
	sessionAgents   map[string]uint   // sessionID -> agentSessionID

	// Stack build state (persiste enquanto o app estiver aberto)
	stackBuildMu      sync.RWMutex
	stackBuildRunning bool
	stackBuildLogs    []string
	stackBuildStart   int64  // unix timestamp (seconds)
	stackBuildResult  string // "", "success", "error"

	gitIndexMu            sync.Mutex
	lastIndexFingerprints map[string]string // repoPath -> fingerprint do estado staged
	anonymousGuestID      string
	sessionGatewayOwner   bool
	sessionGatewayAddr    string
	sessionGatewayURL     string
	signalingAddr         string
	signalingURL          string
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		sessionContainers:     make(map[string]string),
		terminalHistory:       make(map[string]string),
		sessionAgents:         make(map[string]uint),
		lastIndexFingerprints: make(map[string]string),
		anonymousGuestID:      fmt.Sprintf("anonymous-%d", time.Now().UnixNano()),
	}
}

// Startup is called when the app starts
// Inicializa banco, auth, PTY manager e emite evento de hydration para o frontend
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	log.Println("[ORCH] Starting up...")
	a.configureSessionNetworking()

	// 1. Garantir diretórios existem
	if err := config.EnsureDataDirs(); err != nil {
		log.Printf("[ORCH] Error creating data dirs: %v", err)
	}

	// 2. Inicializar banco de dados SQLite
	dbService, err := database.NewService()
	if err != nil {
		log.Printf("[ORCH] Error initializing database: %v", err)
	} else {
		a.db = dbService
		log.Println("[ORCH] Database initialized")
	}

	// 3. Inicializar serviço de auth
	authService := auth.NewService(a.db)
	a.auth = authService
	log.Println("[ORCH] Auth service initialized")

	// 4. Inicializar PTY Manager
	a.ptyMgr = terminal.NewPTYManager()
	a.bridge = terminal.NewBridge(ctx, a.ptyMgr)
	log.Println("[ORCH] PTY Manager initialized")

	// 4.1 Inicializar sanitizer de logs
	a.logSanitizer = security.NewLogSanitizer()

	// 4.2 Inicializar Docker service (sandbox opcional)
	a.docker = docker.NewService()
	if a.docker.IsDockerAvailable() {
		log.Println("[ORCH] Docker service initialized")
	} else {
		log.Println("[ORCH] Docker unavailable - fallback to Live Share enabled")
	}

	// 5. Inicializar GitHub Service
	a.github = gh.NewService(a.auth.GetGitHubToken)
	a.poller = gh.NewPoller(a.github, func(eventName string, data interface{}) {
		runtime.EventsEmit(a.ctx, eventName, data)
	})
	log.Println("[ORCH] GitHub Service + Poller initialized")

	// 6. Inicializar AI Service
	a.ai = ai.NewService(ai.ServiceDeps{
		GitHubCache: a.github,
		TokenBudget: config.TokenBudget,
	})
	a.bridge.RegisterOutputObserver(a.ai.ObserveTerminalOutput)
	a.bridge.RegisterOutputObserver(a.observeTerminalHistory)
	log.Println("[ORCH] AI Service initialized")

	// 6.1 Inicializar serviço de atividade Git (timeline em memória)
	a.gitActivity = ga.NewService(200, 900*time.Millisecond)
	log.Println("[ORCH] GitActivity service initialized")

	// 7. Inicializar File Watcher
	fwService, err := fw.NewService(func(eventName string, data interface{}) {
		a.emitGitRuntimeEvent(eventName, data)
	})
	if err != nil {
		log.Printf("[ORCH] Error initializing FileWatcher: %v", err)
	} else {
		a.fileWatcher = fwService
		log.Println("[ORCH] FileWatcher initialized")

		// Auto-watch workspace ativo se existir
		if a.db != nil {
			if ws, err := a.db.GetActiveWorkspace(); err == nil && ws.Path != "" {
				if watchErr := a.fileWatcher.Watch(ws.Path); watchErr != nil {
					log.Printf("[ORCH] Could not auto-watch workspace: %v", watchErr)
				} else {
					log.Printf("[ORCH] Auto-watching workspace: %s", ws.Path)
				}
			}
		}
	}

	// 8. Inicializar Session Service (P2P)
	a.session = session.NewService(func(eventName string, data interface{}) {
		runtime.EventsEmit(a.ctx, eventName, data)
	})
	a.restorePersistedSessionStates()

	a.signaling = session.NewSignalingService(a.session)
	a.signaling.SetConnectionObserver(func(sessionID, userID string, isHost bool, connected bool) {
		if isHost {
			return
		}
		if connected {
			a.auditSessionEvent(sessionID, userID, "guest_connected", "Guest established signaling connection")
			return
		}
		a.auditSessionEvent(sessionID, userID, "guest_disconnected", "Guest disconnected from signaling channel")
	})
	a.signaling.SetSessionObserver(func(sessionID string) {
		if a.sessionGatewayOwner {
			a.persistSessionState(sessionID)
		}
	})

	// Iniciar servidor de sinalização WebSocket configurável.
	if shouldStartSessionListener(a.signalingAddr) {
		if err := a.signaling.StartSignalingServerAddr(a.signalingAddr); err != nil {
			log.Printf("[ORCH] Error starting signaling server on %s: %v", a.signalingAddr, err)
		} else {
			log.Printf("[ORCH] Signaling server started on %s", a.signalingAddr)
		}
	} else {
		log.Printf("[ORCH] Signaling listener disabled (addr=%q)", a.signalingAddr)
	}

	// Iniciar gateway HTTP local de sessões (compartilha sessões entre instâncias locais).
	if shouldStartSessionListener(a.sessionGatewayAddr) {
		a.sessionHTTP = session.NewGatewayServer(a.session, a.sessionGatewayAddr)
		a.sessionHTTP.SetObservers(a.persistSessionState, a.deletePersistedSessionState)
		if err := a.sessionHTTP.Start(); err != nil {
			a.sessionGatewayOwner = false
			log.Printf("[ORCH] Session gateway unavailable on %s (using client mode): %v", a.sessionGatewayAddr, err)
		} else {
			a.sessionGatewayOwner = true
			log.Printf("[ORCH] Session gateway started on %s", a.sessionGatewayAddr)
		}
	} else {
		a.sessionGatewayOwner = false
		log.Printf("[ORCH] Session gateway listener disabled (addr=%q); using client mode", a.sessionGatewayAddr)
	}

	// 9. Configuração finalizada
	log.Println("[ORCH] Startup complete")

	// 10. Iniciar monitoramento de contexto de terminais (Auto-Git Sync)
	go a.startTerminalContextMonitor()
}

func (a *App) startTerminalContextMonitor() {
	// Poll curto para reduzir janela em que eventos Git podem ser perdidos após um `cd`.
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()

	lastCwds := make(map[string]string)
	lastGitRoots := make(map[string]string)
	lastBranches := make(map[string]string)

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			if a.ptyMgr == nil {
				continue
			}

			sessions := a.ptyMgr.GetSessions()
			activeSessions := make(map[string]struct{}, len(sessions))

			for _, sess := range sessions {
				activeSessions[sess.ID] = struct{}{}
				if !sess.IsAlive {
					continue
				}

				pid, err := a.ptyMgr.GetProcessPID(sess.ID)
				if err != nil {
					log.Printf("[ORCH][GitSync] session=%s unable to read PID: %v", sess.ID, err)
					continue
				}

				cwd := terminal.GetProcessCwd(pid)
				if cwd == "" {
					continue
				}
				prevCwd := lastCwds[sess.ID]
				prevGitRoot := lastGitRoots[sess.ID]
				prevBranch := lastBranches[sess.ID]
				lastCwds[sess.ID] = cwd

				if cwd != prevCwd {
					log.Printf("[ORCH][GitSync] session=%s cwd changed: %s -> %s", sess.ID, prevCwd, cwd)
				}

				// Verificar se está em um repo Git (incluindo subdiretórios do repo).
				gitRoot := findGitRepoRoot(cwd)
				if gitRoot == "" {
					// Só emitir contexto não-git quando houver mudança relevante.
					if cwd != prevCwd || prevGitRoot != "" {
						runtime.EventsEmit(a.ctx, "terminal:context_changed", map[string]string{
							"sessionID": sess.ID,
							"path":      cwd,
							"cwd":       cwd,
							"isGit":     "false",
						})
						log.Printf("[ORCH][GitSync] session=%s context_changed non-git path=%s", sess.ID, cwd)
					}
					delete(lastGitRoots, sess.ID)
					delete(lastBranches, sess.ID)
					continue
				}

				// É um repo Git! Iniciar watcher (se disponível) e buscar branch.
				branch := ""
				if a.fileWatcher != nil {
					if err := a.fileWatcher.Watch(gitRoot); err != nil {
						log.Printf("[ORCH][GitSync] session=%s watch failed repo=%s err=%v", sess.ID, gitRoot, err)
					}
					a.primeIndexActivityBaseline(gitRoot)

					branch, err = a.fileWatcher.GetCurrentBranch(gitRoot)
					if err != nil {
						log.Printf("[ORCH][GitSync] session=%s get branch failed repo=%s err=%v", sess.ID, gitRoot, err)
						branch = ""
					}
				}

				// Emitir contexto quando cwd/repo mudar, para frontend sincronizar.
				if cwd != prevCwd || gitRoot != prevGitRoot {
					runtime.EventsEmit(a.ctx, "terminal:context_changed", map[string]string{
						"sessionID": sess.ID,
						"path":      gitRoot,
						"cwd":       cwd,
						"branch":    branch,
						"isGit":     "true",
					})
					log.Printf("[ORCH][GitSync] session=%s context_changed gitRoot=%s cwd=%s branch=%s", sess.ID, gitRoot, cwd, branch)
				}

				// Fallback: emitir branch_changed quando detectarmos branch nova
				// via polling do contexto (caso fsnotify perca um evento rápido).
				if branch != "" && branch != prevBranch {
					a.emitGitRuntimeEvent("git:branch_changed", fw.FileEvent{
						Type:      "branch_changed",
						Path:      filepath.Join(gitRoot, ".git", "HEAD"),
						Timestamp: time.Now(),
						Details: map[string]string{
							"branch": branch,
							"source": "context_monitor",
						},
					})
					log.Printf("[ORCH][GitSync] session=%s fallback branch_changed: %s -> %s (repo=%s)", sess.ID, prevBranch, branch, gitRoot)
				}

				lastGitRoots[sess.ID] = gitRoot
				if branch != "" {
					lastBranches[sess.ID] = branch
				}
			}

			// Evitar crescimento infinito de estado para sessões já encerradas.
			for sessionID := range lastCwds {
				if _, stillActive := activeSessions[sessionID]; !stillActive {
					delete(lastCwds, sessionID)
					delete(lastGitRoots, sessionID)
					delete(lastBranches, sessionID)
				}
			}
		}
	}
}

func findGitRepoRoot(startPath string) string {
	current := filepath.Clean(startPath)
	for {
		gitPath := filepath.Join(current, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func (a *App) emitGitRuntimeEvent(eventName string, data interface{}) {
	runtime.EventsEmit(a.ctx, eventName, data)
	a.appendGitActivityFromRuntimeEvent(eventName, data)
}

func (a *App) appendGitActivityFromRuntimeEvent(eventName string, data interface{}) {
	if a.gitActivity == nil || !strings.HasPrefix(eventName, "git:") {
		return
	}

	fileEvent, ok := toFileWatcherEvent(data)
	if !ok {
		return
	}

	activityEvent, shouldAppend := a.buildGitActivityEvent(eventName, fileEvent)
	if !shouldAppend {
		return
	}

	if _, accepted := a.gitActivity.AppendEvent(activityEvent); !accepted {
		return
	}
}

func toFileWatcherEvent(data interface{}) (fw.FileEvent, bool) {
	switch v := data.(type) {
	case fw.FileEvent:
		return v, true
	case *fw.FileEvent:
		if v == nil {
			return fw.FileEvent{}, false
		}
		return *v, true
	default:
		return fw.FileEvent{}, false
	}
}

func (a *App) buildGitActivityEvent(eventName string, fileEvent fw.FileEvent) (ga.Event, bool) {
	eventType := mapGitRuntimeEventType(eventName)
	repoPath := extractRepoPathFromGitEventPath(fileEvent.Path)
	branch := strings.TrimSpace(fileEvent.Details["branch"])
	ref := strings.TrimSpace(fileEvent.Details["ref"])
	source := strings.TrimSpace(fileEvent.Details["source"])
	if source == "" {
		source = "filewatcher"
	}

	repoName := ""
	if repoPath != "" {
		repoName = filepath.Base(repoPath)
	}

	actor := a.resolveActivityActorName()
	if actor == "" {
		actor = "local-user"
	}
	message := formatGitActivityMessage(actor, eventType, repoName, branch, ref)

	extra := make(map[string]string)
	for key, value := range fileEvent.Details {
		extra[key] = value
	}
	if len(extra) == 0 {
		extra = nil
	}

	timestamp := fileEvent.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	dedupeKey := fmt.Sprintf("%s|%s|%s|%s|%s", eventType, repoPath, branch, ref, source)

	details := ga.EventDetails{
		Ref:   ref,
		Extra: extra,
	}

	if eventType == ga.EventTypeIndexUpdated && repoPath != "" {
		if files, err := ga.CollectStagedFiles(repoPath); err == nil {
			if !a.shouldRecordIndexActivity(repoPath, files) {
				return ga.Event{}, false
			}
			details.Files = files
			if len(files) > 0 {
				message = fmt.Sprintf("%s atualizou %d arquivo(s) staged no repositório %s", actor, len(files), repoName)
			} else {
				message = fmt.Sprintf("%s limpou arquivos staged no repositório %s", actor, repoName)
			}
		} else {
			log.Printf("[ORCH][GitActivity] staged summary failed repo=%s err=%v", repoPath, err)
			return ga.Event{}, false
		}
	} else if eventType == ga.EventTypeIndexUpdated {
		return ga.Event{}, false
	}

	return ga.Event{
		Type:      eventType,
		ActorName: actor,
		RepoPath:  repoPath,
		RepoName:  repoName,
		Branch:    branch,
		Message:   message,
		Timestamp: timestamp,
		Source:    source,
		DedupeKey: dedupeKey,
		Details:   details,
	}, true
}

func mapGitRuntimeEventType(eventName string) ga.EventType {
	switch eventName {
	case "git:branch_changed":
		return ga.EventTypeBranchChanged
	case "git:commit":
		return ga.EventTypeCommitCreated
	case "git:commit_preparing":
		return ga.EventTypeCommitPreparing
	case "git:index":
		return ga.EventTypeIndexUpdated
	case "git:merge":
		return ga.EventTypeMerge
	case "git:fetch":
		return ga.EventTypeFetch
	default:
		return ga.EventTypeUnknown
	}
}

func formatGitActivityMessage(actor string, eventType ga.EventType, repoName, branch, ref string) string {
	repo := strings.TrimSpace(repoName)
	if repo == "" {
		repo = "repository"
	}

	switch eventType {
	case ga.EventTypeBranchChanged:
		if branch != "" {
			return fmt.Sprintf("%s mudou para a branch %s no repositório %s", actor, branch, repo)
		}
		return fmt.Sprintf("%s alterou branch no repositório %s", actor, repo)
	case ga.EventTypeCommitCreated:
		if ref != "" {
			return fmt.Sprintf("%s atualizou a ref %s no repositório %s", actor, ref, repo)
		}
		return fmt.Sprintf("%s criou um commit no repositório %s", actor, repo)
	case ga.EventTypeCommitPreparing:
		return fmt.Sprintf("%s iniciou preparação de commit no repositório %s", actor, repo)
	case ga.EventTypeIndexUpdated:
		return fmt.Sprintf("%s atualizou arquivos staged no repositório %s", actor, repo)
	case ga.EventTypeMerge:
		return fmt.Sprintf("%s executou merge no repositório %s", actor, repo)
	case ga.EventTypeFetch:
		return fmt.Sprintf("%s executou fetch no repositório %s", actor, repo)
	default:
		return fmt.Sprintf("%s executou uma ação Git no repositório %s", actor, repo)
	}
}

func extractRepoPathFromGitEventPath(eventPath string) string {
	clean := filepath.Clean(eventPath)
	sep := string(os.PathSeparator)
	gitMarker := sep + ".git"

	if strings.HasSuffix(clean, gitMarker) {
		return filepath.Dir(clean)
	}
	if idx := strings.Index(clean, gitMarker+sep); idx >= 0 {
		return clean[:idx]
	}
	if idx := strings.Index(clean, gitMarker); idx >= 0 {
		return clean[:idx]
	}
	return ""
}

func (a *App) resolveActivityActorName() string {
	if a.auth != nil {
		authState := a.auth.GetAuthState()
		if authState != nil && authState.User != nil {
			if name := strings.TrimSpace(authState.User.Name); name != "" {
				return name
			}
			if email := strings.TrimSpace(authState.User.Email); email != "" {
				return email
			}
		}
	}

	if user := strings.TrimSpace(os.Getenv("USER")); user != "" {
		return user
	}
	return "local-user"
}

func (a *App) primeIndexActivityBaseline(repoPath string) {
	repo := filepath.Clean(strings.TrimSpace(repoPath))
	if repo == "" {
		return
	}

	a.gitIndexMu.Lock()
	_, known := a.lastIndexFingerprints[repo]
	a.gitIndexMu.Unlock()
	if known {
		return
	}

	files, err := ga.CollectStagedFiles(repo)
	if err != nil {
		return
	}

	fingerprint := buildIndexFingerprint(files)

	a.gitIndexMu.Lock()
	if _, exists := a.lastIndexFingerprints[repo]; !exists {
		a.lastIndexFingerprints[repo] = fingerprint
	}
	a.gitIndexMu.Unlock()
}

func (a *App) shouldRecordIndexActivity(repoPath string, files []ga.EventFile) bool {
	repo := filepath.Clean(strings.TrimSpace(repoPath))
	if repo == "" {
		return false
	}

	next := buildIndexFingerprint(files)

	a.gitIndexMu.Lock()
	defer a.gitIndexMu.Unlock()

	prev, exists := a.lastIndexFingerprints[repo]
	a.lastIndexFingerprints[repo] = next
	if !exists {
		// Primeiro snapshot vira baseline para evitar ruído.
		return false
	}
	return prev != next
}

func buildIndexFingerprint(files []ga.EventFile) string {
	if len(files) == 0 {
		return "empty"
	}

	rows := make([]string, 0, len(files))
	for _, f := range files {
		rows = append(rows, strings.Join([]string{
			strings.TrimSpace(f.Path),
			strings.TrimSpace(f.Status),
			strconv.Itoa(f.Added),
			strconv.Itoa(f.Removed),
		}, "|"))
	}
	sort.Strings(rows)
	return strings.Join(rows, "||")
}

// DomReady is called when the frontend DOM is ready
func (a *App) DomReady(ctx context.Context) {
	log.Println("[ORCH] DOM Ready")

	// Emitir evento de hydration para o frontend quando ele estiver pronto
	a.emitHydration()
}

// Shutdown is called when the app is shutting down
func (a *App) Shutdown(ctx context.Context) {
	// Parar Poller
	if a.poller != nil {
		a.poller.StopPolling()
	}
	log.Println("[ORCH] Shutting down...")

	// Notificar frontend que app está fechando (para salvar snapshots)
	runtime.EventsEmit(a.ctx, "app:before-shutdown")
	time.Sleep(500 * time.Millisecond) // Esperar frontend processar e salvar

	// Snapshot de terminais antes de destruí-los (fallback se frontend não salvou)
	a.snapshotTerminalsOnShutdown()

	// Fechar FileWatcher
	if a.fileWatcher != nil {
		if err := a.fileWatcher.Close(); err != nil {
			log.Printf("[ORCH] Error closing FileWatcher: %v", err)
		}
	}

	// Destruir todos os terminais
	if a.ptyMgr != nil {
		a.ptyMgr.DestroyAll()
	}

	// Cleanup de containers de sessão (modo Docker)
	if a.docker != nil {
		a.mu.Lock()
		pairs := make(map[string]string, len(a.sessionContainers))
		for sessionID, containerID := range a.sessionContainers {
			pairs[sessionID] = containerID
		}
		a.sessionContainers = make(map[string]string)
		a.mu.Unlock()

		for sessionID, containerID := range pairs {
			_ = a.docker.StopContainer(containerID)
			_ = a.docker.RemoveContainer(containerID)
			a.auditSessionEvent(sessionID, "system", "container_stopped", fmt.Sprintf("container=%s shutdown=true", containerID))
		}
	}

	// Encerrar gateway HTTP de sessões
	if a.sessionHTTP != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if err := a.sessionHTTP.Stop(shutdownCtx); err != nil {
			log.Printf("[ORCH] Error stopping session gateway: %v", err)
		}
		cancel()
	}

	// Fechar conexão com banco
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			log.Printf("[ORCH] Error closing database: %v", err)
		}
	}
}

// emitHydration envia o estado inicial para o frontend
// getHydrationPayload constrói o payload inicial
func (a *App) getHydrationPayload() HydrationPayload {
	payload := HydrationPayload{
		IsAuthenticated:     false,
		Theme:               "dark",
		Language:            "pt-BR",
		TerminalFontSize:    14,
		TerminalFontFamily:  defaultTerminalFontFamily,
		TerminalCursorStyle: defaultTerminalCursorStyle,
		Version:             config.AppVersion,
	}

	// Checar autenticação
	if a.auth != nil {
		isAuth, _ := a.auth.IsAuthenticated()
		payload.IsAuthenticated = isAuth

		if isAuth {
			if user, err := a.auth.GetCurrentUser(); err == nil {
				payload.User = user
			}
		}
	}

	// Carregar configurações do banco
	if a.db != nil {
		if cfg, err := a.db.GetConfig(); err == nil {
			payload.Theme = cfg.Theme
			if cfg.Language != "" {
				payload.Language = cfg.Language
			}
			payload.DefaultShell = cfg.DefaultShell
			payload.TerminalFontSize = normalizeTerminalFontSize(cfg.FontSize)
			payload.TerminalFontFamily = normalizeTerminalFontFamily(cfg.FontFamily)
			payload.TerminalCursorStyle = normalizeTerminalCursorStyle(cfg.CursorStyle)
			payload.OnboardingCompleted = cfg.OnboardingCompleted
			payload.ShortcutBindings = cfg.ShortcutBindings
		}

		if workspaces, err := a.db.ListWorkspaces(); err == nil {
			payload.Workspaces = workspaces
		}
	}

	return payload
}

// emitHydration envia o estado inicial para o frontend
func (a *App) emitHydration() {
	payload := a.getHydrationPayload()
	runtime.EventsEmit(a.ctx, "app:hydrated", payload)
	log.Println("[ORCH] Hydration emitted")
}

// === Tipos expostos ao Frontend via Wails bindings ===

// HydrationPayload é o payload enviado ao frontend no startup
type HydrationPayload struct {
	IsAuthenticated     bool                 `json:"isAuthenticated"`
	User                *auth.User           `json:"user,omitempty"`
	Theme               string               `json:"theme"`
	Language            string               `json:"language"`
	DefaultShell        string               `json:"defaultShell"`
	TerminalFontSize    int                  `json:"terminalFontSize"`
	TerminalFontFamily  string               `json:"terminalFontFamily"`
	TerminalCursorStyle string               `json:"terminalCursorStyle"`
	OnboardingCompleted bool                 `json:"onboardingCompleted"`
	ShortcutBindings    string               `json:"shortcutBindings,omitempty"`
	Version             string               `json:"version"`
	Workspaces          []database.Workspace `json:"workspaces,omitempty"`
}

func (a *App) resolvePreferredLocalShell() string {
	if a.db != nil {
		if cfg, err := a.db.GetConfig(); err == nil {
			if configured := strings.TrimSpace(cfg.DefaultShell); configured != "" {
				return configured
			}
		}
	}
	return terminal.DefaultShell()
}

// === Terminal Bindings (expostos ao Frontend) ===

func (a *App) createTerminalWithArgs(shell string, args []string, cwd string, useDocker bool, cols uint16, rows uint16) (string, error) {
	shell = strings.TrimSpace(shell)
	if useDocker {
		if shell == "" {
			shell = "/bin/sh"
		}
	} else if shell == "" {
		// Para terminais locais: Settings > auto-detect do shell da máquina.
		shell = a.resolvePreferredLocalShell()
	}

	if cwd == "" {
		home, _ := os.UserHomeDir()
		cwd = home
	}

	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	cfg := terminal.PTYConfig{
		Shell:     shell,
		Args:      append([]string(nil), args...),
		Cwd:       cwd,
		Cols:      cols,
		Rows:      rows,
		UseDocker: useDocker,
	}

	// Se for Docker, montar o diretório atual (cwd) no container
	if useDocker {
		cfg.DockerMount = cwd
	}

	// Se for Docker, verificar se existe imagem customizada
	if useDocker && a.docker != nil {
		customImage := "orch-custom-stack:latest"
		if a.docker.ImageExists(customImage) {
			cfg.DockerImage = customImage
			log.Printf("[ORCH] CreateTerminal using custom stack: %s", customImage)
		}
	}

	sessionID, err := a.bridge.CreateTerminal(cfg)
	if err != nil {
		return "", err
	}

	a.terminalStateMu.Lock()
	a.terminalHistory[sessionID] = ""
	a.terminalStateMu.Unlock()

	if a.ai != nil {
		shellForAI := strings.TrimSpace(shell)
		if shellForAI == "" {
			if useDocker {
				shellForAI = "/bin/sh"
			} else {
				shellForAI = a.resolvePreferredLocalShell()
			}
		}

		state := ai.SessionState{
			ProjectName: "ORCH",
			ShellType:   filepath.Base(shellForAI),
		}

		if a.db != nil {
			if ws, wsErr := a.db.GetActiveWorkspace(); wsErr == nil && ws != nil {
				if ws.Name != "" {
					state.ProjectName = ws.Name
				}
				if ws.Path != "" {
					state.ProjectPath = ws.Path
					if ws.Name == "" {
						state.ProjectName = filepath.Base(ws.Path)
					}
					if a.fileWatcher != nil {
						if branch, bErr := a.fileWatcher.GetCurrentBranch(ws.Path); bErr == nil {
							state.CurrentBranch = branch
						}
					}
				}
			}
		}

		a.ai.SetSessionState(sessionID, state)
	}

	// Aplica permissões de sessão colaborativa já existentes em novos terminais.
	a.syncAllGuestPermissionsToPTY(sessionID)

	return sessionID, nil
}

// CreateTerminal cria um novo terminal PTY e retorna o session ID
func (a *App) CreateTerminal(shell string, cwd string, useDocker bool, cols uint16, rows uint16) (string, error) {
	return a.createTerminalWithArgs(shell, nil, cwd, useDocker, cols, rows)
}

func (a *App) resolveEffectiveRuntimeShell(shell string, useDocker bool) string {
	effectiveShell := strings.TrimSpace(shell)
	if effectiveShell != "" {
		return effectiveShell
	}
	if useDocker {
		return "/bin/sh"
	}
	return a.resolvePreferredLocalShell()
}

func resumeCommandForCLI(cliType string) (string, bool) {
	key := terminal.CLIType(strings.ToLower(strings.TrimSpace(cliType)))
	command, ok := terminal.CLIResumeCommands[key]
	if !ok || strings.TrimSpace(command) == "" {
		return "", false
	}
	return command, true
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// CreateTerminalForAgent cria (ou reutiliza) o PTY vinculado a um AgentSession.
func (a *App) CreateTerminalForAgent(agentID uint, shell string, cwd string, useDocker bool, cols uint16, rows uint16) (string, error) {
	if a.db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	agent, err := a.db.GetAgent(agentID)
	if err != nil {
		return "", err
	}

	if agent.SessionID != "" && a.bridge != nil && a.bridge.IsTerminalAlive(agent.SessionID) {
		a.bindTerminalToAgent(agent.SessionID, agent.ID)
		return agent.SessionID, nil
	}

	resolvedShell := strings.TrimSpace(shell)
	if !useDocker && resolvedShell == "" {
		resolvedShell = a.resolvePreferredLocalShell()
	}

	resolvedCwd := cwd
	if strings.TrimSpace(resolvedCwd) == "" {
		resolvedCwd = agent.Cwd
	}

	sessionID, err := a.createTerminalWithArgs(resolvedShell, nil, resolvedCwd, useDocker, cols, rows)
	if err != nil {
		return "", err
	}

	effectiveShell := a.resolveEffectiveRuntimeShell(resolvedShell, useDocker)

	effectiveCwd := strings.TrimSpace(resolvedCwd)
	if effectiveCwd == "" {
		home, _ := os.UserHomeDir()
		effectiveCwd = home
	}

	if err := a.db.UpdateAgentRuntime(agent.ID, sessionID, effectiveShell, effectiveCwd, useDocker, "running"); err != nil {
		a.killTerminalSession(sessionID)
		return "", err
	}

	a.bindTerminalToAgent(sessionID, agent.ID)
	return sessionID, nil
}

// CreateTerminalForAgentResume cria sessão já com comando de resume da CLI, sem "digitar" comando no terminal.
func (a *App) CreateTerminalForAgentResume(agentID uint, cliType string, shell string, cwd string, useDocker bool, cols uint16, rows uint16) (string, error) {
	if a.db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	agent, err := a.db.GetAgent(agentID)
	if err != nil {
		return "", err
	}

	if agent.SessionID != "" && a.bridge != nil && a.bridge.IsTerminalAlive(agent.SessionID) {
		a.bindTerminalToAgent(agent.SessionID, agent.ID)
		return agent.SessionID, nil
	}

	resumeCmd, ok := resumeCommandForCLI(cliType)
	if !ok {
		return "", fmt.Errorf("unsupported cliType for resume: %s", strings.TrimSpace(cliType))
	}

	resolvedShell := strings.TrimSpace(shell)
	if useDocker {
		if resolvedShell == "" {
			resolvedShell = "/bin/sh"
		}
	} else if resolvedShell == "" {
		if agentShell := strings.TrimSpace(agent.Shell); agentShell != "" {
			resolvedShell = agentShell
		} else {
			resolvedShell = a.resolvePreferredLocalShell()
		}
	}

	resolvedCwd := strings.TrimSpace(cwd)
	if resolvedCwd == "" {
		resolvedCwd = strings.TrimSpace(agent.Cwd)
	}
	if resolvedCwd == "" {
		home, _ := os.UserHomeDir()
		resolvedCwd = home
	}

	bootstrap := fmt.Sprintf("%s; exec %s -l", resumeCmd, shellSingleQuote(resolvedShell))
	sessionID, err := a.createTerminalWithArgs(resolvedShell, []string{"-c", bootstrap}, resolvedCwd, useDocker, cols, rows)
	if err != nil {
		return "", err
	}

	effectiveShell := strings.TrimSpace(agent.Shell)
	if effectiveShell == "" {
		effectiveShell = a.resolveEffectiveRuntimeShell(shell, useDocker)
	}

	if err := a.db.UpdateAgentRuntime(agent.ID, sessionID, effectiveShell, resolvedCwd, useDocker, "running"); err != nil {
		a.killTerminalSession(sessionID)
		return "", err
	}

	a.bindTerminalToAgent(sessionID, agent.ID)
	return sessionID, nil
}

// WriteTerminal envia dados para o terminal
func (a *App) WriteTerminal(sessionID string, data string) error {
	decoded := decodeTerminalInput(data)

	if a.ai != nil {
		a.ai.ObserveTerminalInput(sessionID, decoded)
	}

	if a.ptyMgr == nil {
		return fmt.Errorf("pty manager not initialized")
	}
	return a.ptyMgr.Write(sessionID, decoded)
}

func (a *App) resolveSessionHostUserID() string {
	hostUserID := "local"
	if a.auth != nil {
		if user, err := a.auth.GetCurrentUser(); err == nil && user != nil && strings.TrimSpace(user.ID) != "" {
			hostUserID = user.ID
		}
	}
	return hostUserID
}

func guestTerminalPermissionFromSession(sessionGuest session.SessionGuest) terminal.TerminalPermission {
	if sessionGuest.Status != session.GuestApproved && sessionGuest.Status != session.GuestConnected {
		return terminal.PermissionNone
	}
	if sessionGuest.Permission == session.PermReadWrite {
		return terminal.PermissionReadWrite
	}
	return terminal.PermissionReadOnly
}

func (a *App) resolveActiveCollabSession(hostUserID string) (*session.Session, error) {
	hostUserID = strings.TrimSpace(hostUserID)
	if hostUserID == "" {
		return nil, fmt.Errorf("host userID is required")
	}

	if a.sessionGatewayOwner && a.session != nil {
		sess, err := a.session.GetActiveSession(hostUserID)
		if err == nil && sess != nil {
			return sess, nil
		}
		if err != nil && !strings.Contains(err.Error(), "no active session") {
			return nil, err
		}
		return nil, fmt.Errorf("no active collaboration session for user %s", hostUserID)
	}

	sess, err := a.gatewayGetActiveSession(hostUserID)
	if err != nil {
		return nil, fmt.Errorf("no active collaboration session for user %s", hostUserID)
	}
	if sess == nil {
		return nil, fmt.Errorf("no active collaboration session for user %s", hostUserID)
	}
	return sess, nil
}

func (a *App) resolveTerminalWorkspaceID(terminalSessionID string) (uint, error) {
	if a.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	terminalSessionID = strings.TrimSpace(terminalSessionID)
	if terminalSessionID == "" {
		return 0, fmt.Errorf("terminal sessionID is required")
	}

	a.terminalStateMu.RLock()
	agentID, ok := a.sessionAgents[terminalSessionID]
	a.terminalStateMu.RUnlock()
	if !ok || agentID == 0 {
		return 0, fmt.Errorf("terminal %s is not bound to an agent", terminalSessionID)
	}

	agent, err := a.db.GetAgent(agentID)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve terminal workspace: %w", err)
	}
	if agent == nil || agent.WorkspaceID == 0 {
		return 0, fmt.Errorf("terminal %s has invalid workspace binding", terminalSessionID)
	}

	return agent.WorkspaceID, nil
}

func (a *App) resolveGuestTerminalPermission(terminalSessionID, guestUserID string) (terminal.TerminalPermission, error) {
	guestUserID = strings.TrimSpace(guestUserID)
	if guestUserID == "" {
		return terminal.PermissionNone, fmt.Errorf("guest userID is required")
	}

	hostUserID := a.resolveSessionHostUserID()
	activeSession, err := a.resolveActiveCollabSession(hostUserID)
	if err != nil {
		return terminal.PermissionNone, err
	}

	var guestSession session.SessionGuest
	foundGuest := false
	for _, guest := range activeSession.Guests {
		if guest.UserID == guestUserID {
			guestSession = guest
			foundGuest = true
			break
		}
	}
	if !foundGuest {
		return terminal.PermissionNone, fmt.Errorf("guest %s is not part of active session", guestUserID)
	}

	permission := guestTerminalPermissionFromSession(guestSession)
	if permission == terminal.PermissionNone {
		return terminal.PermissionNone, fmt.Errorf("guest %s has no write permission", guestUserID)
	}

	scopedWorkspaceID := activeSession.Config.WorkspaceID
	if scopedWorkspaceID > 0 {
		terminalWorkspaceID, err := a.resolveTerminalWorkspaceID(terminalSessionID)
		if err != nil {
			return terminal.PermissionNone, err
		}
		if terminalWorkspaceID != scopedWorkspaceID {
			return terminal.PermissionNone, fmt.Errorf(
				"terminal %s is outside scoped workspace %d",
				terminalSessionID,
				scopedWorkspaceID,
			)
		}
	}

	return permission, nil
}

func (a *App) applyPermissionToAllPTY(guestUserID string, perm terminal.TerminalPermission) {
	if a.ptyMgr == nil {
		return
	}
	guestUserID = strings.TrimSpace(guestUserID)
	if guestUserID == "" {
		return
	}
	for _, info := range a.ptyMgr.GetSessions() {
		if err := a.ptyMgr.SetPermission(info.ID, guestUserID, perm); err != nil {
			log.Printf("[ORCH][SESSION] failed to set PTY permission session=%s guest=%s: %v", info.ID, guestUserID, err)
		}
	}
}

func (a *App) syncGuestPermissionAcrossPTYs(sessionID, guestUserID string) {
	if a.ptyMgr == nil {
		return
	}
	perm := terminal.PermissionNone
	sess, err := a.SessionGetSession(sessionID)
	if err == nil && sess != nil {
		for _, guest := range sess.Guests {
			if guest.UserID == guestUserID {
				perm = guestTerminalPermissionFromSession(guest)
				break
			}
		}
	}
	a.applyPermissionToAllPTY(guestUserID, perm)
}

func (a *App) syncAllGuestPermissionsToPTY(sessionID string) {
	if a.ptyMgr == nil || strings.TrimSpace(sessionID) == "" {
		return
	}

	hostUserID := a.resolveSessionHostUserID()
	var sess *session.Session
	if a.sessionGatewayOwner && a.session != nil {
		if localSession, err := a.session.GetActiveSession(hostUserID); err == nil {
			sess = localSession
		}
	} else if remoteSession, err := a.gatewayGetActiveSession(hostUserID); err == nil {
		sess = remoteSession
	}

	if sess == nil {
		return
	}

	for _, guest := range sess.Guests {
		if err := a.ptyMgr.SetPermission(sessionID, guest.UserID, guestTerminalPermissionFromSession(guest)); err != nil {
			log.Printf("[ORCH][SESSION] failed to set initial PTY permission session=%s guest=%s: %v", sessionID, guest.UserID, err)
		}
	}
}

// WriteTerminalAsGuest envia input ao PTY com validação de permissão (read_write).
func (a *App) WriteTerminalAsGuest(sessionID, userID, data string) error {
	if a.ptyMgr == nil {
		return fmt.Errorf("pty manager not initialized")
	}

	permission, err := a.resolveGuestTerminalPermission(sessionID, userID)
	if err != nil {
		return err
	}
	if err := a.ptyMgr.SetPermission(sessionID, userID, permission); err != nil {
		return err
	}

	decoded := decodeTerminalInput(data)
	if err := a.ptyMgr.WriteWithPermission(sessionID, userID, decoded); err != nil {
		return err
	}

	// Registra somente entradas com quebra de linha para reduzir ruído.
	raw := string(decoded)
	if strings.ContainsAny(raw, "\r\n") {
		command := strings.TrimSpace(raw)
		if command != "" {
			a.auditSessionEvent(sessionID, userID, "command_executed", command)
		}
	}

	return nil
}

// ResizeTerminal redimensiona o terminal
func (a *App) ResizeTerminal(sessionID string, cols uint16, rows uint16) error {
	return a.bridge.ResizeTerminal(sessionID, cols, rows)
}

// DestroyTerminal encerra um terminal
func (a *App) DestroyTerminal(sessionID string) error {
	if sessionID == "" {
		return nil
	}

	if a.ai != nil {
		a.ai.RemoveSession(sessionID)
	}

	if err := a.bridge.DestroyTerminal(sessionID); err != nil {
		return err
	}

	if agentID, ok := a.unbindTerminalFromAgent(sessionID); ok && a.db != nil {
		if err := a.db.ClearAgentRuntime(agentID); err != nil {
			log.Printf("[ORCH] unable to clear agent runtime (agent=%d): %v", agentID, err)
		}
	}

	return nil
}

// AIListProviders retorna provedores de IA disponíveis.
func (a *App) AIListProviders() []ai.AIProvider {
	if a.ai == nil {
		return []ai.AIProvider{}
	}
	return a.ai.ListProviders()
}

// AISetProvider configura o provedor ativo de IA.
func (a *App) AISetProvider(provider ai.AIProvider) error {
	if a.ai == nil {
		return fmt.Errorf("ai service not initialized")
	}
	return a.ai.SetProvider(provider)
}

// AICancel cancela uma geração de IA em andamento na sessão.
func (a *App) AICancel(sessionID string) error {
	if a.ai == nil {
		return nil
	}
	return a.ai.Cancel(sessionID)
}

// AISetSessionState atualiza o contexto de IA para uma sessão.
func (a *App) AISetSessionState(sessionID string, state ai.SessionState) {
	if a.ai == nil {
		return
	}
	a.ai.SetSessionState(sessionID, state)
}

func decodeTerminalInput(data string) []byte {
	// Input vindo do frontend local é enviado como texto bruto para reduzir
	// overhead por tecla. Mantemos suporte opcional a payload base64 prefixado.
	const b64Prefix = "b64:"
	if strings.HasPrefix(data, b64Prefix) {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, b64Prefix))
		if err == nil {
			return decoded
		}
	}
	return []byte(data)
}

func (a *App) sanitizeForLogs(message string) string {
	if a.logSanitizer == nil {
		return message
	}
	return a.logSanitizer.Sanitize(message)
}

func (a *App) auditSessionEvent(sessionID, userID, action, details string) {
	if a.db == nil || sessionID == "" || action == "" {
		return
	}

	sanitized := a.sanitizeForLogs(details)
	event := &database.AuditLog{
		SessionID: sessionID,
		UserID:    userID,
		Action:    action,
		Details:   sanitized,
		CreatedAt: time.Now(),
	}
	if err := a.db.SaveAuditEvent(event); err != nil {
		log.Printf("[AUDIT] failed to persist event: %s", a.sanitizeForLogs(err.Error()))
	}
}

func (a *App) setSessionContainer(sessionID, containerID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sessionContainers[sessionID] = containerID
}

func (a *App) getSessionContainer(sessionID string) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	containerID, ok := a.sessionContainers[sessionID]
	return containerID, ok
}

func (a *App) popSessionContainer(sessionID string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	containerID, ok := a.sessionContainers[sessionID]
	if ok {
		delete(a.sessionContainers, sessionID)
	}
	return containerID, ok
}

func (a *App) observeTerminalHistory(sessionID string, data []byte) {
	if sessionID == "" || len(data) == 0 {
		return
	}

	a.terminalStateMu.Lock()
	defer a.terminalStateMu.Unlock()

	current := a.terminalHistory[sessionID] + string(data)
	if len(current) > config.TerminalRingBufferSize {
		current = current[len(current)-config.TerminalRingBufferSize:]
	}
	a.terminalHistory[sessionID] = current
}

func (a *App) bindTerminalToAgent(sessionID string, agentID uint) {
	if sessionID == "" || agentID == 0 {
		return
	}
	a.terminalStateMu.Lock()
	defer a.terminalStateMu.Unlock()
	a.sessionAgents[sessionID] = agentID
}

func (a *App) unbindTerminalFromAgent(sessionID string) (uint, bool) {
	if sessionID == "" {
		return 0, false
	}
	a.terminalStateMu.Lock()
	defer a.terminalStateMu.Unlock()
	agentID, ok := a.sessionAgents[sessionID]
	if ok {
		delete(a.sessionAgents, sessionID)
	}
	delete(a.terminalHistory, sessionID)
	return agentID, ok
}

func (a *App) getTerminalHistory(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	a.terminalStateMu.RLock()
	defer a.terminalStateMu.RUnlock()
	return a.terminalHistory[sessionID]
}

func (a *App) killTerminalSession(sessionID string) {
	if sessionID == "" || a.bridge == nil {
		return
	}

	if a.ai != nil {
		a.ai.RemoveSession(sessionID)
	}

	if err := a.bridge.DestroyTerminal(sessionID); err != nil {
		log.Printf("[ORCH] unable to destroy terminal session %s: %v", sessionID, err)
	}

	if agentID, ok := a.unbindTerminalFromAgent(sessionID); ok && a.db != nil {
		if err := a.db.ClearAgentRuntime(agentID); err != nil {
			log.Printf("[ORCH] unable to clear agent runtime (agent=%d): %v", agentID, err)
		}
	}
}

// GetTerminals retorna os terminais ativos
func (a *App) GetTerminals() []terminal.SessionInfo {
	return a.bridge.GetTerminals()
}

// IsTerminalAlive verifica se um terminal está ativo
func (a *App) IsTerminalAlive(sessionID string) bool {
	return a.bridge.IsTerminalAlive(sessionID)
}

// === Métodos expostos ao Frontend (Wails Bindings) ===

// GetAppInfo retorna informações do app
func (a *App) GetAppInfo() map[string]string {
	return map[string]string{
		"name":    config.AppName,
		"version": config.AppVersion,
	}
}

// SaveAgentLayout salva o layout de um agente no banco
func (a *App) SaveAgentLayout(agentID uint, layoutJSON string) error {
	if a.db == nil {
		return nil
	}
	return a.db.UpdateAgentLayout(agentID, layoutJSON)
}

// CreateWorkspace cria um novo workspace persistente.
func (a *App) CreateWorkspace(name string) (*database.Workspace, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = fmt.Sprintf("Workspace %d", time.Now().Unix()%10000)
	}

	ws := &database.Workspace{
		UserID: "local",
		Name:   trimmed,
		Path:   "",
	}

	if err := a.db.CreateWorkspace(ws); err != nil {
		return nil, err
	}

	return ws, nil
}

// SyncGuestWorkspace creates or syncs a shared workspace in the guest's local database.
func (a *App) SyncGuestWorkspace(name string) (*database.Workspace, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = "Shared Workspace"
	}

	// Attempt to find if an existing workspace has this exact name.
	workspaces, err := a.db.ListWorkspaces()
	if err != nil {
		return nil, err
	}

	for _, ws := range workspaces {
		if strings.EqualFold(strings.TrimSpace(ws.Name), trimmed) {
			return &ws, nil
		}
	}

	// Create if it doesn't exist
	ws := &database.Workspace{
		UserID: "local",
		Name:   trimmed,
		Path:   "",
	}

	if err := a.db.CreateWorkspace(ws); err != nil {
		return nil, err
	}

	return ws, nil
}

// RenameWorkspace atualiza o nome de um workspace.
func (a *App) RenameWorkspace(id uint, name string) (*database.Workspace, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if err := a.db.RenameWorkspace(id, name); err != nil {
		return nil, err
	}
	return a.db.GetWorkspace(id)
}

// SetWorkspaceColor atualiza a cor de um workspace.
func (a *App) SetWorkspaceColor(id uint, color string) (*database.Workspace, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if err := a.db.SetWorkspaceColor(id, color); err != nil {
		return nil, err
	}
	return a.db.GetWorkspace(id)
}

// SetActiveWorkspace marca o workspace como ativo.
func (a *App) SetActiveWorkspace(id uint) error {
	if a.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return a.db.SetActiveWorkspace(id)
}

// GetWorkspacesWithAgents retorna workspaces com seus agentes filhos.
func (a *App) GetWorkspacesWithAgents() ([]database.Workspace, error) {
	if a.db == nil {
		return []database.Workspace{}, nil
	}
	return a.db.GetWorkspacesWithAgents()
}

// DeleteWorkspace remove um workspace após matar todos os processos vinculados.
func (a *App) DeleteWorkspace(id uint) error {
	if a.db == nil {
		return fmt.Errorf("database not initialized")
	}

	agents, err := a.db.ListAgents(id)
	if err != nil {
		return err
	}

	for _, agent := range agents {
		if agent.SessionID != "" {
			a.killTerminalSession(agent.SessionID)
		}
	}

	err = a.db.DeleteWorkspace(id)
	if errors.Is(err, database.ErrLastWorkspace) {
		return fmt.Errorf("cannot delete last workspace")
	}
	return err
}

// CreateAgentSession cria um novo agente/sessão no workspace informado.
func (a *App) CreateAgentSession(workspaceID uint, name string, agentType string) (*database.AgentSession, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Regra de negócio: primeiro terminal sempre nasce no workspace "Default".
	if totalAgents, countErr := a.db.CountAgents(); countErr == nil && totalAgents == 0 {
		defaultWorkspaceID := uint(0)
		if workspaces, listErr := a.db.ListWorkspaces(); listErr == nil {
			for _, ws := range workspaces {
				if strings.EqualFold(strings.TrimSpace(ws.Name), "default") {
					defaultWorkspaceID = ws.ID
					break
				}
			}
		}

		if defaultWorkspaceID == 0 {
			defaultWorkspace := &database.Workspace{
				UserID: "local",
				Name:   "Default",
				Path:   "",
			}
			if createErr := a.db.CreateWorkspace(defaultWorkspace); createErr == nil {
				defaultWorkspaceID = defaultWorkspace.ID
			}
		}

		if defaultWorkspaceID != 0 {
			workspaceID = defaultWorkspaceID
			_ = a.db.SetActiveWorkspace(defaultWorkspaceID)
		}
	}

	if workspaceID == 0 {
		ws, err := a.db.GetActiveWorkspace()
		if err != nil {
			return nil, err
		}
		workspaceID = ws.ID
	}

	if _, err := a.db.GetWorkspace(workspaceID); err != nil {
		return nil, err
	}

	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		trimmedName = fmt.Sprintf("Terminal %d", time.Now().Unix()%10000)
	}

	trimmedType := strings.TrimSpace(agentType)
	if trimmedType == "" {
		trimmedType = "terminal"
	}

	existing, err := a.db.ListAgents(workspaceID)
	if err != nil {
		return nil, err
	}

	agent := &database.AgentSession{
		WorkspaceID: workspaceID,
		Name:        trimmedName,
		Type:        trimmedType,
		Shell:       a.resolvePreferredLocalShell(),
		Status:      "idle",
		SortOrder:   len(existing),
	}

	if err := a.db.CreateAgent(agent); err != nil {
		return nil, err
	}

	if agent == nil || agent.ID == 0 {
		return nil, fmt.Errorf("invalid agent session persisted: empty id")
	}

	persisted, err := a.db.GetAgent(agent.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load created agent session: %w", err)
	}
	if persisted == nil || persisted.ID == 0 || persisted.WorkspaceID == 0 {
		return nil, fmt.Errorf("invalid created agent session payload")
	}

	return persisted, nil
}

// DeleteAgentSession remove o agente e encerra seu processo associado.
func (a *App) DeleteAgentSession(agentID uint) error {
	if a.db == nil {
		return fmt.Errorf("database not initialized")
	}

	agent, err := a.db.GetAgent(agentID)
	if err != nil {
		return err
	}

	if agent.SessionID != "" {
		a.killTerminalSession(agent.SessionID)
	}

	return a.db.DeleteAgent(agentID)
}

// MoveAgentSessionToWorkspace move um agente/sessão para outro workspace.
func (a *App) MoveAgentSessionToWorkspace(agentID uint, workspaceID uint) (*database.AgentSession, error) {
	log.Printf("[ORCH][MOVE] request received agentID=%d targetWorkspaceID=%d", agentID, workspaceID)

	if a.db == nil {
		log.Printf("[ORCH][MOVE] aborted: database not initialized")
		return nil, fmt.Errorf("database not initialized")
	}

	moved, err := a.db.MoveAgentToWorkspace(agentID, workspaceID)
	if err != nil {
		log.Printf("[ORCH][MOVE] failed agentID=%d targetWorkspaceID=%d err=%v", agentID, workspaceID, err)
		return nil, err
	}

	if moved == nil {
		log.Printf("[ORCH][MOVE] failed agentID=%d targetWorkspaceID=%d err=empty payload", agentID, workspaceID)
		return nil, fmt.Errorf("move returned empty payload")
	}

	log.Printf(
		"[ORCH][MOVE] success agentID=%d targetWorkspaceID=%d finalWorkspaceID=%d sortOrder=%d sessionID=%q",
		agentID,
		workspaceID,
		moved.WorkspaceID,
		moved.SortOrder,
		moved.SessionID,
	)

	return moved, nil
}

// GetWorkspaceHistoryBuffer retorna o histórico textual dos terminais de um workspace.
// map key = agentID serializado como string.
func (a *App) GetWorkspaceHistoryBuffer(workspaceID uint) (map[string]string, error) {
	if a.db == nil {
		return map[string]string{}, nil
	}

	if workspaceID == 0 {
		active, err := a.db.GetActiveWorkspace()
		if err != nil {
			return map[string]string{}, err
		}
		workspaceID = active.ID
	}

	agents, err := a.db.ListAgents(workspaceID)
	if err != nil {
		return map[string]string{}, err
	}

	history := make(map[string]string, len(agents))
	for _, agent := range agents {
		if agent.ID == 0 || agent.SessionID == "" {
			continue
		}

		buffer := a.getTerminalHistory(agent.SessionID)
		if buffer == "" {
			continue
		}
		history[strconv.FormatUint(uint64(agent.ID), 10)] = buffer
	}

	return history, nil
}

// CreateAgent cria um novo agente no workspace ativo (compatibilidade legada).
func (a *App) CreateAgent(name string, agentType string) (*database.AgentInstance, error) {
	agent, err := a.CreateAgentSession(0, name, agentType)
	if err != nil {
		return nil, err
	}
	return agent, nil
}

// DeleteAgent remove um agente (compatibilidade legada).
func (a *App) DeleteAgent(agentID uint) error {
	return a.DeleteAgentSession(agentID)
}

// ListAgents lista agentes do workspace ativo (compatibilidade legada).
func (a *App) ListAgents() ([]database.AgentInstance, error) {
	if a.db == nil {
		return []database.AgentInstance{}, nil
	}

	ws, err := a.db.GetActiveWorkspace()
	if err != nil {
		return []database.AgentInstance{}, nil
	}

	return a.db.ListAgents(ws.ID)
}

// SaveLayoutState salva o estado do layout (JSON) na config do usuário
func (a *App) SaveLayoutState(layoutJSON string) error {
	if a.db == nil {
		return nil
	}
	cfg, err := a.db.GetConfig()
	if err != nil {
		// Criar config se não existe
		cfg = &database.UserConfig{
			Theme:       "dark",
			LayoutState: layoutJSON,
		}
		return a.db.UpdateConfig(cfg)
	}
	cfg.LayoutState = layoutJSON
	return a.db.UpdateConfig(cfg)
}

// GetLayoutState retorna o estado do layout salvo
func (a *App) GetLayoutState() (string, error) {
	if a.db == nil {
		return "", fmt.Errorf("database not initialized")
	}
	cfg, err := a.db.GetConfig()
	if err != nil {
		return "", err
	}
	return cfg.LayoutState, nil
}

// GetHydrationData retorna o payload inicial (para fallback do frontend)
func (a *App) GetHydrationData() HydrationPayload {
	return a.getHydrationPayload()
}

func normalizeTheme(theme string) string {
	switch strings.ToLower(strings.TrimSpace(theme)) {
	case "light":
		return "light"
	case "hacker":
		return "hacker"
	case "nvim":
		return "nvim"
	case "min-dark", "mindark", "min_dark", "min dark":
		return "min-dark"
	default:
		return "dark"
	}
}

func normalizeLanguage(language string) string {
	normalized := strings.ToLower(strings.TrimSpace(language))
	switch normalized {
	case "en", "en-us", "en_us":
		return "en-US"
	default:
		return "pt-BR"
	}
}

const defaultTerminalFontFamily = "JetBrains Mono"
const defaultTerminalCursorStyle = "line"

func normalizeTerminalFontSize(size int) int {
	if size < 10 {
		return 10
	}
	if size > 24 {
		return 24
	}
	return size
}

func normalizeTerminalFontFamily(family string) string {
	cleaned := strings.TrimSpace(family)
	if cleaned == "" {
		return defaultTerminalFontFamily
	}

	cleaned = strings.ReplaceAll(cleaned, "\n", " ")
	cleaned = strings.ReplaceAll(cleaned, "\r", " ")
	cleaned = strings.ReplaceAll(cleaned, "\t", " ")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if cleaned == "" {
		return defaultTerminalFontFamily
	}

	// Famílias iniciadas com "." são internas do sistema e não são estáveis no xterm.
	if strings.HasPrefix(cleaned, ".") {
		return defaultTerminalFontFamily
	}

	if len(cleaned) > 120 {
		cleaned = strings.TrimSpace(cleaned[:120])
	}

	return cleaned
}

func normalizeTerminalCursorStyle(style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "block":
		return "block"
	case "underline", "under-line", "under_line":
		return "underline"
	case "line", "bar", "beam", "vertical":
		return "line"
	default:
		return defaultTerminalCursorStyle
	}
}

type shortcutBindingPayload struct {
	Key   string `json:"key"`
	Meta  bool   `json:"meta"`
	Shift bool   `json:"shift"`
	Alt   bool   `json:"alt"`
}

func normalizeShortcutBindingsJSON(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return "{}", nil
	}

	var parsed map[string]shortcutBindingPayload
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return "", fmt.Errorf("invalid shortcut bindings JSON: %w", err)
	}

	cleaned := make(map[string]shortcutBindingPayload, len(parsed))
	for id, binding := range parsed {
		normalizedID := strings.TrimSpace(id)
		if normalizedID == "" {
			continue
		}

		key := strings.TrimSpace(binding.Key)
		if key == "" {
			continue
		}

		if len(key) > 24 {
			continue
		}

		cleaned[normalizedID] = shortcutBindingPayload{
			Key:   key,
			Meta:  binding.Meta,
			Shift: binding.Shift,
			Alt:   binding.Alt,
		}
	}

	serialized, err := json.Marshal(cleaned)
	if err != nil {
		return "", fmt.Errorf("failed to serialize shortcut bindings: %w", err)
	}

	return string(serialized), nil
}

// SaveTheme persiste o tema escolhido pelo usuário.
func (a *App) SaveTheme(theme string) error {
	if a.db == nil {
		return nil
	}

	cfg, err := a.db.GetConfig()
	if err != nil {
		return err
	}

	cfg.Theme = normalizeTheme(theme)
	return a.db.UpdateConfig(cfg)
}

// SaveLanguage persiste o idioma escolhido pelo usuário.
func (a *App) SaveLanguage(language string) error {
	if a.db == nil {
		return nil
	}

	cfg, err := a.db.GetConfig()
	if err != nil {
		return err
	}

	cfg.Language = normalizeLanguage(language)
	return a.db.UpdateConfig(cfg)
}

// SaveDefaultShell persiste o shell escolhido pelo usuário.
func (a *App) SaveDefaultShell(shell string) error {
	if a.db == nil {
		return nil
	}

	cfg, err := a.db.GetConfig()
	if err != nil {
		return err
	}

	cfg.DefaultShell = strings.TrimSpace(shell)
	return a.db.UpdateConfig(cfg)
}

// SaveTerminalFontSize persiste o tamanho da fonte do terminal.
func (a *App) SaveTerminalFontSize(size int) error {
	if a.db == nil {
		return nil
	}

	cfg, err := a.db.GetConfig()
	if err != nil {
		return err
	}

	cfg.FontSize = normalizeTerminalFontSize(size)
	return a.db.UpdateConfig(cfg)
}

// SaveTerminalFontFamily persiste a família de fonte do terminal.
func (a *App) SaveTerminalFontFamily(family string) error {
	if a.db == nil {
		return nil
	}

	cfg, err := a.db.GetConfig()
	if err != nil {
		return err
	}

	cfg.FontFamily = normalizeTerminalFontFamily(family)
	return a.db.UpdateConfig(cfg)
}

// SaveTerminalCursorStyle persiste o estilo do cursor do terminal.
func (a *App) SaveTerminalCursorStyle(style string) error {
	if a.db == nil {
		return nil
	}

	cfg, err := a.db.GetConfig()
	if err != nil {
		return err
	}

	cfg.CursorStyle = normalizeTerminalCursorStyle(style)
	return a.db.UpdateConfig(cfg)
}

// SaveShortcutBindings persiste os atalhos customizados do usuário como JSON.
func (a *App) SaveShortcutBindings(bindingsJSON string) error {
	if a.db == nil {
		return nil
	}

	normalized, err := normalizeShortcutBindingsJSON(bindingsJSON)
	if err != nil {
		return err
	}

	cfg, err := a.db.GetConfig()
	if err != nil {
		return err
	}

	cfg.ShortcutBindings = normalized
	return a.db.UpdateConfig(cfg)
}

// GetAvailableShells retorna a lista de shells disponíveis no sistema.
func (a *App) GetAvailableShells() ([]string, error) {
	return terminal.GetAvailableShells(), nil
}

// GetAvailableTerminalFonts retorna famílias de fontes instaladas no sistema.
func (a *App) GetAvailableTerminalFonts() ([]string, error) {
	return terminal.GetAvailableFonts(), nil
}

// CompleteOnboarding marca onboarding como concluído.
func (a *App) CompleteOnboarding() error {
	if a.db == nil {
		return nil
	}

	cfg, err := a.db.GetConfig()
	if err != nil {
		return err
	}

	cfg.OnboardingCompleted = true
	return a.db.UpdateConfig(cfg)
}

// === GitHub Bindings (expostos ao Frontend) ===

// GHListRepositories lista repositórios do usuário
func (a *App) GHListRepositories() ([]gh.Repository, error) {
	if a.github == nil {
		return nil, nil
	}
	return a.github.ListRepositories()
}

// GHListPullRequests lista PRs de um repositório
func (a *App) GHListPullRequests(owner, repo, state string, first int) ([]gh.PullRequest, error) {
	if a.github == nil {
		return nil, nil
	}
	filters := gh.PRFilters{State: state, First: first}
	return a.github.ListPullRequests(owner, repo, filters)
}

// GHGetPullRequest busca detalhes de um PR
func (a *App) GHGetPullRequest(owner, repo string, number int) (*gh.PullRequest, error) {
	if a.github == nil {
		return nil, nil
	}
	return a.github.GetPullRequest(owner, repo, number)
}

// GHGetPullRequestDiff busca o diff de um PR
func (a *App) GHGetPullRequestDiff(owner, repo string, number int, first int, after string) (*gh.Diff, error) {
	if a.github == nil {
		return nil, nil
	}
	pagination := gh.DiffPagination{First: first}
	if after != "" {
		pagination.After = &after
	}
	return a.github.GetPullRequestDiff(owner, repo, number, pagination)
}

// GHCreatePullRequest cria um novo PR
func (a *App) GHCreatePullRequest(owner, repo, title, body, head, base string, isDraft bool) (*gh.PullRequest, error) {
	if a.github == nil {
		return nil, nil
	}
	return a.github.CreatePullRequest(gh.CreatePRInput{
		Owner: owner, Repo: repo, Title: title, Body: body,
		HeadBranch: head, BaseBranch: base, IsDraft: isDraft,
	})
}

// GHMergePullRequest faz merge de um PR
func (a *App) GHMergePullRequest(owner, repo string, number int, method string) error {
	if a.github == nil {
		return nil
	}
	return a.github.MergePullRequest(owner, repo, number, gh.MergeMethod(method))
}

// GHClosePullRequest fecha um PR
func (a *App) GHClosePullRequest(owner, repo string, number int) error {
	if a.github == nil {
		return nil
	}
	return a.github.ClosePullRequest(owner, repo, number)
}

// GHListReviews lista reviews de um PR
func (a *App) GHListReviews(owner, repo string, prNumber int) ([]gh.Review, error) {
	if a.github == nil {
		return nil, nil
	}
	return a.github.ListReviews(owner, repo, prNumber)
}

// GHCreateReview cria um review em um PR
func (a *App) GHCreateReview(owner, repo string, prNumber int, body, event string) (*gh.Review, error) {
	if a.github == nil {
		return nil, nil
	}
	return a.github.CreateReview(gh.CreateReviewInput{
		Owner: owner, Repo: repo, PRNumber: prNumber, Body: body, Event: event,
	})
}

// GHListComments lista comentários de um PR
func (a *App) GHListComments(owner, repo string, prNumber int) ([]gh.Comment, error) {
	if a.github == nil {
		return nil, nil
	}
	return a.github.ListComments(owner, repo, prNumber)
}

// GHCreateComment cria um comentário em um PR
func (a *App) GHCreateComment(owner, repo string, prNumber int, body string) (*gh.Comment, error) {
	if a.github == nil {
		return nil, nil
	}
	return a.github.CreateComment(gh.CreateCommentInput{
		Owner: owner, Repo: repo, PRNumber: prNumber, Body: body,
	})
}

// GHCreateInlineComment cria um comentário inline no diff
func (a *App) GHCreateInlineComment(owner, repo string, prNumber int, body, path string, line int, side string) (*gh.Comment, error) {
	if a.github == nil {
		return nil, nil
	}
	return a.github.CreateInlineComment(gh.InlineCommentInput{
		Owner: owner, Repo: repo, PRNumber: prNumber,
		Body: body, Path: path, Line: line, Side: side,
	})
}

// GHListIssues lista issues de um repositório
func (a *App) GHListIssues(owner, repo, state string, first int) ([]gh.Issue, error) {
	if a.github == nil {
		return nil, nil
	}
	filters := gh.IssueFilters{State: state, First: first}
	return a.github.ListIssues(owner, repo, filters)
}

// GHCreateIssue cria uma nova issue
func (a *App) GHCreateIssue(owner, repo, title, body string) (*gh.Issue, error) {
	if a.github == nil {
		return nil, nil
	}
	return a.github.CreateIssue(gh.CreateIssueInput{
		Owner: owner, Repo: repo, Title: title, Body: body,
	})
}

// GHUpdateIssue atualiza uma issue
func (a *App) GHUpdateIssue(owner, repo string, number int, title, body, state *string) error {
	if a.github == nil {
		return nil
	}
	return a.github.UpdateIssue(owner, repo, number, gh.UpdateIssueInput{
		Title: title, Body: body, State: state,
	})
}

// GHListBranches lista branches de um repositório
func (a *App) GHListBranches(owner, repo string) ([]gh.Branch, error) {
	if a.github == nil {
		return nil, nil
	}
	return a.github.ListBranches(owner, repo)
}

// GHCreateBranch cria uma nova branch
func (a *App) GHCreateBranch(owner, repo, name, sourceBranch string) (*gh.Branch, error) {
	if a.github == nil {
		return nil, nil
	}
	return a.github.CreateBranch(owner, repo, name, sourceBranch)
}

// GHInvalidateCache invalida o cache de um repositório
func (a *App) GHInvalidateCache(owner, repo string) {
	if a.github == nil {
		return
	}
	a.github.InvalidateCache(owner, repo)
}

// === FileWatcher Bindings (expostos ao Frontend) ===

// WatchProject inicia monitoramento do .git de um projeto
func (a *App) WatchProject(projectPath string) error {
	if a.fileWatcher == nil {
		return nil
	}
	return a.fileWatcher.Watch(projectPath)
}

// UnwatchProject para monitoramento de um projeto
func (a *App) UnwatchProject(projectPath string) error {
	if a.fileWatcher == nil {
		return nil
	}
	return a.fileWatcher.Unwatch(projectPath)
}

// GetCurrentBranch retorna a branch atual do projeto
func (a *App) GetCurrentBranch(projectPath string) (string, error) {
	if a.fileWatcher == nil {
		return "", nil
	}
	return a.fileWatcher.GetCurrentBranch(projectPath)
}

// GetLastCommit retorna informações do último commit
func (a *App) GetLastCommit(projectPath string) (*fw.CommitInfo, error) {
	if a.fileWatcher == nil {
		return nil, nil
	}
	return a.fileWatcher.GetLastCommit(projectPath)
}

// === GitActivity Bindings (expostos ao Frontend) ===

// GitActivityList retorna eventos de atividade Git em ordem mais recente primeiro.
func (a *App) GitActivityList(limit int, eventType string, repoPath string) []ga.Event {
	if a.gitActivity == nil {
		return []ga.Event{}
	}

	opts := ga.ListOptions{
		Limit:    limit,
		RepoPath: repoPath,
	}
	if normalized := strings.TrimSpace(eventType); normalized != "" {
		opts.Type = ga.EventType(normalized)
	}

	return a.gitActivity.ListEvents(opts)
}

// GitActivityGet retorna um evento específico por ID.
func (a *App) GitActivityGet(eventID string) (*ga.Event, error) {
	if a.gitActivity == nil {
		return nil, fmt.Errorf("git activity service not initialized")
	}
	event, ok := a.gitActivity.GetEvent(strings.TrimSpace(eventID))
	if !ok {
		return nil, fmt.Errorf("git activity event not found")
	}
	return event, nil
}

// GitActivityClear remove todos os eventos armazenados.
func (a *App) GitActivityClear() {
	if a.gitActivity == nil {
		return
	}
	a.gitActivity.Clear()
}

// GitActivityCount retorna quantidade atual de eventos no buffer.
func (a *App) GitActivityCount() int {
	if a.gitActivity == nil {
		return 0
	}
	return a.gitActivity.Count()
}

// GitActivityGetStagedFiles retorna resumo de arquivos staged para um repositório.
func (a *App) GitActivityGetStagedFiles(repoPath string) ([]ga.EventFile, error) {
	return ga.CollectStagedFiles(repoPath)
}

// GitActivityGetStagedDiff retorna diff staged (geral ou por arquivo).
func (a *App) GitActivityGetStagedDiff(repoPath string, filePath string) (string, error) {
	return ga.GetStagedDiff(repoPath, filePath)
}

// GitActivityUnstageFile remove um arquivo do stage.
func (a *App) GitActivityUnstageFile(repoPath string, filePath string) error {
	return ga.UnstageFile(repoPath, filePath)
}

// GitActivityDiscardFile descarta mudanças locais de um arquivo.
func (a *App) GitActivityDiscardFile(repoPath string, filePath string) error {
	return ga.DiscardFile(repoPath, filePath)
}

// === Polling Bindings (expostos ao Frontend) ===

// StartPolling inicia polling inteligente para um repositório
func (a *App) StartPolling(owner, repo string) {
	if a.poller == nil {
		return
	}
	a.poller.StartPolling(owner, repo)
}

// StopPolling para o polling
func (a *App) StopPolling() {
	if a.poller == nil {
		return
	}
	a.poller.StopPolling()
}

// SetPollingContext atualiza o contexto de polling
func (a *App) SetPollingContext(context string) {
	if a.poller == nil {
		return
	}
	a.poller.SetContext(gh.PollingContext(context))
}

// GetRateLimitInfo retorna informações do rate limit do GitHub
func (a *App) GetRateLimitInfo() gh.RateLimitInfo {
	if a.poller == nil {
		return gh.RateLimitInfo{Remaining: 5000, Limit: 5000}
	}
	return a.poller.GetRateLimitInfo()
}

// === Session / P2P Bindings (expostos ao Frontend) ===

type sessionGatewayErrorResponse struct {
	Error string `json:"error"`
}

func normalizeSessionListenerAddr(raw, fallback string) string {
	if candidate := strings.TrimSpace(raw); candidate != "" {
		return candidate
	}
	return fallback
}

func shouldStartSessionListener(addr string) bool {
	switch strings.ToLower(strings.TrimSpace(addr)) {
	case "", "off", "disabled", "none", "false", "0":
		return false
	default:
		return true
	}
}

func sessionPublicHostFromListenAddr(addr string) string {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return "127.0.0.1"
	}
	if strings.HasPrefix(trimmed, ":") {
		return "127.0.0.1" + trimmed
	}

	host, port, err := net.SplitHostPort(trimmed)
	if err != nil {
		return trimmed
	}

	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func normalizeSessionGatewayBaseURL(raw string, fallbackListenAddr string) string {
	base := strings.TrimSpace(raw)
	if base == "" {
		base = "http://" + sessionPublicHostFromListenAddr(fallbackListenAddr)
	}
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}

	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return defaultSessionGatewayBaseURL
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		scheme = "http"
	}
	parsed.Scheme = scheme
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func normalizeSessionSignalingURL(raw string, fallbackListenAddr string) string {
	base := strings.TrimSpace(raw)
	if base == "" {
		base = "ws://" + sessionPublicHostFromListenAddr(fallbackListenAddr) + "/ws/signal"
	}
	if !strings.Contains(base, "://") {
		base = "ws://" + base
	}

	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return defaultSessionSignalingBaseURL
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	case "ws", "wss":
		// ok
	default:
		parsed.Scheme = "ws"
	}

	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/ws/signal"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func (a *App) configureSessionNetworking() {
	a.sessionGatewayAddr = normalizeSessionListenerAddr(os.Getenv("ORCH_SESSION_GATEWAY_LISTEN_ADDR"), defaultSessionGatewayListenAddr)
	a.sessionGatewayURL = normalizeSessionGatewayBaseURL(os.Getenv("ORCH_SESSION_GATEWAY_BASE_URL"), a.sessionGatewayAddr)
	a.signalingAddr = normalizeSessionListenerAddr(os.Getenv("ORCH_SESSION_SIGNALING_LISTEN_ADDR"), defaultSessionSignalingListenAddr)
	a.signalingURL = normalizeSessionSignalingURL(os.Getenv("ORCH_SESSION_SIGNALING_BASE_URL"), a.signalingAddr)
}

func (a *App) resolvedSessionGatewayBaseURL() string {
	if a != nil {
		if custom := strings.TrimSpace(a.sessionGatewayURL); custom != "" {
			return strings.TrimRight(custom, "/")
		}
	}
	return defaultSessionGatewayBaseURL
}

func (a *App) resolvedSessionSignalingURL() string {
	if a != nil {
		if custom := strings.TrimSpace(a.signalingURL); custom != "" {
			return custom
		}
	}
	return defaultSessionSignalingBaseURL
}

// SessionGetSignalingURL retorna a URL base do signaling WebSocket.
func (a *App) SessionGetSignalingURL() string {
	return a.resolvedSessionSignalingURL()
}

func (a *App) callSessionGateway(method, path string, requestBody interface{}, responseBody interface{}) error {
	var bodyReader io.Reader
	if requestBody != nil {
		payload, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshal gateway request: %w", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, a.resolvedSessionGatewayBaseURL()+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create gateway request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("gateway request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errPayload sessionGatewayErrorResponse
		if decodeErr := json.NewDecoder(resp.Body).Decode(&errPayload); decodeErr == nil && errPayload.Error != "" {
			return errors.New(errPayload.Error)
		}
		return fmt.Errorf("gateway request failed with status %d", resp.StatusCode)
	}

	if responseBody == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(responseBody); err != nil {
		return fmt.Errorf("decode gateway response: %w", err)
	}
	return nil
}

func (a *App) gatewayJoinSession(code, guestUserID string, guestInfo session.GuestInfo) (*session.JoinResult, error) {
	var result session.JoinResult
	err := a.callSessionGateway(http.MethodPost, "/api/session/join", map[string]interface{}{
		"code":        code,
		"guestUserID": guestUserID,
		"guestInfo":   guestInfo,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *App) gatewayCreateSession(hostUserID string, cfg session.SessionConfig) (*session.Session, error) {
	var result session.Session
	err := a.callSessionGateway(http.MethodPost, "/api/session/create", map[string]interface{}{
		"hostUserID": hostUserID,
		"config":     cfg,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *App) gatewayGetSession(sessionID string) (*session.Session, error) {
	var result session.Session
	path := "/api/session/get?sessionID=" + url.QueryEscape(sessionID)
	if err := a.callSessionGateway(http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *App) gatewayGetActiveSession(userID string) (*session.Session, error) {
	var result session.Session
	path := "/api/session/active?userID=" + url.QueryEscape(userID)
	if err := a.callSessionGateway(http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *App) gatewayListPendingGuests(sessionID string) ([]session.GuestRequest, error) {
	var result []session.GuestRequest
	path := "/api/session/pending?sessionID=" + url.QueryEscape(sessionID)
	if err := a.callSessionGateway(http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (a *App) gatewayApproveGuest(sessionID, guestUserID string) error {
	return a.callSessionGateway(http.MethodPost, "/api/session/approve", map[string]interface{}{
		"sessionID":   sessionID,
		"guestUserID": guestUserID,
	}, nil)
}

func (a *App) gatewayRejectGuest(sessionID, guestUserID string) error {
	return a.callSessionGateway(http.MethodPost, "/api/session/reject", map[string]interface{}{
		"sessionID":   sessionID,
		"guestUserID": guestUserID,
	}, nil)
}

func (a *App) gatewayEndSession(sessionID string) error {
	return a.callSessionGateway(http.MethodPost, "/api/session/end", map[string]interface{}{
		"sessionID": sessionID,
	}, nil)
}

func (a *App) gatewaySetGuestPermission(sessionID, guestUserID, permission string) error {
	return a.callSessionGateway(http.MethodPost, "/api/session/permission", map[string]interface{}{
		"sessionID":   sessionID,
		"guestUserID": guestUserID,
		"permission":  permission,
	}, nil)
}

func (a *App) gatewayKickGuest(sessionID, guestUserID string) error {
	return a.callSessionGateway(http.MethodPost, "/api/session/kick", map[string]interface{}{
		"sessionID":   sessionID,
		"guestUserID": guestUserID,
	}, nil)
}

func isSessionNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "session not found") || strings.Contains(msg, "session not found for code")
}

func sessionPersistUntil(sess *session.Session, now time.Time) time.Time {
	if sess == nil {
		return now.Add(30 * time.Minute)
	}
	if sess.Status == session.StatusWaiting {
		if sess.ExpiresAt.After(now.Add(2 * time.Minute)) {
			return sess.ExpiresAt
		}
		return now.Add(2 * time.Minute)
	}
	// Sessões ativas continuam válidas por uma janela curta pós-restart para retomada.
	return now.Add(12 * time.Hour)
}

func (a *App) persistSessionState(sessionID string) {
	if a.db == nil || a.session == nil || strings.TrimSpace(sessionID) == "" {
		return
	}

	sess, err := a.session.GetSession(sessionID)
	if err != nil {
		return
	}
	if sess.Status == session.StatusEnded {
		a.deletePersistedSessionState(sessionID)
		return
	}

	now := time.Now()
	if sess.Status == session.StatusWaiting && now.After(sess.ExpiresAt) {
		a.deletePersistedSessionState(sessionID)
		return
	}

	payload, err := json.Marshal(sess)
	if err != nil {
		log.Printf("[ORCH][SESSION] failed to marshal session state %s: %v", sessionID, err)
		return
	}

	state := &database.CollabSessionState{
		SessionID:    sess.ID,
		HostUserID:   sess.HostUserID,
		Code:         sess.Code,
		Status:       string(sess.Status),
		ExpiresAt:    sess.ExpiresAt,
		PersistUntil: sessionPersistUntil(sess, now),
		Payload:      string(payload),
	}
	if err := a.db.UpsertCollabSessionState(state); err != nil {
		log.Printf("[ORCH][SESSION] failed to persist session state %s: %v", sessionID, err)
	}
}

func (a *App) deletePersistedSessionState(sessionID string) {
	if a.db == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	if err := a.db.DeleteCollabSessionState(sessionID); err != nil {
		log.Printf("[ORCH][SESSION] failed to delete persisted session state %s: %v", sessionID, err)
	}
}

func (a *App) restorePersistedSessionStates() {
	if a.db == nil || a.session == nil {
		return
	}

	now := time.Now()
	if err := a.db.CleanupExpiredCollabSessionStates(now); err != nil {
		log.Printf("[ORCH][SESSION] cleanup persisted states failed: %v", err)
	}

	states, err := a.db.ListRestorableCollabSessionStates(now, 300)
	if err != nil {
		log.Printf("[ORCH][SESSION] failed to load persisted states: %v", err)
		return
	}

	restoredCount := 0
	for _, row := range states {
		var restored session.Session
		if err := json.Unmarshal([]byte(row.Payload), &restored); err != nil {
			log.Printf("[ORCH][SESSION] invalid persisted payload sessionID=%s err=%v", row.SessionID, err)
			a.deletePersistedSessionState(row.SessionID)
			continue
		}
		if restored.ID == "" || restored.Status == session.StatusEnded {
			a.deletePersistedSessionState(row.SessionID)
			continue
		}
		if restored.Status == session.StatusWaiting && now.After(restored.ExpiresAt) {
			a.deletePersistedSessionState(row.SessionID)
			continue
		}

		if err := a.session.RestoreSession(&restored); err != nil {
			log.Printf("[ORCH][SESSION] failed to restore session %s: %v", row.SessionID, err)
			continue
		}
		restoredCount++
	}

	if restoredCount > 0 {
		log.Printf("[ORCH][SESSION] restored %d session(s) from persistence", restoredCount)
	}
}

// SessionCreate cria uma nova sessão de colaboração limitada a um workspace.
func (a *App) SessionCreate(maxGuests int, mode string, allowAnonymous bool, workspaceID uint) (*session.Session, error) {
	hostUserID := a.resolveSessionHostUserID()

	if a.db == nil {
		return nil, fmt.Errorf("database service not initialized")
	}

	var (
		scopedWorkspace *database.Workspace
		err             error
	)
	if workspaceID == 0 {
		scopedWorkspace, err = a.db.GetActiveWorkspace()
	} else {
		scopedWorkspace, err = a.db.GetWorkspace(workspaceID)
	}
	if err != nil || scopedWorkspace == nil {
		return nil, fmt.Errorf("workspace not found")
	}

	cfg := session.SessionConfig{
		MaxGuests:      maxGuests,
		DefaultPerm:    session.PermReadOnly,
		AllowAnonymous: allowAnonymous,
		Mode:           session.SessionMode(mode),
		WorkspaceID:    scopedWorkspace.ID,
		WorkspaceName:  strings.TrimSpace(scopedWorkspace.Name),
	}
	if cfg.WorkspaceName == "" {
		cfg.WorkspaceName = fmt.Sprintf("Workspace %d", scopedWorkspace.ID)
	}

	if cfg.Mode == "" {
		cfg.Mode = session.ModeLiveShare
	}
	if cfg.Mode != session.ModeDocker && cfg.Mode != session.ModeLiveShare {
		cfg.Mode = session.ModeLiveShare
	}

	var containerID string
	if cfg.Mode == session.ModeDocker {
		if a.docker == nil || !a.docker.IsDockerAvailable() {
			cfg.Mode = session.ModeLiveShare
			runtime.EventsEmit(a.ctx, "session:docker_fallback", map[string]string{
				"reason": "Docker não está disponível. Sessão iniciada em Live Share.",
			})
		} else {
			if scopedWorkspace.Path == "" {
				return nil, fmt.Errorf("docker mode requires a workspace with a valid path")
			}

			cfg.ProjectPath = scopedWorkspace.Path

			// Verifica se existe imagem customizada criada pelo Stack Builder
			customImage := "orch-custom-stack:latest"
			if a.docker.ImageExists(customImage) {
				cfg.DockerImage = customImage
				log.Printf("[ORCH] Using custom stack image: %s", customImage)
			} else {
				cfg.DockerImage = a.docker.DetectImage(scopedWorkspace.Path)
			}

			containerCfg := docker.ContainerConfig{
				Image:       cfg.DockerImage,
				ProjectPath: scopedWorkspace.Path,
				Memory:      "2g",
				CPUs:        "2",
				Shell:       "/bin/sh",
				ReadOnly:    true,
				NetworkMode: "none",
			}

			createdContainerID, createErr := a.docker.CreateContainer(containerCfg)
			if createErr != nil {
				return nil, createErr
			}
			containerID = createdContainerID

			if err := a.docker.StartContainer(containerID); err != nil {
				_ = a.docker.RemoveContainer(containerID)
				return nil, err
			}

			if err := a.docker.WaitUntilRunning(containerID, 5*time.Second); err != nil {
				_ = a.docker.StopContainer(containerID)
				_ = a.docker.RemoveContainer(containerID)
				return nil, err
			}
		}
	}

	var createdSession *session.Session

	if !a.sessionGatewayOwner {
		createdSession, err = a.gatewayCreateSession(hostUserID, cfg)
		if err != nil {
			if containerID != "" && a.docker != nil {
				_ = a.docker.StopContainer(containerID)
				_ = a.docker.RemoveContainer(containerID)
			}
			return nil, fmt.Errorf("session gateway create failed in client mode: %w", err)
		}
	} else {
		if a.session == nil {
			if containerID != "" && a.docker != nil {
				_ = a.docker.StopContainer(containerID)
				_ = a.docker.RemoveContainer(containerID)
			}
			return nil, fmt.Errorf("session service not initialized")
		}
		createdSession, err = a.session.CreateSession(hostUserID, cfg)
		if err != nil {
			if containerID != "" && a.docker != nil {
				_ = a.docker.StopContainer(containerID)
				_ = a.docker.RemoveContainer(containerID)
			}
			return nil, err
		}
	}

	if containerID != "" {
		a.setSessionContainer(createdSession.ID, containerID)
		a.auditSessionEvent(createdSession.ID, hostUserID, "container_started", fmt.Sprintf("container=%s image=%s", containerID, cfg.DockerImage))
	}
	a.auditSessionEvent(
		createdSession.ID,
		hostUserID,
		"session_created",
		fmt.Sprintf(
			"mode=%s allowAnonymous=%t workspaceID=%d workspaceName=%s",
			createdSession.Mode,
			allowAnonymous,
			cfg.WorkspaceID,
			cfg.WorkspaceName,
		),
	)
	if a.sessionGatewayOwner {
		a.persistSessionState(createdSession.ID)
	}

	return createdSession, nil
}

// SessionJoin entra em uma sessão usando um código
func (a *App) SessionJoin(code string, name string, email string) (*session.JoinResult, error) {
	guestUserID := a.anonymousGuestID
	avatarURL := ""
	if a.auth != nil {
		if user, err := a.auth.GetCurrentUser(); err == nil && user != nil {
			guestUserID = user.ID
			if name == "" {
				name = user.Name
			}
			if email == "" {
				email = user.Email
			}
			avatarURL = user.AvatarURL
		}
	}

	if name == "" {
		name = "Anonymous Guest"
	}

	guestInfo := session.GuestInfo{
		Name:      name,
		Email:     email,
		AvatarURL: avatarURL,
	}

	if a.session == nil || !a.sessionGatewayOwner {
		result, err := a.gatewayJoinSession(code, guestUserID, guestInfo)
		if err != nil {
			if !a.sessionGatewayOwner {
				return nil, fmt.Errorf("gateway join failed in client mode: %w", err)
			}
			return nil, fmt.Errorf("session service not initialized and gateway join failed: %w", err)
		}
		a.auditSessionEvent(result.SessionID, guestUserID, "guest_requested_join", fmt.Sprintf("name=%s", name))
		return result, nil
	}

	result, err := a.session.JoinSession(code, guestUserID, guestInfo)
	if err != nil {
		// Fallback para gateway compartilhado em outra instância local.
		if isSessionNotFoundErr(err) {
			remoteResult, remoteErr := a.gatewayJoinSession(code, guestUserID, guestInfo)
			if remoteErr != nil {
				return nil, fmt.Errorf("%v (gateway fallback failed: %w)", err, remoteErr)
			}
			a.auditSessionEvent(remoteResult.SessionID, guestUserID, "guest_requested_join", fmt.Sprintf("name=%s", name))
			return remoteResult, nil
		}
		return nil, err
	}

	a.auditSessionEvent(result.SessionID, guestUserID, "guest_requested_join", fmt.Sprintf("name=%s", name))
	a.persistSessionState(result.SessionID)
	return result, nil
}

// SessionApproveGuest aprova um guest na sessão
func (a *App) SessionApproveGuest(sessionID, guestUserID string) error {
	var err error
	if a.session == nil || !a.sessionGatewayOwner {
		err = a.gatewayApproveGuest(sessionID, guestUserID)
	} else {
		err = a.session.ApproveGuest(sessionID, guestUserID)
		if isSessionNotFoundErr(err) {
			err = a.gatewayApproveGuest(sessionID, guestUserID)
		}
	}
	if err != nil {
		return err
	}
	a.auditSessionEvent(sessionID, guestUserID, "guest_approved", "Host approved guest in waiting room")
	a.auditSessionEvent(sessionID, guestUserID, "guest_entered", "Guest admitted into collaboration session")
	a.syncGuestPermissionAcrossPTYs(sessionID, guestUserID)
	if a.sessionGatewayOwner {
		a.persistSessionState(sessionID)
	}
	return nil
}

// SessionRejectGuest rejeita um guest na sessão
func (a *App) SessionRejectGuest(sessionID, guestUserID string) error {
	var err error
	if a.session == nil || !a.sessionGatewayOwner {
		err = a.gatewayRejectGuest(sessionID, guestUserID)
	} else {
		err = a.session.RejectGuest(sessionID, guestUserID)
		if isSessionNotFoundErr(err) {
			err = a.gatewayRejectGuest(sessionID, guestUserID)
		}
	}
	if err != nil {
		return err
	}
	a.auditSessionEvent(sessionID, guestUserID, "guest_rejected", "Host rejected guest in waiting room")
	if a.sessionGatewayOwner {
		a.persistSessionState(sessionID)
	}
	return nil
}

// SessionEnd encerra a sessão ativa
func (a *App) SessionEnd(sessionID string) error {
	guestsToRevoke := make([]string, 0, 8)
	if current, getErr := a.SessionGetSession(sessionID); getErr == nil && current != nil {
		for _, guest := range current.Guests {
			guestsToRevoke = append(guestsToRevoke, guest.UserID)
		}
	}

	if containerID, ok := a.popSessionContainer(sessionID); ok && a.docker != nil {
		if err := a.docker.StopContainer(containerID); err != nil {
			log.Printf("[DOCKER] stop failed for %s: %s", containerID, a.sanitizeForLogs(err.Error()))
		}
		if err := a.docker.RemoveContainer(containerID); err != nil {
			log.Printf("[DOCKER] remove failed for %s: %s", containerID, a.sanitizeForLogs(err.Error()))
		}
		a.auditSessionEvent(sessionID, "system", "container_stopped", fmt.Sprintf("container=%s", containerID))
	}

	var err error
	if a.session == nil || !a.sessionGatewayOwner {
		err = a.gatewayEndSession(sessionID)
	} else {
		err = a.session.EndSession(sessionID)
		if isSessionNotFoundErr(err) {
			err = a.gatewayEndSession(sessionID)
		}
	}
	if err != nil {
		return err
	}
	if a.sessionGatewayOwner && a.signaling != nil {
		a.signaling.NotifySessionEnded(sessionID, "host")
	}
	for _, guestUserID := range guestsToRevoke {
		a.applyPermissionToAllPTY(guestUserID, terminal.PermissionNone)
	}
	a.auditSessionEvent(sessionID, "host", "session_ended", "Host ended the collaboration session")
	a.deletePersistedSessionState(sessionID)
	return nil
}

// SessionListPendingGuests lista pedidos de entrada pendentes
func (a *App) SessionListPendingGuests(sessionID string) ([]session.GuestRequest, error) {
	if a.session == nil || !a.sessionGatewayOwner {
		return a.gatewayListPendingGuests(sessionID)
	}
	pending, err := a.session.ListPendingGuests(sessionID)
	if err == nil {
		return pending, nil
	}
	if isSessionNotFoundErr(err) {
		return a.gatewayListPendingGuests(sessionID)
	}
	return nil, err
}

// SessionSetGuestPermission altera permissão de um guest
func (a *App) SessionSetGuestPermission(sessionID, guestUserID, permission string) error {
	previousPerm := ""
	if a.sessionGatewayOwner && a.session != nil {
		if sess, err := a.session.GetSession(sessionID); err == nil && sess != nil {
			for _, guest := range sess.Guests {
				if guest.UserID == guestUserID {
					previousPerm = string(guest.Permission)
					break
				}
			}
		}
	} else if sess, err := a.gatewayGetSession(sessionID); err == nil && sess != nil {
		for _, guest := range sess.Guests {
			if guest.UserID == guestUserID {
				previousPerm = string(guest.Permission)
				break
			}
		}
	}

	var err error
	if a.session == nil || !a.sessionGatewayOwner {
		err = a.gatewaySetGuestPermission(sessionID, guestUserID, permission)
	} else {
		err = a.session.SetGuestPermission(sessionID, guestUserID, permission)
		if isSessionNotFoundErr(err) {
			err = a.gatewaySetGuestPermission(sessionID, guestUserID, permission)
		}
	}
	if err != nil {
		return err
	}
	if a.sessionGatewayOwner && a.signaling != nil {
		a.signaling.NotifyPermissionChange(sessionID, guestUserID, permission)
	}
	if permission == string(session.PermReadOnly) {
		runtime.EventsEmit(a.ctx, "session:permission_revoked", map[string]string{
			"sessionID":   sessionID,
			"guestUserID": guestUserID,
		})
		a.auditSessionEvent(sessionID, guestUserID, "permission_revoked", "Write permission revoked by host")
	}
	a.syncGuestPermissionAcrossPTYs(sessionID, guestUserID)
	a.auditSessionEvent(sessionID, guestUserID, "permission_changed", fmt.Sprintf("from=%s to=%s", previousPerm, permission))
	if a.sessionGatewayOwner {
		a.persistSessionState(sessionID)
	}
	return nil
}

// SessionKickGuest remove um guest da sessão
func (a *App) SessionKickGuest(sessionID, guestUserID string) error {
	var err error
	if a.session == nil || !a.sessionGatewayOwner {
		err = a.gatewayKickGuest(sessionID, guestUserID)
	} else {
		err = a.session.KickGuest(sessionID, guestUserID)
		if isSessionNotFoundErr(err) {
			err = a.gatewayKickGuest(sessionID, guestUserID)
		}
	}
	if err != nil {
		return err
	}
	a.applyPermissionToAllPTY(guestUserID, terminal.PermissionNone)
	a.auditSessionEvent(sessionID, guestUserID, "guest_kicked", "Guest removed by host")
	a.auditSessionEvent(sessionID, guestUserID, "guest_left", "Guest left session after host removal")
	if a.sessionGatewayOwner {
		a.persistSessionState(sessionID)
	}
	return nil
}

// SessionGetActive retorna a sessão ativa do host
func (a *App) SessionGetActive() (*session.Session, error) {
	hostUserID := a.resolveSessionHostUserID()

	// Instâncias em client-mode não devem hidratar sessão ativa como Host.
	// Isso evita que um guest local assuma role=host por compartilhar o mesmo userID default.
	if !a.sessionGatewayOwner {
		return nil, fmt.Errorf("no active session in client mode")
	}
	if a.session == nil {
		return nil, fmt.Errorf("session service not initialized")
	}
	current, err := a.session.GetActiveSession(hostUserID)
	if err == nil {
		return current, nil
	}
	if strings.Contains(err.Error(), "no active session") {
		return a.gatewayGetActiveSession(hostUserID)
	}
	return nil, err
}

// SessionGetSession retorna detalhes de uma sessão
func (a *App) SessionGetSession(sessionID string) (*session.Session, error) {
	if a.session == nil || !a.sessionGatewayOwner {
		return a.gatewayGetSession(sessionID)
	}
	current, err := a.session.GetSession(sessionID)
	if err == nil {
		return current, nil
	}
	if isSessionNotFoundErr(err) {
		return a.gatewayGetSession(sessionID)
	}
	return nil, err
}

// SessionGetICEServers retorna a configuração de ICE servers
func (a *App) SessionGetICEServers() []session.ICEServerConfig {
	if a.session == nil {
		return []session.ICEServerConfig{}
	}
	return a.session.GetICEServers()
}

// SessionGetAuditLogs retorna os eventos de auditoria de uma sessão.
func (a *App) SessionGetAuditLogs(sessionID string, limit int) ([]database.AuditLog, error) {
	if a.db == nil {
		return []database.AuditLog{}, nil
	}
	return a.db.ListAuditEvents(sessionID, limit)
}

// SessionRestartEnvironment reinicia o container associado à sessão (modo Docker).
func (a *App) SessionRestartEnvironment(sessionID string) error {
	if a.docker == nil {
		return fmt.Errorf("docker service not initialized")
	}

	containerID, ok := a.getSessionContainer(sessionID)
	if !ok {
		return fmt.Errorf("no container found for session %s", sessionID)
	}

	if err := a.docker.RestartContainer(containerID); err != nil {
		return err
	}
	a.auditSessionEvent(sessionID, "host", "container_restarted", fmt.Sprintf("container=%s", containerID))
	return nil
}

// DockerIsAvailable informa se Docker está disponível e operacional.
func (a *App) DockerIsAvailable() bool {
	if a.docker == nil {
		return false
	}
	return a.docker.IsDockerAvailable()
}

// DockerDetectImage detecta automaticamente a imagem recomendada para um projeto.
func (a *App) DockerDetectImage(projectPath string) string {
	if a.docker == nil {
		return "alpine:latest"
	}
	return a.docker.DetectImage(projectPath)
}

// === Auth Bindings (expostos ao Frontend) ===

// AuthLogin inicia o fluxo de login OAuth
func (a *App) AuthLogin(provider string) error {
	if a.auth == nil {
		return fmt.Errorf("auth service not initialized")
	}

	// Configurar handler de callback
	callbackHandler := func(result *auth.AuthResult) {
		if result == nil {
			runtime.EventsEmit(a.ctx, "auth:error", "Authentication failed: no result")
			return
		}

		if result.Success {
			log.Println("[ORCH] Auth success via local callback!")
			runtime.EventsEmit(a.ctx, "auth:changed", a.auth.GetAuthState())
			runtime.WindowShow(a.ctx)
		} else {
			log.Printf("[ORCH] Auth failed: %s", result.Error)
			runtime.EventsEmit(a.ctx, "auth:error", result.Error)
		}
	}

	// Iniciar servidor de callback
	_, err := a.auth.StartCallbackServer(callbackHandler)
	if err != nil {
		return fmt.Errorf("failed to start callback server: %w", err)
	}

	url, err := a.auth.GetAuthURL(provider)
	if err != nil {
		a.auth.StopCallbackServer()
		return err
	}

	// Abrir URL no navegador padrão
	runtime.BrowserOpenURL(a.ctx, url)
	return nil
}

// AuthLogout faz logout do usuário
func (a *App) AuthLogout() error {
	if a.auth == nil {
		return nil
	}

	if err := a.auth.Logout(); err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "auth:changed", a.auth.GetAuthState())
	return nil
}

// GetAuthState retorna o estado atual de autenticação
func (a *App) GetAuthState() *auth.AuthState {
	if a.auth == nil {
		return &auth.AuthState{IsAuthenticated: false}
	}
	return a.auth.GetAuthState()
}

// HandleDeepLink processa links orch:// (chamado pelo macOS)
func (a *App) HandleDeepLink(urlStr string) {
	log.Printf("[ORCH] Deep Link received: %s", urlStr)

	// Validar scheme
	if !strings.HasPrefix(urlStr, "orch://auth/callback") {
		log.Printf("[ORCH] Ignored unknown deep link: %s", urlStr)
		return
	}

	// Parse manual simples para evitar dependencias complexas com net/url e custom schemes
	// orch://auth/callback?code=xxx&state=yyy
	parts := strings.Split(urlStr, "?")
	if len(parts) < 2 {
		log.Println("[ORCH] Deep link missing query params")
		return
	}

	queryParams := parts[1]
	values := make(map[string]string)
	for _, pair := range strings.Split(queryParams, "&") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			values[kv[0]] = kv[1]
		}
	}

	code := values["code"]

	if code == "" {
		log.Println("[ORCH] Deep link missing code")
		return
	}

	// Processar callback
	if a.auth != nil {
		result, err := a.auth.HandleCallback(code)
		if err != nil {
			log.Printf("[ORCH] Auth callback failed: %v", err)
			runtime.EventsEmit(a.ctx, "auth:error", err.Error())
			return
		}

		if result.Success {
			log.Println("[ORCH] Auth success via Deep Link!")
			runtime.EventsEmit(a.ctx, "auth:changed", a.auth.GetAuthState())

			// Focar janela do app
			runtime.WindowShow(a.ctx)
		}
	}
}

// === Docker Stack Builder Bindings ===

// StackBuildState é o estado serializado do build para o frontend
type StackBuildState struct {
	IsBuilding bool     `json:"isBuilding"`
	Logs       []string `json:"logs"`
	StartTime  int64    `json:"startTime"` // unix seconds, 0 se não está construindo
	Result     string   `json:"result"`    // "", "success", "error"
}

// BuildCustomStack inicia o processo de build da imagem customizada
func (a *App) BuildCustomStack(tools map[string]string) error {
	if a.docker == nil {
		return fmt.Errorf("docker service not initialized")
	}

	// Prevenir builds simultâneos
	a.stackBuildMu.Lock()
	if a.stackBuildRunning {
		a.stackBuildMu.Unlock()
		return fmt.Errorf("a build is already in progress")
	}
	a.stackBuildRunning = true
	a.stackBuildLogs = []string{"🚀 Iniciando build do ambiente..."}
	a.stackBuildStart = time.Now().Unix()
	a.stackBuildResult = ""
	a.stackBuildMu.Unlock()

	// Emitir estado inicial para o frontend
	runtime.EventsEmit(a.ctx, "docker:build:started", a.GetStackBuildState())

	go func() {
		cfg := docker.StackConfig{
			ImageName: "orch-custom-stack:latest",
			Tools:     tools,
		}

		logFn := func(line string) {
			a.stackBuildMu.Lock()
			a.stackBuildLogs = append(a.stackBuildLogs, line)
			a.stackBuildMu.Unlock()
			runtime.EventsEmit(a.ctx, "docker:build:log", line)
		}

		if err := a.docker.BuildStackImage(a.ctx, cfg, logFn); err != nil {
			a.stackBuildMu.Lock()
			errorMsg := err.Error()
			a.stackBuildLogs = append(a.stackBuildLogs, "❌ Error: "+errorMsg)
			a.stackBuildRunning = false
			a.stackBuildResult = "error"
			a.stackBuildMu.Unlock()
			runtime.EventsEmit(a.ctx, "docker:build:error", errorMsg)
		} else {
			a.stackBuildMu.Lock()
			a.stackBuildLogs = append(a.stackBuildLogs, "✅ Image built successfully")
			a.stackBuildRunning = false
			a.stackBuildResult = "success"
			a.stackBuildMu.Unlock()
			runtime.EventsEmit(a.ctx, "docker:build:success", "Image built successfully")
		}
	}()

	return nil
}

// GetStackBuildState retorna o estado atual do build (para reconexão do frontend)
func (a *App) GetStackBuildState() StackBuildState {
	a.stackBuildMu.RLock()
	defer a.stackBuildMu.RUnlock()

	logsCopy := make([]string, len(a.stackBuildLogs))
	copy(logsCopy, a.stackBuildLogs)

	return StackBuildState{
		IsBuilding: a.stackBuildRunning,
		Logs:       logsCopy,
		StartTime:  a.stackBuildStart,
		Result:     a.stackBuildResult,
	}
}

// GetCustomStackTools retorna a lista de ferramentas já instaladas na stack atual
func (a *App) GetCustomStackTools() (map[string]string, error) {
	if a.docker == nil {
		return map[string]string{}, nil
	}
	cfg, err := a.docker.LoadStackConfig()
	if err != nil {
		return map[string]string{}, err
	}
	return cfg.Tools, nil
}

// === Terminal Snapshot Bindings (Session Persistence) ===

// TerminalSnapshotDTO é o DTO para comunicação frontend↔backend.
type TerminalSnapshotDTO struct {
	PaneID    string `json:"paneId"`
	SessionID string `json:"sessionId"`
	PaneTitle string `json:"paneTitle"`
	PaneType  string `json:"paneType"`
	Shell     string `json:"shell"`
	Cwd       string `json:"cwd"`
	UseDocker bool   `json:"useDocker"`
	Config    string `json:"config,omitempty"`
	CLIType   string `json:"cliType,omitempty"`
}

// SaveTerminalSnapshots recebe dados dos panes do frontend,
// enriquece com detecção de CLI via process sniffing, e persiste no DB.
func (a *App) SaveTerminalSnapshots(dtos []TerminalSnapshotDTO) error {
	if a.db == nil {
		return fmt.Errorf("database not initialized")
	}

	snapshots := make([]database.TerminalSnapshot, 0, len(dtos))
	for _, dto := range dtos {
		cliType := dto.CLIType
		cwd := dto.Cwd

		// Enriquecer com detecção de CLI se o backend tem PTY ativa
		if a.ptyMgr != nil && dto.SessionID != "" {
			if pid, err := a.ptyMgr.GetProcessPID(dto.SessionID); err == nil {
				detected := terminal.DetectCLI(pid)
				if detected != terminal.CLINone {
					cliType = string(detected)
				}
				// Obter CWD real do processo
				if realCwd := terminal.GetProcessCwd(pid); realCwd != "" {
					cwd = realCwd
				}
			}
		}

		snapshots = append(snapshots, database.TerminalSnapshot{
			PaneID:    dto.PaneID,
			CLIType:   cliType,
			Shell:     dto.Shell,
			Cwd:       cwd,
			UseDocker: dto.UseDocker,
			PaneTitle: dto.PaneTitle,
			PaneType:  dto.PaneType,
			Config:    dto.Config,
		})
	}

	if err := a.db.SaveTerminalSnapshots(snapshots); err != nil {
		log.Printf("[ORCH] Error saving terminal snapshots: %v", err)
		return err
	}

	log.Printf("[ORCH] Saved %d terminal snapshots", len(snapshots))
	return nil
}

// GetTerminalSnapshots retorna os snapshots salvos para restauração.
func (a *App) GetTerminalSnapshots() ([]TerminalSnapshotDTO, error) {
	if a.db == nil {
		return []TerminalSnapshotDTO{}, nil
	}

	snapshots, err := a.db.GetTerminalSnapshots()
	if err != nil {
		return nil, err
	}

	dtos := make([]TerminalSnapshotDTO, 0, len(snapshots))
	for _, s := range snapshots {
		dtos = append(dtos, TerminalSnapshotDTO{
			PaneID:    s.PaneID,
			PaneTitle: s.PaneTitle,
			PaneType:  s.PaneType,
			Shell:     s.Shell,
			Cwd:       s.Cwd,
			UseDocker: s.UseDocker,
			Config:    s.Config,
			CLIType:   s.CLIType,
		})
	}

	return dtos, nil
}

// ClearTerminalSnapshots limpa snapshots após restauração bem-sucedida.
func (a *App) ClearTerminalSnapshots() error {
	if a.db == nil {
		return nil
	}
	log.Println("[ORCH] Clearing terminal snapshots")
	return a.db.ClearTerminalSnapshots()
}

// snapshotTerminalsOnShutdown captura o estado dos terminais ativos
// usando detecção de CLI via process sniffing. Chamado no Shutdown
// como fallback caso o frontend não tenha enviado snapshots.
func (a *App) snapshotTerminalsOnShutdown() {
	if a.ptyMgr == nil || a.db == nil {
		return
	}

	sessions := a.ptyMgr.GetSessions()
	if len(sessions) == 0 {
		log.Println("[ORCH] No active terminals to snapshot")
		return
	}

	// Verificar se já existem snapshots (frontend já enviou)
	existing, _ := a.db.GetTerminalSnapshots()
	if len(existing) > 0 {
		log.Printf("[ORCH] Snapshots already saved by frontend (%d), skipping backend fallback", len(existing))
		return
	}

	// Fallback: backend faz os snapshots baseado apenas nos dados do PTY
	snapshots := make([]database.TerminalSnapshot, 0, len(sessions))
	for _, sess := range sessions {
		if !sess.IsAlive {
			continue
		}

		cliType := terminal.CLINone
		cwd := sess.Cwd
		paneID := a.resolvePaneIDFromSessionID(sess.ID)
		if strings.TrimSpace(paneID) == "" {
			paneID = sess.ID // fallback legado
		}

		if pid, err := a.ptyMgr.GetProcessPID(sess.ID); err == nil {
			cliType = terminal.DetectCLI(pid)
			if realCwd := terminal.GetProcessCwd(pid); realCwd != "" {
				cwd = realCwd
			}
		}

		snapshots = append(snapshots, database.TerminalSnapshot{
			PaneID:  paneID,
			CLIType: string(cliType),
			Shell:   sess.Shell,
			Cwd:     cwd,
		})
	}

	if len(snapshots) > 0 {
		if err := a.db.SaveTerminalSnapshots(snapshots); err != nil {
			log.Printf("[ORCH] Error saving terminal snapshots (fallback): %v", err)
		} else {
			log.Printf("[ORCH] Saved %d terminal snapshots (backend fallback)", len(snapshots))
		}
	}
}

func (a *App) resolvePaneIDFromSessionID(sessionID string) string {
	if a.db == nil || strings.TrimSpace(sessionID) == "" {
		return ""
	}

	workspaces, err := a.db.GetWorkspacesWithAgents()
	if err != nil {
		return ""
	}

	for _, ws := range workspaces {
		for _, agent := range ws.Agents {
			if strings.TrimSpace(agent.SessionID) != sessionID {
				continue
			}
			return fmt.Sprintf("ws-%d-agent-%d", ws.ID, agent.ID)
		}
	}

	return ""
}
