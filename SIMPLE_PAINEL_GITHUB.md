# SIMPLE PAINEL GITHUB (V1)

## 1. Objetivo

Construir um painel simples e persistente de atividades Git/GitHub no ORCH, com entrada direta pelo Titlebar e foco em:

- visibilidade imediata do que acabou de acontecer no repo
- detalhes úteis para staging (`git add`) e mudança de branch
- reversão rápida para ações simples e seguras

## 2. Meta de UX (V1)

### Objetivo do usuário

Entender rapidamente "quem fez o quê, em qual repo/branch, e quando", sem sair do ORCH.

### Ação primária

Abrir o painel pelo Titlebar e inspecionar os últimos eventos.

### Hierarquia visual

- Titlebar: resumo persistente da atividade + contador de não lidos.
- Painel: timeline por ordem temporal, cada item com mensagem curta e metadados.
- Detalhes: seção expansível por item com arquivos staged, stats e diff.
- Ações: `Unstage` e `Discard` por arquivo com confirmação.

### Estados obrigatórios

- Empty: sem eventos ainda.
- Loading: buscando eventos.
- Success: timeline renderizada.
- Error: falha ao carregar detalhe/diff/ação.

## 3. Status atual (implementado)

### Backend

- [x] Serviço de atividade Git com buffer em memória e deduplicação temporal.
- [x] Deduplicação adicional no `filewatcher` para bursts de eventos no mesmo path.
- [x] Mapeamento de eventos `git:*` para eventos canônicos de atividade.
- [x] Enriquecimento básico de evento: ator, repo, branch, timestamp.
- [x] Bindings Wails:
  - [x] `GitActivityList`
  - [x] `GitActivityGet`
  - [x] `GitActivityClear`
  - [x] `GitActivityCount`
  - [x] `GitActivityGetStagedFiles`
  - [x] `GitActivityGetStagedDiff`
  - [x] `GitActivityUnstageFile`
  - [x] `GitActivityDiscardFile`
- [x] Coleta de staged files (`numstat` + `name-status`) para `index_updated`.
- [x] Log cru (`[FileWatcher][raw]`) controlado por env (`ORCH_FILEWATCHER_DEBUG_RAW`).

### Frontend

- [x] Feature `git-activity` com store, hook e painel.
- [x] Titlebar persistente com última atividade e badge de não lidos.
- [x] Abertura/fechamento do painel por clique no Titlebar.
- [x] Atalho `Cmd+Shift+G` para abrir/fechar painel.
- [x] Timeline com expansão de detalhes.
- [x] Ações `Unstage` e `Discard` no item de staging.

### Testes

- [x] Testes unitários backend para serviço de atividade e parsing básico de git ops.
- [x] Build frontend validado localmente.
- [ ] Suite completa `go test ./...` ainda pendente em ambiente com rede liberada.

## 4. Backlog ordenado (o que falta)

## Fase 1 — Qualidade de sinal (P0)

- [x] Reduzir ruído de eventos na origem (`filewatcher`) agregando lock/write/rename da mesma operação em uma janela curta.
- [x] Garantir 1 evento semântico por ação relevante (branch change, index update, commit preparing).
- [x] Reduzir logs verbosos (`[FileWatcher][raw]`) para nível debug/configurável.

## Fase 2 — Detalhe de diff e segurança (P0)

- [ ] Limitar tamanho de diff retornado para UI (truncamento com aviso "diff parcial").
- [ ] Sanitizar paths de arquivo em ações de reversão para evitar path traversal.
- [ ] Garantir fallback claro quando `git` retornar erro (mensagem amigável na UI + log técnico no backend).

## Fase 3 — UX de feedback (P1)

- [ ] Mostrar feedback explícito após ação (`Unstage`/`Discard`): sucesso, erro e atualização automática da linha.
- [ ] Exibir indicador de ação em progresso por arquivo (loading state local no botão).
- [ ] Melhorar texto da timeline para ficar consistente:
  - [ ] `"perucci criou a branch X no repo Y às HH:MM"`
  - [ ] `"perucci adicionou N arquivos ao stage no repo Y às HH:MM"`

## Fase 4 — Acessibilidade e navegação (P1)

- [ ] Garantir foco por teclado no painel (ordem previsível e `Esc` para fechar).
- [ ] Adicionar labels acessíveis nos botões de ação por arquivo.
- [ ] Validar contraste dos chips/status no tema atual.

## Fase 5 — Cobertura de testes (P1)

- [ ] Backend:
  - [ ] teste de dedupe para bursts de eventos reais do `.git`
  - [ ] teste de truncamento de diff
  - [ ] teste de validação de path nas ações
- [ ] Frontend:
  - [ ] render de timeline com tipos mistos de evento
  - [ ] expand/collapse de detalhe
  - [ ] fluxo de `Unstage` e `Discard` (sucesso e erro)

## 5. Critérios de aceite V1

- [ ] Após `git checkout -b feature/x`, aparece atividade persistente no Titlebar e item único na timeline.
- [ ] Item de branch mostra ator, repo, branch e horário.
- [ ] Após `git add .`, item mostra arquivos staged com `+/-` por arquivo.
- [ ] "Ver diff" abre conteúdo (com truncamento quando necessário).
- [ ] `Unstage` e `Discard` funcionam sem reiniciar app e atualizam o item.
- [ ] A duplicação visual de eventos é baixa e não polui a leitura do painel.

## 6. V2+ (futuro)

- Reversão por hunk.
- Ações de branch (checkout anterior, delete local com proteção).
- Ações de PR no mesmo painel (abrir PR, status, merge checks).
- Contexto colaborativo por sessão (mostrar participante remoto que acionou o evento).
