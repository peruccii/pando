# 📋 TASKS — ORCH (Orchestrator)

> **Arquivo de tarefas do projeto ORCH**
> Cada seção corresponde a um arquivo de spec. Tarefas ordenadas por dependência e fase do roadmap.
>
> **Última Atualização**: 12 de Fevereiro de 2026

---

## Fase 0 — Fundação

### 🏗️ Project Structure — [spec](specs/project_structure.md)

- [x] Inicializar projeto Wails v2 com React + Vite + TypeScript
- [x] Configurar estrutura de pastas (`internal/`, `frontend/src/features/`, etc.)
- [x] Criar `app.go` com struct `App` e lifecycle hooks (`Startup`, `Shutdown`, `DomReady`)
- [x] Criar `main.go` como entrypoint do Wails
- [x] Configurar `wails.json` (nome, build, dev server)
- [x] Configurar `go.mod` com dependências principais (Wails, GORM, SQLite, go-keyring, creack/pty, fsnotify)
- [x] Configurar `package.json` com dependências frontend (React, xterm, react-mosaic, zustand, lucide-react, yjs)
- [x] Configurar `tsconfig.json` e `vite.config.ts`
- [x] Configurar `Info.plist` para deep links (`orch://`)
- [x] Criar `internal/config/config.go` com paths do macOS e constantes
- [x] Criar scripts `scripts/dev.sh` e `scripts/build.sh`
- [x] Validar que `wails dev` roda com hot reload funcional

### 🎨 Design System — [spec](specs/design_system.md)

- [x] Criar `frontend/src/index.css` com CSS Custom Properties (tokens)
- [x] Implementar tema **Dark** (padrão) com todas as variáveis de cor
- [x] Implementar tema **Light** com todas as variáveis de cor
- [x] Implementar tema **Hacker** com todas as variáveis de cor
- [x] Definir tipografia (`--font-sans: Inter`, `--font-mono: JetBrains Mono`)
- [x] Definir escala de espaçamento (`--space-1` a `--space-12`)
- [x] Definir border radius (`--radius-sm` a `--radius-full`)
- [x] Incluir Google Fonts no `index.html` (Inter + JetBrains Mono)
- [x] Implementar componentes CSS base: `.btn` (primary, ghost, danger, disabled)
- [x] Implementar componentes CSS base: `.input` com estados (focus, hover)
- [x] Implementar componentes CSS base: `.badge` (success, warning, error, info)
- [x] Implementar animações: `fadeIn`, `pulse`, `slideUp`, `glow`
- [x] Criar classes utilitárias de animação (`.animate-fade-in`, `.animate-pulse`, etc.)
- [x] Implementar mecanismo de troca de tema via `data-theme` no `:root`

### 🔐 Auth & Persistence — [spec](specs/auth_and_persistence.md)

- [x] Criar `internal/auth/types.go` com structs `User`, `AuthResult`, `AuthState`
- [x] Criar `internal/auth/service.go` implementando `IAuthService`
- [x] Implementar PKCE Flow helpers em `internal/auth/pkce.go` (code_verifier + code_challenge)
- [x] Configurar Supabase Auth como provedor OAuth (GitHub + Google)
- [x] Implementar `Login(provider)` — abre Safari na URL de Auth do Supabase
- [x] Implementar captura de deep link `orch://auth/callback` no Wails
- [x] Implementar `HandleCallback(code)` — troca code por access_token + refresh_token
- [x] Implementar `storeTokens()` — armazenar tokens no macOS Keychain via `go-keyring`
- [x] Implementar `getAccessToken()` e `getRefreshToken()` do Keychain
- [x] Implementar `RefreshToken()` — renovação silenciosa de token expirado
- [x] Implementar `Logout()` — limpar tokens do Keychain
- [x] Implementar `GetCurrentUser()` — buscar perfil do Supabase
- [x] Implementar `IsAuthenticated()` — checar validade do token
- [x] Implementar `GetGitHubToken()` — retornar token OAuth do GitHub para API
- [x] Criar `internal/database/models.go` com GORM models (`UserConfig`, `Workspace`, `AgentInstance`, `ChatHistory`, `SessionHistory`)
- [x] Criar `internal/database/service.go` implementando `IDBService`
- [x] Implementar `getDBPath()` — caminho do SQLite (`~/Library/Application Support/ORCH/orch_data.db`)
- [x] Implementar inicialização do banco com `gorm.Open(sqlite.Open(...))` usando pure-Go driver
- [x] Implementar `AutoMigrate` para todos os models
- [x] Implementar CRUD de `UserConfig` (GetConfig, UpdateConfig)
- [x] Implementar CRUD de `Workspace` (List, Get, Create, SetActive, Delete)
- [x] Implementar CRUD de `AgentInstance` (List, Create, Update, Delete, UpdateLayout)
- [x] Implementar CRUD de `ChatHistory` (GetHistory, SaveMessage, ClearHistory)
- [x] Implementar criptografia AES-256 para API Keys stored no SQLite
- [x] Implementar rotina de **Bootstrap** no `Startup()` (Check Auth → Check DB → Restore State)
- [x] Emitir evento Wails `app:hydrated` com `HydrationPayload` para o frontend
- [x] Criar `frontend/src/stores/authStore.ts` (Zustand) com `AuthState`
- [x] Implementar hook `useAuth.ts` que escuta eventos Wails de auth
- [x] Configurar permissão `0600` no arquivo SQLite
- [x] Garantir que logs não contêm dados sensíveis (sanitizer)

---

## Fase 1 — Terminal & UI Core

### 🖥️ Command Center UI — [spec](specs/command_center_ui.md)

- [x] Instalar e configurar `react-mosaic-component`
- [x] Criar `CommandCenter.tsx` — container principal com Mosaic
- [x] Criar `PaneContainer.tsx` — wrapper de cada painel
- [x] Criar `PaneHeader.tsx` — header (28px) com nome, status indicator e controles
- [x] Implementar indicadores de status no header: 🟢 idle, 🟡 running (pulsante), 🔴 error
- [x] Implementar controles rápidos no header: Kill (🗑️), Restart (🔄), Logs (🔍), Zen (⛶)
- [x] Criar `TerminalPane.tsx` — painel de terminal com xterm.js
- [x] Criar `AIAgentPane.tsx` — painel de agente de IA
- [x] Criar `GitHubPane.tsx` — painel GitHub (será populado na Fase 2)
- [x] Implementar **Smart Layout** (`calculateLayout`) — regras automáticas de 1 a 10+ painéis
- [x] Criar `frontend/src/stores/layoutStore.ts` (Zustand) com estado do grid
- [x] Criar hook `useLayout.ts` — lógica de layout automático
- [x] Implementar **Resizing** com Draggable Gutters (6px, cursor `col-resize`/`row-resize`)
- [x] Disparar `fitAddon.fit()` em cada redimensionamento de painel
- [x] Implementar **Drag & Drop** — arrastar header para trocar posição entre painéis
- [x] Implementar feedback visual no drag (opacity, scale) e drop (border accent, glow)
- [x] Criar `ZenModeOverlay.tsx` — overlay de tela cheia
- [x] Criar hook `useZenMode.ts` — enter/exit/toggle (duplo-clique ou `Cmd+Enter`)
- [x] Implementar **hierarquia visual**: painel ativo (borda accent + glow) vs inativos (dimmed 85%)
- [x] Criar hook `usePaneFocus.ts` — gerenciamento de foco entre painéis
- [x] Implementar CSS de transição suave para foco/opacity/border
- [x] Implementar **persistência de layout** — salvar coordenadas no SQLite via `DBService.UpdateAgentLayout`
- [x] Implementar **restore de layout** — reconstruir `MosaicNode` a partir de `AgentInstance[]`
- [x] Implementar **virtualização de renderização** — WebGL para foco, Canvas 2D sem foco, `display:none` para minimizados

### 📺 Terminal Sharing — [spec](specs/terminal_sharing.md)

- [x] Criar `internal/terminal/types.go` com structs `PTYConfig`, `PTYSession`, `OutputMessage`, `InputMessage`
- [x] Criar `internal/terminal/pty_manager.go` implementando `IPTYManager`
- [x] Implementar `Create(config)` — spawn de processo PTY via `creack/pty`
- [x] Implementar `Destroy(sessionID)` — kill do processo PTY
- [x] Implementar `Resize(sessionID, cols, rows)` — resize do PTY
- [x] Implementar `Write(sessionID, data)` — enviar dados para stdin do PTY
- [x] Implementar `OnOutput(sessionID, handler)` — callback para stdout do PTY
- [x] Criar `internal/terminal/bridge.go` — Terminal Bridge para streaming I/O
- [x] Implementar broadcast de output via Wails Events para o frontend
- [x] Integrar xterm.js no frontend com `FitAddon`, `WebglAddon` e `SearchAddon`
- [x] Implementar `ResizeObserver` no container → `fitAddon.fit()` → `PTYManager.Resize`
- [x] Implementar temas de terminal (Dark: Tokyo Night, Light: One Light, Hacker: Matrix)
- [x] Implementar tipo `TerminalPermission` (`none`, `read_only`, `read_write`)
- [x] Implementar validação de permissão no input do Guest antes de enviar ao PTY
- [x] Implementar virtualização: pausar renderização de terminais minimizados, ring buffer 64KB
- [x] Implementar alternância WebGL (60fps com foco) / Canvas 2D (30fps sem foco)
- [x] Preparar suporte a modo Docker (spawn de container via `DockerService`)
- [x] Preparar suporte a modo Live Share (PTY local com controle de permissão)
- [x] Implementar Cursor Awareness multi-user (barra vertical colorida + label + isTyping)
- [x] Integrar Yjs (CRDTs) para resolução de conflitos em input simultâneo

### ⌨️ Keyboard Shortcuts — [spec](specs/keyboard_shortcuts.md)

- [x] Criar hook `useKeyboardShortcuts.ts`
- [x] Implementar detecção de conflito: se `xterm` em foco, apenas atalhos "escape" passam
- [x] Implementar atalho `Cmd+N` — Novo terminal/agente
- [x] Implementar atalho `Cmd+W` — Fechar painel ativo
- [x] Implementar atalhos `Cmd+1` a `Cmd+9` — Focar painel por índice
- [x] Implementar atalhos `Cmd+[` / `Cmd+]` — Navegar entre painéis
- [x] Implementar atalho `Cmd+Enter` — Toggle Zen Mode
- [x] Implementar atalho `Cmd+\` — Split vertical
- [x] Implementar atalho `Cmd+Shift+\` — Split horizontal
- [x] Implementar atalho `Cmd+B` — Toggle sidebar
- [x] Implementar atalho `Cmd+Shift+B` — Toggle Broadcast Mode
- [x] Implementar atalho `Cmd+K` — Command Palette
- [x] Implementar atalho `Cmd+,` — Abrir Settings
- [x] Implementar atalho `Cmd+Shift+D` — Toggle Dark/Light theme
- [x] Implementar atalho `Escape` — Sair do Zen Mode / Broadcast / Modal
- [x] Criar **Command Palette** — busca fuzzy de todas as ações com atalhos exibidos
- [x] Adicionar ARIA labels em todos os botões interativos com menção ao atalho

---

## Fase 2 — GitHub Integration

### 🐙 GitHub Integration — [spec](specs/github_integration.md)

- [x] Criar `internal/github/types.go` com structs (`PullRequest`, `Diff`, `DiffFile`, `DiffHunk`, `DiffLine`, `Issue`, `Branch`, etc.)
- [x] Criar `internal/github/service.go` implementando `IGitHubService`
- [x] Implementar client HTTP autenticado com Bearer Token (OAuth do usuário)
- [x] Criar `internal/github/queries.go` — queries GraphQL (ListPRs, GetPRDiff, ListIssues, ListBranches)
- [x] Implementar `ListRepositories()` — listar repos do usuário
- [x] Implementar `ListPullRequests()` — listar PRs com filtros (state, author, labels, paginação)
- [x] Implementar `GetPullRequest()` — detalhe de um PR
- [x] Implementar `GetPullRequestDiff()` — diff paginado de um PR
- [x] Implementar `CreatePullRequest()` — mutation GraphQL
- [x] Implementar `MergePullRequest()` — mutation com método (merge, squash, rebase)
- [x] Implementar `ClosePullRequest()`
- [x] Implementar `ListReviews()` e `CreateReview()`
- [x] Implementar `ListComments()`, `CreateComment()` e `CreateInlineComment()`
- [x] Implementar `ListIssues()` e `CreateIssue()` e `UpdateIssue()`
- [x] Implementar `ListBranches()` e `CreateBranch()`
- [x] Criar `internal/github/cache.go` — cache em memória (`PRCache` com `sync.RWMutex`, TTL 30s)
- [x] Implementar `Get()`, `Update()`, `Invalidate()`, `GetUpdatedAt()` no cache
- [x] Implementar tratamento de erros (401→refresh, 403/429→rate limit, 404, offline)
- [x] Criar componente `PRList.tsx` — listagem de Pull Requests
- [x] Criar componente `PRListItem.tsx` — item individual na lista
- [x] Criar componente `PRDetail.tsx` — detalhe do PR selecionado
- [x] Criar componente `PRDiffViewer.tsx` — visualizador de Diff com syntax highlighting
- [x] Criar componente `DiffFile.tsx` — arquivo individual no diff (expand/collapse)
- [x] Criar componente `DiffHunk.tsx` — bloco de mudanças
- [x] Criar componente `DiffLine.tsx` — linha do diff (add/del/context)
- [x] Implementar modo **Side-by-Side** e **Unified** no DiffViewer
- [x] Implementar paginação de Diffs (chunks de 20 arquivos)
- [x] Implementar lazy loading de hunks grandes (virtualização)
- [x] Criar componente `InlineComment.tsx` — comentário inline no diff
- [x] Criar componente `ReviewPanel.tsx` — painel de review
- [x] Criar componente `ConversationThread.tsx` — thread de conversa
- [x] Criar componente `IssueBoard.tsx` — Kanban de Issues (Backlog / In Progress / Done)
- [x] Criar componente `IssueCard.tsx` — card no Kanban (título, labels, avatar, nº)
- [x] Implementar Drag & Drop entre colunas do Kanban (atualiza Label/State via Mutation)
- [x] Criar componente `BranchSelector.tsx` — dropdown de branches com checkout rápido
- [x] Criar componente `CreatePRDialog.tsx` — modal de criação de PR
- [x] Criar componente `MergeDialog.tsx` — modal de merge com opções
- [x] Criar `frontend/src/stores/githubStore.ts` (Zustand) — estado global GitHub
- [x] Criar hooks: `useGitHub.ts`, `usePullRequests.ts`, `useDiff.ts`, `useIssues.ts`, `useBranches.ts`

### 📂 File Watcher — [spec](specs/file_watcher.md)

- [x] Criar `internal/filewatcher/types.go` com structs `FileEvent`, `CommitInfo`
- [x] Criar `internal/filewatcher/service.go` implementando `IFileWatcher`
- [x] Implementar `Watch(projectPath)` — monitorar `.git/`, `.git/refs/heads/`, `.git/refs/remotes/` via `fsnotify`
- [x] Implementar `Unwatch(projectPath)` — parar monitoramento
- [x] Implementar `eventLoop()` com debounce de 200ms por arquivo
- [x] Implementar `classifyEvent()` — classificar eventos (branch_changed, commit, merge, fetch, index)
- [x] Implementar `readCurrentBranch()` — ler `.git/HEAD` e parsear branch atual
- [x] Implementar `GetLastCommit()` — ler último commit
- [x] Emitir eventos Wails (`git:branch_changed`, `git:commit`, `git:merge`, `git:fetch`)
- [x] Integrar no frontend: escutar eventos e atualizar `githubStore` (invalidar cache, atualizar BranchSelector)
- [x] Iniciar Watch automaticamente ao abrir/ativar um Workspace
- [x] Cleanup (Unwatch) ao fechar/trocar workspace

### 🔄 Polling Strategy — [spec](specs/polling_strategy.md)

- [x] Criar `internal/github/polling.go` implementando polling inteligente
- [x] Implementar `StartPolling(ctx, owner, repo, interval)` com `time.Ticker`
- [x] Implementar **Polling Adaptativo** — intervalos variáveis (10s-300s conforme contexto)
- [x] Implementar **Delta-Based Polling** — comparar `updatedAt` com cache local, só re-fetch se mudou
- [x] Criar `RateLimitTracker` — ler headers `X-RateLimit-Remaining` e `X-RateLimit-Reset`
- [x] Implementar `ShouldPoll()` — pausar se restam < 100 pontos
- [x] Implementar `GetSafeInterval()` — ajustar intervalo baseado em rate limit restante
- [x] Emitir evento `github:prs:updated` quando polling detectar mudanças
- [x] Implementar broadcast P2P do estado atualizado em sessões colaborativas
- [x] Criar componente `RateLimitBanner.tsx` — aviso quando rate limit está baixo
- [x] Implementar `StopPolling()` e cleanup

### 🔒 Identity Barrier — [spec](specs/identity_barrier.md)

- [x] Criar componente `AuthGuard.tsx` com props `children`, `fallback`, `action`, `requireGitHub`
- [x] Implementar lógica: se `!isAuthenticated` → renderizar botão disabled com tooltip
- [x] Implementar lógica: se `requireGitHub` e provider != github → botão "Conectar GitHub"
- [x] Aplicar `AuthGuard` em todas as ações de escrita (Criar PR, Comentar, Aprovar, Merge, Criar Issue)
- [x] Manter áreas read-only acessíveis sem login (DiffViewer, PRList)
- [x] Implementar **Login Prompt Contextual** — modal com opções GitHub/Google e "Continuar sem login"
- [x] Implementar CSS `.btn--auth-required` (opacity, 🔒 badge, tooltip)
- [x] Terminal P2P acessível mesmo sem login GitHub (se sessão permite anônimos)

### 📌 Optimistic UI — [spec](specs/optimistic_ui.md)

- [x] Implementar hook genérico `useOptimisticAction<T>` com status (idle, pending, success, error)
- [x] Implementar pipeline: atualização local imediata → broadcast P2P → persist async
- [x] Implementar **retry** automático com contador (max 3 tentativas)
- [x] Implementar **rollback** — remover item e notificar
- [x] Implementar broadcast P2P de ações optimistic (`optimistic:pending`, `optimistic:success`, `optimistic:error`)
- [x] Aplicar Optimistic UI em: Criar Comentário, Criar Review, Aprovar/Rejeitar PR, Criar Issue, Mover Issue, Criar Branch
- [x] Implementar CSS de feedback: `.comment--pending`, `.comment--error`, `.comment--success`
- [x] Implementar caso especial para **Merge PR** — modal de confirmação síncrono (NÃO usar Optimistic)

---

## Fase 3 — Colaboração P2P

### 🤝 Invite & P2P — [spec](specs/invite_and_p2p.md)

- [x] Criar `internal/session/types.go` com structs `Session`, `SessionConfig`, `SessionGuest`, `GuestRequest`, `SignalMessage`
- [x] Criar `internal/session/service.go` implementando `ISessionService`
- [x] Implementar `CreateSession(hostUserID, config)` — criar sessão com código
- [x] Criar `internal/session/short_code.go` — gerador de Short Codes (`XXXX-XXX`, charset sem ambíguos)
- [x] Implementar expiração de código (15 min, configurável) e uso único
- [x] Implementar `JoinSession(code, guestUserID)` — validar código, criar pedido de entrada
- [x] Implementar `ApproveGuest()` e `RejectGuest()` — controle do Host
- [x] Implementar `EndSession()` — encerrar sessão e desconectar todos
- [x] Implementar `ListPendingGuests()` — listar pedidos pendentes
- [x] Criar `internal/session/signaling.go` — Signaling Server via WebSocket
- [x] Implementar endpoint WebSocket `ws://localhost:PORT/ws/signal`
- [x] Implementar troca de SDP Offer/Answer entre Host e Guest
- [x] Implementar forwarding de ICE Candidates
- [x] Configurar ICE Servers (STUN: `stun.l.google.com:19302` + TURN como fallback)
- [x] Implementar Data Channels no frontend: `terminal-io`, `github-state`, `cursor-awareness`, `control`, `chat`
- [x] Implementar classe `P2PConnection` com reconexão automática (backoff exponencial, max 5 retries)
- [x] Criar UI **Waiting Room** — Host View (aprovar/rejeitar) e Guest View (aguardando)
- [x] Implementar topologia: Full Mesh (1-4 guests), Star (5-10), considerar SFU (10+)
- [x] Implementar atalhos `Cmd+Shift+S` (Start/Stop sessão) e `Cmd+Shift+J` (Join sessão)
- [x] Emitir evento quando backend "sai da jogada" após WebRTC P2P estabelecido

### 📜 Scroll Sync — [spec](specs/scroll_sync.md)

- [x] Definir interface `ScrollSyncEvent` (type, file, line, userID, userName, userColor, action)
- [x] Implementar emissão de evento WebRTC ao comentar em linha do DiffViewer
- [x] Implementar `handleScrollSync()` no receptor: navegar para arquivo, expandir, scroll suave, highlight
- [x] Implementar highlight temporário (2s) com cor do usuário emissor
- [x] Implementar toast discreto: "Fulano está em arquivo:linha"
- [x] Implementar settings do usuário: `scrollSync.enabled`, `autoFollow`, `showToast`
- [x] Se `autoFollow = false`, exibir apenas toast com link "[Ir para lá]"
- [x] Implementar anti-spam: debounce 2s por usuário, ignore self, max 10 eventos/min por sessão

---

## Fase 4 — Motor de IA

### 🤖 AI Engine — [spec](specs/ai_engine.md)

- [x] Criar `internal/ai/types.go` com structs `AIProvider`, `SessionState`, `PRContext`, `IssueContext`
- [x] Criar `internal/ai/service.go` implementando `IAIService`
- [x] Implementar `GenerateResponse(ctx, userMessage, sessionID)` — retorna channel de streaming
- [x] Implementar `SetProvider(provider)` — configurar provedor ativo
- [x] Implementar `ListProviders()` — listar provedores disponíveis
- [x] Implementar `Cancel(sessionID)` — cancelar geração em andamento
- [x] Criar `internal/ai/context_builder.go` — montagem do `SessionState`
- [x] Implementar `buildContext(sessionID)` — enriquecer com dados do GitHub (cache) e histórico do terminal
- [x] Implementar `assemblePrompt(context, userMessage)` — concatenar SystemPrompt + Contexto + UserMessage
- [x] Implementar **System Prompt Template** dinâmico com placeholders (ProjectName, CurrentBranch, PR, Errors)
- [x] Implementar **Token Budget** (~4000 tokens) com orçamento por seção (Role:200, AppState:100, PRDiff:2000, Terminal:500, User:1000)
- [x] Implementar `truncateDiff()` — truncamento inteligente por prioridade de extensão (.go/.ts > .py > .css > .json)
- [x] Implementar lista de ignore (`package-lock.json`, `yarn.lock`, `go.sum`, `*.min.js`, `*.map`)
- [x] Criar `internal/ai/sanitizer.go` — `SecretSanitizer` com regex para tokens (GitHub PAT, OpenAI, Google API, Bearer)
- [x] Implementar `Clean(text)` — substituir padrões sensíveis por `[REDACTED]`
- [x] Criar `internal/ai/providers.go` — implementação para cada provedor
- [x] Implementar provedor **Gemini** via `google.golang.org/genai` (streaming)
- [x] Implementar provedor **OpenAI** via `github.com/sashabaranov/go-openai` (streaming)
- [x] Implementar provedor **Ollama** via HTTP API (`localhost:11434`) (streaming)
- [x] Implementar `streamToFrontend()` — emitir chunks via Wails Events (`ai:response:chunk`, `ai:response:done`)
- [x] Implementar listener no frontend: escrever chunks no xterm.js simulando digitação
- [x] Criar **Interceptador de Comandos** (`IsAICommand`, `ExtractMessage`) com prefixos `/ai`, `/ask`, `/explain`, `/fix`, `@ai`, `@orch`
- [x] Integrar interceptador no fluxo de input do terminal (desviar para AIService ao invés do shell)

### ⚡ Broadcast Input — [spec](specs/broadcast_input.md)

- [x] Criar `BroadcastBar.tsx` — barra fixa no rodapé com toggle, target selector, input field, botão send
- [x] Criar store `BroadcastStore` (Zustand) com `isActive`, `targetAgentIDs`, `history`
- [x] Implementar `activate()`, `deactivate()`, `toggle()`, `setTargets()`, `send()`
- [x] Implementar `broadcastSend()` — enviar mensagem para todos os PTYs dos agentes-alvo
- [x] Implementar **Target Selector** — dropdown com filtros: all, running, idle, custom
- [x] Implementar histórico de broadcast (últimos 20 comandos, navegação com ↑/↓)
- [x] Implementar feedback visual: borda inferior pulsante, badge ⚡ nos terminais-alvo, highlight ao receber
- [x] Implementar atalhos: `Cmd+Shift+B` (toggle), `Ctrl+Enter` (enviar), `Escape` (desativar)
- [x] Garantir que broadcast **não** envia para terminais de Guests P2P (apenas locais)

---

## Fase 5 — Segurança & Docker

### 🛡️ Security & Sandboxing — [spec](specs/security_sandboxing.md)

- [x] Criar `internal/docker/types.go` com structs `ContainerConfig`, `ContainerInfo`
- [x] Criar `internal/docker/service.go` implementando `IDockerService`
- [x] Implementar `IsDockerAvailable()` — checar se Docker está instalado e rodando
- [x] Implementar `CreateContainer(config)` — criar container com limites (memory, cpus, pids, read-only, no-new-privileges)
- [x] Implementar `StartContainer()`, `StopContainer()`, `RemoveContainer()`, `RestartContainer()`
- [x] Implementar `ExecInContainer()` — exec interativo dentro do container
- [x] Implementar `GetContainerStatus()` e `ListContainers()`
- [x] Criar `internal/docker/detector.go` — auto-detect de imagem Docker pelo projeto (package.json→node, go.mod→golang, etc.)
- [x] Implementar `buildRunArgs()` — montar flags de segurança (`--security-opt`, `--read-only`, `--tmpfs`, `--network`)
- [x] Implementar bind mount da pasta do projeto em `/workspace`
- [x] Implementar funcionalidade "Reiniciar Ambiente" (rebuild container em ~5s)
- [x] Implementar fallback para modo Live Share quando Docker não está disponível
- [x] Implementar modal de concessão de Write com alerta de segurança obrigatório
- [x] Implementar revogação instantânea de permissão via WebRTC
- [x] Criar tabela `audit_log` no SQLite para eventos auditáveis
- [x] Implementar `AuditEvent` logging: guest entrou/saiu, permissão alterada, comando executado, container reiniciado
- [x] Implementar retenção de audit log (últimas 1000 entradas por sessão)
- [x] Criar UI para "Ver Logs" de auditoria no painel de sessão
- [x] Implementar `LogSanitizer` — sanitizar tokens/senhas antes de escrever em logs
- [x] Validar checklist de segurança completo antes do release

---

## Fase 6 — Polish & Launch

- [x] Implementar **Onboarding Wizard** — fluxo guiado na primeira execução
- [x] Implementar seletor de tema na interface (Settings)
- [x] Implementar i18n (Português BR e Inglês) — Fase 2 do roadmap
- [x] Implementar **virtualização avançada** para 10+ terminais simultâneos sem lag
- [x] Implementar **reconexão automática** WebRTC em caso de queda
- [x] Implementar testes E2E para fluxos críticos
- [x] Escrever documentação de usuário
- [x] Build de produção (`.dmg`) para macOS via `wails build -platform darwin/universal`
- [x] Release v1.0.0

---

> **Nota**: As tarefas dentro de cada seção estão ordenadas por dependência lógica. Fases devem ser executadas sequencialmente, mas tarefas dentro de uma mesma fase podem ser paralelizadas quando não há dependência direta.
