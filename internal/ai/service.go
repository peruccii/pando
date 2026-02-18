package ai

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	gh "orch/internal/github"
)

const (
	defaultTokenBudget = 4000
	maxHistoryLines    = 50
)

var commandPrefixes = []string{
	"/ai ",
	"/ask ",
	"/explain ",
	"/fix ",
	"@ai ",
	"@orch ",
}

type providerRegistration struct {
	meta   AIProvider
	client providerClient
}

type terminalSessionState struct {
	lineBuffer  strings.Builder
	lastCommand string
	stdoutLines []string
	stderrLines []string
}

// GitHubCacheReader é o mínimo necessário do serviço GitHub para contexto.
type GitHubCacheReader interface {
	GetCachedPullRequest(owner, repo string, number int) (*gh.PullRequest, bool)
}

// ServiceDeps encapsula dependências do AI service.
type ServiceDeps struct {
	GitHubCache GitHubCacheReader
	TokenBudget int
}

// Service implementa IAIService.
type Service struct {
	mu sync.RWMutex

	providers      map[string]providerRegistration
	activeProvider string
	cancels        map[string]context.CancelFunc

	sessionState  map[string]SessionState
	terminalState map[string]*terminalSessionState

	githubCache GitHubCacheReader
	tokenBudget int
	sanitizer   *SecretSanitizer
}

// NewService cria um novo serviço de IA.
func NewService(deps ServiceDeps) *Service {
	tokenBudget := deps.TokenBudget
	if tokenBudget <= 0 {
		tokenBudget = defaultTokenBudget
	}

	svc := &Service{
		providers:     make(map[string]providerRegistration),
		cancels:       make(map[string]context.CancelFunc),
		sessionState:  make(map[string]SessionState),
		terminalState: make(map[string]*terminalSessionState),
		githubCache:   deps.GitHubCache,
		tokenBudget:   tokenBudget,
		sanitizer:     NewSecretSanitizer(),
	}

	svc.bootstrapProviders()
	return svc
}

func (s *Service) bootstrapProviders() {
	// Ollama local pode funcionar sem chave.
	ollama := AIProvider{
		ID:       "ollama",
		Name:     "Ollama (Local)",
		Model:    "llama3",
		Endpoint: "http://localhost:11434",
		Enabled:  true,
	}
	s.providers[ollama.ID] = providerRegistration{
		meta:   ollama,
		client: newOllamaProvider(ollama.Endpoint, ollama.Model),
	}

	// Gemini via env.
	geminiKey := strings.TrimSpace(os.Getenv("ORCH_GEMINI_API_KEY"))
	if geminiKey == "" {
		geminiKey = strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))
	}
	if geminiKey == "" {
		geminiKey = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	}
	gemini := AIProvider{
		ID:      "gemini",
		Name:    "Gemini",
		Model:   "gemini-2.5-flash",
		APIKey:  geminiKey,
		Enabled: geminiKey != "",
	}
	var geminiClient providerClient
	if geminiKey != "" {
		client, err := newGeminiProvider(geminiKey, gemini.Model)
		if err == nil {
			geminiClient = client
		}
	}
	s.providers[gemini.ID] = providerRegistration{
		meta:   gemini,
		client: geminiClient,
	}

	// OpenAI via env.
	openAIKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if openAIKey == "" {
		openAIKey = strings.TrimSpace(os.Getenv("ORCH_OPENAI_API_KEY"))
	}
	openAI := AIProvider{
		ID:      "openai",
		Name:    "OpenAI",
		Model:   "gpt-4.1-mini",
		APIKey:  openAIKey,
		Enabled: openAIKey != "",
	}
	var openAIClient providerClient
	if openAIKey != "" {
		client, err := newOpenAIProvider(openAIKey, openAI.Model)
		if err == nil {
			openAIClient = client
		}
	}
	s.providers[openAI.ID] = providerRegistration{
		meta:   openAI,
		client: openAIClient,
	}

	// Provider padrão: OpenAI > Gemini > Ollama.
	if openAIClient != nil {
		s.activeProvider = "openai"
	} else if geminiClient != nil {
		s.activeProvider = "gemini"
	} else {
		s.activeProvider = "ollama"
	}
}

// GenerateResponse monta contexto + prompt e realiza streaming da resposta.
func (s *Service) GenerateResponse(ctx context.Context, userMessage string, sessionID string) (<-chan string, error) {
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return nil, fmt.Errorf("mensagem vazia")
	}

	state := s.buildContext(sessionID)
	prompt := s.assemblePrompt(state, msg)
	prompt = s.sanitizer.Clean(prompt)
	prompt = s.truncateToFit(prompt, s.tokenBudget)

	provider, client, err := s.getActiveProvider()
	if err != nil {
		return nil, err
	}

	stream := make(chan string, 128)

	if err := s.Cancel(sessionID); err != nil {
		// ignore cancel errors de sessão inexistente
		_ = err
	}

	pctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancels[sessionID] = cancel
	s.mu.Unlock()

	go func() {
		defer close(stream)
		defer s.clearCancel(sessionID)

		if err := client.Stream(pctx, prompt, stream); err != nil {
			stream <- fmt.Sprintf("\r\n[AI:%s erro] %s\r\n", provider.Name, err.Error())
		}
	}()

	return stream, nil
}

// SetProvider configura/ativa o provedor escolhido.
func (s *Service) SetProvider(provider AIProvider) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := strings.ToLower(strings.TrimSpace(provider.ID))
	if id == "" {
		return fmt.Errorf("provider id inválido")
	}

	current, ok := s.providers[id]
	if !ok {
		return fmt.Errorf("provider %q não suportado", id)
	}

	if provider.Model == "" {
		provider.Model = current.meta.Model
	}
	if provider.Name == "" {
		provider.Name = current.meta.Name
	}
	if provider.Endpoint == "" {
		provider.Endpoint = current.meta.Endpoint
	}
	if provider.APIKey == "" {
		provider.APIKey = current.meta.APIKey
	}

	var client providerClient
	switch id {
	case "openai":
		c, err := newOpenAIProvider(provider.APIKey, provider.Model)
		if err != nil {
			return err
		}
		client = c
		provider.Enabled = true
	case "ollama":
		client = newOllamaProvider(provider.Endpoint, provider.Model)
		provider.Enabled = true
	case "gemini":
		c, err := newGeminiProvider(provider.APIKey, provider.Model)
		if err != nil {
			return err
		}
		client = c
		provider.Enabled = true
	default:
		return fmt.Errorf("provider %q não suportado", id)
	}

	s.providers[id] = providerRegistration{
		meta:   provider,
		client: client,
	}
	s.activeProvider = id
	return nil
}

// ListProviders lista provedores disponíveis.
func (s *Service) ListProviders() []AIProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.providers))
	for k := range s.providers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	list := make([]AIProvider, 0, len(keys))
	for _, k := range keys {
		meta := s.providers[k].meta
		meta.APIKey = "" // nunca expor chave ao frontend
		list = append(list, meta)
	}
	return list
}

// Cancel cancela geração de IA em andamento para uma sessão.
func (s *Service) Cancel(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cancel, ok := s.cancels[sessionID]
	if !ok {
		return nil
	}
	cancel()
	delete(s.cancels, sessionID)
	return nil
}

// StreamToFrontend envia chunks para o frontend em eventos Wails.
func (s *Service) StreamToFrontend(ctx context.Context, sessionID string, stream <-chan string, emit func(context.Context, string, interface{})) {
	// Enviar prefixo visual para indicar que a IA está respondendo
	emit(ctx, "ai:response:chunk", map[string]string{
		"sessionID": sessionID,
		"chunk":     "\r\n\x1b[1;35m🤖 ORCH AI\x1b[0m \x1b[90m(Gemini)\x1b[0m\r\n",
	})

	for chunk := range stream {
		formatted := s.FormatForTerminal(chunk)
		emit(ctx, "ai:response:chunk", map[string]string{
			"sessionID": sessionID,
			"chunk":     formatted,
		})
	}

	emit(ctx, "ai:response:chunk", map[string]string{
		"sessionID": sessionID,
		"chunk":     "\r\n",
	})

	emit(ctx, "ai:response:done", map[string]string{
		"sessionID": sessionID,
	})
}

// FormatForTerminal converte Markdown básico em sequências ANSI e normaliza line endings.
func (s *Service) FormatForTerminal(text string) string {
	if text == "" {
		return ""
	}

	// 1. Normalizar line endings para CRLF (\r\n) para evitar efeito "escadinha" no xterm.js
	res := strings.ReplaceAll(text, "\r\n", "\n")
	res = strings.ReplaceAll(res, "\n", "\r\n")

	// 2. Formatação básica de Markdown para ANSI
	// Negrito (**texto**) -> \x1b[1mtexto\x1b[0m
	// Bloco de código (```) -> \x1b[38;5;244m (cinza)
	// Inline code (`) -> \x1b[32m (verde)

	// Nota: Como estamos lidando com chunks (stream), uma regex completa pode falhar
	// em tags cortadas ao meio. Para uma v1 estável, focamos na normalização de linha
	// e substituições simples.

	// Substituições simples que funcionam bem em stream se a tag estiver completa no chunk
	res = strings.ReplaceAll(res, "**", "\x1b[1m") // Inicia negrito (precisa de fechamento, mas terminais costumam resetar no final da linha)
	res = strings.ReplaceAll(res, "```", "\r\n\x1b[38;5;244m")
	res = strings.ReplaceAll(res, "`", "\x1b[32m")

	// Reseta formatação ao final do chunk para garantir que não vaze
	if strings.Contains(res, "\x1b[") {
		res += "\x1b[0m"
	}

	return res
}

// SetSessionState atualiza o contexto da sessão.
func (s *Service) SetSessionState(sessionID string, state SessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionState[sessionID] = state
}

// RemoveSession limpa estado de contexto/histórico da sessão.
func (s *Service) RemoveSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessionState, sessionID)
	delete(s.terminalState, sessionID)
	if cancel, ok := s.cancels[sessionID]; ok {
		cancel()
		delete(s.cancels, sessionID)
	}
}

// ObserveTerminalInput coleta histórico de comandos para contexto da IA.
func (s *Service) ObserveTerminalInput(sessionID string, data []byte) {
	if len(data) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	term := s.getOrCreateTerminalStateLocked(sessionID)

	for _, b := range data {
		switch b {
		case '\r', '\n':
			line := term.lineBuffer.String()
			term.lineBuffer.Reset()
			if line != "" {
				term.lastCommand = line
			}
		case 0x03: // Ctrl+C
			term.lineBuffer.Reset()
		case 0x08, 0x7f: // Backspace
			popLastRune(&term.lineBuffer)
		default:
			if b >= 32 || b == '\t' {
				term.lineBuffer.WriteByte(b)
			}
		}
	}
}

// ObserveTerminalOutput coleta stdout/stderr recente para contexto.
func (s *Service) ObserveTerminalOutput(sessionID string, data []byte) {
	if len(data) == 0 {
		return
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r", ""), "\n")

	s.mu.Lock()
	defer s.mu.Unlock()

	term := s.getOrCreateTerminalStateLocked(sessionID)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if looksLikeErrorLine(line) {
			term.stderrLines = appendHistoryLine(term.stderrLines, line, maxHistoryLines)
		} else {
			term.stdoutLines = appendHistoryLine(term.stdoutLines, line, maxHistoryLines)
		}
	}
}

// IsAICommand verifica se o comando usa prefixo de IA.
func (s *Service) IsAICommand(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return false
	}

	for _, prefix := range commandPrefixes {
		p := strings.TrimSpace(prefix)
		if normalized == p || strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

// ExtractMessage remove o prefixo do comando e retorna a pergunta.
func (s *Service) ExtractMessage(input string) string {
	raw := strings.TrimSpace(input)
	lower := strings.ToLower(raw)

	for _, prefix := range commandPrefixes {
		p := strings.TrimSpace(prefix)
		if lower == p {
			return ""
		}
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(raw[len(prefix):])
		}
	}
	return raw
}

func (s *Service) getOrCreateTerminalStateLocked(sessionID string) *terminalSessionState {
	if st, ok := s.terminalState[sessionID]; ok {
		return st
	}
	st := &terminalSessionState{
		stdoutLines: make([]string, 0, maxHistoryLines),
		stderrLines: make([]string, 0, maxHistoryLines),
	}
	s.terminalState[sessionID] = st
	return st
}

func (s *Service) getActiveProvider() (AIProvider, providerClient, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	reg, ok := s.providers[s.activeProvider]
	if !ok || reg.client == nil {
		return AIProvider{}, nil, fmt.Errorf("nenhum provider ativo configurado")
	}
	return reg.meta, reg.client, nil
}

func (s *Service) clearCancel(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancels, sessionID)
}

func appendHistoryLine(lines []string, line string, limit int) []string {
	lines = append(lines, line)
	if len(lines) <= limit {
		return lines
	}
	return lines[len(lines)-limit:]
}

func looksLikeErrorLine(line string) bool {
	l := strings.ToLower(line)
	return strings.Contains(l, "error") ||
		strings.Contains(l, "panic") ||
		strings.Contains(l, "traceback") ||
		strings.Contains(l, "exception") ||
		strings.Contains(l, "failed")
}

func popLastRune(sb *strings.Builder) {
	runes := []rune(sb.String())
	if len(runes) == 0 {
		return
	}
	sb.Reset()
	sb.WriteString(string(runes[:len(runes)-1]))
}
