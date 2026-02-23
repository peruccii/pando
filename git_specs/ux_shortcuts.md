# Spec: UX and Shortcuts

## 1. Objetivo

Definir UX oficial do Git Panel v1: blueprint de layout inspirado no Git Extensions (sem grafos) e navegacao da tela dedicada sem impacto no estado dos terminais/workspace.

## 2. User Goal e Acao Primaria

- user goal: revisar e operar Git local com menor troca de contexto possivel
- acao primaria: selecionar itens (refs, commits, arquivos) e executar operacoes de source control com feedback imediato
- restricao chave: Git Panel fica fora do mosaico; terminais continuam vivos em background

## 3. Blueprint Oficial de Layout (v1)

Referencia visual: fluxo "View commit log" do Git Extensions, com acabamento moderno/profissional e sem graph.

### 3.1 Hierarquia desktop

- esquerda (20-24%): `BranchesSidebar`
  - branch atual (obrigatorio)
  - ahead/behind
  - refs principais e filtros de contexto
- centro (46-56%): `CommitLogList`
  - lista linear virtualizada
  - foco de leitura rapida
  - infinite scroll
- direita (24-32%): `InspectorPanel`
  - abas `Working Tree`, `Commit`, `Diff`, `Conflicts`
  - acoes locais de stage/unstage/discard e resolucao de conflito
- rodape opcional: `GitCommandConsole` (v1.1+)

### 3.2 Responsivo

- largura reduzida: inspector desce para area inferior colapsavel
- sidebar de refs pode colapsar para drawer, mas commit log permanece area principal
- proibido remover commit log como coluna central dominante

### 3.3 Regras de composicao

- um CTA primario por tela (entrada via botao `GitHub`)
- alto sinal visual para estado selecionado (branch/commit/arquivo)
- sem elementos de graph, sem excesso de chrome visual

## 4. Navegacao da Tela Dedicada (oficial v1)

### 4.1 Entradas permitidas

- clique no botao `GitHub` da Empty State
- acao `new-github` na Command Palette (abre tela dedicada, nao cria pane)
- evento de migracao legado `github` -> `git-panel:open`

### 4.2 Abertura

- evento `git-panel:open` ativa `GitPanelScreen`
- workspace ativo nao muda
- layout do Command Center nao e recalculado
- nenhum terminal e encerrado, recriado ou reinicializado

### 4.3 Fechamento/volta

- botao `Voltar` no header da tela Git
- `Esc` fecha tela Git e retorna para a vista anterior do app
- evento `git-panel:close` possui o mesmo efeito do botao `Voltar`

### 4.4 Garantias de estado

- `activeWorkspaceId` preservado
- `paneOrder`, `mosaicNode` e `activePaneId` preservados
- sessoes PTY continuam executando em background
- snapshots de terminal continuam validos; abrir/fechar Git Panel nao os invalida

## 5. Interacoes Principais

- click em branch/ref atualiza contexto do log
- click em commit atualiza metadados e diff no inspector
- click em arquivo atualiza diff
- click em hunk/linha seleciona escopo de acao (quando feature habilitada)
- conflito exibe acoes rapidas `Mine` e `Theirs`

## 6. Atalhos

- `Cmd+S`: stage selecao atual
- `Cmd+Shift+S`: unstage selecao atual
- `Cmd+D`: toggle diff panel
- `J/K`: navegar lista de itens
- `Alt+1/2/3/4`: mover foco entre sidebar, commit log, inspector e acoes
- `Enter`: abrir/fechar item selecionado
- `Esc`: fechar overlays/dialogs e voltar da tela Git
- `Cmd+Enter`: commit (v1.1; fora do corte do v1)

## 7. Estados da Tela

- empty: repo sem commits/alteracoes, com CTA de proxima acao
- loading: skeleton para status/historico/diff incremental
- success: feedback local no item acionado (`success`)
- error: erro curto com acao sugerida (`retry`, `abrir terminal`, `revisar path`)

## 8. Micro-interactions e Feedback

- cada acao mostra estado local `running/success/error`
- feedback e inline por item afetado (arquivo, conflito, hunk)
- manter foco no contexto atual apos refresh/reconciliacao
- evitar toasts globais para operacoes granulares de lista

## 9. Acessibilidade

- ordem de foco: sidebar -> commit log -> inspector -> acoes
- foco visivel e consistente em teclado
- labels ARIA claras para acoes de Git
- navegacao de lista por teclado (setas/JK) respeitando contexto focado
- contraste suficiente para diff/estado de erro e badges de status

## 10. Criterios de Aceite

- blueprint final tem refs na esquerda, historico no centro e inspector dedicado
- Git Panel abre em tela dedicada e fecha por `Voltar`/`Esc` sem perda de estado
- nenhum novo pane do mosaico e criado ao acionar `GitHub`
- fluxo principal de source control pode ser executado por teclado
