package github

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"
)

// === Polling Context ===

// PollingContext indica o contexto atual para adaptar o intervalo
type PollingContext string

const (
	PollingContextPRDetail    PollingContext = "pr_detail"   // PR aberto na tela
	PollingContextPRList      PollingContext = "pr_list"     // Listagem de PRs visível
	PollingContextBackground  PollingContext = "background"  // Nenhum painel GitHub aberto
	PollingContextMinimized   PollingContext = "minimized"   // App em background
	PollingContextCollaborate PollingContext = "collaborate" // Sessão colaborativa ativa
)

// pollingIntervals mapeia contexto para intervalo base
var pollingIntervals = map[PollingContext]time.Duration{
	PollingContextPRDetail:    15 * time.Second,
	PollingContextPRList:      30 * time.Second,
	PollingContextBackground:  120 * time.Second,
	PollingContextMinimized:   300 * time.Second,
	PollingContextCollaborate: 10 * time.Second,
}

// === Rate Limit Tracker ===

// RateLimitTracker rastreia o rate limit do GitHub API
type RateLimitTracker struct {
	mu        sync.RWMutex
	Remaining int       `json:"remaining"`
	Limit     int       `json:"limit"`
	ResetAt   time.Time `json:"resetAt"`
}

// NewRateLimitTracker cria um novo tracker com defaults
func NewRateLimitTracker() *RateLimitTracker {
	return &RateLimitTracker{
		Remaining: 5000,
		Limit:     5000,
		ResetAt:   time.Now().Add(time.Hour),
	}
}

// Update atualiza o tracker a partir dos valores recebidos
func (t *RateLimitTracker) Update(remaining, limit int, resetAt time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Remaining = remaining
	t.Limit = limit
	t.ResetAt = resetAt
}

// ShouldPoll retorna se é seguro fazer polling agora
func (t *RateLimitTracker) ShouldPoll() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Se restam menos de 100 pontos, pausar polling
	if t.Remaining < 100 {
		return false
	}
	return true
}

// GetSafeInterval retorna o intervalo seguro baseado no rate limit
func (t *RateLimitTracker) GetSafeInterval(base time.Duration) time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.Remaining < 100 {
		// Rate limit crítico — parar polling até reset
		timeUntilReset := time.Until(t.ResetAt)
		if timeUntilReset > 0 {
			return timeUntilReset
		}
		return 300 * time.Second
	}

	if t.Remaining < 200 {
		// Modo econômico — no mínimo 120s
		if base < 120*time.Second {
			return 120 * time.Second
		}
		return base
	}

	if t.Remaining < 500 {
		// Modo cauteloso — no mínimo 60s
		if base < 60*time.Second {
			return 60 * time.Second
		}
		return base
	}

	// Normal — usar intervalo base
	return base
}

// GetInfo retorna info do rate limit para o frontend
func (t *RateLimitTracker) GetInfo() RateLimitInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return RateLimitInfo{
		Remaining: t.Remaining,
		Limit:     t.Limit,
		ResetAt:   t.ResetAt,
	}
}

// RateLimitInfo é a info de rate limit exposta ao frontend
type RateLimitInfo struct {
	Remaining int       `json:"remaining"`
	Limit     int       `json:"limit"`
	ResetAt   time.Time `json:"resetAt"`
}

// === Poller ===

// Poller gerencia o polling inteligente de dados GitHub
type Poller struct {
	mu        sync.RWMutex
	service   *Service
	rateLimit *RateLimitTracker
	context   PollingContext
	owner     string
	repo      string
	cancel    context.CancelFunc
	running   bool

	// Callback para emitir eventos Wails
	emitEvent func(eventName string, data interface{})

	// lastPRsUpdatedAt armazena o updatedAt mais recente dos PRs
	lastPRsUpdatedAt time.Time
}

// NewPoller cria um novo Poller
func NewPoller(service *Service, emitEvent func(eventName string, data interface{})) *Poller {
	return &Poller{
		service:   service,
		rateLimit: NewRateLimitTracker(),
		context:   PollingContextBackground,
		emitEvent: emitEvent,
	}
}

// StartPolling inicia o polling para um repositório
func (p *Poller) StartPolling(owner, repo string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Parar polling anterior se existir
	if p.cancel != nil {
		p.cancel()
	}

	p.owner = owner
	p.repo = repo
	p.running = true
	p.lastPRsUpdatedAt = time.Time{}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	go p.pollingLoop(ctx)
	log.Printf("[Poller] Started polling for %s/%s (context: %s)", owner, repo, p.context)
}

// StopPolling para o polling
func (p *Poller) StopPolling() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.running = false
	log.Println("[Poller] Stopped polling")
}

// SetContext atualiza o contexto de polling (muda o intervalo)
func (p *Poller) SetContext(ctx PollingContext) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.context == ctx {
		return
	}

	oldCtx := p.context
	p.context = ctx
	log.Printf("[Poller] Context changed: %s → %s", oldCtx, ctx)
}

// GetRateLimitInfo retorna informações do rate limit
func (p *Poller) GetRateLimitInfo() RateLimitInfo {
	return p.rateLimit.GetInfo()
}

// IsRunning retorna se o poller está rodando
func (p *Poller) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

// === Internal ===

func (p *Poller) pollingLoop(ctx context.Context) {
	// Poll imediato na primeira vez
	p.poll(ctx)

	for {
		// Calcular intervalo dinâmico
		interval := p.calculateInterval()

		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			p.poll(ctx)
		}
	}
}

func (p *Poller) calculateInterval() time.Duration {
	p.mu.RLock()
	pollingCtx := p.context
	p.mu.RUnlock()

	// Pegar intervalo base do contexto
	base, ok := pollingIntervals[pollingCtx]
	if !ok {
		base = 120 * time.Second
	}

	// Ajustar baseado no rate limit
	return p.rateLimit.GetSafeInterval(base)
}

func (p *Poller) poll(ctx context.Context) {
	// Checar rate limit antes de fazer request
	if !p.rateLimit.ShouldPoll() {
		info := p.rateLimit.GetInfo()
		log.Printf("[Poller] Skipping poll — rate limit low (%d remaining, resets at %s)",
			info.Remaining, info.ResetAt.Format(time.Kitchen))

		// Emitir evento de rate limit baixo
		if p.emitEvent != nil {
			p.emitEvent("github:ratelimit", info)
		}
		return
	}

	p.mu.RLock()
	owner := p.owner
	repo := p.repo
	p.mu.RUnlock()

	if owner == "" || repo == "" {
		return
	}

	// Delta-based polling: buscar PRs recentes e comparar updatedAt
	changes := p.pollForPRChanges(ctx, owner, repo)

	if len(changes) > 0 {
		log.Printf("[Poller] Detected %d PR changes in %s/%s", len(changes), owner, repo)

		// Invalidar cache para forçar re-fetch
		p.service.cache.Invalidate(owner, repo)

		// Emitir evento para o frontend
		if p.emitEvent != nil {
			p.emitEvent("github:prs:updated", map[string]interface{}{
				"owner":   owner,
				"repo":    repo,
				"changes": changes,
				"count":   len(changes),
			})
		}
	}

	// Atualizar rate limit tracker com dados do serviço
	p.syncRateLimit()
}

// PRChange representa uma mudança detectada em um PR
type PRChange struct {
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	UpdatedAt  time.Time `json:"updatedAt"`
	ChangeType string    `json:"changeType"` // "new", "updated"
}

// pollForPRChanges faz um poll leve para detectar mudanças em PRs
func (p *Poller) pollForPRChanges(ctx context.Context, owner, repo string) []PRChange {
	// Query leve: buscar apenas number e updatedAt dos PRs recentes
	query := `query($owner: String!, $repo: String!) {
		repository(owner: $owner, name: $repo) {
			pullRequests(
				first: 20,
				orderBy: {field: UPDATED_AT, direction: DESC},
				states: [OPEN]
			) {
				nodes {
					number
					title
					updatedAt
				}
			}
		}
	}`

	data, err := p.service.executeQuery(query, map[string]interface{}{
		"owner": owner,
		"repo":  repo,
	})
	if err != nil {
		log.Printf("[Poller] Poll query failed: %v", err)
		return nil
	}

	var result struct {
		Repository struct {
			PullRequests struct {
				Nodes []struct {
					Number    int       `json:"number"`
					Title     string    `json:"title"`
					UpdatedAt time.Time `json:"updatedAt"`
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		log.Printf("[Poller] Failed to parse poll response: %v", err)
		return nil
	}

	// Comparar com último poll
	p.mu.RLock()
	lastPoll := p.lastPRsUpdatedAt
	p.mu.RUnlock()

	var changes []PRChange
	var newestUpdatedAt time.Time

	for _, pr := range result.Repository.PullRequests.Nodes {
		if pr.UpdatedAt.After(newestUpdatedAt) {
			newestUpdatedAt = pr.UpdatedAt
		}

		// Se é o primeiro poll, não reportar changes
		if lastPoll.IsZero() {
			continue
		}

		if pr.UpdatedAt.After(lastPoll) {
			changeType := "updated"
			// Se o PR foi criado depois do último poll, é novo
			if pr.UpdatedAt.Sub(lastPoll) < 2*time.Second {
				changeType = "new"
			}
			changes = append(changes, PRChange{
				Number:     pr.Number,
				Title:      pr.Title,
				UpdatedAt:  pr.UpdatedAt,
				ChangeType: changeType,
			})
		}
	}

	// Atualizar timestamp do último poll
	if !newestUpdatedAt.IsZero() {
		p.mu.Lock()
		p.lastPRsUpdatedAt = newestUpdatedAt
		p.mu.Unlock()
	}

	return changes
}

// syncRateLimit sincroniza o rate limit do serviço com o tracker
func (p *Poller) syncRateLimit() {
	p.rateLimit.Update(p.service.rateLeft, 5000, p.service.rateReset)
}
