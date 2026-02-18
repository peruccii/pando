# Spec: Polling Strategy (GitHub Sync)

> **Módulo**: Transversal — Data Sync  
> **Status**: Draft  
> **PRD Ref**: Seção 7.3  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Como o GitHub não possui WebSockets para todos os eventos, o Host deve manter um **Polling Inteligente** para detectar mudanças externas (commits de outros devs, comentários, merges). O polling deve ser eficiente para não esgotar o rate limit.

---

## 2. Estratégia

### 2.1 Polling Adaptativo

| Estado                          | Intervalo | Motivo                                    |
| -------------------------------- | --------- | ----------------------------------------- |
| PR aberto na tela                | 15s       | Mudanças frequentes durante review         |
| Listagem de PRs visível          | 30s       | Atualizações moderadas                     |
| Nenhum painel GitHub aberto      | 120s      | Só para notificações passivas              |
| App em background/minimizado     | 300s      | Economia de rate-limit                     |
| Sessão colaborativa ativa        | 10s       | Múltiplos devs trabalhando simultaneamente |

### 2.2 Delta-Based Polling

```go
func (s *GitHubService) pollForChanges(ctx context.Context, owner, repo string) {
    // Buscar apenas PRs atualizados desde o último poll
    lastPoll := s.cache.GetLastPollTime(owner + "/" + repo)

    query := `
        query($owner: String!, $repo: String!, $since: DateTime!) {
            repository(owner: $owner, name: $repo) {
                pullRequests(
                    first: 20,
                    orderBy: {field: UPDATED_AT, direction: DESC}
                ) {
                    nodes {
                        number
                        updatedAt
                    }
                }
            }
        }
    `

    // Comparar updatedAt com cache local
    // Só re-fetch completo para PRs que mudaram
    for _, pr := range result.Nodes {
        if pr.UpdatedAt.After(lastPoll) {
            s.fetchAndCachePR(ctx, owner, repo, pr.Number)
        }
    }

    s.cache.SetLastPollTime(owner+"/"+repo, time.Now())
}
```

### 2.3 Rate Limit Awareness

```go
type RateLimitTracker struct {
    remaining int
    resetAt   time.Time
    mu        sync.Mutex
}

func (t *RateLimitTracker) Update(headers http.Header) {
    t.mu.Lock()
    defer t.mu.Unlock()
    t.remaining, _ = strconv.Atoi(headers.Get("X-RateLimit-Remaining"))
    reset, _ := strconv.ParseInt(headers.Get("X-RateLimit-Reset"), 10, 64)
    t.resetAt = time.Unix(reset, 0)
}

func (t *RateLimitTracker) ShouldPoll() bool {
    t.mu.Lock()
    defer t.mu.Unlock()

    // Se restam menos de 100 pontos, desacelerar
    if t.remaining < 100 {
        return false
    }
    return true
}

func (t *RateLimitTracker) GetSafeInterval() time.Duration {
    t.mu.Lock()
    defer t.mu.Unlock()

    if t.remaining < 200 {
        return 120 * time.Second // Modo econômico
    }
    if t.remaining < 500 {
        return 60 * time.Second
    }
    return 30 * time.Second // Normal
}
```

---

## 3. Notificação de Mudanças

Quando o polling detecta mudanças:

```go
func (s *GitHubService) notifyChanges(changes []PRChange) {
    for _, change := range changes {
        // Emitir evento para o frontend
        runtime.EventsEmit(s.ctx, "github:pr:updated", change)

        // Se em sessão P2P, broadcast para guests
        if s.p2pService.HasActiveSession() {
            s.p2pService.Broadcast("github:state:update", change)
        }
    }
}
```

---

## 4. Frontend — Banner de Rate Limit

```typescript
// Quando rate limit está baixo, exibir aviso
function RateLimitBanner({ remaining, resetAt }: RateLimitInfo) {
    if (remaining > 200) return null

    const minutesUntilReset = Math.ceil((resetAt - Date.now()) / 60000)

    return (
        <div className="rate-limit-banner">
            ⚠️ Rate limit baixo ({remaining} restantes).
            Reset em {minutesUntilReset} min.
            Polling reduzido automaticamente.
        </div>
    )
}
```

---

## 5. Dependências

| Dependência               | Tipo       | Spec Relacionada     |
| -------------------------- | ---------- | -------------------- |
| GitHub GraphQL API v4      | Bloqueador | github_integration   |
| Cache em memória           | Bloqueador | github_integration   |
| Wails Events               | Bloqueador | —                    |
