# Spec: Product Scope

## 1. Objetivo

Definir o escopo do Git Panel do ORCH para substituir o placeholder do botao/pane "GitHub" por uma funcionalidade de Source Control local, inspirada em Git Extensions, focada em velocidade e clareza.

## 2. Problema Resolvido

- excesso de troca de contexto entre terminal e app
- baixa visibilidade do estado staged/unstaged/conflicted
- dificuldade para montar commits atomicos sem fluxo visual

## 3. Metas do v1

- historico linear performatico com infinite scroll
- layout inspirado no "View commit log" do Git Extensions (sem grafos)
- workflow completo de stage/unstage/discard
- diff legivel com modo side-by-side
- resolucao basica de conflitos (Mine/Theirs)
- refresh por file watcher, sem polling continuo

## 4. Nao-Metas do v1

- grafos de commit complexos
- rebase interativo e tooling avancado de historico
- paridade total com clientes Git desktop maduros
- merge editor de tres vias embutido

## 5. Personas

- dev individual que usa ORCH como command center
- tech lead/reviewer que precisa validar alteracoes locais rapidamente

## 6. Definicoes de Dominio

- `working tree`: alteracoes locais nao staged
- `index`: alteracoes staged prontas para commit
- `conflicted`: arquivo em estado de conflito durante merge/rebase
- `write command`: qualquer comando que altera index, working tree ou refs locais

## 7. Guardrails de Escopo

- manter interface simples e orientada a lista
- evitar overengineering visual
- manter acabamento moderno e profissional (sem replicar visual legado)
- preferir comandos Git nativos e robustos
- priorizar confiabilidade sobre "features wow"

## 8. Definition of Done (produto)

- pane deixa de ser placeholder
- usuario faz fluxo local de source control sem sair do ORCH
- layout final segue padrao de commit log com branches/refs na esquerda e historico no centro
- estados de erro sao claros e recuperaveis
- performance atende budget definido em NFR
