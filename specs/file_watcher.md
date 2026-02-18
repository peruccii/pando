# Spec: File Watcher (.git)

> **Módulo**: Transversal — File System  
> **Status**: Draft  
> **PRD Ref**: Seção 7.4  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Monitorar a pasta `.git` do projeto ativo para detectar mudanças de branch, commits e alterações feitas via terminal (fora da GUI). Quando detectada uma mudança, a **GUI atualiza automaticamente** o estado visual.

---

## 2. Eventos Monitorados

| Arquivo/Dir Monitorado   | Evento Detectado              | Ação na GUI                              |
| ------------------------- | ----------------------------- | ---------------------------------------- |
| `.git/HEAD`               | Mudança de branch (checkout)  | Atualizar BranchSelector + invalidar cache de PRs |
| `.git/refs/heads/*`       | Novo commit (push/commit)     | Atualizar contadores + invalidar cache    |
| `.git/MERGE_HEAD`         | Merge em andamento            | Exibir indicador "Merge in progress"      |
| `.git/FETCH_HEAD`         | Fetch do remote               | Atualizar estado de branches remotas      |
| `.git/index`              | Stage/Unstage de arquivos     | Atualizar status de arquivos (se visível)  |
| `.git/COMMIT_EDITMSG`     | Commit sendo preparado        | Indicador visual                           |

---

## 3. Implementação (Backend Go)

### 3.1 Interface

```go
type IFileWatcher interface {
    Watch(projectPath string) error
    Unwatch(projectPath string) error
    OnChange(handler func(event FileEvent))
    GetCurrentBranch(projectPath string) (string, error)
    GetLastCommit(projectPath string) (*CommitInfo, error)
}

type FileEvent struct {
    Type      string    // "branch_changed", "commit", "merge", "fetch", "index"
    Path      string    // Caminho do arquivo alterado
    Timestamp time.Time
    Details   map[string]string // Detalhes extras (nova branch, etc.)
}

type CommitInfo struct {
    Hash    string
    Message string
    Author  string
    Date    time.Time
}
```

### 3.2 Watcher com fsnotify

```go
import "github.com/fsnotify/fsnotify"

type GitWatcher struct {
    watcher  *fsnotify.Watcher
    handlers []func(FileEvent)
    debounce map[string]*time.Timer // Debounce por arquivo
}

func (w *GitWatcher) Watch(projectPath string) error {
    gitDir := filepath.Join(projectPath, ".git")

    // Monitorar arquivos específicos
    paths := []string{
        gitDir,
        filepath.Join(gitDir, "refs", "heads"),
        filepath.Join(gitDir, "refs", "remotes"),
    }

    for _, p := range paths {
        if err := w.watcher.Add(p); err != nil {
            return err
        }
    }

    go w.eventLoop(projectPath)
    return nil
}

func (w *GitWatcher) eventLoop(projectPath string) {
    for {
        select {
        case event, ok := <-w.watcher.Events:
            if !ok { return }

            // Debounce: 200ms para evitar rafais de eventos
            key := event.Name
            if timer, exists := w.debounce[key]; exists {
                timer.Stop()
            }
            w.debounce[key] = time.AfterFunc(200*time.Millisecond, func() {
                fileEvent := w.classifyEvent(event, projectPath)
                if fileEvent != nil {
                    for _, handler := range w.handlers {
                        handler(*fileEvent)
                    }
                }
            })

        case err, ok := <-w.watcher.Errors:
            if !ok { return }
            log.Error("FileWatcher error:", err)
        }
    }
}

func (w *GitWatcher) classifyEvent(event fsnotify.Event, projectPath string) *FileEvent {
    name := filepath.Base(event.Name)

    switch {
    case name == "HEAD":
        branch, _ := w.readCurrentBranch(projectPath)
        return &FileEvent{
            Type: "branch_changed",
            Path: event.Name,
            Details: map[string]string{"branch": branch},
        }

    case strings.Contains(event.Name, "refs/heads"):
        return &FileEvent{
            Type: "commit",
            Path: event.Name,
            Details: map[string]string{"ref": name},
        }

    case name == "MERGE_HEAD":
        return &FileEvent{Type: "merge", Path: event.Name}

    case name == "FETCH_HEAD":
        return &FileEvent{Type: "fetch", Path: event.Name}

    case name == "index":
        return &FileEvent{Type: "index", Path: event.Name}
    }

    return nil
}
```

### 3.3 Leitura de Branch Atual

```go
func (w *GitWatcher) readCurrentBranch(projectPath string) (string, error) {
    headPath := filepath.Join(projectPath, ".git", "HEAD")
    data, err := os.ReadFile(headPath)
    if err != nil { return "", err }

    content := strings.TrimSpace(string(data))

    // "ref: refs/heads/main" → "main"
    if strings.HasPrefix(content, "ref: refs/heads/") {
        return strings.TrimPrefix(content, "ref: refs/heads/"), nil
    }

    // Detached HEAD (hash direto)
    return content[:8] + "... (detached)", nil
}
```

---

## 4. Frontend — Reação a Eventos

```typescript
// Escutar eventos do FileWatcher via Wails Events
wails.Events.On('git:branch_changed', (event) => {
    const { branch } = event.data
    // Atualizar BranchSelector
    githubStore.setCurrentBranch(branch)
    // Invalidar cache de PRs (branch pode ter PRs diferentes)
    githubStore.invalidateCache()
})

wails.Events.On('git:commit', (event) => {
    // Atualizar indicadores visuais
    githubStore.refreshBranchStatus()
})

wails.Events.On('git:merge', (event) => {
    // Exibir badge "Merge in progress"
    githubStore.setMergeInProgress(true)
})
```

---

## 5. Performance

| Aspecto             | Especificação                     |
| -------------------- | --------------------------------- |
| Debounce             | 200ms por arquivo                 |
| Polling fallback     | Nenhum (fsnotify é event-driven)  |
| CPU overhead         | Negligível (kernel events)        |
| Startup              | Watch inicia com o workspace       |
| Cleanup              | Unwatch ao fechar workspace        |

---

## 6. Dependências

| Dependência               | Tipo       |
| -------------------------- | ---------- |
| `github.com/fsnotify/fsnotify` | Bloqueador |
| github_integration (cache) | Consumer   |
| command_center_ui          | Consumer   |
