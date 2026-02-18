import { useEffect, useMemo, useState } from 'react'
import { useAppStore } from '../stores/appStore'
import { useLayout } from '../features/command-center'
import { useI18n } from '../hooks/useI18n'
import './OnboardingWizard.css'

interface OnboardingWizardProps {
  isOpen: boolean
}

type StepId = 'language' | 'theme' | 'launch'

const STEPS: StepId[] = ['language', 'theme', 'launch']

export function OnboardingWizard({ isOpen }: OnboardingWizardProps) {
  const [stepIndex, setStepIndex] = useState(0)
  const theme = useAppStore((s) => s.theme)
  const language = useAppStore((s) => s.language)
  const setTheme = useAppStore((s) => s.setTheme)
  const setLanguage = useAppStore((s) => s.setLanguage)
  const completeOnboarding = useAppStore((s) => s.completeOnboarding)
  const { hasPanes, newTerminal } = useLayout()
  const { t } = useI18n()

  useEffect(() => {
    if (!isOpen) {
      setStepIndex(0)
    }
  }, [isOpen])

  const stepId = STEPS[stepIndex]
  const isFirstStep = stepIndex === 0
  const isLastStep = stepIndex === STEPS.length - 1

  const themeOptions = useMemo(
    () => [
      { id: 'dark' as const, label: t('common.theme.dark') },
      { id: 'light' as const, label: t('common.theme.light') },
      { id: 'hacker' as const, label: t('common.theme.hacker') },
      { id: 'nvim' as const, label: t('common.theme.nvim') },
      { id: 'min-dark' as const, label: t('common.theme.min-dark') },
    ],
    [t]
  )

  const languageOptions = useMemo(
    () => [
      { id: 'pt-BR' as const, label: t('common.language.ptBR') },
      { id: 'en-US' as const, label: t('common.language.enUS') },
    ],
    [t]
  )

  const handleBack = () => {
    if (!isFirstStep) {
      setStepIndex((current) => current - 1)
    }
  }

  const handleSkip = () => {
    completeOnboarding()
  }

  const handleFinish = () => {
    if (!hasPanes) {
      newTerminal()
    }
    completeOnboarding()
  }

  const handleNext = () => {
    if (isLastStep) {
      handleFinish()
      return
    }
    setStepIndex((current) => current + 1)
  }

  if (!isOpen) return null

  return (
    <div className="onboarding-backdrop">
      <section
        className="onboarding-card"
        role="dialog"
        aria-modal="true"
        aria-label={t('onboarding.title')}
      >
        <header className="onboarding-header">
          <span className="onboarding-badge">{t('onboarding.badge')}</span>
          <h2 className="onboarding-title">{t('onboarding.title')}</h2>
          <p className="onboarding-subtitle">{t('onboarding.subtitle')}</p>
        </header>

        <div className="onboarding-progress">
          {STEPS.map((step, index) => (
            <div
              key={step}
              className={`onboarding-progress__dot ${index <= stepIndex ? 'onboarding-progress__dot--active' : ''}`}
            />
          ))}
        </div>

        <div className="onboarding-content">
          {stepId === 'language' && (
            <div className="onboarding-step">
              <h3>{t('onboarding.step.language.title')}</h3>
              <p>{t('onboarding.step.language.description')}</p>
              <div className="onboarding-options onboarding-options--language">
                {languageOptions.map((option) => (
                  <button
                    key={option.id}
                    className={`onboarding-option ${language === option.id ? 'onboarding-option--active' : ''}`}
                    onClick={() => setLanguage(option.id)}
                  >
                    {option.label}
                  </button>
                ))}
              </div>
            </div>
          )}

          {stepId === 'theme' && (
            <div className="onboarding-step">
              <h3>{t('onboarding.step.theme.title')}</h3>
              <p>{t('onboarding.step.theme.description')}</p>
              <div className="onboarding-options onboarding-options--theme">
                {themeOptions.map((option) => (
                  <button
                    key={option.id}
                    className={`onboarding-option ${theme === option.id ? 'onboarding-option--active' : ''}`}
                    onClick={() => setTheme(option.id)}
                  >
                    <span className={`onboarding-theme-preview onboarding-theme-preview--${option.id}`} />
                    <span>{option.label}</span>
                  </button>
                ))}
              </div>
            </div>
          )}

          {stepId === 'launch' && (
            <div className="onboarding-step">
              <h3>{t('onboarding.step.launch.title')}</h3>
              <p>{t('onboarding.step.launch.description')}</p>
              <ul className="onboarding-checklist">
                <li>{t('onboarding.step.launch.point1')}</li>
                <li>{t('onboarding.step.launch.point2')}</li>
                <li>{t('onboarding.step.launch.point3')}</li>
              </ul>
            </div>
          )}
        </div>

        <footer className="onboarding-footer">
          <div className="onboarding-footer__left">
            {!isFirstStep && (
              <button className="btn btn--ghost" onClick={handleBack}>
                {t('common.cta.back')}
              </button>
            )}
          </div>

          <div className="onboarding-footer__right">
            {!isLastStep && (
              <button className="btn btn--ghost" onClick={handleSkip}>
                {t('common.cta.skip')}
              </button>
            )}
            <button className="btn btn--primary" onClick={handleNext}>
              {isLastStep ? t('onboarding.cta.start') : t('common.cta.next')}
            </button>
          </div>
        </footer>
      </section>
    </div>
  )
}
