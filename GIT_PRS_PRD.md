# PRD - Git Panel PRs (GitHub Pull Requests via REST)

## 1. Contexto

O Git Panel atual foi desenhado com foco em Git local (working tree, index, history, conflicts).
Agora existe uma necessidade objetiva de produto: gerenciar Pull Requests direto no ORCH e garantir que toda acao feita no app reflita no GitHub imediatamente.

Requisito chave:

- quando criar uma PR no ORCH, a PR deve existir no GitHub (source of truth remoto)

---

## 2. Objetivo do Produto

Entregar uma aba `PRs` dentro do Git Panel para:

- listar PRs do repositorio ativo
- abrir detalhes e diffs por arquivo
- criar, atualizar e mergear PRs
- atualizar branch de PR e verificar status de merge

Tudo isso usando a **GitHub REST API de Pull Requests** (nao GraphQL para este fluxo).

---

## 3. Metas e Nao-Metas

### 3.1 Metas (v1)

- Aba `PRs` integrada ao Git Panel dedicado.
- Fluxo completo de leitura:
  - listagem de PRs
  - detalhes da PR
  - commits da PR
  - arquivos da PR com patch por arquivo
  - diff completo opcional sob demanda
- Fluxo completo de escrita:
  - criar PR
  - atualizar PR
  - merge PR
  - update branch da PR
- Resolver repositorio remoto a partir do repo local ativo (origin -> owner/repo), com fallback manual.
- Performance previsivel com cache, paginacao e lazy loading.

### 3.2 Nao-Metas (v1)

- Reescrever todo modulo GitHub legado.
- Implementar revisao inline completa de code review nesta fase.
- Suportar provedores alem de GitHub.
- Sincronizacao offline com replay de mutacoes.

---

## 4. Escopo Funcional

### 4.1 Descoberta de Repositorio Alvo

- Entrada principal: `repoPath` ativo no Git Panel.
- Resolver `owner/repo` via `git remote get-url origin`.
- Suportar formatos:
  - `git@github.com:owner/repo.git`
  - `https://github.com/owner/repo.git`
- Se nao resolver:
  - mostrar erro acionavel + seletor manual de `owner/repo`.

### 4.2 Listagem de PRs

- Endpoint: `GET /repos/{owner}/{repo}/pulls`
- Default: `state=open`
- Suportar filtro:
  - `open`, `closed`, `all`
- Paginacao:
  - `per_page` e `page`
- Render:
  - lista com numero, titulo, autor, estado, draft, updated_at.

### 4.3 Detalhe da PR

- Endpoint: `GET /repos/{owner}/{repo}/pulls/{pull_number}`
- Mostrar:
  - titulo, descricao, head/base, estado, stats, links, mergeability quando disponivel.

### 4.4 Commits da PR

- Endpoint: `GET /repos/{owner}/{repo}/pulls/{pull_number}/commits`
- Mostrar lista paginada de commits da PR.

### 4.5 Arquivos e Diff por Arquivo

- Endpoint: `GET /repos/{owner}/{repo}/pulls/{pull_number}/files`
- Mostrar:
  - lista de arquivos alterados
  - patch por arquivo (`patch`)
  - status do arquivo (`added`, `modified`, `removed`, `renamed`)
- Tratamento:
  - arquivo binario sem `patch`
  - patch truncado em PRs grandes.

### 4.6 Diff Completo Opcional

- Endpoint: `GET /repos/{owner}/{repo}/pulls/{pull_number}`
- Header:
  - `Accept: application/vnd.github.diff`
- So carregar sob demanda (botao "Ver diff completo"), nunca no load inicial da tela.

### 4.7 Criar PR

- Endpoint: `POST /repos/{owner}/{repo}/pulls`
- Campos:
  - `title`, `head`, `base`
  - opcionais: `body`, `draft`, `maintainer_can_modify`
- Regra:
  - apos sucesso, refresh da lista e selecao da PR criada.

### 4.8 Atualizar PR

- Endpoint: `PATCH /repos/{owner}/{repo}/pulls/{pull_number}`
- Campos suportados:
  - `title`, `body`, `state`, `base`, `maintainer_can_modify`.

### 4.9 Verificar Merge

- Endpoint: `GET /repos/{owner}/{repo}/pulls/{pull_number}/merge`
- Interpretacao:
  - `204` = merged
  - `404` = nao merged.

### 4.10 Merge da PR

- Endpoint: `PUT /repos/{owner}/{repo}/pulls/{pull_number}/merge`
- Suportar:
  - `merge_method`: `merge`, `squash`, `rebase`
  - `sha` opcional para lock otimista.

### 4.11 Update Branch da PR

- Endpoint: `PUT /repos/{owner}/{repo}/pulls/{pull_number}/update-branch`
- Campo opcional:
  - `expected_head_sha`.

---

## 5. Arquitetura Proposta

### 5.1 Backend (Go / Wails)

Estrategia recomendada: evoluir `internal/github` com cliente REST dedicado para Pull Requests.

- Manter o que ja existe sem quebrar.
- Adicionar camada REST para PRs com:
  - `X-GitHub-Api-Version: 2022-11-28`
  - `Accept: application/vnd.github+json` (default JSON)
  - `Accept: application/vnd.github.diff` (quando diff completo for solicitado)

Bindings Wails novos (orientados ao Git Panel):

- `GitPanelPRList(repoPath, state, page, perPage)`
- `GitPanelPRGet(repoPath, prNumber)`
- `GitPanelPRGetFiles(repoPath, prNumber, page, perPage)`
- `GitPanelPRGetCommits(repoPath, prNumber, page, perPage)`
- `GitPanelPRGetRawDiff(repoPath, prNumber)`
- `GitPanelPRCreate(repoPath, payload)`
- `GitPanelPRUpdate(repoPath, prNumber, payload)`
- `GitPanelPRCheckMerged(repoPath, prNumber)`
- `GitPanelPRMerge(repoPath, prNumber, payload)`
- `GitPanelPRUpdateBranch(repoPath, prNumber, payload)`

Observacao:

- `repoPath` e usado para resolver `owner/repo`.
- Nao expor token nem detalhes sensiveis no frontend.

### 5.2 Frontend (React)

Adicionar modo/aba `PRs` dentro da tela do Git Panel, com layout:

- coluna esquerda: lista de PRs + filtros
- coluna central: detalhe da PR
- coluna direita: arquivos/commits/diff

Regras:

- carregamento lazy por secao
- loading state claro por bloco
- fallback de erro por endpoint.

---

## 6. Performance e Otimizacao

### 6.1 Principios

- Nunca carregar diff completo no boot.
- Buscar detalhes apenas quando a PR for selecionada.
- Buscar arquivos e commits sob demanda (aba interna ativa).

### 6.2 Caching

- Cache em memoria por chave de endpoint + parametros.
- TTL curto (15s-30s) para leitura.
- ETag/If-None-Match para reduzir consumo de rate limit.
- Invalida cache apos mutacoes (`create/update/merge/update-branch`).

### 6.3 Polling e Refresh

- Polling somente quando aba `PRs` estiver ativa.
- Context-aware interval:
  - detalhe aberto: mais frequente
  - lista apenas: medio
  - app em background: lento ou pausado
- Backoff automatico em erro/ratelimit.

### 6.4 Budgets iniciais

- abrir aba `PRs` (first paint): < 350ms
- primeira lista de PRs visivel (rede normal): p50 < 1200ms
- abrir detalhe de PR: p50 < 900ms
- abrir arquivos da PR: p50 < 1000ms

---

## 7. Confiabilidade e Tratamento de Erros

Contrato tecnico final de erro (fixado em 24/02/2026):

- Formato obrigatorio unico em backend/frontend:
  - `code`: identificador estavel da classe de erro.
  - `message`: mensagem curta para UX.
  - `details`: detalhe tecnico opcional (debug, sem segredo).
- Shape JSON obrigatorio:
  - `{"code":"E_PR_*","message":"...","details":"..."}`
- Backend:
  - bindings de PR REST devem sempre normalizar para `code/message/details`.
  - nao retornar erro cru de provider para o frontend.
- Frontend:
  - parser unico de erro deve priorizar `code/message/details`.
  - fallback para erro desconhecido quando payload nao estiver no contrato.

Mapeamento obrigatorio HTTP -> `code`:

- 401 -> `E_PR_UNAUTHORIZED`
- 403 -> `E_PR_FORBIDDEN`
- 404 -> `E_PR_NOT_FOUND`
- 409 -> `E_PR_CONFLICT`
- 422 -> `E_PR_VALIDATION_FAILED`
- 429 -> `E_PR_RATE_LIMITED`
- demais -> `E_PR_UNKNOWN`

Comportamento esperado por classe:

- 401: token ausente/expirado -> CTA de reconectar GitHub.
- 403: permissao ou rate limit -> mensagem tecnica + retry/backoff.
- 404: repo/PR inexistente -> estado vazio acionavel.
- 409: conflito de merge/update-branch -> refresh + instrucoes.
- 422: payload invalido (ex: head/base) -> feedback por campo.
- 429/secondary limit: reduzir throughput automatico.

Mutacoes nao devem fazer retry cego.
Somente GET pode ter retry com jitter e limite curto.

---

## 8. Seguranca

- Nao logar token nem payload sensivel.
- Sanitizar owner/repo resolvidos.
- Impedir repositorio alvo divergente do repo selecionado sem confirmacao explicita.
- Headers e permissao minima:
  - leitura PR: `pull_requests:read`
  - escrita PR: `pull_requests:write`
  - merge pode exigir `contents:write`.

---

## 9. Observabilidade

Emitir telemetria por request:

- endpoint
- metodo
- status code
- duracao
- cache hit/miss
- rate limit remaining

Eventos recomendados:

- `gitpanel:prs_request`
- `gitpanel:prs_cache`
- `gitpanel:prs_action_result`

---

## 10. Estrategia de Testes

### 10.1 Backend

- Unit:
  - parse de Link header (paginacao)
  - parse/normalizacao de owner/repo por origin
  - mapeamento de erros HTTP -> erro de dominio
- Integration (httptest/mock server):
  - endpoints REST de PR
  - ETag/304
  - rate limit e secondary limit.

### 10.2 Frontend

- Unit:
  - store de PRs (loading/success/error)
  - render de lista/detalhe/arquivos
  - comportamento com patch truncado/binario
- E2E:
  - listar PRs
  - abrir PR e diff por arquivo
  - criar PR e validar que aparece na lista apos sucesso
  - merge/update branch com feedback correto.

---

## 11. Fases de Entrega

### Fase 1 - Read-only PRs (P0)

- aba `PRs`
- listagem
- detalhe
- commits
- arquivos com patch
- diff completo opcional
- cache + paginacao.

### Fase 2 - Mutacoes (P1)

- criar PR
- atualizar PR
- check merged
- merge
- update branch
- invalidacao de cache pos-acao.

### Fase 3 - Hardening (P2)

- ETag full
- telemetria completa
- backoff avancado de ratelimit
- testes E2E completos
- tuning de performance.

---

## 12. Criterios de Aceite

- Usuario consegue listar PRs do repo ativo sem sair do Git Panel.
- Usuario consegue abrir PR e visualizar arquivos alterados com patch por arquivo.
- Usuario consegue criar PR no ORCH e a PR aparece no GitHub imediatamente apos sucesso da API.
- Usuario consegue mergear PR com metodo selecionado e receber status final confiavel.
- App nao trava em PRs grandes; UI degrada com mensagens claras para patch truncado/binario.
- Rate limit baixo nao derruba a UX (degrada com backoff e aviso).

---

## 13. Riscos Principais

- Risco de regressao se misturar demais fluxo GraphQL legado com novo fluxo REST.
- Risco de estourar rate limit sem ETag/paginacao.
- Risco de UX lenta se carregar diffs grandes de forma eager.

Mitigacao:

- rollout em fases
- lazy loading estrito
- cache + observabilidade desde P0.

---

## 14. Decisoes Oficiais

Decisoes oficiais (fechado em 24/02/2026):

- A aba `PRs` sera uma aba dentro do Git Panel (inspector), nao um modo de tela separado na v1.
- O fallback manual de `owner/repo` fica no header da aba `PRs` (acao local ao contexto de repo), nao em Settings global.
- PRs passam a usar REST como trilha principal; GraphQL legado permanece apenas para fluxos ainda nao migrados, com desativacao progressiva por fase.
