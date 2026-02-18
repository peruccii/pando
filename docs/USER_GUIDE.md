# ORCH User Guide (v1.0.0)

## 1. Primeira execução
1. Abra o app ORCH.
2. Complete o onboarding (idioma, tema, primeiro terminal).
3. Use `Cmd+N` para novo terminal e `Cmd+K` para abrir a Command Palette.

## 2. Command Center
- `Cmd+N`: novo painel terminal/agente.
- `Cmd+W`: fecha painel ativo.
- `Cmd+Enter`: Zen mode.
- `Cmd+Shift+B`: Broadcast mode.

## 3. Sessão colaborativa
1. Abra `Session Panel`.
2. Clique em **Start Session** e selecione modo:
- `Live Share`: terminal local compartilhado.
- `Docker`: ambiente sandbox com container isolado.
3. Compartilhe o código curto com convidados.
4. Aprove/rejeite pedidos na waiting room.

## 4. Segurança em sessão
- Permissão padrão de convidado: `read_only`.
- Concessão `read_write` exige confirmação de risco.
- Revogação de escrita é imediata em tempo real.
- Reinício de ambiente disponível para sessões Docker.
- Auditoria disponível no painel da sessão.

## 5. GitHub e PRs
- PRs/Issues/Branches são atualizados por polling inteligente.
- Em sessão colaborativa, mudanças detectadas são broadcast via P2P.
- Diffs grandes usam carregamento progressivo de hunks.

## 6. Atalhos principais
- `Cmd+1...9`: focar painel por índice.
- `Cmd+[`, `Cmd+]`: navegar entre painéis.
- `Cmd+,`: abrir Settings.
- `Escape`: sair de modal/zen/broadcast.

## 7. Troubleshooting rápido
- Sem backend Wails: rode `wails dev`.
- E2E local: `cd frontend && npm run e2e`.
- Build frontend: `cd frontend && npm run build`.
- Testes backend: `go test ./...`.
