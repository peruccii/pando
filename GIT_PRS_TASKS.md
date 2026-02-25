# GIT PRs Tasks (Prioridade)

## P0 - Bloqueadores de entrega (sem isso a aba PRs nao entra em producao)

Ref principal: [GIT_PRS_PRD.md](GIT_PRS_PRD.md)

[x] Fechar open questions de produto e registrar decisao oficial (aba `PRs` no inspector vs modo dedicado, fallback manual de `owner/repo`, coexistencia GraphQL+REST) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 14  
[x] Definir contrato tecnico final de erro para PR REST (`code/message/details`) e padrao unico frontend/backend - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 7  
[x] Implementar resolucao de repositorio remoto alvo a partir de `repoPath` ativo (`origin` -> `owner/repo`) com fallback manual seguro - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.1  
[x] Implementar cliente REST de Pull Requests no backend com headers oficiais (`X-GitHub-Api-Version`, `Accept`) e autenticacao existente - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 5.1  
[x] Implementar endpoint backend para listagem de PRs (`GET /pulls`) com filtros (`open/closed/all`) e paginacao - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.2  
[x] Implementar endpoint backend para detalhe de PR (`GET /pulls/{pull_number}` JSON) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.3  
[x] Implementar endpoint backend para commits da PR (`GET /pulls/{pull_number}/commits`) com paginacao - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.4  
[x] Implementar endpoint backend para arquivos da PR (`GET /pulls/{pull_number}/files`) com tratamento de patch ausente/binario/truncado - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.5  
[x] Implementar endpoint backend para diff completo sob demanda (`Accept: application/vnd.github.diff`) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.6  
[x] Expor novos bindings Wails `GitPanelPR*` para fluxo read-only (list/get/files/commits/rawdiff) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 5.1  
[x] Regenerar e versionar wrappers tipados de frontend (`frontend/wailsjs/*`) apos novos bindings - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 5.1  
[x] Implementar estado/store de PRs no frontend do Git Panel (loading/success/error por bloco, sem bloquear toda tela) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 5.2  
[x] Implementar UI da aba `PRs` no Git Panel (lista + filtros + detalhe + arquivos + commits) com lazy loading por secao - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 5.2  
[x] Implementar acao "Ver diff completo" sob demanda (sem carregar diff bruto no boot) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.6  
[x] Implementar cache read-through com TTL curto (15-30s) por endpoint/parametros - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 6.2  
[x] Implementar ETag/If-None-Match para endpoints de leitura com suporte a 304 - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 6.2  
[x] Implementar polling/refresh context-aware somente com aba `PRs` ativa + pausa quando app minimizado - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 6.3  
[x] Implementar tratamento de erro completo para 401/403/404/409/422/429 com feedback acionavel na UI - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 7  
[x] Implementar telemetria minima de request (metodo, endpoint, status, duracao, cache hit/miss, rate remaining) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 9  
[x] Implementar seguranca minima (sanitizacao de owner/repo, nao logar token, bloqueio de alvo divergente sem confirmacao) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 8  

## P1 - Core de mutacoes de PR

Ref principal: [GIT_PRS_PRD.md](GIT_PRS_PRD.md)

[x] Implementar criacao de PR (`POST /pulls`) no backend e binding `GitPanelPRCreate` - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.7  
[x] Implementar formulario/UI para criar PR no Git Panel com validacao de campos obrigatorios (`title/head/base`) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.7  
[x] Implementar atualizacao de PR (`PATCH /pulls/{pull_number}`) e binding `GitPanelPRUpdate` - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.8  
[x] Implementar UI de edicao de PR (titulo, descricao, base, state, maintainer_can_modify) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.8  
[x] Implementar check de merge (`GET /pulls/{pull_number}/merge`) e binding `GitPanelPRCheckMerged` - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.9  
[x] Implementar merge de PR (`PUT /pulls/{pull_number}/merge`) com metodos `merge/squash/rebase` e `sha` opcional - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.10  
[x] Implementar update branch (`PUT /pulls/{pull_number}/update-branch`) com `expected_head_sha` opcional - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.11  
[x] Implementar invalidacao de cache pos-mutacao e refresh seletivo de lista/detalhe - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 6.2  
[x] Garantir que criar PR no ORCH reflita imediatamente no GitHub apos sucesso de API + refresh visual confiavel - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 12  

## P2 - Hardening, performance e release quality

Ref principal: [GIT_PRS_PRD.md](GIT_PRS_PRD.md)

[x] Implementar backoff com jitter para erros de leitura e secondary rate limit (sem retry cego em mutacoes) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 7  
[] Implementar degradacao robusta para PRs grandes (patch truncado, arquivo binario, lista de arquivos extensa) sem travar UI - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 4.5  
[x] Validar budgets de performance e corrigir gargalos de render/rede na aba `PRs` - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 6.4  
[x] Implementar suite de testes unitarios backend para cliente REST PR (headers, pagina, ETag, mapeamento de erro) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 10.1  
[x] Implementar testes de integracao backend (httptest/mock GitHub API) para fluxos read/write de PR - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 10.1  
[x] Implementar suite de testes frontend (store/UI/estados de erro/degradacao) da aba `PRs` - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 10.2  
[x] Implementar testes E2E criticos (listar PR, abrir detalhe, arquivos, criar PR, merge, update branch) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 10.2  
[x] Revisar observabilidade final e garantir eventos `gitpanel:prs_request`, `gitpanel:prs_cache`, `gitpanel:prs_action_result` - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 9  
[x] Revisar seguranca final (logs, escopo de token, confirmacoes de alvo) antes de rollout - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 8  

## Gates finais (obrigatorio para marcar rollout)

[] `go test ./...` verde no CI e local (incluindo testes de PR REST) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 10  
[] `npm --prefix frontend run build` verde no CI e local - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 10  
[] E2E critico de PRs verde em ambiente limpo - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 10  
[] Budgets de performance validados com evidencia (latencia e responsividade da aba `PRs`) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 6.4  
[] Criterios de aceite da feature todos concluido (incluindo criacao de PR refletindo no GitHub) - [GIT_PRS_PRD.md](GIT_PRS_PRD.md) Secao 12  
