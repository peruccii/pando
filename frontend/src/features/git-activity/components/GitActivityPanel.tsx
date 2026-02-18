import { useMemo, useState } from 'react'
import { Bell, ChevronDown, ChevronRight, GitBranch, GitCommitHorizontal, RefreshCcw, Trash2, X } from 'lucide-react'
import { useGitActivityStore } from '../stores/gitActivityStore'
import type { GitActivityEvent } from '../stores/gitActivityStore'
import './GitActivityPanel.css'

interface StagedFile {
  path: string
  status?: string
  added?: number
  removed?: number
}

function formatDate(value?: unknown): string {
  if (!value) return ''
  const date = new Date(String(value))
  if (Number.isNaN(date.getTime())) return String(value)
  return date.toLocaleString()
}

function formatRelative(value?: unknown): string {
  if (!value) return ''
  const date = new Date(String(value))
  if (Number.isNaN(date.getTime())) return ''
  const diffMs = Date.now() - date.getTime()
  const diffSec = Math.floor(diffMs / 1000)
  if (diffSec < 60) return `${diffSec}s atrás`
  const diffMin = Math.floor(diffSec / 60)
  if (diffMin < 60) return `${diffMin}m atrás`
  const diffHour = Math.floor(diffMin / 60)
  if (diffHour < 24) return `${diffHour}h atrás`
  const diffDay = Math.floor(diffHour / 24)
  return `${diffDay}d atrás`
}

function getTypeClass(event: GitActivityEvent): string {
  switch (event.type) {
    case 'branch_changed':
    case 'branch_created':
      return 'branch'
    case 'commit_created':
    case 'commit_preparing':
      return 'commit'
    case 'index_updated':
      return 'index'
    case 'merge':
      return 'merge'
    case 'fetch':
      return 'fetch'
    default:
      return 'unknown'
  }
}

function getTypeLabel(event: GitActivityEvent): string {
  switch (event.type) {
    case 'branch_changed':
      return 'Branch'
    case 'branch_created':
      return 'Nova branch'
    case 'commit_created':
      return 'Commit'
    case 'commit_preparing':
      return 'Preparing'
    case 'index_updated':
      return 'Staged'
    case 'merge':
      return 'Merge'
    case 'fetch':
      return 'Fetch'
    default:
      return 'Git'
  }
}

export function GitActivityPanel() {
  const isOpen = useGitActivityStore((s) => s.isOpen)
  const events = useGitActivityStore((s) => s.events)
  const isLoading = useGitActivityStore((s) => s.isLoading)
  const error = useGitActivityStore((s) => s.error)
  const expandedEventIDs = useGitActivityStore((s) => s.expandedEventIDs)
  const close = useGitActivityStore((s) => s.close)
  const refresh = useGitActivityStore((s) => s.refresh)
  const clear = useGitActivityStore((s) => s.clear)
  const toggleExpanded = useGitActivityStore((s) => s.toggleExpanded)
  const [stagedFilesByEvent, setStagedFilesByEvent] = useState<Record<string, StagedFile[]>>({})
  const [diffByFileKey, setDiffByFileKey] = useState<Record<string, string>>({})
  const [openDiffKeys, setOpenDiffKeys] = useState<Record<string, boolean>>({})
  const [loadingDiffKeys, setLoadingDiffKeys] = useState<Record<string, boolean>>({})
  const [busyFileActions, setBusyFileActions] = useState<Record<string, boolean>>({})

  const headerCount = useMemo(() => events.length, [events.length])
  const api = (window.go?.main?.App as any) || null

  if (!isOpen) return null

  const getStagedFiles = (event: GitActivityEvent): StagedFile[] => {
    if (stagedFilesByEvent[event.id]) {
      return stagedFilesByEvent[event.id]
    }
    return (event.details?.files || []) as StagedFile[]
  }

  const loadStagedFiles = async (event: GitActivityEvent) => {
    if (!api?.GitActivityGetStagedFiles) return
    if (!event.repoPath || stagedFilesByEvent[event.id]) return
    try {
      const files = await api.GitActivityGetStagedFiles(event.repoPath)
      setStagedFilesByEvent((prev) => ({ ...prev, [event.id]: files || [] }))
    } catch (err) {
      console.warn('[GitActivityPanel] failed to load staged files:', err)
    }
  }

  const handleToggleDetails = async (event: GitActivityEvent) => {
    const willExpand = !expandedEventIDs[event.id]
    toggleExpanded(event.id)
    if (willExpand && event.type === 'index_updated') {
      await loadStagedFiles(event)
    }
  }

  const toggleFileDiff = async (event: GitActivityEvent, file: StagedFile) => {
    const diffKey = `${event.id}::${file.path}`
    const nextOpen = !openDiffKeys[diffKey]
    setOpenDiffKeys((prev) => ({ ...prev, [diffKey]: nextOpen }))
    if (!nextOpen) return
    if (diffByFileKey[diffKey]) return
    if (!api?.GitActivityGetStagedDiff) return

    setLoadingDiffKeys((prev) => ({ ...prev, [diffKey]: true }))
    try {
      const diff = await api.GitActivityGetStagedDiff(event.repoPath, file.path)
      setDiffByFileKey((prev) => ({ ...prev, [diffKey]: diff || '(sem diff)' }))
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setDiffByFileKey((prev) => ({ ...prev, [diffKey]: `Erro ao carregar diff: ${message}` }))
    } finally {
      setLoadingDiffKeys((prev) => ({ ...prev, [diffKey]: false }))
    }
  }

  const runFileAction = async (
    actionKey: string,
    action: () => Promise<void>,
    after: () => Promise<void>,
  ) => {
    setBusyFileActions((prev) => ({ ...prev, [actionKey]: true }))
    try {
      await action()
      await after()
    } catch (err) {
      console.error('[GitActivityPanel] file action failed:', err)
    } finally {
      setBusyFileActions((prev) => ({ ...prev, [actionKey]: false }))
    }
  }

  return (
    <aside className="git-activity-panel" aria-label="Git Activity Panel">
      <div className="git-activity-panel__header">
        <div className="git-activity-panel__title">
          <Bell size={14} />
          <span>Git Activity</span>
          <span className="git-activity-panel__count">{headerCount}</span>
        </div>
        <div className="git-activity-panel__actions">
          <button className="btn btn--ghost btn--icon" onClick={() => refresh()} title="Atualizar">
            <RefreshCcw size={14} />
          </button>
          <button className="btn btn--ghost btn--icon" onClick={() => clear()} title="Limpar eventos">
            <Trash2 size={14} />
          </button>
          <button className="btn btn--ghost btn--icon" onClick={() => close()} title="Fechar painel">
            <X size={14} />
          </button>
        </div>
      </div>

      <div className="git-activity-panel__body">
        {isLoading && (
          <div className="git-activity-panel__empty">Carregando eventos...</div>
        )}

        {!isLoading && error && (
          <div className="git-activity-panel__empty git-activity-panel__empty--error">
            Erro ao carregar atividades: {error}
          </div>
        )}

        {!isLoading && !error && events.length === 0 && (
          <div className="git-activity-panel__empty">
            Nenhuma atividade registrada ainda.
          </div>
        )}

        {!isLoading && !error && events.map((event) => {
          const expanded = !!expandedEventIDs[event.id]
          const typeClass = getTypeClass(event)
          return (
            <article key={event.id} className={`git-activity-item git-activity-item--${typeClass}`}>
              <div className="git-activity-item__main">
                <div className="git-activity-item__line">
                  <span className={`git-activity-item__chip git-activity-item__chip--${typeClass}`}>
                    {getTypeLabel(event)}
                  </span>
                  <span className="git-activity-item__message">{event.message}</span>
                </div>
                <div className="git-activity-item__meta">
                  <span className="git-activity-item__repo">
                    <GitBranch size={12} />
                    {event.repoName || event.repoPath || 'repo'}
                  </span>
                  {event.branch && (
                    <span className="git-activity-item__branch">
                      <GitCommitHorizontal size={12} />
                      {event.branch}
                    </span>
                  )}
                  <span className="git-activity-item__time" title={formatDate(event.timestamp)}>
                    {formatRelative(event.timestamp)}
                  </span>
                </div>
              </div>

              <button className="git-activity-item__expand" onClick={() => handleToggleDetails(event)} title={expanded ? 'Ocultar detalhes' : 'Ver mais detalhes'}>
                {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                {expanded ? 'Ocultar' : 'Detalhes'}
              </button>

              {expanded && (
                <div className="git-activity-item__details">
                  <div><strong>Ator:</strong> {event.actorName || 'local-user'}</div>
                  <div><strong>Repositório:</strong> {event.repoName || event.repoPath || '-'}</div>
                  <div><strong>Caminho:</strong> {event.repoPath || '-'}</div>
                  <div><strong>Horário:</strong> {formatDate(event.timestamp)}</div>
                  <div><strong>Fonte:</strong> {event.source || '-'}</div>
                  {event.details?.ref && (
                    <div><strong>Ref:</strong> {event.details.ref}</div>
                  )}
                  {event.details?.extra && Object.keys(event.details.extra).length > 0 && (
                    <div className="git-activity-item__extra">
                      <strong>Contexto:</strong>
                      <pre>{JSON.stringify(event.details.extra, null, 2)}</pre>
                    </div>
                  )}
                  {event.type === 'index_updated' && (
                    <div className="git-activity-item__staged">
                      <strong>Arquivos staged:</strong>
                      {getStagedFiles(event).length === 0 && (
                        <div className="git-activity-item__staged-empty">Nenhum arquivo staged.</div>
                      )}
                      {getStagedFiles(event).map((file) => {
                        const diffKey = `${event.id}::${file.path}`
                        const actionPrefix = `${event.id}:${file.path}`
                        const unstageKey = `${actionPrefix}:unstage`
                        const discardKey = `${actionPrefix}:discard`
                        const isBusy = !!busyFileActions[unstageKey] || !!busyFileActions[discardKey]
                        return (
                          <div key={diffKey} className="git-activity-file">
                            <div className="git-activity-file__meta">
                              <span className="git-activity-file__path">{file.path}</span>
                              <span className="git-activity-file__stats">
                                <span className="git-activity-file__added">+{file.added || 0}</span>
                                <span className="git-activity-file__removed">-{file.removed || 0}</span>
                                {file.status && <span className="git-activity-file__status">{file.status}</span>}
                              </span>
                            </div>
                            <div className="git-activity-file__actions">
                              <button className="btn btn--ghost btn--sm" onClick={() => toggleFileDiff(event, file)}>
                                {openDiffKeys[diffKey] ? 'Ocultar diff' : 'Ver diff'}
                              </button>
                              <button
                                className="btn btn--ghost btn--sm"
                                disabled={isBusy}
                                onClick={() => runFileAction(
                                  unstageKey,
                                  async () => { await api?.GitActivityUnstageFile?.(event.repoPath, file.path) },
                                  async () => {
                                    await refresh()
                                    await loadStagedFiles(event)
                                  },
                                )}
                              >
                                Unstage
                              </button>
                              <button
                                className="btn btn--danger btn--sm"
                                disabled={isBusy}
                                onClick={() => {
                                  const ok = window.confirm(`Descartar mudanças de ${file.path}?`)
                                  if (!ok) return
                                  runFileAction(
                                    discardKey,
                                    async () => { await api?.GitActivityDiscardFile?.(event.repoPath, file.path) },
                                    async () => {
                                      await refresh()
                                      await loadStagedFiles(event)
                                    },
                                  )
                                }}
                              >
                                Discard
                              </button>
                            </div>
                            {openDiffKeys[diffKey] && (
                              <div className="git-activity-file__diff">
                                {loadingDiffKeys[diffKey] ? (
                                  <div className="git-activity-file__diff-loading">Carregando diff...</div>
                                ) : (
                                  <pre>{diffByFileKey[diffKey] || '(sem diff)'}</pre>
                                )}
                              </div>
                            )}
                          </div>
                        )
                      })}
                    </div>
                  )}
                </div>
              )}
            </article>
          )
        })}
      </div>
    </aside>
  )
}
