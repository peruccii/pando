const SESSION_CODE_CHARSET = 'ABCDEFGHJKMNPQRSTUVWXYZ23456789'

const SESSION_CODE_CURRENT_RE = new RegExp(`^[${SESSION_CODE_CHARSET}]{4}-[${SESSION_CODE_CHARSET}]{3}$`)
const SESSION_CODE_LEGACY_RE = new RegExp(`^[${SESSION_CODE_CHARSET}]{3}-[${SESSION_CODE_CHARSET}]{2}$`)
const SESSION_CODE_INPUT_SANITIZE_RE = new RegExp(`[^${SESSION_CODE_CHARSET}-]`, 'g')
const SESSION_CODE_COMPACT_SANITIZE_RE = new RegExp(`[^${SESSION_CODE_CHARSET}]`, 'g')

export function isSessionCodeReady(code: string): boolean {
  const normalized = code.trim().toUpperCase()
  return SESSION_CODE_CURRENT_RE.test(normalized) || SESSION_CODE_LEGACY_RE.test(normalized)
}

export function sanitizeSessionCodeInput(value: string): string {
  return value.toUpperCase().replace(SESSION_CODE_INPUT_SANITIZE_RE, '')
}

export function normalizeSessionCode(code: string): string {
  const cleaned = code.trim().toUpperCase().replace(SESSION_CODE_COMPACT_SANITIZE_RE, '')
  if (cleaned.length <= 4) {
    return cleaned
  }
  if (cleaned.length === 5) {
    return `${cleaned.slice(0, 3)}-${cleaned.slice(3, 5)}`
  }
  return `${cleaned.slice(0, 4)}-${cleaned.slice(4, 7)}`
}
