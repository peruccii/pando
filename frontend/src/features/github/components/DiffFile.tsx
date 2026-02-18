import { forwardRef, useEffect, useMemo, useState } from 'react'
import type { DiffFile, DiffViewMode } from '../types/github'
import { DiffHunkComponent } from './DiffHunk'

interface Props {
  file: DiffFile
  viewMode: DiffViewMode
  isCollapsed?: boolean
  highlightedLines?: Array<{ line: number; color: string }>
  onToggleCollapse?: () => void
  onLineClick?: (lineNumber: number) => void
}

const STATUS_ICONS: Record<string, { icon: string; color: string }> = {
  added: { icon: 'A', color: 'var(--gh-add)' },
  modified: { icon: 'M', color: 'var(--gh-modified)' },
  deleted: { icon: 'D', color: 'var(--gh-del)' },
  renamed: { icon: 'R', color: 'var(--gh-renamed)' },
}

const HUNK_BATCH_SIZE = 8

export const DiffFileComponent = forwardRef<HTMLDivElement, Props>(
  ({ file, viewMode, isCollapsed = false, highlightedLines = [], onToggleCollapse, onLineClick }, ref) => {
    const status = STATUS_ICONS[file.status] ?? STATUS_ICONS.modified
    const [visibleHunks, setVisibleHunks] = useState(HUNK_BATCH_SIZE)

    useEffect(() => {
      setVisibleHunks(HUNK_BATCH_SIZE)
    }, [file.filename, isCollapsed])

    const hunksToRender = useMemo(
      () => file.hunks.slice(0, visibleHunks),
      [file.hunks, visibleHunks]
    )
    const hasMoreHunks = visibleHunks < file.hunks.length

    return (
      <div 
        ref={ref}
        className={`gh-diff-file ${isCollapsed ? 'gh-diff-file--collapsed' : ''}`}
        data-filename={file.filename}
      >
        <div className="gh-diff-file__header" onClick={onToggleCollapse}>
          <button className="gh-diff-file__toggle">
            <svg
              width="12" height="12" viewBox="0 0 16 16" fill="currentColor"
              style={{ transform: isCollapsed ? 'rotate(-90deg)' : 'rotate(0)', transition: 'transform 150ms' }}
            >
              <path d="M12.78 6.22a.75.75 0 010 1.06l-4.25 4.25a.75.75 0 01-1.06 0L3.22 7.28a.75.75 0 011.06-1.06L8 9.94l3.72-3.72a.75.75 0 011.06 0z" />
            </svg>
          </button>

          <span className="gh-diff-file__status" style={{ color: status.color }}>
            {status.icon}
          </span>

          <span className="gh-diff-file__name">{file.filename}</span>

          <div className="gh-diff-file__stats">
            {file.additions > 0 && <span className="gh-stat gh-stat--add">+{file.additions}</span>}
            {file.deletions > 0 && <span className="gh-stat gh-stat--del">-{file.deletions}</span>}
          </div>
        </div>

        {!isCollapsed && (
          <div className="gh-diff-file__content">
            {file.hunks.length > 0 ? (
              <>
                {hunksToRender.map((hunk, i) => (
                <DiffHunkComponent 
                  key={`${file.filename}-${i}`} 
                  hunk={hunk} 
                  viewMode={viewMode}
                  highlightedLines={highlightedLines}
                  onLineClick={onLineClick}
                />
                ))}
                {hasMoreHunks && (
                  <button
                    className="gh-diff-viewer__load-more"
                    onClick={(event) => {
                      event.stopPropagation()
                      setVisibleHunks((prev) => Math.min(prev + HUNK_BATCH_SIZE, file.hunks.length))
                    }}
                  >
                    Load more hunks ({hunksToRender.length}/{file.hunks.length})
                  </button>
                )}
              </>
            ) : (
              <div className="gh-diff-file__binary">Binary file or no changes to display</div>
            )}
          </div>
        )}
      </div>
    )
  }
)

DiffFileComponent.displayName = 'DiffFileComponent'
