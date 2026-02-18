package ai

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const (
	roleBudgetTokens     = 200
	appStateBudgetTokens = 100
	prDiffBudgetTokens   = 2000
	termBudgetTokens     = 500
	userBudgetTokens     = 1000
)

func (s *Service) buildContext(sessionID string) SessionState {
	s.mu.RLock()
	state := s.sessionState[sessionID]
	term := s.terminalState[sessionID]
	s.mu.RUnlock()

	// Fallback de projeto baseado no path.
	if state.ProjectName == "" && state.ProjectPath != "" {
		state.ProjectName = filepath.Base(state.ProjectPath)
	}
	if state.ProjectName == "" {
		state.ProjectName = "Unknown Project"
	}

	// Enriquecer com terminal.
	if term != nil {
		state.LastCommand = term.lastCommand
		state.LastStdout = strings.Join(term.stdoutLines, "\n")
		state.LastStderr = strings.Join(term.stderrLines, "\n")
	}

	// Enriquecer com GitHub cacheado (quando disponível).
	if state.ActivePR != nil && s.githubCache != nil {
		if state.ActivePR.Owner != "" && state.ActivePR.Repo != "" && state.ActivePR.Number > 0 {
			if pr, ok := s.githubCache.GetCachedPullRequest(state.ActivePR.Owner, state.ActivePR.Repo, state.ActivePR.Number); ok && pr != nil {
				if state.ActivePR.Title == "" {
					state.ActivePR.Title = pr.Title
				}
				if state.ActivePR.Body == "" {
					state.ActivePR.Body = pr.Body
				}
				if state.ActivePR.Author == "" {
					state.ActivePR.Author = pr.Author.Login
				}
				if state.ActivePR.Branch == "" {
					state.ActivePR.Branch = pr.HeadBranch
				}
			}
		}

		if state.ActivePR.Diff != "" {
			state.ActivePR.Diff = s.truncateDiff(state.ActivePR.Diff, prDiffBudgetTokens)
		}
	}

	state.LastStdout = truncateByTokens(state.LastStdout, termBudgetTokens/2)
	state.LastStderr = truncateByTokens(state.LastStderr, termBudgetTokens/2)

	return state
}

func (s *Service) assemblePrompt(state SessionState, userMessage string) string {
	project := fallback(state.ProjectName, "Unknown Project")
	branch := fallback(state.CurrentBranch, "unknown")
	currentFile := fallback(state.CurrentFile, "(nenhum)")
	lastCmd := fallback(state.LastCommand, "(nenhum)")
	lastErr := fallback(state.LastStderr, "(sem erros)")

	role := strings.TrimSpace(`
[ROLE]
Você é um Arquiteto de Software Sênior assistindo um desenvolvedor dentro de um terminal.
Seja conciso, técnico e direto. Evite markdown complexo que quebre em terminais TTY.
`)

	role = truncateByTokens(role, roleBudgetTokens)

	appState := fmt.Sprintf(`[CURRENT APP STATE]
- Projeto: %s
- Branch Atual: %s
- Arquivo Aberto (Visualizador): %s
`, project, branch, currentFile)
	appState = truncateByTokens(appState, appStateBudgetTokens)

	var ghCtx string
	if state.ActivePR != nil {
		diff := truncateByTokens(state.ActivePR.Diff, prDiffBudgetTokens)
		ghCtx = fmt.Sprintf(`[GITHUB CONTEXT — ACTIVE PR]
- PR ID: #%d
- Título: %s
- Diff (Resumo):
%s
`, state.ActivePR.Number, fallback(state.ActivePR.Title, "(sem título)"), fallback(diff, "(diff indisponível)"))
	} else {
		ghCtx = "[GITHUB CONTEXT — ACTIVE PR]\n(Nenhum PR ativo na lateral)\n"
	}

	terminalCtx := fmt.Sprintf(`[TERMINAL HISTORY]
- Último comando: %s
- Saída de erro: %s
`, lastCmd, lastErr)
	terminalCtx = truncateByTokens(terminalCtx, termBudgetTokens)

	user := truncateByTokens(strings.TrimSpace(userMessage), userBudgetTokens)

	return strings.TrimSpace(fmt.Sprintf(`
--- SYSTEM CONTEXT (INJECTED) ---
%s

%s

%s

%s
---------------------------------

[USER INPUT]
%s
`, role, appState, ghCtx, terminalCtx, user))
}

func (s *Service) truncateDiff(diff string, maxTokens int) string {
	files := parseDiffFiles(diff)
	if len(files) == 0 {
		return truncateByTokens(diff, maxTokens)
	}

	priority := map[string]int{
		".go": 1, ".ts": 1, ".tsx": 1, ".js": 1, ".jsx": 1,
		".py": 2, ".rs": 2, ".java": 2,
		".css": 3, ".html": 3,
		".json": 4, ".yaml": 4, ".yml": 4,
	}

	ignore := []string{
		"package-lock.json", "yarn.lock", "go.sum",
		"*.min.js", "*.min.css", "*.map",
	}

	sort.Slice(files, func(i, j int) bool {
		pi := filePriority(files[i].name, priority)
		pj := filePriority(files[j].name, priority)
		if pi == pj {
			return files[i].name < files[j].name
		}
		return pi < pj
	})

	var result strings.Builder
	tokens := 0

	for _, f := range files {
		if shouldIgnore(f.name, ignore) {
			continue
		}
		fileTokens := estimateTokens(f.content)
		if fileTokens == 0 {
			continue
		}
		if tokens+fileTokens > maxTokens {
			remaining := maxTokens - tokens
			if remaining <= 0 {
				break
			}
			result.WriteString(truncateByTokens(f.content, remaining))
			tokens = maxTokens
			break
		}
		result.WriteString(f.content)
		if !strings.HasSuffix(f.content, "\n") {
			result.WriteString("\n")
		}
		tokens += fileTokens
	}

	return strings.TrimSpace(result.String())
}

func (s *Service) truncateToFit(prompt string, maxTokens int) string {
	return truncateByTokens(prompt, maxTokens)
}

type diffFileChunk struct {
	name    string
	content string
}

func parseDiffFiles(diff string) []diffFileChunk {
	if strings.TrimSpace(diff) == "" {
		return nil
	}

	lines := strings.Split(diff, "\n")
	chunks := make([]diffFileChunk, 0, 16)
	var currentName string
	var currentContent strings.Builder

	flush := func() {
		if currentContent.Len() == 0 {
			return
		}
		chunks = append(chunks, diffFileChunk{
			name:    currentName,
			content: currentContent.String(),
		})
		currentContent.Reset()
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			currentName = parseDiffFilename(line)
		}
		currentContent.WriteString(line)
		currentContent.WriteByte('\n')
	}
	flush()

	// Se não havia marcador "diff --git", tratar tudo como bloco único.
	if len(chunks) == 1 && chunks[0].name == "" {
		chunks[0].name = "unknown.diff"
	}
	return chunks
}

func parseDiffFilename(header string) string {
	// Ex: diff --git a/internal/ai/service.go b/internal/ai/service.go
	parts := strings.Split(header, " ")
	if len(parts) < 4 {
		return ""
	}
	raw := strings.TrimPrefix(parts[3], "b/")
	return strings.TrimSpace(raw)
}

func filePriority(name string, priorities map[string]int) int {
	ext := strings.ToLower(filepath.Ext(name))
	if p, ok := priorities[ext]; ok {
		return p
	}
	return 5
}

func shouldIgnore(name string, patterns []string) bool {
	lower := strings.ToLower(name)
	base := strings.ToLower(filepath.Base(name))

	for _, p := range patterns {
		pattern := strings.ToLower(p)
		switch {
		case strings.HasPrefix(pattern, "*"):
			if ok, _ := filepath.Match(pattern, base); ok {
				return true
			}
			if ok, _ := filepath.Match(pattern, lower); ok {
				return true
			}
		default:
			if base == pattern || lower == pattern {
				return true
			}
		}
	}
	return false
}

func fallback(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func truncateByTokens(text string, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}
	if estimateTokens(text) <= maxTokens {
		return text
	}

	// Aproximação: 1 token ~ 4 chars.
	maxChars := maxTokens * 4
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}

	const suffix = "\n...[TRUNCATED]"
	suffixRunes := []rune(suffix)
	if len(suffixRunes) >= maxChars {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-len(suffixRunes)]) + suffix
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	runes := len([]rune(text))
	tokens := runes / 4
	if tokens == 0 {
		return 1
	}
	return tokens
}
