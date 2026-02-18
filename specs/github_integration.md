# Spec: Integração GitHub & Colaboração Real-Time

> **Módulo**: 1 — GitHub Integration  
> **Status**: Draft  
> **PRD Ref**: Seção 7  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Implementar uma **Deep Integration** com o GitHub dentro do ORCH, focando em fluxos colaborativos (Code Review, PR Management, Issues). O app **NÃO** replica um cliente Git completo — operações complexas (`rebase`, `stash`, `cherry-pick`) permanecem no terminal.

---

## 2. API & Comunicação

### 2.1 GitHub GraphQL API v4

- **Endpoint**: `https://api.github.com/graphql`
- **Autenticação**: Bearer Token (OAuth do usuário autenticado)
- **Motivação**: Evitar over-fetching e problemas N+1 da REST API v3

### 2.2 Rate Limiting

| Tipo        | Limite                  | Estratégia                     |
| ----------- | ----------------------- | ------------------------------ |
| **GraphQL** | 5.000 pontos/hora/user  | Cache local agressivo           |
| **REST**    | 5.000 requests/hora     | Evitar uso; fallback apenas     |

---

## 3. Arquitetura de Dados — "Single Source of Truth"

### 3.1 Modo Solo (Sem Colaboração)

```
App (Go Backend)
    │
    ├── GitHubService.FetchPRs(repoOwner, repoName)
    │       │
    │       ▼
    │   GitHub GraphQL API v4
    │       │
    │       ▼
    │   Parse Response → PRList[]
    │       │
    │       ▼
    │   Cache em memória (map[string]PR)
    │       │
    │       ▼
    └── Wails Event → Frontend atualiza UI
```

### 3.2 Modo Colaborativo (Host + Guests)

#### Leitura (Host-Driven)

```go
// O Host é o único que consulta o GitHub
type GitHubService struct {
    client    *http.Client
    cache     *PRCache          // Cache em memória
    pollTick  *time.Ticker      // Polling a cada 30s
    eventBus  EventBus          // Para notificar o frontend
}

// O Host processa e transmite "Hydrated State"
func (s *GitHubService) BroadcastState(sessionID string) {
    state := s.cache.Serialize() // JSON otimizado
    s.p2p.Send(sessionID, "github:state", state)
}
```

- O **Host** consulta a API e mantém cache local.
- O Host transmite **Hydrated State** (JSON otimizado) para Guests via WebRTC.
- **Benefício**: Economia de rate-limit; todos veem exatamente o mesmo estado.

#### Escrita (Guest-Authenticated)

```
Guest (React)
    │
    ├── Ação: "Criar Comentário no PR #42"
    │
    ├── 1. Optimistic UI (insere localmente como "Pendente")
    │
    ├── 2. Broadcast P2P (envia para outros como "Pendente")
    │
    ├── 3. Mutation GraphQL → GitHub API (com TOKEN DO GUEST)
    │       │
    │       ├── Sucesso → Status: "Enviado ✓"
    │       └── Erro → Status: "Falhou" + Retry
    │
    └── 4. Host re-fetcha para sincronizar cache
```

> **Regra de Ouro**: O Guest **JAMAIS** usa as credenciais do Host. Cada ação de escrita usa o OAuth token do próprio Guest.

---

## 4. GitHubService (Backend Go)

### 4.1 Interface

```go
type IGitHubService interface {
    // Repositórios
    ListRepositories(ctx context.Context) ([]Repository, error)
    GetRepository(ctx context.Context, owner, name string) (*Repository, error)

    // Pull Requests
    ListPullRequests(ctx context.Context, owner, repo string, filters PRFilters) ([]PullRequest, error)
    GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error)
    GetPullRequestDiff(ctx context.Context, owner, repo string, number int, pagination DiffPagination) (*Diff, error)
    CreatePullRequest(ctx context.Context, input CreatePRInput) (*PullRequest, error)
    MergePullRequest(ctx context.Context, owner, repo string, number int, method MergeMethod) error
    ClosePullRequest(ctx context.Context, owner, repo string, number int) error

    // Reviews & Comentários
    ListReviews(ctx context.Context, owner, repo string, prNumber int) ([]Review, error)
    CreateReview(ctx context.Context, input CreateReviewInput) (*Review, error)
    ListComments(ctx context.Context, owner, repo string, prNumber int) ([]Comment, error)
    CreateComment(ctx context.Context, input CreateCommentInput) (*Comment, error)
    CreateInlineComment(ctx context.Context, input InlineCommentInput) (*Comment, error)

    // Issues
    ListIssues(ctx context.Context, owner, repo string, filters IssueFilters) ([]Issue, error)
    CreateIssue(ctx context.Context, input CreateIssueInput) (*Issue, error)
    UpdateIssue(ctx context.Context, owner, repo string, number int, input UpdateIssueInput) error

    // Branches
    ListBranches(ctx context.Context, owner, repo string) ([]Branch, error)
    CreateBranch(ctx context.Context, owner, repo, name, sourceBranch string) (*Branch, error)

    // Cache & Polling
    StartPolling(ctx context.Context, owner, repo string, interval time.Duration)
    StopPolling()
    InvalidateCache(owner, repo string)
}
```

### 4.2 Structs Principais

```go
type PullRequest struct {
    ID          string
    Number      int
    Title       string
    Body        string
    State       string    // "OPEN", "CLOSED", "MERGED"
    Author      User
    Reviewers   []User
    Labels      []Label
    CreatedAt   time.Time
    UpdatedAt   time.Time
    MergeCommit *string
    HeadBranch  string
    BaseBranch  string
    Additions   int
    Deletions   int
    ChangedFiles int
}

type Diff struct {
    Files       []DiffFile
    TotalFiles  int
    Pagination  DiffPagination
}

type DiffFile struct {
    Filename    string
    Status      string   // "added", "modified", "deleted", "renamed"
    Additions   int
    Deletions   int
    Hunks       []DiffHunk
}

type DiffHunk struct {
    OldStart    int
    OldLines    int
    NewStart    int
    NewLines    int
    Lines       []DiffLine
}

type DiffLine struct {
    Type    string // "add", "delete", "context"
    Content string
    OldLine *int
    NewLine *int
}

type PRFilters struct {
    State       string   // "OPEN", "CLOSED", "MERGED", "ALL"
    Author      *string
    Labels      []string
    OrderBy     string   // "CREATED_AT", "UPDATED_AT"
    Direction   string   // "ASC", "DESC"
    First       int      // Paginação
    After       *string  // Cursor
}

type Issue struct {
    ID          string
    Number      int
    Title       string
    Body        string
    State       string   // "OPEN", "CLOSED"
    Author      User
    Assignees   []User
    Labels      []Label
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type IssueFilters struct {
    State       string
    Labels      []string
    Assignee    *string
    First       int
    After       *string
}
```

### 4.3 Queries GraphQL (Exemplos)

#### Listar PRs

```graphql
query ListPullRequests($owner: String!, $repo: String!, $first: Int!, $states: [PullRequestState!]) {
  repository(owner: $owner, name: $repo) {
    pullRequests(first: $first, states: $states, orderBy: {field: UPDATED_AT, direction: DESC}) {
      totalCount
      pageInfo {
        hasNextPage
        endCursor
      }
      nodes {
        id
        number
        title
        body
        state
        createdAt
        updatedAt
        additions
        deletions
        changedFiles
        headRefName
        baseRefName
        author {
          login
          avatarUrl
        }
        labels(first: 10) {
          nodes {
            name
            color
          }
        }
        reviewRequests(first: 10) {
          nodes {
            requestedReviewer {
              ... on User {
                login
                avatarUrl
              }
            }
          }
        }
      }
    }
  }
}
```

#### Buscar Diff de um PR

```graphql
query GetPRDiff($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      files(first: 100) {
        totalCount
        pageInfo {
          hasNextPage
          endCursor
        }
        nodes {
          path
          additions
          deletions
          changeType
          patch
        }
      }
    }
  }
}
```

---

## 5. Cache Strategy

### 5.1 Cache em Memória

```go
type PRCache struct {
    mu        sync.RWMutex
    prs       map[string][]PullRequest  // key: "owner/repo"
    issues    map[string][]Issue
    branches  map[string][]Branch
    updatedAt map[string]time.Time
    ttl       time.Duration             // 30 segundos
}

func (c *PRCache) Get(key string) ([]PullRequest, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    if t, ok := c.updatedAt[key]; ok {
        if time.Since(t) > c.ttl {
            return nil, false // Expirado
        }
    }
    prs, ok := c.prs[key]
    return prs, ok
}
```

### 5.2 Polling Inteligente

```go
func (s *GitHubService) StartPolling(ctx context.Context, owner, repo string, interval time.Duration) {
    s.pollTick = time.NewTicker(interval) // Default: 30s

    go func() {
        for {
            select {
            case <-s.pollTick.C:
                key := owner + "/" + repo
                lastUpdate := s.cache.GetUpdatedAt(key)

                // Só re-fetcha se updatedAt mudou
                prs, _ := s.fetchPRsSince(ctx, owner, repo, lastUpdate)
                if len(prs) > 0 {
                    s.cache.Update(key, prs)
                    s.eventBus.Emit("github:prs:updated", prs)
                }

            case <-ctx.Done():
                s.pollTick.Stop()
                return
            }
        }
    }()
}
```

### 5.3 Invalidação

| Evento                       | Ação                                                |
| ----------------------------- | --------------------------------------------------- |
| User cria PR/Comentário       | Invalidar cache do repo; re-fetch imediato           |
| Polling detecta mudança       | Update incremental no cache                          |
| User troca de repositório     | Carregar novo cache; manter anterior por 5 min       |
| File Watcher detecta commit   | Invalidar cache de branches                          |

---

## 6. Componentes Frontend (React)

### 6.1 Estrutura de Componentes

```
src/
├── features/
│   └── github/
│       ├── components/
│       │   ├── PRList.tsx              # Lista de Pull Requests
│       │   ├── PRListItem.tsx          # Item individual na lista
│       │   ├── PRDetail.tsx            # Detalhe do PR selecionado
│       │   ├── PRDiffViewer.tsx        # Visualizador de Diff
│       │   ├── DiffFile.tsx            # Arquivo individual no diff
│       │   ├── DiffHunk.tsx            # Bloco de mudanças
│       │   ├── DiffLine.tsx            # Linha do diff (add/del/ctx)
│       │   ├── InlineComment.tsx       # Comentário inline no diff
│       │   ├── ReviewPanel.tsx         # Painel de review
│       │   ├── ConversationThread.tsx  # Thread de conversa
│       │   ├── IssueBoard.tsx          # Kanban de Issues
│       │   ├── IssueCard.tsx           # Card no Kanban
│       │   ├── BranchSelector.tsx      # Dropdown de branches
│       │   ├── CreatePRDialog.tsx      # Modal de criação de PR
│       │   └── MergeDialog.tsx         # Modal de merge com opções
│       ├── hooks/
│       │   ├── useGitHub.ts            # Hook principal
│       │   ├── usePullRequests.ts      # CRUD de PRs
│       │   ├── useDiff.ts             # Carregamento de diff
│       │   ├── useIssues.ts           # CRUD de Issues
│       │   └── useBranches.ts         # Listagem/criação
│       ├── stores/
│       │   └── githubStore.ts          # Estado global GitHub
│       └── types/
│           └── github.ts              # TypeScript types
```

### 6.2 PRDiffViewer — Requisitos

| Requisito                    | Especificação                                            |
| ----------------------------- | -------------------------------------------------------- |
| **Syntax Highlighting**       | Por extensão do arquivo (`.go`, `.ts`, `.py`, etc.)       |
| **Paginação**                 | Carregar Diffs em chunks de 20 arquivos                   |
| **Inline Comments**           | Clicar na linha → abrir input de comentário               |
| **Expand/Collapse**           | Cada arquivo pode ser expandido/colapsado                 |
| **Scroll Sync (Collab)**      | Comentar em linha → dispara evento WebRTC p/ todos        |
| **Side-by-Side / Unified**    | Toggle entre visualização lado a lado e unificada         |
| **Lazy Loading**              | Hunks grandes carregam por demanda (virtualização)        |

### 6.3 IssueBoard (Kanban) — Requisitos

| Coluna        | Filter                        |
| ------------- | ----------------------------- |
| **Backlog**   | `state: OPEN`, sem assignee    |
| **In Progress** | `state: OPEN`, com assignee  |
| **Done**      | `state: CLOSED`                |

- Drag & Drop entre colunas atualiza Label/State via Mutation.
- Cards exibem: Título, Labels (coloridas), Avatar do Assignee, Nº da Issue.

---

## 7. Scroll Sync (Colaboração)

Quando um usuário comenta em uma linha de código na GUI:

```
User A comenta na linha 42 do arquivo "main.go"
    │
    ├── 1. UI abre InlineComment na linha 42
    │
    ├── 2. WebRTC Event: { type: "scroll_sync", file: "main.go", line: 42 }
    │
    └── 3. Todos os Guests recebem o evento
            │
            ├── DiffViewer scrolla para "main.go"
            ├── Expande o arquivo se colapsado
            └── Scrolls viewport para a linha 42 com highlight
```

---

## 8. Tratamento de Erros

| Erro                          | Código HTTP | Ação                                                     |
| ------------------------------ | ----------- | -------------------------------------------------------- |
| **Token expirado**             | 401         | Refresh silencioso; se falhar, redirecionar para login    |
| **Rate limit**                 | 403/429     | Exibir toast com countdown; pausar polling                |
| **PR não encontrado**          | 404         | Remover do cache; exibir mensagem na UI                   |
| **Permissão negada**           | 403         | Exibir "Você não tem permissão neste repositório"         |
| **Merge conflict**             | 405/409     | Exibir modal com instrução para resolver via terminal      |
| **Network offline**            | —           | Modo offline; exibir banner; usar cache                    |

---

## 9. Métricas de Performance

| Operação                     | Meta                    |
| ----------------------------- | ----------------------- |
| Listar PRs (com cache)       | < 50ms                  |
| Listar PRs (sem cache)       | < 2s                    |
| Carregar Diff (20 arquivos)  | < 1.5s                  |
| Criar comentário (Optimistic)| < 100ms (visual)        |
| Polling cycle                | ~30s (configurável)     |
| Scroll Sync (P2P)            | < 200ms                 |

---

## 10. Dependências

| Dependência                  | Tipo      | Módulo Dependente          |
| ----------------------------- | --------- | -------------------------- |
| Autenticação OAuth (GitHub)   | Bloqueador| auth_and_persistence       |
| WebRTC Data Channel           | Bloqueador| invite_and_p2p             |
| File Watcher (.git)           | Opcional  | file_watcher               |
| Optimistic UI Pattern         | Padrão    | optimistic_ui              |
| Identity Barrier              | Padrão    | identity_barrier           |
