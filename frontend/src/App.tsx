import { useEffect, useCallback } from 'react'
import { useState } from 'react'
import { useWailsEvents } from './hooks/useWailsEvents'
import { useKeyboardShortcuts } from './hooks/useKeyboardShortcuts'
import { useStackBuildEvents } from './hooks/useStackBuildEvents'
import { useI18n } from './hooks/useI18n'
import { useAppStore } from './stores/appStore'
import { Titlebar } from './components/Titlebar'
import { EmptyState } from './components/EmptyState'
import { CommandPalette } from './components/CommandPalette'
import { NewTerminalDialog } from './components/NewTerminalDialog'
import { Settings } from './components/Settings'
import { TabBar } from './components/TabBar'
import { OnboardingWizard } from './components/OnboardingWizard'
import { ErrorBoundary } from './components/ErrorBoundary'
import './components/ErrorBoundary.css'
import { CommandCenter, useLayout } from './features/command-center'
import { useLayoutStore } from './features/command-center/stores/layoutStore'
import { BroadcastBar, useBroadcastStore } from './features/broadcast'
import { GitActivityPanel, useGitActivity } from './features/git-activity'
import { useWorkspaceStore } from './stores/workspaceStore'
import { SessionPanel, JoinSessionDialog } from './features/session'
import { GitPanelScreen } from './features/git-panel/components/GitPanelScreen'

export function App() {
    useWailsEvents()
    useKeyboardShortcuts()
    useStackBuildEvents()
    useGitActivity()

    const isReady = useAppStore((s) => s.isReady)
    const version = useAppStore((s) => s.version)
    const onboardingCompleted = useAppStore((s) => s.onboardingCompleted)
    const { hasPanes } = useLayout()
    const resetLayout = useLayoutStore((s) => s.reset)
    const loadWorkspaces = useWorkspaceStore((s) => s.loadWorkspaces)
    const { t } = useI18n()
    const isBroadcastActive = useBroadcastStore((s) => s.isActive)
    const [isSessionPanelOpen, setIsSessionPanelOpen] = useState(false)
    const [isJoinDialogOpen, setIsJoinDialogOpen] = useState(false)
    const [isGitPanelOpen, setIsGitPanelOpen] = useState(false)

    /** Quando o CommandCenter crashar, resetar o layout para estado limpo */
    const handleLayoutError = useCallback(() => {
        console.warn('[App] CommandCenter crashed, resetting layout')
        resetLayout()
    }, [resetLayout])

    // Carregar workspaces e sincronizar o layout do workspace ativo
    useEffect(() => {
        if (isReady) {
            loadWorkspaces().catch((err) => {
                console.error('[App] Failed to load workspaces:', err)
            })
        }
    }, [isReady, loadWorkspaces])

    // Escutar evento de shutdown para salvar sessões de terminal
    useEffect(() => {
        if (window.runtime) {
            const off = window.runtime.EventsOn('app:before-shutdown', async () => {
                console.log('[App] Capturing terminal snapshots before shutdown...')
                const snapshots = useLayoutStore.getState().captureTerminalSnapshots()
                if (snapshots.length > 0) {
                    try {
                        await window.go?.main?.App?.SaveTerminalSnapshots(snapshots)
                    } catch (err) {
                        console.error('[App] Failed to save terminal snapshots:', err)
                    }
                }
            })
            return () => off()
        }
    }, [])

    useEffect(() => {
        const onToggleSessionPanel = () => setIsSessionPanelOpen((prev) => !prev)
        const onOpenSessionPanel = () => setIsSessionPanelOpen(true)
        const onToggleJoin = () => setIsJoinDialogOpen((prev) => !prev)
        const onOpenJoin = () => setIsJoinDialogOpen(true)

        window.addEventListener('session:panel:toggle', onToggleSessionPanel)
        window.addEventListener('session:panel:open', onOpenSessionPanel)
        window.addEventListener('session:join:toggle', onToggleJoin)
        window.addEventListener('session:join:open', onOpenJoin)

        return () => {
            window.removeEventListener('session:panel:toggle', onToggleSessionPanel)
            window.removeEventListener('session:panel:open', onOpenSessionPanel)
            window.removeEventListener('session:join:toggle', onToggleJoin)
            window.removeEventListener('session:join:open', onOpenJoin)
        }
    }, [])

    useEffect(() => {
        const onOpenGitPanel = () => setIsGitPanelOpen(true)
        const onCloseGitPanel = () => setIsGitPanelOpen(false)
        const onToggleGitPanel = () => setIsGitPanelOpen((prev) => !prev)

        window.addEventListener('git-panel:open', onOpenGitPanel)
        window.addEventListener('git-panel:close', onCloseGitPanel)
        window.addEventListener('git-panel:toggle', onToggleGitPanel)

        return () => {
            window.removeEventListener('git-panel:open', onOpenGitPanel)
            window.removeEventListener('git-panel:close', onCloseGitPanel)
            window.removeEventListener('git-panel:toggle', onToggleGitPanel)
        }
    }, [])

    if (!isReady) {
        return (
            <div className="app-loading">
                <div className="app-loading__spinner" />
                <span className="app-loading__text">{t('app.loading')}</span>
            </div>
        )
    }

    return (
        <div className={`app ${isBroadcastActive ? 'app--broadcast-active' : ''} ${isGitPanelOpen ? 'app--git-open' : ''}`}>
            <Titlebar />
            <TabBar />
            <main className="app__main">
                <div
                    className={`app__main-content ${isGitPanelOpen ? 'app__main-content--hidden' : ''}`}
                    aria-hidden={isGitPanelOpen}
                >
                    {hasPanes ? (
                        <ErrorBoundary onReset={handleLayoutError}>
                            <CommandCenter />
                        </ErrorBoundary>
                    ) : (
                        <EmptyState version={version} />
                    )}
                </div>

                {isGitPanelOpen && (
                    <div className="app__git-overlay" role="dialog" aria-modal="true">
                        <GitPanelScreen onBack={() => setIsGitPanelOpen(false)} />
                    </div>
                )}
            </main>

            {isBroadcastActive && <BroadcastBar />}

            {/* Command Palette (global overlay) */}
            <CommandPalette />

            <NewTerminalDialog />

            <GitActivityPanel />

            {/* Settings Modal (global overlay) */}
            <Settings />

            {/* Onboarding Wizard (first run) */}
            <OnboardingWizard isOpen={!onboardingCompleted} />

            {isSessionPanelOpen && (
                <div className="app-session-overlay" onClick={() => setIsSessionPanelOpen(false)}>
                    <aside
                        className="app-session-overlay__panel animate-fade-in-up"
                        onClick={(e) => e.stopPropagation()}
                    >
                        <SessionPanel />
                    </aside>
                </div>
            )}

            <JoinSessionDialog
                isOpen={isJoinDialogOpen}
                onClose={() => setIsJoinDialogOpen(false)}
            />
        </div>
    )
}
