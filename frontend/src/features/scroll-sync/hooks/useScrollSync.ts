import { useCallback, useRef, useEffect } from 'react'
import type { ScrollSyncEvent, ScrollSyncSettings, ScrollSyncDebounceState } from '../types'
import { getUserColor, DEFAULT_SCROLL_SYNC_SETTINGS } from '../types'

// Rate limiting constants
const DEBOUNCE_MS = 2000 // 2 segundos por usuário
const MAX_EVENTS_PER_MINUTE = 10
const RATE_LIMIT_WINDOW_MS = 60000 // 1 minuto

interface ScrollSyncOptions {
  currentUserID: string
  currentUserName: string
  settings: ScrollSyncSettings
  onSend: (event: ScrollSyncEvent) => void
  onReceive: (event: ScrollSyncEvent, autoFollow: boolean) => void
}

interface ScrollSyncHook {
  sendScrollSync: (file: string, line: number, action: ScrollSyncEvent['action']) => void
  handleIncomingEvent: (event: ScrollSyncEvent) => void
  isEnabled: boolean
}

/**
 * Hook para gerenciar Scroll Sync com anti-spam
 */
export function useScrollSync({
  currentUserID,
  currentUserName,
  settings = DEFAULT_SCROLL_SYNC_SETTINGS,
  onSend,
  onReceive,
}: ScrollSyncOptions): ScrollSyncHook {
  // Refs para debounce e rate limiting
  const debounceState = useRef<ScrollSyncDebounceState>({})
  const eventCount = useRef<{ count: number; windowStart: number }>({
    count: 0,
    windowStart: Date.now(),
  })

  // Reset rate limit window periodicamente
  useEffect(() => {
    const interval = setInterval(() => {
      const now = Date.now()
      if (now - eventCount.current.windowStart > RATE_LIMIT_WINDOW_MS) {
        eventCount.current = { count: 0, windowStart: now }
      }
    }, RATE_LIMIT_WINDOW_MS)

    return () => clearInterval(interval)
  }, [])

  const sendScrollSync = useCallback((
    file: string,
    line: number,
    action: ScrollSyncEvent['action']
  ) => {
    if (!settings.enabled) return

    // Check rate limit
    const now = Date.now()
    if (now - eventCount.current.windowStart > RATE_LIMIT_WINDOW_MS) {
      eventCount.current = { count: 0, windowStart: now }
    }

    if (eventCount.current.count >= MAX_EVENTS_PER_MINUTE) {
      console.warn('[ScrollSync] Rate limit exceeded, skipping event')
      return
    }

    // Check debounce for current user
    const lastSent = debounceState.current[currentUserID]
    if (lastSent && now - lastSent < DEBOUNCE_MS) {
      return // Skip - debounce
    }

    // Update state
    debounceState.current[currentUserID] = now
    eventCount.current.count++

    // Create and send event
    const event: ScrollSyncEvent = {
      type: 'scroll_sync',
      file,
      line,
      userID: currentUserID,
      userName: currentUserName,
      userColor: getUserColor(currentUserID),
      action,
      timestamp: now,
    }

    onSend(event)
  }, [currentUserID, currentUserName, settings.enabled, onSend])

  const handleIncomingEvent = useCallback((event: ScrollSyncEvent) => {
    // Ignore self
    if (event.userID === currentUserID) return

    // Check if enabled
    if (!settings.enabled) return

    // Pass to handler with autoFollow setting
    onReceive(event, settings.autoFollow)
  }, [currentUserID, settings.enabled, onReceive])

  return {
    sendScrollSync,
    handleIncomingEvent,
    isEnabled: settings.enabled,
  }
}
