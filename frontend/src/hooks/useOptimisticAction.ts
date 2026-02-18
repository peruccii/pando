import { useState, useCallback, useRef } from 'react'

// ============================================
// useOptimisticAction — Hook genérico para Optimistic UI
// ============================================

export type OptimisticStatus = 'idle' | 'pending' | 'success' | 'error'

export interface OptimisticAction<T> {
  /** ID único da ação */
  id: string
  /** Dados da ação */
  data: T
  /** Status atual */
  status: OptimisticStatus
  /** Mensagem de erro (se houver) */
  error?: string
  /** Número de retentativas já feitas */
  retryCount: number
  /** Máximo de retentativas */
  maxRetries: number
  /** Timestamp de criação */
  createdAt: number
}

export interface OptimisticOptions<T, R = T> {
  /** Callback de sucesso */
  onSuccess?: (result: R) => void
  /** Callback de erro */
  onError?: (err: Error, data: T) => void
  /** Callback de rollback */
  onRollback?: (data: T) => void
  /** Máximo de retentativas (default: 3) */
  maxRetries?: number
  /** Canal P2P para broadcast (futuro) */
  broadcastChannel?: string
  /** Tempo (ms) para limpar itens com sucesso (default: 3000) */
  successTimeout?: number
}

let idCounter = 0
function generateId(): string {
  return `opt_${Date.now()}_${++idCounter}`
}

/**
 * useOptimisticAction — Hook genérico para Optimistic UI.
 *
 * Pipeline:
 *   1. AÇÃO LOCAL → UI atualiza imediatamente (status: "pending")
 *   2. BROADCAST P2P → Outros participantes veem a ação
 *   3. PERSIST ASYNC → Backend envia para API externa
 *   4a. SUCESSO → Status muda para "success" (auto-limpa após timeout)
 *   4b. ERRO → Feedback visual + opção de retry + rollback
 *
 * @param action Função assíncrona a executar
 * @param options Configurações de retry, callbacks, etc.
 */
export function useOptimisticAction<T, R = T>(
  action: (data: T) => Promise<R>,
  options: OptimisticOptions<T, R> = {}
) {
  const [items, setItems] = useState<OptimisticAction<T>[]>([])
  const timeoutRefs = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map())

  const {
    maxRetries = 3,
    successTimeout = 3000,
    broadcastChannel,
    onSuccess,
    onError,
    onRollback,
  } = options

  // Limpa um item do estado após timeout
  const scheduleCleanup = useCallback((id: string, delay: number) => {
    const existing = timeoutRefs.current.get(id)
    if (existing) clearTimeout(existing)

    const timeout = setTimeout(() => {
      setItems(prev => prev.filter(i => i.id !== id))
      timeoutRefs.current.delete(id)
    }, delay)

    timeoutRefs.current.set(id, timeout)
  }, [])

  // Executar ação com optimistic update
  const execute = useCallback(async (data: T): Promise<R | undefined> => {
    const id = generateId()

    const optimistic: OptimisticAction<T> = {
      id,
      data,
      status: 'pending',
      retryCount: 0,
      maxRetries,
      createdAt: Date.now(),
    }

    // 1. Atualizar UI imediatamente
    setItems(prev => [...prev, optimistic])

    // 2. Broadcast P2P (preparado para futuro)
    if (broadcastChannel && typeof window !== 'undefined') {
      // TODO: Integrar com módulo P2P quando implementado
      // p2p.broadcast(broadcastChannel, { type: 'optimistic:pending', id, data })
      console.debug(`[Optimistic] Broadcast pending → ${broadcastChannel}`, { id })
    }

    // 3. Persistir assincronamente
    try {
      const result = await action(data)

      setItems(prev => prev.map(i =>
        i.id === id ? { ...i, status: 'success' as const } : i
      ))

      // Broadcast sucesso
      if (broadcastChannel) {
        console.debug(`[Optimistic] Broadcast success → ${broadcastChannel}`, { id })
      }

      onSuccess?.(result)
      scheduleCleanup(id, successTimeout)
      return result
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err))

      setItems(prev => prev.map(i =>
        i.id === id
          ? { ...i, status: 'error' as const, error: error.message }
          : i
      ))

      // Broadcast erro
      if (broadcastChannel) {
        console.debug(`[Optimistic] Broadcast error → ${broadcastChannel}`, { id, error: error.message })
      }

      onError?.(error, data)
      return undefined
    }
  }, [action, maxRetries, broadcastChannel, onSuccess, onError, successTimeout, scheduleCleanup])

  // Retry de uma ação com erro
  const retry = useCallback(async (id: string): Promise<void> => {
    const item = items.find(i => i.id === id)
    if (!item || item.status !== 'error') return
    if (item.retryCount >= item.maxRetries) return

    // Atualizar para pending com retry count incrementado
    setItems(prev => prev.map(i =>
      i.id === id
        ? { ...i, status: 'pending' as const, retryCount: i.retryCount + 1, error: undefined }
        : i
    ))

    try {
      const result = await action(item.data)

      setItems(prev => prev.map(i =>
        i.id === id ? { ...i, status: 'success' as const } : i
      ))

      onSuccess?.(result)
      scheduleCleanup(id, successTimeout)
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err))

      setItems(prev => prev.map(i =>
        i.id === id
          ? { ...i, status: 'error' as const, error: error.message }
          : i
      ))

      onError?.(error, item.data)
    }
  }, [items, action, onSuccess, onError, successTimeout, scheduleCleanup])

  // Rollback — remove o item e notifica
  const rollback = useCallback((id: string): void => {
    const item = items.find(i => i.id === id)
    if (!item) return

    // Limpar timeout se existir
    const timeout = timeoutRefs.current.get(id)
    if (timeout) {
      clearTimeout(timeout)
      timeoutRefs.current.delete(id)
    }

    setItems(prev => prev.filter(i => i.id !== id))
    onRollback?.(item.data)

    // Broadcast rollback
    if (broadcastChannel) {
      console.debug(`[Optimistic] Broadcast rollback → ${broadcastChannel}`, { id })
    }
  }, [items, broadcastChannel, onRollback])

  // Dismiss — remove item do estado (sem callback de rollback)
  const dismiss = useCallback((id: string): void => {
    setItems(prev => prev.filter(i => i.id !== id))
    const timeout = timeoutRefs.current.get(id)
    if (timeout) {
      clearTimeout(timeout)
      timeoutRefs.current.delete(id)
    }
  }, [])

  // Estado derivado
  const pendingCount = items.filter(i => i.status === 'pending').length
  const errorCount = items.filter(i => i.status === 'error').length
  const hasPending = pendingCount > 0
  const hasErrors = errorCount > 0

  return {
    /** Lista de ações optimistic em andamento */
    items,
    /** Executar uma nova ação */
    execute,
    /** Retentar uma ação com erro */
    retry,
    /** Rollback — remove item e chama onRollback */
    rollback,
    /** Dismiss — remove item sem callback */
    dismiss,
    /** Número de ações pendentes */
    pendingCount,
    /** Número de ações com erro */
    errorCount,
    /** Tem ações pendentes? */
    hasPending,
    /** Tem erros? */
    hasErrors,
  }
}
