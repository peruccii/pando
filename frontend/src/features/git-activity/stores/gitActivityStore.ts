import { create } from 'zustand'
import * as AppAPI from '../../../../wailsjs/go/main/App'
import type { gitactivity } from '../../../../wailsjs/go/models'

export type GitActivityEvent = gitactivity.Event

interface GitActivityState {
  events: GitActivityEvent[]
  lastEvent: GitActivityEvent | null
  unreadCount: number
  isOpen: boolean
  expandedEventIDs: Record<string, boolean>
  isLoading: boolean
  error: string | null

  loadEvents: (opts?: { limit?: number; type?: string; repoPath?: string; markUnread?: boolean }) => Promise<void>
  refresh: () => Promise<void>
  clear: () => Promise<void>
  toggleOpen: () => void
  open: () => void
  close: () => void
  toggleExpanded: (eventID: string) => void
  markAllRead: () => void
}

function countNewEvents(next: GitActivityEvent[], previousFirstID: string): number {
  let count = 0
  for (const event of next) {
    if (event.id === previousFirstID) {
      break
    }
    count++
  }
  return count
}

export const useGitActivityStore = create<GitActivityState>((set, get) => ({
  events: [],
  lastEvent: null,
  unreadCount: 0,
  isOpen: false,
  expandedEventIDs: {},
  isLoading: false,
  error: null,

  loadEvents: async (opts) => {
    const limit = opts?.limit ?? 80
    const type = opts?.type ?? ''
    const repoPath = opts?.repoPath ?? ''
    const markUnread = opts?.markUnread ?? false

    set({ isLoading: true, error: null })
    try {
      const nextEvents = await AppAPI.GitActivityList(limit, type, repoPath)
      const prevFirstID = get().events[0]?.id || ''
      const nextFirstID = nextEvents[0]?.id || ''
      let unread = get().unreadCount

      if (markUnread && !get().isOpen && nextFirstID && nextFirstID !== prevFirstID) {
        if (!prevFirstID) {
          unread = Math.min(99, unread + 1)
        } else {
          unread = Math.min(99, unread + countNewEvents(nextEvents, prevFirstID))
        }
      }
      if (get().isOpen) {
        unread = 0
      }

      set({
        events: nextEvents || [],
        lastEvent: (nextEvents && nextEvents.length > 0) ? nextEvents[0] : null,
        unreadCount: unread,
        isLoading: false,
      })
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      set({ isLoading: false, error: message })
    }
  },

  refresh: async () => {
    await get().loadEvents({ markUnread: false })
  },

  clear: async () => {
    try {
      await AppAPI.GitActivityClear()
      set({
        events: [],
        lastEvent: null,
        unreadCount: 0,
        expandedEventIDs: {},
        error: null,
      })
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      set({ error: message })
    }
  },

  toggleOpen: () => {
    const currentlyOpen = get().isOpen
    set({
      isOpen: !currentlyOpen,
      unreadCount: currentlyOpen ? get().unreadCount : 0,
    })
  },

  open: () => set({ isOpen: true, unreadCount: 0 }),

  close: () => set({ isOpen: false }),

  toggleExpanded: (eventID) => {
    if (!eventID) return
    const expanded = get().expandedEventIDs[eventID]
    set({
      expandedEventIDs: {
        ...get().expandedEventIDs,
        [eventID]: !expanded,
      },
    })
  },

  markAllRead: () => set({ unreadCount: 0 }),
}))
