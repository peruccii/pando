# Spec: Optimistic UI Pattern

> **Módulo**: Transversal — UX Pattern  
> **Status**: Draft  
> **PRD Ref**: Seção 7.3  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Implementar o padrão **Optimistic UI Update** em todas as ações que envolvem persistência remota (GitHub API, WebRTC broadcast). A latência de rede **não deve travar a interface**.

---

## 2. Conceito

```
1. AÇÃO LOCAL      → UI atualiza imediatamente (status: "Pendente")
2. BROADCAST P2P   → Outros participantes veem a ação (status: "Pendente")
3. PERSIST ASYNC   → Backend envia para API externa (GitHub, etc.)
4a. SUCESSO        → Status muda para "Enviado ✓"
4b. ERRO           → Feedback visual + opção de retry + rollback
```

---

## 3. Ações que usam Optimistic UI

| Ação                        | API Destino      | Rollback possível? |
| ---------------------------- | ---------------- | ------------------ |
| Criar comentário em PR       | GitHub GraphQL   | ✅ Sim              |
| Criar review                 | GitHub GraphQL   | ✅ Sim              |
| Aprovar/Rejeitar PR          | GitHub GraphQL   | ✅ Sim              |
| Merge PR                     | GitHub GraphQL   | ❌ Não (irreversível) |
| Criar Issue                  | GitHub GraphQL   | ✅ Sim              |
| Mover Issue no Kanban        | GitHub GraphQL   | ✅ Sim              |
| Criar branch                 | GitHub GraphQL   | ✅ Sim              |

---

## 4. Implementação (React/TypeScript)

### 4.1 Hook genérico

```typescript
type OptimisticStatus = 'idle' | 'pending' | 'success' | 'error'

interface OptimisticAction<T> {
    id: string
    data: T
    status: OptimisticStatus
    error?: string
    retryCount: number
    maxRetries: number
    createdAt: number
}

function useOptimisticAction<T>(
    action: (data: T) => Promise<T>,
    options: {
        onSuccess?: (result: T) => void
        onError?: (err: Error, data: T) => void
        onRollback?: (data: T) => void
        maxRetries?: number
        broadcastChannel?: string
    }
) {
    const [items, setItems] = useState<OptimisticAction<T>[]>([])

    const execute = async (data: T) => {
        const id = generateID()
        const optimistic: OptimisticAction<T> = {
            id, data,
            status: 'pending',
            retryCount: 0,
            maxRetries: options.maxRetries ?? 3,
            createdAt: Date.now(),
        }

        // 1. Atualizar UI imediatamente
        setItems(prev => [...prev, optimistic])

        // 2. Broadcast P2P (se configurado)
        if (options.broadcastChannel) {
            p2p.broadcast(options.broadcastChannel, {
                type: 'optimistic:pending', id, data
            })
        }

        // 3. Persistir assincronamente
        try {
            const result = await action(data)
            setItems(prev => prev.map(i =>
                i.id === id ? { ...i, status: 'success', data: result } : i
            ))
            // Broadcast sucesso
            if (options.broadcastChannel) {
                p2p.broadcast(options.broadcastChannel, {
                    type: 'optimistic:success', id, data: result
                })
            }
            options.onSuccess?.(result)
        } catch (err) {
            setItems(prev => prev.map(i =>
                i.id === id ? { ...i, status: 'error', error: err.message } : i
            ))
            // Broadcast erro
            if (options.broadcastChannel) {
                p2p.broadcast(options.broadcastChannel, {
                    type: 'optimistic:error', id, error: err.message
                })
            }
            options.onError?.(err, data)
        }
    }

    const retry = async (id: string) => {
        const item = items.find(i => i.id === id)
        if (!item || item.retryCount >= item.maxRetries) return
        // Re-executar com contador incrementado
        setItems(prev => prev.map(i =>
            i.id === id ? { ...i, status: 'pending', retryCount: i.retryCount + 1 } : i
        ))
        await execute(item.data)
    }

    const rollback = (id: string) => {
        const item = items.find(i => i.id === id)
        if (item) {
            setItems(prev => prev.filter(i => i.id !== id))
            options.onRollback?.(item.data)
        }
    }

    return { items, execute, retry, rollback }
}
```

### 4.2 Exemplo de Uso — Comentário em PR

```typescript
const { items: comments, execute: addComment, retry } = useOptimisticAction(
    async (comment: CreateCommentInput) => {
        return await wails.Call('GitHubService.CreateComment', comment)
    },
    {
        broadcastChannel: 'github:comments',
        onError: (err) => toast.error(`Falha ao enviar: ${err.message}`),
        maxRetries: 3,
    }
)
```

### 4.3 Feedback Visual

```css
/* Comentário pendente */
.comment--pending {
    opacity: 0.7;
    border-left: 3px solid var(--color-warning);
}

.comment--pending::after {
    content: "Enviando...";
    font-size: 11px;
    color: var(--color-warning);
}

/* Comentário com erro */
.comment--error {
    opacity: 0.8;
    border-left: 3px solid var(--color-error);
}

/* Comentário confirmado */
.comment--success {
    opacity: 1;
    border-left: 3px solid var(--color-success);
    transition: border-color 0.5s ease;
}
```

---

## 5. Merge PR — Caso Especial (Irreversível)

Para ações irreversíveis, **NÃO** usar Optimistic UI. Usar fluxo síncrono com loading:

```
1. Modal de confirmação: "Tem certeza que deseja fazer merge?"
2. Botão muda para loading spinner
3. Aguardar resposta da API
4. Sucesso → Fechar modal + Toast de sucesso
5. Erro → Exibir erro no modal + retry
```

---

## 6. Dependências

| Dependência            | Tipo       | Spec Relacionada     |
| ----------------------- | ---------- | -------------------- |
| WebRTC Data Channel     | Bloqueador | invite_and_p2p       |
| GitHub GraphQL          | Bloqueador | github_integration   |
