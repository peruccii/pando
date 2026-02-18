import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useLayoutStore } from '../../command-center/stores/layoutStore'
import { resolveTargetPaneIDs, useBroadcastStore, type BroadcastTarget, type TargetFilter } from '../stores/broadcastStore'
import './BroadcastBar.css'

export function BroadcastBar() {
  const inputRef = useRef<HTMLInputElement>(null)

  const panes = useLayoutStore((s) => s.panes)

  const isActive = useBroadcastStore((s) => s.isActive)
  const targetFilter = useBroadcastStore((s) => s.targetFilter)
  const targetAgentIDs = useBroadcastStore((s) => s.targetAgentIDs)
  const history = useBroadcastStore((s) => s.history)
  const toggle = useBroadcastStore((s) => s.toggle)
  const deactivate = useBroadcastStore((s) => s.deactivate)
  const setTargetFilter = useBroadcastStore((s) => s.setTargetFilter)
  const setTargets = useBroadcastStore((s) => s.setTargets)
  const send = useBroadcastStore((s) => s.send)

  const [inputValue, setInputValue] = useState('')
  const [historyCursor, setHistoryCursor] = useState(-1)
  const [historyDraft, setHistoryDraft] = useState('')
  const [feedback, setFeedback] = useState('')

  const terminalTargets = useMemo<BroadcastTarget[]>(
    () =>
      Object.values(panes)
        .filter((pane) => pane.type === 'terminal')
        .map((pane) => ({
          id: pane.id,
          status: pane.status,
          sessionID: pane.sessionID,
        })),
    [panes]
  )

  const selectedPaneIDs = useMemo(
    () => resolveTargetPaneIDs(terminalTargets, targetFilter, targetAgentIDs),
    [terminalTargets, targetFilter, targetAgentIDs]
  )

  const selectedCount = selectedPaneIDs.length

  useEffect(() => {
    if (!isActive) {
      setHistoryCursor(-1)
      setHistoryDraft('')
      return
    }

    requestAnimationFrame(() => {
      inputRef.current?.focus()
    })
  }, [isActive])

  useEffect(() => {
    if (!isActive) return

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        deactivate()
      }
    }

    window.addEventListener('keydown', handleKeyDown, true)
    return () => window.removeEventListener('keydown', handleKeyDown, true)
  }, [isActive, deactivate])

  useEffect(() => {
    if (!feedback) return
    const timer = setTimeout(() => setFeedback(''), 2200)
    return () => clearTimeout(timer)
  }, [feedback])

  const handleSend = useCallback(async () => {
    if (!isActive) return

    const message = inputValue.trim()
    if (!message) return

    const result = await send(message)

    if (result.targeted === 0) {
      setFeedback('Nenhum terminal alvo disponível.')
    } else {
      const details = [`${result.sent}/${result.targeted} enviados`]
      if (result.skipped > 0) details.push(`${result.skipped} ignorados`)
      if (result.failed > 0) details.push(`${result.failed} falhas`)
      setFeedback(details.join(' · '))
    }

    setInputValue('')
    setHistoryCursor(-1)
    setHistoryDraft('')
  }, [isActive, inputValue, send])

  const handleInputKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter' && e.ctrlKey) {
        e.preventDefault()
        void handleSend()
        return
      }

      if (e.key === 'ArrowUp') {
        if (history.length === 0) return
        e.preventDefault()

        if (historyCursor === -1) {
          setHistoryDraft(inputValue)
          const nextCursor = history.length - 1
          setHistoryCursor(nextCursor)
          setInputValue(history[nextCursor])
          return
        }

        const nextCursor = Math.max(0, historyCursor - 1)
        setHistoryCursor(nextCursor)
        setInputValue(history[nextCursor])
        return
      }

      if (e.key === 'ArrowDown') {
        if (historyCursor === -1) return
        e.preventDefault()

        if (historyCursor >= history.length - 1) {
          setHistoryCursor(-1)
          setInputValue(historyDraft)
          return
        }

        const nextCursor = historyCursor + 1
        setHistoryCursor(nextCursor)
        setInputValue(history[nextCursor])
      }
    },
    [handleSend, history, historyCursor, historyDraft, inputValue]
  )

  const handleTargetFilterChange = useCallback(
    (e: React.ChangeEvent<HTMLSelectElement>) => {
      const nextFilter = e.target.value as TargetFilter
      setTargetFilter(nextFilter)
    },
    [setTargetFilter]
  )

  const handleCustomTargetsChange = useCallback(
    (e: React.ChangeEvent<HTMLSelectElement>) => {
      const ids = Array.from(e.target.selectedOptions).map((option) => option.value)
      setTargets(ids)
    },
    [setTargets]
  )

  return (
    <div className={`broadcast-shell ${isActive ? 'broadcast-shell--active' : 'broadcast-shell--inactive'}`}>
      <div className="broadcast-shell__row">
        <button
          className={`broadcast-shell__toggle ${isActive ? 'broadcast-shell__toggle--active' : ''}`}
          type="button"
          onClick={toggle}
          aria-label="Alternar Broadcast Mode"
        >
          ⚡ BROADCAST
        </button>

        <div className="broadcast-shell__targets">
          <label htmlFor="broadcast-target-filter">Targets</label>
          <select
            id="broadcast-target-filter"
            className="broadcast-shell__select"
            value={targetFilter}
            onChange={handleTargetFilterChange}
          >
            <option value="all">All</option>
            <option value="running">Running</option>
            <option value="idle">Idle</option>
            <option value="custom">Custom</option>
          </select>
          <span className="broadcast-shell__count">({selectedCount})</span>
        </div>

        <input
          ref={inputRef}
          className="broadcast-shell__input"
          type="text"
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          onKeyDown={handleInputKeyDown}
          placeholder="Digite o comando para broadcast..."
          aria-label="Mensagem de broadcast"
        />

        <button
          className="broadcast-shell__send"
          type="button"
          onClick={() => void handleSend()}
          disabled={inputValue.trim().length === 0}
          aria-label="Enviar broadcast (Ctrl+Enter)"
        >
          ⌃↵ Send
        </button>

        <span className="broadcast-shell__hint">Esc fecha</span>
      </div>

      {isActive && targetFilter === 'custom' && (
        <div className="broadcast-shell__custom">
          <label htmlFor="broadcast-custom-targets" className="broadcast-shell__custom-label">
            Selecione os terminais alvo
          </label>
          <select
            id="broadcast-custom-targets"
            className="broadcast-shell__multi-select"
            multiple
            value={targetAgentIDs}
            onChange={handleCustomTargetsChange}
            aria-label="Selecionar terminais para broadcast"
          >
            {terminalTargets.map((target) => (
              <option key={target.id} value={target.id}>
                {panes[target.id]?.title || target.id} ({target.status})
              </option>
            ))}
          </select>
        </div>
      )}

      {feedback && <div className="broadcast-shell__feedback">{feedback}</div>}
    </div>
  )
}
