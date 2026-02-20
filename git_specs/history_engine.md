# Spec: History Engine

## 1. Objetivo

Implementar historico linear de commits, com alta performance para repositorios grandes, sem renderizacao de grafos.

## 2. Requisitos Funcionais

- listagem linear de commits ordenada por data/ordem do Git
- carregamento incremental (infinite scroll)
- busca por autor/hash/trecho de mensagem (fase 1.1)
- atualizacao quando refs locais mudarem

## 3. Backend Design

Comando base:

```bash
git log --date=iso-strict --pretty=format:%H%x1f%h%x1f%an%x1f%aI%x1f%s%x1e
```

Regras:

- usar delimitadores seguros (`\x1f`, `\x1e`) no parser
- paginacao por cursor (hash do ultimo commit retornado)
- limite padrao por pagina: 200
- suporte a cancelamento por contexto (timeout/cancel user action)

## 4. Frontend Design

- lista virtualizada (ex.: react-window)
- renderizar apenas itens visiveis
- disparar `load more` quando chegar proximo ao fim da viewport
- exibir loading skeleton para pagina seguinte

## 5. Modelo de Dados

```ts
type HistoryItem = {
  hash: string
  shortHash: string
  author: string
  authoredAt: string
  subject: string
}
```

## 6. Criterios de Aceite

- primeira pagina disponivel em <= 200ms em repositorio medio local
- scroll fluido sem travamento perceptivel
- parsing resiliente para mensagens com pipe, tab e caracteres especiais

## 7. Edge Cases

- repo vazio (mostrar estado Empty)
- detached HEAD (mostrar label apropriada)
- shallow clone (historico parcial)
- erro de permissao/acesso ao repo

