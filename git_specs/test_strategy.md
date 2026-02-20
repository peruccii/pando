# Spec: Test Strategy

## 1. Objetivo

Definir cobertura minima para garantir qualidade do Git Panel em cenarios reais.

## 2. Backend Tests

- parser de historico com delimitadores seguros
- parser de diff e tokenizacao basica
- geracao e aplicacao de patch parcial
- validacao de path e bloqueio de traversal
- queue sequencial:
  - ordem FIFO
  - retry de lock
  - timeout/cancelamento

## 3. Frontend Tests

- render virtualizado de lista historico
- navegacao keyboard-first
- estados `loading/success/error`
- interacoes de stage/unstage/discard
- selecao multipla por `Shift+Click`
- fallback de arquivos grandes/binarios no diff

## 4. E2E Tests

- fluxo happy path:
  - editar arquivo
  - stage parcial
  - validar diff
- conflito de merge:
  - detectar conflito
  - executar mine/theirs
- concorrencia:
  - disparar writes rapidos no mesmo repo
  - validar queue sem colisoes

## 5. Test Data

- repositorio pequeno (smoke)
- repositorio medio (performance basica)
- arquivos grandes e binarios
- mensagens de commit com caracteres especiais

## 6. Gates de Qualidade

- testes backend/tsx obrigatorios em CI
- e2e critico rodando antes de release
- regressao de performance bloqueia merge quando ultrapassa budget acordado

