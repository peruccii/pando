# Spec: FileWatcher Integration

## 1. Objetivo

Garantir sincronizacao da UI com estado Git real usando eventos do file watcher, sem polling continuo.

## 2. Principio

- proibido loop de `git status` por timer fixo
- refresh orientado por eventos `.git`

## 3. Eventos de Entrada

- `git:branch_changed`
- `git:commit`
- `git:commit_preparing`
- `git:index`
- `git:merge`
- `git:fetch`

## 4. Estrategia de Debounce/Coalesce

- backend watcher: debounce curto por path
- frontend: debounce adicional para burst handling
- consolidar eventos para reduzir refresh redundante

## 5. Matriz de Invalidação

- `git:index`: invalidar working tree/staging
- `git:merge`: invalidar conflito e status
- `git:branch_changed`: invalidar branch atual e historico
- `git:commit`: invalidar historico e staged
- `git:fetch`: opcionalmente invalidar indicadores ahead/behind

## 6. Reconciliacao Pos-Write

Mesmo com watcher:

- apos cada comando write concluido, executar reconciliacao leve do estado do painel
- objetivo: evitar janela de inconsistencia caso evento seja perdido

## 7. Criterios de Aceite

- sem polling de status em loop
- burst de mudancas nao gera spike de CPU perceptivel
- UI converge para estado correto apos write local
