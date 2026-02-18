import React from 'react'
import { RotateCcw, X, Check, Loader2, AlertTriangle } from 'lucide-react'
import type { OptimisticAction, OptimisticStatus } from '../hooks/useOptimisticAction'
import './OptimisticFeedback.css'

interface OptimisticFeedbackProps<T> {
  /** Ação optimistic a exibir */
  item: OptimisticAction<T>
  /** Label do item (ex: "Comment", "Review") */
  label?: string
  /** Callback de retry */
  onRetry?: (id: string) => void
  /** Callback de rollback */
  onRollback?: (id: string) => void
  /** Callback de dismiss */
  onDismiss?: (id: string) => void
  /** Renderizador customizado para o conteúdo */
  renderContent?: (data: T) => React.ReactNode
}

const STATUS_CONFIG: Record<OptimisticStatus, {
  icon: React.ReactNode
  text: string
  className: string
}> = {
  idle: { icon: null, text: '', className: '' },
  pending: {
    icon: <Loader2 size={14} className="optimistic__spinner" />,
    text: 'Enviando...',
    className: 'optimistic--pending',
  },
  success: {
    icon: <Check size={14} />,
    text: 'Enviado ✓',
    className: 'optimistic--success',
  },
  error: {
    icon: <AlertTriangle size={14} />,
    text: 'Falha ao enviar',
    className: 'optimistic--error',
  },
}

/**
 * OptimisticFeedback — Componente visual de status para ações optimistic.
 *
 * Exibe o estado atual (pending/success/error) com ícone, texto,
 * e ações de retry/rollback quando aplicável.
 */
export function OptimisticFeedback<T>({
  item,
  label,
  onRetry,
  onRollback,
  onDismiss,
  renderContent,
}: OptimisticFeedbackProps<T>) {
  const config = STATUS_CONFIG[item.status]
  if (item.status === 'idle') return null

  return (
    <div
      className={`optimistic ${config.className}`}
      data-optimistic-id={item.id}
    >
      {/* Status indicator */}
      <div className="optimistic__status">
        {config.icon}
        <span className="optimistic__text">
          {label && <strong>{label}: </strong>}
          {config.text}
        </span>
      </div>

      {/* Custom content */}
      {renderContent && (
        <div className="optimistic__content">
          {renderContent(item.data)}
        </div>
      )}

      {/* Error details + actions */}
      {item.status === 'error' && (
        <div className="optimistic__error-details">
          <span className="optimistic__error-msg">{item.error}</span>
          <div className="optimistic__actions">
            {onRetry && item.retryCount < item.maxRetries && (
              <button
                className="optimistic__btn optimistic__btn--retry"
                onClick={() => onRetry(item.id)}
                title={`Tentar novamente (${item.retryCount}/${item.maxRetries})`}
              >
                <RotateCcw size={12} />
                Retry ({item.maxRetries - item.retryCount} left)
              </button>
            )}
            {onRollback && (
              <button
                className="optimistic__btn optimistic__btn--rollback"
                onClick={() => onRollback(item.id)}
                title="Desfazer ação"
              >
                <X size={12} />
                Undo
              </button>
            )}
          </div>
        </div>
      )}

      {/* Dismiss for success */}
      {item.status === 'success' && onDismiss && (
        <button
          className="optimistic__dismiss"
          onClick={() => onDismiss(item.id)}
          aria-label="Dismiss"
        >
          <X size={12} />
        </button>
      )}
    </div>
  )
}

/**
 * OptimisticList — Renderiza uma lista de ações optimistic.
 */
interface OptimisticListProps<T> {
  items: OptimisticAction<T>[]
  label?: string
  onRetry?: (id: string) => void
  onRollback?: (id: string) => void
  onDismiss?: (id: string) => void
  renderContent?: (data: T) => React.ReactNode
}

export function OptimisticList<T>({
  items,
  label,
  onRetry,
  onRollback,
  onDismiss,
  renderContent,
}: OptimisticListProps<T>) {
  const activeItems = items.filter(i => i.status !== 'idle')
  if (activeItems.length === 0) return null

  return (
    <div className="optimistic-list">
      {activeItems.map(item => (
        <OptimisticFeedback
          key={item.id}
          item={item}
          label={label}
          onRetry={onRetry}
          onRollback={onRollback}
          onDismiss={onDismiss}
          renderContent={renderContent}
        />
      ))}
    </div>
  )
}
