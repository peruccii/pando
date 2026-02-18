# Spec: Scroll Sync (Colaboração em Diff)

> **Módulo**: Transversal — Collaboration UX  
> **Status**: Draft  
> **PRD Ref**: Seção 7.4  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Quando um usuário comenta em uma linha de código na GUI (Diff Viewer), o sistema deve enviar um evento de **"Scroll Sync"** via WebRTC para alinhar a tela de todos os participantes na mesma posição.

---

## 2. Fluxo

```
User A comenta na linha 42 de "main.go"
    │
    ├── 1. UI abre InlineComment na linha 42
    │
    ├── 2. WebRTC Event enviado:
    │       {
    │         type: "scroll_sync",
    │         file: "main.go",
    │         line: 42,
    │         userID: "user-a",
    │         userName: "Perucci",
    │         action: "comment"
    │       }
    │
    └── 3. Todos os Guests recebem o evento:
            ├── DiffViewer navega para "main.go"
            ├── Expande o arquivo (se colapsado)
            ├── Scroll viewport para linha 42
            └── Highlight temporário (2s) com cor do User A
```

---

## 3. Evento WebRTC

```typescript
interface ScrollSyncEvent {
    type: 'scroll_sync'
    file: string          // Path relativo do arquivo
    line: number          // Linha no diff
    userID: string
    userName: string
    userColor: string     // Cor do cursor de awareness
    action: 'comment' | 'navigate' | 'review'
    timestamp: number
}
```

---

## 4. Comportamento no Receptor

```typescript
function handleScrollSync(event: ScrollSyncEvent) {
    const diffViewer = getDiffViewer()

    // 1. Navegar para o arquivo
    diffViewer.navigateToFile(event.file)

    // 2. Expandir se colapsado
    diffViewer.expandFile(event.file)

    // 3. Scroll para a linha
    diffViewer.scrollToLine(event.line, {
        behavior: 'smooth',
        block: 'center',
    })

    // 4. Highlight temporário
    diffViewer.highlightLine(event.line, {
        color: event.userColor,
        duration: 2000, // 2 segundos
        label: event.userName,
    })

    // 5. Toast discreto
    toast.info(`${event.userName} está olhando ${event.file}:${event.line}`, {
        duration: 3000,
        position: 'bottom-right',
    })
}
```

---

## 5. Configuração do Usuário

| Setting                  | Default | Descrição                                |
| ------------------------- | ------- | ---------------------------------------- |
| `scrollSync.enabled`      | `true`  | Habilitar/desabilitar scroll sync         |
| `scrollSync.autoFollow`   | `true`  | Seguir automaticamente ou apenas notificar |
| `scrollSync.showToast`    | `true`  | Exibir toast de navegação                  |

Se `autoFollow = false`, o usuário vê apenas um toast: *"Perucci está em main.go:42 — [Ir para lá]"*

---

## 6. Anti-Spam

- **Debounce**: Máximo 1 evento de scroll sync por usuário a cada 2 segundos.
- **Ignore self**: O emissor não recebe seu próprio evento.
- **Rate limit**: Máximo 10 eventos por minuto por sessão.

---

## 7. Dependências

| Dependência            | Tipo       | Spec Relacionada     |
| ----------------------- | ---------- | -------------------- |
| WebRTC Data Channel     | Bloqueador | invite_and_p2p       |
| PRDiffViewer             | Bloqueador | github_integration   |
