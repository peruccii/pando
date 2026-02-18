# Spec: Broadcast Input — "God Mode"

> **Módulo**: Transversal — UX Feature  
> **Status**: Draft  
> **PRD Ref**: Seção 11.6  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Permitir que o usuário envie um único input para **TODOS** os agentes/terminais simultaneamente. Funcionalidade de orquestração para "vibe coding" extremo.

---

## 2. UI

### 2.1 Barra de Broadcast (Rodapé)

```
┌──────────────────────────────────────────────────────────────────────┐
│  ⚡ BROADCAST  │  Targets: All (10) ▾  │  [___input___]  │  ⌃↵ Send │
└──────────────────────────────────────────────────────────────────────┘
```

- **Toggle**: `Cmd+Shift+B` ativa/desativa.
- **Target Selector**: Dropdown para selecionar quais agentes receberão (default: todos).
- **Input Field**: Auto-focus quando ativado.
- **Send**: `Ctrl+Enter` ou botão.
- **Escape**: Desativa o modo.

### 2.2 Feedback Visual

Quando o broadcast mode está ativo:
- Borda inferior da tela pulsa com a cor accent.
- Todos os terminais-alvo recebem badge "⚡" temporário.
- Header de cada terminal pisca ao receber o input broadcast.

---

## 3. Implementação

### 3.1 Store (Zustand)

```typescript
interface BroadcastStore {
    isActive: boolean
    targetAgentIDs: string[]  // IDs dos agentes alvo
    history: string[]         // Últimos 20 comandos broadcast

    activate: () => void
    deactivate: () => void
    toggle: () => void
    setTargets: (ids: string[]) => void
    send: (message: string) => void
}
```

### 3.2 Envio

```typescript
function broadcastSend(message: string, targetIDs: string[]) {
    const agents = targetIDs.length > 0
        ? layoutStore.getAgentsByIDs(targetIDs)
        : layoutStore.getAllAgents()

    agents.forEach(agent => {
        if (agent.status !== 'stopped') {
            // Enviar para o PTY de cada agente
            wails.Call('PTYManager.Write', agent.sessionID, message + '\n')
        }
    })

    // Feedback visual em cada terminal
    agents.forEach(agent => {
        highlightTerminal(agent.id, 'broadcast', 500) // 500ms highlight
    })
}
```

### 3.3 Target Selector

```typescript
type TargetFilter = 'all' | 'running' | 'idle' | 'custom'

function getTargets(filter: TargetFilter, customIDs?: string[]): string[] {
    const agents = layoutStore.getAllAgents()

    switch (filter) {
        case 'all':     return agents.map(a => a.id)
        case 'running': return agents.filter(a => a.status === 'running').map(a => a.id)
        case 'idle':    return agents.filter(a => a.status === 'idle').map(a => a.id)
        case 'custom':  return customIDs || []
    }
}
```

---

## 4. Atalhos

| Atalho           | Ação                          |
| ----------------- | ----------------------------- |
| `Cmd+Shift+B`     | Toggle Broadcast Mode          |
| `Ctrl+Enter`      | Enviar mensagem broadcast      |
| `Escape`           | Desativar Broadcast Mode       |
| `↑` / `↓`         | Navegar histórico de broadcast |

---

## 5. Casos de Uso

| Cenário                    | Input                            | Resultado                              |
| --------------------------- | -------------------------------- | -------------------------------------- |
| Parar tudo                  | `Ctrl+C`                         | Envia SIGINT para todos os terminais   |
| Atualizar dependências      | `npm update`                     | Roda em todos os terminais Node        |
| Pergunta à IA (broadcast)   | `/ai Resuma o que você fez`      | Todos os agentes respondem             |
| Limpar terminais            | `clear`                          | Todos os terminais são limpos          |

---

## 6. Segurança

- Broadcast **não** envia para terminais de Guests em sessão P2P (apenas terminais locais).
- Broadcast para terminais Docker-sandboxed: seguro.
- Broadcast para terminais Live Share: usar com responsabilidade.

---

## 7. Dependências

| Dependência            | Tipo       | Spec Relacionada    |
| ----------------------- | ---------- | ------------------- |
| command_center_ui       | Bloqueador | command_center_ui   |
| terminal_sharing (PTY)  | Bloqueador | terminal_sharing    |
| zustand (store)         | Bloqueador | —                   |
