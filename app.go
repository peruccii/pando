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
	"os/exec"
	"path/filepath"
	"regexp"
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
	gp "orch/internal/gitpanel"
	gpr "orch/internal/gitprs"
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
	gitPanelEventDebounceWindow       = 120 * time.Millisecond
	gitPanelAuthorCacheTTL            = 15 * time.Minute
	gitPanelAuthorMissTTL             = 5 * time.Minute
	gitPanelRepoIdentityCacheTTL      = 10 * time.Minute
	gitPanelAuthorLookupTimeout       = 4 * time.Second
	gitPanelAuthorLookupPerRequest    = 40
	gitPanelPRLocalBranchTimeout      = 12 * time.Second
)

var gitPanelCommitHashRegex = regexp.MustCompile(`^[a-f0-9]{7,40}$`)
var gitPanelFullCommitHashRegex = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)
var gitPanelLabelColorRegex = regexp.MustCompile(`^#?[a-fA-F0-9]{6}$`)

type gitPanelInvalidationPlan struct {
	Status    bool
	History   bool
	Conflicts bool
}

func (p gitPanelInvalidationPlan) isZero() bool {
	return !p.Status && !p.History && !p.Conflicts
}

type gitPanelPendingInvalidation struct {
	repoPath    string
	sourceEvent string
	reason      string
	status      bool
	history     bool
	conflicts   bool
	timer       *time.Timer
}

type gitPanelCommitAuthorCacheEntry struct {
	login     string
	avatarURL string
	found     bool
	expiresAt time.Time
}

type gitPanelRepoIdentityCacheEntry struct {
	owner     string
	repo      string
	expiresAt time.Time
}

// GitPanelPRRepositoryTargetDTO representa o repositorio remoto resolvido para operacoes de PR.
type GitPanelPRRepositoryTargetDTO struct {
	RepoPath          string `json:"repoPath"`
	RepoRoot          string `json:"repoRoot"`
	Owner             string `json:"owner"`
	Repo              string `json:"repo"`
	Source            string `json:"source"` // "origin" | "manual"
	OriginOwner       string `json:"originOwner,omitempty"`
	OriginRepo        string `json:"originRepo,omitempty"`
	ManualOwner       string `json:"manualOwner,omitempty"`
	ManualRepo        string `json:"manualRepo,omitempty"`
	OverrideConfirmed bool   `json:"overrideConfirmed,omitempty"`
}

// GitPanelPRCreatePayloadDTO representa o payload de criacao de PR via Git Panel.
type GitPanelPRCreatePayloadDTO struct {
	Title               string `json:"title"`
	Head                string `json:"head"`
	Base                string `json:"base"`
	Body                string `json:"body,omitempty"`
	Draft               bool   `json:"draft,omitempty"`
	MaintainerCanModify *bool  `json:"maintainerCanModify,omitempty"`
	ManualOwner         string `json:"manualOwner,omitempty"`
	ManualRepo          string `json:"manualRepo,omitempty"`
	AllowTargetOverride bool   `json:"allowTargetOverride,omitempty"`
}

// GitPanelPRCreateLabelPayloadDTO representa payload para criacao de label via aba de PR.
type GitPanelPRCreateLabelPayloadDTO struct {
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	Description *string `json:"description,omitempty"`
}

// GitPanelPRUpdatePayloadDTO representa o payload de atualização de PR via Git Panel.
type GitPanelPRUpdatePayloadDTO struct {
	Title               *string `json:"title,omitempty"`
	Body                *string `json:"body,omitempty"`
	State               *string `json:"state,omitempty"`
	Base                *string `json:"base,omitempty"`
	MaintainerCanModify *bool   `json:"maintainerCanModify,omitempty"`
}

// GitPanelPRMergePayloadDTO representa o payload de merge de PR via Git Panel.
type GitPanelPRMergePayloadDTO struct {
	MergeMethod string  `json:"mergeMethod,omitempty"` // "merge" | "squash" | "rebase"
	SHA         *string `json:"sha,omitempty"`
}

// GitPanelPRUpdateBranchPayloadDTO representa payload de update branch de PR via Git Panel.
type GitPanelPRUpdateBranchPayloadDTO struct {
	ExpectedHeadSHA *string `json:"expectedHeadSha,omitempty"`
}

// GitPanelPRMergeResultDTO representa resultado de merge de PR.
type GitPanelPRMergeResultDTO struct {
	SHA     string `json:"sha,omitempty"`
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
}

// GitPanelPRUpdateBranchResultDTO representa resultado de update branch de PR.
type GitPanelPRUpdateBranchResultDTO struct {
	Message string `json:"message"`
}

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
	gitPanel    *gp.Service
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

	gitIndexMu             sync.Mutex
	lastIndexFingerprints  map[string]string // repoPath -> fingerprint do estado staged
	gitPanelEventsMu       sync.Mutex
	gitPanelPendingEvents  map[string]*gitPanelPendingInvalidation
	gitPanelAuthorMu       sync.Mutex
	gitPanelAuthorCache    map[string]gitPanelCommitAuthorCacheEntry
	gitPanelAuthorInFlight map[string]struct{}
	gitPanelRepoIdentity   map[string]gitPanelRepoIdentityCacheEntry
	anonymousGuestID       string
	sessionGatewayOwner    bool
	sessionGatewayAddr     string
	sessionGatewayURL      string
	signalingAddr          string
	signalingURL           string
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		sessionContainers:      make(map[string]string),
		terminalHistory:        make(map[string]string),
		sessionAgents:          make(map[string]uint),
		lastIndexFingerprints:  make(map[string]string),
		gitPanelPendingEvents:  make(map[string]*gitPanelPendingInvalidation),
		gitPanelAuthorCache:    make(map[string]gitPanelCommitAuthorCacheEntry),
		gitPanelAuthorInFlight: make(map[string]struct{}),
		gitPanelRepoIdentity:   make(map[string]gitPanelRepoIdentityCacheEntry),
		anonymousGuestID:       fmt.Sprintf("anonymous-%d", time.Now().UnixNano()),
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
	a.github.SetTelemetryEmitter(func(eventName string, data interface{}) {
		if a.ctx == nil {
			return
		}
		if strings.TrimSpace(eventName) == "" {
			return
		}
		runtime.EventsEmit(a.ctx, eventName, data)
	})
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

	// 6.2 Inicializar serviço dedicado do Git Panel (read/write/eventos)
	a.gitPanel = gp.NewService(func(eventName string, data interface{}) {
		a.emitGitPanelRuntimeEvent(eventName, data)
	})
	log.Println("[ORCH] GitPanel service initialized")

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
		a.observeSessionTelemetry(eventName, data)
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
	a.bridgeLegacyGitEventToGitPanel(eventName, data)
	a.appendGitActivityFromRuntimeEvent(eventName, data)
}

func (a *App) emitGitPanelRuntimeEvent(eventName string, data interface{}) {
	runtime.EventsEmit(a.ctx, eventName, data)
}

func (a *App) bridgeLegacyGitEventToGitPanel(eventName string, data interface{}) {
	plan := mapLegacyGitEventToGitPanelInvalidation(eventName)
	if plan.isZero() {
		return
	}

	repoPath := extractRepoPathFromGitEventData(data)
	a.queueGitPanelInvalidation(repoPath, eventName, "filewatcher_bridge", plan)
}

func mapLegacyGitEventToGitPanelInvalidation(eventName string) gitPanelInvalidationPlan {
	switch strings.TrimSpace(eventName) {
	case "git:index":
		return gitPanelInvalidationPlan{Status: true}
	case "git:merge":
		return gitPanelInvalidationPlan{Status: true, Conflicts: true}
	case "git:branch_changed":
		return gitPanelInvalidationPlan{Status: true, History: true}
	case "git:commit":
		return gitPanelInvalidationPlan{Status: true, History: true}
	case "git:fetch":
		return gitPanelInvalidationPlan{Status: true}
	default:
		return gitPanelInvalidationPlan{}
	}
}

func (a *App) queueGitPanelInvalidation(repoPath string, sourceEvent string, reason string, plan gitPanelInvalidationPlan) {
	if plan.isZero() {
		return
	}

	key, normalizedRepoPath := normalizeGitPanelInvalidationRepo(repoPath)

	a.gitPanelEventsMu.Lock()
	pending, exists := a.gitPanelPendingEvents[key]
	if !exists {
		pending = &gitPanelPendingInvalidation{
			repoPath: normalizedRepoPath,
		}
		a.gitPanelPendingEvents[key] = pending
	}

	pending.repoPath = normalizedRepoPath
	pending.sourceEvent = strings.TrimSpace(sourceEvent)
	pending.reason = strings.TrimSpace(reason)
	pending.status = pending.status || plan.Status
	pending.history = pending.history || plan.History
	pending.conflicts = pending.conflicts || plan.Conflicts

	if pending.timer == nil {
		pending.timer = time.AfterFunc(gitPanelEventDebounceWindow, func() {
			a.flushGitPanelInvalidation(key)
		})
	}
	a.gitPanelEventsMu.Unlock()
}

func (a *App) flushGitPanelInvalidation(key string) {
	a.gitPanelEventsMu.Lock()
	pending, exists := a.gitPanelPendingEvents[key]
	if !exists {
		a.gitPanelEventsMu.Unlock()
		return
	}
	delete(a.gitPanelPendingEvents, key)
	if pending.timer != nil {
		pending.timer.Stop()
		pending.timer = nil
	}

	repoPath := pending.repoPath
	sourceEvent := pending.sourceEvent
	reason := pending.reason
	status := pending.status
	history := pending.history
	conflicts := pending.conflicts
	a.gitPanelEventsMu.Unlock()

	a.emitGitPanelInvalidationEvents(repoPath, sourceEvent, reason, status, history, conflicts)
}

func (a *App) emitGitPanelInvalidationEvents(repoPath string, sourceEvent string, reason string, status bool, history bool, conflicts bool) {
	if a.ctx == nil {
		return
	}
	if !status && !history && !conflicts {
		return
	}
	if a.gitPanel != nil {
		a.gitPanel.InvalidateRepoCache(repoPath)
	}

	basePayload := map[string]string{
		"repoPath":    strings.TrimSpace(repoPath),
		"sourceEvent": strings.TrimSpace(sourceEvent),
		"reason":      strings.TrimSpace(reason),
	}

	if status {
		runtime.EventsEmit(a.ctx, "gitpanel:status_changed", cloneGitPanelEventPayload(basePayload))
	}
	if history {
		runtime.EventsEmit(a.ctx, "gitpanel:history_invalidated", cloneGitPanelEventPayload(basePayload))
	}
	if conflicts {
		runtime.EventsEmit(a.ctx, "gitpanel:conflicts_changed", cloneGitPanelEventPayload(basePayload))
	}
}

func (a *App) stopGitPanelEventBridge() {
	a.gitPanelEventsMu.Lock()
	defer a.gitPanelEventsMu.Unlock()

	for key, pending := range a.gitPanelPendingEvents {
		if pending != nil && pending.timer != nil {
			pending.timer.Stop()
			pending.timer = nil
		}
		delete(a.gitPanelPendingEvents, key)
	}
}

func normalizeGitPanelInvalidationRepo(repoPath string) (string, string) {
	trimmed := strings.TrimSpace(repoPath)
	if trimmed == "" {
		return "__global__", ""
	}

	cleaned := filepath.Clean(trimmed)
	if cleaned == "." {
		return "__global__", ""
	}

	return cleaned, cleaned
}

func cloneGitPanelEventPayload(payload map[string]string) map[string]string {
	out := make(map[string]string, len(payload))
	for key, value := range payload {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func extractRepoPathFromGitEventData(data interface{}) string {
	if event, ok := toFileWatcherEvent(data); ok {
		return extractRepoPathFromGitEventPath(event.Path)
	}

	switch payload := data.(type) {
	case map[string]string:
		if repoPath := strings.TrimSpace(payload["repoPath"]); repoPath != "" {
			return filepath.Clean(repoPath)
		}
		if pathValue := strings.TrimSpace(payload["path"]); pathValue != "" {
			return extractRepoPathFromGitEventPath(pathValue)
		}
	case map[string]interface{}:
		if rawRepoPath, ok := payload["repoPath"].(string); ok && strings.TrimSpace(rawRepoPath) != "" {
			return filepath.Clean(strings.TrimSpace(rawRepoPath))
		}
		if rawPath, ok := payload["path"].(string); ok && strings.TrimSpace(rawPath) != "" {
			return extractRepoPathFromGitEventPath(rawPath)
		}
	}

	return ""
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
	a.stopGitPanelEventBridge()

	// Encerrar workers da fila de comandos Git Panel
	if a.gitPanel != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if err := a.gitPanel.Close(shutdownCtx); err != nil {
			log.Printf("[ORCH] Error closing GitPanel service: %v", err)
		}
		cancel()
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

func (a *App) requireGitHubSessionUser() (*auth.User, error) {
	if a.auth == nil {
		return nil, fmt.Errorf("serviço de autenticação não inicializado")
	}

	user, err := a.auth.GetCurrentUser()
	if err != nil || user == nil {
		return nil, fmt.Errorf("sessões colaborativas exigem autenticação com GitHub")
	}

	if strings.TrimSpace(user.ID) == "" {
		return nil, fmt.Errorf("sessões colaborativas exigem um usuário autenticado válido")
	}

	if strings.ToLower(strings.TrimSpace(user.Provider)) != "github" {
		return nil, fmt.Errorf("sessões colaborativas exigem autenticação com GitHub")
	}

	return user, nil
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

func (a *App) observeSessionTelemetry(eventName string, data interface{}) {
	switch eventName {
	case session.JoinSecurityEventInvalidAttempt, session.JoinSecurityEventBlocked:
		payload, ok := data.(session.JoinSecurityEvent)
		if !ok {
			return
		}
		a.handleJoinSecurityTelemetry(eventName, payload)
	}
}

func (a *App) handleJoinSecurityTelemetry(eventName string, payload session.JoinSecurityEvent) {
	log.Printf(
		"[SESSION][SECURITY] event=%s session=%s guest=%s reason=%s attempt=%d/%d retryAfter=%ds locked=%t",
		eventName,
		payload.SessionID,
		payload.GuestUserID,
		payload.Reason,
		payload.Attempt,
		payload.MaxAttempts,
		payload.RetryAfterSeconds,
		payload.Locked,
	)

	if payload.SessionID == "" {
		return
	}

	action := "guest_join_failed"
	if eventName == session.JoinSecurityEventBlocked || payload.Locked {
		action = "guest_join_blocked"
	}
	details := fmt.Sprintf(
		"reason=%s attempt=%d/%d retryAfter=%ds locked=%t",
		payload.Reason,
		payload.Attempt,
		payload.MaxAttempts,
		payload.RetryAfterSeconds,
		payload.Locked,
	)
	a.auditSessionEvent(payload.SessionID, payload.GuestUserID, action, details)
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

	workspaces, err := a.db.GetWorkspacesWithAgents()
	if err != nil {
		return nil, err
	}

	sanitized, legacyAgentIDs := filterLegacyGitHubAgents(workspaces)
	if len(legacyAgentIDs) == 0 {
		return sanitized, nil
	}

	log.Printf("[ORCH][MIGRATION] found %d legacy github agents; migrating to dedicated Git Panel flow", len(legacyAgentIDs))

	for _, legacyAgentID := range legacyAgentIDs {
		if deleteErr := a.DeleteAgentSession(legacyAgentID); deleteErr != nil {
			log.Printf("[ORCH][MIGRATION] failed to delete legacy github agent id=%d: %v", legacyAgentID, deleteErr)
		}
	}

	refreshed, refreshErr := a.db.GetWorkspacesWithAgents()
	if refreshErr != nil {
		log.Printf("[ORCH][MIGRATION] unable to refresh workspaces after legacy cleanup: %v", refreshErr)
		return sanitized, nil
	}

	refreshedSanitized, pendingLegacy := filterLegacyGitHubAgents(refreshed)
	if len(pendingLegacy) > 0 {
		log.Printf("[ORCH][MIGRATION] %d legacy github agents still present after cleanup", len(pendingLegacy))
	}

	return refreshedSanitized, nil
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

	normalizedType, err := normalizeAgentSessionType(agentType)
	if err != nil {
		return nil, err
	}

	existing, err := a.db.ListAgents(workspaceID)
	if err != nil {
		return nil, err
	}

	agent := &database.AgentSession{
		WorkspaceID: workspaceID,
		Name:        trimmedName,
		Type:        normalizedType,
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

func normalizeAgentSessionType(agentType string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(agentType))
	switch normalized {
	case "", "terminal":
		return "terminal", nil
	case "ai_agent":
		return "ai_agent", nil
	case "github":
		return "", fmt.Errorf("agent type 'github' foi descontinuado; abra o Git Panel dedicado")
	default:
		return "", fmt.Errorf("unsupported agent type: %s", agentType)
	}
}

func isLegacyGitHubAgentType(agentType string) bool {
	return strings.EqualFold(strings.TrimSpace(agentType), "github")
}

func filterLegacyGitHubAgents(workspaces []database.Workspace) ([]database.Workspace, []uint) {
	sanitized := make([]database.Workspace, 0, len(workspaces))
	legacyAgentIDs := make([]uint, 0)

	for _, ws := range workspaces {
		keptAgents := make([]database.AgentSession, 0, len(ws.Agents))
		for _, agent := range ws.Agents {
			if isLegacyGitHubAgentType(agent.Type) {
				legacyAgentIDs = append(legacyAgentIDs, agent.ID)
				continue
			}
			keptAgents = append(keptAgents, agent)
		}

		ws.Agents = keptAgents
		sanitized = append(sanitized, ws)
	}

	return sanitized, legacyAgentIDs
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

// === GitPanel Bindings (expostos ao Frontend) ===

func (a *App) requireGitPanelService() (*gp.Service, error) {
	if a.gitPanel == nil {
		return nil, gp.NewBindingError(
			gp.CodeServiceUnavailable,
			"Git Panel service indisponível.",
			"O backend do Git Panel ainda não foi inicializado.",
		)
	}
	return a.gitPanel, nil
}

func (a *App) normalizeGitPanelBindingError(err error) error {
	if err == nil {
		return nil
	}
	return gp.NormalizeBindingError(err)
}

// GitPanelPreflight valida runtime e contexto de repositório antes das operações.
func (a *App) GitPanelPreflight(repoPath string) (gp.PreflightResult, error) {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return gp.PreflightResult{}, err
	}

	result, preflightErr := svc.Preflight(repoPath)
	if preflightErr != nil {
		return gp.PreflightResult{}, a.normalizeGitPanelBindingError(preflightErr)
	}
	return result, nil
}

// GitPanelPRResolveRepository resolve owner/repo via origin e aceita fallback manual validado.
func (a *App) GitPanelPRResolveRepository(repoPath, manualOwner, manualRepo string, allowTargetOverride bool) (GitPanelPRRepositoryTargetDTO, error) {
	result := GitPanelPRRepositoryTargetDTO{}
	normalizedRepoPath := strings.TrimSpace(repoPath)
	if normalizedRepoPath == "" {
		return result, gpr.NewBindingError(
			gpr.CodeRepoPathRequired,
			"repoPath obrigatorio para resolver destino de Pull Request.",
			"Abra um repositorio Git valido antes de acessar a aba PRs.",
		)
	}

	svc, svcErr := a.requireGitPanelService()
	if svcErr != nil {
		return result, gpr.NewBindingError(
			gpr.CodeServiceUnavailable,
			"Git Panel service indisponivel para resolver Pull Requests.",
			"O backend do Git Panel ainda nao foi inicializado.",
		)
	}

	preflight, preflightErr := svc.Preflight(normalizedRepoPath)
	if preflightErr != nil {
		if bindingErr := gp.AsBindingError(preflightErr); bindingErr != nil {
			return result, gpr.NewBindingError(
				gpr.CodeRepoUnavailable,
				"Repositorio Git invalido para operacoes de Pull Request.",
				fmt.Sprintf("%s (%s)", strings.TrimSpace(bindingErr.Message), strings.TrimSpace(bindingErr.Code)),
			)
		}
		return result, gpr.NewBindingError(
			gpr.CodeRepoUnavailable,
			"Repositorio Git invalido para operacoes de Pull Request.",
			preflightErr.Error(),
		)
	}

	repoRoot := strings.TrimSpace(preflight.RepoRoot)
	if repoRoot == "" {
		repoRoot = strings.TrimSpace(preflight.RepoPath)
	}
	if repoRoot == "" {
		repoRoot = filepath.Clean(normalizedRepoPath)
	}

	result.RepoPath = normalizedRepoPath
	result.RepoRoot = repoRoot

	manualProvided := strings.TrimSpace(manualOwner) != "" || strings.TrimSpace(manualRepo) != ""
	var normalizedManualOwner string
	var normalizedManualRepo string
	if manualProvided {
		var normalizeErr error
		normalizedManualOwner, normalizedManualRepo, normalizeErr = gpr.NormalizeOwnerRepo(manualOwner, manualRepo)
		if normalizeErr != nil {
			return GitPanelPRRepositoryTargetDTO{}, gpr.NewBindingError(
				gpr.CodeManualRepoInvalid,
				"Fallback manual owner/repo invalido.",
				normalizeErr.Error(),
			)
		}
		result.ManualOwner = normalizedManualOwner
		result.ManualRepo = normalizedManualRepo
	}

	originOwner, originRepo, originResolved := a.resolveGitHubOwnerRepo(repoRoot)
	if originResolved {
		result.OriginOwner = originOwner
		result.OriginRepo = originRepo

		if manualProvided && !gpr.SameOwnerRepo(originOwner, originRepo, normalizedManualOwner, normalizedManualRepo) {
			if !allowTargetOverride {
				return GitPanelPRRepositoryTargetDTO{}, gpr.NewBindingError(
					gpr.CodeRepoTargetMismatch,
					"Repositorio manual diverge do origin detectado.",
					fmt.Sprintf("origin=%s/%s manual=%s/%s", originOwner, originRepo, normalizedManualOwner, normalizedManualRepo),
				)
			}

			result.Owner = normalizedManualOwner
			result.Repo = normalizedManualRepo
			result.Source = "manual"
			result.OverrideConfirmed = true
			return result, nil
		}

		result.Owner = originOwner
		result.Repo = originRepo
		result.Source = "origin"
		return result, nil
	}

	if manualProvided {
		result.Owner = normalizedManualOwner
		result.Repo = normalizedManualRepo
		result.Source = "manual"
		return result, nil
	}

	return GitPanelPRRepositoryTargetDTO{}, gpr.NewBindingError(
		gpr.CodeRepoResolveFailed,
		"Nao foi possivel resolver owner/repo via remote origin.",
		"Informe owner/repo manualmente para continuar.",
	)
}

func (a *App) requireGitHubServiceForPRs() (*gh.Service, error) {
	if a.github == nil {
		return nil, gpr.NewBindingError(
			gpr.CodeServiceUnavailable,
			"GitHub service indisponivel para operacoes de Pull Request.",
			"Conecte uma conta GitHub valida antes de usar a aba PRs.",
		)
	}
	return a.github, nil
}

func (a *App) normalizeGitPanelPRError(err error) error {
	if err == nil {
		return nil
	}

	if bindingErr := gpr.AsBindingError(err); bindingErr != nil {
		return bindingErr
	}

	var githubErr *gh.GitHubError
	if errors.As(err, &githubErr) {
		details := strings.TrimSpace(githubErr.Message)
		normalizedType := strings.TrimSpace(githubErr.Type)
		if normalizedType != "" {
			if details == "" {
				details = "type=" + normalizedType
			} else {
				details = fmt.Sprintf("%s (type=%s)", details, normalizedType)
			}
		}
		return gpr.NewHTTPBindingError(githubErr.StatusCode, details)
	}

	return gpr.NormalizeBindingError(err)
}

func (a *App) logGitPanelPROperationError(operation, owner, repo string, prNumber int, err error) {
	if a == nil || err == nil {
		return
	}

	sanitizedOperation := strings.TrimSpace(operation)
	if sanitizedOperation == "" {
		sanitizedOperation = "unknown"
	}
	sanitizedOwner := strings.TrimSpace(owner)
	if sanitizedOwner == "" {
		sanitizedOwner = "-"
	}
	sanitizedRepo := strings.TrimSpace(repo)
	if sanitizedRepo == "" {
		sanitizedRepo = "-"
	}

	bindingErr := gpr.NormalizeBindingError(err)
	code := strings.TrimSpace(bindingErr.Code)
	if code == "" {
		code = gpr.CodeUnknown
	}

	log.Printf(
		"[GitPanel][PR] operation=%s owner=%s repo=%s pr=%d code=%s message=%s details=%s",
		sanitizedOperation,
		sanitizedOwner,
		sanitizedRepo,
		prNumber,
		code,
		a.sanitizeForLogs(strings.TrimSpace(bindingErr.Message)),
		a.sanitizeForLogs(strings.TrimSpace(bindingErr.Details)),
	)
}

func (a *App) resolveGitPanelPROwnerRepo(repoPath string) (string, string, error) {
	target, err := a.GitPanelPRResolveRepository(repoPath, "", "", false)
	if err != nil {
		return "", "", a.normalizeGitPanelPRError(err)
	}

	owner, repo, normalizeErr := gpr.NormalizeOwnerRepo(target.Owner, target.Repo)
	if normalizeErr != nil {
		return "", "", gpr.NewBindingError(
			gpr.CodeRepoResolveFailed,
			"Nao foi possivel validar owner/repo para operacoes de Pull Request.",
			normalizeErr.Error(),
		)
	}

	return owner, repo, nil
}

func (a *App) resolveGitPanelPRRepoRoot(repoPath string) (string, error) {
	normalizedRepoPath := strings.TrimSpace(repoPath)
	if normalizedRepoPath == "" {
		return "", gpr.NewBindingError(
			gpr.CodeRepoPathRequired,
			"repoPath obrigatorio para operacoes de Pull Request.",
			"Abra um repositorio Git valido antes de executar operacoes locais.",
		)
	}

	svc, svcErr := a.requireGitPanelService()
	if svcErr != nil {
		return "", gpr.NewBindingError(
			gpr.CodeServiceUnavailable,
			"Git Panel service indisponivel para operacoes de Pull Request.",
			"O backend do Git Panel ainda nao foi inicializado.",
		)
	}

	preflight, preflightErr := svc.Preflight(normalizedRepoPath)
	if preflightErr != nil {
		if bindingErr := gp.AsBindingError(preflightErr); bindingErr != nil {
			return "", gpr.NewBindingError(
				gpr.CodeRepoUnavailable,
				"Repositorio Git invalido para operacoes de Pull Request.",
				fmt.Sprintf("%s (%s)", strings.TrimSpace(bindingErr.Message), strings.TrimSpace(bindingErr.Code)),
			)
		}
		return "", gpr.NewBindingError(
			gpr.CodeRepoUnavailable,
			"Repositorio Git invalido para operacoes de Pull Request.",
			preflightErr.Error(),
		)
	}

	repoRoot := strings.TrimSpace(preflight.RepoRoot)
	if repoRoot == "" {
		repoRoot = strings.TrimSpace(preflight.RepoPath)
	}
	if repoRoot == "" {
		repoRoot = filepath.Clean(normalizedRepoPath)
	}

	return repoRoot, nil
}

func normalizeGitPanelPRLocalRef(value string, fieldName string, required bool) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		if required {
			return "", gpr.NewBindingError(
				gpr.CodeValidationFailed,
				fmt.Sprintf(`Campo "%s" obrigatorio para criar branch local.`, fieldName),
				fmt.Sprintf(`Campo "%s" deve ser preenchido.`, fieldName),
			)
		}
		return "", nil
	}

	if strings.ContainsAny(normalized, " \t\r\n") {
		return "", gpr.NewBindingError(
			gpr.CodeValidationFailed,
			fmt.Sprintf(`Campo "%s" invalido para criar branch local.`, fieldName),
			fmt.Sprintf(`Campo "%s" nao pode conter espacos.`, fieldName),
		)
	}

	return normalized, nil
}

func runGitPanelPRCommand(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitPanelPRLocalBranchTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	rawOut, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(rawOut))
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		if output != "" {
			return output, context.DeadlineExceeded
		}
		return "", context.DeadlineExceeded
	}
	return output, err
}

func ensureGitPanelPRValidBranchName(repoRoot string, branch string) error {
	output, err := runGitPanelPRCommand(
		"-C", repoRoot,
		"check-ref-format",
		"--branch",
		branch,
	)
	if err == nil {
		return nil
	}

	details := strings.TrimSpace(output)
	if details == "" {
		details = err.Error()
	}
	return gpr.NewBindingError(
		gpr.CodeValidationFailed,
		"Nome de branch invalido para criacao local.",
		details,
	)
}

func resolveGitPanelPRLocalStartPoint(repoRoot string, base string) (string, error) {
	candidates := []string{base}
	if !strings.HasPrefix(base, "origin/") {
		candidates = append(candidates, "origin/"+base)
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		_, err := runGitPanelPRCommand(
			"-C", repoRoot,
			"rev-parse",
			"--verify",
			"--quiet",
			candidate,
		)
		if err == nil {
			return candidate, nil
		}
	}

	return "", gpr.NewBindingError(
		gpr.CodeValidationFailed,
		"Referencia base nao encontrada para criar branch local.",
		fmt.Sprintf(`Nao foi possivel resolver a referencia "%s" localmente nem em origin/%s.`, base, base),
	)
}

func normalizeGitPanelPRListState(state string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(state))
	switch normalized {
	case "", "open":
		return "open", nil
	case "closed":
		return "closed", nil
	case "all":
		return "all", nil
	default:
		return "", gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Filtro de estado de Pull Request invalido.",
			`Use "open", "closed" ou "all".`,
		)
	}
}

type normalizedGitPanelPRCreatePayload struct {
	Input               gh.CreatePRInput
	ManualOwner         string
	ManualRepo          string
	AllowTargetOverride bool
}

func normalizeGitPanelPRCreatePayload(payload GitPanelPRCreatePayloadDTO) (normalizedGitPanelPRCreatePayload, error) {
	normalizedTitle := strings.TrimSpace(payload.Title)
	if normalizedTitle == "" {
		return normalizedGitPanelPRCreatePayload{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Titulo da Pull Request obrigatorio.",
			`Campo "title" deve ser preenchido.`,
		)
	}

	normalizedHead := strings.TrimSpace(payload.Head)
	if normalizedHead == "" {
		return normalizedGitPanelPRCreatePayload{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Branch de origem obrigatoria para criar Pull Request.",
			`Campo "head" deve ser preenchido.`,
		)
	}

	normalizedBase := strings.TrimSpace(payload.Base)
	if normalizedBase == "" {
		return normalizedGitPanelPRCreatePayload{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Branch de destino obrigatoria para criar Pull Request.",
			`Campo "base" deve ser preenchido.`,
		)
	}

	input := gh.CreatePRInput{
		Title:      normalizedTitle,
		Body:       payload.Body,
		HeadBranch: normalizedHead,
		BaseBranch: normalizedBase,
		IsDraft:    payload.Draft,
	}
	if payload.MaintainerCanModify != nil {
		flag := *payload.MaintainerCanModify
		input.MaintainerCanModify = &flag
	}

	normalizedManualOwner := strings.TrimSpace(payload.ManualOwner)
	normalizedManualRepo := strings.TrimSpace(payload.ManualRepo)
	hasManualOwner := normalizedManualOwner != ""
	hasManualRepo := normalizedManualRepo != ""
	if hasManualOwner != hasManualRepo {
		return normalizedGitPanelPRCreatePayload{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Destino manual invalido para criar Pull Request.",
			`Informe "manualOwner" e "manualRepo" juntos quando usar override manual.`,
		)
	}

	if hasManualOwner {
		owner, repo, normalizeErr := gpr.NormalizeOwnerRepo(normalizedManualOwner, normalizedManualRepo)
		if normalizeErr != nil {
			return normalizedGitPanelPRCreatePayload{}, gpr.NewBindingError(
				gpr.CodeManualRepoInvalid,
				"Destino manual invalido para criar Pull Request.",
				normalizeErr.Error(),
			)
		}
		normalizedManualOwner = owner
		normalizedManualRepo = repo
	}

	return normalizedGitPanelPRCreatePayload{
		Input:               input,
		ManualOwner:         normalizedManualOwner,
		ManualRepo:          normalizedManualRepo,
		AllowTargetOverride: payload.AllowTargetOverride,
	}, nil
}

func normalizeGitPanelPRCreateLabelPayload(payload GitPanelPRCreateLabelPayloadDTO) (gh.CreateLabelInput, error) {
	normalizedName := strings.TrimSpace(payload.Name)
	if normalizedName == "" {
		return gh.CreateLabelInput{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Nome da etiqueta obrigatorio.",
			`Campo "name" deve ser preenchido.`,
		)
	}

	normalizedColor := strings.TrimSpace(payload.Color)
	if normalizedColor == "" {
		return gh.CreateLabelInput{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Cor da etiqueta obrigatoria.",
			`Campo "color" deve ser preenchido com 6 caracteres hexadecimais.`,
		)
	}
	if !gitPanelLabelColorRegex.MatchString(normalizedColor) {
		return gh.CreateLabelInput{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Cor da etiqueta invalida.",
			`Campo "color" deve usar formato hexadecimal de 6 caracteres (ex: #0e8a16).`,
		)
	}

	normalizedColor = strings.TrimPrefix(normalizedColor, "#")
	normalizedColor = strings.ToLower(normalizedColor)

	var normalizedDescription *string
	if payload.Description != nil {
		description := strings.TrimSpace(*payload.Description)
		if description != "" {
			normalizedDescription = &description
		}
	}

	return gh.CreateLabelInput{
		Name:        normalizedName,
		Color:       normalizedColor,
		Description: normalizedDescription,
	}, nil
}

func normalizeGitPanelPRUpdatePayload(payload GitPanelPRUpdatePayloadDTO) (gh.UpdatePRInput, error) {
	input := gh.UpdatePRInput{}
	hasField := false

	if payload.Title != nil {
		hasField = true
		normalizedTitle := strings.TrimSpace(*payload.Title)
		if normalizedTitle == "" {
			return gh.UpdatePRInput{}, gpr.NewBindingError(
				gpr.CodeValidationFailed,
				"Titulo invalido para atualizacao de Pull Request.",
				`Campo "title" nao pode ser vazio quando informado.`,
			)
		}
		input.Title = &normalizedTitle
	}

	if payload.Body != nil {
		hasField = true
		body := *payload.Body
		input.Body = &body
	}

	if payload.State != nil {
		hasField = true
		normalizedState := strings.ToLower(strings.TrimSpace(*payload.State))
		switch normalizedState {
		case "open", "closed":
			input.State = &normalizedState
		default:
			return gh.UpdatePRInput{}, gpr.NewBindingError(
				gpr.CodeValidationFailed,
				"Estado invalido para atualizacao de Pull Request.",
				`Campo "state" aceita apenas "open" ou "closed".`,
			)
		}
	}

	if payload.Base != nil {
		hasField = true
		normalizedBase := strings.TrimSpace(*payload.Base)
		if normalizedBase == "" {
			return gh.UpdatePRInput{}, gpr.NewBindingError(
				gpr.CodeValidationFailed,
				"Branch base invalida para atualizacao de Pull Request.",
				`Campo "base" nao pode ser vazio quando informado.`,
			)
		}
		input.BaseBranch = &normalizedBase
	}

	if payload.MaintainerCanModify != nil {
		hasField = true
		flag := *payload.MaintainerCanModify
		input.MaintainerCanModify = &flag
	}

	if !hasField {
		return gh.UpdatePRInput{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Payload de atualizacao de Pull Request vazio.",
			`Informe ao menos um campo: "title", "body", "state", "base" ou "maintainerCanModify".`,
		)
	}

	return input, nil
}

func normalizeGitPanelOptionalFullSHA(value *string, fieldName string) (*string, error) {
	if value == nil {
		return nil, nil
	}

	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil, nil
	}

	if !gitPanelFullCommitHashRegex.MatchString(normalized) {
		return nil, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			fmt.Sprintf(`Campo "%s" invalido para operacao de Pull Request.`, fieldName),
			fmt.Sprintf(`Campo "%s" deve conter um SHA completo de 40 caracteres hexadecimais.`, fieldName),
		)
	}

	normalized = strings.ToLower(normalized)
	return &normalized, nil
}

func normalizeGitPanelPRMergePayload(payload GitPanelPRMergePayloadDTO) (gh.MergePRInput, error) {
	normalizedMethod := strings.ToLower(strings.TrimSpace(payload.MergeMethod))
	switch normalizedMethod {
	case "", "merge":
		normalizedMethod = "merge"
	case "squash":
		normalizedMethod = "squash"
	case "rebase":
		normalizedMethod = "rebase"
	default:
		return gh.MergePRInput{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Metodo de merge invalido para Pull Request.",
			`Campo "mergeMethod" aceita apenas "merge", "squash" ou "rebase".`,
		)
	}

	normalizedSHA, shaErr := normalizeGitPanelOptionalFullSHA(payload.SHA, "sha")
	if shaErr != nil {
		return gh.MergePRInput{}, shaErr
	}

	return gh.MergePRInput{
		MergeMethod: gh.PRMergeMethod(normalizedMethod),
		SHA:         normalizedSHA,
	}, nil
}

func normalizeGitPanelPRUpdateBranchPayload(payload GitPanelPRUpdateBranchPayloadDTO) (gh.UpdatePRBranchInput, error) {
	normalizedSHA, shaErr := normalizeGitPanelOptionalFullSHA(payload.ExpectedHeadSHA, "expectedHeadSha")
	if shaErr != nil {
		return gh.UpdatePRBranchInput{}, shaErr
	}

	return gh.UpdatePRBranchInput{
		ExpectedHeadSHA: normalizedSHA,
	}, nil
}

func (a *App) emitGitPanelPRMutationRefresh(owner, repo string, prNumber int, action string) {
	if a == nil || a.ctx == nil {
		return
	}

	normalizedOwner := strings.TrimSpace(owner)
	normalizedRepo := strings.TrimSpace(repo)
	if normalizedOwner == "" || normalizedRepo == "" {
		return
	}

	normalizedAction := strings.ToLower(strings.TrimSpace(action))
	if normalizedAction == "" {
		normalizedAction = "updated"
	}

	runtime.EventsEmit(a.ctx, "github:prs:updated", map[string]interface{}{
		"owner":  normalizedOwner,
		"repo":   normalizedRepo,
		"count":  1,
		"source": "mutation",
		"action": normalizedAction,
		"changes": []map[string]interface{}{{
			"number":     prNumber,
			"changeType": normalizedAction,
		}},
	})
}

// GitPanelPRList retorna PRs do repositorio alvo com filtro e paginacao REST.
func (a *App) GitPanelPRList(repoPath string, state string, page int, perPage int) ([]gh.PullRequest, error) {
	normalizedState, stateErr := normalizeGitPanelPRListState(state)
	if stateErr != nil {
		return nil, stateErr
	}

	githubService, svcErr := a.requireGitHubServiceForPRs()
	if svcErr != nil {
		return nil, svcErr
	}

	owner, repo, resolveErr := a.resolveGitPanelPROwnerRepo(repoPath)
	if resolveErr != nil {
		return nil, resolveErr
	}

	items, err := githubService.ListPullRequests(owner, repo, gh.PRFilters{
		State:   normalizedState,
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		return nil, a.normalizeGitPanelPRError(err)
	}
	if items == nil {
		return []gh.PullRequest{}, nil
	}

	return items, nil
}

// GitPanelPRCreate cria um Pull Request no repositorio alvo via GitHub REST.
func (a *App) GitPanelPRCreate(repoPath string, payload GitPanelPRCreatePayloadDTO) (gh.PullRequest, error) {
	normalizedPayload, payloadErr := normalizeGitPanelPRCreatePayload(payload)
	if payloadErr != nil {
		return gh.PullRequest{}, payloadErr
	}

	githubService, svcErr := a.requireGitHubServiceForPRs()
	if svcErr != nil {
		return gh.PullRequest{}, svcErr
	}

	createInput := normalizedPayload.Input
	owner := ""
	repo := ""
	if normalizedPayload.ManualOwner != "" && normalizedPayload.ManualRepo != "" {
		target, targetErr := a.GitPanelPRResolveRepository(
			repoPath,
			normalizedPayload.ManualOwner,
			normalizedPayload.ManualRepo,
			normalizedPayload.AllowTargetOverride,
		)
		if targetErr != nil {
			return gh.PullRequest{}, a.normalizeGitPanelPRError(targetErr)
		}

		owner = strings.TrimSpace(target.Owner)
		repo = strings.TrimSpace(target.Repo)
	} else {
		var resolveErr error
		owner, repo, resolveErr = a.resolveGitPanelPROwnerRepo(repoPath)
		if resolveErr != nil {
			return gh.PullRequest{}, resolveErr
		}
	}

	if owner == "" || repo == "" {
		return gh.PullRequest{}, gpr.NewBindingError(
			gpr.CodeRepoResolveFailed,
			"Nao foi possivel resolver owner/repo para criar Pull Request.",
			"Revise o repositorio alvo e tente novamente.",
		)
	}

	createInput.Owner = owner
	createInput.Repo = repo

	created, err := githubService.CreatePullRequest(createInput)
	if err != nil {
		normalizedErr := a.normalizeGitPanelPRError(err)
		a.logGitPanelPROperationError("create", owner, repo, 0, normalizedErr)
		return gh.PullRequest{}, normalizedErr
	}
	if created == nil {
		return gh.PullRequest{}, nil
	}

	a.emitGitPanelPRMutationRefresh(owner, repo, created.Number, "created")
	return *created, nil
}

// GitPanelPRCreateLabel cria uma label de repositorio a partir da aba de PR.
func (a *App) GitPanelPRCreateLabel(repoPath string, payload GitPanelPRCreateLabelPayloadDTO) (gh.Label, error) {
	normalizedPayload, payloadErr := normalizeGitPanelPRCreateLabelPayload(payload)
	if payloadErr != nil {
		return gh.Label{}, payloadErr
	}

	githubService, svcErr := a.requireGitHubServiceForPRs()
	if svcErr != nil {
		return gh.Label{}, svcErr
	}

	owner, repo, resolveErr := a.resolveGitPanelPROwnerRepo(repoPath)
	if resolveErr != nil {
		return gh.Label{}, resolveErr
	}

	normalizedPayload.Owner = owner
	normalizedPayload.Repo = repo

	created, err := githubService.CreateLabel(normalizedPayload)
	if err != nil {
		normalizedErr := a.normalizeGitPanelPRError(err)
		a.logGitPanelPROperationError("create_label", owner, repo, 0, normalizedErr)
		return gh.Label{}, normalizedErr
	}
	if created == nil {
		return gh.Label{}, nil
	}

	return *created, nil
}

// GitPanelPRCreateLocalBranch cria e faz checkout de uma branch local sem usar API do GitHub.
func (a *App) GitPanelPRCreateLocalBranch(repoPath string, branch string, base string) error {
	repoRoot, repoErr := a.resolveGitPanelPRRepoRoot(repoPath)
	if repoErr != nil {
		return repoErr
	}

	normalizedBranch, branchErr := normalizeGitPanelPRLocalRef(branch, "branch", true)
	if branchErr != nil {
		return branchErr
	}

	normalizedBase, baseErr := normalizeGitPanelPRLocalRef(base, "base", false)
	if baseErr != nil {
		return baseErr
	}

	if validationErr := ensureGitPanelPRValidBranchName(repoRoot, normalizedBranch); validationErr != nil {
		return validationErr
	}

	checkoutBase := normalizedBase
	if checkoutBase != "" {
		resolvedBase, resolveErr := resolveGitPanelPRLocalStartPoint(repoRoot, checkoutBase)
		if resolveErr != nil {
			return resolveErr
		}
		checkoutBase = resolvedBase
	}

	checkoutArgs := []string{"-C", repoRoot, "checkout", "-b", normalizedBranch}
	if checkoutBase != "" {
		checkoutArgs = append(checkoutArgs, checkoutBase)
	}

	output, checkoutErr := runGitPanelPRCommand(checkoutArgs...)
	if checkoutErr == nil {
		return nil
	}

	details := strings.TrimSpace(output)
	loweredDetails := strings.ToLower(details)
	if strings.Contains(loweredDetails, "already exists") {
		switchOutput, switchErr := runGitPanelPRCommand("-C", repoRoot, "checkout", normalizedBranch)
		if switchErr == nil {
			return nil
		}
		switchDetails := strings.TrimSpace(switchOutput)
		switch {
		case details == "" && switchDetails != "":
			details = switchDetails
		case details != "" && switchDetails != "":
			details = details + " | " + switchDetails
		}
		checkoutErr = switchErr
	}

	if details == "" {
		details = checkoutErr.Error()
	}

	if errors.Is(checkoutErr, context.DeadlineExceeded) {
		return gpr.NewBindingError(
			gpr.CodeUnknown,
			"Tempo limite excedido ao criar branch local.",
			details,
		)
	}

	return gpr.NewBindingError(
		gpr.CodeConflict,
		"Falha ao criar branch local para a Pull Request.",
		details,
	)
}

// GitPanelPRPushLocalBranch publica uma branch local no remote origin com upstream.
func (a *App) GitPanelPRPushLocalBranch(repoPath string, branch string) error {
	repoRoot, repoErr := a.resolveGitPanelPRRepoRoot(repoPath)
	if repoErr != nil {
		return repoErr
	}

	normalizedBranch, branchErr := normalizeGitPanelPRLocalRef(branch, "branch", true)
	if branchErr != nil {
		return branchErr
	}

	if validationErr := ensureGitPanelPRValidBranchName(repoRoot, normalizedBranch); validationErr != nil {
		return validationErr
	}

	_, localLookupErr := runGitPanelPRCommand(
		"-C", repoRoot,
		"rev-parse",
		"--verify",
		"--quiet",
		"refs/heads/"+normalizedBranch,
	)
	if localLookupErr != nil {
		return gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Branch local nao encontrada para publicacao.",
			fmt.Sprintf(`Crie a branch "%s" localmente antes de publicar no origin.`, normalizedBranch),
		)
	}

	pushOutput, pushErr := runGitPanelPRCommand(
		"-C", repoRoot,
		"push",
		"-u",
		"origin",
		normalizedBranch,
	)
	if pushErr == nil {
		return nil
	}

	details := strings.TrimSpace(pushOutput)
	if details == "" {
		details = pushErr.Error()
	}

	if errors.Is(pushErr, context.DeadlineExceeded) {
		return gpr.NewBindingError(
			gpr.CodeUnknown,
			"Tempo limite excedido ao publicar branch local no origin.",
			details,
		)
	}

	return gpr.NewBindingError(
		gpr.CodeConflict,
		"Falha ao publicar branch local no origin.",
		details,
	)
}

// GitPanelPRUpdate atualiza um Pull Request existente no repositorio alvo via GitHub REST.
func (a *App) GitPanelPRUpdate(repoPath string, prNumber int, payload GitPanelPRUpdatePayloadDTO) (gh.PullRequest, error) {
	if prNumber <= 0 {
		return gh.PullRequest{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Numero de Pull Request invalido.",
			"Informe um numero de PR maior que zero.",
		)
	}

	updateInput, payloadErr := normalizeGitPanelPRUpdatePayload(payload)
	if payloadErr != nil {
		return gh.PullRequest{}, payloadErr
	}

	githubService, svcErr := a.requireGitHubServiceForPRs()
	if svcErr != nil {
		return gh.PullRequest{}, svcErr
	}

	owner, repo, resolveErr := a.resolveGitPanelPROwnerRepo(repoPath)
	if resolveErr != nil {
		return gh.PullRequest{}, resolveErr
	}

	updateInput.Owner = owner
	updateInput.Repo = repo
	updateInput.Number = prNumber

	updated, err := githubService.UpdatePullRequest(updateInput)
	if err != nil {
		normalizedErr := a.normalizeGitPanelPRError(err)
		a.logGitPanelPROperationError("update", owner, repo, prNumber, normalizedErr)
		return gh.PullRequest{}, normalizedErr
	}
	if updated == nil {
		return gh.PullRequest{}, nil
	}

	a.emitGitPanelPRMutationRefresh(owner, repo, updated.Number, "updated")
	return *updated, nil
}

// GitPanelPRCheckMerged verifica se a PR alvo ja foi mergeada.
func (a *App) GitPanelPRCheckMerged(repoPath string, prNumber int) (bool, error) {
	if prNumber <= 0 {
		return false, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Numero de Pull Request invalido.",
			"Informe um numero de PR maior que zero.",
		)
	}

	githubService, svcErr := a.requireGitHubServiceForPRs()
	if svcErr != nil {
		return false, svcErr
	}

	owner, repo, resolveErr := a.resolveGitPanelPROwnerRepo(repoPath)
	if resolveErr != nil {
		return false, resolveErr
	}

	merged, err := githubService.CheckPullRequestMerged(owner, repo, prNumber)
	if err != nil {
		return false, a.normalizeGitPanelPRError(err)
	}

	return merged, nil
}

// GitPanelPRMerge executa merge de PR no repositorio alvo via GitHub REST.
func (a *App) GitPanelPRMerge(repoPath string, prNumber int, payload GitPanelPRMergePayloadDTO) (GitPanelPRMergeResultDTO, error) {
	if prNumber <= 0 {
		return GitPanelPRMergeResultDTO{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Numero de Pull Request invalido.",
			"Informe um numero de PR maior que zero.",
		)
	}

	mergeInput, payloadErr := normalizeGitPanelPRMergePayload(payload)
	if payloadErr != nil {
		return GitPanelPRMergeResultDTO{}, payloadErr
	}

	githubService, svcErr := a.requireGitHubServiceForPRs()
	if svcErr != nil {
		return GitPanelPRMergeResultDTO{}, svcErr
	}

	owner, repo, resolveErr := a.resolveGitPanelPROwnerRepo(repoPath)
	if resolveErr != nil {
		return GitPanelPRMergeResultDTO{}, resolveErr
	}

	mergeInput.Owner = owner
	mergeInput.Repo = repo
	mergeInput.Number = prNumber

	result, err := githubService.MergePullRequestREST(mergeInput)
	if err != nil {
		normalizedErr := a.normalizeGitPanelPRError(err)
		a.logGitPanelPROperationError("merge", owner, repo, prNumber, normalizedErr)
		return GitPanelPRMergeResultDTO{}, normalizedErr
	}
	if result == nil {
		return GitPanelPRMergeResultDTO{}, nil
	}

	return GitPanelPRMergeResultDTO{
		SHA:     strings.TrimSpace(result.SHA),
		Merged:  result.Merged,
		Message: strings.TrimSpace(result.Message),
	}, nil
}

// GitPanelPRUpdateBranch solicita update da branch da PR alvo via GitHub REST.
func (a *App) GitPanelPRUpdateBranch(repoPath string, prNumber int, payload GitPanelPRUpdateBranchPayloadDTO) (GitPanelPRUpdateBranchResultDTO, error) {
	if prNumber <= 0 {
		return GitPanelPRUpdateBranchResultDTO{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Numero de Pull Request invalido.",
			"Informe um numero de PR maior que zero.",
		)
	}

	updateInput, payloadErr := normalizeGitPanelPRUpdateBranchPayload(payload)
	if payloadErr != nil {
		return GitPanelPRUpdateBranchResultDTO{}, payloadErr
	}

	githubService, svcErr := a.requireGitHubServiceForPRs()
	if svcErr != nil {
		return GitPanelPRUpdateBranchResultDTO{}, svcErr
	}

	owner, repo, resolveErr := a.resolveGitPanelPROwnerRepo(repoPath)
	if resolveErr != nil {
		return GitPanelPRUpdateBranchResultDTO{}, resolveErr
	}

	updateInput.Owner = owner
	updateInput.Repo = repo
	updateInput.Number = prNumber

	result, err := githubService.UpdatePullRequestBranch(updateInput)
	if err != nil {
		normalizedErr := a.normalizeGitPanelPRError(err)
		a.logGitPanelPROperationError("update_branch", owner, repo, prNumber, normalizedErr)
		return GitPanelPRUpdateBranchResultDTO{}, normalizedErr
	}
	if result == nil {
		return GitPanelPRUpdateBranchResultDTO{}, nil
	}

	return GitPanelPRUpdateBranchResultDTO{
		Message: strings.TrimSpace(result.Message),
	}, nil
}

// GitPanelPRGet retorna detalhes do Pull Request alvo.
func (a *App) GitPanelPRGet(repoPath string, prNumber int) (gh.PullRequest, error) {
	if prNumber <= 0 {
		return gh.PullRequest{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Numero de Pull Request invalido.",
			"Informe um numero de PR maior que zero.",
		)
	}

	githubService, svcErr := a.requireGitHubServiceForPRs()
	if svcErr != nil {
		return gh.PullRequest{}, svcErr
	}

	owner, repo, resolveErr := a.resolveGitPanelPROwnerRepo(repoPath)
	if resolveErr != nil {
		return gh.PullRequest{}, resolveErr
	}

	pr, err := githubService.GetPullRequest(owner, repo, prNumber)
	if err != nil {
		return gh.PullRequest{}, a.normalizeGitPanelPRError(err)
	}
	if pr == nil {
		return gh.PullRequest{}, nil
	}

	return *pr, nil
}

// GitPanelPRGetCommits retorna commits paginados da PR alvo.
func (a *App) GitPanelPRGetCommits(repoPath string, prNumber int, page int, perPage int) (gh.PRCommitPage, error) {
	if prNumber <= 0 {
		return gh.PRCommitPage{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Numero de Pull Request invalido.",
			"Informe um numero de PR maior que zero.",
		)
	}

	githubService, svcErr := a.requireGitHubServiceForPRs()
	if svcErr != nil {
		return gh.PRCommitPage{}, svcErr
	}

	owner, repo, resolveErr := a.resolveGitPanelPROwnerRepo(repoPath)
	if resolveErr != nil {
		return gh.PRCommitPage{}, resolveErr
	}

	pageResult, err := githubService.GetPullRequestCommits(owner, repo, prNumber, page, perPage)
	if err != nil {
		return gh.PRCommitPage{}, a.normalizeGitPanelPRError(err)
	}
	if pageResult == nil {
		return gh.PRCommitPage{}, nil
	}

	return *pageResult, nil
}

// GitPanelPRGetFiles retorna arquivos paginados da PR alvo.
func (a *App) GitPanelPRGetFiles(repoPath string, prNumber int, page int, perPage int) (gh.PRFilePage, error) {
	if prNumber <= 0 {
		return gh.PRFilePage{}, gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Numero de Pull Request invalido.",
			"Informe um numero de PR maior que zero.",
		)
	}

	githubService, svcErr := a.requireGitHubServiceForPRs()
	if svcErr != nil {
		return gh.PRFilePage{}, svcErr
	}

	owner, repo, resolveErr := a.resolveGitPanelPROwnerRepo(repoPath)
	if resolveErr != nil {
		return gh.PRFilePage{}, resolveErr
	}

	pageResult, err := githubService.GetPullRequestFiles(owner, repo, prNumber, page, perPage)
	if err != nil {
		return gh.PRFilePage{}, a.normalizeGitPanelPRError(err)
	}
	if pageResult == nil {
		return gh.PRFilePage{}, nil
	}

	return *pageResult, nil
}

// GitPanelPRGetRawDiff retorna diff completo bruto da PR alvo sob demanda.
func (a *App) GitPanelPRGetRawDiff(repoPath string, prNumber int) (string, error) {
	if prNumber <= 0 {
		return "", gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Numero de Pull Request invalido.",
			"Informe um numero de PR maior que zero.",
		)
	}

	githubService, svcErr := a.requireGitHubServiceForPRs()
	if svcErr != nil {
		return "", svcErr
	}

	owner, repo, resolveErr := a.resolveGitPanelPROwnerRepo(repoPath)
	if resolveErr != nil {
		return "", resolveErr
	}

	rawDiff, err := githubService.GetPullRequestRawDiff(owner, repo, prNumber)
	if err != nil {
		return "", a.normalizeGitPanelPRError(err)
	}

	return rawDiff, nil
}

// GitPanelPRGetCommitRawDiff retorna diff bruto de um commit especifico da PR alvo.
func (a *App) GitPanelPRGetCommitRawDiff(repoPath string, prNumber int, commitSHA string) (string, error) {
	if prNumber <= 0 {
		return "", gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Numero de Pull Request invalido.",
			"Informe um numero de PR maior que zero.",
		)
	}

	normalizedSHA := strings.TrimSpace(commitSHA)
	if normalizedSHA == "" {
		return "", gpr.NewBindingError(
			gpr.CodeValidationFailed,
			"Commit SHA invalido.",
			"Informe um commit SHA valido para carregar o diff.",
		)
	}

	githubService, svcErr := a.requireGitHubServiceForPRs()
	if svcErr != nil {
		return "", svcErr
	}

	owner, repo, resolveErr := a.resolveGitPanelPROwnerRepo(repoPath)
	if resolveErr != nil {
		return "", resolveErr
	}

	rawDiff, err := githubService.GetCommitRawDiff(owner, repo, normalizedSHA)
	if err != nil {
		return "", a.normalizeGitPanelPRError(err)
	}

	return rawDiff, nil
}

// GitPanelPickRepositoryDirectory abre o seletor nativo para escolher diretório do repositório.
func (a *App) GitPanelPickRepositoryDirectory(defaultPath string) (string, error) {
	if a.ctx == nil {
		return "", gp.NewBindingError(
			gp.CodeServiceUnavailable,
			"Janela indisponível para seleção de diretório.",
			"Contexto do runtime ainda não foi inicializado.",
		)
	}

	defaultDirectory := resolveExistingDirectory(defaultPath)
	selectedViaAppleScript, appleScriptErr := pickDirectoryWithAppleScript("Selecionar repositório Git", defaultDirectory)
	if appleScriptErr == nil {
		return strings.TrimSpace(selectedViaAppleScript), nil
	}

	options := runtime.OpenDialogOptions{
		Title:                "Selecionar repositório Git",
		ShowHiddenFiles:      true,
		CanCreateDirectories: true,
	}
	if defaultDirectory != "" {
		options.DefaultDirectory = defaultDirectory
	}

	selectedPath, err := runtime.OpenDirectoryDialog(a.ctx, options)
	if err != nil {
		details := strings.TrimSpace(err.Error())
		if appleScriptErr != nil && !errors.Is(appleScriptErr, exec.ErrNotFound) {
			details = fmt.Sprintf("apple script: %s | runtime: %s", strings.TrimSpace(appleScriptErr.Error()), details)
		}
		return "", gp.NewBindingError(
			gp.CodeCommandFailed,
			"Falha ao abrir seletor de diretório.",
			details,
		)
	}

	return strings.TrimSpace(selectedPath), nil
}

func resolveExistingDirectory(rawPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return ""
	}

	cleaned := filepath.Clean(trimmed)
	info, err := os.Stat(cleaned)
	if err == nil {
		if info.IsDir() {
			return cleaned
		}
		parent := filepath.Dir(cleaned)
		if parentInfo, parentErr := os.Stat(parent); parentErr == nil && parentInfo.IsDir() {
			return parent
		}
		return ""
	}

	parent := filepath.Dir(cleaned)
	if parent == "" || parent == "." {
		return ""
	}
	if parentInfo, parentErr := os.Stat(parent); parentErr == nil && parentInfo.IsDir() {
		return parent
	}
	return ""
}

func pickDirectoryWithAppleScript(title string, defaultDirectory string) (string, error) {
	if _, err := exec.LookPath("osascript"); err != nil {
		return "", err
	}

	command := "set selectedFolder to choose folder"
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle != "" {
		command += " with prompt " + quoteAppleScriptString(trimmedTitle)
	}

	trimmedDefault := strings.TrimSpace(defaultDirectory)
	if trimmedDefault != "" {
		command += " default location POSIX file " + quoteAppleScriptString(trimmedDefault)
	}

	output, err := exec.Command(
		"osascript",
		"-e", command,
		"-e", "POSIX path of selectedFolder",
	).CombinedOutput()
	if err != nil {
		details := strings.TrimSpace(string(output))
		lowerDetails := strings.ToLower(details)
		if strings.Contains(lowerDetails, "user canceled") || strings.Contains(lowerDetails, "user cancelled") {
			return "", nil
		}
		if details == "" {
			details = strings.TrimSpace(err.Error())
		}
		return "", errors.New(details)
	}

	return strings.TrimSpace(string(output)), nil
}

func quoteAppleScriptString(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return "\"" + escaped + "\""
}

// GitPanelGetStatus retorna snapshot de status staged/unstaged/conflicted.
func (a *App) GitPanelGetStatus(repoPath string) (gp.StatusDTO, error) {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return gp.StatusDTO{}, err
	}

	result, statusErr := svc.GetStatus(repoPath)
	if statusErr != nil {
		return gp.StatusDTO{}, a.normalizeGitPanelBindingError(statusErr)
	}
	return result, nil
}

// GitPanelGetHistory retorna página de histórico linear.
func (a *App) GitPanelGetHistory(repoPath string, cursor string, limit int, query string) (gp.HistoryPageDTO, error) {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return gp.HistoryPageDTO{}, err
	}

	result, historyErr := svc.GetHistory(repoPath, cursor, limit, query)
	if historyErr != nil {
		return gp.HistoryPageDTO{}, a.normalizeGitPanelBindingError(historyErr)
	}

	repoRoot := strings.TrimSpace(repoPath)
	if preflight, preflightErr := svc.Preflight(repoPath); preflightErr == nil {
		if normalized := strings.TrimSpace(preflight.RepoRoot); normalized != "" {
			repoRoot = normalized
		}
	}

	a.enrichGitPanelHistoryWithAuthIdentity(repoRoot, &result)
	return result, nil
}

func (a *App) enrichGitPanelHistoryWithAuthIdentity(repoRoot string, page *gp.HistoryPageDTO) {
	if page == nil || len(page.Items) == 0 {
		return
	}

	a.applyLocalAuthIdentityToHistory(page)

	pendingHashes := a.applyCachedGitHubAuthorsToHistory(repoRoot, page)
	if len(pendingHashes) == 0 {
		return
	}

	a.queueGitHubAuthorEnrichment(repoRoot, pendingHashes)
}

func (a *App) applyLocalAuthIdentityToHistory(page *gp.HistoryPageDTO) {
	if page == nil || len(page.Items) == 0 || a.auth == nil {
		return
	}

	user, err := a.auth.GetCurrentUser()
	if err != nil || user == nil {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(user.Provider), "github") {
		return
	}

	login := strings.TrimSpace(user.Username)
	if login == "" {
		login = strings.TrimSpace(user.Name)
	}
	avatarURL := strings.TrimSpace(user.AvatarURL)
	if login == "" && avatarURL == "" {
		return
	}

	emailToken := normalizeGitPanelIdentityToken(user.Email)
	nameToken := normalizeGitPanelIdentityToken(user.Name)
	usernameToken := normalizeGitPanelIdentityToken(user.Username)

	for index := range page.Items {
		item := &page.Items[index]
		if !historyItemMatchesAuthUser(item, emailToken, nameToken, usernameToken) {
			continue
		}
		item.GitHubLogin = login
		item.GitHubAvatarURL = avatarURL
	}
}

func (a *App) applyCachedGitHubAuthorsToHistory(repoRoot string, page *gp.HistoryPageDTO) []string {
	if page == nil || len(page.Items) == 0 {
		return nil
	}

	normalizedRoot := filepath.Clean(strings.TrimSpace(repoRoot))
	if normalizedRoot == "" {
		return nil
	}

	now := time.Now()
	pending := make([]string, 0, gitPanelAuthorLookupPerRequest)

	a.gitPanelAuthorMu.Lock()
	defer a.gitPanelAuthorMu.Unlock()

	for index := range page.Items {
		item := &page.Items[index]
		hash := normalizeGitPanelCommitHash(item.Hash)
		if hash == "" {
			continue
		}
		if strings.TrimSpace(item.GitHubLogin) != "" && strings.TrimSpace(item.GitHubAvatarURL) != "" {
			continue
		}

		cacheKey := buildGitPanelCommitAuthorCacheKey(normalizedRoot, hash)
		if cached, ok := a.gitPanelAuthorCache[cacheKey]; ok {
			if now.After(cached.expiresAt) {
				delete(a.gitPanelAuthorCache, cacheKey)
			} else {
				if cached.found {
					if strings.TrimSpace(item.GitHubLogin) == "" {
						item.GitHubLogin = cached.login
					}
					if strings.TrimSpace(item.GitHubAvatarURL) == "" {
						item.GitHubAvatarURL = cached.avatarURL
					}
				}
				continue
			}
		}

		if len(pending) >= gitPanelAuthorLookupPerRequest {
			continue
		}
		if _, inFlight := a.gitPanelAuthorInFlight[cacheKey]; inFlight {
			continue
		}

		a.gitPanelAuthorInFlight[cacheKey] = struct{}{}
		pending = append(pending, hash)
	}

	return pending
}

func (a *App) queueGitHubAuthorEnrichment(repoRoot string, hashes []string) {
	normalizedRoot := filepath.Clean(strings.TrimSpace(repoRoot))
	if normalizedRoot == "" || len(hashes) == 0 {
		a.releaseGitPanelAuthorInFlight(normalizedRoot, hashes)
		return
	}
	if a.github == nil || a.auth == nil {
		a.releaseGitPanelAuthorInFlight(normalizedRoot, hashes)
		return
	}

	authState := a.auth.GetAuthState()
	if authState == nil || !authState.IsAuthenticated || !authState.HasGitHubToken {
		a.releaseGitPanelAuthorInFlight(normalizedRoot, hashes)
		return
	}

	go a.resolveGitHubAuthorsForHistory(normalizedRoot, hashes)
}

func (a *App) resolveGitHubAuthorsForHistory(repoRoot string, hashes []string) {
	defer a.releaseGitPanelAuthorInFlight(repoRoot, hashes)

	owner, repo, ok := a.resolveGitHubOwnerRepo(repoRoot)
	if !ok {
		a.cacheGitPanelAuthorMisses(repoRoot, hashes)
		return
	}

	resolvedAuthors, err := a.github.ResolveCommitAuthors(owner, repo, hashes)
	if err != nil {
		log.Printf("[ORCH][GitPanel] resolve commit authors failed repo=%s remote=%s/%s err=%v", repoRoot, owner, repo, err)
		return
	}

	a.applyResolvedGitHubAuthors(repoRoot, hashes, resolvedAuthors)
}

func (a *App) applyResolvedGitHubAuthors(repoRoot string, requestedHashes []string, resolved map[string]gh.User) {
	normalizedRoot := filepath.Clean(strings.TrimSpace(repoRoot))
	if normalizedRoot == "" || len(requestedHashes) == 0 {
		return
	}

	now := time.Now()
	updates := make([]map[string]string, 0, len(requestedHashes))

	a.gitPanelAuthorMu.Lock()
	for _, rawHash := range requestedHashes {
		hash := normalizeGitPanelCommitHash(rawHash)
		if hash == "" {
			continue
		}

		cacheKey := buildGitPanelCommitAuthorCacheKey(normalizedRoot, hash)
		user, ok := resolved[hash]
		if !ok {
			a.gitPanelAuthorCache[cacheKey] = gitPanelCommitAuthorCacheEntry{
				found:     false,
				expiresAt: now.Add(gitPanelAuthorMissTTL),
			}
			continue
		}

		login := strings.TrimSpace(user.Login)
		if login == "" {
			a.gitPanelAuthorCache[cacheKey] = gitPanelCommitAuthorCacheEntry{
				found:     false,
				expiresAt: now.Add(gitPanelAuthorMissTTL),
			}
			continue
		}

		avatarURL := strings.TrimSpace(user.AvatarURL)
		a.gitPanelAuthorCache[cacheKey] = gitPanelCommitAuthorCacheEntry{
			login:     login,
			avatarURL: avatarURL,
			found:     true,
			expiresAt: now.Add(gitPanelAuthorCacheTTL),
		}

		updates = append(updates, map[string]string{
			"hash":            hash,
			"githubLogin":     login,
			"githubAvatarUrl": avatarURL,
		})
	}
	a.gitPanelAuthorMu.Unlock()

	if len(updates) == 0 || a.ctx == nil {
		return
	}

	runtime.EventsEmit(a.ctx, "gitpanel:history_authors_enriched", map[string]interface{}{
		"repoPath": normalizedRoot,
		"items":    updates,
	})
}

func (a *App) cacheGitPanelAuthorMisses(repoRoot string, hashes []string) {
	normalizedRoot := filepath.Clean(strings.TrimSpace(repoRoot))
	if normalizedRoot == "" || len(hashes) == 0 {
		return
	}

	now := time.Now()
	a.gitPanelAuthorMu.Lock()
	defer a.gitPanelAuthorMu.Unlock()

	for _, rawHash := range hashes {
		hash := normalizeGitPanelCommitHash(rawHash)
		if hash == "" {
			continue
		}
		cacheKey := buildGitPanelCommitAuthorCacheKey(normalizedRoot, hash)
		a.gitPanelAuthorCache[cacheKey] = gitPanelCommitAuthorCacheEntry{
			found:     false,
			expiresAt: now.Add(gitPanelAuthorMissTTL),
		}
	}
}

func (a *App) releaseGitPanelAuthorInFlight(repoRoot string, hashes []string) {
	normalizedRoot := filepath.Clean(strings.TrimSpace(repoRoot))
	if normalizedRoot == "" || len(hashes) == 0 {
		return
	}

	a.gitPanelAuthorMu.Lock()
	defer a.gitPanelAuthorMu.Unlock()

	for _, rawHash := range hashes {
		hash := normalizeGitPanelCommitHash(rawHash)
		if hash == "" {
			continue
		}
		cacheKey := buildGitPanelCommitAuthorCacheKey(normalizedRoot, hash)
		delete(a.gitPanelAuthorInFlight, cacheKey)
	}
}

func (a *App) resolveGitHubOwnerRepo(repoRoot string) (string, string, bool) {
	normalizedRoot := filepath.Clean(strings.TrimSpace(repoRoot))
	if normalizedRoot == "" {
		return "", "", false
	}

	now := time.Now()

	a.gitPanelAuthorMu.Lock()
	if cached, ok := a.gitPanelRepoIdentity[normalizedRoot]; ok && now.Before(cached.expiresAt) {
		a.gitPanelAuthorMu.Unlock()
		if cached.owner != "" && cached.repo != "" {
			return cached.owner, cached.repo, true
		}
		return "", "", false
	}
	a.gitPanelAuthorMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), gitPanelAuthorLookupTimeout)
	defer cancel()

	originOutput, err := exec.CommandContext(ctx, "git", "-C", normalizedRoot, "remote", "get-url", "origin").Output()
	if err != nil {
		a.gitPanelAuthorMu.Lock()
		a.gitPanelRepoIdentity[normalizedRoot] = gitPanelRepoIdentityCacheEntry{
			expiresAt: now.Add(gitPanelAuthorMissTTL),
		}
		a.gitPanelAuthorMu.Unlock()
		return "", "", false
	}

	owner, repo, ok := gpr.ParseGitHubRemoteURL(string(originOutput))
	a.gitPanelAuthorMu.Lock()
	if ok {
		a.gitPanelRepoIdentity[normalizedRoot] = gitPanelRepoIdentityCacheEntry{
			owner:     owner,
			repo:      repo,
			expiresAt: now.Add(gitPanelRepoIdentityCacheTTL),
		}
	} else {
		a.gitPanelRepoIdentity[normalizedRoot] = gitPanelRepoIdentityCacheEntry{
			expiresAt: now.Add(gitPanelAuthorMissTTL),
		}
	}
	a.gitPanelAuthorMu.Unlock()

	return owner, repo, ok
}

func normalizeGitPanelCommitHash(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || !gitPanelCommitHashRegex.MatchString(normalized) {
		return ""
	}
	return normalized
}

func buildGitPanelCommitAuthorCacheKey(repoRoot string, hash string) string {
	root := filepath.Clean(strings.TrimSpace(repoRoot))
	normalizedHash := normalizeGitPanelCommitHash(hash)
	if root == "" || normalizedHash == "" {
		return ""
	}
	return root + "|" + normalizedHash
}

func historyItemMatchesAuthUser(item *gp.HistoryItemDTO, emailToken string, nameToken string, usernameToken string) bool {
	if item == nil {
		return false
	}

	authorToken := normalizeGitPanelIdentityToken(item.Author)
	authorEmailToken := normalizeGitPanelIdentityToken(item.AuthorEmail)

	if emailToken != "" && authorEmailToken != "" && emailToken == authorEmailToken {
		return true
	}
	if nameToken != "" && authorToken != "" && nameToken == authorToken {
		return true
	}
	if usernameToken != "" && authorToken != "" && usernameToken == authorToken {
		return true
	}

	return false
}

func normalizeGitPanelIdentityToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// GitPanelGetDiff retorna diff textual base para o frontend.
func (a *App) GitPanelGetDiff(repoPath string, filePath string, mode string, contextLines int) (gp.DiffDTO, error) {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return gp.DiffDTO{}, err
	}

	result, diffErr := svc.GetDiff(repoPath, filePath, mode, contextLines)
	if diffErr != nil {
		return gp.DiffDTO{}, a.normalizeGitPanelBindingError(diffErr)
	}
	return result, nil
}

// GitPanelGetCommitDetails retorna detalhes de um commit específico (ex: lista de arquivos).
func (a *App) GitPanelGetCommitDetails(repoPath string, commitHash string) (gp.CommitDetailsDTO, error) {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return gp.CommitDetailsDTO{}, err
	}
	result, err := svc.GetCommitDetails(repoPath, commitHash)
	if err != nil {
		return gp.CommitDetailsDTO{}, a.normalizeGitPanelBindingError(err)
	}
	return result, nil
}

// GitPanelGetCommitDiff retorna diff textual de um arquivo em um commit específico.
func (a *App) GitPanelGetCommitDiff(repoPath string, filePath string, commitHash string, contextLines int) (gp.DiffDTO, error) {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return gp.DiffDTO{}, err
	}
	result, err := svc.GetCommitDiff(repoPath, filePath, commitHash, contextLines)
	if err != nil {
		return gp.DiffDTO{}, a.normalizeGitPanelBindingError(err)
	}
	return result, nil
}

// GitPanelGetConflicts retorna arquivos em estado de conflito.
func (a *App) GitPanelGetConflicts(repoPath string) ([]gp.ConflictFileDTO, error) {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return nil, err
	}

	result, conflictsErr := svc.GetConflicts(repoPath)
	if conflictsErr != nil {
		return nil, a.normalizeGitPanelBindingError(conflictsErr)
	}
	return result, nil
}

// GitPanelStageFile adiciona arquivo ao stage.
func (a *App) GitPanelStageFile(repoPath string, filePath string) error {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return err
	}
	return a.normalizeGitPanelBindingError(svc.StageFile(repoPath, filePath))
}

// GitPanelUnstageFile remove arquivo do stage.
func (a *App) GitPanelUnstageFile(repoPath string, filePath string) error {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return err
	}
	return a.normalizeGitPanelBindingError(svc.UnstageFile(repoPath, filePath))
}

// GitPanelDiscardFile descarta alterações locais de um arquivo.
func (a *App) GitPanelDiscardFile(repoPath string, filePath string) error {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return err
	}
	return a.normalizeGitPanelBindingError(svc.DiscardFile(repoPath, filePath))
}

// GitPanelStagePatch aplica patch parcial no stage.
func (a *App) GitPanelStagePatch(repoPath string, patchText string) error {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return err
	}
	return a.normalizeGitPanelBindingError(svc.StagePatch(repoPath, patchText))
}

// GitPanelUnstagePatch remove patch parcial do stage.
func (a *App) GitPanelUnstagePatch(repoPath string, patchText string) error {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return err
	}
	return a.normalizeGitPanelBindingError(svc.UnstagePatch(repoPath, patchText))
}

// GitPanelAcceptOurs aplica resolução de conflito com versão local.
func (a *App) GitPanelAcceptOurs(repoPath string, filePath string, autoStage bool) error {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return err
	}
	return a.normalizeGitPanelBindingError(svc.AcceptOurs(repoPath, filePath, autoStage))
}

// GitPanelAcceptTheirs aplica resolução de conflito com versão remota.
func (a *App) GitPanelAcceptTheirs(repoPath string, filePath string, autoStage bool) error {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return err
	}
	return a.normalizeGitPanelBindingError(svc.AcceptTheirs(repoPath, filePath, autoStage))
}

// GitPanelOpenExternalMergeTool abre a ferramenta de merge configurada no Git.
func (a *App) GitPanelOpenExternalMergeTool(repoPath string, filePath string) error {
	svc, err := a.requireGitPanelService()
	if err != nil {
		return err
	}
	return a.normalizeGitPanelBindingError(svc.OpenExternalMergeTool(repoPath, filePath))
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

func (a *App) gatewayCreateSession(hostUserID, hostName, hostAvatarURL string, cfg session.SessionConfig) (*session.Session, error) {
	var result session.Session
	err := a.callSessionGateway(http.MethodPost, "/api/session/create", map[string]interface{}{
		"hostUserID":    hostUserID,
		"hostName":      hostName,
		"hostAvatarUrl": hostAvatarURL,
		"config":        cfg,
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

func (a *App) gatewayRegenerateSessionCode(sessionID string) (*session.Session, error) {
	var result session.Session
	err := a.callSessionGateway(http.MethodPost, "/api/session/code/regenerate", map[string]interface{}{
		"sessionID": sessionID,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *App) gatewayRevokeSessionCode(sessionID string) (*session.Session, error) {
	var result session.Session
	err := a.callSessionGateway(http.MethodPost, "/api/session/code/revoke", map[string]interface{}{
		"sessionID": sessionID,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (a *App) gatewaySetAllowNewJoins(sessionID string, allow bool) (*session.Session, error) {
	var result session.Session
	err := a.callSessionGateway(http.MethodPost, "/api/session/allow-joins", map[string]interface{}{
		"sessionID": sessionID,
		"allow":     allow,
	}, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func isSessionNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "session not found") || strings.Contains(msg, "session not found for code")
}

const genericSessionJoinCodeError = "código inválido ou expirado"

func isSessionJoinCodeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid code format") ||
		strings.Contains(msg, "session not found for code") ||
		strings.Contains(msg, "session code has expired") ||
		strings.Contains(msg, "too many invalid join attempts")
}

func sanitizeSessionJoinError(err error) error {
	if err == nil {
		return nil
	}
	if isSessionJoinCodeError(err) {
		return errors.New(genericSessionJoinCodeError)
	}
	return err
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
	hostUser, err := a.requireGitHubSessionUser()
	if err != nil {
		return nil, err
	}

	hostUserID := strings.TrimSpace(hostUser.ID)
	hostName := strings.TrimSpace(hostUser.Name)
	if hostName == "" {
		hostName = hostUserID
	}
	hostAvatarURL := strings.TrimSpace(hostUser.AvatarURL)
	allowAnonymous = false

	if a.db == nil {
		return nil, fmt.Errorf("database service not initialized")
	}

	var (
		scopedWorkspace *database.Workspace
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
		AllowAnonymous: false,
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
		createdSession, err = a.gatewayCreateSession(hostUserID, hostName, hostAvatarURL, cfg)
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
	createdSession.HostUserID = hostUserID
	createdSession.HostName = hostName
	createdSession.HostAvatarURL = hostAvatarURL

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
	guestUser, err := a.requireGitHubSessionUser()
	if err != nil {
		return nil, err
	}

	guestUserID := strings.TrimSpace(guestUser.ID)
	if guestUserID == "" {
		return nil, fmt.Errorf("sessões colaborativas exigem um usuário autenticado válido")
	}
	avatarURL := strings.TrimSpace(guestUser.AvatarURL)

	if strings.TrimSpace(name) == "" {
		name = strings.TrimSpace(guestUser.Name)
	}
	if strings.TrimSpace(email) == "" {
		email = strings.TrimSpace(guestUser.Email)
	}
	if strings.TrimSpace(name) == "" {
		name = guestUserID
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
				return nil, sanitizeSessionJoinError(fmt.Errorf("gateway join failed in client mode: %w", err))
			}
			return nil, sanitizeSessionJoinError(fmt.Errorf("session service not initialized and gateway join failed: %w", err))
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
				return nil, sanitizeSessionJoinError(fmt.Errorf("%v (gateway fallback failed: %w)", err, remoteErr))
			}
			a.auditSessionEvent(remoteResult.SessionID, guestUserID, "guest_requested_join", fmt.Sprintf("name=%s", name))
			return remoteResult, nil
		}
		return nil, sanitizeSessionJoinError(err)
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

// SessionRegenerateCode gera um novo código de convite para a sessão.
func (a *App) SessionRegenerateCode(sessionID string) (*session.Session, error) {
	var (
		updated *session.Session
		err     error
	)
	if a.session == nil || !a.sessionGatewayOwner {
		updated, err = a.gatewayRegenerateSessionCode(sessionID)
	} else {
		updated, err = a.session.RegenerateCode(sessionID)
		if isSessionNotFoundErr(err) {
			updated, err = a.gatewayRegenerateSessionCode(sessionID)
		}
	}
	if err != nil {
		return nil, err
	}

	a.auditSessionEvent(sessionID, "host", "code_regenerated", "Host generated a new session join code")
	if a.sessionGatewayOwner {
		a.persistSessionState(sessionID)
	}
	return updated, nil
}

// SessionRevokeCode invalida o código de convite atual da sessão.
func (a *App) SessionRevokeCode(sessionID string) (*session.Session, error) {
	var (
		updated *session.Session
		err     error
	)
	if a.session == nil || !a.sessionGatewayOwner {
		updated, err = a.gatewayRevokeSessionCode(sessionID)
	} else {
		updated, err = a.session.RevokeCode(sessionID)
		if isSessionNotFoundErr(err) {
			updated, err = a.gatewayRevokeSessionCode(sessionID)
		}
	}
	if err != nil {
		return nil, err
	}

	a.auditSessionEvent(sessionID, "host", "code_revoked", "Host revoked the current session join code")
	if a.sessionGatewayOwner {
		a.persistSessionState(sessionID)
	}
	return updated, nil
}

// SessionSetAllowNewJoins habilita/desabilita novos pedidos de entrada.
func (a *App) SessionSetAllowNewJoins(sessionID string, allow bool) (*session.Session, error) {
	var (
		updated *session.Session
		err     error
	)
	if a.session == nil || !a.sessionGatewayOwner {
		updated, err = a.gatewaySetAllowNewJoins(sessionID, allow)
	} else {
		updated, err = a.session.SetAllowNewJoins(sessionID, allow)
		if isSessionNotFoundErr(err) {
			updated, err = a.gatewaySetAllowNewJoins(sessionID, allow)
		}
	}
	if err != nil {
		return nil, err
	}

	action := "allow_new_joins_disabled"
	details := "Host disabled new join requests"
	if allow {
		action = "allow_new_joins_enabled"
		details = "Host enabled new join requests"
	}
	a.auditSessionEvent(sessionID, "host", action, details)
	if a.sessionGatewayOwner {
		a.persistSessionState(sessionID)
	}
	return updated, nil
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

// SessionGetJoinSecurityMetrics retorna métricas agregadas de tentativas inválidas/bloqueios de join.
func (a *App) SessionGetJoinSecurityMetrics() session.JoinSecurityMetrics {
	var metrics session.JoinSecurityMetrics
	if a.session == nil {
		return metrics
	}
	if a.sessionGatewayOwner {
		return a.session.GetJoinSecurityMetrics()
	}
	if err := a.callSessionGateway(http.MethodGet, "/api/session/metrics/join-security", nil, &metrics); err != nil {
		log.Printf("[SESSION][SECURITY] unable to fetch join metrics from gateway: %v", err)
	}
	return metrics
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

	if !a.docker.IsDockerAvailable() {
		return fmt.Errorf("Docker não detectado ou não está rodando. Verifique se o Docker Desktop está aberto.")
	}

	// Prevenir builds simultâneos
	a.stackBuildMu.Lock()
	if a.stackBuildRunning {
		a.stackBuildMu.Unlock()
		return fmt.Errorf("a build is already in progress")
	}
	a.stackBuildRunning = true
	a.stackBuildLogs = []string{"Starting environment build..."}
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
