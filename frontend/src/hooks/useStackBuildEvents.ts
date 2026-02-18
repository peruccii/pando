import { useEffect, useRef } from 'react'
import { useStackBuildStore } from '../stores/stackBuildStore'
import * as WailsApp from '../../wailsjs/go/main/App'

/**
 * Hook global que escuta eventos de build do Docker (docker:build:*)
 * e mantém o stackBuildStore atualizado.
 * 
 * DEVE ser montado no App.tsx (nível raiz) para garantir que os eventos
 * sejam capturados mesmo quando o StackBuilder não está montado.
 */
export function useStackBuildEvents() {
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const store = useStackBuildStore

  // Hydrate from backend on mount (reconectar ao estado do build em andamento)
  useEffect(() => {
    const hydrateFromBackend = async () => {
      try {
        const state = await WailsApp.GetStackBuildState()
        if (state && (state.isBuilding || (state.logs && state.logs.length > 0))) {
          store.getState().hydrate(state)
        }
      } catch (err) {
        console.warn('[StackBuild] Failed to hydrate build state:', err)
      }
    }
    hydrateFromBackend()
  }, [])

  // Timer para elapsed time
  useEffect(() => {
    const unsubscribe = store.subscribe((state, prevState) => {
      // Start timer quando isBuilding muda para true
      if (state.isBuilding && !prevState.isBuilding) {
        if (timerRef.current) clearInterval(timerRef.current)
        timerRef.current = setInterval(() => {
          const { startTime, isBuilding } = store.getState()
          if (!isBuilding) {
            if (timerRef.current) clearInterval(timerRef.current)
            timerRef.current = null
            return
          }
          const now = Math.floor(Date.now() / 1000)
          const elapsed = startTime > 0 ? now - startTime : 0
          store.getState().setElapsedTime(elapsed)
        }, 1000)
      }

      // Stop timer quando isBuilding muda para false
      if (!state.isBuilding && prevState.isBuilding) {
        if (timerRef.current) {
          clearInterval(timerRef.current)
          timerRef.current = null
        }
      }
    })

    // Se já estava building ao montar (hydrate), iniciar timer imediatamente
    if (store.getState().isBuilding) {
      timerRef.current = setInterval(() => {
        const { startTime, isBuilding } = store.getState()
        if (!isBuilding) {
          if (timerRef.current) clearInterval(timerRef.current)
          timerRef.current = null
          return
        }
        const now = Math.floor(Date.now() / 1000)
        const elapsed = startTime > 0 ? now - startTime : 0
        store.getState().setElapsedTime(elapsed)
      }, 1000)
    }

    return () => {
      unsubscribe()
      if (timerRef.current) {
        clearInterval(timerRef.current)
        timerRef.current = null
      }
    }
  }, [])

  // Registrar listeners de eventos Wails (globais, vivem enquanto o app existir)
  useEffect(() => {
    if (!window.runtime) return

    const handleLog = (line: string) => {
      store.getState().addLog(line)
    }

    const handleSuccess = (msg: string) => {
      store.getState().completeBuild('success', `✅ ${msg}`)
    }

    const handleError = (err: string) => {
      store.getState().completeBuild('error', `❌ Error: ${err}`)
    }

    const handleStarted = (state: { isBuilding: boolean; logs: string[]; startTime: number; result: string }) => {
      store.getState().hydrate(state)
    }

    window.runtime.EventsOn('docker:build:log', handleLog)
    window.runtime.EventsOn('docker:build:success', handleSuccess)
    window.runtime.EventsOn('docker:build:error', handleError)
    window.runtime.EventsOn('docker:build:started', handleStarted)

    return () => {
      window.runtime.EventsOff('docker:build:log')
      window.runtime.EventsOff('docker:build:success')
      window.runtime.EventsOff('docker:build:error')
      window.runtime.EventsOff('docker:build:started')
    }
  }, [])
}

