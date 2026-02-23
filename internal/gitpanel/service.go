package gitpanel

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type EventEmitter func(eventName string, data interface{})

const (
	defaultReadTimeout  = 8 * time.Second
	defaultWriteTimeout = 12 * time.Second
	externalToolTimeout = 5 * time.Minute
	maxHistoryLimit     = 500
	historyFallbackMax  = 80
	defaultHistoryLimit = 200
	maxDiffBytes        = 256000
	maxDiffPreviewBytes = 1 * 1024 * 1024

	preflightCacheTTL = 2 * time.Second
	statusCacheTTL    = 1200 * time.Millisecond
	historyCacheTTL   = 2 * time.Second
	diffCacheTTL      = 2 * time.Second
)

var conflictStatuses = map[string]struct{}{
	"UU": {},
	"AA": {},
	"DD": {},
	"AU": {},
	"UA": {},
	"DU": {},
	"UD": {},
}

var diffHunkHeaderRegex = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)

type preflightCacheEntry struct {
	value     PreflightResult
	expiresAt time.Time
}

type statusCacheEntry struct {
	value     StatusDTO
	expiresAt time.Time
}

type historyCacheEntry struct {
	value     HistoryPageDTO
	expiresAt time.Time
}

type diffCacheEntry struct {
	value     DiffDTO
	expiresAt time.Time
}

// Service encapsula operações de leitura/write/eventos do Git Panel.
type Service struct {
	emit       EventEmitter
	runGit     gitRunner
	sleep      backoffSleeper
	commandSeq uint64

	queueMu        sync.Mutex
	queues         map[string]*repoCommandQueue
	workerWG       sync.WaitGroup
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	closed         atomic.Bool

	cacheMu        sync.RWMutex
	preflightCache map[string]preflightCacheEntry
	statusCache    map[string]statusCacheEntry
	historyCache   map[string]historyCacheEntry
	diffCache      map[string]diffCacheEntry
}

func NewService(emit EventEmitter) *Service {
	return newServiceWithDeps(emit, runGitWithInput, sleepWithContext)
}

func newServiceWithDeps(emit EventEmitter, runner gitRunner, sleeper backoffSleeper) *Service {
	if emit == nil {
		emit = func(string, interface{}) {}
	}
	if runner == nil {
		runner = runGitWithInput
	}
	if sleeper == nil {
		sleeper = sleepWithContext
	}

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	return &Service{
		emit:           emit,
		runGit:         runner,
		sleep:          sleeper,
		queues:         make(map[string]*repoCommandQueue),
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
		preflightCache: make(map[string]preflightCacheEntry),
		statusCache:    make(map[string]statusCacheEntry),
		historyCache:   make(map[string]historyCacheEntry),
		diffCache:      make(map[string]diffCacheEntry),
	}
}

func (s *Service) Preflight(repoPath string) (PreflightResult, error) {
	result := PreflightResult{}

	if _, err := exec.LookPath("git"); err != nil {
		return result, NewBindingError(
			CodeGitUnavailable,
			"Git CLI não encontrado no ambiente.",
			"Instale o Git e reinicie o ORCH.",
		)
	}
	result.GitAvailable = true

	normalizedRepoPath := strings.TrimSpace(repoPath)
	if normalizedRepoPath == "" {
		return result, NewBindingError(
			CodeRepoNotResolved,
			"Repositório não resolvido.",
			"Selecione um repositório Git antes de executar ações de escrita.",
		)
	}

	absRepoPath, err := filepath.Abs(normalizedRepoPath)
	if err != nil {
		return result, NewBindingError(
			CodeRepoNotFound,
			"Não foi possível resolver o caminho do repositório.",
			err.Error(),
		)
	}
	absRepoPath = filepath.Clean(absRepoPath)

	if cached, ok := s.getCachedPreflight(absRepoPath); ok {
		return cached, nil
	}

	stat, err := os.Stat(absRepoPath)
	if err != nil {
		return result, NewBindingError(
			CodeRepoNotFound,
			"Caminho do repositório não encontrado.",
			err.Error(),
		)
	}
	if !stat.IsDir() {
		return result, NewBindingError(
			CodeRepoNotFound,
			"Caminho informado não é um diretório.",
			absRepoPath,
		)
	}

	rootOut, rootErrOut, rootExitCode, rootErr := s.runGit(context.Background(), defaultReadTimeout, "", "-C", absRepoPath, "rev-parse", "--show-toplevel")
	if rootErr != nil {
		return result, NewBindingError(
			CodeRepoNotGit,
			"Caminho informado não é um repositório Git válido.",
			formatCommandFailureDetails(rootErrOut, rootExitCode, rootErr),
		)
	}
	repoRoot := strings.TrimSpace(rootOut)
	if repoRoot == "" {
		return result, NewBindingError(
			CodeRepoNotGit,
			"Não foi possível determinar a raiz do repositório Git.",
			absRepoPath,
		)
	}

	branchOut, branchErrOut, branchExitCode, branchErr := s.runGit(context.Background(), defaultReadTimeout, "", "-C", repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	branch := strings.TrimSpace(branchOut)
	if branchErr != nil {
		branch = ""
		if strings.TrimSpace(branchErrOut) != "" {
			branch = ""
		}
		_ = branchExitCode
	}

	result.RepoPath = absRepoPath
	result.RepoRoot = repoRoot
	result.Branch = branch

	// Detect active merge by checking .git/MERGE_HEAD
	mergeHeadPath := filepath.Join(repoRoot, ".git", "MERGE_HEAD")
	if _, mergeErr := os.Stat(mergeHeadPath); mergeErr == nil {
		result.MergeActive = true
	}

	s.setCachedPreflight(absRepoPath, result)
	if cleanedRoot := filepath.Clean(strings.TrimSpace(repoRoot)); cleanedRoot != "" && cleanedRoot != absRepoPath {
		s.setCachedPreflight(cleanedRoot, result)
	}

	return result, nil
}

func (s *Service) GetStatus(repoPath string) (StatusDTO, error) {
	preflight, err := s.Preflight(repoPath)
	if err != nil {
		return StatusDTO{}, err
	}

	if cached, ok := s.getCachedStatus(preflight.RepoRoot); ok {
		return cached, nil
	}

	out, errOut, exitCode, runErr := s.runGit(
		context.Background(),
		defaultReadTimeout,
		"",
		"-C", preflight.RepoRoot,
		"status",
		"--porcelain=v1",
		"-z",
		"--branch",
	)
	if runErr != nil {
		return StatusDTO{}, NewBindingError(
			CodeCommandFailed,
			"Falha ao obter status do repositório.",
			formatCommandFailureDetails(errOut, exitCode, runErr),
		)
	}

	status := parsePorcelainStatus(out)
	if status.Branch == "" {
		status.Branch = preflight.Branch
	}
	s.setCachedStatus(preflight.RepoRoot, status)
	return status, nil
}

func (s *Service) GetHistory(repoPath string, cursor string, limit int, search string) (HistoryPageDTO, error) {
	preflight, err := s.Preflight(repoPath)
	if err != nil {
		return HistoryPageDTO{}, err
	}

	cursorHash, cursorErr := parseHistoryCursor(cursor)
	if cursorErr != nil {
		return HistoryPageDTO{}, cursorErr
	}
	skipCount, skipErr := s.resolveHistorySkip(preflight.RepoRoot, cursorHash, search)
	if skipErr != nil {
		return HistoryPageDTO{}, skipErr
	}

	if limit <= 0 {
		limit = defaultHistoryLimit
	}
	if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}
	cacheKey := buildHistoryCacheKey(preflight.RepoRoot, cursorHash, limit, search)
	if cached, ok := s.getCachedHistory(cacheKey); ok {
		return cached, nil
	}

	pageSize := limit + 1
	format := "%H%x1f%h%x1f%an%x1f%aI%x1f%ae%x1f%s%x1e"
	buildLogArgs := func(pageLimit int) []string {
		args := []string{
			"-C", preflight.RepoRoot,
			"log",
			"--date=iso-strict",
			"--pretty=format:" + format,
			"--numstat",
		}

		trimmedSearch := strings.TrimSpace(search)
		if trimmedSearch != "" {
			args = append(args, "--grep="+trimmedSearch, "-i")
		}

		args = append(args,
			fmt.Sprintf("--skip=%d", skipCount),
			"-n", strconv.Itoa(pageLimit),
		)
		return args
	}

	args := buildLogArgs(pageSize)

	out, errOut, exitCode, runErr := s.runGit(context.Background(), defaultReadTimeout, "", args...)
	if runErr != nil && isTimeoutBindingError(runErr) && limit > historyFallbackMax {
		fallbackLimit := historyFallbackMax
		if fallbackLimit > limit {
			fallbackLimit = limit
		}
		fallbackPageSize := fallbackLimit + 1
		out, errOut, exitCode, runErr = s.runGit(context.Background(), defaultReadTimeout, "", buildLogArgs(fallbackPageSize)...)
		if runErr == nil {
			limit = fallbackLimit
		}
	}
	if runErr != nil {
		return HistoryPageDTO{}, NewBindingError(
			CodeCommandFailed,
			"Falha ao obter histórico do repositório.",
			formatCommandFailureDetails(errOut, exitCode, runErr),
		)
	}

	items := parseHistoryItems(out)
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if hasMore {
		nextCursor = strings.TrimSpace(items[len(items)-1].Hash)
	}

	page := HistoryPageDTO{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}
	s.setCachedHistory(cacheKey, page)
	return page, nil
}

func (s *Service) GetDiff(repoPath string, filePath string, mode string, contextLines int) (DiffDTO, error) {
	preflight, err := s.Preflight(repoPath)
	if err != nil {
		return DiffDTO{}, err
	}

	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	if normalizedMode == "" {
		normalizedMode = "unified"
	}

	if contextLines <= 0 {
		contextLines = 3
	}
	if contextLines > 120 {
		contextLines = 120
	}

	args := []string{
		"-C", preflight.RepoRoot,
		"diff",
		fmt.Sprintf("--unified=%d", contextLines),
	}
	if normalizedMode == "staged" {
		args = append(args, "--cached")
	}

	cleanFilePath := ""
	if strings.TrimSpace(filePath) != "" {
		pathWithinRepo, pathErr := ensurePathWithinRepo(preflight.RepoRoot, filePath)
		if pathErr != nil {
			return DiffDTO{}, pathErr
		}
		cleanFilePath = pathWithinRepo
		args = append(args, "--", cleanFilePath)
	}
	cacheKey := buildDiffCacheKey(preflight.RepoRoot, cleanFilePath, normalizedMode, contextLines)
	if cached, ok := s.getCachedDiff(cacheKey); ok {
		return cached, nil
	}
	if cleanFilePath != "" {
		if size, hasSize := statRepoFileSize(preflight.RepoRoot, cleanFilePath); hasSize && size > maxDiffPreviewBytes {
			degraded := buildLargeDiffFallback(normalizedMode, cleanFilePath, size)
			s.setCachedDiff(cacheKey, degraded)
			return degraded, nil
		}
	}

	out, errOut, exitCode, runErr := s.runGit(context.Background(), defaultReadTimeout, "", args...)
	if runErr != nil {
		if isTimeoutBindingError(runErr) {
			degraded := buildTimeoutDiffFallback(normalizedMode, cleanFilePath)
			s.setCachedDiff(cacheKey, degraded)
			return degraded, nil
		}
		return DiffDTO{}, NewBindingError(
			CodeCommandFailed,
			"Falha ao obter diff do repositório.",
			formatCommandFailureDetails(errOut, exitCode, runErr),
		)
	}

	files := parseDiffFiles(out)
	isBinary := strings.Contains(out, "Binary files ") || strings.Contains(out, "GIT binary patch")
	if !isBinary {
		for _, file := range files {
			if file.IsBinary {
				isBinary = true
				break
			}
		}
	}

	isTruncated := false
	raw := out
	if len(out) > maxDiffBytes {
		raw = out[:maxDiffBytes] + "\n\n... (diff truncado para manter responsividade)"
		isTruncated = true
	}

	result := DiffDTO{
		Mode:        normalizedMode,
		FilePath:    cleanFilePath,
		Raw:         raw,
		Files:       files,
		IsBinary:    isBinary,
		IsTruncated: isTruncated,
	}
	s.setCachedDiff(cacheKey, result)
	return result, nil
}

func (s *Service) GetConflicts(repoPath string) ([]ConflictFileDTO, error) {
	status, err := s.GetStatus(repoPath)
	if err != nil {
		return nil, err
	}
	return status.Conflicted, nil
}

func (s *Service) StageFile(repoPath string, filePath string) error {
	preflight, cleanPath, commandID, startedAt, err := s.prepareWrite(repoPath, filePath, "stage_file")
	if err != nil {
		s.emitCommandFailure(commandID, repoPath, "stage_file", []string{"add", "--", filePath}, startedAt, err)
		return err
	}

	if err := s.executeWrite(
		preflight.RepoRoot,
		commandID,
		"stage_file",
		[]string{"add", "--", cleanPath},
		startedAt,
		defaultWriteTimeout,
		func(ctx context.Context, diag *commandDiagnosticState) error {
			_, errOut, exitCode, runErr := s.runWriteGitWithRetry(
				ctx,
				diag,
				"",
				"-C", preflight.RepoRoot,
				"add",
				"--",
				cleanPath,
			)
			if runErr != nil {
				return wrapWriteCommandError(
					CodeCommandFailed,
					"Falha ao adicionar arquivo ao stage.",
					errOut,
					exitCode,
					runErr,
				)
			}
			return nil
		}); err != nil {
		return err
	}

	s.emitPostWriteReconciliation(preflight.RepoRoot, "stage_file", false)
	return nil
}

func (s *Service) UnstageFile(repoPath string, filePath string) error {
	preflight, cleanPath, commandID, startedAt, err := s.prepareWrite(repoPath, filePath, "unstage_file")
	if err != nil {
		s.emitCommandFailure(commandID, repoPath, "unstage_file", []string{"restore", "--staged", "--", filePath}, startedAt, err)
		return err
	}

	if err := s.executeWrite(
		preflight.RepoRoot,
		commandID,
		"unstage_file",
		[]string{"restore", "--staged", "--", cleanPath},
		startedAt,
		defaultWriteTimeout,
		func(ctx context.Context, diag *commandDiagnosticState) error {
			_, errOut, exitCode, runErr := s.runWriteGitWithRetry(
				ctx,
				diag,
				"",
				"-C", preflight.RepoRoot,
				"restore",
				"--staged",
				"--",
				cleanPath,
			)
			if runErr != nil {
				return wrapWriteCommandError(
					CodeCommandFailed,
					"Falha ao remover arquivo do stage.",
					errOut,
					exitCode,
					runErr,
				)
			}
			return nil
		}); err != nil {
		return err
	}

	s.emitPostWriteReconciliation(preflight.RepoRoot, "unstage_file", false)
	return nil
}

func (s *Service) DiscardFile(repoPath string, filePath string) error {
	preflight, cleanPath, commandID, startedAt, err := s.prepareWrite(repoPath, filePath, "discard_file")
	if err != nil {
		s.emitCommandFailure(commandID, repoPath, "discard_file", []string{"checkout", "--", filePath}, startedAt, err)
		return err
	}

	if err := s.executeWrite(
		preflight.RepoRoot,
		commandID,
		"discard_file",
		[]string{"checkout", "--", cleanPath},
		startedAt,
		defaultWriteTimeout,
		func(ctx context.Context, diag *commandDiagnosticState) error {
			_, errOut, exitCode, runErr := s.runWriteGitWithRetry(
				ctx,
				diag,
				"",
				"-C", preflight.RepoRoot,
				"checkout",
				"--",
				cleanPath,
			)
			if runErr != nil {
				return wrapWriteCommandError(
					CodeCommandFailed,
					"Falha ao descartar alterações do arquivo.",
					errOut,
					exitCode,
					runErr,
				)
			}
			return nil
		}); err != nil {
		return err
	}

	s.emitPostWriteReconciliation(preflight.RepoRoot, "discard_file", false)
	return nil
}

func (s *Service) StagePatch(repoPath string, patchText string) error {
	commandID, startedAt := s.beginCommand("stage_patch")

	trimmedPatch := strings.TrimSpace(patchText)
	if trimmedPatch == "" {
		err := NewBindingError(
			CodePatchInvalid,
			"Patch inválido para stage parcial.",
			"O texto do patch está vazio.",
		)
		s.emitCommandFailure(commandID, repoPath, "stage_patch", []string{"apply", "--cached", "--unidiff-zero", "--whitespace=nowarn", "-"}, startedAt, err)
		return err
	}

	preflight, err := s.Preflight(repoPath)
	if err != nil {
		s.emitCommandFailure(commandID, repoPath, "stage_patch", []string{"apply", "--cached", "--unidiff-zero", "--whitespace=nowarn", "-"}, startedAt, err)
		return err
	}
	if validationErr := validatePatchText(preflight.RepoRoot, trimmedPatch); validationErr != nil {
		s.emitCommandFailure(commandID, preflight.RepoRoot, "stage_patch", []string{"apply", "--cached", "--unidiff-zero", "--whitespace=nowarn", "-"}, startedAt, validationErr)
		return validationErr
	}

	if err := s.executeWrite(
		preflight.RepoRoot,
		commandID,
		"stage_patch",
		[]string{"apply", "--cached", "--unidiff-zero", "--whitespace=nowarn", "-"},
		startedAt,
		defaultWriteTimeout,
		func(ctx context.Context, diag *commandDiagnosticState) error {
			_, errOut, exitCode, runErr := s.runWriteGitWithRetry(
				ctx,
				diag,
				trimmedPatch,
				"-C", preflight.RepoRoot,
				"apply",
				"--cached",
				"--unidiff-zero",
				"--whitespace=nowarn",
				"-",
			)
			if runErr != nil {
				return wrapWriteCommandError(
					CodePatchInvalid,
					"Falha ao aplicar patch no stage.",
					errOut,
					exitCode,
					runErr,
				)
			}
			return nil
		}); err != nil {
		return err
	}

	s.emitPostWriteReconciliation(preflight.RepoRoot, "stage_patch", false)
	return nil
}

func (s *Service) UnstagePatch(repoPath string, patchText string) error {
	commandID, startedAt := s.beginCommand("unstage_patch")

	trimmedPatch := strings.TrimSpace(patchText)
	if trimmedPatch == "" {
		err := NewBindingError(
			CodePatchInvalid,
			"Patch inválido para unstage parcial.",
			"O texto do patch está vazio.",
		)
		s.emitCommandFailure(commandID, repoPath, "unstage_patch", []string{"apply", "--cached", "--reverse", "--unidiff-zero", "--whitespace=nowarn", "-"}, startedAt, err)
		return err
	}

	preflight, err := s.Preflight(repoPath)
	if err != nil {
		s.emitCommandFailure(commandID, repoPath, "unstage_patch", []string{"apply", "--cached", "--reverse", "--unidiff-zero", "--whitespace=nowarn", "-"}, startedAt, err)
		return err
	}
	if validationErr := validatePatchText(preflight.RepoRoot, trimmedPatch); validationErr != nil {
		s.emitCommandFailure(commandID, preflight.RepoRoot, "unstage_patch", []string{"apply", "--cached", "--reverse", "--unidiff-zero", "--whitespace=nowarn", "-"}, startedAt, validationErr)
		return validationErr
	}

	if err := s.executeWrite(
		preflight.RepoRoot,
		commandID,
		"unstage_patch",
		[]string{"apply", "--cached", "--reverse", "--unidiff-zero", "--whitespace=nowarn", "-"},
		startedAt,
		defaultWriteTimeout,
		func(ctx context.Context, diag *commandDiagnosticState) error {
			_, errOut, exitCode, runErr := s.runWriteGitWithRetry(
				ctx,
				diag,
				trimmedPatch,
				"-C", preflight.RepoRoot,
				"apply",
				"--cached",
				"--reverse",
				"--unidiff-zero",
				"--whitespace=nowarn",
				"-",
			)
			if runErr != nil {
				return wrapWriteCommandError(
					CodePatchInvalid,
					"Falha ao remover patch do stage.",
					errOut,
					exitCode,
					runErr,
				)
			}
			return nil
		}); err != nil {
		return err
	}

	s.emitPostWriteReconciliation(preflight.RepoRoot, "unstage_patch", false)
	return nil
}

func (s *Service) AcceptOurs(repoPath string, filePath string, autoStage bool) error {
	preflight, cleanPath, commandID, startedAt, err := s.prepareWrite(repoPath, filePath, "accept_ours")
	if err != nil {
		s.emitCommandFailure(commandID, repoPath, "accept_ours", []string{"checkout", "--ours", "--", filePath}, startedAt, err)
		return err
	}

	if err := s.executeWrite(
		preflight.RepoRoot,
		commandID,
		"accept_ours",
		[]string{"checkout", "--ours", "--", cleanPath},
		startedAt,
		defaultWriteTimeout,
		func(ctx context.Context, diag *commandDiagnosticState) error {
			_, errOut, exitCode, runErr := s.runWriteGitWithRetry(
				ctx,
				diag,
				"",
				"-C", preflight.RepoRoot,
				"checkout",
				"--ours",
				"--",
				cleanPath,
			)
			if runErr != nil {
				return wrapWriteCommandError(
					CodeCommandFailed,
					"Falha ao aplicar versão local (ours).",
					errOut,
					exitCode,
					runErr,
				)
			}

			if !autoStage {
				return nil
			}

			_, stageErrOut, stageExitCode, stageErr := s.runWriteGitWithRetry(
				ctx,
				diag,
				"",
				"-C", preflight.RepoRoot,
				"add",
				"--",
				cleanPath,
			)
			if stageErr != nil {
				return wrapWriteCommandError(
					CodeCommandFailed,
					"Resolução aplicada, mas o auto-stage falhou.",
					stageErrOut,
					stageExitCode,
					stageErr,
				)
			}
			return nil
		}); err != nil {
		return err
	}

	s.emitPostWriteReconciliation(preflight.RepoRoot, "accept_ours", true)
	return nil
}

func (s *Service) AcceptTheirs(repoPath string, filePath string, autoStage bool) error {
	preflight, cleanPath, commandID, startedAt, err := s.prepareWrite(repoPath, filePath, "accept_theirs")
	if err != nil {
		s.emitCommandFailure(commandID, repoPath, "accept_theirs", []string{"checkout", "--theirs", "--", filePath}, startedAt, err)
		return err
	}

	if err := s.executeWrite(
		preflight.RepoRoot,
		commandID,
		"accept_theirs",
		[]string{"checkout", "--theirs", "--", cleanPath},
		startedAt,
		defaultWriteTimeout,
		func(ctx context.Context, diag *commandDiagnosticState) error {
			_, errOut, exitCode, runErr := s.runWriteGitWithRetry(
				ctx,
				diag,
				"",
				"-C", preflight.RepoRoot,
				"checkout",
				"--theirs",
				"--",
				cleanPath,
			)
			if runErr != nil {
				return wrapWriteCommandError(
					CodeCommandFailed,
					"Falha ao aplicar versão remota (theirs).",
					errOut,
					exitCode,
					runErr,
				)
			}

			if !autoStage {
				return nil
			}

			_, stageErrOut, stageExitCode, stageErr := s.runWriteGitWithRetry(
				ctx,
				diag,
				"",
				"-C", preflight.RepoRoot,
				"add",
				"--",
				cleanPath,
			)
			if stageErr != nil {
				return wrapWriteCommandError(
					CodeCommandFailed,
					"Resolução aplicada, mas o auto-stage falhou.",
					stageErrOut,
					stageExitCode,
					stageErr,
				)
			}
			return nil
		}); err != nil {
		return err
	}

	s.emitPostWriteReconciliation(preflight.RepoRoot, "accept_theirs", true)
	return nil
}

func (s *Service) OpenExternalMergeTool(repoPath string, filePath string) error {
	preflight, cleanPath, commandID, startedAt, err := s.prepareWrite(repoPath, filePath, "open_external_tool")
	if err != nil {
		s.emitCommandFailure(commandID, repoPath, "open_external_tool", []string{"mergetool", "--no-prompt", "--", filePath}, startedAt, err)
		return err
	}

	if err := s.executeWrite(
		preflight.RepoRoot,
		commandID,
		"open_external_tool",
		[]string{"mergetool", "--no-prompt", "--", cleanPath},
		startedAt,
		externalToolTimeout,
		func(ctx context.Context, diag *commandDiagnosticState) error {
			_, errOut, exitCode, runErr := s.runWriteGitWithRetry(
				ctx,
				diag,
				"",
				"-C", preflight.RepoRoot,
				"mergetool",
				"--no-prompt",
				"--",
				cleanPath,
			)
			if runErr != nil {
				return wrapWriteCommandError(
					CodeCommandFailed,
					"Falha ao abrir a ferramenta externa de merge.",
					errOut,
					exitCode,
					runErr,
				)
			}
			return nil
		}); err != nil {
		return err
	}

	s.emitPostWriteReconciliation(preflight.RepoRoot, "open_external_tool", true)
	return nil
}

func (s *Service) prepareWrite(repoPath string, filePath string, action string) (PreflightResult, string, string, time.Time, error) {
	commandID, startedAt := s.beginCommand(action)

	preflight, err := s.Preflight(repoPath)
	if err != nil {
		return PreflightResult{}, "", commandID, startedAt, err
	}

	cleanPath, pathErr := ensurePathWithinRepo(preflight.RepoRoot, filePath)
	if pathErr != nil {
		return PreflightResult{}, "", commandID, startedAt, pathErr
	}

	return preflight, cleanPath, commandID, startedAt, nil
}

func wrapWriteCommandError(code string, message string, stderr string, exitCode int, runErr error) error {
	if bindingErr := AsBindingError(runErr); bindingErr != nil {
		return bindingErr
	}
	return NewBindingError(
		code,
		message,
		formatCommandFailureDetails(stderr, exitCode, runErr),
	)
}

func (s *Service) beginCommand(action string) (string, time.Time) {
	startedAt := time.Now()
	seq := atomic.AddUint64(&s.commandSeq, 1)
	commandID := fmt.Sprintf("gpc_%d_%d", startedAt.UnixMilli(), seq)
	return commandID, startedAt
}

func (s *Service) emitCommandFailure(commandID string, repoPath string, action string, args []string, startedAt time.Time, err error) {
	s.emitCommandDiagnostic(
		newCommandDiagnosticState(commandID, repoPath, action, args, startedAt),
		commandStatusFailed,
		err,
	)
}

func (s *Service) emitStatusChanged(repoPath string) {
	s.emitStatusChangedWithContext(repoPath, "", "")
}

func (s *Service) emitStatusChangedWithContext(repoPath string, reason string, sourceEvent string) {
	payload := buildPanelEventPayload(repoPath, reason, sourceEvent)
	s.emit("gitpanel:status_changed", payload)
}

func (s *Service) emitConflictsChanged(repoPath string) {
	s.emitConflictsChangedWithContext(repoPath, "", "")
}

func (s *Service) emitConflictsChangedWithContext(repoPath string, reason string, sourceEvent string) {
	payload := buildPanelEventPayload(repoPath, reason, sourceEvent)
	s.emit("gitpanel:conflicts_changed", payload)
}

func (s *Service) emitHistoryInvalidated(repoPath string) {
	s.emitHistoryInvalidatedWithContext(repoPath, "", "")
}

func (s *Service) emitHistoryInvalidatedWithContext(repoPath string, reason string, sourceEvent string) {
	payload := buildPanelEventPayload(repoPath, reason, sourceEvent)
	s.emit("gitpanel:history_invalidated", payload)
}

func (s *Service) emitPostWriteReconciliation(repoPath string, action string, includeConflicts bool) {
	s.invalidateRepoCaches(repoPath)
	s.emitStatusChangedWithContext(repoPath, "post_write_reconcile", action)
	if includeConflicts {
		s.emitConflictsChangedWithContext(repoPath, "post_write_reconcile", action)
	}
}

func (s *Service) InvalidateRepoCache(repoPath string) {
	s.invalidateRepoCaches(repoPath)
}

func buildPanelEventPayload(repoPath string, reason string, sourceEvent string) map[string]string {
	payload := map[string]string{
		"repoPath": strings.TrimSpace(repoPath),
	}
	if trimmedReason := strings.TrimSpace(reason); trimmedReason != "" {
		payload["reason"] = trimmedReason
	}
	if trimmedSourceEvent := strings.TrimSpace(sourceEvent); trimmedSourceEvent != "" {
		payload["sourceEvent"] = trimmedSourceEvent
	}
	return payload
}

func parseHistoryCursor(cursor string) (string, error) {
	normalized := strings.TrimSpace(cursor)
	if normalized == "" {
		return "", nil
	}

	if len(normalized) < 7 || len(normalized) > 64 || !isHexToken(normalized) {
		return "", NewBindingError(
			CodeInvalidCursor,
			"Cursor de histórico inválido.",
			"O cursor deve ser um hash de commit válido.",
		)
	}

	return strings.ToLower(normalized), nil
}

func (s *Service) resolveHistorySkip(repoRoot string, cursorHash string, search string) (int, error) {
	if strings.TrimSpace(cursorHash) == "" {
		return 0, nil
	}

	args := []string{
		"-C", repoRoot,
		"rev-list",
		"--count",
	}

	trimmedSearch := strings.TrimSpace(search)
	if trimmedSearch != "" {
		args = append(args, "--grep="+trimmedSearch, "-i")
	}

	args = append(args, cursorHash+"..HEAD")

	out, errOut, exitCode, runErr := s.runGit(
		context.Background(),
		defaultReadTimeout,
		"",
		args...,
	)
	if runErr != nil {
		return 0, NewBindingError(
			CodeInvalidCursor,
			"Cursor de histórico inválido.",
			formatCommandFailureDetails(errOut, exitCode, runErr),
		)
	}

	count, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil || count < 0 {
		return 0, NewBindingError(
			CodeInvalidCursor,
			"Cursor de histórico inválido.",
			fmt.Sprintf("Não foi possível converter o contador do cursor: %q", strings.TrimSpace(out)),
		)
	}
	return count + 1, nil
}

func parseHistoryItems(raw string) []HistoryItemDTO {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	items := make([]HistoryItemDTO, 0, 64)
	records := strings.Split(raw, "\x1e")
	for _, record := range records {
		trimmedRecord := strings.Trim(record, "\r\n")
		if trimmedRecord == "" {
			continue
		}

		newlineIndex := strings.IndexByte(trimmedRecord, '\n')
		header := trimmedRecord
		numstatRaw := ""
		if newlineIndex >= 0 {
			header = strings.TrimSpace(trimmedRecord[:newlineIndex])
			numstatRaw = strings.TrimSpace(trimmedRecord[newlineIndex+1:])
		}
		if header == "" {
			continue
		}

		fields := strings.SplitN(header, "\x1f", 6)
		if len(fields) < 6 {
			continue
		}

		hash := strings.TrimSpace(fields[0])
		shortHash := strings.TrimSpace(fields[1])
		author := strings.TrimSpace(fields[2])
		authoredAt := strings.TrimSpace(fields[3])
		authorEmail := strings.TrimSpace(fields[4])
		subject := strings.TrimRight(fields[5], "\r\n")
		if hash == "" || shortHash == "" {
			continue
		}

		additions := 0
		deletions := 0
		changedFiles := 0
		if numstatRaw != "" {
			for _, line := range strings.Split(numstatRaw, "\n") {
				parsed, parsedAdditions, parsedDeletions := parseHistoryNumstatLine(line)
				if !parsed {
					continue
				}
				changedFiles++
				additions += parsedAdditions
				deletions += parsedDeletions
			}
		}

		items = append(items, HistoryItemDTO{
			Hash:         hash,
			ShortHash:    shortHash,
			Author:       author,
			AuthoredAt:   authoredAt,
			Subject:      subject,
			Additions:    additions,
			Deletions:    deletions,
			ChangedFiles: changedFiles,
			AuthorEmail:  authorEmail,
		})
	}
	return items
}

func parseHistoryNumstatLine(rawLine string) (bool, int, int) {
	line := strings.TrimSpace(rawLine)
	if line == "" {
		return false, 0, 0
	}

	parts := strings.SplitN(line, "\t", 3)
	if len(parts) < 3 {
		return false, 0, 0
	}

	additions := 0
	if parts[0] != "-" {
		if parsed, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil && parsed > 0 {
			additions = parsed
		}
	}

	deletions := 0
	if parts[1] != "-" {
		if parsed, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && parsed > 0 {
			deletions = parsed
		}
	}

	return true, additions, deletions
}

func parseDiffFiles(raw string) []DiffFileDTO {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	lines := strings.Split(raw, "\n")
	files := make([]DiffFileDTO, 0, 8)

	var currentFile *DiffFileDTO
	var currentHunk *DiffHunkDTO
	oldLine := 0
	newLine := 0

	flushHunk := func() {
		if currentFile == nil || currentHunk == nil {
			return
		}
		currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
		currentHunk = nil
	}

	flushFile := func() {
		if currentFile == nil {
			return
		}
		flushHunk()
		if strings.TrimSpace(currentFile.Status) == "" {
			currentFile.Status = "modified"
		}
		if strings.TrimSpace(currentFile.Path) == "" {
			currentFile.Path = strings.TrimSpace(currentFile.OldPath)
		}
		files = append(files, *currentFile)
		currentFile = nil
	}

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")

		if strings.HasPrefix(line, "diff --git ") {
			flushFile()

			oldPath := ""
			newPath := ""
			if leftRaw, rightRaw, ok := parseDiffGitPathPair(strings.TrimPrefix(line, "diff --git ")); ok {
				if decoded, decodeOk := decodePatchPathToken(leftRaw); decodeOk {
					oldPath = normalizeDiffFilePathToken(decoded)
				}
				if decoded, decodeOk := decodePatchPathToken(rightRaw); decodeOk {
					newPath = normalizeDiffFilePathToken(decoded)
				}
			}

			currentPath := newPath
			if strings.TrimSpace(currentPath) == "" {
				currentPath = oldPath
			}

			currentFile = &DiffFileDTO{
				Path:    strings.TrimSpace(currentPath),
				OldPath: strings.TrimSpace(oldPath),
				Status:  "modified",
				Hunks:   make([]DiffHunkDTO, 0, 4),
			}

			if oldPath == "" && newPath != "" {
				currentFile.Status = "added"
			}
			if newPath == "" && oldPath != "" {
				currentFile.Status = "deleted"
			}
			continue
		}

		if currentFile == nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "new file mode "):
			currentFile.Status = "added"
			continue
		case strings.HasPrefix(line, "deleted file mode "):
			currentFile.Status = "deleted"
			continue
		case strings.HasPrefix(line, "rename from "):
			if decoded, ok := decodePatchPathToken(strings.TrimSpace(strings.TrimPrefix(line, "rename from "))); ok {
				currentFile.OldPath = decoded
				if strings.TrimSpace(currentFile.Path) == "" {
					currentFile.Path = decoded
				}
			}
			currentFile.Status = "renamed"
			continue
		case strings.HasPrefix(line, "rename to "):
			if decoded, ok := decodePatchPathToken(strings.TrimSpace(strings.TrimPrefix(line, "rename to "))); ok {
				currentFile.Path = decoded
			}
			currentFile.Status = "renamed"
			continue
		case strings.HasPrefix(line, "copy from "):
			if decoded, ok := decodePatchPathToken(strings.TrimSpace(strings.TrimPrefix(line, "copy from "))); ok {
				currentFile.OldPath = decoded
			}
			currentFile.Status = "copied"
			continue
		case strings.HasPrefix(line, "copy to "):
			if decoded, ok := decodePatchPathToken(strings.TrimSpace(strings.TrimPrefix(line, "copy to "))); ok {
				currentFile.Path = decoded
			}
			currentFile.Status = "copied"
			continue
		case strings.HasPrefix(line, "--- "):
			token := strings.TrimSpace(strings.TrimPrefix(line, "--- "))
			if token == "/dev/null" {
				currentFile.Status = "added"
				continue
			}
			if decoded, ok := decodePatchPathToken(token); ok {
				currentFile.OldPath = decoded
				if strings.TrimSpace(currentFile.Path) == "" {
					currentFile.Path = decoded
				}
			}
			continue
		case strings.HasPrefix(line, "+++ "):
			token := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			if token == "/dev/null" {
				currentFile.Status = "deleted"
				continue
			}
			if decoded, ok := decodePatchPathToken(token); ok {
				currentFile.Path = decoded
			}
			continue
		case strings.HasPrefix(line, "Binary files ") || strings.HasPrefix(line, "GIT binary patch"):
			flushHunk()
			currentFile.IsBinary = true
			continue
		}

		if matches := diffHunkHeaderRegex.FindStringSubmatch(line); matches != nil {
			flushHunk()

			oldStart := parseDiffInt(matches[1], 0)
			oldLines := parseDiffOptionalCount(matches[2])
			newStart := parseDiffInt(matches[3], 0)
			newLines := parseDiffOptionalCount(matches[4])

			currentHunk = &DiffHunkDTO{
				Header:   strings.TrimSpace(matches[5]),
				OldStart: oldStart,
				OldLines: oldLines,
				NewStart: newStart,
				NewLines: newLines,
				Lines:    make([]DiffLineDTO, 0, oldLines+newLines+4),
			}
			oldLine = oldStart
			newLine = newStart
			continue
		}

		if currentHunk == nil {
			continue
		}

		if strings.HasPrefix(line, "\\ No newline at end of file") {
			currentHunk.Lines = append(currentHunk.Lines, DiffLineDTO{
				Type:    "meta",
				Content: line,
			})
			continue
		}

		if len(line) == 0 {
			oldValue := oldLine
			newValue := newLine
			currentHunk.Lines = append(currentHunk.Lines, DiffLineDTO{
				Type:    "context",
				Content: "",
				OldLine: &oldValue,
				NewLine: &newValue,
			})
			oldLine++
			newLine++
			continue
		}

		switch line[0] {
		case '+':
			newValue := newLine
			currentHunk.Lines = append(currentHunk.Lines, DiffLineDTO{
				Type:    "add",
				Content: line[1:],
				NewLine: &newValue,
			})
			currentFile.Additions++
			newLine++
		case '-':
			oldValue := oldLine
			currentHunk.Lines = append(currentHunk.Lines, DiffLineDTO{
				Type:    "delete",
				Content: line[1:],
				OldLine: &oldValue,
			})
			currentFile.Deletions++
			oldLine++
		case ' ':
			oldValue := oldLine
			newValue := newLine
			currentHunk.Lines = append(currentHunk.Lines, DiffLineDTO{
				Type:    "context",
				Content: line[1:],
				OldLine: &oldValue,
				NewLine: &newValue,
			})
			oldLine++
			newLine++
		default:
			currentHunk.Lines = append(currentHunk.Lines, DiffLineDTO{
				Type:    "meta",
				Content: line,
			})
		}
	}

	flushFile()
	return files
}

func parseDiffOptionalCount(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 1
	}
	return parseDiffInt(trimmed, 1)
}

func parseDiffInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func normalizeDiffFilePathToken(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "dev/null" {
		return ""
	}
	return trimmed
}

func isTimeoutBindingError(err error) bool {
	if bindingErr := AsBindingError(err); bindingErr != nil {
		return bindingErr.Code == CodeTimeout
	}
	return false
}

func statRepoFileSize(repoRoot string, filePath string) (int64, bool) {
	root := filepath.Clean(strings.TrimSpace(repoRoot))
	normalized := strings.TrimSpace(filePath)
	if root == "" || normalized == "" {
		return 0, false
	}

	absPath := filepath.Join(root, filepath.FromSlash(normalized))
	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		return 0, false
	}
	return info.Size(), true
}

func buildLargeDiffFallback(mode string, filePath string, sizeBytes int64) DiffDTO {
	raw := fmt.Sprintf(
		"Preview desativado automaticamente: o arquivo %q tem %d bytes e excede o limite de %d bytes.",
		filePath,
		sizeBytes,
		maxDiffPreviewBytes,
	)
	return DiffDTO{
		Mode:        mode,
		FilePath:    filePath,
		Raw:         raw,
		IsBinary:    false,
		IsTruncated: true,
	}
}

func buildTimeoutDiffFallback(mode string, filePath string) DiffDTO {
	target := strings.TrimSpace(filePath)
	if target == "" {
		target = "repositório"
	}
	return DiffDTO{
		Mode:        mode,
		FilePath:    filePath,
		Raw:         fmt.Sprintf("Diff parcial indisponível no momento: a leitura de %q excedeu o timeout. Ajuste filtros/contexto e tente novamente.", target),
		IsBinary:    false,
		IsTruncated: true,
	}
}

func buildHistoryCacheKey(repoRoot string, cursorHash string, limit int, search string) string {
	return strings.Join([]string{
		filepath.Clean(strings.TrimSpace(repoRoot)),
		strings.ToLower(strings.TrimSpace(cursorHash)),
		strconv.Itoa(limit),
		strings.ToLower(strings.TrimSpace(search)),
	}, "\x1f")
}

func buildDiffCacheKey(repoRoot string, filePath string, mode string, contextLines int) string {
	return strings.Join([]string{
		filepath.Clean(strings.TrimSpace(repoRoot)),
		strings.TrimSpace(filePath),
		strings.ToLower(strings.TrimSpace(mode)),
		strconv.Itoa(contextLines),
	}, "\x1f")
}

func (s *Service) getCachedPreflight(repoPath string) (PreflightResult, bool) {
	s.cacheMu.RLock()
	entry, ok := s.preflightCache[filepath.Clean(strings.TrimSpace(repoPath))]
	s.cacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return PreflightResult{}, false
	}
	return entry.value, true
}

func (s *Service) setCachedPreflight(repoPath string, value PreflightResult) {
	key := filepath.Clean(strings.TrimSpace(repoPath))
	if key == "" || key == "." {
		return
	}

	s.cacheMu.Lock()
	s.preflightCache[key] = preflightCacheEntry{
		value:     value,
		expiresAt: time.Now().Add(preflightCacheTTL),
	}
	s.cacheMu.Unlock()
}

func (s *Service) getCachedStatus(repoRoot string) (StatusDTO, bool) {
	key := filepath.Clean(strings.TrimSpace(repoRoot))
	s.cacheMu.RLock()
	entry, ok := s.statusCache[key]
	s.cacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return StatusDTO{}, false
	}
	return entry.value, true
}

func (s *Service) setCachedStatus(repoRoot string, value StatusDTO) {
	key := filepath.Clean(strings.TrimSpace(repoRoot))
	if key == "" || key == "." {
		return
	}

	s.cacheMu.Lock()
	s.statusCache[key] = statusCacheEntry{
		value:     value,
		expiresAt: time.Now().Add(statusCacheTTL),
	}
	s.cacheMu.Unlock()
}

func (s *Service) getCachedHistory(cacheKey string) (HistoryPageDTO, bool) {
	key := strings.TrimSpace(cacheKey)
	s.cacheMu.RLock()
	entry, ok := s.historyCache[key]
	s.cacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return HistoryPageDTO{}, false
	}
	return entry.value, true
}

func (s *Service) setCachedHistory(cacheKey string, value HistoryPageDTO) {
	key := strings.TrimSpace(cacheKey)
	if key == "" {
		return
	}

	s.cacheMu.Lock()
	s.historyCache[key] = historyCacheEntry{
		value:     value,
		expiresAt: time.Now().Add(historyCacheTTL),
	}
	s.cacheMu.Unlock()
}

func (s *Service) getCachedDiff(cacheKey string) (DiffDTO, bool) {
	key := strings.TrimSpace(cacheKey)
	s.cacheMu.RLock()
	entry, ok := s.diffCache[key]
	s.cacheMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return DiffDTO{}, false
	}
	return entry.value, true
}

func (s *Service) setCachedDiff(cacheKey string, value DiffDTO) {
	key := strings.TrimSpace(cacheKey)
	if key == "" {
		return
	}

	s.cacheMu.Lock()
	s.diffCache[key] = diffCacheEntry{
		value:     value,
		expiresAt: time.Now().Add(diffCacheTTL),
	}
	s.cacheMu.Unlock()
}

func (s *Service) invalidateRepoCaches(repoPath string) {
	normalized := filepath.Clean(strings.TrimSpace(repoPath))

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	if normalized == "" || normalized == "." {
		for key := range s.preflightCache {
			delete(s.preflightCache, key)
		}
		for key := range s.statusCache {
			delete(s.statusCache, key)
		}
		for key := range s.historyCache {
			delete(s.historyCache, key)
		}
		for key := range s.diffCache {
			delete(s.diffCache, key)
		}
		return
	}

	for key := range s.preflightCache {
		if key == normalized {
			delete(s.preflightCache, key)
		}
	}
	delete(s.statusCache, normalized)

	historyPrefix := normalized + "\x1f"
	for key := range s.historyCache {
		if strings.HasPrefix(key, historyPrefix) {
			delete(s.historyCache, key)
		}
	}

	diffPrefix := normalized + "\x1f"
	for key := range s.diffCache {
		if strings.HasPrefix(key, diffPrefix) {
			delete(s.diffCache, key)
		}
	}
}

func isHexToken(value string) bool {
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func parsePorcelainStatus(raw string) StatusDTO {
	if strings.IndexByte(raw, 0) >= 0 {
		return parsePorcelainStatusZ(raw)
	}
	return parsePorcelainStatusText(raw)
}

func parsePorcelainStatusText(raw string) StatusDTO {
	status := StatusDTO{
		Staged:     make([]FileChangeDTO, 0),
		Unstaged:   make([]FileChangeDTO, 0),
		Conflicted: make([]ConflictFileDTO, 0),
	}

	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}

		if strings.HasPrefix(line, "## ") {
			branch, ahead, behind := parseBranchHeader(line)
			status.Branch = branch
			status.Ahead = ahead
			status.Behind = behind
			continue
		}

		if len(line) < 3 {
			continue
		}

		xy := line[:2]
		pathPart := strings.TrimSpace(line[3:])
		if pathPart == "" {
			continue
		}
		path, originalPath := parsePorcelainPathPair(pathPart)
		appendStatusEntry(&status, xy, path, originalPath)
	}

	return status
}

func parsePorcelainStatusZ(raw string) StatusDTO {
	status := StatusDTO{
		Staged:     make([]FileChangeDTO, 0),
		Unstaged:   make([]FileChangeDTO, 0),
		Conflicted: make([]ConflictFileDTO, 0),
	}

	records := strings.Split(raw, "\x00")
	for i := 0; i < len(records); i++ {
		record := strings.TrimRight(records[i], "\r\n")
		if strings.TrimSpace(record) == "" {
			continue
		}

		if strings.HasPrefix(record, "## ") {
			branch, ahead, behind := parseBranchHeader(record)
			status.Branch = branch
			status.Ahead = ahead
			status.Behind = behind
			continue
		}

		if len(record) < 3 {
			continue
		}

		xy := record[:2]
		path := record[3:]
		if strings.TrimSpace(path) == "" {
			continue
		}

		originalPath := ""
		if porcelainEntryHasSecondaryPath(xy) && i+1 < len(records) {
			originalPath = records[i+1]
			i++
		}

		appendStatusEntry(&status, xy, path, originalPath)
	}

	return status
}

func appendStatusEntry(status *StatusDTO, xy string, path string, originalPath string) {
	if status == nil || len(xy) < 2 {
		return
	}

	path = strings.TrimSpace(path)
	originalPath = strings.TrimSpace(originalPath)
	if path == "" {
		return
	}

	change := FileChangeDTO{
		Path:         path,
		OriginalPath: originalPath,
		Status:       xy,
		Added:        0,
		Removed:      0,
	}

	if _, isConflict := conflictStatuses[xy]; isConflict {
		status.Conflicted = append(status.Conflicted, ConflictFileDTO{
			Path:   path,
			Status: xy,
		})
		return
	}

	if xy == "??" {
		status.Unstaged = append(status.Unstaged, change)
		return
	}

	if xy[0] != ' ' {
		status.Staged = append(status.Staged, change)
	}
	if xy[1] != ' ' {
		status.Unstaged = append(status.Unstaged, change)
	}
}

func porcelainEntryHasSecondaryPath(xy string) bool {
	if len(xy) < 2 {
		return false
	}
	return xy[0] == 'R' || xy[0] == 'C' || xy[1] == 'R' || xy[1] == 'C'
}

func parseBranchHeader(line string) (string, int, int) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(line, "## "))
	if trimmed == "" {
		return "", 0, 0
	}

	branchSection := trimmed
	metaSection := ""

	if idx := strings.Index(trimmed, "["); idx >= 0 {
		branchSection = strings.TrimSpace(trimmed[:idx])
		metaSection = strings.TrimSuffix(strings.TrimSpace(trimmed[idx:]), "]")
		metaSection = strings.TrimPrefix(metaSection, "[")
	}

	branch := branchSection
	if idx := strings.Index(branchSection, "..."); idx >= 0 {
		branch = strings.TrimSpace(branchSection[:idx])
	}

	ahead := 0
	behind := 0
	if metaSection != "" {
		parts := strings.Split(metaSection, ",")
		for _, part := range parts {
			token := strings.TrimSpace(part)
			if strings.HasPrefix(token, "ahead ") {
				if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, "ahead "))); err == nil {
					ahead = value
				}
			}
			if strings.HasPrefix(token, "behind ") {
				if value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(token, "behind "))); err == nil {
					behind = value
				}
			}
		}
	}

	return branch, ahead, behind
}

func parsePorcelainPath(raw string) string {
	path, _ := parsePorcelainPathPair(raw)
	return path
}

func parsePorcelainPathPair(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if idx := strings.Index(trimmed, " -> "); idx >= 0 {
		oldPath := strings.TrimSpace(trimmed[:idx])
		newPath := strings.TrimSpace(trimmed[idx+4:])
		return newPath, oldPath
	}
	return trimmed, ""
}

func ensurePathWithinRepo(repoRoot string, filePath string) (string, error) {
	trimmed := strings.TrimSpace(filePath)
	if trimmed == "" {
		return "", NewBindingError(
			CodeInvalidPath,
			"Caminho de arquivo obrigatório.",
			"Informe um caminho relativo válido ao repositório.",
		)
	}

	if strings.ContainsRune(trimmed, '\x00') {
		return "", NewBindingError(
			CodeInvalidPath,
			"Caminho de arquivo inválido.",
			"Caracter nulo não é permitido no caminho.",
		)
	}

	normalizedInput := strings.ReplaceAll(filepath.ToSlash(trimmed), "\\", "/")
	normalized := path.Clean(normalizedInput)
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") || strings.HasPrefix(normalized, "/") {
		return "", NewBindingError(
			CodeInvalidPath,
			"Caminho de arquivo inválido.",
			"Use apenas caminhos relativos dentro do repositório.",
		)
	}
	if filepath.IsAbs(trimmed) {
		return "", NewBindingError(
			CodeInvalidPath,
			"Caminho de arquivo inválido.",
			"Use apenas caminhos relativos dentro do repositório.",
		)
	}

	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", NewBindingError(
			CodeRepoNotFound,
			"Falha ao validar escopo do repositório.",
			err.Error(),
		)
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(normalized)))
	if err != nil {
		return "", NewBindingError(
			CodeInvalidPath,
			"Caminho de arquivo inválido.",
			err.Error(),
		)
	}

	relPath, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return "", NewBindingError(
			CodeInvalidPath,
			"Caminho de arquivo inválido.",
			err.Error(),
		)
	}
	if relPath == "." || relPath == ".." || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
		return "", NewBindingError(
			CodeRepoOutOfScope,
			"Caminho fora do escopo permitido.",
			normalized,
		)
	}

	return normalized, nil
}

func formatCommandFailureDetails(stderr string, exitCode int, err error) string {
	parts := make([]string, 0, 3)

	trimmedStderr := strings.TrimSpace(stderr)
	if trimmedStderr != "" {
		parts = append(parts, trimmedStderr)
	}
	if exitCode != 0 {
		parts = append(parts, fmt.Sprintf("exit_code=%d", exitCode))
	}
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		parts = append(parts, err.Error())
	}

	return strings.Join(parts, " | ")
}

func runGitWithInput(ctx context.Context, timeout time.Duration, stdin string, args ...string) (string, string, int, error) {
	if timeout <= 0 {
		timeout = defaultReadTimeout
	}

	childCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(childCtx, "git", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if childCtx.Err() == context.DeadlineExceeded {
			return stdout.String(), stderr.String(), exitCode, NewBindingError(
				CodeTimeout,
				"Comando Git excedeu o tempo limite.",
				formatCommandFailureDetails(stderr.String(), exitCode, runErr),
			)
		}
		return stdout.String(), stderr.String(), exitCode, runErr
	}

	return stdout.String(), stderr.String(), exitCode, nil
}
