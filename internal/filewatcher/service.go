package filewatcher

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Service implementa IFileWatcher usando fsnotify
type Service struct {
	mu       sync.RWMutex
	watcher  *fsnotify.Watcher
	handlers []func(FileEvent)
	debounce map[string]*time.Timer
	recent   map[string]time.Time
	projects map[string]string // projectPath -> gitDir real monitorado
	loopOn   bool
	done     chan struct{}
	closed   bool
	rawLogs  bool
	ignored  bool
	window   time.Duration

	// Callback para emitir eventos Wails (injetado pelo app.go)
	emitEvent func(eventName string, data interface{})
}

// NewService cria um novo FileWatcher Service
func NewService(emitEvent func(eventName string, data interface{})) (*Service, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	return &Service{
		watcher:   watcher,
		handlers:  make([]func(FileEvent), 0),
		debounce:  make(map[string]*time.Timer),
		recent:    make(map[string]time.Time),
		projects:  make(map[string]string),
		done:      make(chan struct{}),
		rawLogs:   readEnvBool("ORCH_FILEWATCHER_DEBUG_RAW"),
		ignored:   readEnvBool("ORCH_FILEWATCHER_DEBUG_IGNORED"),
		window:    900 * time.Millisecond,
		emitEvent: emitEvent,
	}, nil
}

// Watch inicia o monitoramento da pasta .git de um projeto
func (s *Service) Watch(projectPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("watcher is closed")
	}

	projectPath = filepath.Clean(projectPath)

	// Verificar se já está monitorando
	if _, alreadyWatching := s.projects[projectPath]; alreadyWatching {
		return nil
	}

	gitDir, err := resolveGitDir(projectPath)
	if err != nil {
		return fmt.Errorf("not a git repository: %s", projectPath)
	}

	paths := collectWatchPaths(gitDir)

	// Adicionar watchers para cada path
	for _, p := range paths {
		if err := s.watcher.Add(p); err != nil {
			log.Printf("[FileWatcher] Warning: could not watch %s: %v", p, err)
		}
	}

	s.projects[projectPath] = gitDir
	log.Printf("[FileWatcher] Watching %s", projectPath)

	// Iniciar event loop apenas uma vez
	if !s.loopOn {
		s.loopOn = true
		go s.eventLoop()
	}

	return nil
}

// Unwatch para o monitoramento de um projeto
func (s *Service) Unwatch(projectPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	projectPath = filepath.Clean(projectPath)
	gitDir, exists := s.projects[projectPath]
	if !exists {
		return nil
	}

	paths := collectWatchPaths(gitDir)

	for _, p := range paths {
		_ = s.watcher.Remove(p)
	}

	delete(s.projects, projectPath)
	log.Printf("[FileWatcher] Unwatched %s", projectPath)
	return nil
}

// OnChange registra um handler para receber eventos
func (s *Service) OnChange(handler func(event FileEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers = append(s.handlers, handler)
}

// GetCurrentBranch retorna a branch atual do projeto
func (s *Service) GetCurrentBranch(projectPath string) (string, error) {
	return readCurrentBranch(projectPath)
}

// GetLastCommit retorna informações do último commit
func (s *Service) GetLastCommit(projectPath string) (*CommitInfo, error) {
	return readLastCommit(projectPath)
}

// Close encerra todos os watchers
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	// Cancelar todos os debounce timers
	for _, timer := range s.debounce {
		timer.Stop()
	}

	close(s.done)
	return s.watcher.Close()
}

// === Event Loop ===

func (s *Service) eventLoop() {
	for {
		select {
		case <-s.done:
			return

		case event, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			if s.rawLogs {
				log.Printf("[FileWatcher][raw] op=%s path=%s", event.Op.String(), event.Name)
			}

			// Git atualiza refs via lock+rename; precisamos considerar mais operações.
			if !event.Has(fsnotify.Write) &&
				!event.Has(fsnotify.Create) &&
				!event.Has(fsnotify.Rename) &&
				!event.Has(fsnotify.Remove) &&
				!event.Has(fsnotify.Chmod) {
				continue
			}

			// Debounce: 200ms por arquivo (path normalizado).
			key := normalizeGitEventPath(event.Name)
			s.mu.Lock()
			if timer, exists := s.debounce[key]; exists {
				timer.Stop()
			}
			ev := event
			s.debounce[key] = time.AfterFunc(200*time.Millisecond, func() {
				s.handleDebouncedEvent(ev)
			})
			s.mu.Unlock()

		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[FileWatcher] Error: %v", err)
		}
	}
}

func (s *Service) handleDebouncedEvent(event fsnotify.Event) {
	// Se uma nova subpasta de refs foi criada (ex: refs/heads/feature),
	// adicionamos watch dinâmico para não perder eventos de branches com slash.
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			s.mu.Lock()
			if !s.closed {
				if err := s.watcher.Add(event.Name); err != nil {
					log.Printf("[FileWatcher] Warning: could not watch new directory %s: %v", event.Name, err)
				}
			}
			s.mu.Unlock()
		}
	}

	normalizedPath := normalizeGitEventPath(event.Name)

	// Determinar o projectPath a partir do caminho do evento
	projectPath := s.findProjectPath(normalizedPath)
	if projectPath == "" {
		return
	}

	fileEvent := s.classifyEvent(event, projectPath)
	if fileEvent == nil {
		if s.ignored {
			log.Printf("[FileWatcher] Event ignored after classify: op=%s path=%s project=%s", event.Op.String(), event.Name, projectPath)
		}
		return
	}

	if !s.shouldEmit(*fileEvent) {
		if s.ignored {
			log.Printf("[FileWatcher] Event deduped: %s (%s)", fileEvent.Type, fileEvent.Path)
		}
		return
	}

	log.Printf("[FileWatcher] Event: %s (%s)", fileEvent.Type, fileEvent.Path)

	// Notificar handlers registrados
	s.mu.RLock()
	handlers := make([]func(FileEvent), len(s.handlers))
	copy(handlers, s.handlers)
	s.mu.RUnlock()

	for _, handler := range handlers {
		handler(*fileEvent)
	}

	// Emitir evento Wails se callback configurado
	if s.emitEvent != nil {
		eventName := "git:" + fileEvent.Type
		s.emitEvent(eventName, fileEvent)
	}
}

func (s *Service) shouldEmit(event FileEvent) bool {
	key := semanticEventKey(event)
	now := time.Now()
	cutoff := now.Add(-3 * s.window)

	s.mu.Lock()
	defer s.mu.Unlock()

	for k, ts := range s.recent {
		if ts.Before(cutoff) {
			delete(s.recent, k)
		}
	}

	if last, exists := s.recent[key]; exists && now.Sub(last) <= s.window {
		return false
	}

	s.recent[key] = now
	return true
}

func semanticEventKey(event FileEvent) string {
	var b strings.Builder
	b.Grow(128)
	b.WriteString(event.Type)
	b.WriteString("|")
	b.WriteString(normalizeGitEventPath(event.Path))
	if branch, ok := event.Details["branch"]; ok && branch != "" {
		b.WriteString("|branch=")
		b.WriteString(branch)
	}
	if ref, ok := event.Details["ref"]; ok && ref != "" {
		b.WriteString("|ref=")
		b.WriteString(ref)
	}
	return b.String()
}

// findProjectPath encontra o projectPath correspondente ao evento
func (s *Service) findProjectPath(eventPath string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cleanEventPath := filepath.Clean(eventPath)
	for projectPath, gitDir := range s.projects {
		cleanGitDir := filepath.Clean(gitDir)
		if cleanEventPath == cleanGitDir ||
			strings.HasPrefix(cleanEventPath, cleanGitDir+string(os.PathSeparator)) {
			return projectPath
		}
	}
	return ""
}

// classifyEvent classifica um evento fsnotify em um FileEvent
func (s *Service) classifyEvent(event fsnotify.Event, projectPath string) *FileEvent {
	eventPath := normalizeGitEventPath(event.Name)
	name := filepath.Base(eventPath)
	now := time.Now()

	switch {
	case name == "HEAD":
		branch, _ := readCurrentBranch(projectPath)
		return &FileEvent{
			Type:      "branch_changed",
			Path:      eventPath,
			Timestamp: now,
			Details:   map[string]string{"branch": branch},
		}

	case strings.Contains(eventPath, filepath.Join("refs", "heads")):
		return &FileEvent{
			Type:      "commit",
			Path:      eventPath,
			Timestamp: now,
			Details:   map[string]string{"ref": name},
		}

	case strings.Contains(eventPath, filepath.Join("refs", "remotes")):
		return &FileEvent{
			Type:      "fetch",
			Path:      eventPath,
			Timestamp: now,
			Details:   map[string]string{"ref": name},
		}

	case name == "MERGE_HEAD":
		return &FileEvent{
			Type:      "merge",
			Path:      eventPath,
			Timestamp: now,
			Details:   map[string]string{},
		}

	case name == "FETCH_HEAD":
		return &FileEvent{
			Type:      "fetch",
			Path:      eventPath,
			Timestamp: now,
			Details:   map[string]string{},
		}

	case name == "index":
		return &FileEvent{
			Type:      "index",
			Path:      eventPath,
			Timestamp: now,
			Details:   map[string]string{},
		}

	case name == "COMMIT_EDITMSG":
		return &FileEvent{
			Type:      "commit_preparing",
			Path:      eventPath,
			Timestamp: now,
			Details:   map[string]string{},
		}
	}

	return nil
}

// === Helper Functions ===

// readCurrentBranch lê a branch atual a partir de .git/HEAD
func readCurrentBranch(projectPath string) (string, error) {
	gitDir, err := resolveGitDir(projectPath)
	if err != nil {
		return "", err
	}

	headPath := filepath.Join(gitDir, "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return "", fmt.Errorf("failed to read HEAD: %w", err)
	}

	content := strings.TrimSpace(string(data))

	// "ref: refs/heads/main" → "main"
	if strings.HasPrefix(content, "ref: refs/heads/") {
		return strings.TrimPrefix(content, "ref: refs/heads/"), nil
	}

	// Detached HEAD (hash direto) — mostrar primeiros 8 chars
	if len(content) >= 8 {
		return content[:8] + " (detached)", nil
	}

	return content, nil
}

// readLastCommit lê informações do último commit usando git log
func readLastCommit(projectPath string) (*CommitInfo, error) {
	// Usar git log para obter informações do último commit
	cmd := exec.Command("git", "log", "-1", "--format=%H%n%s%n%an%n%aI")
	cmd.Dir = projectPath

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to read last commit: %w", err)
	}

	lines := make([]string, 0, 4)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if len(lines) < 4 {
		return nil, fmt.Errorf("unexpected git log output")
	}

	date, _ := time.Parse(time.RFC3339, lines[3])

	return &CommitInfo{
		Hash:    lines[0],
		Message: lines[1],
		Author:  lines[2],
		Date:    date,
	}, nil
}

func resolveGitDir(projectPath string) (string, error) {
	gitPath := filepath.Join(projectPath, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", err
	}

	// Repo padrão: .git é diretório.
	if info.IsDir() {
		return filepath.Clean(gitPath), nil
	}

	// Worktree/submodule: .git é arquivo com "gitdir: <path>".
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("failed to read .git file: %w", err)
	}

	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(strings.ToLower(content), "gitdir:") {
		return "", fmt.Errorf("invalid .git file format")
	}

	gitDir := strings.TrimSpace(content[len("gitdir:"):])
	if gitDir == "" {
		return "", fmt.Errorf("empty gitdir in .git file")
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(projectPath, gitDir)
	}

	return filepath.Clean(gitDir), nil
}

func collectWatchPaths(gitDir string) []string {
	paths := []string{gitDir}
	candidates := []string{
		filepath.Join(gitDir, "refs", "heads"),
		filepath.Join(gitDir, "refs", "remotes"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err != nil {
			continue
		}

		paths = append(paths, candidate)
		entries, _ := os.ReadDir(candidate)
		for _, entry := range entries {
			if entry.IsDir() {
				paths = append(paths, filepath.Join(candidate, entry.Name()))
			}
		}
	}

	unique := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		unique = append(unique, clean)
	}
	return unique
}

func normalizeGitEventPath(path string) string {
	clean := filepath.Clean(path)
	if strings.HasSuffix(clean, ".lock") {
		return strings.TrimSuffix(clean, ".lock")
	}
	return clean
}

func readEnvBool(key string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}
