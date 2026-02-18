import { useEffect, useRef } from 'react'
import { useGitActivityStore } from '../stores/gitActivityStore'

/**
 * Hook global para sincronizar o painel de atividade Git com eventos Wails.
 * Deve ser montado no App raiz.
 */
export function useGitActivity() {
  const loadEvents = useGitActivityStore((s) => s.loadEvents)
  const open = useGitActivityStore((s) => s.open)
  const close = useGitActivityStore((s) => s.close)
  const toggleOpen = useGitActivityStore((s) => s.toggleOpen)

  const refreshTimerRef = useRef<number | null>(null)

  useEffect(() => {
    loadEvents({ markUnread: false }).catch((err) => {
      console.error('[GitActivity] initial load failed:', err)
    })
  }, [loadEvents])

  useEffect(() => {
    const scheduleRefresh = () => {
      if (refreshTimerRef.current !== null) {
        window.clearTimeout(refreshTimerRef.current)
      }
      refreshTimerRef.current = window.setTimeout(() => {
        refreshTimerRef.current = null
        useGitActivityStore.getState().loadEvents({ markUnread: true }).catch((err) => {
          console.error('[GitActivity] refresh failed:', err)
        })
      }, 260)
    }

    const onToggle = () => toggleOpen()
    const onOpen = () => open()
    const onClose = () => close()
    window.addEventListener('git-activity:toggle', onToggle)
    window.addEventListener('git-activity:open', onOpen)
    window.addEventListener('git-activity:close', onClose)

    if (!window.runtime) {
      return () => {
        window.removeEventListener('git-activity:toggle', onToggle)
        window.removeEventListener('git-activity:open', onOpen)
        window.removeEventListener('git-activity:close', onClose)
      }
    }

    const offBranch = window.runtime.EventsOn('git:branch_changed', scheduleRefresh)
    const offCommit = window.runtime.EventsOn('git:commit', scheduleRefresh)
    const offCommitPreparing = window.runtime.EventsOn('git:commit_preparing', scheduleRefresh)
    const offIndex = window.runtime.EventsOn('git:index', scheduleRefresh)
    const offFetch = window.runtime.EventsOn('git:fetch', scheduleRefresh)
    const offMerge = window.runtime.EventsOn('git:merge', scheduleRefresh)

    return () => {
      if (refreshTimerRef.current !== null) {
        window.clearTimeout(refreshTimerRef.current)
        refreshTimerRef.current = null
      }
      offBranch()
      offCommit()
      offCommitPreparing()
      offIndex()
      offFetch()
      offMerge()
      window.removeEventListener('git-activity:toggle', onToggle)
      window.removeEventListener('git-activity:open', onOpen)
      window.removeEventListener('git-activity:close', onClose)
    }
  }, [toggleOpen, open, close])
}

