import { useCallback } from 'react'
import type { ScrollSyncEvent } from '../types'

interface ScrollSyncHandlerOptions {
  onNavigateToFile: (file: string) => void
  onExpandFile: (file: string) => void
  onScrollToLine: (file: string, line: number) => void
  onHighlightLine: (file: string, line: number, color: string, duration: number) => void
  onShowToast: (userName: string, file: string, line: number, autoFollow: boolean) => void
}

/**
 * Hook para manipular eventos de Scroll Sync recebidos
 */
export function useScrollSyncHandler({
  onNavigateToFile,
  onExpandFile,
  onScrollToLine,
  onHighlightLine,
  onShowToast,
}: ScrollSyncHandlerOptions) {
  return useCallback((event: ScrollSyncEvent, autoFollow: boolean) => {
    // 1. Mostrar toast (sempre, independente de autoFollow)
    onShowToast(event.userName, event.file, event.line, autoFollow)

    if (autoFollow) {
      // 2. Navegar para o arquivo
      onNavigateToFile(event.file)

      // 3. Expandir arquivo (se estiver colapsado)
      onExpandFile(event.file)

      // 4. Scroll para a linha (com delay para permitir renderização)
      setTimeout(() => {
        onScrollToLine(event.file, event.line)

        // 5. Highlight temporário (2 segundos)
        onHighlightLine(event.file, event.line, event.userColor, 2000)
      }, 100)
    }
  }, [onNavigateToFile, onExpandFile, onScrollToLine, onHighlightLine, onShowToast])
}
