# Spec: Terminal Sharing & Session Mirroring

> **Módulo**: 2 — Terminal Sharing  
> **Status**: Draft  
> **PRD Ref**: Seção 8  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Terminal colaborativo onde múltiplos usuários visualizam e interagem com a mesma sessão em tempo real. O **Host** executa o processo real; **Guests** recebem stream de I/O via WebRTC.

---

## 2. Arquitetura

```
HOST:
  PTY Process (zsh/bash) → PTY Manager (Go) → Terminal Bridge
      │                                            │
      ▼                                     ┌──────┴──────┐
  Host xterm.js                         WebRTC DC    WebRTC DC
                                        (Guest 1)   (Guest N)

GUEST:
  WebRTC DC → xterm.js (render only ou read/write)
  Guest Input → WebRTC DC → Host PTY Manager → Permission Check → PTY stdin
```

---

## 3. PTY Manager (Backend Go)

### 3.1 Interface

```go
type IPTYManager interface {
    Create(config PTYConfig) (sessionID string, err error)
    Destroy(sessionID string) error
    Resize(sessionID string, cols, rows uint16) error
    Write(sessionID string, data []byte) error
    OnOutput(sessionID string, handler func(data []byte))
    GetSessions() []PTYSession
    IsAlive(sessionID string) bool
}

type PTYConfig struct {
    Shell       string   // "/bin/zsh"
    Cwd         string
    Env         []string
    Cols        uint16   // Default: 80
    Rows        uint16   // Default: 24
    UseDocker   bool
    DockerImage string
    DockerMount string
}
```

### 3.2 Dependência

- macOS: `github.com/creack/pty` para pseudo-terminal nativo.

---

## 4. Terminal Bridge (Streaming)

```go
type OutputMessage struct {
    SessionID string `json:"sessionID"`
    Data      []byte `json:"data"`
    Timestamp int64  `json:"timestamp"`
    Sequence  uint64 `json:"sequence"` // ordenação no guest
}

type InputMessage struct {
    SessionID string `json:"sessionID"`
    UserID    string `json:"userID"`
    Data      []byte `json:"data"`
}
```

- Output é broadcast via WebRTC Data Channel para todos os guests.
- Input do guest é validado contra permissões antes de ser enviado ao PTY.

---

## 5. CRDTs — Resolução de Conflitos

### 5.1 Biblioteca: Yjs

Quando dois guests digitam simultaneamente, Yjs sincroniza os inputs via WebRTC.

| Uso                         | CRDT? | Motivo                              |
| ---------------------------- | ----- | ----------------------------------- |
| Input simultâneo no terminal | ✅    | Evitar conflito de bytes             |
| Output do terminal           | ❌    | Fonte única (Host PTY)               |
| Cursor position awareness    | ✅    | Mostrar onde cada guest "está"       |

---

## 6. Modos de Terminal

### 6.1 Modo Docker (Seguro)

- Terminal roda dentro de container isolado.
- Bind mount apenas da pasta do projeto.
- Auto-detect `Dockerfile` na raiz do projeto.
- Imagens padrão: `node:20-alpine`, `python:3.12-slim`, `golang:1.22`.
- Recursos: `--memory=2g --cpus=2` (configurável).
- **Desastre**: Guest roda `rm -rf /` → container morre → Host intacto → "Reiniciar Ambiente" em 5s.

### 6.2 Modo Live Share (Sem Docker)

- Terminal roda no SO do Host diretamente.
- **Read-Only** por padrão.
- Host pode conceder **Read/Write** explicitamente com alerta de segurança.
- Baseado em confiança, não em bloqueio técnico.

---

## 7. Permissões

```go
type TerminalPermission string

const (
    PermissionNone      TerminalPermission = "none"
    PermissionReadOnly  TerminalPermission = "read_only"
    PermissionReadWrite TerminalPermission = "read_write"
)
```

**Fluxo de alteração**:
1. Host clica "Tornar Editável" para Guest X.
2. Modal de segurança: *"Cuidado: Dar acesso de escrita permite que o convidado controle seu terminal."*
3. Backend atualiza `SessionPermissions`.
4. WebRTC event notifica o Guest.
5. Guest vê: "Você agora pode digitar neste terminal."

---

## 8. xterm.js (Frontend)

### 8.1 Addons obrigatórios

| Addon        | Função                                  |
| ------------ | --------------------------------------- |
| FitAddon     | Reflow automático no resize             |
| WebglAddon   | Renderização GPU (performance)          |
| SearchAddon  | Busca no buffer do terminal             |

### 8.2 Resize

- `ResizeObserver` no container → `fitAddon.fit()` → notifica backend (`PTYManager.Resize`) → notifica guests via WebRTC.

### 8.3 Virtualização

- Terminais minimizados: pausar renderização gráfica, manter buffer de dados (64KB ring buffer).
- Terminais fora do viewport: `display: none`.
- Terminal com foco: WebGL (60fps). Sem foco: Canvas 2D (30fps).

---

## 9. Temas

| Tema    | Background | Foreground | Descrição        |
| ------- | ---------- | ---------- | ---------------- |
| Dark    | `#1a1b26`  | `#c0caf5`  | Tokyo Night      |
| Light   | `#fafafa`  | `#383a42`  | One Light        |
| Hacker  | `#0a0a0a`  | `#00ff41`  | Matrix-style     |

---

## 10. Cursor Awareness (Multi-User)

Cada guest tem cor única. Renderizar overlay decorativo sobre xterm.js mostrando:
- Barra vertical colorida na posição do guest.
- Label flutuante com nome do guest.
- Indicador `isTyping` (animação de digitação).

---

## 11. Métricas de Performance

| Operação                       | Meta           |
| ------------------------------- | -------------- |
| Input → Output (local)          | < 16ms (60fps) |
| Input → Output (P2P)            | < 100ms        |
| Resize reflow                   | < 50ms         |
| Boot container Docker            | < 5s           |
| Reconnect WebRTC                | < 3s           |
| Scroll 5000 linhas              | < 100ms        |

---

## 12. Dependências

| Dependência                | Tipo       | Spec Relacionada      |
| --------------------------- | ---------- | --------------------- |
| WebRTC Data Channel         | Bloqueador | invite_and_p2p        |
| Yjs (CRDTs)                | Bloqueador | —                     |
| Docker Engine               | Opcional   | security_sandboxing   |
| react-mosaic (layout)       | Bloqueador | command_center_ui     |
| `github.com/creack/pty`     | Bloqueador | —                     |
