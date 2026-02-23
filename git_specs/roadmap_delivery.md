# Spec: Roadmap Delivery

## 1. Milestones e Gates

### M0 - Scope Freeze (P0, pre-implementacao)

Objetivo:

- congelar escopo v1 e decisoes oficiais (UX dedicada, corte v1/v1.1, compatibilidade legado)

Gate de entrada:

- PRD com open questions resolvidas
- `product_scope.md` e `ux_shortcuts.md` atualizados com decisoes oficiais

Gate de saida:

- checklist P0 de definicao estrategica marcado
- backlog ordenado por prioridade com itens bloqueadores identificados

### M1 - Foundation Runtime (P0)

Objetivo:

- habilitar host dedicado do Git Panel e contratos base de backend/frontend

Entrega minima:

- tela dedicada acionada pelo botao `GitHub`
- layout base (refs esquerda, historico centro, inspector direita)
- bindings iniciais e contrato de erro normalizado
- estrategia de compatibilidade `github` legado aplicada

Gate de entrada:

- M0 concluido
- estrategia de `repoPath` ativo definida

Gate de saida:

- abrir/fechar tela Git sem perder estado de terminais/workspace
- migracao legado sem quebrar layout/snapshots
- `go test ./...` verde
- `npm --prefix frontend run build` verde

### M2 - Core Git Operations (P0)

Objetivo:

- entregar fluxo Git local confiavel para leitura + writes basicos

Entrega minima:

- status `staged/unstaged/conflicted` via porcelain parser robusto
- fila sequencial por repo com retry de lock e cancelamento seguro
- acoes `stage/unstage/discard` por arquivo
- invalidacao via watcher + reconciliacao pos-write
- historico linear com virtualizacao

Gate de entrada:

- M1 concluido
- preflight runtime e validacoes de path especificadas

Gate de saida:

- write sem colisao `index.lock` em concorrencia local
- sem polling continuo de `git status`
- smoke tests P0 (parser/queue/bindings/build) verdes

### M3 - Diff and Conflict Flow (P1)

Objetivo:

- aumentar capacidade de revisao e resolucao direta no painel

Entrega minima:

- diff viewer com modo unified/split e fallback para arquivos grandes/binarios
- painel de conflitos com `Accept Mine/Theirs` (auto-stage opcional)
- diagnostico de comando (`queued/started/retried/succeeded/failed`)

Gate de entrada:

- M2 concluido
- budgets de performance baseline medidos

Gate de saida:

- conflitos detectados e resolvidos sem reinicio do app
- diff navegavel sem freeze perceptivel
- testes backend e frontend criticos ampliados

### M4 - Release Hardening (P1/P2)

Objetivo:

- readiness final para rollout de producao

Entrega minima:

- tuning para repositorios grandes
- acessibilidade por teclado refinada
- cobertura E2E de caminhos criticos e edge-cases

Gate de entrada:

- M3 concluido
- riscos tecnicos abertos classificados e com plano de mitigacao

Gate de saida (Go/No-Go):

- `go test ./...` verde no CI e local
- `npm --prefix frontend run build` verde no CI e local
- E2E critico do Git Panel verde em ambiente limpo
- budgets de performance validados com evidencia (latencia, CPU watcher, responsividade)

## 2. Regras de Avanco Entre Milestones

- sem Gate de saida concluido, milestone nao avanca
- pendencia de seguranca/confiabilidade em P0 bloqueia proxima fase
- features fora do escopo v1 sao movidas para v1.1/P1 sem excecao

## 3. Artefatos Obrigatorios por Milestone

- specs atualizadas no `git_specs/`
- checklist em `GIT_TASKS.md` sincronizado
- evidencias tecnicas (testes/build/metricas) registradas no PR de milestone

## 4. Validacao de Readiness (snapshot em 2026-02-23)

### 4.1 KPIs e evidencias

- latencia `open_panel` <= 300ms: **118.936541ms** (mediana)  
  evidencia: `docs/GIT_PANEL_PERFORMANCE_BASELINE.md`
- latencia `history_first_page` <= 200ms: **83.622916ms** (mediana)  
  evidencia: `docs/GIT_PANEL_PERFORMANCE_BASELINE.md`
- latencia `stage_file` <= 150ms: **61.141791ms** (mediana)  
  evidencia: `docs/GIT_PANEL_PERFORMANCE_BASELINE.md`
- latencia `unstage_file` <= 150ms: **60.151458ms** (mediana)  
  evidencia: `docs/GIT_PANEL_PERFORMANCE_BASELINE.md`
- suite backend local: `go test ./...` **verde**
- suite frontend local: `npm --prefix frontend run build` **verde**
- suite E2E local: `npm --prefix frontend run e2e` **verde**

### 4.2 Status por milestone

- M0 Scope Freeze: **concluido**
- M1 Foundation Runtime: **concluido**
- M2 Core Git Operations: **concluido**
- M3 Diff and Conflict Flow: **concluido**
- M4 Release Hardening: **concluido** (com riscos residuais abaixo)

### 4.3 Riscos tecnicos abertos

- warning de minificacao CSS no build (`Expected identifier` em bundle): nao bloqueia build/testes, mas precisa saneamento antes de release final.
- chunk JS principal acima de 500kB gzip warning do Vite: risco de cold start em ambientes lentos; recomendada estrategia de split por rota/modulo.

### 4.4 Criterio de aceite consolidado

- botao `GitHub` abre tela dedicada do Git Panel: **ok**
- abrir/fechar tela Git preserva estado de terminais/workspace: **ok**
- fluxo principal por teclado (`Cmd/Ctrl+S`, `Cmd/Ctrl+Shift+S`, `J/K`, `Esc`) funcional: **ok**
- conflitos com `Accept Mine/Theirs` e `Open External Tool`: **ok**
- fallback para arquivo grande/binario no diff: **ok**
- write commands serializados por repo sem colisao: **ok**
