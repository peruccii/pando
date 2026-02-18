# Spec: Keyboard Shortcuts

> **Módulo**: Transversal — UX  
> **Status**: Draft  
> **PRD Ref**: Seção 11.9, 14  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Atalhos de teclado para todas as ações principais do ORCH, garantindo que não conflitem com o terminal (xterm.js) quando em foco.

---

## 2. Mapa de Atalhos

### 2.1 Gerenciamento de Painéis

| Atalho             | Ação                               | Contexto         |
| ------------------- | ---------------------------------- | ---------------- |
| `Cmd+N`             | Novo terminal/agente                | Global           |
| `Cmd+W`             | Fechar painel ativo                 | Global           |
| `Cmd+1` a `Cmd+9`   | Focar painel por índice (1-9)       | Global           |
| `Cmd+[`             | Focar painel anterior               | Global           |
| `Cmd+]`             | Focar painel seguinte               | Global           |
| `Cmd+Enter`         | Toggle Zen Mode (maximizar/restaurar)| Global          |
| `Cmd+\`             | Split vertical (novo painel ao lado) | Global          |
| `Cmd+Shift+\`       | Split horizontal (empilhar)          | Global          |

### 2.2 Sidebar & Navegação

| Atalho             | Ação                               |
| ------------------- | ---------------------------------- |
| `Cmd+B`             | Toggle sidebar                      |
| `Cmd+Shift+G`       | Abrir painel GitHub                  |
| `Cmd+Shift+I`       | Abrir painel Issues                  |
| `Cmd+Shift+P`       | Abrir painel Pull Requests           |
| `Cmd+K`             | Abrir Command Palette (busca rápida) |

### 2.3 Broadcast & Colaboração

| Atalho             | Ação                               |
| ------------------- | ---------------------------------- |
| `Cmd+Shift+B`       | Toggle Broadcast Mode               |
| `Ctrl+Enter`        | Enviar broadcast (quando no modo)    |
| `Cmd+Shift+S`       | Start/Stop sessão de compartilhamento|
| `Cmd+Shift+J`       | Join sessão (abrir diálogo)          |

### 2.4 Geral

| Atalho             | Ação                               |
| ------------------- | ---------------------------------- |
| `Cmd+,`             | Abrir Settings                      |
| `Cmd+Q`             | Sair do app                         |
| `Escape`            | Sair do Zen Mode / Broadcast / Modal |
| `Cmd+Shift+D`       | Toggle Dark/Light theme              |

---

## 3. Conflito com Terminal

### 3.1 Problema

Quando o xterm.js está em foco, atalhos como `Cmd+C`, `Cmd+V` devem funcionar no terminal, não na aplicação.

### 3.2 Solução

```typescript
function useKeyboardShortcuts() {
    useEffect(() => {
        const handler = (e: KeyboardEvent) => {
            // Se o foco está no terminal, permitir apenas atalhos "escape"
            const isTerminalFocused = document.activeElement?.closest('.xterm')

            if (isTerminalFocused) {
                // Apenas atalhos que DEVEM funcionar mesmo com terminal em foco
                const allowedInTerminal = [
                    { key: 'Escape' },
                    { key: 'Enter', meta: true },           // Zen Mode
                    { key: 'b', meta: true, shift: true },  // Broadcast
                    { key: '1', meta: true },               // Focus pane 1
                    { key: '2', meta: true },               // Focus pane 2
                    // ... Cmd+1-9
                    { key: '[', meta: true },               // Prev pane
                    { key: ']', meta: true },               // Next pane
                    { key: 'n', meta: true },               // New pane
                    { key: 'w', meta: true },               // Close pane
                ]

                const isAllowed = allowedInTerminal.some(s =>
                    e.key === s.key &&
                    (!s.meta || e.metaKey) &&
                    (!s.shift || e.shiftKey)
                )

                if (!isAllowed) return // Deixar o terminal processar
            }

            // Processar atalho da aplicação
            handleShortcut(e)
        }

        document.addEventListener('keydown', handler)
        return () => document.removeEventListener('keydown', handler)
    }, [])
}
```

---

## 4. Command Palette (`Cmd+K`)

Quick-search para todas as ações do app:

```
┌──────────────────────────────────────────────┐
│  🔍 Buscar ação...                           │
│                                              │
│  > Novo Terminal           Cmd+N             │
│  > Abrir Pull Requests     Cmd+Shift+P       │
│  > Iniciar Sessão          Cmd+Shift+S       │
│  > Broadcast Mode          Cmd+Shift+B       │
│  > Configurações           Cmd+,             │
│  > Trocar Tema             Cmd+Shift+D       │
└──────────────────────────────────────────────┘
```

- Filtro fuzzy conforme o usuário digita.
- `Enter` executa. `Escape` fecha.
- Mostra o atalho correspondente ao lado de cada ação.

---

## 5. ARIA Labels

Todos os elementos interativos devem ter ARIA labels que mencionam o atalho:

```html
<button
    aria-label="Novo terminal (Cmd+N)"
    title="Novo terminal (⌘N)"
>
    + Novo
</button>
```

---

## 6. Dependências

| Dependência            | Tipo       |
| ----------------------- | ---------- |
| command_center_ui       | Bloqueador |
| terminal_sharing        | Bloqueador |
