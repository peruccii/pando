package gitpanel

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	commandStatusQueued    = "queued"
	commandStatusStarted   = "started"
	commandStatusRetried   = "retried"
	commandStatusSucceeded = "succeeded"
	commandStatusFailed    = "failed"

	maxDiagnosticStderrLength = 1200
)

type commandDiagnosticState struct {
	commandID string
	repoPath  string
	action    string
	startedAt time.Time
	baseArgs  []string

	mu           sync.Mutex
	lastArgs     []string
	lastStderr   string
	lastExitCode int
	lastAttempt  int
}

type commandDiagnosticSnapshot struct {
	args     []string
	stderr   string
	exitCode int
	attempt  int
}

func newCommandDiagnosticState(commandID string, repoPath string, action string, args []string, startedAt time.Time) *commandDiagnosticState {
	base := cloneStringSlice(args)
	return &commandDiagnosticState{
		commandID: strings.TrimSpace(commandID),
		repoPath:  strings.TrimSpace(repoPath),
		action:    strings.TrimSpace(action),
		startedAt: startedAt,
		baseArgs:  base,
		lastArgs:  cloneStringSlice(base),
	}
}

func (d *commandDiagnosticState) recordAttempt(args []string, stderr string, exitCode int, attempt int) {
	if d == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if len(args) > 0 {
		d.lastArgs = cloneStringSlice(args)
	}
	d.lastStderr = strings.TrimSpace(stderr)
	d.lastExitCode = exitCode
	if attempt > d.lastAttempt {
		d.lastAttempt = attempt
	}
}

func (d *commandDiagnosticState) snapshot() commandDiagnosticSnapshot {
	if d == nil {
		return commandDiagnosticSnapshot{}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	args := d.lastArgs
	if len(args) == 0 {
		args = d.baseArgs
	}

	return commandDiagnosticSnapshot{
		args:     cloneStringSlice(args),
		stderr:   d.lastStderr,
		exitCode: d.lastExitCode,
		attempt:  d.lastAttempt,
	}
}

func (s *Service) emitCommandDiagnostic(diag *commandDiagnosticState, status string, err error) {
	if diag == nil {
		return
	}

	snapshot := diag.snapshot()
	result := CommandResultDTO{
		CommandID:       diag.commandID,
		RepoPath:        strings.TrimSpace(diag.repoPath),
		Action:          diag.action,
		Args:            sanitizeDiagnosticArgs(diag.repoPath, snapshot.args),
		DurationMs:      time.Since(diag.startedAt).Milliseconds(),
		ExitCode:        snapshot.exitCode,
		StderrSanitized: sanitizeDiagnosticStderr(diag.repoPath, snapshot.stderr),
		Status:          strings.TrimSpace(status),
		Attempt:         snapshot.attempt,
	}

	if err != nil {
		if bindingErr := AsBindingError(err); bindingErr != nil {
			result.Error = bindingErr.Error()
		} else {
			result.Error = strings.TrimSpace(err.Error())
		}
	}

	s.emit("gitpanel:command_result", result)
}

func sanitizeDiagnosticArgs(repoPath string, args []string) []string {
	if len(args) == 0 {
		return nil
	}

	repoAbs := filepath.Clean(strings.TrimSpace(repoPath))
	homeDir, _ := os.UserHomeDir()
	homeAbs := filepath.Clean(strings.TrimSpace(homeDir))

	sanitized := make([]string, 0, len(args))
	for _, arg := range args {
		token := strings.TrimSpace(arg)
		if token == "" {
			continue
		}

		if token == repoAbs {
			sanitized = append(sanitized, "<repo>")
			continue
		}

		if repoAbs != "" && strings.Contains(token, repoAbs) {
			token = strings.ReplaceAll(token, repoAbs, "<repo>")
		}
		if repoAbs != "" {
			repoAbsSlash := filepath.ToSlash(repoAbs)
			if repoAbsSlash != repoAbs && strings.Contains(token, repoAbsSlash) {
				token = strings.ReplaceAll(token, repoAbsSlash, "<repo>")
			}
		}

		if filepath.IsAbs(token) {
			if rel, ok := relativizePathWithinRepo(repoAbs, token); ok {
				sanitized = append(sanitized, "<repo>/"+rel)
				continue
			}
			if homeAbs != "" {
				if rel, ok := relativizePathWithinRepo(homeAbs, token); ok {
					sanitized = append(sanitized, "~/"+rel)
					continue
				}
			}
			sanitized = append(sanitized, "<abs-path>")
			continue
		}

		if homeAbs != "" && strings.Contains(token, homeAbs) {
			token = strings.ReplaceAll(token, homeAbs, "~")
		}
		token = strings.ReplaceAll(token, "\n", " ")
		token = strings.ReplaceAll(token, "\r", " ")
		sanitized = append(sanitized, strings.TrimSpace(token))
	}

	return sanitized
}

func sanitizeDiagnosticStderr(repoPath string, stderr string) string {
	trimmed := strings.TrimSpace(stderr)
	if trimmed == "" {
		return ""
	}

	repoAbs := filepath.Clean(strings.TrimSpace(repoPath))
	if repoAbs != "" {
		trimmed = strings.ReplaceAll(trimmed, repoAbs, "<repo>")
		repoAbsSlash := filepath.ToSlash(repoAbs)
		if repoAbsSlash != repoAbs {
			trimmed = strings.ReplaceAll(trimmed, repoAbsSlash, "<repo>")
		}
	}

	homeDir, _ := os.UserHomeDir()
	homeAbs := filepath.Clean(strings.TrimSpace(homeDir))
	if homeAbs != "" {
		trimmed = strings.ReplaceAll(trimmed, homeAbs, "~")
		homeAbsSlash := filepath.ToSlash(homeAbs)
		if homeAbsSlash != homeAbs {
			trimmed = strings.ReplaceAll(trimmed, homeAbsSlash, "~")
		}
	}

	lines := strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n")
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		clean := strings.TrimSpace(line)
		if clean == "" {
			continue
		}
		clean = strings.Map(func(r rune) rune {
			if r < 0x20 && r != '\t' {
				return -1
			}
			return r
		}, clean)
		if clean != "" {
			parts = append(parts, clean)
		}
	}

	sanitized := strings.Join(parts, " | ")
	if len(sanitized) <= maxDiagnosticStderrLength {
		return sanitized
	}
	return strings.TrimSpace(sanitized[:maxDiagnosticStderrLength]) + "... (truncated)"
}

func relativizePathWithinRepo(base string, candidate string) (string, bool) {
	if strings.TrimSpace(base) == "" || strings.TrimSpace(candidate) == "" {
		return "", false
	}

	rel, err := filepath.Rel(base, candidate)
	if err != nil {
		return "", false
	}
	if rel == "." {
		return ".", true
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return "", false
	}

	return filepath.ToSlash(strings.TrimSpace(rel)), true
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
