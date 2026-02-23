#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

echo "[gitpanel-perf] measuring baseline against latency budgets"
go test ./internal/gitpanel -run '^TestGitPanelLatencyBudgets$' -count=1 -v

echo "[gitpanel-perf] measuring watcher burst coalescing and flush responsiveness"
go test . -run '^TestGitPanelWatcherBurstBudgets$' -count=1 -v
