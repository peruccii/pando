# Spec: API Contract (Wails Bindings)

## 1. Objetivo

Definir contrato entre frontend e backend para o Git Panel, com tipagem clara e erros previsiveis.

## 2. Bindings de Leitura

- `GitPanelGetStatus(repoPath string) (StatusDTO, error)`
- `GitPanelGetHistory(repoPath string, cursor string, limit int) (HistoryPageDTO, error)`
- `GitPanelGetDiff(repoPath string, filePath string, mode string, contextLines int) (DiffDTO, error)`
- `GitPanelGetConflicts(repoPath string) ([]ConflictFileDTO, error)`

## 3. Bindings de Escrita

- `GitPanelStageFile(repoPath string, filePath string) error`
- `GitPanelUnstageFile(repoPath string, filePath string) error`
- `GitPanelDiscardFile(repoPath string, filePath string) error`
- `GitPanelStagePatch(repoPath string, patchText string) error`
- `GitPanelUnstagePatch(repoPath string, patchText string) error`
- `GitPanelAcceptOurs(repoPath string, filePath string, autoStage bool) error`
- `GitPanelAcceptTheirs(repoPath string, filePath string, autoStage bool) error`

## 4. Eventos Runtime

- `gitpanel:status_changed`
- `gitpanel:history_invalidated`
- `gitpanel:conflicts_changed`
- `gitpanel:command_result`

## 5. DTOs Minimos

`StatusDTO`:

- `branch string`
- `ahead int`
- `behind int`
- `staged []FileChangeDTO`
- `unstaged []FileChangeDTO`
- `conflicted []ConflictFileDTO`

`FileChangeDTO`:

- `path string`
- `status string`
- `added int`
- `removed int`

`HistoryPageDTO`:

- `items []HistoryItemDTO`
- `nextCursor string`
- `hasMore bool`

`CommandResultDTO`:

- `commandId string`
- `action string`
- `status string`
- `durationMs int`
- `error string`

## 6. Contrato de Erro

Frontend deve receber erro normalizado:

- `code` (ex.: `E_REPO_NOT_FOUND`, `E_GIT_LOCK`, `E_PATCH_INVALID`)
- `message` (humano)
- `details` (tecnico resumido)

## 7. Compatibilidade

- novos campos devem ser adicionados de forma backward-compatible
- frontend ignora campos desconhecidos

