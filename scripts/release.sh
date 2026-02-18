#!/bin/bash
# Release helper for ORCH.
set -euo pipefail

VERSION="${1:-1.0.0}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "${ROOT_DIR}"

echo "[release] validating security checklist"
./scripts/validate_security_checklist.sh

echo "[release] packaging ORCH ${VERSION}"
./scripts/build.sh "${VERSION}"

echo "[release] completed for ${VERSION}"
