import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { Search, Terminal, GitBranch, Bot, Palette, Keyboard, Trash2, Maximize2, Settings2 } from 'lucide-react'
import { useLayoutStore } from '../features/command-center/stores/layoutStore'
import { useAppStore } from '../stores/appStore'
import { formatShortcutBinding, resolveShortcutBindings } from '../features/shortcuts/shortcuts'
import './CommandPalette.css'

interface Command {
  id: string
  label: string
  description?: string
  icon: React.ReactNode
  shortcut?: string
  category: string
  action: () => void
}

/**
 * CommandPalette — overlay de busca fuzzy para ações rápidas.
 * Ativado com Cmd+K.
 */
export function CommandPalette() {
  const [isOpen, setIsOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [selectedIndex, setSelectedIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)

  // Store actions
  const addPane = useLayoutStore((s) => s.addPane)
  const activePaneId = useLayoutStore((s) => s.activePaneId)
  const removePane = useLayoutStore((s) => s.removePane)
  const toggleZenMode = useLayoutStore((s) => s.toggleZenMode)
  const setTheme = useAppStore((s) => s.setTheme)
  const shortcutOverrides = useAppStore((s) => s.shortcutBindings)

  const shortcutLabels = useMemo(() => {
    const bindings = resolveShortcutBindings(shortcutOverrides)
    return {
      newTerminal: formatShortcutBinding(bindings.newTerminal),
      closePane: formatShortcutBinding(bindings.closePane),
      zenMode: formatShortcutBinding(bindings.zenMode),
      openSettings: formatShortcutBinding(bindings.openSettings),
    }
  }, [shortcutOverrides])

  /** Toggle do command palette via evento customizado */
  useEffect(() => {
    const handleToggle = () => setIsOpen((v) => !v)
    window.addEventListener('command-palette:toggle', handleToggle)
    return () => window.removeEventListener('command-palette:toggle', handleToggle)
  }, [])

  /** Todos os comandos disponíveis */
  const commands = useMemo((): Command[] => [
    {
      id: 'new-terminal',
      label: 'Novo Terminal',
      description: 'Criar um novo painel de terminal',
      icon: <Terminal size={16} />,
      shortcut: shortcutLabels.newTerminal,
      category: 'Painéis',
      action: () => window.dispatchEvent(new CustomEvent('new-terminal:toggle')),
    },
    {
      id: 'new-ai-agent',
      label: 'Novo AI Agent',
      description: 'Criar um painel de agente de IA',
      icon: <Bot size={16} />,
      category: 'Painéis',
      action: () => addPane('ai_agent'),
    },
    {
      id: 'new-github',
      label: 'Novo GitHub',
      description: 'Criar um painel de integração GitHub',
      icon: <GitBranch size={16} />,
      category: 'Painéis',
      action: () => addPane('github'),
    },
    {
      id: 'close-pane',
      label: 'Fechar Painel',
      description: 'Fechar o painel ativo',
      icon: <Trash2 size={16} />,
      shortcut: shortcutLabels.closePane,
      category: 'Painéis',
      action: () => { if (activePaneId) removePane(activePaneId) },
    },
    {
      id: 'zen-mode',
      label: 'Zen Mode',
      description: 'Maximizar/restaurar painel ativo',
      icon: <Maximize2 size={16} />,
      shortcut: shortcutLabels.zenMode,
      category: 'Painéis',
      action: () => toggleZenMode(),
    },
    {
      id: 'theme-dark',
      label: 'Tema: Dark',
      icon: <Palette size={16} />,
      category: 'Aparência',
      action: () => setTheme('dark'),
    },
    {
      id: 'theme-light',
      label: 'Tema: Light',
      icon: <Palette size={16} />,
      category: 'Aparência',
      action: () => setTheme('light'),
    },
    {
      id: 'theme-hacker',
      label: 'Tema: Hacker',
      icon: <Palette size={16} />,
      category: 'Aparência',
      action: () => setTheme('hacker'),
    },
    {
      id: 'theme-nvim',
      label: 'Tema: Nvim',
      icon: <Palette size={16} />,
      category: 'Aparência',
      action: () => setTheme('nvim'),
    },
    {
      id: 'theme-min-dark',
      label: 'Tema: Min Dark',
      icon: <Palette size={16} />,
      category: 'Aparência',
      action: () => setTheme('min-dark'),
    },
    {
      id: 'settings',
      label: 'Abrir Configurações',
      description: 'Configurações do ORCH',
      icon: <Settings2 size={16} />,
      shortcut: shortcutLabels.openSettings,
      category: 'Geral',
      action: () => window.dispatchEvent(new CustomEvent('settings:toggle')),
    },
    {
      id: 'shortcuts',
      label: 'Ver Atalhos de Teclado',
      icon: <Keyboard size={16} />,
      category: 'Ajuda',
      action: () => { /* TODO: Abrir modal de atalhos */ },
    },
  ], [addPane, activePaneId, removePane, setTheme, shortcutLabels, toggleZenMode])

  /** Filtrar comandos baseado na query (fuzzy) */
  const filteredCommands = useMemo(() => {
    if (!query.trim()) return commands

    const normalizedQuery = query.toLowerCase().trim()
    return commands.filter((cmd) => {
      const text = `${cmd.label} ${cmd.description || ''} ${cmd.category}`.toLowerCase()
      // Simple fuzzy: verificar se todas as letras da query aparecem em ordem
      let qi = 0
      for (let i = 0; i < text.length && qi < normalizedQuery.length; i++) {
        if (text[i] === normalizedQuery[qi]) qi++
      }
      return qi === normalizedQuery.length
    })
  }, [commands, query])

  /** Reset ao abrir */
  useEffect(() => {
    if (isOpen) {
      setQuery('')
      setSelectedIndex(0)
      requestAnimationFrame(() => inputRef.current?.focus())
    }
  }, [isOpen])

  /** Reset selectedIndex quando filtered commands muda */
  useEffect(() => {
    setSelectedIndex(0)
  }, [filteredCommands.length])

  /** Handler de teclado no input */
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        setSelectedIndex((i) => Math.min(i + 1, filteredCommands.length - 1))
        break
      case 'ArrowUp':
        e.preventDefault()
        setSelectedIndex((i) => Math.max(i - 1, 0))
        break
      case 'Enter':
        e.preventDefault()
        if (filteredCommands[selectedIndex]) {
          filteredCommands[selectedIndex].action()
          setIsOpen(false)
        }
        break
      case 'Escape':
        e.preventDefault()
        setIsOpen(false)
        break
    }
  }, [filteredCommands, selectedIndex])

  /** Executar comando ao clicar */
  const handleCommandClick = useCallback((cmd: Command) => {
    cmd.action()
    setIsOpen(false)
  }, [])

  if (!isOpen) return null

  // Agrupar por categoria
  const grouped = filteredCommands.reduce<Record<string, Command[]>>((acc, cmd) => {
    const cat = cmd.category
    if (!acc[cat]) acc[cat] = []
    acc[cat].push(cmd)
    return acc
  }, {})

  let flatIndex = 0

  return (
    <div className="command-palette-backdrop" onClick={() => setIsOpen(false)}>
      <div className="command-palette animate-scale-in" onClick={(e) => e.stopPropagation()}>
        {/* Search Input */}
        <div className="command-palette__search">
          <Search size={16} className="command-palette__search-icon" />
          <input
            ref={inputRef}
            type="text"
            className="command-palette__input"
            placeholder="Buscar comandos..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            aria-label="Buscar comandos"
            id="command-palette-input"
          />
          <kbd className="command-palette__hint">ESC</kbd>
        </div>

        {/* Command List */}
        <div className="command-palette__list">
          {Object.entries(grouped).map(([category, cmds]) => (
            <div key={category} className="command-palette__group">
              <div className="command-palette__group-label">{category}</div>
              {cmds.map((cmd) => {
                const idx = flatIndex++
                return (
                  <button
                    key={cmd.id}
                    className={`command-palette__item ${idx === selectedIndex ? 'command-palette__item--selected' : ''}`}
                    onClick={() => handleCommandClick(cmd)}
                    onMouseEnter={() => setSelectedIndex(idx)}
                    aria-label={cmd.label}
                  >
                    <span className="command-palette__item-icon">{cmd.icon}</span>
                    <div className="command-palette__item-text">
                      <span className="command-palette__item-label">{cmd.label}</span>
                      {cmd.description && (
                        <span className="command-palette__item-desc">{cmd.description}</span>
                      )}
                    </div>
                    {cmd.shortcut && (
                      <kbd className="command-palette__item-shortcut">{cmd.shortcut}</kbd>
                    )}
                  </button>
                )
              })}
            </div>
          ))}

          {filteredCommands.length === 0 && (
            <div className="command-palette__empty">
              Nenhum comando encontrado
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
