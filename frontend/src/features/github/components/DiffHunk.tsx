import React from 'react'
import type { DiffHunk, DiffViewMode } from '../types/github'
import { DiffLineComponent } from './DiffLine'

interface Props {
  hunk: DiffHunk
  viewMode: DiffViewMode
  highlightedLines?: Array<{ line: number; color: string }>
  onLineClick?: (lineNumber: number) => void
}

export const DiffHunkComponent: React.FC<Props> = ({ hunk, viewMode, highlightedLines = [], onLineClick }) => {
  return (
    <div className="gh-diff-hunk">
      {/* Hunk header */}
      <div className="gh-diff-hunk__header">
        <span className="gh-diff-hunk__range">
          @@ -{hunk.oldStart},{hunk.oldLines} +{hunk.newStart},{hunk.newLines} @@
        </span>
        {hunk.header && (
          <span className="gh-diff-hunk__context">{hunk.header}</span>
        )}
      </div>

      {/* Lines */}
      {viewMode === 'unified' ? (
        <div className="gh-diff-hunk__lines gh-diff-hunk__lines--unified">
          {hunk.lines.map((line, i) => (
            <DiffLineComponent 
              key={i} 
              line={line} 
              viewMode="unified"
              highlight={highlightedLines.find(h => h.line === (line.newLine || line.oldLine))}
              onClick={() => onLineClick && onLineClick(line.newLine || line.oldLine || 0)}
            />
          ))}
        </div>
      ) : (
        <div className="gh-diff-hunk__lines gh-diff-hunk__lines--split">
          <SplitView 
            hunk={hunk} 
            highlightedLines={highlightedLines}
            onLineClick={onLineClick}
          />
        </div>
      )}
    </div>
  )
}

// Side-by-side rendering
interface SplitViewProps {
  hunk: DiffHunk
  highlightedLines?: Array<{ line: number; color: string }>
  onLineClick?: (lineNumber: number) => void
}

const SplitView: React.FC<SplitViewProps> = ({ hunk, highlightedLines = [], onLineClick }) => {
  // Build paired lines for side-by-side
  const pairs: Array<{ left: typeof hunk.lines[0] | null; right: typeof hunk.lines[0] | null }> = []

  const deletes: typeof hunk.lines = []
  const adds: typeof hunk.lines = []

  for (const line of hunk.lines) {
    if (line.type === 'context') {
      // Flush pending deletes/adds
      flushPairs(deletes, adds, pairs)
      pairs.push({ left: line, right: line })
    } else if (line.type === 'delete') {
      deletes.push(line)
    } else if (line.type === 'add') {
      adds.push(line)
    }
  }
  flushPairs(deletes, adds, pairs)

  return (
    <table className="gh-diff-split-table">
      <tbody>
        {pairs.map(({ left, right }, i) => {
          const leftHighlight = left?.oldLine ? highlightedLines.find(h => h.line === left.oldLine) : undefined
          const rightHighlight = right?.newLine ? highlightedLines.find(h => h.line === right.newLine) : undefined
          
          return (
            <tr 
              key={i} 
              className="gh-diff-split-row"
              data-line-left={left?.oldLine}
              data-line-right={right?.newLine}
            >
              <td 
                className={`gh-diff-split__gutter gh-diff-split__gutter--old ${leftHighlight ? 'gh-diff-line--highlight' : ''}`}
                style={leftHighlight ? { '--highlight-color': leftHighlight.color } as React.CSSProperties : undefined}
                onClick={() => left?.oldLine && onLineClick?.(left.oldLine)}
              >
                {left?.oldLine ?? ''}
              </td>
              <td 
                className={`gh-diff-split__content gh-diff-split__content--old ${left?.type === 'delete' ? 'gh-diff-line--delete' : ''} ${leftHighlight ? 'gh-diff-line--highlight' : ''}`}
                style={leftHighlight ? { '--highlight-color': leftHighlight.color } as React.CSSProperties : undefined}
                onClick={() => left?.oldLine && onLineClick?.(left.oldLine)}
              >
                {left ? left.content : ''}
              </td>
              <td 
                className={`gh-diff-split__gutter gh-diff-split__gutter--new ${rightHighlight ? 'gh-diff-line--highlight' : ''}`}
                style={rightHighlight ? { '--highlight-color': rightHighlight.color } as React.CSSProperties : undefined}
                onClick={() => right?.newLine && onLineClick?.(right.newLine)}
              >
                {right?.newLine ?? ''}
              </td>
              <td 
                className={`gh-diff-split__content gh-diff-split__content--new ${right?.type === 'add' ? 'gh-diff-line--add' : ''} ${rightHighlight ? 'gh-diff-line--highlight' : ''}`}
                style={rightHighlight ? { '--highlight-color': rightHighlight.color } as React.CSSProperties : undefined}
                onClick={() => right?.newLine && onLineClick?.(right.newLine)}
              >
                {right ? right.content : ''}
              </td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}

function flushPairs(
  deletes: Array<{ type: string; content: string; oldLine?: number; newLine?: number }>,
  adds: Array<{ type: string; content: string; oldLine?: number; newLine?: number }>,
  pairs: Array<{ left: typeof deletes[0] | null; right: typeof adds[0] | null }>
) {
  const maxLen = Math.max(deletes.length, adds.length)
  for (let i = 0; i < maxLen; i++) {
    pairs.push({
      left: i < deletes.length ? deletes[i] : null,
      right: i < adds.length ? adds[i] : null,
    })
  }
  deletes.length = 0
  adds.length = 0
}
