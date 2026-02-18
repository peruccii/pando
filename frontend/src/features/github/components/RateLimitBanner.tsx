import { useEffect, useState } from 'react'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import './RateLimitBanner.css'

interface RateLimitInfo {
  remaining: number
  limit: number
  resetAt: string
}

/**
 * Banner que aparece quando o rate limit do GitHub está baixo.
 * Mostra quantos requests restam e quando o rate limit será resetado.
 */
export function RateLimitBanner() {
  const [info, setInfo] = useState<RateLimitInfo | null>(null)
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    if (!window.runtime) return

    // Escutar eventos de rate limit
    const off = window.runtime.EventsOn('github:ratelimit', (data: RateLimitInfo) => {
      setInfo(data)
      setVisible(true)
    })

    return () => {
      off()
    }
  }, [])

  // Também verificar periodicamente via API
  useEffect(() => {
    const api = window.go?.main?.App
    if (!api) return

    const check = async () => {
      try {
        const rateLimitInfo = await api.GetRateLimitInfo()
        if (rateLimitInfo && rateLimitInfo.remaining < 200) {
          setInfo(rateLimitInfo)
          setVisible(true)
        } else {
          setVisible(false)
        }
      } catch {
        // Ignorar erro silenciosamente
      }
    }

    // Checar imediatamente e a cada 30s
    check()
    const interval = setInterval(check, 30000)
    return () => clearInterval(interval)
  }, [])

  if (!visible || !info) return null

  const resetDate = new Date(info.resetAt)
  const minutesUntilReset = Math.max(0, Math.ceil((resetDate.getTime() - Date.now()) / 60000))
  const percentage = Math.round((info.remaining / Math.max(info.limit, 1)) * 100)
  const isCritical = info.remaining < 100

  return (
    <div className={`rate-limit-banner ${isCritical ? 'critical' : 'warning'}`}>
      <div className="rate-limit-banner__icon">
        {isCritical ? <AlertTriangle size={16} /> : <RefreshCw size={16} />}
      </div>
      <div className="rate-limit-banner__content">
        <span className="rate-limit-banner__text">
          {isCritical
            ? `⚠️ Rate limit crítico (${info.remaining} restantes). Polling pausado.`
            : `Rate limit baixo (${info.remaining}/${info.limit}).`
          }
          {' '}Reset em {minutesUntilReset} min.
        </span>
        <div className="rate-limit-banner__bar">
          <div
            className="rate-limit-banner__bar-fill"
            style={{ width: `${percentage}%` }}
          />
        </div>
      </div>
      <button
        className="rate-limit-banner__dismiss"
        onClick={() => setVisible(false)}
        title="Fechar"
      >
        ×
      </button>
    </div>
  )
}
