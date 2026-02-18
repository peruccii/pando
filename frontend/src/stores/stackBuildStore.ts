import { create } from 'zustand'

export interface StackBuildState {
  isBuilding: boolean
  logs: string[]
  startTime: number // unix seconds (0 = not building)
  result: '' | 'success' | 'error'
  elapsedTime: number // seconds since build started (client-side timer)
}

interface StackBuildActions {
  setBuilding: (isBuilding: boolean) => void
  setStartTime: (time: number) => void
  setResult: (result: '' | 'success' | 'error') => void
  addLog: (line: string) => void
  setLogs: (logs: string[]) => void
  setElapsedTime: (seconds: number) => void
  /** Hydrate state from backend (used when the component re-mounts) */
  hydrate: (state: { isBuilding: boolean; logs: string[]; startTime: number; result: string }) => void
  /** Full reset for starting a new build */
  startBuild: () => void
  /** Mark build as complete */
  completeBuild: (result: 'success' | 'error', logLine: string) => void
  reset: () => void
}

const INITIAL_STATE: StackBuildState = {
  isBuilding: false,
  logs: [],
  startTime: 0,
  result: '',
  elapsedTime: 0,
}

export const useStackBuildStore = create<StackBuildState & StackBuildActions>((set) => ({
  ...INITIAL_STATE,

  setBuilding: (isBuilding) => set({ isBuilding }),
  setStartTime: (startTime) => set({ startTime }),
  setResult: (result) => set({ result }),
  addLog: (line) => set((s) => ({ logs: [...s.logs, line] })),
  setLogs: (logs) => set({ logs }),
  setElapsedTime: (elapsedTime) => set({ elapsedTime }),

  hydrate: (state) => {
    const now = Math.floor(Date.now() / 1000)
    const elapsed = state.isBuilding && state.startTime > 0 ? now - state.startTime : 0
    set({
      isBuilding: state.isBuilding,
      logs: state.logs || [],
      startTime: state.startTime,
      result: (state.result as '' | 'success' | 'error') || '',
      elapsedTime: elapsed,
    })
  },

  startBuild: () => set({
    isBuilding: true,
    logs: ['🚀 Iniciando build do ambiente...'],
    startTime: Math.floor(Date.now() / 1000),
    result: '',
    elapsedTime: 0,
  }),

  completeBuild: (result, logLine) => set((s) => ({
    isBuilding: false,
    result,
    logs: [...s.logs, logLine],
  })),

  reset: () => set(INITIAL_STATE),
}))
