# ORCH v1.0.0

## Highlights
- Sessões colaborativas com modos `Live Share` e `Docker`.
- Revogação imediata de permissão de escrita.
- Auditoria de sessão no painel (eventos de segurança e colaboração).
- Broadcast P2P de mudanças de PR detectadas por polling.
- Diff viewer com lazy loading de hunks grandes.
- Cursor awareness multiusuário e CRDT de input com Yjs.
- Virtualização aprimorada para cenários com 10+ terminais.
- Testes E2E de fluxos críticos (Playwright).

## Build / Distribution
- Build universal macOS + pacote `.dmg` via `./scripts/build.sh 1.0.0`.

## Quality gates
- Backend: `go test ./...`
- Frontend: `npm run build`
- E2E: `npm run e2e`
- Security gate: `./scripts/validate_security_checklist.sh`
