# Spec: UX and Shortcuts

## 1. Objetivo

Definir comportamento de interface do Git Panel com foco em velocidade, legibilidade e fluxo por teclado no macOS.

## 2. Estrutura de Tela (v1)

- referencia visual: Git Extensions (View commit log), sem graph de commits
- coluna esquerda: contexto de branches/refs (branch atual, ahead/behind e filtros)
- coluna central: commit log linear
- coluna direita: inspector com abas (Commit, Working Tree, Diff, Conflicts)
- rodape opcional: console de saida Git
- direcao visual: moderna e profissional (hierarquia clara, densidade alta e baixo ruido)

## 3. Interacoes Principais

- click em branch/ref atualiza o contexto do log (quando a fonte estiver disponivel)
- click em commit atualiza metadados e diff no inspector
- click em arquivo atualiza diff
- click em hunk abre acao de stage parcial
- selecao de range por `Shift+Click`
- conflito exibe acoes rapidas (`Mine`, `Theirs`)

## 4. Atalhos

- `Cmd+Enter`: commit (quando habilitado)
- `Cmd+S`: stage selecao atual
- `Cmd+Shift+S`: unstage selecao atual
- `Cmd+D`: toggle diff panel
- `J/K`: navegar lista de itens
- `Enter`: abrir/fechar item selecionado
- `Esc`: fechar overlays/dialogs

## 5. Feedback

- cada acao mostra estado local:
  - running
  - success
  - error
- erro deve incluir acao sugerida (retry, reset selecao, abrir terminal)
- console opcional mostra comando executado e tempo

## 6. Acessibilidade

- foco visivel por teclado
- labels ARIA em botoes de acoes Git
- contraste suficiente em dark mode

## 7. Criterios de Aceite

- fluxo principal de source control pode ser executado apenas por teclado
- usuario entende claramente o que foi executado e o resultado
- layout final lembra o fluxo do Git Extensions para commit log, com acabamento moderno/profissional e sem grafos
