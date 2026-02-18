# Spec: Segurança & Sandboxing

> **Módulo**: 7 — Security  
> **Status**: Draft  
> **PRD Ref**: Seção 13  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Garantir que sessões colaborativas não comprometam a segurança do Host. Implementar sandboxing via Docker e controles de permissão granulares para modo Live Share.

---

## 2. Modelo de Ameaças

| ID  | Ameaça                          | Vetor                          | Probabilidade | Impacto | Mitigação                           |
| --- | ------------------------------- | ------------------------------ | ------------- | ------- | ----------------------------------- |
| T1  | Código de sessão vazado         | Guest malicioso obtém código   | Média         | Alto    | Waiting Room + aprovação do Host     |
| T2  | Guest executa `rm -rf /`        | Write access no terminal       | Baixa         | Crítico | Docker (container isolado)           |
| T3  | Token OAuth interceptado        | MITM                           | Baixa         | Alto    | PKCE + Keychain + HTTPS only         |
| T4  | Dados sensíveis no prompt IA    | Token/senha enviado ao LLM     | Média         | Alto    | SecretSanitizer automático           |
| T5  | Rate-limit GitHub excedido      | Polling agressivo              | Alta          | Médio   | Cache + polling inteligente (30s)    |
| T6  | Guest escala privilégios        | Container escape               | Muito Baixa   | Crítico | `--security-opt=no-new-privileges`   |
| T7  | Replay attack no signaling      | SDP capturado                  | Baixa         | Médio   | Códigos expiráveis + uso único       |
| T8  | Guest acessa rede do Host       | Container com network bridge   | Média         | Alto    | `--network=none` ou rede restrita    |

---

## 3. Docker-First (Sandboxing)

### 3.1 Arquitetura

```
┌──────────────────────────────────────────┐
│             macOS Host                   │
│                                          │
│  ┌─────────────────────────────────────┐ │
│  │       Docker Container              │ │
│  │                                     │ │
│  │  Terminal (zsh/bash)                │ │
│  │  • Guest executa comandos aqui      │ │
│  │  • Isolado do Host OS               │ │
│  │                                     │ │
│  │  /workspace ← Bind Mount (código)   │ │
│  │  /home, /etc ← Container próprio    │ │
│  │                                     │ │
│  │  Limites:                           │ │
│  │  • --memory=2g                      │ │
│  │  • --cpus=2                         │ │
│  │  • --pids-limit=256                 │ │
│  │  • --read-only (rootfs)             │ │
│  │  • --security-opt=no-new-privileges │ │
│  └─────────────────────────────────────┘ │
│                                          │
│  Sistema do Host → INTACTO               │
└──────────────────────────────────────────┘
```

### 3.2 Docker Service (Go)

```go
type IDockerService interface {
    // Lifecycle
    CreateContainer(config ContainerConfig) (containerID string, err error)
    StartContainer(containerID string) error
    StopContainer(containerID string) error
    RemoveContainer(containerID string) error
    RestartContainer(containerID string) error

    // Status
    IsDockerAvailable() bool
    GetContainerStatus(containerID string) (string, error)
    ListContainers() ([]ContainerInfo, error)

    // Exec
    ExecInContainer(containerID string, cmd []string) (io.ReadWriteCloser, error)
}

type ContainerConfig struct {
    Image       string   // "node:20-alpine"
    ProjectPath string   // Path local para bind mount
    Memory      string   // "2g"
    CPUs        string   // "2"
    Shell       string   // "/bin/sh"
    Ports       []string // Port mapping (se necessário)
    EnvVars     []string
    ReadOnly    bool     // rootfs read-only
    NetworkMode string   // "none", "bridge"
}
```

### 3.3 Auto-Detection

```go
func (s *DockerService) DetectImage(projectPath string) string {
    // 1. Verificar Dockerfile
    if fileExists(filepath.Join(projectPath, "Dockerfile")) {
        return "build:" + projectPath // Build local
    }

    // 2. Detectar por arquivo de projeto
    detectors := map[string]string{
        "package.json": "node:20-alpine",
        "go.mod":       "golang:1.22-alpine",
        "requirements.txt": "python:3.12-slim",
        "Cargo.toml":   "rust:1.75-slim",
        "Gemfile":      "ruby:3.3-slim",
    }

    for file, image := range detectors {
        if fileExists(filepath.Join(projectPath, file)) {
            return image
        }
    }

    return "alpine:latest" // Fallback
}
```

### 3.4 Docker Run Flags

```go
func (s *DockerService) buildRunArgs(config ContainerConfig) []string {
    args := []string{
        "run", "-it", "--rm",
        "--name", "orch-session-" + generateShortID(),
        "-v", config.ProjectPath + ":/workspace",
        "-w", "/workspace",
        "--memory", config.Memory,
        "--cpus", config.CPUs,
        "--pids-limit", "256",
        "--security-opt", "no-new-privileges",
    }

    if config.ReadOnly {
        args = append(args, "--read-only",
            "--tmpfs", "/tmp:rw,noexec,nosuid,size=512m",
            "--tmpfs", "/var/tmp:rw,noexec,nosuid,size=256m",
        )
    }

    if config.NetworkMode != "" {
        args = append(args, "--network", config.NetworkMode)
    }

    args = append(args, config.Image, config.Shell)
    return args
}
```

---

## 4. Modo Live Share (Sem Docker)

### 4.1 Permissões

| Nível       | Default? | Capacidade                                |
| ------------ | -------- | ----------------------------------------- |
| `none`       | —        | Sem acesso ao terminal                     |
| `read_only`  | ✅ Sim    | Vê output, não pode digitar                |
| `read_write` | ❌ Não    | Pode digitar (Host concede explicitamente) |

### 4.2 Fluxo de Concessão de Write

```
Host clica "Tornar Editável" para Guest X
    │
    ├── 1. Modal de confirmação:
    │       ┌─────────────────────────────────────────┐
    │       │  ⚠️  ATENÇÃO                            │
    │       │                                         │
    │       │  Dar acesso de escrita permite que       │
    │       │  o convidado controle seu terminal.      │
    │       │                                         │
    │       │  Só faça isso com pessoas de confiança.  │
    │       │                                         │
    │       │  [ Cancelar ]    [ Conceder Acesso ]    │
    │       └─────────────────────────────────────────┘
    │
    ├── 2. Host confirma → Permissão atualizada
    │
    ├── 3. WebRTC Event → Guest notificado
    │
    └── 4. Guest vê: "Agora você pode digitar"
```

### 4.3 Revogação

- Host pode revogar a qualquer momento clicando no ícone de permissão do Guest.
- Revogação é instantânea (evento WebRTC).
- Guest recebe: "Seu acesso de escrita foi revogado."

---

## 5. Segurança de Tokens

### 5.1 Armazenamento

| Token                | Onde                          | Motivo                     |
| --------------------- | ----------------------------- | -------------------------- |
| Supabase Access       | macOS Keychain                 | Persistência segura         |
| Supabase Refresh      | macOS Keychain                 | Para renovação silenciosa   |
| GitHub OAuth          | macOS Keychain                 | Para GitHub API              |
| API Keys (OpenAI/etc) | SQLite (criptografado AES-256) | Config do usuário            |
| Tokens de sessão      | Memória (runtime apenas)       | Temporários                  |

### 5.2 Sanitização de Logs

```go
type LogSanitizer struct {
    patterns []*regexp.Regexp
}

// Remove tokens/senhas de logs antes de escrever
func (s *LogSanitizer) Sanitize(message string) string {
    for _, p := range s.patterns {
        message = p.ReplaceAllString(message, "[REDACTED]")
    }
    return message
}
```

---

## 6. Auditoria

### 6.1 Eventos Auditáveis

```go
type AuditEvent struct {
    Timestamp time.Time
    SessionID string
    UserID    string
    Action    string // "command_executed", "permission_changed", etc.
    Details   string
}
```

| Evento                    | Logado? | Detalhes                       |
| -------------------------- | ------- | ------------------------------ |
| Guest entrou na sessão     | ✅      | UserID, timestamp               |
| Guest saiu da sessão       | ✅      | UserID, timestamp               |
| Permissão alterada         | ✅      | De/Para, por quem               |
| Comando executado (guest)  | ✅      | Comando (sanitizado)            |
| Container reiniciado       | ✅      | Motivo                          |
| Sessão encerrada           | ✅      | Por quem, duração               |

### 6.2 Armazenamento

- Auditoria armazenada em tabela `audit_log` no SQLite.
- Retenção: últimas 1000 entradas por sessão.
- Acessível ao Host via UI (botão "Ver Logs" no painel de sessão).

---

## 7. Checklist de Segurança (Release)

- [ ] Tokens nunca em plaintext (JSON, SQLite raw, localStorage)
- [ ] PKCE Flow para OAuth
- [ ] Códigos de sessão expiram em 15 min
- [ ] Waiting Room obrigatória
- [ ] Read-Only como padrão para Guests
- [ ] Modal de alerta para concessão de Write
- [ ] Docker com `--security-opt=no-new-privileges`
- [ ] SecretSanitizer nos prompts de IA
- [ ] Sanitização de logs
- [ ] HTTPS only para todas as requests externas
- [ ] SQLite com permissão `0600`
- [ ] Reconexão WebRTC não bypass a Waiting Room

---

## 8. Dependências

| Dependência                | Tipo       | Spec Relacionada       |
| --------------------------- | ---------- | ---------------------- |
| Docker Engine (macOS)       | Opcional   | —                      |
| `go-keyring`                | Bloqueador | auth_and_persistence   |
| WebRTC Data Channel         | Bloqueador | invite_and_p2p         |
| SecretSanitizer             | Bloqueador | ai_engine              |
