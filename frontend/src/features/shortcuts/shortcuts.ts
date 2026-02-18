export type ShortcutCategory = 'panel' | 'navigation' | 'collaboration' | 'general'

export type ShortcutId =
  | 'newTerminal'
  | 'closePane'
  | 'zenMode'
  | 'focusNextPane'
  | 'focusPrevPane'
  | 'focusPane1'
  | 'focusPane2'
  | 'focusPane3'
  | 'focusPane4'
  | 'focusPane5'
  | 'focusPane6'
  | 'focusPane7'
  | 'focusPane8'
  | 'focusPane9'
  | 'splitVertical'
  | 'splitHorizontal'
  | 'commandPalette'
  | 'toggleTheme'
  | 'toggleSidebar'
  | 'toggleGitActivity'
  | 'toggleBroadcast'
  | 'openSettings'

export interface ShortcutBinding {
  key: string
  meta: boolean
  shift: boolean
  alt: boolean
}

export interface ShortcutDefinition {
  id: ShortcutId
  description: string
  category: ShortcutCategory
  defaultBinding: ShortcutBinding
}

export type ShortcutBindingOverrides = Partial<Record<ShortcutId, ShortcutBinding>>
export type ShortcutResolvedBindings = Record<ShortcutId, ShortcutBinding>

const SPECIAL_KEYS = new Set([
  'Enter',
  'Tab',
  'Escape',
  'Space',
  'Backspace',
  'Delete',
  'Home',
  'End',
  'PageUp',
  'PageDown',
  'ArrowUp',
  'ArrowDown',
  'ArrowLeft',
  'ArrowRight',
])

const ALIAS_KEYS: Record<string, string> = {
  esc: 'Escape',
  escape: 'Escape',
  return: 'Enter',
  spacebar: 'Space',
  space: 'Space',
  del: 'Delete',
}

const KEY_FROM_CODE: Record<string, string> = {
  Backquote: '`',
  Minus: '-',
  Equal: '=',
  BracketLeft: '[',
  BracketRight: ']',
  Backslash: '\\',
  Semicolon: ';',
  Quote: "'",
  Comma: ',',
  Period: '.',
  Slash: '/',
}

for (let i = 0; i <= 9; i += 1) {
  KEY_FROM_CODE[`Digit${i}`] = String(i)
}

for (let i = 1; i <= 9; i += 1) {
  KEY_FROM_CODE[`Numpad${i}`] = String(i)
}
KEY_FROM_CODE.Numpad0 = '0'

for (let i = 0; i < 26; i += 1) {
  const letter = String.fromCharCode(97 + i)
  KEY_FROM_CODE[`Key${letter.toUpperCase()}`] = letter
}

export const SHORTCUT_DEFINITIONS: ShortcutDefinition[] = [
  { id: 'newTerminal', description: 'Novo Terminal', category: 'panel', defaultBinding: { key: 'n', meta: true, shift: false, alt: false } },
  { id: 'closePane', description: 'Fechar Painel', category: 'panel', defaultBinding: { key: 'w', meta: true, shift: false, alt: false } },
  { id: 'zenMode', description: 'Zen Mode (Maximizar)', category: 'panel', defaultBinding: { key: 'Enter', meta: true, shift: false, alt: false } },
  { id: 'focusNextPane', description: 'Próximo Painel', category: 'panel', defaultBinding: { key: ']', meta: true, shift: false, alt: false } },
  { id: 'focusPrevPane', description: 'Painel Anterior', category: 'panel', defaultBinding: { key: '[', meta: true, shift: false, alt: false } },
  { id: 'focusPane1', description: 'Focar Painel 1', category: 'panel', defaultBinding: { key: '1', meta: true, shift: false, alt: false } },
  { id: 'focusPane2', description: 'Focar Painel 2', category: 'panel', defaultBinding: { key: '2', meta: true, shift: false, alt: false } },
  { id: 'focusPane3', description: 'Focar Painel 3', category: 'panel', defaultBinding: { key: '3', meta: true, shift: false, alt: false } },
  { id: 'focusPane4', description: 'Focar Painel 4', category: 'panel', defaultBinding: { key: '4', meta: true, shift: false, alt: false } },
  { id: 'focusPane5', description: 'Focar Painel 5', category: 'panel', defaultBinding: { key: '5', meta: true, shift: false, alt: false } },
  { id: 'focusPane6', description: 'Focar Painel 6', category: 'panel', defaultBinding: { key: '6', meta: true, shift: false, alt: false } },
  { id: 'focusPane7', description: 'Focar Painel 7', category: 'panel', defaultBinding: { key: '7', meta: true, shift: false, alt: false } },
  { id: 'focusPane8', description: 'Focar Painel 8', category: 'panel', defaultBinding: { key: '8', meta: true, shift: false, alt: false } },
  { id: 'focusPane9', description: 'Focar Painel 9', category: 'panel', defaultBinding: { key: '9', meta: true, shift: false, alt: false } },
  { id: 'splitVertical', description: 'Split Vertical', category: 'panel', defaultBinding: { key: '\\', meta: true, shift: false, alt: false } },
  { id: 'splitHorizontal', description: 'Split Horizontal', category: 'panel', defaultBinding: { key: '\\', meta: true, shift: true, alt: false } },
  { id: 'commandPalette', description: 'Command Palette', category: 'navigation', defaultBinding: { key: 'k', meta: true, shift: false, alt: false } },
  { id: 'toggleTheme', description: 'Alternar Tema', category: 'general', defaultBinding: { key: 'd', meta: true, shift: true, alt: false } },
  { id: 'toggleSidebar', description: 'Toggle Sidebar', category: 'navigation', defaultBinding: { key: 'b', meta: true, shift: false, alt: false } },
  { id: 'toggleGitActivity', description: 'Toggle Git Activity', category: 'navigation', defaultBinding: { key: 'g', meta: true, shift: true, alt: false } },
  { id: 'toggleBroadcast', description: 'Toggle Broadcast Mode', category: 'collaboration', defaultBinding: { key: 'b', meta: true, shift: true, alt: false } },
  { id: 'openSettings', description: 'Abrir Settings', category: 'general', defaultBinding: { key: ',', meta: true, shift: false, alt: false } },
]

const SHORTCUT_IDS = new Set<ShortcutId>(SHORTCUT_DEFINITIONS.map((def) => def.id))

export const RESERVED_SHORTCUTS = [
  { id: 'terminalZoomIn', description: 'Aumentar Zoom do Terminal', test: (binding: ShortcutBinding) => binding.meta && !binding.alt && binding.key === '=' },
  { id: 'terminalZoomOut', description: 'Diminuir Zoom do Terminal', test: (binding: ShortcutBinding) => binding.meta && !binding.alt && binding.key === '-' },
  { id: 'terminalZoomReset', description: 'Resetar Zoom do Terminal', test: (binding: ShortcutBinding) => binding.meta && !binding.alt && binding.key === '0' && !binding.shift },
] as const

function normalizeKey(key: string): string | null {
  const trimmed = key.trim()
  if (!trimmed) return null

  if (trimmed.length === 1) {
    return trimmed.toLowerCase()
  }

  const lower = trimmed.toLowerCase()
  if (ALIAS_KEYS[lower]) {
    return ALIAS_KEYS[lower]
  }

  if (SPECIAL_KEYS.has(trimmed)) {
    return trimmed
  }

  if (/^F(?:[1-9]|1[0-2])$/.test(trimmed.toUpperCase())) {
    return trimmed.toUpperCase()
  }

  return null
}

export function normalizeShortcutBinding(value: Partial<ShortcutBinding> | null | undefined): ShortcutBinding | null {
  if (!value || typeof value.key !== 'string') return null

  const normalizedKey = normalizeKey(value.key)
  if (!normalizedKey) return null

  return {
    key: normalizedKey,
    meta: Boolean(value.meta),
    shift: Boolean(value.shift),
    alt: Boolean(value.alt),
  }
}

export function getShortcutSignature(binding: ShortcutBinding): string {
  return `${binding.meta ? '1' : '0'}:${binding.shift ? '1' : '0'}:${binding.alt ? '1' : '0'}:${binding.key}`
}

function getDefaultBindings(): ShortcutResolvedBindings {
  return SHORTCUT_DEFINITIONS.reduce((acc, def) => {
    acc[def.id] = { ...def.defaultBinding }
    return acc
  }, {} as ShortcutResolvedBindings)
}

export function resolveShortcutBindings(overrides: ShortcutBindingOverrides = {}): ShortcutResolvedBindings {
  const bindings = getDefaultBindings()
  for (const [id, rawBinding] of Object.entries(overrides)) {
    if (!SHORTCUT_IDS.has(id as ShortcutId)) continue
    const normalized = normalizeShortcutBinding(rawBinding)
    if (!normalized) continue
    bindings[id as ShortcutId] = normalized
  }
  return bindings
}

export function parseShortcutBindingsJSON(raw: string | undefined | null): ShortcutBindingOverrides {
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw) as Record<string, Partial<ShortcutBinding>>
    if (!parsed || typeof parsed !== 'object') return {}

    const parsedOverrides: ShortcutBindingOverrides = {}
    for (const [id, binding] of Object.entries(parsed)) {
      if (!SHORTCUT_IDS.has(id as ShortcutId)) continue
      const normalized = normalizeShortcutBinding(binding)
      if (!normalized) continue
      parsedOverrides[id as ShortcutId] = normalized
    }

    const resolved = getDefaultBindings()
    for (const definition of SHORTCUT_DEFINITIONS) {
      const override = parsedOverrides[definition.id]
      if (!override) continue

      const isReserved = RESERVED_SHORTCUTS.some((reserved) => reserved.test(override))
      if (isReserved) continue

      const nextSignature = getShortcutSignature(override)
      const hasConflict = SHORTCUT_DEFINITIONS.some((otherDefinition) => {
        if (otherDefinition.id === definition.id) return false
        return getShortcutSignature(resolved[otherDefinition.id]) === nextSignature
      })
      if (hasConflict) continue

      resolved[definition.id] = override
    }

    const safeOverrides: ShortcutBindingOverrides = {}
    for (const definition of SHORTCUT_DEFINITIONS) {
      const resolvedSignature = getShortcutSignature(resolved[definition.id])
      const defaultSignature = getShortcutSignature(definition.defaultBinding)
      if (resolvedSignature !== defaultSignature) {
        safeOverrides[definition.id] = resolved[definition.id]
      }
    }

    return safeOverrides
  } catch {
    return {}
  }
}

export function serializeShortcutBindingsJSON(overrides: ShortcutBindingOverrides): string {
  return JSON.stringify(overrides)
}

export function toShortcutOverride(
  currentOverrides: ShortcutBindingOverrides,
  id: ShortcutId,
  binding: ShortcutBinding,
): ShortcutBindingOverrides {
  const normalized = normalizeShortcutBinding(binding)
  if (!normalized) return currentOverrides

  const isReserved = RESERVED_SHORTCUTS.some((reserved) => reserved.test(normalized))
  if (isReserved) return currentOverrides

  const resolved = resolveShortcutBindings(currentOverrides)
  const nextSignature = getShortcutSignature(normalized)
  for (const definition of SHORTCUT_DEFINITIONS) {
    if (definition.id === id) continue
    if (getShortcutSignature(resolved[definition.id]) === nextSignature) {
      return currentOverrides
    }
  }

  const definition = SHORTCUT_DEFINITIONS.find((def) => def.id === id)
  if (!definition) return currentOverrides

  const defaultSignature = getShortcutSignature(definition.defaultBinding)
  const next: ShortcutBindingOverrides = { ...currentOverrides }

  if (getShortcutSignature(normalized) === defaultSignature) {
    delete next[id]
  } else {
    next[id] = normalized
  }

  return next
}

export function formatShortcutBinding(binding: ShortcutBinding): string {
  const modifiers: string[] = []
  if (binding.meta) modifiers.push('⌘')
  if (binding.shift) modifiers.push('⇧')
  if (binding.alt) modifiers.push('⌥')

  const keyLabel = formatShortcutKey(binding.key)
  if (modifiers.length === 0) return keyLabel
  return `${modifiers.join('')} ${keyLabel}`
}

function formatShortcutKey(key: string): string {
  switch (key) {
    case 'Enter': return 'Enter'
    case 'Tab': return 'Tab'
    case 'Escape': return 'Esc'
    case 'Space': return 'Space'
    case 'ArrowUp': return '↑'
    case 'ArrowDown': return '↓'
    case 'ArrowLeft': return '←'
    case 'ArrowRight': return '→'
    default:
      return key.length === 1 ? key.toUpperCase() : key
  }
}

function normalizeEventKey(event: Pick<KeyboardEvent, 'key' | 'code'>): string | null {
  if (KEY_FROM_CODE[event.code]) {
    return KEY_FROM_CODE[event.code]
  }
  return normalizeKey(event.key)
}

export function getBindingFromKeyboardEvent(event: Pick<KeyboardEvent, 'key' | 'code' | 'metaKey' | 'ctrlKey' | 'shiftKey' | 'altKey'>): ShortcutBinding | null {
  const key = normalizeEventKey(event)
  if (!key) return null

  return {
    key,
    meta: event.metaKey || event.ctrlKey,
    shift: event.shiftKey,
    alt: event.altKey,
  }
}

export function eventMatchesShortcutBinding(event: Pick<KeyboardEvent, 'key' | 'code' | 'metaKey' | 'ctrlKey' | 'shiftKey' | 'altKey'>, binding: ShortcutBinding): boolean {
  const key = normalizeEventKey(event)
  if (!key) return false

  const metaMatch = binding.meta ? (event.metaKey || event.ctrlKey) : !(event.metaKey || event.ctrlKey)
  const shiftMatch = binding.shift ? event.shiftKey : !event.shiftKey
  const altMatch = binding.alt ? event.altKey : !event.altKey

  return metaMatch && shiftMatch && altMatch && key === binding.key
}

export function getShortcutConflict(
  targetID: ShortcutId,
  nextBinding: ShortcutBinding,
  resolvedBindings: ShortcutResolvedBindings,
): { id: ShortcutId; description: string } | null {
  const signature = getShortcutSignature(nextBinding)
  for (const def of SHORTCUT_DEFINITIONS) {
    if (def.id === targetID) continue
    const binding = resolvedBindings[def.id]
    if (getShortcutSignature(binding) === signature) {
      return { id: def.id, description: def.description }
    }
  }
  return null
}

export function getReservedConflict(binding: ShortcutBinding): { id: string; description: string } | null {
  const conflict = RESERVED_SHORTCUTS.find((reserved) => reserved.test(binding))
  if (!conflict) return null
  return { id: conflict.id, description: conflict.description }
}

export function isShortcutID(value: string): value is ShortcutId {
  return SHORTCUT_IDS.has(value as ShortcutId)
}
