# Spec: Performance and Reliability

## 1. Objetivo

Estabelecer budgets de performance e regras de resiliencia para o Git Panel.

## 2. Performance Budgets

- abrir pane Git (primeira pintura): <= 300ms
- primeira pagina de historico: <= 200ms
- stage/unstage arquivo pequeno/medio: <= 150ms
- atualizacao de conflito apos evento watcher: <= 300ms

## 3. Estrategias

- virtualizacao para listas longas
- carregamento lazy de diff/hunks
- parsing incremental (streaming) no backend
- cache leve de estado atual com invalidacao por eventos

## 4. Fallbacks

- arquivo > 1MB: nao carregar preview automaticamente
- arquivo binario: sem renderizacao textual
- erro de parser diff: fallback para raw text

## 5. Confiabilidade

- write commands sempre na queue por repo
- retries para lock transiente
- cancelamento seguro em operacoes longas
- reconciliacao de estado apos write

## 6. Criterios de Aceite

- sem freeze perceptivel com 10k+ commits no historico
- sem condicao de corrida write/write no mesmo repo
- sem regressao de CPU por burst de eventos `.git`

