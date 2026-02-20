# PRD - Git Panel (Source Control) para ORCH

## 1. Contexto

O botao "GitHub" da tela inicial do ORCH hoje abre um pane placeholder.
No v1 desta feature, o clique deve abrir uma tela dedicada do Git Panel (fora do mosaico de terminais).
O objetivo deste PRD e definir a implementacao de um painel de controle de Git local, inspirado em Git Extensions, com foco em:

- velocidade de staging
- clareza de diff
- operacao por teclado
- comportamento previsivel em repositorios grandes

Importante: apesar do nome visual atual ser "GitHub", o dominio desta funcionalidade e Git local (working tree, index, history, merge conflicts).

## 2. Objetivo do Produto

Entregar um painel de Source Control no ORCH que permita ao desenvolvedor:

- inspecionar historico em lista linear de alta performance
- montar commits atomicos com stage parcial (hunk/linhas)
- revisar alteracoes side-by-side com scroll sincronizado
- resolver conflitos de merge com acoes rapidas

Sem depender de polling continuo e sem comprometer responsividade do app.

## 3. Metas e Nao-Metas

### 3.1 Metas

- Carregamento inicial perceptivelmente rapido para historico local.
- Fluxo de stage/unstage/discard confiavel.
- UX de diff legivel e navegavel por teclado.
- Integracao com file watcher existente para refresh orientado a eventos.
- Execucao segura de comandos Git concorrentes via fila sequencial.

### 3.2 Nao-Metas (v1)

- Graph de commits visual complexo.
- Rebase interativo, reflog explorer, bisect wizard.
- Paridade total com clientes Git desktop maduros.
- Resolucao semiautomatica de conflitos em tres vias.

## 4. Publico-Alvo

- Desenvolvedor individual usando ORCH como command center.
- Tech Lead que precisa revisar mudancas locais rapidamente antes de commit/push.

## 5. Escopo Funcional (v1)

### 5.1 Historico Linear (Performance First)

Descricao:

- lista unica de commits com infinite scroll
- sem renderizacao de grafo
- layout inspirado em "View commit log" do Git Extensions: branches/refs na esquerda e log no centro

Requisitos:

- Backend via `git log` com streaming e paginacao por cursor.
- Parsing robusto com separadores seguros (`%x1f` e `%x1e`) para evitar quebra por `|` em mensagem.
- Frontend com virtualizacao (ex.: react-window) para renderizar apenas viewport.
- UI moderna e profissional, sem copiar visual legado e sem adicionar graph de commits.

Criterios de aceite:

- primeira pagina visivel em ate 200ms para repositorio medio local
- scroll sem stutter perceptivel em listas longas

### 5.2 Stage Parcial (Hunk e Linhas)

Descricao:

- selecionar hunks e subconjuntos de linhas para stage

Requisitos:

- leitura base por `git diff -U3`
- geracao de patch parcial no frontend/backend
- aplicacao via `git apply --cached --unidiff-zero` (ou estrategia equivalente validada)
- suporte a unstage por arquivo e por trecho staged
- selecao multipla com `Shift+Click`

Criterios de aceite:

- usuario consegue criar commit atomico sem ir ao terminal
- patch parcial invalido mostra erro claro e opcao de retry/reset selecao

### 5.3 Diff Side-by-Side

Descricao:

- visualizacao "Original" e "Modified" lado a lado

Requisitos:

- parser de diff no backend (Git raw parsing; biblioteca opcional)
- scroll lock-step configuravel
- highlight de sintaxe para linguagens principais (JS/TS, Go, Rust, Python)
- lazy render para arquivos grandes

Criterios de aceite:

- navegacao por arquivo e por hunk sem travamento
- sincronizacao de scroll consistente entre os dois paines

### 5.4 Merge Conflict Management

Descricao:

- detectar e resolver conflitos comuns direto no painel

Requisitos:

- detectar merge ativo por `.git/MERGE_HEAD`
- listar arquivos em conflito via status porcelain (`UU`, etc.)
- acoes rapidas:
  - Accept Mine (`git checkout --ours -- <file>` + opcao de stage)
  - Accept Theirs (`git checkout --theirs -- <file>` + opcao de stage)
  - Open External Tool (configuravel)

Criterios de aceite:

- conflitos aparecem em ate 300ms apos evento do watcher
- acoes atualizam estado da UI sem reiniciar app

## 6. Requisitos Nao Funcionais

### 6.1 Performance

- startup da tela dedicada do Git Panel: < 300ms para primeira pintura
- primeira pagina de historico: < 200ms (repositorio medio)
- tempo de resposta para stage/unstage: < 150ms em arquivos pequenos/medios
- fallback para arquivos grandes:
  - sem preview automatico para arquivo > 1MB (configuravel)
  - sem syntax highlight para arquivo muito grande

### 6.2 Confiabilidade

- operacoes de escrita Git devem ser serializadas (fila unica por repositorio)
- nenhuma operacao de escrita executa em paralelo no mesmo repo
- erros de `index.lock` devem ser tratados com retry curto e mensagem clara

### 6.3 Observabilidade

- proibido polling de `git status` em loop
- refresh orientado por eventos do file watcher + debounce
- cada comando Git executado gera evento de diagnostico (duracao, exit code, stderr sanitizado)
- console de saida opcional na UI para transparencia tecnica

### 6.4 Seguranca

- sanitizacao de paths para evitar traversal (`..`, paths absolutos fora repo)
- comandos sempre com `--` antes de path quando aplicavel
- sem execucao shell concatenada; usar argumentos estruturados

## 7. Arquitetura Proposta

### 7.1 Frontend

- Reutilizar o botao/entrada visual `GitHub` para abrir uma tela dedicada do novo `GitPanel` (fora do Command Center).
- Modulos principais:
  - `BranchesSidebar` (contexto de branch atual, ahead/behind, refs/filtros quando disponivel)
  - `CommitLogList` (virtualizado + infinite scroll)
  - `InspectorPanel` (abas para WorkingTree, Commit, Diff, Conflicts)
  - `GitCommandConsole` (opcional)

### 7.2 Backend (Go / Wails)

- Novo service de dominio Git (ex.: `internal/gitpanel`) ou evolucao do `internal/gitactivity`.
- Responsabilidades:
  - leitura de status/historico/diff
  - aplicacao de stage parcial
  - queue sequencial de comandos write por repo
  - normalizacao de erros

### 7.3 Integracoes existentes

- `internal/filewatcher`: fonte primaria de invalidacao de cache/refresh.
- `internal/gitactivity`: pode ser mantido para timeline e audit trail.
- bindings Wails: expandir superficie de API para historico paginado e hunk staging.

## 8. Contrato de API (proposta inicial)

Leitura:

- `GitPanelGetStatus(repoPath) -> { staged[], unstaged[], conflicted[], branch, aheadBehind }`
- `GitPanelGetHistory(repoPath, cursor, limit) -> { items[], nextCursor }`
- `GitPanelGetDiff(repoPath, filePath, mode, contextLines) -> DiffModel`
- `GitPanelGetConflicts(repoPath) -> ConflictFile[]`

Escrita:

- `GitPanelStageFile(repoPath, filePath)`
- `GitPanelUnstageFile(repoPath, filePath)`
- `GitPanelStagePatch(repoPath, patchText)`
- `GitPanelUnstagePatch(repoPath, patchText)`
- `GitPanelDiscardFile(repoPath, filePath)`
- `GitPanelAcceptOurs(repoPath, filePath, autoStage)`
- `GitPanelAcceptTheirs(repoPath, filePath, autoStage)`

Eventos:

- `gitpanel:status_changed`
- `gitpanel:history_invalidated`
- `gitpanel:conflicts_changed`
- `gitpanel:command_result`

## 9. UX e Atalhos (macOS first)

Principios:

- minimalista
- baixo ruido visual
- alta densidade de informacao util
- referencia de layout: Git Extensions (View commit log), com acabamento moderno e profissional

Layout alvo (sem grafos):

- esquerda: branches/refs (com branch atual obrigatoria e filtros de contexto)
- centro: commit log linear com foco em leitura rapida
- direita: painel de detalhes (commit metadata, diff, estado staged/unstaged/conflicted)
- responsivo: em largura reduzida, painel da direita vira area inferior colapsavel

Atalhos alvo:

- `Cmd+Enter` commit
- `Cmd+S` stage selecao atual
- `Cmd+Shift+S` unstage selecao atual
- `Cmd+D` toggle diff
- `J/K` navegacao em lista
- `Shift+Click` range select

Feedback obrigatorio:

- status de acao (`Running`, `Success`, `Error`)
- comando Git executado (modo console opcional)
- erros com mensagem tecnica compacta + acao recomendada

## 10. Roadmap de Entrega

### Fase 1 - Foundation (P0)

- substituir placeholder atual por tela dedicada base do Git Panel acionada pelo botao GitHub
- implementar layout base inspirado no Git Extensions (sem grafos): branches/refs na esquerda, commit log no centro e inspector dedicado
- status staged/unstaged/conflicted
- historico linear paginado (sem grafos)
- virtualizacao da lista

### Fase 2 - Diff e staging avancado (P0)

- diff side-by-side com scroll sync
- stage/unstage por hunk
- stage multiplo por selecao de linhas

### Fase 3 - Merge e robustez (P1)

- painel de conflitos
- acoes Accept Mine/Theirs
- command queue sequencial por repo
- console de saida opcional

### Fase 4 - Polish (P1)

- afinacao de performance para repositorios grandes
- acessibilidade por teclado
- cobertura de testes de regressao

## 11. Criterios de Aceite (Definition of Done)

- O botao "GitHub" abre tela dedicada do Git Panel funcional (nao placeholder).
- Abrir/fechar a tela Git nao reinicia nem perde estado dos terminais ativos.
- O layout final remete ao fluxo de "View commit log" (branches/refs na esquerda e historico no centro), com visual moderno/profissional.
- Usuario consegue:
  - ver historico paginado
  - ver diff de arquivo
  - stage/unstage arquivo
  - stage parcial de hunk/linhas
  - resolver conflito basico com Mine/Theirs
- Nao existe polling continuo de `git status`.
- Operacoes write passam por fila sequencial e nao colidem em `index.lock`.
- Falhas exibem erro claro sem congelar UI.
- Testes criticos de parser de diff e patch parcial passam.

## 12. Riscos e Mitigacoes

Risco: patch parcial invalido em cenarios edge-case.

- Mitigacao: validacao previa de patch, fallback para stage por hunk completo, mensagens de erro acionaveis.

Risco: arquivos gigantes/binarios degradam UI.

- Mitigacao: limite de preview por tamanho, modo simplificado sem highlight.

Risco: corrida entre watcher e comandos locais.

- Mitigacao: invalida cache por evento + reconciliacao apos cada comando write concluido.

Risco: encoding heterogeneo.

- Mitigacao: leitura defensiva, fallback de exibicao e indicacao de encoding nao suportado.

## 13. Plano de Testes

Backend:

- parser de `git log` com delimitadores seguros
- parser de diff e geracao de patch parcial
- serializacao da fila de comandos write
- validacao/sanitizacao de path

Frontend:

- virtualizacao da lista e paginacao
- selecao de linhas/hunks
- estados de loading/erro/sucesso
- atalhos de teclado criticos

E2E:

- repo com alteracoes simples
- repo com conflito de merge
- arquivo grande e binario
- concorrencia de operacoes write

## 14. Dependencias

- Git CLI disponivel no host
- file watcher ja operacional (existente no ORCH)
- bindings Wails adicionais para Git Panel

## 15. Open Questions

- O nome visual do botao/tela permanece "GitHub" ou muda para "Git" no v1?
- Commit action entra no v1 ou fica para v1.1?
- External merge tool sera configuravel na primeira entrega ou fixo por ambiente?

## 16. Decisao de Escopo Recomendada

Para reduzir risco e entregar valor rapido:

- v1: historico linear + layout inspirado no Git Extensions (sem grafos) + diff + stage/unstage + conflitos basicos
- v1.1: line-level staging completo + console avancado + refinamentos de UX
