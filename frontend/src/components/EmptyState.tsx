import { Zap, Terminal, Bot, Github } from 'lucide-react'
import { useLayout } from '../features/command-center'
import { useI18n } from '../hooks/useI18n'
import { getShortcutLabel } from '../hooks/useKeyboardShortcuts'
import { useAppStore } from '../stores/appStore'
import './EmptyState.css'

interface EmptyStateProps {
  version: string
}

export function EmptyState({ version }: EmptyStateProps) {
  const { newTerminal, newAIAgent, newGitHub } = useLayout()
  const { t } = useI18n()
  useAppStore((s) => s.shortcutBindings)
  const newTerminalShortcut = getShortcutLabel('newTerminal')
  const commandPaletteShortcut = getShortcutLabel('commandPalette')
  const toggleThemeShortcut = getShortcutLabel('toggleTheme')

  return (
    <div className="empty-state animate-fade-in" id="empty-state">
      <div className="empty-state__content">
        <div className="empty-state__logo">
          <Zap size={32} className="empty-state__logo-icon" fill="currentColor" />
        </div>
        <h1 className="empty-state__title">ORCH</h1>
        <p className="empty-state__subtitle">
          {t('emptyState.subtitle')}
        </p>

        <div className="empty-state__actions">
          <button
            className="btn btn--primary empty-state__btn"
            id="btn-new-terminal"
            aria-label={`Criar novo terminal (${newTerminalShortcut})`}
            onClick={() => newTerminal()}
          >
            <Terminal size={14} style={{ marginRight: 8 }} />
            {t('emptyState.newTerminal')}
          </button>
          <button
            className="btn btn--ghost empty-state__btn"
            id="btn-new-ai-agent"
            aria-label="Novo AI Agent"
            onClick={() => newAIAgent()}
          >
            <Bot size={14} style={{ marginRight: 8 }} />
            {t('emptyState.newAIAgent')}
          </button>
          <button
            className="btn btn--ghost empty-state__btn"
            id="btn-new-github"
            aria-label="Novo painel GitHub"
            onClick={() => newGitHub()}
          >
            <Github size={14} style={{ marginRight: 8 }} />
            {t('emptyState.newGitHub')}
          </button>
        </div>

        <div className="empty-state__shortcuts">
          <div className="empty-state__shortcut">
            <kbd>{newTerminalShortcut}</kbd>
            <span>{t('emptyState.shortcut.newTerminal')}</span>
          </div>
          <div className="empty-state__shortcut">
            <kbd>{commandPaletteShortcut}</kbd>
            <span>{t('emptyState.shortcut.commandPalette')}</span>
          </div>
          <div className="empty-state__shortcut">
            <kbd>{toggleThemeShortcut}</kbd>
            <span>{t('emptyState.shortcut.toggleTheme')}</span>
          </div>
        </div>

        <span className="empty-state__version">v{version}</span>
      </div>
    </div>
  )
}
