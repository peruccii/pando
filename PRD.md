# 📋 PRD — ORCH (Orchestrator)

> **Orquestrador Colaborativo de IA & GitHub para macOS**

---

| Campo               | Valor                                      |
| ------------------- | ------------------------------------------ |
| **Produto**         | ORCH                                       |
| **Versão do PRD**   | 1.0.0                                      |
| **Data**            | 12 de Fevereiro de 2026                    |
| **Plataforma**      | macOS (exclusivo)                          |
| **Stack**           | Go · React (Vite) · Wails · SQLite         |
| **Autor**           | @perucci                                   |
| **Status**          | Draft                                      |

---

## Índice

1. [Visão Geral](#1-visão-geral)
2. [Problema](#2-problema)
3. [Solução Proposta](#3-solução-proposta)
4. [Público-Alvo](#4-público-alvo)
5. [Stack Tecnológica](#5-stack-tecnológica)
6. [Arquitetura de Alto Nível](#6-arquitetura-de-alto-nível)
7. [Módulo 1 — Integração GitHub & Colaboração Real-Time](#7-módulo-1--integração-github--colaboração-real-time)
8. [Módulo 2 — Terminal Sharing & Session Mirroring](#8-módulo-2--terminal-sharing--session-mirroring)
9. [Módulo 3 — Motor de IA com Injeção de Contexto](#9-módulo-3--motor-de-ia-com-injeção-de-contexto)
10. [Módulo 4 — Autenticação Híbrida & Persistência Local](#10-módulo-4--autenticação-híbrida--persistência-local)
11. [Módulo 5 — UX/UI "Command Center"](#11-módulo-5--uxui-command-center)
12. [Módulo 6 — Sistema de Convite & Conexão P2P](#12-módulo-6--sistema-de-convite--conexão-p2p)
13. [Módulo 7 — Segurança & Sandboxing](#13-módulo-7--segurança--sandboxing)
14. [Requisitos Não-Funcionais](#14-requisitos-não-funcionais)
15. [Fases de Entrega (Roadmap)](#15-fases-de-entrega-roadmap)
16. [Métricas de Sucesso](#16-métricas-de-sucesso)
17. [Riscos & Mitigações](#17-riscos--mitigações)
18. [Glossário](#18-glossário)

---

## 1. Visão Geral

**ORCH** é um aplicativo desktop nativo para **macOS** que funciona como um **orquestrador colaborativo** unificando três pilares:

1. **Gerenciamento de GitHub** — Pull Requests, Diffs, Issues, Branches e Code Review em tempo real.
2. **Colaboração P2P** — Terminal compartilhado, sessões sincronizadas e comunicação direta entre desenvolvedores via WebRTC.
3. **Orquestração de IA** — Múltiplos agentes de IA operando simultaneamente com injeção de contexto da aplicação (PR aberto, erro no terminal, branch atual).

O objetivo é **centralizar todo o fluxo do programador** em uma única interface de "Command Center", eliminando a troca constante entre GitHub, terminal, IDE e ferramentas de comunicação.

---

## 2. Problema

| Dor                                   | Impacto                                                                        |
| -------------------------------------- | ------------------------------------------------------------------------------ |
| **Fragmentação de ferramentas**        | Desenvolvedores alternam entre 5+ aplicações (GitHub, Terminal, IDE, Slack, IA) |
| **Falta de contexto da IA**           | CLIs de IA (Copilot, Ollama) são "cegas" — não sabem o que o dev está vendo    |
| **Colaboração assíncrona**             | Code Review e pair programming dependem de calls separadas                     |
| **Overhead cognitivo**                 | Múltiplos agentes de IA não podem ser observados simultaneamente               |
| **Risco de segurança em sessions**     | Compartilhar terminal sem sandboxing expõe a máquina do host                   |

---

## 3. Solução Proposta

Um **app desktop nativo** (Wails + React) que opera como hub centralizado:

- **Deep Integration com GitHub** via GraphQL API v4 — PRs, Diffs, Issues, Branches com UI rica e colaborativa.
- **Terminal Sharing** via WebRTC + CRDTs — sessões sincronizadas onde múltiplos usuários interagem no mesmo terminal.
- **IA Context-Aware** — motor de IA que injeta automaticamente o estado da aplicação (PR, erros, branch) no prompt antes de chamar LLMs.
- **Command Center UI** — grid dinâmico de mosaico (tiling window manager) para orquestrar múltiplos agentes simultâneos.
- **Arquitetura Local-First** — dados persistidos em SQLite local; autenticação via OAuth + Keychain do macOS.

---

## 4. Público-Alvo

| Persona                        | Descrição                                                                                 |
| ------------------------------- | ----------------------------------------------------------------------------------------- |
| **Desenvolvedor Individual**    | Usa múltiplos agentes de IA para acelerar desenvolvimento; quer visão panóptica            |
| **Tech Lead / Revisor**         | Gerencia PRs e Code Reviews; quer contexto instantâneo e anotações colaborativas           |
| **Squad / Time Remoto**         | Precisa de pair programming e sessões compartilhadas sem configuração complexa             |
| **DevOps / SRE**                | Monitora múltiplos terminais simultaneamente; precisa de "God Mode" para broadcast de input |

---

## 5. Stack Tecnológica

### Core

| Camada       | Tecnologia                   | Justificativa                                              |
| ------------ | ---------------------------- | ---------------------------------------------------------- |
| **Backend**  | Go (Wails Runtime)           | Performance, binário único, acesso nativo ao macOS          |
| **Frontend** | React + Vite                 | Hot reload, ecossistema maduro, TypeScript                  |
| **Desktop**  | Wails v2                     | Bindings Go↔JS nativos, WebView nativo do macOS             |
| **Database** | SQLite (embedded, Pure Go)   | Local-first, zero dependência externa, sem CGO              |
| **ORM**      | GORM                         | Migrations automáticas, API fluente                         |

### Comunicação & Real-Time

| Recurso      | Tecnologia                   | Função                                                     |
| ------------ | ---------------------------- | ---------------------------------------------------------- |
| **API**      | GraphQL (GitHub API v4)      | Consultas eficientes, sem over-fetching                     |
| **Real-Time**| WebSockets                   | Eventos de plataforma (notificações, subscription)          |
| **P2P**      | WebRTC (Data Channels)       | Streaming de terminal, comunicação direta host↔guest        |
| **Sync**     | CRDTs                        | Resolução de conflitos em edição simultânea do terminal      |
| **Terminal** | xterm.js + FitAddon          | Emulação de terminal no frontend com reflow automático       |
| **Container**| Docker                       | Sandboxing de sessões compartilhadas                        |

### Autenticação & Segurança

| Recurso         | Tecnologia                  | Função                                                  |
| ---------------- | --------------------------- | ------------------------------------------------------- |
| **Auth (BaaS)**  | Supabase Auth               | OAuth 2.0 PKCE (GitHub & Google), zero infra própria     |
| **Token Store**  | macOS Keychain (`go-keyring`) | Armazenamento seguro de tokens, nunca em plaintext       |
| **Deep Links**   | `orch://` protocol           | Captura de callback OAuth no app desktop                 |

---

## 6. Arquitetura de Alto Nível

```
┌─────────────────────────────────────────────────────────────┐
│                      ORCH (macOS App)                       │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │                  Frontend (React/Vite)                 │  │
│  │                                                       │  │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌───────────┐   │  │
│  │  │ Command │ │ GitHub  │ │Terminal │ │    AI     │   │  │
│  │  │ Center  │ │  Panel  │ │ Grid   │ │  Agents   │   │  │
│  │  │  (Grid) │ │(PR/Diff)│ │(xterm) │ │ (Mosaic)  │   │  │
│  │  └────┬────┘ └────┬────┘ └────┬───┘ └─────┬─────┘   │  │
│  │       │           │           │            │          │  │
│  │  ─────┴───────────┴───────────┴────────────┴──────    │  │
│  │                    Wails Bindings                      │  │
│  └───────────────────────┬───────────────────────────────┘  │
│                          │                                   │
│  ┌───────────────────────┴───────────────────────────────┐  │
│  │                  Backend (Go/Wails)                    │  │
│  │                                                       │  │
│  │  ┌───────────┐ ┌───────────┐ ┌───────────────────┐    │  │
│  │  │  GitHub   │ │    AI     │ │    Collaboration  │    │  │
│  │  │  Service  │ │  Service  │ │     Service       │    │  │
│  │  │(GraphQL)  │ │(LLM Proxy)│ │  (WebRTC/CRDT)   │    │  │
│  │  └─────┬─────┘ └─────┬─────┘ └────────┬──────────┘    │  │
│  │        │              │                │               │  │
│  │  ┌─────┴──────────────┴────────────────┴───────────┐   │  │
│  │  │              Core Services                      │   │  │
│  │  │  ┌──────────┐ ┌──────────┐ ┌──────────────────┐ │   │  │
│  │  │  │  Auth    │ │ SQLite   │ │  Session Manager │ │   │  │
│  │  │  │(Keychain)│ │ (GORM)   │ │  (Signaling)     │ │   │  │
│  │  │  └──────────┘ └──────────┘ └──────────────────┘ │   │  │
│  │  └─────────────────────────────────────────────────┘   │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
└──────────────┬──────────────────────┬───────────────────────┘
               │                      │
      ┌────────▼────────┐    ┌────────▼────────┐
      │  GitHub API v4  │    │  Supabase Auth  │
      │   (GraphQL)     │    │   (OAuth PKCE)  │
      └─────────────────┘    └─────────────────┘
               │
      ┌────────▼────────┐    ┌─────────────────┐
      │   LLM APIs      │    │  WebRTC STUN/   │
      │ (Gemini/OpenAI) │    │  TURN Servers   │
      └─────────────────┘    └─────────────────┘
```

### Modelo de Dados Simplificado

```
┌──────────────┐     ┌──────────────────┐     ┌────────────────┐
│  UserConfig  │     │    Workspace     │────▶│ AgentInstance  │
│──────────────│     │──────────────────│     │────────────────│
│ Theme        │     │ Name             │     │ Name           │
│ OpenAIKey    │     │ Agents[]         │     │ Type (LLM)     │
│ DefaultShell │     │                  │     │ Status         │
└──────────────┘     └──────────────────┘     │ WindowX/Y/W/H  │
                                              │ IsMinimized    │
                                              └───────┬────────┘
                                                      │
                                              ┌───────▼────────┐
                                              │  ChatHistory   │
                                              │────────────────│
                                              │ Role           │
                                              │ Content        │
                                              │ Timestamp      │
                                              └────────────────┘
```

---

## 7. Módulo 1 — Integração GitHub & Colaboração Real-Time

### 7.1 Objetivo

Implementar uma **Deep Integration** com o GitHub focando em fluxos colaborativos (Code Review, PR Management, Issues). **Não** replicar um cliente Git completo — operações complexas (`rebase`, `stash`, `cherry-pick`) permanecem no terminal.

### 7.2 Arquitetura de Dados — "Single Source of Truth com Escrita Autenticada"

#### Leitura (Host-Driven)

```
Host (Go Backend) ──► GitHub GraphQL API v4 ──► Hydrated State (JSON)
                                                       │
                                                       ▼
                                              WebRTC Data Channel
                                                       │
                                              ┌────────▼────────┐
                                              │  Guest 1 (UI)   │
                                              │  Guest 2 (UI)   │
                                              │  Guest N (UI)   │
                                              └─────────────────┘
```

- O **Host** atua como proxy de leitura, consultando a API do GitHub.
- O Host processa e transmite um **"Hydrated State"** (JSON otimizado) para os Guests via WebRTC.
- **Benefício**: Economia de rate-limit e garantia de que todos veem o mesmo estado.

#### Escrita (Guest-Authenticated)

> **Regra de Ouro**: O Guest **JAMAIS** usa as credenciais do Host.

- Para ações de plataforma (Criar PR, Comentar, Aprovar, Merge), o Guest deve estar autenticado com seu **próprio Token OAuth**.
- O App do Guest envia a Mutation GraphQL **diretamente** para a API do GitHub.
- **Segurança**: Garante auditoria correta (avatar do Guest aparece no GitHub) e respeita ACLs nativas.

### 7.3 UX — Optimistic UI & Real-Time

| Etapa                      | Ação                                                                     |
| -------------------------- | ------------------------------------------------------------------------ |
| **1. Ação Local**          | Usuário clica em "Enviar Comentário"                                      |
| **2. Feedback Imediato**   | UI insere o comentário localmente com status `Pendente/Enviando...`       |
| **3. Broadcast P2P**       | Evento enviado via WebRTC para outros participantes (também como Pendente) |
| **4. Persistência Async**  | Backend dispara Mutation para o GitHub                                    |
| **5a. Sucesso**            | Status muda para `Enviado ✓`                                             |
| **5b. Erro**               | Feedback visual + opção de retry                                          |

#### Sincronização Passiva (Polling)

Como o GitHub não possui WebSockets para todos os eventos, o Host mantém um **Polling Inteligente** (a cada 30s) verificando o `updatedAt` dos PRs abertos. Se houver mudança externa, faz fetch e atualiza a sala via WebRTC.

### 7.4 Escopo Funcional — GUI vs. CLI

#### ✅ Deve ter GUI (Interface Rica)

| Feature               | Detalhes                                                                          |
| ---------------------- | --------------------------------------------------------------------------------- |
| **Pull Requests**      | Listagem, Diffs paginados, Reviews, Conversas (Threads)                            |
| **Issues**             | Kanban simplificado (Título, Label, Assignee, Status)                              |
| **Branches**           | Dropdown para troca rápida (Checkout), criação de branch                           |
| **Annotations**        | Comentário em linha de código → evento "Scroll Sync" via WebRTC para todos          |

#### 🖥️ Deve ser CLI (Terminal)

| Operação                         | Justificativa                          |
| -------------------------------- | -------------------------------------- |
| `rebase`, `reset`, `reflog`     | Operações de histórico complexas        |
| `stash`, `clean`                | Manipulação de arquivos locais          |

#### 🔄 File Watcher (.git)

O App deve monitorar a pasta `.git`. Se o usuário usar o terminal para mudar de branch ou commitar, a **GUI detecta a mudança e atualiza o estado visual automaticamente**.

### 7.5 Barreira de Identidade (Identity Guardrails)

```
┌──────────────────────────────────────────────┐
│            Estado de Autenticação            │
│                                              │
│  user.isAuthenticated : boolean              │
│  user.githubToken     : string (em memória)  │
│  user.profile         : { login, avatar }    │
└──────────────────────────────────────────────┘
```

| Estado             | Comportamento da UI                                                |
| ------------------- | ----------------------------------------------------------------- |
| `!isAuthenticated`  | Botões de ação (Criar PR, Comentar) ficam **disabled** ou exibem "Logar no GitHub para..." |
| `isAuthenticated`   | Acesso completo às ações de escrita                                |
| **Terminal (P2P)**  | Acessível mesmo sem login GitHub (se sessão permitir anônimos)     |

> **Progressive Disclosure**: O usuário pode entrar para **observar** (Read-Only) sem login, mas para **agir** no GitHub, o app exige autenticação.

---

## 8. Módulo 2 — Terminal Sharing & Session Mirroring

### 8.1 Modelo de Operação

```
┌─────────────────────────────────────────────────────┐
│                    HOST (Anfitrião)                  │
│                                                     │
│  ┌─────────────┐    ┌──────────────────────────┐    │
│  │  Processo    │───▶│  xterm.js (TTY Local)    │    │
│  │ (Node/Python)│    │                          │    │
│  └─────────────┘    └──────────┬───────────────┘    │
│                                │                     │
│                     Stream I/O (stdin/stdout)        │
│                                │                     │
│                     WebRTC Data Channel              │
└────────────────────────────────┬─────────────────────┘
                                 │
              ┌──────────────────┼──────────────────┐
              │                  │                  │
    ┌─────────▼──────┐ ┌────────▼───────┐ ┌────────▼───────┐
    │   GUEST 1      │ │   GUEST 2      │ │   GUEST N      │
    │ (Visualização) │ │ (Visualização) │ │ (Visualização) │
    │  xterm.js      │ │  xterm.js      │ │  xterm.js      │
    │ + Input (opt.) │ │ + Input (opt.) │ │ + Input (opt.) │
    └────────────────┘ └────────────────┘ └────────────────┘
```

- **Host**: Roda o processo real (Node, Python, CLI) na máquina local ou container Docker.
- **Guests**: Recebem o stream de texto (I/O) e **podem enviar comandos** (se autorizados).
- **CRDTs**: Garantem que digitação simultânea de múltiplos usuários não quebre o texto.

### 8.2 Modos de Terminal

| Modo                | Descrição                                                                     |
| ------------------- | ----------------------------------------------------------------------------- |
| **Docker (Seguro)** | Terminal roda dentro de um container. Guest pode fazer qualquer coisa sem risco. |
| **Live Share**      | Terminal roda no SO do Host. Guest começa Read-Only; Host pode conceder Write.   |

### 8.3 Permissões de Escrita

| Nível          | Descrição                                                    |
| -------------- | ------------------------------------------------------------ |
| **Read-Only**  | Padrão. Guest vê output mas **não** pode digitar.             |
| **Read/Write** | Host concede explicitamente. Alerta de segurança exibido.     |

> **Alerta obrigatório**: *"Cuidado: Dar acesso de escrita permite que o convidado controle seu terminal. Só faça isso com pessoas de confiança."*

---

## 9. Módulo 3 — Motor de IA com Injeção de Contexto

### 9.1 Problema

IAs em CLI são **processos isolados** — não sabem qual PR, Issue ou arquivo o usuário está visualizando. Resultado: respostas genéricas e descontextualizadas.

### 9.2 Solução — Prompt Augmentation via "Man-in-the-Middle"

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────┐
│  Frontend    │────▶│ Interceptor  │────▶│   Context    │────▶│ LLM API  │
│ (xterm.js)   │     │  (Wails/Go)  │     │   Builder    │     │(Gemini/  │
│              │     │              │     │              │     │ OpenAI)  │
│ "Explique    │     │ Detecta cmd  │     │ Injeta:      │     │          │
│  este PR"    │     │ de IA. NÃO   │     │ - PR Diff    │     │ Prompt   │
│              │     │ envia p/ shell│    │ - Branch     │     │ Aumentado│
└──────────────┘     └──────────────┘     │ - LastStderr │     └────┬─────┘
                                          │ - File Open  │          │
                                          └──────────────┘     ┌────▼─────┐
                                                               │ Streaming│
                                                               │ Response │
                                                               │ → xterm  │
                                                               └──────────┘
```

### 9.3 Template do System Prompt (Dinâmico)

```
--- SYSTEM CONTEXT (INJECTED) ---
[ROLE]
Você é um Arquiteto de Software Sênior assistindo um desenvolvedor dentro de um terminal.
Seja conciso, técnico e direto. Evite markdown complexo que quebre em terminais TTY.

[CURRENT APP STATE]
- Projeto: {{ProjectName}}
- Branch Atual: {{CurrentBranch}}
- Arquivo Aberto (Visualizador): {{CurrentFile}}

[GITHUB CONTEXT — ACTIVE PR]
- PR ID: #{{PRNumber}}
- Título: {{PRTitle}}
- Diff (Resumo das últimas 50 linhas):
  {{DiffContentSnippet}}

[TERMINAL HISTORY]
- Último comando rodado: {{LastCommand}}
- Saída de erro (se houver): {{LastStderr}}
---------------------------------

[USER INPUT]
{{UserMessage}}
```

### 9.4 Requisitos de Implementação

| Requisito               | Especificação                                                        |
| ------------------------ | -------------------------------------------------------------------- |
| **Struct**               | `AIService` com método `GenerateResponse(msg, sessionID) (string, error)` |
| **Dependência**          | Acesso ao `GitHubService` para dados cacheados (sem bater na API)     |
| **Token Budget**         | Truncar Diffs gigantes; priorizar `.go`, `.js`, `.ts`; ignorar `package-lock.json` |
| **Sanitização**          | Remover segredos/tokens do contexto antes de enviar para a IA         |
| **Streaming**            | Resposta streamed via evento Wails → xterm.js (simulando digitação)   |

### 9.5 Casos de Uso

| Cenário                  | Input                       | Contexto Injetado              | Resposta Esperada                              |
| ------------------------ | --------------------------- | ------------------------------ | ---------------------------------------------- |
| **Explique o PR**        | "O que esse PR faz?"        | `PR.Body` + `PR.Diff`          | Resumo das mudanças baseado no diff             |
| **Correção de Erro**     | "Como arrumo isso?"         | `LastStderr` do cmd anterior   | Análise do stack trace + sugestão de correção   |
| **Sem Contexto**         | "Gere uma função de soma"   | PR e Logs vazios               | Resposta genérica de chatbot de código           |

---

## 10. Módulo 4 — Autenticação Híbrida & Persistência Local

### 10.1 Arquitetura — "Local-First, Cloud-Auth"

#### Fluxo OAuth (PKCE para Desktop)

```
┌─────────┐     ┌──────────┐     ┌──────────────┐     ┌─────────────┐
│ Frontend│────▶│ Go Auth  │────▶│   Browser    │────▶│  Supabase   │
│ "Login" │     │ Service  │     │ (Safari)     │     │  Auth       │
└─────────┘     └──────────┘     └──────┬───────┘     └──────┬──────┘
                                        │                     │
                                        │   OAuth Callback    │
                                        │◀────────────────────┘
                                        │
                                orch://auth/callback
                                        │
                                ┌───────▼────────┐
                                │  Wails captura │
                                │  access_token  │
                                │  refresh_token │
                                │       │        │
                                │       ▼        │
                                │  macOS Keychain│
                                │  (go-keyring)  │
                                └────────────────┘
```

#### Regras de Segurança de Tokens

| ✅ Obrigatório                        | 🚫 Proibido                                    |
| ------------------------------------- | ----------------------------------------------- |
| macOS Keychain via `go-keyring`       | Salvar tokens em JSON, SQLite ou LocalStorage    |
| Token em memória durante execução     | Persistir token em arquivo de texto              |
| Refresh token silencioso no startup   | Expor token em logs                              |

### 10.2 Persistência Local — SQLite

#### Localização (macOS)

```
~/Library/Application Support/ORCH/orch_data.db
```

#### Schema (GORM Structs)

```go
type UserConfig struct {
    gorm.Model
    Theme        string // "dark", "light", "hacker"
    OpenAIKey    string // Opcional: chave própria do usuário
    DefaultShell string // "zsh", "bash"
}

type Workspace struct {
    gorm.Model
    Name   string
    Agents []AgentInstance `gorm:"foreignKey:WorkspaceID"`
}

type AgentInstance struct {
    gorm.Model
    WorkspaceID  uint
    Name         string // ex: "Refatorador SQL"
    Type         string // ex: "Gemini Pro", "GPT-4"
    Status       string // "idle", "running", "error"
    WindowX      int
    WindowY      int
    WindowWidth  int
    WindowHeight int
    IsMinimized  bool
}

type ChatHistory struct {
    gorm.Model
    AgentInstanceID uint
    Role            string // "user", "assistant", "system"
    Content         string
    Timestamp       int64
}
```

### 10.3 Rotina de Bootstrap (Startup)

```
┌────────────────────────────────────────────────┐
│              App Startup (main.go)              │
│                                                │
│  1. CHECK AUTH                                 │
│     ├─ Ler token do Keychain                   │
│     ├─ Validar expiração                       │
│     ├─ Se expirado → Refresh silencioso        │
│     └─ Se falhar → Estado: LoggedOut           │
│                                                │
│  2. CHECK DB                                   │
│     ├─ Verificar se orch_data.db existe        │
│     └─ Se não → Criar + AutoMigrate (GORM)    │
│                                                │
│  3. RESTORE STATE (Hydration)                  │
│     ├─ Buscar último Workspace ativo           │
│     ├─ Carregar AgentInstances + coordenadas   │
│     └─ Enviar para Frontend → Remontar Grid    │
└────────────────────────────────────────────────┘
```

### 10.4 Privacidade

> **Zero Telemetria de Código**: O código do usuário, prompts e histórico do SQLite **JAMAIS** devem sair da máquina, exceto para a API da IA escolhida durante a execução.

---

## 11. Módulo 5 — UX/UI "Command Center"

### 11.1 Conceito — "Bento Box Dinâmico"

Ao invés de abas ocultas (modelo Chrome), o ORCH utiliza **Split Panes** (painéis divididos), similar ao `tmux` ou `i3wm`, mas com facilidade de mouse.

### 11.2 Smart Layout (Auto-Grid)

| Nº de Agentes     | Layout                                                      |
| ------------------- | ----------------------------------------------------------- |
| **1**               | Tela Cheia                                                   |
| **2**               | Split Vertical (50/50)                                       |
| **3**               | Um principal esquerda (50%) + 2 menores direita empilhados    |
| **4**               | Grid 2×2                                                     |
| **5-9**             | Grid adaptativo com barra de rolagem se necessário            |
| **10+**             | Grid automático (3×4 ou 4×3)                                 |

### 11.3 Interações

| Feature                   | Comportamento                                                              |
| ------------------------- | -------------------------------------------------------------------------- |
| **Resizing**              | Bordas "agarráveis" (Draggable Gutters) entre painéis                      |
| **Drag & Drop**           | Arrastar header de terminal para trocar posição; Drop Zone Highlighting     |
| **Zen Mode (Foco)**       | Botão "Maximizar" → terminal ocupa 100% (z-index superior); toggle volta   |
| **xterm.js Reflow**       | `fitAddon.fit()` disparado em cada redimensionamento                        |

### 11.4 Hierarquia Visual

| Elemento                        | Especificação                                                  |
| -------------------------------- | -------------------------------------------------------------- |
| **Foco Ativo**                   | Borda brilhante (Accent Color) + sombra (Glow)                 |
| **Terminais Inativos**           | 10-20% opacidade reduzida (Dimmed)                              |
| **Header do Painel (20px)**      | Nome do Agente + Indicador de Status + Controles Rápidos        |

#### Indicadores de Status

| Ícone | Estado                      |
| ----- | --------------------------- |
| 🟢    | Ocioso / Pronto              |
| 🟡    | Escrevendo / Pensando        |
| 🔴    | Erro / Ação Necessária       |

#### Controles Rápidos (Header)

| Ícone | Ação              |
| ----- | ------------------ |
| 🗑️    | Matar Processo     |
| 🔄    | Reiniciar          |
| 🔍    | Ver Logs           |

### 11.5 Stack de UI (Frontend)

| Componente              | Biblioteca Recomendada                     |
| ----------------------- | ------------------------------------------ |
| **Tiling/Mosaic**       | `react-mosaic-component` ou `rc-dock`       |
| **Grid Livre**          | `react-grid-layout` (alternativa)           |
| **Terminal**            | `xterm.js` + `FitAddon`                     |
| **Virtualização**       | Renderizar apenas terminais visíveis no viewport; pausar canvas de minimizados |

### 11.6 Broadcast Input — "God Mode"

**Global Input Bar** no rodapé da interface:

- Quando ativado, o input do usuário é enviado para **TODOS** os agentes simultaneamente.
- **Casos de uso**: `"Parem todos agora"`, `"Atualizem suas dependências"`.
- **Sensação**: Orquestração total, "command center" de verdade.

---

## 12. Módulo 6 — Sistema de Convite & Conexão P2P

### 12.1 Fluxo de Convite — "Handshake"

```
┌─────────┐                    ┌──────────┐                    ┌─────────┐
│  HOST   │                    │ BACKEND  │                    │  GUEST  │
│         │                    │ (Signal) │                    │         │
│ 1. Start│───────────────────▶│          │                    │         │
│  Session│                    │ Gera     │                    │         │
│         │◀───────────────────│ X92B-4K7 │                    │         │
│         │                    │          │                    │         │
│ 2. Envia│ Slack/WhatsApp     │          │                    │         │
│  código │─────────────────────────────────────────────────▶  │         │
│         │                    │          │                    │ 3. Join │
│         │                    │          │◀───────────────────│ X92B-4K7│
│         │                    │ 4. Valida│                    │         │
│         │                    │  código  │                    │         │
│         │  "Fulano quer     │          │                    │         │
│ 5. Sala │◀──entrar.──────────│ Waiting  │                    │         │
│ Espera  │  Permitir?"       │  Room    │                    │         │
│         │                    │          │                    │         │
│ 6. OK!  │───────────────────▶│ 7. SDP   │                    │         │
│ Aprovar │                    │ Exchange │───────────────────▶│         │
│         │                    │          │◀───────────────────│         │
│         │                    │          │                    │         │
│         │◀═══════════════════════WebRTC P2P════════════════▶│         │
│         │        (Backend sai da jogada)                    │         │
└─────────┘                                                   └─────────┘
```

### 12.2 Especificações do Código de Sessão

| Requisito                | Especificação                                  |
| ------------------------ | ---------------------------------------------- |
| **Formato**              | Short Code: `XXXX-XXX` (fácil de ditar)          |
| **Expiração**            | 15 minutos após criação (configurável)          |
| **Uso Único**            | Código invalidado após conexão bem-sucedida      |
| **Waiting Room**         | Guest **nunca** conecta sem aprovação do Host     |

### 12.3 Sinalização (Signaling Server)

O backend Go atua como **Signaling Server** temporário:

1. Armazena SDP Offer do Host.
2. Entrega SDP Offer ao Guest aprovado.
3. Recebe SDP Answer do Guest e entrega ao Host.
4. Após a conexão WebRTC ser estabelecida, o **backend sai da jogada** para dados pesados.

---

## 13. Módulo 7 — Segurança & Sandboxing

### 13.1 Modelo de Ameaças

| Ameaça                                 | Vetor                               | Mitigação                                       |
| --------------------------------------- | ----------------------------------- | ------------------------------------------------ |
| **Código de sessão vazado**             | Guest malicioso obtém código         | Waiting Room obrigatória + aprovação do Host      |
| **Guest executa `rm -rf /`**            | Acesso Write no terminal             | Modo Docker (container isolado) por padrão        |
| **Token OAuth interceptado**            | Man-in-the-middle                    | PKCE Flow + macOS Keychain + HTTPS only           |
| **Dados sensíveis no prompt da IA**     | Token/senha enviado ao LLM           | Sanitização automática antes do envio              |
| **Rate-limit do GitHub excedido**       | Polling agressivo                    | Cache local + Polling inteligente (30s)            |

### 13.2 Docker-First (Sandboxing Recomendado)

```
┌──────────────────────────────────────────┐
│             macOS Host                   │
│                                          │
│  ┌─────────────────────────────────────┐ │
│  │        Docker Container             │ │
│  │  ┌──────────────────────────────┐   │ │
│  │  │  Terminal (zsh/bash)         │   │ │
│  │  │  • Guest pode executar       │   │ │
│  │  │    qualquer comando          │   │ │
│  │  │  • Isolado do Host OS        │   │ │
│  │  └──────────────────────────────┘   │ │
│  │                                     │ │
│  │  /workspace ← Bind Mount (código)   │ │
│  │  /home, /etc ← Container próprio    │ │
│  └─────────────────────────────────────┘ │
│                                          │
│  Fotos, Documentos, Drivers → INTACTOS   │
└──────────────────────────────────────────┘
```

- **Cenário de desastre**: Guest roda `rm -rf /` → container morre → Host intacto → "Reiniciar Ambiente" em 5 segundos.
- **Código afetado?** Possível (volume montado), mas o sistema operacional está seguro.

### 13.3 Modo Live Share (Sem Docker)

| Configuração              | Comportamento                                                   |
| ------------------------- | --------------------------------------------------------------- |
| **Terminal Read-Only**    | Padrão. Guest vê, mas não digita.                                |
| **Terminal Read/Write**   | Host concede explicitamente com alerta de segurança.             |
| **Baseado em Confiança**  | Sem bloqueio técnico de comandos; responsabilidade do Host.       |

---

## 14. Requisitos Não-Funcionais

| Categoria         | Requisito                                                                   | Meta                    |
| ----------------- | --------------------------------------------------------------------------- | ----------------------- |
| **Performance**   | Latência de UI < 60ms para input no terminal                                 | 16ms (60fps)            |
| **Performance**   | Tempo de startup (cold) do app                                               | < 3 segundos            |
| **Performance**   | Renderização de 10+ terminais simultâneos sem travamento                      | Virtualização de canvas |
| **Escalabilidade**| Suporte a até 10 Guests simultâneos por sessão                               | P2P mesh ou SFU         |
| **Confiabilidade**| Reconexão automática WebRTC em caso de queda temporária                       | Retry com backoff       |
| **Segurança**     | Zero telemetria de código — dados nunca saem da máquina exceto para LLM API   | Auditável               |
| **UX**            | Onboarding (primeira sessão) em menos de 2 minutos                            | Wizard simplificado     |
| **Compatibilidade**| macOS 12+ (Monterey e superiores)                                            | WebView nativo          |
| **Acessibilidade**| Atalhos de teclado para todas as ações principais                             | `Cmd+N`, `Cmd+W`, etc.  |
| **i18n**          | Interface em Português (BR) e Inglês                                          | Fase 2                  |

---

## 15. Fases de Entrega (Roadmap)

### Fase 0 — Fundação (Semanas 1-4)

- [ ] Setup do projeto Wails + React (Vite) + TypeScript
- [ ] Configuração de SQLite (GORM) com AutoMigrate
- [ ] Autenticação OAuth (GitHub) via Supabase + macOS Keychain
- [ ] Estrutura de pastas e módulos Go (Services)
- [ ] Rotina de Bootstrap (Check Auth → Check DB → Restore State)
- [ ] Deep Link `orch://` para callback OAuth

### Fase 1 — Terminal & UI Core (Semanas 5-8)

- [ ] xterm.js integrado com FitAddon e WebGL Renderer
- [ ] Grid dinâmico (react-mosaic ou rc-dock)
- [ ] Smart Layout automático (1→N agentes)
- [ ] Resizing com Draggable Gutters
- [ ] Drag & Drop de painéis
- [ ] Zen Mode (Maximizar/Restaurar)
- [ ] Hierarquia visual (foco ativo, dimming, indicadores de status)
- [ ] Atalhos de teclado (`Cmd+N`, `Cmd+W`, `Cmd+1-9`, `Cmd+B`)

### Fase 2 — GitHub Integration (Semanas 9-12)

- [ ] GitHubService (Go) — consultas GraphQL v4
- [ ] Listagem de Pull Requests com filtros
- [ ] Visualização de Diffs (paginados, syntax highlighting)
- [ ] Reviews e Conversas (Threads) inline
- [ ] Issues — Kanban simplificado
- [ ] Branches — Dropdown de checkout rápido + criação
- [ ] File Watcher (.git) para sincronização GUI↔Terminal
- [ ] Cache local de dados GitHub no backend
- [ ] Barreira de Identidade (botões disabled quando !isAuthenticated)

### Fase 3 — Colaboração P2P (Semanas 13-18)

- [ ] Signaling Server (Go) para troca WebRTC SDP
- [ ] Geração de Short Codes para sessões
- [ ] Waiting Room — aprovação do Host
- [ ] WebRTC Data Channel para streaming de terminal
- [ ] CRDTs para edição simultânea de input
- [ ] Terminal Sharing (modo Read-Only padrão)
- [ ] Escrita autenticada (Guest → GitHub direto)
- [ ] Leitura proxy (Host → Hydrated State → WebRTC → Guests)
- [ ] Optimistic UI para comentários e ações
- [ ] Scroll Sync via WebRTC (annotations)

### Fase 4 — Motor de IA (Semanas 19-22)

- [ ] AIService (Go) com `GenerateResponse()`
- [ ] Interceptador de comandos de IA no terminal
- [ ] Context Builder — injeção de PR, Branch, Errors
- [ ] Template de System Prompt dinâmico
- [ ] Streaming de resposta → xterm.js
- [ ] Token Budget + truncamento inteligente de Diffs
- [ ] Sanitização de segredos
- [ ] Suporte a múltiplos provedores (Gemini, OpenAI)
- [ ] Broadcast Input — "God Mode"

### Fase 5 — Segurança & Docker (Semanas 23-26)

- [ ] Integração Docker para sessões containerizadas
- [ ] Detecção de Dockerfile / imagem padrão
- [ ] Bind Mount da pasta de código
- [ ] "Reiniciar Ambiente" (rebuild container)
- [ ] Modo Live Share (sem Docker) com permissões explícitas
- [ ] Alertas de segurança para concessão de Write
- [ ] Auditoria de ações do Guest
- [ ] Testes de penetração e hardening

### Fase 6 — Polish & Launch (Semanas 27-30)

- [ ] Onboarding Wizard (primeira execução)
- [ ] Temas (Dark, Light, Hacker)
- [ ] Persistência de layout (WindowX/Y/W/H no SQLite)
- [ ] Virtualização de renderização (10+ terminais)
- [ ] Reconexão automática WebRTC
- [ ] Documentação de usuário
- [ ] Testes E2E
- [ ] Build de produção (.dmg) para macOS
- [ ] Release v1.0.0

---

## 16. Métricas de Sucesso

| Métrica                              | Meta (v1.0)          | Como Medir                           |
| ------------------------------------ | -------------------- | ------------------------------------ |
| **Tempo de onboarding**             | < 2 min              | Timer da primeira sessão completa     |
| **Latência P2P (terminal)**          | < 100ms              | Ping mão dupla via WebRTC            |
| **Taxa de reconexão automática**     | > 95%                | Logs de WebRTC                       |
| **Terminais simultâneos sem lag**     | 10+                  | FPS do canvas xterm.js               |
| **Startup time (cold)**             | < 3s                 | Timestamp main.go → UI ready         |
| **Satisfação do desenvolvedor**      | > 8/10               | Survey pós-uso                       |

---

## 17. Riscos & Mitigações

| Risco                                     | Probabilidade | Impacto | Mitigação                                                    |
| ----------------------------------------- | ------------- | ------- | ------------------------------------------------------------ |
| **Rate-limit GitHub API**                 | Alta          | Médio   | Cache agressivo + Polling inteligente (30s)                   |
| **NAT/Firewall bloqueia WebRTC**          | Média         | Alto    | TURN server como fallback                                     |
| **Performance com 10+ xterm.js**          | Média         | Alto    | Virtualização de renderização + pause em minimizados          |
| **Complexidade de CRDTs**                 | Média         | Médio   | Usar lib madura (Yjs ou Automerge)                            |
| **Docker não instalado no Host**          | Alta          | Baixo   | Fallback para modo Live Share + prompt de instalação           |
| **Tokens OAuth expiram em sessão longa**  | Média         | Médio   | Refresh silencioso automático + re-auth graceful               |
| **Diff gigante trava Context Builder**     | Média         | Médio   | Token Budget + truncamento por tipo de arquivo                 |

---

## 18. Glossário

| Termo                  | Definição                                                                                  |
| ---------------------- | ------------------------------------------------------------------------------------------ |
| **Host**               | Usuário que criou a sessão colaborativa e roda os processos                                  |
| **Guest**              | Usuário convidado que visualiza e (opcionalmente) interage com a sessão                      |
| **Hydrated State**     | JSON otimizado com o estado completo do repositório, transmitido do Host para Guests          |
| **CRDT**               | Conflict-free Replicated Data Type — estrutura de dados que resolve conflitos automaticamente |
| **Optimistic UI**      | Padrão onde a UI reflete a ação antes da confirmação do servidor                             |
| **Prompt Augmentation**| Injeção de contexto no prompt antes de enviar para a IA                                      |
| **SDP**                | Session Description Protocol — usado no handshake WebRTC                                     |
| **PKCE**               | Proof Key for Code Exchange — fluxo OAuth seguro para apps desktop                           |
| **Tiling**             | Gerenciamento de janelas em mosaico (sem sobreposição)                                       |
| **Zen Mode**           | Modo de foco onde um painel ocupa 100% da tela                                               |
| **God Mode**           | Broadcast Input — enviar comando para todos os agentes simultaneamente                       |
| **Signaling Server**   | Servidor intermediário para troca de informações de conexão WebRTC                           |
| **Waiting Room**       | Mecanismo de segurança onde o Host aprova a entrada de cada Guest                            |
| **File Watcher**       | Monitor de mudanças no filesystem (`.git`) para sincronizar GUI                              |
| **Smart Layout**       | Algoritmo que distribui automaticamente os painéis baseado na quantidade de agentes           |

---

> **Nota**: Este PRD é um documento vivo. Deve ser atualizado conforme decisões arquiteturais evoluam durante o desenvolvimento.
>
> **Última atualização**: 12 de Fevereiro de 2026
