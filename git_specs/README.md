# Git Specs Index

Este diretorio concentra as especificacoes derivadas de `GIT_PRD.md`.

## 1. Fonte de Verdade e Fluxo de Rastreabilidade

Ordem oficial de decisao:

1. `GIT_PRD.md` define objetivo e requisitos de produto.
2. `git_specs/*.md` detalham contrato tecnico por dominio.
3. `GIT_TASKS.md` materializa backlog, prioridade e progresso.
4. codigo/testes implementam os contratos das specs.

Regra:

- se houver conflito entre implementacao e spec, a implementacao deve ser ajustada ou a spec atualizada com decisao explicita.

## 2. Mapa de Specs por Dominio

- `product_scope.md`: escopo v1, limites, criterio de corte, compatibilidade legado, estrategia de `repoPath`.
- `ux_shortcuts.md`: blueprint oficial de layout e navegacao da tela dedicada.
- `api_contract.md`: bindings Wails, DTOs e contrato de erro.
- `working_tree_staging.md`: status parser e writes de staging.
- `history_engine.md`: historico linear paginado e virtualizado.
- `diff_viewer.md`: modelo de diff, modos e degradacao.
- `merge_conflict_management.md`: deteccao e acoes de conflito.
- `command_queue.md`: serializacao e retry de writes por repositorio.
- `filewatcher_integration.md`: invalidacao por eventos e reconciliacao.
- `observability_security.md`: diagnostico, logs e guardrails de seguranca.
- `performance_reliability.md`: budgets e metas de estabilidade.
- `test_strategy.md`: cobertura backend/frontend/e2e.
- `roadmap_delivery.md`: milestones e gates de entrada/saida.

## 3. Matriz de Rastreabilidade (PRD -> Specs)

- Escopo do produto e decisoes v1: `GIT_PRD.md` -> `product_scope.md`
- UX da tela dedicada Git: `GIT_PRD.md` -> `ux_shortcuts.md`
- Contratos backend/frontend: `GIT_PRD.md` -> `api_contract.md`
- Status/staging/discard: `GIT_PRD.md` -> `working_tree_staging.md`
- Historico linear: `GIT_PRD.md` -> `history_engine.md`
- Diff viewer: `GIT_PRD.md` -> `diff_viewer.md`
- Merge conflicts: `GIT_PRD.md` -> `merge_conflict_management.md`
- Confiabilidade de writes: `GIT_PRD.md` -> `command_queue.md`
- Sync sem polling: `GIT_PRD.md` -> `filewatcher_integration.md`
- Seguranca/observabilidade: `GIT_PRD.md` -> `observability_security.md`
- Performance e budgets: `GIT_PRD.md` -> `performance_reliability.md`
- Qualidade e gates de teste: `GIT_PRD.md` -> `test_strategy.md`
- Planejamento por fases: `GIT_PRD.md` -> `roadmap_delivery.md`

## 4. Checklist de Consistencia do Pacote Tecnico

Usar este checklist antes de marcar milestone como concluida:

- [ ] `GIT_PRD.md` sem open questions pendentes para o milestone atual.
- [ ] cada item P0 em `GIT_TASKS.md` referencia ao menos uma spec.
- [ ] toda spec alterada tem impacto refletido em `GIT_TASKS.md`.
- [ ] contratos de API/erro em `api_contract.md` batem com bindings reais.
- [ ] politicas de seguranca/path validation aparecem em `product_scope.md` e `observability_security.md`.
- [ ] estrategia de `repoPath` e compativel com fluxo de write do staging/conflict.
- [ ] roadmap (`roadmap_delivery.md`) com gates de entrada/saida coerentes com testes/budgets.
- [ ] `test_strategy.md` cobre parser, queue, validacao de path, UI state e E2E critico.
- [ ] budgets de `performance_reliability.md` usados como criterio de Go/No-Go.
- [ ] backlog de v1.1/P1 separado do corte v1 (sem leakage de escopo).

## 5. Criterio de Pronto do Pacote de Specs

Pacote tecnico e considerado consistente quando:

- rastreabilidade PRD -> specs -> tasks esta completa;
- nenhum requisito P0 esta sem dono explicito em spec;
- gates de milestone e qualidade estao objetivos e verificaveis.
