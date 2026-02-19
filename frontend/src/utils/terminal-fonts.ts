import { DEFAULT_TERMINAL_FONT_FAMILY } from '../stores/appStore'

const TERMINAL_FONT_FALLBACKS = [
  DEFAULT_TERMINAL_FONT_FAMILY,
  'SF Mono',
  'Menlo',
  'Monaco',
  'Fira Code',
  'Consolas',
  'Courier New',
  'monospace',
]

const quoteCssFontFamily = (family: string): string => {
  if (family.toLowerCase() === 'monospace') {
    return 'monospace'
  }

  const escaped = family.replace(/\\/g, '\\\\').replace(/'/g, "\\'")
  return `'${escaped}'`
}

export const buildTerminalFontStack = (selectedFamily: string): string => {
  const stack = [selectedFamily, ...TERMINAL_FONT_FALLBACKS]
  const seen = new Set<string>()
  const normalized = stack
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
    .filter((item) => {
      const key = item.toLowerCase()
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })

  return normalized.map(quoteCssFontFamily).join(', ')
}
