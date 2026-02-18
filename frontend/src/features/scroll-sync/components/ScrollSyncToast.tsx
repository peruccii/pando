import React, { useState, useEffect, useCallback } from 'react'

interface ScrollSyncToast {
  id: string
  userName: string
  file: string
  line: number
  autoFollow: boolean
}

interface ScrollSyncToastContainerProps {
  toasts: ScrollSyncToast[]
  onNavigate: (file: string, line: number) => void
  onDismiss: (id: string) => void
}

const ToastItem: React.FC<{
  toast: ScrollSyncToast
  onNavigate: (file: string, line: number) => void
  onDismiss: (id: string) => void
}> = ({ toast, onNavigate, onDismiss }) => {
  const [isVisible, setIsVisible] = useState(false)

  useEffect(() => {
    // Trigger enter animation
    requestAnimationFrame(() => setIsVisible(true))

    // Auto dismiss after 3 seconds
    const timer = setTimeout(() => {
      setIsVisible(false)
      setTimeout(() => onDismiss(toast.id), 300)
    }, 3000)

    return () => clearTimeout(timer)
  }, [toast.id, onDismiss])

  const handleNavigate = useCallback(() => {
    onNavigate(toast.file, toast.line)
    onDismiss(toast.id)
  }, [toast.file, toast.line, toast.id, onNavigate, onDismiss])

  return (
    <div className={`scroll-sync-toast ${isVisible ? 'scroll-sync-toast--visible' : ''}`}>
      <div className="scroll-sync-toast__content">
        <span className="scroll-sync-toast__user">{toast.userName}</span>
        <span className="scroll-sync-toast__message">
          está em {toast.file}:{toast.line}
        </span>
      </div>
      {!toast.autoFollow && (
        <button
          className="scroll-sync-toast__action"
          onClick={handleNavigate}
        >
          Ir para lá
        </button>
      )}
      <button
        className="scroll-sync-toast__close"
        onClick={() => onDismiss(toast.id)}
        aria-label="Fechar"
      >
        ×
      </button>
    </div>
  )
}

export const ScrollSyncToastContainer: React.FC<ScrollSyncToastContainerProps> = ({
  toasts,
  onNavigate,
  onDismiss,
}) => {
  return (
    <div className="scroll-sync-toast-container">
      {toasts.map((toast) => (
        <ToastItem
          key={toast.id}
          toast={toast}
          onNavigate={onNavigate}
          onDismiss={onDismiss}
        />
      ))}
    </div>
  )
}
