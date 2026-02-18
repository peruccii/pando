import React, { useCallback } from 'react'
import type { ScrollSyncSettings } from '../types'

interface ScrollSyncSettingsPanelProps {
  settings: ScrollSyncSettings
  onUpdate: (settings: ScrollSyncSettings) => void
}

export const ScrollSyncSettingsPanel: React.FC<ScrollSyncSettingsPanelProps> = ({
  settings,
  onUpdate,
}) => {
  const handleToggleEnabled = useCallback(() => {
    onUpdate({ ...settings, enabled: !settings.enabled })
  }, [settings, onUpdate])

  const handleToggleAutoFollow = useCallback(() => {
    onUpdate({ ...settings, autoFollow: !settings.autoFollow })
  }, [settings, onUpdate])

  const handleToggleShowToast = useCallback(() => {
    onUpdate({ ...settings, showToast: !settings.showToast })
  }, [settings, onUpdate])

  return (
    <div className="scroll-sync-settings">
      <h4 className="scroll-sync-settings__title">Scroll Sync</h4>
      
      <div className="scroll-sync-settings__item">
        <label className="scroll-sync-settings__label">
          <input
            type="checkbox"
            checked={settings.enabled}
            onChange={handleToggleEnabled}
            className="scroll-sync-settings__checkbox"
          />
          <span className="scroll-sync-settings__text">Habilitar sincronização</span>
        </label>
        <p className="scroll-sync-settings__desc">
          Sincroniza a visualização do diff quando outros usuários comentam
        </p>
      </div>

      <div className="scroll-sync-settings__item">
        <label className={`scroll-sync-settings__label ${!settings.enabled ? 'scroll-sync-settings__label--disabled' : ''}`}>
          <input
            type="checkbox"
            checked={settings.autoFollow}
            onChange={handleToggleAutoFollow}
            disabled={!settings.enabled}
            className="scroll-sync-settings__checkbox"
          />
          <span className="scroll-sync-settings__text">Seguir automaticamente</span>
        </label>
        <p className="scroll-sync-settings__desc">
          Navega automaticamente para a posição do outro usuário
        </p>
      </div>

      <div className="scroll-sync-settings__item">
        <label className={`scroll-sync-settings__label ${!settings.enabled ? 'scroll-sync-settings__label--disabled' : ''}`}>
          <input
            type="checkbox"
            checked={settings.showToast}
            onChange={handleToggleShowToast}
            disabled={!settings.enabled}
            className="scroll-sync-settings__checkbox"
          />
          <span className="scroll-sync-settings__text">Mostrar notificações</span>
        </label>
        <p className="scroll-sync-settings__desc">
          Exibe toast quando outros usuários navegam no diff
        </p>
      </div>
    </div>
  )
}
