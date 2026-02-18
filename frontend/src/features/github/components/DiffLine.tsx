import React from 'react'
import type { DiffLine, DiffViewMode } from '../types/github'

interface Props {
  line: DiffLine
  viewMode: DiffViewMode
  highlight?: { line: number; color: string }
  onClick?: () => void
}

export const DiffLineComponent: React.FC<Props> = ({ line, viewMode, highlight, onClick }) => {
  const lineClass = `gh-diff-line gh-diff-line--${line.type} ${highlight ? 'gh-diff-line--highlight' : ''} ${onClick ? 'gh-diff-line--commentable' : ''}`
  const lineNumber = line.newLine || line.oldLine || 0

  if (viewMode === 'unified') {
    return (
      <div 
        className={lineClass}
        data-line={lineNumber}
        style={highlight ? { '--highlight-color': highlight.color } as React.CSSProperties : undefined}
        onClick={onClick}
      >
        <span className="gh-diff-line__gutter gh-diff-line__gutter--old">
          {line.oldLine ?? ''}
        </span>
        <span className="gh-diff-line__gutter gh-diff-line__gutter--new">
          {line.newLine ?? ''}
        </span>
        <span className="gh-diff-line__prefix">
          {line.type === 'add' ? '+' : line.type === 'delete' ? '-' : ' '}
        </span>
        <span className="gh-diff-line__content">
          {line.content}
        </span>
      </div>
    )
  }

  return null // Split view handled by DiffHunk's SplitView
}
