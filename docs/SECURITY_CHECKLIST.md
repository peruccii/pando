# Security Checklist (Release Gate)

## Core controls
- [x] Tokens/segredos são sanitizados em logs e auditoria (`[REDACTED]`).
- [x] Sessões Docker usam flags restritivas (`--security-opt`, `--read-only`, `--tmpfs`, `--network`).
- [x] Permissões de convidado (`read_only`/`read_write`) com revogação imediata.
- [x] Eventos auditáveis persistidos no SQLite com retenção (1000 por sessão).
- [x] Reinício de ambiente Docker auditado.

## Collaboration controls
- [x] Waiting room com approve/reject.
- [x] Mudanças de permissão propagadas em tempo real.
- [x] Cursor awareness e input CRDT (Yjs) para colaboração concorrente.

## Verification commands
- `go test ./...`
- `cd frontend && npm run build`
- `cd frontend && npm run e2e`
- `./scripts/validate_security_checklist.sh`

## Status
- [x] Aprovado para release v1.0.0
