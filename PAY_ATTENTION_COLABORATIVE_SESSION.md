# PAY_ATTENTION_COLABORATIVE_SESSION

Este arquivo existe para evitar regressão no fluxo de **Collaborative Session (host -> guest com workspace escopada)**.
Se você (IA) vai mexer em qualquer coisa de sessão colaborativa, **leia isto antes**.

## Trajetória resumida (como chegamos no comportamento correto)

### Fase 1: bug inicial de hidratação

- Sintoma inicial: guest não recebia `WS-A` ou recebia `WS-A` com `0` terminais.
- Ajustes que destravaram:
  - retry de `workspace_scope_request` mais robusto;
  - critério de "sync concluído" no guest mais rígido;
  - snapshot do host combinando `workspaceStore` + `layoutStore`.

### Fase 2: regressão de UX/estado local do guest

- Sintoma: workspaces locais do guest sumiam ao entrar na sessão.
- Causa: `applySessionWorkspaceSnapshot` sobrescrevia `workspaces` com 1 item.
- Correção: atualizar/inserir workspace escopada sem destruir a lista local.

### Fase 3: regressão de visibilidade da workspace escopada

- Sintoma: `WS-A` não aparecia no guest em alguns cenários.
- Causa: materialização da workspace escopada condicionada ao estado de P2P.
- Correção: materializar aba escopada imediatamente ao entrar em sessão (hidratação dos terminais vem depois).

### Fase 4: causa raiz crítica de handshake (log ESSENCIAL)

Logs fornecidos que fecharam diagnóstico:

- Host backend mostrava:
  - `Stored SDP offer from host for session ...`
  - `guest ... connected to session ... (role: guest)`
- Guest frontend **não** mostrava:
  - `SDP Answer sent (peer=host)`

Conclusão: o guest conectava tarde e perdia o `sdp_offer` já emitido.

Correção definitiva:

- Backend de signaling passou a guardar offer pendente por guest (`HostOffers`) e fazer replay quando guest conecta:
  - `Replayed pending SDP offer to guest ...`
- Após isso, fluxo correto voltou:
  - guest envia `SDP Answer`,
  - P2P conecta,
  - workspace escopada e terminais aparecem.

## Objetivo que DEVE continuar funcionando

1. Host cria sessão com uma workspace alvo (ex.: `WS-A` com 3 terminais).
2. Guest entra com código e aguarda aprovação.
3. Host aprova.
4. Conexão P2P fecha handshake (offer/answer + data channels).
5. Guest vê a workspace escopada com os terminais do host.
6. Ao encerrar sessão no host, guest volta ao estado local normal sem workspace fantasma.

## Arquivos críticos (não mexer sem extremo cuidado)

- `frontend/src/features/session/hooks/useSession.ts`
- `frontend/src/stores/workspaceStore.ts`
- `frontend/src/features/session/services/P2PConnection.ts`
- `internal/session/signaling.go`
- `internal/session/types.go`

## Invariantes obrigatórias

### 1) Signaling precisa tolerar corrida de conexão (offer antes do guest conectar)

- O host pode enviar `sdp_offer` **antes** do guest abrir o websocket de signaling.
- O backend deve armazenar offer por guest e fazer replay quando guest conectar.
- Implementação atual:
  - `SignalingSession.HostOffers map[targetUserID]offer`
  - `replayPendingOfferForGuest(...)` chamado em `HandleWebSocket` após registrar guest.
  - Ao receber `sdp_answer`, remover pending offer daquele guest.

Se remover isso, o sintoma volta: guest conecta em signaling, mas nunca envia `SDP Answer`.

### 2) Workspace escopada do guest é efêmera (não persistir no DB do guest)

- Não persistir workspace de sessão via `SyncGuestWorkspace` para o guest.
- Persistência gerou bug de “workspace fantasma” após fim da sessão.
- Binding local de workspace escopada deve ser de sessão/memória (ID efêmero).

### 3) `applySessionWorkspaceSnapshot` NÃO pode apagar todas as workspaces locais

- Antes: sobrescrevia `workspaces = [scopedWorkspace]` e escondia tudo do guest.
- Correto: atualizar/inserir a workspace escopada sem destruir o conjunto local.
- Durante sessão, ativa escopo; ao terminar, remove escopo e recarrega estado local.

### 4) Guest precisa materializar workspace escopada imediatamente

- A aba da workspace escopada deve aparecer mesmo antes da hidratação de agentes.
- Não depender de `isP2PConnected` para criar a visão escopada inicial.
- P2P hidrata terminais depois.

### 5) Critério de “sync concluído” no guest não pode ser ingênuo

- `workspace_scope_request` retry só para quando houver evidência real de sync recebido.
- Só existir workspace local por si só não prova hidratação.
- Deve haver marcação explícita de recebimento de `workspace_scope_sync`.

### 6) Snapshot do host precisa refletir estado operacional real

- Snapshot da workspace escopada deve combinar:
  - dados persistidos (`workspaceStore`)
  - panes/terminais vivos (`layoutStore`) como fonte operacional.
- Evita sync com `agents=[]` quando DB/store está defasado.

## Logs de saúde esperados (fluxo bom)

No host (backend):

- `Stored SDP offer from host ...`
- `guest ... connected to session ... (role: guest)`
- `Replayed pending SDP offer to guest ...` (quando houver corrida)
- `Stored SDP answer from guest ...`
- `P2P connection established ... user <guest>`

No guest (frontend):

- `[P2P] Connected as guest to session ...`
- `[P2P] SDP Answer sent (peer=host)`
- `[P2P] Peer host state: connected` (ou conectando -> connected)
- abertura de data channels (`terminal-io`, `control`, etc.)

## Logs ruidosos (não confundir com bug principal)

- Erros de `wails.localhost`/HMR em dev podem aparecer e não quebrar colaboração.
- Erros de `User-Initiated Abort / close called` no fechamento são comuns no teardown.

## Sinais de regressão (PARE e investigue)

1. Guest aprovado, mas não aparece `SDP Answer sent`.
2. Guest vê workspace escopada com `0` terminais quando host tem terminais vivos.
3. Workspaces locais do guest somem ao entrar na sessão e voltam depois.
4. Após encerrar sessão, fica workspace fantasma da colaboração no guest.

## Diagnóstico rápido (Sintoma -> Causa provável -> Onde olhar primeiro)

| Sintoma observado | Causa provável | Onde olhar primeiro |
| --- | --- | --- |
| Guest aprovado, mas sem `SDP Answer sent` | `sdp_offer` não chegou ao guest (corrida de conexão sem replay) | `internal/session/signaling.go` (`HostOffers`, `replayPendingOfferForGuest`) + logs `[SIGNALING]` |
| Guest conecta, mas não abre data channels | Handshake WebRTC incompleto (offer/answer/ICE) ou peer não chegou em `connected` | `frontend/src/features/session/services/P2PConnection.ts` + logs `[P2P]` host/guest |
| `WS-A` aparece com `0` terminais | Snapshot do host desatualizado ou sync incompleto de workspace scope | `frontend/src/features/session/hooks/useSession.ts` (`buildScopedHostWorkspaceSnapshot`, `workspace_scope_sync`) |
| Workspaces locais do guest somem ao entrar | Snapshot da sessão sobrescrevendo lista inteira de workspaces | `frontend/src/stores/workspaceStore.ts` (`applySessionWorkspaceSnapshot`) |
| Após encerrar sessão, sobra workspace fantasma | Persistência indevida da workspace escopada no guest | `frontend/src/features/session/hooks/useSession.ts` (binding efêmero, sem `SyncGuestWorkspace`) |
| Guest não materializa `WS-A` ao entrar | Criação da workspace escopada dependente de P2P conectado | `frontend/src/features/session/hooks/useSession.ts` (`ensureGuestWorkspaceScope`) |

## Funções sensíveis (dobrar o cuidado ao mexer)

- `internal/session/signaling.go`: `replayPendingOfferForGuest`
- `internal/session/signaling.go`: fluxo de `handleMessage` para `sdp_offer` e `sdp_answer`
- `frontend/src/stores/workspaceStore.ts`: `applySessionWorkspaceSnapshot`
- `frontend/src/features/session/hooks/useSession.ts`: `requestGuestWorkspaceScopeSync`
- `frontend/src/features/session/hooks/useSession.ts`: `buildScopedHostWorkspaceSnapshot`
- `frontend/src/features/session/hooks/useSession.ts`: `ensureGuestWorkspaceScope`
- `frontend/src/features/session/hooks/useSession.ts`: `applyGuestWorkspaceScopeSync`

## Checklist mínimo obrigatório após qualquer mudança em colaboração

1. Host com múltiplas workspaces (`WS-A` escopada + outras), com terminais abertos.
2. Guest com workspaces locais próprias.
3. Criar sessão no host -> guest entra -> host aprova.
4. Confirmar guest:
   - recebeu workspace escopada,
   - recebeu terminais,
   - P2P conectado/data channels abertos.
5. Encerrar sessão no host.
6. Confirmar guest:
   - voltou às workspaces locais,
   - sem workspace fantasma da sessão.
7. Rodar:
   - `go test ./internal/session -run Signaling -count=1`
   - `go test ./...`
   - `npm --prefix frontend run build`

## Regra de ouro para IA

Se a mudança mexe em signaling, sync de workspace escopada, ou lifecycle de sessão:

- Não simplifique lógica por “achismo”.
- Não remova retries/flags de hidratação sem evidência.
- Não altere persistência do guest sem validar fantasma/sumir workspaces.
- Não mergear sem executar checklist completo acima.

## Prompt padrão (copiar e colar antes de mexer em colaboração)

Use este prompt para forçar disciplina técnica:

```txt
Leia primeiro /Users/perucci/Desktop/www/orch/PAY_ATTENTION_COLABORATIVE_SESSION.md e siga estritamente.

Tarefa: [descrever mudança].

Regras obrigatórias:
1) Não quebrar invariantes do fluxo host->guest com workspace escopada.
2) Antes de codar, diga quais invariantes podem ser afetadas e em quais arquivos.
3) Implementar a menor mudança possível, sem simplificações perigosas.
4) Preservar tolerância a corrida de signaling (offer antes do guest conectar).
5) Não reintroduzir persistência indevida de workspace escopada no guest.
6) Executar checklist de regressão do arquivo e reportar resultado objetivo.
7) Em caso de conflito entre mudança solicitada e invariantes, parar e explicar com clareza.

Entregue:
- diff resumido,
- riscos residuais,
- evidências de teste (logs/comandos).
```
