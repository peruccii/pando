import { useState, useEffect } from 'react'
import { Settings as SettingsIcon } from 'lucide-react'
import { useAppStore } from '../stores/appStore'
import { useStackBuildStore } from '../stores/stackBuildStore'
import { useI18n } from '../hooks/useI18n'
import { useGitActivityStore } from '../features/git-activity/stores/gitActivityStore'
import './Titlebar.css'

function mapTypeToClass(type?: string): string {
  switch (type) {
    case 'branch_changed':
    case 'branch_created':
      return 'branch'
    case 'commit_created':
    case 'commit_preparing':
      return 'commit'
    case 'merge':
      return 'merge'
    case 'fetch':
      return 'fetch'
    case 'index_updated':
      return 'index'
    default:
      return 'unknown'
  }
}

export function Titlebar() {
  const theme = useAppStore((s) => s.theme)
  const setTheme = useAppStore((s) => s.setTheme)
  const isBuilding = useStackBuildStore((s) => s.isBuilding)
  const elapsedTime = useStackBuildStore((s) => s.elapsedTime)
  const latestActivity = useGitActivityStore((s) => s.lastEvent)
  const unreadCount = useGitActivityStore((s) => s.unreadCount)
  const toggleGitActivityPanel = useGitActivityStore((s) => s.toggleOpen)
  const { t } = useI18n()

  const [asciiFrame, setAsciiFrame] = useState(0)
  const asciiFrames = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"]

  useEffect(() => {
    if (!isBuilding) return;
    const frameTimer = setInterval(() => {
      setAsciiFrame(prev => (prev + 1) % asciiFrames.length);
    }, 80);
    return () => clearInterval(frameTimer);
  }, [isBuilding]);

  const formatTime = (seconds: number) => {
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins}:${secs.toString().padStart(2, "0")}`;
  };

  const handleSettings = () => {
    window.dispatchEvent(new CustomEvent('settings:toggle'))
  }

  const cycleTheme = () => {
    const themes = ['dark', 'light', 'hacker', 'nvim', 'min-dark'] as const
    const currentIndex = themes.indexOf(theme)
    const nextIndex = (currentIndex + 1) % themes.length
    setTheme(themes[nextIndex])
  }

  return (
    <header className="titlebar" id="titlebar">
      <div className="titlebar__actions">
        {latestActivity && (
          <button
            className={`titlebar-git-activity titlebar-git-activity--${mapTypeToClass(latestActivity.type)}`}
            title="Abrir painel de atividade Git"
            onClick={toggleGitActivityPanel}
            aria-label="Open Git activity panel"
          >
            <span className="titlebar-git-activity__dot" />
            <span className="titlebar-git-activity__text">Git</span>
            {unreadCount > 0 && (
              <span className="titlebar-git-activity__badge">{unreadCount > 99 ? '99+' : unreadCount}</span>
            )}
          </button>
        )}
        {isBuilding && (
          <div className="titlebar-build-indicator" onClick={handleSettings} title="Construindo ambiente... Clique para ver detalhes.">
            <span className="ascii-loader">{asciiFrames[asciiFrame]}</span>
            <span className="build-timer">{formatTime(elapsedTime)}</span>
          </div>
        )}
        <button
          className="btn btn--icon btn--ghost"
          onClick={handleSettings}
          title={t('settings.title') + ' (⌘,)'}
          aria-label={t('settings.title')}
        >
          <SettingsIcon size={16} />
        </button>
        <button
          className="btn btn--icon btn--ghost"
          onClick={cycleTheme}
          title={t('titlebar.themeToggle.title', { theme: t(`common.theme.${theme}`) })}
          aria-label={t('titlebar.themeToggle.aria', { theme: t(`common.theme.${theme}`) })}
          id="btn-theme-toggle"
        >
          {theme === 'dark' && '🌙'}
          {theme === 'light' && '☀️'}
          {theme === 'hacker' && '💀'}
          {theme === 'nvim' && '⌨️'}
          {theme === 'min-dark' && '◼︎'}
        </button>
      </div>
    </header>
  )
}
