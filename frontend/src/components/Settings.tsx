import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { X, Settings2, Github, LogOut, GitPullRequest, GitBranch, MessageSquare, Eye, CheckCircle, Move, Layers, Keyboard } from 'lucide-react'
import {
  useAppStore,
  MIN_TERMINAL_FONT_SIZE,
  MAX_TERMINAL_FONT_SIZE,
  DEFAULT_TERMINAL_FONT_SIZE,
} from '../stores/appStore'
import { useAuthStore } from '../stores/authStore'
import { useI18n } from '../hooks/useI18n'
import { ScrollSyncSettingsPanel } from '../features/scroll-sync'
import { StackBuilder } from './StackBuilder'
import type { ShortcutId } from '../features/shortcuts/shortcuts'
import {
  SHORTCUT_DEFINITIONS,
  formatShortcutBinding,
  getBindingFromKeyboardEvent,
  getReservedConflict,
  getShortcutConflict,
  getShortcutSignature,
  resolveShortcutBindings,
} from '../features/shortcuts/shortcuts'
import * as App from '../../wailsjs/go/main/App'
import './Settings.css'

interface WindowState {
  x: number
  y: number
  width: number
  height: number
  isMinimized: boolean
  isMaximized: boolean
}

const DEFAULT_WINDOW: WindowState = {
  x: 80,
  y: 60,
  width: 1100,
  height: 800,
  isMinimized: false,
  isMaximized: false,
}

const MIN_WIDTH = 600
const MIN_HEIGHT = 400

/**
 * Settings Window — janela flutuante de configurações do ORCH
 * Pode ser movida, redimensionada e minimizada
 */
export function Settings() {
  const [isOpen, setIsOpen] = useState(false)
  const [activeTab, setActiveTab] = useState<'general' | 'github' | 'sync' | 'appearance' | 'stacks' | 'shortcuts'>('general')
  const [windowState, setWindowState] = useState<WindowState>(DEFAULT_WINDOW)
  const [availableShells, setAvailableShells] = useState<string[]>([])
  const [capturingShortcutId, setCapturingShortcutId] = useState<ShortcutId | null>(null)
  const [shortcutMessage, setShortcutMessage] = useState<string>('')
  const { t } = useI18n()
  
  // Refs para drag e resize
  const windowRef = useRef<HTMLDivElement>(null)
  const isDraggingRef = useRef(false)
  const isResizingRef = useRef(false)
  const dragStartRef = useRef({ x: 0, y: 0, windowX: 0, windowY: 0 })
  const resizeStartRef = useRef({ x: 0, y: 0, width: 0, height: 0 })
  
  // Store actions
  const theme = useAppStore((s) => s.theme)
  const setTheme = useAppStore((s) => s.setTheme)
  const language = useAppStore((s) => s.language)
  const setLanguage = useAppStore((s) => s.setLanguage)
  const defaultShell = useAppStore((s) => s.defaultShell)
  const setDefaultShell = useAppStore((s) => s.setDefaultShell)
  const terminalFontSize = useAppStore((s) => s.terminalFontSize)
  const setTerminalFontSize = useAppStore((s) => s.setTerminalFontSize)
  const resetTerminalZoom = useAppStore((s) => s.resetTerminalZoom)
  const scrollSyncSettings = useAppStore((s) => s.scrollSyncSettings)
  const setScrollSyncSettings = useAppStore((s) => s.setScrollSyncSettings)
  const shortcutOverrides = useAppStore((s) => s.shortcutBindings)
  const setShortcutBinding = useAppStore((s) => s.setShortcutBinding)
  const resetShortcutBindings = useAppStore((s) => s.resetShortcutBindings)

  const resolvedShortcutBindings = useMemo(
    () => resolveShortcutBindings(shortcutOverrides),
    [shortcutOverrides],
  )

  const shortcutGroups = useMemo(() => {
    const grouped: Record<string, {
      id: ShortcutId
      description: string
      shortcut: string
      isDefault: boolean
    }[]> = {}

    for (const definition of SHORTCUT_DEFINITIONS) {
      const binding = resolvedShortcutBindings[definition.id]
      const currentSignature = getShortcutSignature(binding)
      const defaultSignature = getShortcutSignature(definition.defaultBinding)
      if (!grouped[definition.category]) {
        grouped[definition.category] = []
      }

      grouped[definition.category].push({
        id: definition.id,
        description: definition.description,
        shortcut: formatShortcutBinding(binding),
        isDefault: currentSignature === defaultSignature,
      })
    }

    return grouped
  }, [resolvedShortcutBindings])

  // Auth state
  const { isAuthenticated, user, isLoading: authLoading } = useAuthStore()

  useEffect(() => {
    if (isOpen) {
      App.GetAvailableShells()
        .then(setAvailableShells)
        .catch(err => console.error('Failed to fetch shells:', err))
    }
  }, [isOpen])

  // Toggle handler via evento customizado
  useEffect(() => {
    const handleToggle = () => setIsOpen((v) => !v)
    window.addEventListener('settings:toggle', handleToggle)
    return () => window.removeEventListener('settings:toggle', handleToggle)
  }, [])

  // Fechar com ESC
  useEffect(() => {
    if (!isOpen) return
    
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setIsOpen(false)
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [isOpen])

  useEffect(() => {
    if (isOpen) return
    setCapturingShortcutId(null)
  }, [isOpen])

  useEffect(() => {
    if (!isOpen || !capturingShortcutId) return

    const handleCapture = (e: KeyboardEvent) => {
      e.preventDefault()
      e.stopPropagation()

      if (
        e.key === 'Escape' &&
        !e.metaKey &&
        !e.ctrlKey &&
        !e.altKey &&
        !e.shiftKey
      ) {
        setCapturingShortcutId(null)
        setShortcutMessage('Captura cancelada.')
        return
      }

      const binding = getBindingFromKeyboardEvent(e)
      if (!binding) {
        setShortcutMessage('Tecla não suportada. Tente outra combinação.')
        return
      }

      if (!binding.meta) {
        setShortcutMessage('Use uma combinação com ⌘ (ou Ctrl) para evitar conflito com digitação.')
        return
      }

      const reservedConflict = getReservedConflict(binding)
      if (reservedConflict) {
        setShortcutMessage(`Conflito com atalho reservado: ${reservedConflict.description}.`)
        return
      }

      const conflict = getShortcutConflict(capturingShortcutId, binding, resolvedShortcutBindings)
      if (conflict) {
        setShortcutMessage(`Duplicado com "${conflict.description}". Escolha outra combinação.`)
        return
      }

      setShortcutBinding(capturingShortcutId, binding)
      setCapturingShortcutId(null)
      setShortcutMessage(`Atalho atualizado para ${formatShortcutBinding(binding)}.`)
    }

    window.addEventListener('keydown', handleCapture, true)
    return () => window.removeEventListener('keydown', handleCapture, true)
  }, [capturingShortcutId, isOpen, resolvedShortcutBindings, setShortcutBinding])

  const handleShortcutCaptureStart = useCallback((shortcutId: ShortcutId) => {
    setCapturingShortcutId(shortcutId)
    setShortcutMessage('Pressione a nova combinação de teclas...')
  }, [])

  const handleShortcutCaptureCancel = useCallback(() => {
    setCapturingShortcutId(null)
    setShortcutMessage('Captura cancelada.')
  }, [])

  const handleShortcutReset = useCallback((shortcutId: ShortcutId) => {
    const definition = SHORTCUT_DEFINITIONS.find((item) => item.id === shortcutId)
    if (!definition) return
    setShortcutBinding(shortcutId, definition.defaultBinding)
    setCapturingShortcutId(null)
    setShortcutMessage(`Atalho "${definition.description}" restaurado.`)
  }, [setShortcutBinding])

  const handleShortcutResetAll = useCallback(() => {
    resetShortcutBindings()
    setCapturingShortcutId(null)
    setShortcutMessage('Todos os atalhos foram restaurados para o padrão.')
  }, [resetShortcutBindings])

  const categoryLabels: Record<string, string> = {
    panel: 'Painéis',
    navigation: 'Navegação',
    collaboration: 'Colaboração',
    general: 'Geral',
  }

  // Drag handlers
  const handleDragStart = useCallback((e: React.MouseEvent) => {
    if (windowState.isMaximized) return
    
    isDraggingRef.current = true
    dragStartRef.current = {
      x: e.clientX,
      y: e.clientY,
      windowX: windowState.x,
      windowY: windowState.y,
    }
    
    document.body.style.cursor = 'grabbing'
    e.preventDefault()
  }, [windowState.x, windowState.y, windowState.isMaximized])

  // Resize handlers
  const handleResizeStart = useCallback((e: React.MouseEvent, direction: string) => {
    isResizingRef.current = true
    resizeStartRef.current = {
      x: e.clientX,
      y: e.clientY,
      width: windowState.width,
      height: windowState.height,
    }
    
    document.body.style.cursor = direction === 'se' ? 'se-resize' : 'nwse-resize'
    e.preventDefault()
    e.stopPropagation()
  }, [windowState.width, windowState.height])

  // Global mouse move/up handlers
  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (isDraggingRef.current) {
        const deltaX = e.clientX - dragStartRef.current.x
        const deltaY = e.clientY - dragStartRef.current.y
        
        setWindowState(prev => ({
          ...prev,
          x: Math.max(0, dragStartRef.current.windowX + deltaX),
          y: Math.max(0, dragStartRef.current.windowY + deltaY),
        }))
      }
      
      if (isResizingRef.current) {
        const deltaX = e.clientX - resizeStartRef.current.x
        const deltaY = e.clientY - resizeStartRef.current.y
        
        setWindowState(prev => ({
          ...prev,
          width: Math.max(MIN_WIDTH, resizeStartRef.current.width + deltaX),
          height: Math.max(MIN_HEIGHT, resizeStartRef.current.height + deltaY),
        }))
      }
    }

    const handleMouseUp = () => {
      if (isDraggingRef.current || isResizingRef.current) {
        isDraggingRef.current = false
        isResizingRef.current = false
        document.body.style.cursor = ''
      }
    }

    window.addEventListener('mousemove', handleMouseMove)
    window.addEventListener('mouseup', handleMouseUp)
    
    return () => {
      window.removeEventListener('mousemove', handleMouseMove)
      window.removeEventListener('mouseup', handleMouseUp)
    }
  }, [])

  const handleClose = useCallback(() => {
    setIsOpen(false)
  }, [])

  const handleMinimize = useCallback(() => {
    setWindowState(prev => ({ ...prev, isMinimized: true }))
  }, [])

  const handleMaximize = useCallback(() => {
    setWindowState(prev => {
      const newMaximized = !prev.isMaximized
      if (newMaximized) {
        // Maximize
        return {
          ...prev,
          isMaximized: true,
          x: 20,
          y: 20,
          width: window.innerWidth - 40,
          height: window.innerHeight - 40,
        }
      } else {
        // Restore to default
        return {
          ...DEFAULT_WINDOW,
          ...prev,
          isMaximized: false,
        }
      }
    })
  }, [])

  const handleLogin = async () => {
    try {
      await App.AuthLogin('github')
    } catch (err) {
      console.error('Failed to login:', err)
    }
  }

  const handleLogout = async () => {
    try {
      await App.AuthLogout()
    } catch (err) {
      console.error('Failed to logout:', err)
    }
  }

  if (!isOpen) return null

  const windowStyle: React.CSSProperties = windowState.isMaximized
    ? {
        position: 'fixed',
        top: 20,
        left: 20,
        right: 20,
        bottom: 20,
        width: 'auto',
        height: 'auto',
      }
    : {
        position: 'fixed',
        top: windowState.y,
        left: windowState.x,
        width: windowState.width,
        height: windowState.height,
      }

  return (
    <div 
      ref={windowRef}
      className={`settings-window ${windowState.isMinimized ? 'settings-window--minimized' : ''}`}
      style={windowStyle}
    >
      {/* Header / Title Bar */}
      <div 
        className="settings-window__header"
        onMouseDown={handleDragStart}
      >
        <div className="settings-window__title">
          <Move size={14} className="settings-window__drag-icon" />
          <Settings2 size={16} />
          <span>{t('settings.title')}</span>
        </div>
        <div className="settings-window__controls">
          <button 
            className="settings-window__btn settings-window__btn--minimize"
            onClick={handleMinimize}
            title="Minimize"
          >
            <span>─</span>
          </button>
          <button 
            className="settings-window__btn settings-window__btn--maximize"
            onClick={handleMaximize}
            title={windowState.isMaximized ? 'Restore' : 'Maximize'}
          >
            <span>{windowState.isMaximized ? '❐' : '□'}</span>
          </button>
          <button 
            className="settings-window__btn settings-window__btn--close"
            onClick={handleClose}
            title="Close"
          >
            <X size={14} />
          </button>
        </div>
      </div>

      {/* Body */}
      <div className="settings-window__body">
        {/* Sidebar */}
        <div className="settings-sidebar">
          <button
            className={`settings-sidebar__item ${activeTab === 'general' ? 'settings-sidebar__item--active' : ''}`}
            onClick={() => setActiveTab('general')}
          >
            {t('settings.tabs.general')}
          </button>
          <button
            className={`settings-sidebar__item ${activeTab === 'github' ? 'settings-sidebar__item--active' : ''}`}
            onClick={() => setActiveTab('github')}
          >
            {t('settings.tabs.github')}
          </button>
          <button
            className={`settings-sidebar__item ${activeTab === 'sync' ? 'settings-sidebar__item--active' : ''}`}
            onClick={() => setActiveTab('sync')}
          >
            {t('settings.tabs.sync')}
          </button>
          <button
            className={`settings-sidebar__item ${activeTab === 'appearance' ? 'settings-sidebar__item--active' : ''}`}
            onClick={() => setActiveTab('appearance')}
          >
            {t('settings.tabs.appearance')}
          </button>
          <button
            className={`settings-sidebar__item ${activeTab === 'shortcuts' ? 'settings-sidebar__item--active' : ''}`}
            onClick={() => setActiveTab('shortcuts')}
          >
            <Keyboard size={14} style={{ marginRight: 8 }} />
            {t('settings.tabs.shortcuts')}
          </button>
          <button
            className={`settings-sidebar__item ${activeTab === 'stacks' ? 'settings-sidebar__item--active' : ''}`}
            onClick={() => setActiveTab('stacks')}
          >
            <Layers size={14} style={{ marginRight: 8 }} />
            Ambientes
          </button>
        </div>

        {/* Content */}
        <div className="settings-content">
          {activeTab === 'general' && (
            <div className="settings-section">
              <h3 className="settings-section__title">{t('settings.general.title')}</h3>
              <p className="settings-section__description">
                {t('settings.general.description')}
              </p>

              <div className="settings-form-group">
                <label className="settings-label">{t('settings.general.language')}</label>
                <div className="settings-theme-options">
                  <button
                    className={`settings-theme-btn ${language === 'pt-BR' ? 'settings-theme-btn--active' : ''}`}
                    onClick={() => setLanguage('pt-BR')}
                  >
                    <span>{t('common.language.ptBR')}</span>
                  </button>
                  <button
                    className={`settings-theme-btn ${language === 'en-US' ? 'settings-theme-btn--active' : ''}`}
                    onClick={() => setLanguage('en-US')}
                  >
                    <span>{t('common.language.enUS')}</span>
                  </button>
                </div>
              </div>

              <div className="settings-form-group">
                <label className="settings-label">{t('settings.general.shell')}</label>
                <p className="settings-field-description">{t('settings.general.shell.description')}</p>
                <select 
                  className="settings-select"
                  value={defaultShell}
                  onChange={(e) => setDefaultShell(e.target.value)}
                >
                  <option value="">{t('settings.general.shell.auto')}</option>
                  {availableShells.map(shell => (
                    <option key={shell} value={shell}>
                      {shell}
                    </option>
                  ))}
                </select>
              </div>

            </div>
          )}

          {activeTab === 'github' && (
            <div className="settings-section">
              <h3 className="settings-section__title">{t('settings.github.title')}</h3>
              <p className="settings-section__description">
                {t('settings.github.description')}
              </p>

              {isAuthenticated && user ? (
                <>
                  <div className="settings-github-profile">
                    <div className="settings-github-info">
                      <img 
                        src={user.avatarUrl} 
                        alt={user.name} 
                        className="settings-github-avatar"
                      />
                      <div className="settings-github-text">
                        <span className="settings-github-name">{user.name}</span>
                        <span className="settings-github-email">{user.email}</span>
                      </div>
                    </div>
                    <button 
                      className="settings-github-btn settings-github-btn--disconnect"
                      onClick={handleLogout}
                    >
                      <LogOut size={16} />
                      {t('settings.github.disconnect')}
                    </button>
                  </div>

                  <div className="settings-github-capabilities">
                    <h4 className="settings-github-capabilities__title">
                      {t('settings.github.capabilities.title')}
                    </h4>
                    <p className="settings-github-capabilities__description">
                      {t('settings.github.capabilities.description')}
                    </p>
                    
                    <div className="settings-github-features">
                      <div className="settings-github-feature">
                        <GitPullRequest size={18} className="settings-github-feature__icon" />
                        <div className="settings-github-feature__content">
                          <span className="settings-github-feature__title">{t('settings.github.features.prs.title')}</span>
                          <span className="settings-github-feature__desc">{t('settings.github.features.prs.desc')}</span>
                        </div>
                      </div>
                      
                      <div className="settings-github-feature">
                        <GitBranch size={18} className="settings-github-feature__icon" />
                        <div className="settings-github-feature__content">
                          <span className="settings-github-feature__title">{t('settings.github.features.branches.title')}</span>
                          <span className="settings-github-feature__desc">{t('settings.github.features.branches.desc')}</span>
                        </div>
                      </div>
                      
                      <div className="settings-github-feature">
                        <MessageSquare size={18} className="settings-github-feature__icon" />
                        <div className="settings-github-feature__content">
                          <span className="settings-github-feature__title">{t('settings.github.features.comments.title')}</span>
                          <span className="settings-github-feature__desc">{t('settings.github.features.comments.desc')}</span>
                        </div>
                      </div>
                      
                      <div className="settings-github-feature">
                        <Eye size={18} className="settings-github-feature__icon" />
                        <div className="settings-github-feature__content">
                          <span className="settings-github-feature__title">{t('settings.github.features.review.title')}</span>
                          <span className="settings-github-feature__desc">{t('settings.github.features.review.desc')}</span>
                        </div>
                      </div>
                      
                      <div className="settings-github-feature">
                        <CheckCircle size={18} className="settings-github-feature__icon" />
                        <div className="settings-github-feature__content">
                          <span className="settings-github-feature__title">{t('settings.github.features.merge.title')}</span>
                          <span className="settings-github-feature__desc">{t('settings.github.features.merge.desc')}</span>
                        </div>
                      </div>
                    </div>

                    <div className="settings-github-collab">
                      <h4 className="settings-github-collab__title">
                        {t('settings.github.collab.title')}
                      </h4>
                      <p className="settings-github-collab__text">
                        {t('settings.github.collab.description')}
                      </p>
                    </div>
                  </div>
                </>
              ) : (
                <div className="settings-github-connect-section">
                  <p className="settings-field-description">
                    {t('settings.github.connectDescription')}
                  </p>
                  <button 
                    className="settings-github-btn settings-github-btn--connect"
                    onClick={handleLogin}
                    disabled={authLoading}
                  >
                    <Github size={18} />
                    {t('settings.github.connect')}
                  </button>
                  
                  <div className="settings-github-preview">
                    <h4>{t('settings.github.preview.title')}</h4>
                    <ul className="settings-github-preview__list">
                      <li>{t('settings.github.preview.item1')}</li>
                      <li>{t('settings.github.preview.item2')}</li>
                      <li>{t('settings.github.preview.item3')}</li>
                      <li>{t('settings.github.preview.item4')}</li>
                    </ul>
                  </div>
                </div>
              )}
            </div>
          )}

          {activeTab === 'sync' && (
            <div className="settings-section">
              <h3 className="settings-section__title">{t('settings.sync.title')}</h3>
              <p className="settings-section__description">
                {t('settings.sync.description')}
              </p>
              <ScrollSyncSettingsPanel 
                settings={scrollSyncSettings} 
                onUpdate={setScrollSyncSettings} 
              />
            </div>
          )}

          {activeTab === 'appearance' && (
            <div className="settings-section">
              <h3 className="settings-section__title">{t('settings.appearance.title')}</h3>
              <p className="settings-section__description">
                {t('settings.appearance.description')}
              </p>
              
              <div className="settings-form-group">
                <label className="settings-label">{t('settings.appearance.theme')}</label>
                <div className="settings-theme-options">
                  <button
                    className={`settings-theme-btn ${theme === 'dark' ? 'settings-theme-btn--active' : ''}`}
                    onClick={() => setTheme('dark')}
                  >
                    <div className="settings-theme-preview settings-theme-preview--dark" />
                    <span>{t('common.theme.dark')}</span>
                  </button>
                  <button
                    className={`settings-theme-btn ${theme === 'light' ? 'settings-theme-btn--active' : ''}`}
                    onClick={() => setTheme('light')}
                  >
                    <div className="settings-theme-preview settings-theme-preview--light" />
                    <span>{t('common.theme.light')}</span>
                  </button>
                  <button
                    className={`settings-theme-btn ${theme === 'hacker' ? 'settings-theme-btn--active' : ''}`}
                    onClick={() => setTheme('hacker')}
                  >
                    <div className="settings-theme-preview settings-theme-preview--hacker" />
                    <span>{t('common.theme.hacker')}</span>
                  </button>
                  <button
                    className={`settings-theme-btn ${theme === 'nvim' ? 'settings-theme-btn--active' : ''}`}
                    onClick={() => setTheme('nvim')}
                  >
                    <div className="settings-theme-preview settings-theme-preview--nvim" />
                    <span>{t('common.theme.nvim')}</span>
                  </button>
                  <button
                    className={`settings-theme-btn ${theme === 'min-dark' ? 'settings-theme-btn--active' : ''}`}
                    onClick={() => setTheme('min-dark')}
                  >
                    <div className="settings-theme-preview settings-theme-preview--min-dark" />
                    <span>{t('common.theme.min-dark')}</span>
                  </button>
                </div>
              </div>

              <div className="settings-form-group">
                <label className="settings-label">{t('settings.appearance.terminalZoom')}</label>
                <p className="settings-field-description">{t('settings.appearance.terminalZoom.description')}</p>

                <div className="settings-terminal-zoom">
                  <div className="settings-terminal-zoom__controls">
                    <button
                      className="settings-terminal-zoom__btn"
                      onClick={() => setTerminalFontSize(terminalFontSize - 1)}
                      disabled={terminalFontSize <= MIN_TERMINAL_FONT_SIZE}
                      aria-label={t('settings.appearance.terminalZoom.decrease')}
                    >
                      -
                    </button>
                    <input
                      className="settings-terminal-zoom__slider"
                      type="range"
                      min={MIN_TERMINAL_FONT_SIZE}
                      max={MAX_TERMINAL_FONT_SIZE}
                      step={1}
                      value={terminalFontSize}
                      onChange={(e) => setTerminalFontSize(Number(e.target.value))}
                      aria-label={t('settings.appearance.terminalZoom')}
                    />
                    <button
                      className="settings-terminal-zoom__btn"
                      onClick={() => setTerminalFontSize(terminalFontSize + 1)}
                      disabled={terminalFontSize >= MAX_TERMINAL_FONT_SIZE}
                      aria-label={t('settings.appearance.terminalZoom.increase')}
                    >
                      +
                    </button>
                  </div>

                  <div className="settings-terminal-zoom__meta">
                    <span>{terminalFontSize}px</span>
                    <button
                      className="settings-terminal-zoom__reset"
                      onClick={resetTerminalZoom}
                      disabled={terminalFontSize === DEFAULT_TERMINAL_FONT_SIZE}
                    >
                      {t('settings.appearance.terminalZoom.reset')}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'shortcuts' && (
            <div className="settings-section">
              <h3 className="settings-section__title">{t('settings.tabs.shortcuts')}</h3>
              <p className="settings-section__description">
                Personalize os atalhos e troque as combinações para o seu fluxo.
              </p>

              <div className="settings-shortcuts-toolbar">
                <button
                  className="settings-shortcut-action settings-shortcut-action--secondary"
                  onClick={handleShortcutResetAll}
                >
                  Restaurar todos
                </button>
                {capturingShortcutId && (
                  <button
                    className="settings-shortcut-action settings-shortcut-action--secondary"
                    onClick={handleShortcutCaptureCancel}
                  >
                    Cancelar captura
                  </button>
                )}
              </div>

              {shortcutMessage && (
                <div className="settings-shortcuts-message">
                  {shortcutMessage}
                </div>
              )}

              <div className="settings-shortcuts-list">
                {Object.entries(shortcutGroups).map(([category, shortcuts]) => (
                  <div key={category}>
                    <div className="settings-shortcut-category">{categoryLabels[category] || category}</div>
                    <div className="settings-shortcuts-list" style={{ gap: 'var(--space-1)' }}>
                      {shortcuts.map((s) => (
                        <div
                          key={s.id}
                          className={`settings-shortcut-item ${capturingShortcutId === s.id ? 'settings-shortcut-item--capturing' : ''}`}
                        >
                          <span className="settings-shortcut-description">{s.description}</span>
                          <div className="settings-shortcut-controls">
                            <kbd className="settings-shortcut-key">{s.shortcut}</kbd>
                            <button
                              className={`settings-shortcut-action ${capturingShortcutId === s.id ? 'settings-shortcut-action--active' : ''}`}
                              onClick={() => handleShortcutCaptureStart(s.id)}
                            >
                              {capturingShortcutId === s.id ? 'Pressione...' : 'Trocar'}
                            </button>
                            <button
                              className="settings-shortcut-action settings-shortcut-action--secondary"
                              onClick={() => handleShortcutReset(s.id)}
                              disabled={s.isDefault}
                            >
                              Resetar
                            </button>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {activeTab === 'stacks' && (
            <div className="settings-section" style={{ height: '100%', padding: 0 }}>
              <div style={{ padding: '0 var(--space-6)', height: '100%' }}>
                <StackBuilder />
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Resize Handle */}
      <div 
        className="settings-window__resize-handle"
        onMouseDown={(e) => handleResizeStart(e, 'se')}
        title="Resize"
      />
    </div>
  )
}
