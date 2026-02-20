# Spec: Roadmap Delivery

## 1. Fase 1 - Foundation (P0)

Entrega:

- substituir placeholder do pane por Git Panel base
- layout base inspirado no Git Extensions (View commit log), sem graph de commits
- status staged/unstaged/conflicted
- historico linear paginado
- virtualizacao da lista

Pronto quando:

- fluxo basico de visualizacao de historico e status funciona
- estrutura visual de branches/refs na esquerda e log no centro esta valida e estavel
- sem polling continuo de status

## 2. Fase 2 - Diff and Advanced Staging (P0)

Entrega:

- diff unified e side-by-side
- stage/unstage por hunk
- selecao multipla de linhas para stage parcial

Pronto quando:

- usuario monta commit atomico sem terminal na maioria dos casos

## 3. Fase 3 - Merge and Robustness (P1)

Entrega:

- painel de conflitos
- acoes mine/theirs com opcao de auto-stage
- queue sequencial por repo para write commands
- console de saida opcional

Pronto quando:

- conflitos sao resolvidos no painel com feedback confiavel
- sem erros recorrentes de concorrencia no index

## 4. Fase 4 - Polish (P1)

Entrega:

- tuning de performance para repos grandes
- acessibilidade por teclado refinada
- cobertura de testes e2e ampliada

Pronto quando:

- KPIs de performance e estabilidade atingidos
- UX fluida para uso diario
