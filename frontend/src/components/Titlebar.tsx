import { useState, useEffect, useRef } from 'react'
import { Clock3, Settings as SettingsIcon, Users, UserPlus, ChevronDown, Share2 } from 'lucide-react'
import { useAppStore } from '../stores/appStore'
import { useStackBuildStore } from '../stores/stackBuildStore'
import { useI18n } from '../hooks/useI18n'
import { useGitActivityStore } from '../features/git-activity/stores/gitActivityStore'
import { useSessionStore } from '../features/session/stores/sessionStore'
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
  const session = useSessionStore((s) => s.session)
  const role = useSessionStore((s) => s.role)
  const isWaitingApproval = useSessionStore((s) => s.isWaitingApproval)
  const joinResult = useSessionStore((s) => s.joinResult)
  const { t } = useI18n()

  const [asciiFrame, setAsciiFrame] = useState(0)
  const [approvalCountdown, setApprovalCountdown] = useState<string>('5:00')
  const [isCollabOpen, setIsCollabOpen] = useState(false)
  const collabMenuRef = useRef<HTMLDivElement>(null)

  const asciiFrames = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"]

  useEffect(() => {
    if (!isBuilding) return;
    const frameTimer = setInterval(() => {
      setAsciiFrame(prev => (prev + 1) % asciiFrames.length);
    }, 80);
    return () => clearInterval(frameTimer);
  }, [isBuilding]);

  useEffect(() => {
    if (role !== 'guest' || !isWaitingApproval) {
      setApprovalCountdown('5:00')
      return
    }

    const parsedDeadline = Date.parse(joinResult?.approvalExpiresAt || '')
    const targetDeadline = Number.isNaN(parsedDeadline)
      ? Date.now() + 5 * 60 * 1000
      : parsedDeadline

    const tick = () => {
      const seconds = Math.max(0, Math.ceil((targetDeadline - Date.now()) / 1000))
      const mins = Math.floor(seconds / 60)
      const secs = seconds % 60
      setApprovalCountdown(`${mins}:${secs.toString().padStart(2, '0')}`)
    }

    tick()
    const timer = window.setInterval(tick, 1000)

    return () => {
      window.clearInterval(timer)
    }
  }, [isWaitingApproval, joinResult?.approvalExpiresAt, role])

  // Close dropdown on outside click
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (collabMenuRef.current && !collabMenuRef.current.contains(event.target as Node)) {
        setIsCollabOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const formatTime = (seconds: number) => {
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins}:${secs.toString().padStart(2, "0")}`;
  };

  const handleSettings = () => {
    window.dispatchEvent(new CustomEvent('settings:toggle'))
  }

  const openSessionPanel = () => {
    window.dispatchEvent(new CustomEvent('session:panel:toggle'))
    setIsCollabOpen(false)
  }

  const openJoinDialog = () => {
    window.dispatchEvent(new CustomEvent('session:join:open'))
    setIsCollabOpen(false)
  }

  const cycleTheme = () => {
    const themes = ['dark', 'light', 'hacker', 'nvim', 'min-dark'] as const
    const currentIndex = themes.indexOf(theme)
    const nextIndex = (currentIndex + 1) % themes.length
    setTheme(themes[nextIndex])
  }

  const isSessionActive = Boolean(session && (session.status === 'waiting' || session.status === 'active'))
  const sessionLabel = role === 'host' ? 'Host' : role === 'guest' ? 'Guest' : 'Collab'
  const showApprovalIndicator = isWaitingApproval

  return (
    <header className="titlebar" id="titlebar">
      <div className="titlebar__actions">
        <div className="titlebar-collab-group" ref={collabMenuRef}>
          <button
            className={`titlebar-collab-trigger ${isSessionActive ? 'titlebar-collab-trigger--active' : ''} ${isCollabOpen ? 'titlebar-collab-trigger--open' : ''}`}
            onClick={() => setIsCollabOpen(!isCollabOpen)}
            aria-label="Collaboration options"
          >
            <Users size={13} />
            <span>{isSessionActive ? sessionLabel : 'Collab'}</span>
            <ChevronDown size={12} className={`trigger-chevron ${isCollabOpen ? 'rotated' : ''}`} />
          </button>

          {isCollabOpen && (
            <div className="titlebar-collab-dropdown">
              <button className="collab-dropdown-item" onClick={openSessionPanel}>
                <div className="collab-dropdown-item__icon collab-dropdown-item__icon--host">
                  <Share2 size={14} />
                </div>
                <div className="collab-dropdown-item__text">
                  <span className="collab-dropdown-item__title">Ativar Colaboração</span>
                  <span className="collab-dropdown-item__desc">Compartilhe seu terminal com outros</span>
                </div>
              </button>
              
              <button className="collab-dropdown-item" onClick={openJoinDialog}>
                <div className="collab-dropdown-item__icon collab-dropdown-item__icon--join">
                  <UserPlus size={14} />
                </div>
                <div className="collab-dropdown-item__text">
                  <span className="collab-dropdown-item__title">Entrar em Sessão</span>
                  <span className="collab-dropdown-item__desc">Conecte-se em um terminal remoto</span>
                </div>
              </button>
            </div>
          )}
        </div>

        {showApprovalIndicator && (
          <button
            className="titlebar-approval-btn"
            title={`Aguardando aprovação do host (${approvalCountdown})`}
            onClick={openJoinDialog}
            aria-label="Show waiting approval status"
          >
            <Clock3 size={12} />
            <span>Approval {approvalCountdown}</span>
          </button>
        )}
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
