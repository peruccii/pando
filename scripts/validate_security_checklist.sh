#!/bin/bash
# Validate core security checklist before release.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

echo "[security] running Go tests"
go test ./... >/dev/null

echo "[security] checking log sanitizers"
rg -n "LogSanitizer|SecretSanitizer|\[REDACTED\]" internal >/dev/null

echo "[security] checking docker hardening flags"
rg -n -- "--security-opt|--read-only|--tmpfs|--network" internal/docker/service.go >/dev/null

echo "[security] checking permission revocation flow"
rg -n "permission_revoked|permission_change|NotifyPermissionChange" app.go internal/session frontend/src/features/session >/dev/null

echo "[security] checking frontend build"
cd frontend
npm run build >/dev/null
cd "${ROOT_DIR}"

echo "[security] checklist status: PASS"
