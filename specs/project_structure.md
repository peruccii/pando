# Spec: Wails App Bootstrap & Project Structure

> **Módulo**: Core — Foundation  
> **Status**: Draft  
> **PRD Ref**: Seção 5, 6, 15 (Fase 0)  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Definir a estrutura do projeto Wails + React (Vite), organização de módulos Go (services), configuração do build e ponto de entrada do app.

---

## 2. Estrutura do Projeto

```
orch/
├── PRD.md
├── specs/
│   └── *.md
├── build/                       # Wails build config
│   ├── appicon.png
│   ├── darwin/
│   │   └── Info.plist           # Deep links (orch://)
│   └── windows/                 # N/A (macOS only)
├── main.go                      # Entrypoint
├── app.go                       # Wails App struct + lifecycle
├── go.mod
├── go.sum
├── wails.json                   # Wails config
│
├── internal/                    # Business logic (Go)
│   ├── auth/
│   │   ├── service.go           # AuthService (OAuth, Keychain)
│   │   ├── pkce.go              # PKCE flow helpers
│   │   └── types.go
│   ├── database/
│   │   ├── service.go           # DBService (GORM + SQLite)
│   │   ├── models.go            # GORM models
│   │   └── migrations.go
│   ├── github/
│   │   ├── service.go           # GitHubService (GraphQL)
│   │   ├── queries.go           # GraphQL query strings
│   │   ├── cache.go             # In-memory cache
│   │   ├── polling.go           # Polling strategy
│   │   └── types.go
│   ├── ai/
│   │   ├── service.go           # AIService (LLM proxy)
│   │   ├── context_builder.go   # Prompt augmentation
│   │   ├── providers.go         # Gemini, OpenAI, Ollama
│   │   ├── sanitizer.go         # SecretSanitizer
│   │   └── types.go
│   ├── terminal/
│   │   ├── pty_manager.go       # PTY lifecycle
│   │   ├── bridge.go            # Terminal Bridge (I/O streaming)
│   │   └── types.go
│   ├── session/
│   │   ├── service.go           # Session management
│   │   ├── signaling.go         # WebRTC signaling
│   │   ├── short_code.go        # Code generator
│   │   └── types.go
│   ├── docker/
│   │   ├── service.go           # Docker container management
│   │   ├── detector.go          # Auto-detect project type
│   │   └── types.go
│   ├── filewatcher/
│   │   ├── service.go           # Git file watcher
│   │   └── types.go
│   └── config/
│       ├── paths.go             # OS-specific paths
│       └── constants.go
│
├── frontend/                    # React (Vite)
│   ├── index.html
│   ├── package.json
│   ├── tsconfig.json
│   ├── vite.config.ts
│   ├── src/
│   │   ├── main.tsx             # React root
│   │   ├── App.tsx              # Root component
│   │   ├── index.css            # Design system tokens
│   │   │
│   │   ├── components/          # Shared/generic components
│   │   │   ├── Button.tsx
│   │   │   ├── Input.tsx
│   │   │   ├── Modal.tsx
│   │   │   ├── Toast.tsx
│   │   │   ├── Badge.tsx
│   │   │   ├── Dropdown.tsx
│   │   │   └── AuthGuard.tsx
│   │   │
│   │   ├── features/            # Feature modules
│   │   │   ├── command-center/  # Grid/Mosaic UI
│   │   │   ├── terminal/        # xterm.js integration
│   │   │   ├── github/          # PR, Issues, Branches
│   │   │   ├── ai/              # AI agent panels
│   │   │   ├── session/         # P2P collaboration
│   │   │   └── settings/        # User settings
│   │   │
│   │   ├── hooks/               # Shared hooks
│   │   │   ├── useAuth.ts
│   │   │   ├── useWails.ts
│   │   │   ├── useKeyboardShortcuts.ts
│   │   │   └── useTheme.ts
│   │   │
│   │   ├── stores/              # Zustand stores
│   │   │   ├── authStore.ts
│   │   │   ├── layoutStore.ts
│   │   │   ├── githubStore.ts
│   │   │   └── sessionStore.ts
│   │   │
│   │   └── lib/                 # Utilities
│   │       ├── wails.ts         # Wails bindings wrapper
│   │       ├── p2p.ts           # WebRTC helpers
│   │       └── utils.ts
│   │
│   └── wailsjs/                 # Auto-generated Wails bindings
│       ├── go/
│       └── runtime/
│
└── scripts/
    ├── build.sh
    └── dev.sh
```

---

## 3. App Lifecycle (app.go)

```go
type App struct {
    ctx         context.Context

    // Services
    auth        IAuthService
    db          IDBService
    github      IGitHubService
    ai          IAIService
    pty         IPTYManager
    session     ISessionService
    docker      IDockerService
    fileWatcher IFileWatcher

    // State
    state       *AppState
}

type AppState struct {
    IsAuthenticated bool
    User            *User
    ActiveWorkspace *Workspace
}

// Wails lifecycle hooks
func (a *App) Startup(ctx context.Context)  { /* Bootstrap */ }
func (a *App) Shutdown(ctx context.Context) { /* Cleanup */ }
func (a *App) DomReady(ctx context.Context) { /* Post-render init */ }
```

---

## 4. Wails Configuration (wails.json)

```json
{
    "name": "ORCH",
    "outputfilename": "ORCH",
    "frontend:install": "npm install",
    "frontend:build": "npm run build",
    "frontend:dev:watcher": "npm run dev",
    "frontend:dev:serverUrl": "auto",
    "author": {
        "name": "perucci"
    },
    "info": {
        "companyName": "ORCH",
        "productName": "ORCH",
        "copyright": "2026"
    }
}
```

---

## 5. Build & Dev

```bash
# Desenvolvimento
wails dev

# Build para macOS
wails build -platform darwin/universal

# O output será um .app bundle em build/bin/
```

---

## 6. Dependências Go

```
// go.mod (principais)
require (
    github.com/wailsapp/wails/v2
    gorm.io/gorm
    github.com/glebarez/sqlite
    github.com/zalando/go-keyring
    github.com/creack/pty
    github.com/fsnotify/fsnotify
    github.com/gorilla/websocket
    github.com/sashabaranov/go-openai
    google.golang.org/genai
)
```

---

## 7. Dependências Frontend (package.json)

```json
{
    "dependencies": {
        "react": "^18.x",
        "react-dom": "^18.x",
        "xterm": "^5.x",
        "@xterm/addon-fit": "^0.10",
        "@xterm/addon-webgl": "^0.18",
        "@xterm/addon-search": "^0.15",
        "react-mosaic-component": "^6.x",
        "zustand": "^4.x",
        "lucide-react": "^0.x",
        "yjs": "^13.x",
        "y-webrtc": "^10.x"
    },
    "devDependencies": {
        "typescript": "^5.x",
        "vite": "^5.x",
        "@vitejs/plugin-react": "^4.x"
    }
}
```

---

## 8. Dependências

Todas as specs dependem desta spec como fundação.
