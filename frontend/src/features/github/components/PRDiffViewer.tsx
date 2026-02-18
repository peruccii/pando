import React, { useRef, useCallback, useState, useEffect } from 'react'
import { useDiff } from '../hooks/useDiff'
import { DiffFileComponent } from './DiffFile'
import { useScrollSyncIntegration } from '../../scroll-sync/hooks/useScrollSyncIntegration'
import './github.css'

interface FileState {
  isCollapsed: boolean
  highlightedLines: Array<{
    line: number
    color: string
    timeoutId: number
  }>
}

export const PRDiffViewer: React.FC = () => {
  const { files, totalFiles, hasMoreFiles, viewMode, isLoading, loadMoreFiles, toggleViewMode } = useDiff()
  const containerRef = useRef<HTMLDivElement>(null)
  const fileRefs = useRef<Map<string, HTMLDivElement>>(new Map())
  
  // Estado de colapso e highlight por arquivo
  const [fileStates, setFileStates] = useState<Map<string, FileState>>(new Map())

  // Handler para navegar para um arquivo específico
  const navigateToFile = useCallback((filename: string) => {
    const fileElement = fileRefs.current.get(filename)
    if (fileElement && containerRef.current) {
      // Scroll suave para o arquivo
      fileElement.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, [])

  // Handler para expandir um arquivo
  const expandFile = useCallback((filename: string) => {
    setFileStates(prev => {
      const newMap = new Map(prev)
      const state = newMap.get(filename) || { isCollapsed: false, highlightedLines: [] }
      newMap.set(filename, { ...state, isCollapsed: false })
      return newMap
    })
  }, [])

  // Handler para scrollar até uma linha específica
  const scrollToLine = useCallback((filename: string, lineNumber: number) => {
    const fileElement = fileRefs.current.get(filename)
    if (!fileElement) return

    // Procurar a linha no arquivo
    const lineElement = fileElement.querySelector(`[data-line="${lineNumber}"]`)
    if (lineElement) {
      lineElement.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }
  }, [])

  // Handler para destacar uma linha
  const highlightLine = useCallback((filename: string, lineNumber: number, color: string, duration: number) => {
    setFileStates(prev => {
      const newMap = new Map(prev)
      const state = newMap.get(filename) || { isCollapsed: false, highlightedLines: [] }
      
      // Remover highlight anterior da mesma linha
      const filtered = state.highlightedLines.filter(h => h.line !== lineNumber)
      
      // Adicionar novo highlight
      const timeoutId = window.setTimeout(() => {
        setFileStates(current => {
          const updatedMap = new Map(current)
          const currentState = updatedMap.get(filename)
          if (currentState) {
            updatedMap.set(filename, {
              ...currentState,
              highlightedLines: currentState.highlightedLines.filter(h => h.line !== lineNumber)
            })
          }
          return updatedMap
        })
      }, duration)
      
      newMap.set(filename, {
        ...state,
        highlightedLines: [...filtered, { line: lineNumber, color, timeoutId }]
      })
      return newMap
    })
  }, [])

  // Cleanup de timeouts ao desmontar
  useEffect(() => {
    return () => {
      fileStates.forEach(state => {
        state.highlightedLines.forEach(h => clearTimeout(h.timeoutId))
      })
    }
  }, [fileStates])

  // Integração com Scroll Sync
  const { 
    sendScrollSync, 
    ToastContainer, 
    toasts, 
    onNavigateToast, 
    dismissToast 
  } = useScrollSyncIntegration(
    navigateToFile,
    expandFile,
    scrollToLine,
    highlightLine
  )

  // Handler para registrar ref de arquivo
  const registerFileRef = useCallback((filename: string, element: HTMLDivElement | null) => {
    if (element) {
      fileRefs.current.set(filename, element)
    }
  }, [])

  // Toggle de colapso de arquivo
  const toggleFileCollapse = useCallback((filename: string) => {
    setFileStates(prev => {
      const newMap = new Map(prev)
      const state = newMap.get(filename) || { isCollapsed: false, highlightedLines: [] }
      newMap.set(filename, { ...state, isCollapsed: !state.isCollapsed })
      return newMap
    })
  }, [])

  if (files.length === 0 && !isLoading) {
    return null
  }

  return (
    <div className="gh-diff-viewer" ref={containerRef}>
      <div className="gh-diff-viewer__header">
        <h3 className="gh-diff-viewer__title">
          Files Changed
          <span className="gh-diff-viewer__count">
            {files.length} / {totalFiles}
          </span>
        </h3>

        <div className="gh-diff-viewer__controls">
          <div className="gh-view-toggle">
            <button
              className={`gh-view-toggle__btn ${viewMode === 'unified' ? 'gh-view-toggle__btn--active' : ''}`}
              onClick={() => { if (viewMode !== 'unified') toggleViewMode() }}
            >
              Unified
            </button>
            <button
              className={`gh-view-toggle__btn ${viewMode === 'side-by-side' ? 'gh-view-toggle__btn--active' : ''}`}
              onClick={() => { if (viewMode !== 'side-by-side') toggleViewMode() }}
            >
              Split
            </button>
          </div>
        </div>
      </div>

      <div className="gh-diff-viewer__files">
        {files.map((file, index) => (
          <DiffFileComponent 
            key={`${file.filename}-${index}`} 
            file={file} 
            viewMode={viewMode}
            ref={el => registerFileRef(file.filename, el)}
            isCollapsed={fileStates.get(file.filename)?.isCollapsed ?? false}
            highlightedLines={fileStates.get(file.filename)?.highlightedLines ?? []}
            onToggleCollapse={() => toggleFileCollapse(file.filename)}
            onLineClick={(lineNumber) => sendScrollSync(file.filename, lineNumber, 'comment')}
          />
        ))}
      </div>

      {hasMoreFiles && (
        <button className="gh-diff-viewer__load-more" onClick={loadMoreFiles} disabled={isLoading}>
          {isLoading ? (
            <>
              <div className="gh-spinner gh-spinner--small" />
              Loading...
            </>
          ) : (
            `Load more files (${files.length} / ${totalFiles})`
          )}
        </button>
      )}

      {/* Toast Container para notificações de Scroll Sync */}
      <ToastContainer 
        toasts={toasts} 
        onNavigate={onNavigateToast} 
        onDismiss={dismissToast} 
      />
    </div>
  )
}
