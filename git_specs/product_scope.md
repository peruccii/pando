# Spec: Product Scope

## 1. Objetivo

Fechar o escopo de v1 do Git Panel com fronteira tecnica clara: o que entra no release, o que fica fora e quando a entrega deve ser cortada para evitar rollout fraco.

## 2. Problema Resolvido

- excesso de troca de contexto entre terminal e app
- baixa visibilidade do estado staged/unstaged/conflicted
- risco de operacoes write em repo errado sem validacao de contexto

## 3. Escopo Funcional Fechado do v1 (MUST)

1. Entrada unica: clique no botao `GitHub` abre uma tela dedicada do `Git Panel`, fora do mosaico de terminais.
2. Host dedicado com layout oficial de 3 colunas:
   - esquerda: branches/refs e contexto do repo
   - centro: historico linear virtualizado
   - direita: inspector (Working Tree, Commit, Diff, Conflicts)
3. Leitura funcional:
   - status `staged/unstaged/conflicted`, branch atual, ahead/behind
   - historico paginado com cursor
   - diff por arquivo (unified e split) com fallback para arquivo grande/binario
4. Escrita funcional base:
   - `stage`, `unstage`, `discard` por arquivo
   - operacoes sempre pela queue sequencial por repositorio
5. Seguranca e confiabilidade:
   - preflight de runtime (Git CLI, `repoPath` valido, repo acessivel)
   - validacao de paths com bloqueio de traversal
   - erro normalizado (`code/message/details`) em todos os bindings
6. Sincronizacao de estado:
   - invalidacao por file watcher e reconciliacao leve pos-write
   - proibido polling continuo de `git status`
7. Compatibilidade e navegacao:
   - migracao do fluxo legado `github` para tela dedicada sem quebrar snapshots/layout salvo
   - abrir/fechar tela Git nao mata nem recria terminais ativos

## 4. Limites e Nao-Metas do v1 (CUT para v1.1+)

- commit action na UI (`Cmd+Enter`) e composer de commit
- stage parcial por hunk e por linha
- external merge tool configuravel
- console avancado de comando com historico longo
- graph de commits, rebase interativo, merge editor de tres vias
- qualquer feature que introduza polling continuo, shell concatenada ou risco de write fora do repo

## 5. Criterio de Corte (Go/No-Go)

### 5.1 Regra de release

- release de v1 so acontece se **100% do escopo MUST (secao 3)** estiver concluido e validado.
- item incompleto da secao 3 bloqueia rollout (No-Go), sem excecao.

### 5.2 Regra de downgrade

- qualquer item da secao 4 pode ser removido do release sem renegociar o v1.
- itens cortados da secao 4 entram automaticamente no backlog de v1.1/P1.

### 5.3 Gate minimo de qualidade

- `go test ./...` verde
- `npm --prefix frontend run build` verde
- smoke P0 de parser/queue/bindings verde
- budgets de latencia definidos em NFR respeitados

## 6. Decisao Oficial de UX (2026-02-22)

### 6.1 User goal e acao primaria

- objetivo do usuario: revisar e preparar mudancas Git locais sem sair do ORCH
- acao primaria da tela: selecionar arquivos/commits e executar operacoes Git locais com feedback imediato

### 6.2 Layout e hierarquia oficial

- tela dedicada do Git Panel (fora do mosaico de terminais)
- abertura pelo botao `GitHub` (entrypoint mantido no v1 por compatibilidade)
- hierarquia fixa: esquerda (refs) -> centro (commit log) -> direita (inspector)

### 6.3 CTA e acoes secundarias

- CTA primario de entrada: `GitHub`
- acoes secundarias no painel: stage, unstage, discard, abrir diff, resolver conflito (Mine/Theirs)

### 6.4 Estados obrigatorios

- empty: repo sem alteracoes/sem commits
- loading: historico/status em carregamento incremental
- success: acao write concluida com atualizacao local
- error: mensagem curta com proxima acao (retry, abrir terminal, revisar path)

### 6.5 Micro-interactions e feedback

- cada acao mostra estado local `running/success/error`
- feedback deve ser inline no item afetado (arquivo, hunk ou conflito)
- transicoes devem preservar foco e contexto do item selecionado

### 6.6 Acessibilidade minima do v1

- ordem de foco previsivel da esquerda para direita
- atalhos e botoes com label ARIA claro
- contraste suficiente para leitura de diff e estado de erro

## 7. Personas

- dev individual que usa ORCH como command center
- tech lead/reviewer que precisa validar alteracoes locais rapidamente

## 8. Definicoes de Dominio

- `working tree`: alteracoes locais nao staged
- `index`: alteracoes staged prontas para commit
- `conflicted`: arquivo em estado de conflito durante merge/rebase
- `write command`: qualquer comando que altera index, working tree ou refs locais

## 9. Guardrails de Escopo

- simplicidade operacional acima de "feature bloat"
- comandos Git nativos com argumentos estruturados (sem shell concatenada)
- observabilidade suficiente para diagnostico sem expor dado sensivel
- qualquer ganho visual nao pode degradar performance/base de confiabilidade

## 10. Definition of Done (produto)

- placeholder legado deixa de existir e fluxo passa para tela dedicada
- usuario faz fluxo local de source control sem sair do ORCH
- layout oficial (refs esquerda, historico centro, inspector direita) esta estavel
- estados de erro sao claros, recuperaveis e acionaveis
- budgets de performance e gates de teste estao cumpridos

## 11. Compatibilidade `github` legado -> Git Panel dedicado

### 11.1 Layout salvo (UserConfig.LayoutState)

- ao restaurar layout legado, qualquer pane com `type=github` deve ser removido antes de hidratar o mosaico
- se houver pelo menos um pane legado removido, o app deve abrir a tela dedicada via `git-panel:open`
- panes `terminal` e `ai_agent` devem ser preservados sem reorder forcado
- snapshots de terminal nao podem ser descartados por causa da migracao

### 11.2 Workspaces e agentes persistidos

- ao carregar workspaces, agentes com `type=github` sao tratados como legado e removidos da estrutura ativa
- a migracao nao pode alterar `activeWorkspaceId`
- a migracao nao pode encerrar sessoes PTY validas
- apos migracao, o workspace deve continuar carregando com os agentes restantes e mesmo estado de terminais

### 11.3 Criterio de seguranca da migracao

- migracao deve ser idempotente (rodar mais de uma vez sem efeito colateral)
- falha na limpeza legado nao pode bloquear abertura do app nem carregamento de terminais

## 12. Politica Oficial de Criacao de Pane/Agente (v1+)

- proibido criar novos panes do tipo `github`
- proibido criar novos agentes persistidos com `agentType=github`
- entrada `GitHub` da UI sempre abre tela dedicada (`git-panel:open`)
- tentativas de criar `agentType=github` via API retornam erro acionavel orientando o fluxo correto

## 13. Estrategia Oficial de Resolucao do `repoPath` Ativo (v1)

Objetivo: garantir que operacoes write (`stage`, `unstage`, `discard`, `accept mine/theirs`) nunca executem no repositorio errado.

### 13.1 Fontes de contexto e prioridade

1. `terminal context` (prioridade maxima):
   - usar `cwd` do terminal ativo quando houver pane terminal selecionado
   - resolver para raiz Git via `git rev-parse --show-toplevel`
2. `workspace path` (fallback primario):
   - usar `workspace.path` da workspace ativa quando nao houver terminal elegivel
   - resolver para raiz Git via `git -C <workspace.path> rev-parse --show-toplevel`
3. `fallback manual` (obrigatorio quando 1/2 falham):
   - usuario escolhe repo explicitamente (picker/path input)
   - valor selecionado vira contexto ativo ate mudanca de workspace/terminal

### 13.2 Regras de elegibilidade

- somente path que:
  - existe localmente
  - e diretorio
  - pertence a repositorio Git valido
- para write, `repoPath` deve estar em estado `resolved` com confianca alta
- se houver ambiguidade (multiplos repos candidatos), nao escolher implicitamente

### 13.3 Modelo de decisao de confianca

- `high`: resolvido por terminal ativo ou workspace com unica raiz Git valida
- `medium`: resolvido por ultimo repo manual ainda valido
- `low`: nenhum repo valido ou multiplos candidatos sem selecao explicita

Regra de seguranca:

- `low` bloqueia qualquer write e mostra erro acionavel (`selecionar repositorio`)
- `medium` permite leitura; write exige confirmacao explicita no primeiro comando da sessao
- `high` permite leitura e write sem prompt adicional

### 13.4 Cache e invalidação

- manter `resolvedRepoPath` por `workspaceID`
- invalidar quando:
  - muda `activeWorkspaceId`
  - muda terminal ativo/cwd
  - watcher detectar alteracao estrutural de `.git` (repo removido/renomeado)
- fallback manual expira quando path deixa de ser repo valido

### 13.5 Contrato minimo de erro

- `E_REPO_NOT_RESOLVED`: sem contexto suficiente para operar
- `E_REPO_AMBIGUOUS`: mais de um candidato sem escolha manual
- `E_REPO_INVALID`: path nao e repo Git acessivel
- `E_REPO_OUT_OF_SCOPE`: path fora do escopo permitido de workspace/politica

### 13.6 Criterio de aceite

- write nunca ocorre sem `repoPath` validado
- mudanca de terminal/workspace reavalia contexto antes de qualquer write
- fallback manual cobre 100% dos casos sem terminal ativo
