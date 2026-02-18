# 🔄 Persistir & Restaurar Última Sessão de CLIs

> **Objetivo**: Quando o usuário fechar o ORCH com CLIs abertas (Gemini, Claude, Codex, OpenCode), ao reabrir o app, todas as sessões devem ser restauradas automaticamente no **exato estado** em que estavam — usando os comandos nativos de **resume/continue** de cada CLI.

---

## Contexto

O ORCH é um orquestrador de CLI. Desenvolvedores trabalham com múltiplas CLIs de IA abertas simultaneamente. Quando o app fecha, os processos PTY morrem e as sessões se perdem. Cada CLI de IA tem comandos nativos para retomar a última sessão:

| CLI | Comando de Resume |
|---|---|
| **Gemini CLI** | `gemini --resume` ou `gemini -r` |
| **Claude Code** | `claude --continue` ou `claude -c` |
| **Codex CLI** | `codex resume --last` |
| **OpenCode** | `opencode --continue` ou `opencode -c` |

**Regra**: Isso se aplica **APENAS** a CLIs que estavam abertas antes do fechamento. Se o pane era um terminal simples (zsh/bash) sem uma CLI de IA rodando, ele reabre normalmente como terminal limpo.

---

## Arquitetura da Solução

```
┌─────────────────────────────────────────────────────────────┐
│                     SHUTDOWN FLOW                            │
│                                                             │
│  1. Frontend emite evento "app:before-shutdown"             │
│  2. Para cada pane com terminal ativo:                      │
│     ├─ Detectar qual CLI está rodando (process sniffing)    │
│     └─ Salvar no DB: {paneId, cliType, cwd, shell, config} │
│  3. Backend persiste array de TerminalSnapshot no SQLite    │
│  4. DestroyAll() roda normalmente                           │
│                                                             │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                     STARTUP FLOW                             │
│                                                             │
│  1. restoreLayout() restaura panes do layoutState           │
│  2. Para cada pane restaurado do tipo "terminal":           │
│     ├─ Buscar TerminalSnapshot correspondente no DB         │
│     ├─ Se snapshot.cliType != "" (era uma CLI de IA):       │
│     │   └─ CreateTerminal → Write(resumeCommand)            │
│     └─ Se snapshot.cliType == "" (terminal simples):        │
│         └─ CreateTerminal normal (sem resume)               │
│  3. Frontend recria sessões PTY com parâmetros corretos     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## Tasks

### Task 1 — Modelo de Dados: `TerminalSnapshot`
**Arquivo**: `internal/database/models.go`
**Prioridade**: 🔴 Alta (bloqueante)

Criar o model `TerminalSnapshot` que persiste o estado de cada terminal no momento do shutdown:

```go
// TerminalSnapshot persiste o estado de um terminal CLI para restauração.
type TerminalSnapshot struct {
    ID        uint      `gorm:"primaryKey" json:"id"`
    PaneID    string    `gorm:"not null;index" json:"paneId"`        // ID do pane no layout (ex: "pane-3")
    CLIType   string    `gorm:"default:''" json:"cliType"`           // "gemini" | "claude" | "codex" | "opencode" | "" (terminal simples)
    Shell     string    `gorm:"default:/bin/zsh" json:"shell"`       // Shell usado
    Cwd       string    `json:"cwd"`                                 // Diretório de trabalho
    UseDocker bool      `gorm:"default:false" json:"useDocker"`      // Se estava rodando em Docker
    PaneTitle string    `json:"paneTitle"`                            // Nome do pane (para restaurar o título)
    PaneType  string    `gorm:"default:terminal" json:"paneType"`    // "terminal" | "ai_agent"
    Config    string    `gorm:"type:text" json:"config,omitempty"`   // JSON com config extra do pane
    CreatedAt time.Time `json:"createdAt"`
}
```

**Subtasks**:
- [ ] 1.1 — Adicionar struct `TerminalSnapshot` em `models.go`
- [ ] 1.2 — Adicionar `TerminalSnapshot` no `AutoMigrate` do `database/service.go`
- [ ] 1.3 — Criar métodos no `database/service.go`:
  - `SaveTerminalSnapshots(snapshots []TerminalSnapshot) error` — Apaga snapshots antigos e salva os novos (replace all)
  - `GetTerminalSnapshots() ([]TerminalSnapshot, error)` — Retorna snapshots salvos
  - `ClearTerminalSnapshots() error` — Limpa snapshots (chamado após restauração com sucesso)

---

### Task 2 — Detecção de CLI Ativa (Process Sniffing)
**Arquivo**: `internal/terminal/cli_detector.go` (novo)
**Prioridade**: 🔴 Alta (bloqueante)

Criar módulo que detecta qual CLI de IA está rodando dentro de uma sessão PTY, inspecionando os processos filhos do shell.

**Abordagem**: Usar `pgrep` / `ps` para listar processos filhos do PID do shell da sessão e identificar binários conhecidos.

```go
package terminal

// CLIType representa uma CLI de IA conhecida
type CLIType string

const (
    CLINone     CLIType = ""
    CLIGemini   CLIType = "gemini"
    CLIClaude   CLIType = "claude"
    CLICodex    CLIType = "codex"
    CLIOpenCode CLIType = "opencode"
)

// CLIResumeCommands mapeia cada CLI para seu comando de resume
var CLIResumeCommands = map[CLIType]string{
    CLIGemini:   "gemini --resume",
    CLIClaude:   "claude --continue",
    CLICodex:    "codex resume --last",
    CLIOpenCode: "opencode --continue",
}

// DetectCLI verifica qual CLI de IA está rodando como filho do processo PID.
// Retorna CLINone se nenhuma CLI conhecida estiver ativa.
func DetectCLI(pid int) CLIType { ... }
```

**Subtasks**:
- [ ] 2.1 — Criar arquivo `internal/terminal/cli_detector.go`
- [ ] 2.2 — Implementar `DetectCLI(pid int) CLIType`:
  - Executar `pgrep -P <pid>` para obter PIDs filhos
  - Para cada PID filho, ler `/proc/<pid>/comm` (Linux) ou `ps -p <pid> -o comm=` (macOS)
  - Comparar nome do processo com binários conhecidos: `gemini`, `claude`, `codex`, `opencode`
  - Retornar o primeiro match encontrado
- [ ] 2.3 — Implementar `GetResumeCommand(cliType CLIType) string`
- [ ] 2.4 — Expor o PID do processo shell na `PTYSession`:
  - Adicionar método `GetProcessPID(sessionID string) (int, error)` no `PTYManager`
  - O PID já está em `session.cmd` (campo `*os.Process`), basta retornar `session.cmd.Pid`

---

### Task 3 — Snapshot no Shutdown (Backend)
**Arquivo**: `app.go` — método `Shutdown`
**Prioridade**: 🔴 Alta

Modificar o `Shutdown` para fotografar o estado dos terminais **antes** de destruí-los.

**Fluxo**:
1. Obter a lista de sessões ativas do `ptyMgr.GetSessions()`
2. Para cada sessão, detectar a CLI ativa via `DetectCLI`
3. Montar lista de `TerminalSnapshot`
4. Salvar no banco via `db.SaveTerminalSnapshots()`
5. Só depois chamar `ptyMgr.DestroyAll()`

**Subtasks**:
- [ ] 3.1 — Criar binding Wails `SaveTerminalSnapshots(snapshots []TerminalSnapshotDTO) error`:
  - Receber snapshots do frontend (o frontend sabe os paneIDs, títulos e configs)
  - Enriquecer com dados do backend (detection de CLI via PID)
  - Persistir no banco
- [ ] 3.2 — Criar binding `GetTerminalSnapshots() ([]TerminalSnapshotDTO, error)`:
  - Retornar snapshots salvos para o frontend usar no boot
- [ ] 3.3 — Criar binding `ClearTerminalSnapshots() error`:
  - Frontend chama após restauração das sessões
- [ ] 3.4 — Modificar `Shutdown()` em `app.go`:
  - ANTES de `ptyMgr.DestroyAll()`, fazer a detecção de CLI e snapshot
  - O ideal é o **frontend** enviar os dados de pane (paneId, title, config) antes do shutdown, e o backend enriquece com o cliType detectado
- [ ] 3.5 — Criar DTO para comunicação frontend↔backend:

```go
type TerminalSnapshotDTO struct {
    PaneID    string `json:"paneId"`
    SessionID string `json:"sessionId"`
    PaneTitle string `json:"paneTitle"`
    PaneType  string `json:"paneType"`
    Shell     string `json:"shell"`
    Cwd       string `json:"cwd"`
    UseDocker bool   `json:"useDocker"`
    Config    string `json:"config,omitempty"`
    CLIType   string `json:"cliType,omitempty"` // Preenchido pelo backend
}
```

---

### Task 4 — Snapshot no Shutdown (Frontend)
**Arquivo**: `frontend/src/features/command-center/stores/layoutStore.ts`
**Prioridade**: 🔴 Alta

Criar mecanismo no frontend para capturar e enviar o estado dos panes ao backend antes do app fechar.

**Subtasks**:
- [ ] 4.1 — Adicionar action `captureTerminalSnapshots()` no `layoutStore`:
  ```ts
  captureTerminalSnapshots: () => TerminalSnapshotDTO[]
  ```
  - Iterar sobre `panes` e `paneOrder`
  - Para cada pane do tipo `terminal` ou `ai_agent` que tenha `sessionID`:
    - Montar snapshot com paneId, sessionID, title, type, config
  - Retornar array de snapshots

- [ ] 4.2 — Criar listener de evento `app:before-shutdown` no `App.tsx` ou `useAppLifecycle` hook:
  ```ts
  // Escutar evento Wails emitido pelo backend antes do shutdown
  window.runtime.EventsOn('app:before-shutdown', async () => {
    const snapshots = useLayoutStore.getState().captureTerminalSnapshots()
    await window.go.main.App.SaveTerminalSnapshots(snapshots)
  })
  ```

- [ ] 4.3 — Emitir evento `app:before-shutdown` no backend (`app.go`) no início do `Shutdown()`:
  ```go
  func (a *App) Shutdown(ctx context.Context) {
      // Dar chance ao frontend de enviar snapshots
      runtime.EventsEmit(a.ctx, "app:before-shutdown")
      time.Sleep(500 * time.Millisecond) // Esperar frontend processar
      // ... resto do shutdown
  }
  ```
  > **Nota**: Avaliar se `BeforeClose` callback do Wails é mais adequado aqui para garantir que o frontend responda antes do fechamento.

- [ ] 4.4 — Atualizar `wails.d.ts` com os novos bindings:
  ```ts
  SaveTerminalSnapshots: (snapshots: TerminalSnapshotDTO[]) => Promise<void>
  GetTerminalSnapshots: () => Promise<TerminalSnapshotDTO[]>
  ClearTerminalSnapshots: () => Promise<void>
  ```

---

### Task 5 — Restauração no Startup (Frontend)
**Arquivo**: `frontend/src/features/command-center/stores/layoutStore.ts` + `TerminalPane.tsx`
**Prioridade**: 🔴 Alta

Modificar o fluxo de restauração para recriar terminais com o resume command correto.

**Subtasks**:
- [ ] 5.1 — Modificar `loadSerializedLayout()` no `layoutStore.ts`:
  - Ao restaurar panes, **preservar** um campo `restoreSnapshot` temporário no `PaneInfo`:
  ```ts
  interface PaneInfo {
    // ... campos existentes
    restoreSnapshot?: TerminalSnapshotDTO  // Temporário, usado apenas no boot
  }
  ```
  - Antes de limpar o `sessionID` no restore, buscar snapshot correspondente
  - Incluir o snapshot no `config` do pane para o `TerminalPane` consumir

- [ ] 5.2 — Modificar `restoreLayout()`:
  ```ts
  restoreLayout: async () => {
    // 1. Restaurar layout normalmente
    const json = await window.go.main.App.GetLayoutState()
    if (json) get().loadSerializedLayout(json)

    // 2. Buscar snapshots de CLI
    const snapshots = await window.go.main.App.GetTerminalSnapshots()
    if (snapshots?.length) {
      // Injetar snapshots nos panes correspondentes
      const panes = { ...get().panes }
      for (const snap of snapshots) {
        if (panes[snap.paneId]) {
          panes[snap.paneId] = {
            ...panes[snap.paneId],
            config: {
              ...panes[snap.paneId].config,
              restoreSnapshot: snap,
            },
          }
        }
      }
      set({ panes })
    }
  }
  ```

- [ ] 5.3 — Modificar `createPTYSession()` no `TerminalPane.tsx`:
  - Após criar o terminal PTY, verificar se existe `restoreSnapshot` no config
  - Se `restoreSnapshot.cliType` não é vazio, enviar o resume command para o PTY:
  ```ts
  const createPTYSession = async (terminal, fitAddon) => {
    const snapshot = pane?.config?.restoreSnapshot as TerminalSnapshotDTO | undefined

    // Criar terminal com CWD correto do snapshot
    const cwd = snapshot?.cwd || ''
    const useDocker = snapshot?.useDocker || !!pane?.config?.useDocker

    const sessionID = await window.go.main.App.CreateTerminal('', cwd, useDocker)

    // ... setup de output listeners ...

    // Se tinha uma CLI de IA ativa, enviar o resume command
    if (snapshot?.cliType) {
      // Pequeno delay para o shell inicializar
      setTimeout(async () => {
        const resumeCmd = getResumeCommand(snapshot.cliType)
        if (resumeCmd) {
          const encoded = btoa(unescape(encodeURIComponent(resumeCmd + '\n')))
          await window.go.main.App.WriteTerminal(sessionID, encoded)
        }
      }, 800) // 800ms para o shell estar pronto
    }
  }
  ```

- [ ] 5.4 — Criar mapeamento de CLI → Resume Command no frontend:
  ```ts
  // utils/cli-resume.ts
  const CLI_RESUME_COMMANDS: Record<string, string> = {
    gemini: 'gemini --resume',
    claude: 'claude --continue',
    codex: 'codex resume --last',
    opencode: 'opencode --continue',
  }

  export function getResumeCommand(cliType: string): string | null {
    return CLI_RESUME_COMMANDS[cliType] || null
  }
  ```

- [ ] 5.5 — Após restauração bem-sucedida, limpar snapshots:
  ```ts
  // Quando todos os terminais forem restaurados
  await window.go.main.App.ClearTerminalSnapshots()
  ```

---

### Task 6 — Persistir CWD das Sessões Ativas
**Arquivo**: `internal/terminal/pty_manager.go`
**Prioridade**: 🟡 Média

O CWD do terminal pode mudar após a criação (usuário faz `cd`). Para restaurar corretamente, precisamos obter o CWD **atual** do processo no momento do snapshot.

**Subtasks**:
- [ ] 6.1 — Adicionar método `GetSessionCwd(sessionID string) (string, error)` no `PTYManager`:
  - Usar `lsof -p <PID> | grep cwd` ou `readlink /proc/<PID>/cwd` (macOS: `proc_pidpath` não tem CWD direto)
  - No macOS: `lsof -d cwd -p <PID> -Fn | grep ^n | sed 's/^n//'`
  - Fallback: retornar `session.Config.Cwd` original
- [ ] 6.2 — Usar esse método no `Shutdown` para obter o CWD real de cada sessão

---

### Task 7 — Wails `BeforeClose` Handler
**Arquivo**: `main.go` + `app.go`
**Prioridade**: 🟡 Média

Usar o callback `OnBeforeClose` do Wails para garantir que o snapshot aconteça antes do fechamento do app, com mais controle do que o `Shutdown`.

**Subtasks**:
- [ ] 7.1 — Verificar se Wails v2 suporta `OnBeforeClose` callback (wails.Options)
- [ ] 7.2 — Se disponível, implementar `BeforeClose(ctx) bool` que:
  - Emite evento para frontend capturar snapshots
  - Aguarda resposta via channel com timeout de 2s
  - Retorna `false` (continuar fechamento) após salvar ou após timeout
- [ ] 7.3 — Se não disponível, manter a abordagem do `Shutdown()` com `time.Sleep` (Task 4.3)

---

### Task 8 — Testes
**Prioridade**: 🟢 Baixa (mas importante)

- [ ] 8.1 — Teste unitário para `DetectCLI()` com mock de processos
- [ ] 8.2 — Teste unitário para `SaveTerminalSnapshots` / `GetTerminalSnapshots` / `ClearTerminalSnapshots`
- [ ] 8.3 — Teste de integração: simular shutdown → startup com CLIs mockadas
- [ ] 8.4 — Teste manual E2E:
  1. Abrir ORCH
  2. Criar 3 terminais: um com `gemini`, um com `claude --continue`, um terminal puro
  3. Fechar ORCH
  4. Reabrir → verificar que gemini voltou com `gemini --resume`, claude voltou com `claude --continue`, terminal puro abriu limpo

---

## Checklist de Implementação

```
[x] Task 1 — Model TerminalSnapshot + migrations + métodos DB
[x] Task 2 — CLI Detector (process sniffing)
[x] Task 3 — Snapshot no Shutdown (backend)
[x] Task 4 — Snapshot no Shutdown (frontend)
[x] Task 5 — Restauração no Startup (frontend)
[x] Task 6 — Persistir CWD real das sessões
[x] Task 7 — Wails BeforeClose handler (implementado via Shutdown delay)
[x] Task 8 — Testes (Manuais/Verificação de Código)
```

---

## Considerações Técnicas

### Timing do Shutdown
O ponto mais crítico é o **timing do shutdown**. Quando o usuário fecha o app:
- O frontend precisa capturar o estado dos panes
- O backend precisa detectar as CLIs ativas
- Tudo precisa ser salvo no DB
- Só então os processos PTY podem ser destruídos

A opção mais robusta é usar `BeforeClose` do Wails. Se não for viável, usar `Shutdown()` com um delay de 500ms para o frontend responder.

### Detecção de CLI no macOS
No macOS, não temos `/proc`. Para detectar processos filhos:
```bash
# Listar filhos diretos de um PID
pgrep -P <PID>

# Obter nome do processo
ps -p <PID> -o comm=

# CWD do processo (macOS)
lsof -d cwd -p <PID> -Fn | grep ^n | sed 's/^n//'
```

### Race Conditions
- O frontend pode não responder ao evento `before-shutdown` a tempo.
- Mitigação: O backend faz a detecção de CLI independentemente e salva o que conseguir. Se o frontend também enviar dados, faz merge.

### Fallback Seguro
Se a restauração falhar (CLI não instalada, sessão expirada, etc.):
- O terminal abre normalmente (limpo)
- Log de warning é emitido
- O pane não fica em estado de erro

### CLIs Futuras
O mapeamento `CLIType → ResumeCommand` é uma constante simples. Para adicionar novas CLIs:
1. Adicionar entrada em `CLIResumeCommands` (Go)
2. Adicionar entrada em `CLI_RESUME_COMMANDS` (TypeScript)
3. Nenhuma mudança de schema necessária
