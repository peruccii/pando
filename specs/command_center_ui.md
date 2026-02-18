# Spec: UX/UI — "Command Center" (Grid Dinâmico de Agentes)

> **Módulo**: 5 — Command Center UI  
> **Status**: Draft  
> **PRD Ref**: Seção 11  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Interface de **"Mosaico Infinito"** para orquestrar múltiplos processos de IA simultaneamente. Eliminar troca de abas, oferecendo visão **panóptica** com controle granular de redimensionamento e foco.

---

## 2. Conceito — "Bento Box Dinâmico"

Padrão **Split Panes** (painéis divididos), similar ao `tmux`/`i3wm`, com facilidade de mouse.

---

## 3. Smart Layout (Auto-Grid)

| Nº Agentes | Layout                                                    |
| ----------- | --------------------------------------------------------- |
| 1           | Tela Cheia (100%)                                          |
| 2           | Split Vertical 50/50                                       |
| 3           | Principal esquerda (50%) + 2 menores empilhados (25/25)    |
| 4           | Grid 2×2                                                   |
| 5-6         | Grid 2×3 ou 3×2                                            |
| 7-9         | Grid 3×3 com slots vazios                                  |
| 10+         | Grid automático (3×4 ou 4×3) com scroll vertical           |

### 3.1 Regras de Layout

```typescript
interface LayoutRule {
    minPaneWidth: number   // 300px
    minPaneHeight: number  // 200px
    gutterSize: number     // 6px (draggable border)
    headerHeight: number   // 28px
    padding: number        // 2px entre painéis
}

function calculateLayout(count: number, container: DOMRect): PaneLayout[] {
    if (count === 1) return [fullscreen(container)]
    if (count === 2) return splitVertical(container, [50, 50])
    if (count === 3) return [
        { ...leftPane(container, 50) },
        { ...topRight(container, 50, 50) },
        { ...bottomRight(container, 50, 50) },
    ]
    // Grid automático para 4+
    const cols = Math.ceil(Math.sqrt(count))
    const rows = Math.ceil(count / cols)
    return generateGrid(container, cols, rows, count)
}
```

---

## 4. Interações

### 4.1 Resizing (Draggable Gutters)

```typescript
// Bordas entre painéis são "agarráveis"
interface GutterProps {
    direction: 'horizontal' | 'vertical'
    size: number          // 6px
    cursor: string        // "col-resize" | "row-resize"
    onDragStart: () => void
    onDrag: (delta: number) => void
    onDragEnd: () => void
}

// No resize, terminais adjacentes recalculam
function handleGutterDrag(paneA: Pane, paneB: Pane, delta: number) {
    paneA.width += delta
    paneB.width -= delta
    // Disparar fitAddon.fit() em ambos os terminais
    paneA.terminal.fitAddon.fit()
    paneB.terminal.fitAddon.fit()
}
```

### 4.2 Drag & Drop (Reorganização)

```typescript
interface DragDropBehavior {
    dragHandle: string     // ".pane-header" (clicável)
    dropZones: string      // ".pane-container"
    feedback: {
        dragging: 'opacity-50 scale-95'      // Feedback no drag
        dropTarget: 'border-accent glow'     // Drop zone highlight
    }
    onDrop: (sourceID: string, targetID: string) => void  // Swap positions
}
```

### 4.3 Zen Mode (Foco)

```typescript
interface ZenMode {
    isActive: boolean
    paneID: string | null
    previousLayout: PaneLayout[]  // Para restaurar

    enter(paneID: string): void   // Maximiza (z-index superior, 100% tela)
    exit(): void                  // Restaura layout anterior
    toggle(paneID: string): void  // Atalho: duplo-clique no header
}
```

**Atalho**: Duplo-clique no header ou `Cmd+Enter` no painel ativo.

---

## 5. Hierarquia Visual

### 5.1 Foco Ativo

```css
/* Painel com foco */
.pane--active {
    border: 2px solid var(--accent-color);
    box-shadow: 0 0 12px rgba(var(--accent-rgb), 0.3);
}

/* Painéis inativos */
.pane--inactive {
    opacity: 0.85;
    border: 1px solid var(--border-muted);
}

/* Transição suave */
.pane {
    transition: opacity 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}
```

### 5.2 Header do Painel (28px)

```
┌──────────────────────────────────────────────┐
│ 🟢 Refatorador SQL          🔍  🔄  🗑️  ⛶  │
└──────────────────────────────────────────────┘
  │        │                   │   │   │   │
  │        └─ Nome do Agente   │   │   │   └─ Zen Mode
  │                            │   │   └───── Kill
  └─ Status Indicator         │   └───────── Restart
                               └──────────── Search/Logs

Indicadores:
  🟢 idle     (Ocioso/Pronto)
  🟡 running  (Escrevendo/Pensando) — animação pulsante
  🔴 error    (Erro/Ação Necessária) — badge de notificação
```

### 5.3 CSS do Header

```css
.pane-header {
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 8px;
    background: var(--bg-header);
    border-bottom: 1px solid var(--border-subtle);
    cursor: grab;
    user-select: none;
    font-size: 12px;
    font-weight: 500;
}

.pane-header__name {
    display: flex;
    align-items: center;
    gap: 6px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.pane-header__controls {
    display: flex;
    gap: 4px;
}

.pane-header__controls button {
    width: 22px;
    height: 22px;
    border-radius: 4px;
    border: none;
    background: transparent;
    cursor: pointer;
    opacity: 0.6;
    transition: opacity 0.15s, background 0.15s;
}

.pane-header__controls button:hover {
    opacity: 1;
    background: var(--bg-hover);
}

/* Indicador pulsante para "running" */
@keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
}

.status-indicator--running {
    animation: pulse 1.5s ease-in-out infinite;
}
```

---

## 6. Stack Frontend

### 6.1 Bibliotecas

| Componente        | Biblioteca                       | Versão   |
| ------------------ | -------------------------------- | -------- |
| Tiling/Mosaic      | `react-mosaic-component`         | ^6.x     |
| Terminal           | `xterm` + `@xterm/addon-fit`     | ^5.x     |
| WebGL              | `@xterm/addon-webgl`             | ^0.18    |
| Icons              | `lucide-react`                   | ^0.x     |
| State Management   | `zustand`                        | ^4.x     |

### 6.2 Estrutura de Componentes

```
src/features/command-center/
├── components/
│   ├── CommandCenter.tsx      # Container principal (Mosaic)
│   ├── PaneContainer.tsx      # Wrapper de cada painel
│   ├── PaneHeader.tsx         # Header com status + controles
│   ├── TerminalPane.tsx       # Painel de terminal (xterm.js)
│   ├── AIAgentPane.tsx        # Painel de agente de IA
│   ├── GitHubPane.tsx         # Painel GitHub (PR/Issues)
│   ├── ZenModeOverlay.tsx     # Overlay de tela cheia
│   └── BroadcastBar.tsx       # Barra de God Mode (rodapé)
├── hooks/
│   ├── useLayout.ts           # Lógica de layout automático
│   ├── usePaneFocus.ts        # Gerenciamento de foco
│   ├── useZenMode.ts          # Toggle zen mode
│   └── useBroadcast.ts        # Broadcast input
├── stores/
│   └── layoutStore.ts         # Estado do grid (zustand)
└── types/
    └── layout.ts
```

---

## 7. Broadcast Input — "God Mode"

### 7.1 UI

Barra fixa no rodapé da interface:

```
┌──────────────────────────────────────────────────────────────┐
│  ⚡ BROADCAST MODE  │  [____input_field____]  │  Send All  │
│                     │                          │  [Ctrl+↵]  │
└──────────────────────────────────────────────────────────────┘
```

### 7.2 Comportamento

```typescript
interface BroadcastState {
    isActive: boolean
    targetAgents: string[]  // IDs dos agentes alvo (default: todos)

    activate(): void
    deactivate(): void
    send(message: string): void
}

function broadcastMessage(message: string, agents: AgentInstance[]) {
    agents.forEach(agent => {
        const term = getTerminal(agent.id)
        // Escrever no stdin de cada agente
        wails.Call('PTYManager.Write', agent.sessionID, message + '\n')
    })
}
```

### 7.3 Atalho

- **Ativar**: `Cmd+Shift+B`
- **Enviar**: `Ctrl+Enter` (dentro da barra)
- **Desativar**: `Escape`

---

## 8. Persistência de Layout

```typescript
// Salvar layout no SQLite ao modificar
function saveLayout(agents: AgentInstance[]) {
    agents.forEach(agent => {
        wails.Call('DBService.UpdateAgentLayout', agent.id,
            agent.windowX, agent.windowY,
            agent.windowWidth, agent.windowHeight
        )
    })
}

// Restaurar layout ao abrir o app
function restoreLayout(agents: AgentInstance[]): MosaicNode<string> {
    // Converte AgentInstance[] com coordenadas para MosaicNode tree
    return buildMosaicTree(agents)
}
```

---

## 9. Atalhos de Teclado

| Atalho            | Ação                               |
| ------------------ | ---------------------------------- |
| `Cmd+N`            | Novo agente/terminal                |
| `Cmd+W`            | Fechar painel ativo                 |
| `Cmd+1-9`          | Focar painel por índice             |
| `Cmd+[` / `Cmd+]`  | Navegar entre painéis               |
| `Cmd+Enter`        | Toggle Zen Mode no painel ativo     |
| `Cmd+B`            | Toggle sidebar                      |
| `Cmd+Shift+B`      | Toggle Broadcast Mode               |
| `Escape`           | Sair do Zen Mode / Broadcast         |

---

## 10. Virtualização de Renderização

| Estado do Painel   | Renderização                                  |
| ------------------- | --------------------------------------------- |
| Foco ativo          | WebGL, 60fps                                   |
| Visível, sem foco   | Canvas 2D, 30fps                               |
| Minimizado          | `display:none`, buffer de dados mantido (64KB) |
| Fora do viewport    | `display:none`, buffer mantido                 |

---

## 11. Dependências

| Dependência                     | Tipo       |
| -------------------------------- | ---------- |
| `react-mosaic-component`        | Bloqueador |
| `xterm` + FitAddon + WebGL      | Bloqueador |
| `zustand` (state management)    | Bloqueador |
| auth_and_persistence (layout)   | Bloqueador |
| terminal_sharing                 | Bloqueador |
