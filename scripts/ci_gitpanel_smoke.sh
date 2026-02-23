#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

echo "[gitpanel-smoke] running parser + queue + performance budgets"
go test ./internal/gitpanel -count=1 -v -run '^(TestParsePorcelainStatusZIncludesBranchAndBuckets|TestWriteQueueSerializesCommandsPerRepository|TestBasicWriteActionsUseQueueSequentially|TestGitPanelLatencyBudgets)$'

echo "[gitpanel-smoke] running app-level smoke (bindings + watcher burst budget)"
go test . -count=1 -v -run '^(TestGitPanelBindingsRequireService|TestGitPanelPreflightNormalizesErrors|TestGitPanelWatcherBurstBudgets)$'

echo "[gitpanel-smoke] installing frontend dependencies"
npm --prefix frontend ci

echo "[gitpanel-smoke] building frontend"
npm --prefix frontend run build

echo "[gitpanel-smoke] completed successfully"
