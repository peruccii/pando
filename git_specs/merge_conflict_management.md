# Spec: Merge Conflict Management

## 1. Objetivo

Detectar e resolver conflitos de merge com fluxo minimo, rapido e seguro.

## 2. Deteccao de Estado

- merge ativo: existencia de `.git/MERGE_HEAD`
- arquivos conflitados: `git status --porcelain=v1 -z` com codigos de conflito (`UU`, `AA`, `DD`, `AU`, `UA`, `DU`, `UD`)

## 3. Requisitos Funcionais

- exibir lista dedicada de arquivos em conflito
- acao `Accept Mine`
- acao `Accept Theirs`
- acao opcional `Open External Tool`
- opcao de auto-stage apos resolucao

## 4. Comandos

- accept mine:
  - `git checkout --ours -- <file>`
- accept theirs:
  - `git checkout --theirs -- <file>`
- opcional auto-stage:
  - `git add -- <file>`
- open external tool:
  - `git mergetool --no-prompt -- <file>`

## 5. Regras de Negocio

- toda acao de conflito passa pela queue sequencial
- path validation obrigatoria
- se auto-stage falhar, informar que resolucao foi aplicada mas stage nao concluiu

## 6. Estado de UI

- badge "Merge in progress"
- cada arquivo conflitado mostra estado local de execucao
- log resumido da acao e resultado

## 7. Criterios de Aceite

- conflito aparece no painel em <= 300ms apos evento do watcher
- `Mine/Theirs` atualiza status sem reiniciar app
- erros sao claros e com instrucoes de proximo passo
