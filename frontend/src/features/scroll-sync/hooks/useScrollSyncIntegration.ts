import { useCallback, useEffect, useState } from 'react'
import { useAppStore } from '../../../stores/appStore'
import { useAuthStore } from '../../../stores/authStore'
import { useSessionStore } from '../../session/stores/sessionStore'
import { useScrollSync, useScrollSyncHandler, ScrollSyncToastContainer } from '../index'
import type { ScrollSyncEvent } from '../types'

interface ScrollSyncIntegrationResult {
  sendScrollSync: (file: string, line: number, action: ScrollSyncEvent['action']) => void
  ToastContainer: typeof ScrollSyncToastContainer
  toasts: Array<{
    id: string
    userName: string
    file: string
    line: number
    autoFollow: boolean
  }>
  onNavigateToast: (file: string, line: number) => void
  dismissToast: (id: string) => void
}

/**
 * Hook que integra Scroll Sync com P2P e stores globais
 */
export function useScrollSyncIntegration(
  onNavigateToFile: (file: string) => void,
  onExpandFile: (file: string) => void,
  onScrollToLine: (file: string, line: number) => void,
  onHighlightLine: (file: string, line: number, color: string, duration: number) => void
): ScrollSyncIntegrationResult {
  const { scrollSyncSettings } = useAppStore()
  const { user } = useAuthStore()
  const { isP2PConnected } = useSessionStore()
  
  const [toasts, setToasts] = useState<Array<{
    id: string
    userName: string
    file: string
    line: number
    autoFollow: boolean
  }>>([])

  const currentUserID = user?.id || 'anonymous'
  const currentUserName = user?.name || 'Anonymous'

  // Handler para enviar eventos via P2P
  const handleSend = useCallback((event: ScrollSyncEvent) => {
    if (!isP2PConnected) return
    
    // Enviar via WebRTC pelo canal CONTROL
    // O P2PConnection será acessado pelo componente pai
    window.dispatchEvent(new CustomEvent('scrollsync:outgoing', { detail: event }))
  }, [isP2PConnected])

  // Handler para mostrar toast
  const handleShowToast = useCallback((userName: string, file: string, line: number, autoFollow: boolean) => {
    if (!scrollSyncSettings.showToast) return
    
    const id = `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`
    setToasts(prev => [...prev, { id, userName, file, line, autoFollow }])
  }, [scrollSyncSettings.showToast])

  // Handler para receber eventos
  const handleReceive = useScrollSyncHandler({
    onNavigateToFile,
    onExpandFile,
    onScrollToLine,
    onHighlightLine,
    onShowToast: handleShowToast,
  })

  const { sendScrollSync, handleIncomingEvent } = useScrollSync({
    currentUserID,
    currentUserName,
    settings: scrollSyncSettings,
    onSend: handleSend,
    onReceive: handleReceive,
  })

  // Ouvir eventos P2P recebidos
  useEffect(() => {
    const handleIncoming = (e: Event) => {
      const customEvent = e as CustomEvent<ScrollSyncEvent>
      handleIncomingEvent(customEvent.detail)
    }

    window.addEventListener('scrollsync:incoming', handleIncoming)
    return () => window.removeEventListener('scrollsync:incoming', handleIncoming)
  }, [handleIncomingEvent])

  const onNavigateToast = useCallback((file: string, line: number) => {
    onNavigateToFile(file)
    onExpandFile(file)
    setTimeout(() => {
      onScrollToLine(file, line)
    }, 100)
  }, [onNavigateToFile, onExpandFile, onScrollToLine])

  const dismissToast = useCallback((id: string) => {
    setToasts(prev => prev.filter(t => t.id !== id))
  }, [])

  return {
    sendScrollSync,
    ToastContainer: ScrollSyncToastContainer,
    toasts,
    onNavigateToast,
    dismissToast,
  }
}
