# Spec: Working Tree and Staging

## 1. Objetivo

Permitir organizacao de commits atomicos por arquivo, hunk e linha, com fluxo confiavel para stage/unstage/discard.

## 2. Requisitos Funcionais

- listar alteracoes em:
  - staged
  - unstaged
  - conflicted
- stage/unstage por arquivo
- discard por arquivo com confirmacao
- stage parcial por hunk
- stage parcial por linhas selecionadas (`Shift+Click`)

## 3. Fontes de Dados

Leitura de estado:

```bash
git status --porcelain=v1 -z
```

Leitura de diff:

```bash
git diff --unified=3 -- <file>
git diff --cached --unified=3 -- <file>
```

## 4. Operacoes Write

- stage arquivo:
  - `git add -- <file>`
- unstage arquivo:
  - `git restore --staged -- <file>`
- discard arquivo:
  - `git checkout -- <file>`
- stage parcial:
  - gerar patch valido e aplicar com `git apply --cached --unidiff-zero`

## 5. Regras de Negocio

- selecao parcial invalida nao pode corromper index
- se `git apply` falhar, retornar erro explicito e manter selecao
- paths devem ser validados para permanecer dentro do repo
- operacoes write devem passar pela fila sequencial do repo

## 6. Estado de UI

- `idle`
- `running`
- `success`
- `error`

Cada arquivo/linha em acao precisa de estado local para evitar clique duplicado.

## 7. Criterios de Aceite

- usuario consegue montar commit atomico sem terminal
- stage/unstage/discard atualizam UI sem reload global do app
- falhas de patch parcial sao recuperaveis com feedback claro

## 8. Nao Funcional

- tempo alvo para stage/unstage por arquivo pequeno: <= 150ms
- limite de retries para lock: definido na spec de queue

