import { useCallback } from 'react'
import { useAppStore } from '../stores/appStore'
import { translate } from '../i18n/translations'

export function useI18n() {
  const language = useAppStore((s) => s.language)

  const t = useCallback(
    (key: string, params?: Record<string, string | number>) => {
      return translate(language, key, params)
    },
    [language]
  )

  return { language, t }
}
