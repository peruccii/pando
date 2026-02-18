import React, { useState } from 'react'
import { useBranches } from '../hooks/useBranches'
import { useGitHubStore } from '../stores/githubStore'

export const BranchSelector: React.FC = () => {
  const { branches, isLoading, createBranch } = useBranches()
  const currentBranch = useGitHubStore(s => s.currentBranch)
  const [isOpen, setIsOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [newBranchName, setNewBranchName] = useState('')
  const [sourceBranch, setSourceBranch] = useState('main')

  const filtered = branches.filter(b => b.name.toLowerCase().includes(search.toLowerCase()))

  const handleCreate = async () => {
    if (!newBranchName.trim()) return
    await createBranch(newBranchName, sourceBranch)
    setNewBranchName('')
    setShowCreate(false)
  }

  return (
    <div className="gh-branch-selector">
      <button className="gh-branch-selector__trigger" onClick={() => setIsOpen(!isOpen)}>
        <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor">
          <path d="M11.75 2.5a.75.75 0 100 1.5.75.75 0 000-1.5zm-2.25.75a2.25 2.25 0 113 2.122V6A2.5 2.5 0 0110 8.5H6a1 1 0 00-1 1v1.128a2.251 2.251 0 11-1.5 0V5.372a2.25 2.25 0 111.5 0v1.836A2.492 2.492 0 016 7h4a1 1 0 001-1v-.628A2.25 2.25 0 019.5 3.25zM4.25 12a.75.75 0 100 1.5.75.75 0 000-1.5zM3.5 3.25a.75.75 0 111.5 0 .75.75 0 01-1.5 0z" />
        </svg>
        {currentBranch || 'Branches'}
        <span className="gh-branch-selector__count">{branches.length}</span>
        <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor" style={{ marginLeft: 'auto', transform: isOpen ? 'rotate(180deg)' : '', transition: 'transform 150ms' }}>
          <path d="M12.78 6.22a.75.75 0 010 1.06l-4.25 4.25a.75.75 0 01-1.06 0L3.22 7.28a.75.75 0 011.06-1.06L8 9.94l3.72-3.72a.75.75 0 011.06 0z" />
        </svg>
      </button>

      {isOpen && (
        <div className="gh-branch-selector__dropdown">
          <div className="gh-branch-selector__search">
            <input
              type="text"
              placeholder="Filter branches..."
              value={search}
              onChange={e => setSearch(e.target.value)}
              autoFocus
            />
          </div>

          <div className="gh-branch-selector__list">
            {isLoading ? (
              <div className="gh-branch-selector__loading">
                <div className="gh-spinner gh-spinner--small" />
              </div>
            ) : filtered.length === 0 ? (
              <div className="gh-branch-selector__empty">No branches found</div>
            ) : (
              filtered.map(branch => (
                <div 
                  key={branch.name} 
                  className={`gh-branch-selector__item ${branch.name === currentBranch ? 'gh-branch-selector__item--active' : ''}`}
                >
                  <svg width="12" height="12" viewBox="0 0 16 16" fill="currentColor" opacity={branch.name === currentBranch ? '1' : '0.5'}>
                    <path d="M11.75 2.5a.75.75 0 100 1.5.75.75 0 000-1.5z" />
                  </svg>
                  <span className="gh-branch-selector__name">{branch.name}</span>
                  <code className="gh-branch-selector__sha">{branch.commit.substring(0, 7)}</code>
                </div>
              ))
            )}
          </div>

          <div className="gh-branch-selector__footer">
            {showCreate ? (
              <div className="gh-branch-selector__create">
                <input
                  type="text"
                  placeholder="New branch name..."
                  value={newBranchName}
                  onChange={e => setNewBranchName(e.target.value)}
                />
                <select value={sourceBranch} onChange={e => setSourceBranch(e.target.value)}>
                  {branches.map(b => (
                    <option key={b.name} value={b.name}>{b.name}</option>
                  ))}
                </select>
                <button className="gh-btn gh-btn--primary gh-btn--small" onClick={handleCreate}>
                  Create
                </button>
              </div>
            ) : (
              <button className="gh-btn gh-btn--ghost gh-btn--small" onClick={() => setShowCreate(true)}>
                + New Branch
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
