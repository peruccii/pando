export interface PRBindingError {
  code: string
  message: string
  details?: string
}

export const PR_ERROR_CODES = {
  serviceUnavailable: 'E_PR_SERVICE_UNAVAILABLE',
  repoPathRequired: 'E_PR_REPO_PATH_REQUIRED',
  repoUnavailable: 'E_PR_REPO_UNAVAILABLE',
  repoResolveFailed: 'E_PR_REPO_RESOLVE_FAILED',
  manualRepoInvalid: 'E_PR_MANUAL_REPO_INVALID',
  repoTargetMismatch: 'E_PR_REPO_TARGET_MISMATCH',
  unauthorized: 'E_PR_UNAUTHORIZED',
  forbidden: 'E_PR_FORBIDDEN',
  notFound: 'E_PR_NOT_FOUND',
  conflict: 'E_PR_CONFLICT',
  validationFailed: 'E_PR_VALIDATION_FAILED',
  rateLimited: 'E_PR_RATE_LIMITED',
  unknown: 'E_PR_UNKNOWN',
} as const

const DEFAULT_ERROR: PRBindingError = {
  code: PR_ERROR_CODES.unknown,
  message: 'Falha ao executar operacao de Pull Requests.',
}

function isPRBindingError(value: unknown): value is PRBindingError {
  if (!value || typeof value !== 'object') {
    return false
  }
  const candidate = value as Partial<PRBindingError>
  return typeof candidate.code === 'string' && typeof candidate.message === 'string'
}

function parsePRBindingErrorFromRaw(raw: string): PRBindingError | null {
  const trimmed = raw.trim()
  if (!trimmed.startsWith('{') || !trimmed.endsWith('}')) {
    return null
  }

  try {
    const parsed = JSON.parse(trimmed)
    if (!isPRBindingError(parsed)) {
      return null
    }
    return {
      code: parsed.code.trim() || PR_ERROR_CODES.unknown,
      message: parsed.message.trim() || DEFAULT_ERROR.message,
      details: typeof parsed.details === 'string' ? parsed.details : undefined,
    }
  } catch {
    return null
  }
}

function parsePRBindingErrorFromMessage(raw: string): PRBindingError | null {
  const fromJSON = parsePRBindingErrorFromRaw(raw)
  if (fromJSON) {
    return fromJSON
  }

  const trimmed = raw.trim()
  if (!trimmed) {
    return null
  }

  return {
    code: PR_ERROR_CODES.unknown,
    message: trimmed || DEFAULT_ERROR.message,
    details: trimmed || undefined,
  }
}

function parsePRBindingErrorFromObject(input: unknown): PRBindingError | null {
  if (!input || typeof input !== 'object') {
    return null
  }

  if (isPRBindingError(input)) {
    return {
      code: input.code.trim() || PR_ERROR_CODES.unknown,
      message: input.message.trim() || DEFAULT_ERROR.message,
      details: typeof input.details === 'string' ? input.details : undefined,
    }
  }

  const candidate = input as Record<string, unknown>
  const nestedCandidates = [
    candidate.error,
    candidate.err,
    candidate.cause,
  ]
  for (const nested of nestedCandidates) {
    if (nested === input) {
      continue
    }
    if (isPRBindingError(nested)) {
      return {
        code: nested.code.trim() || PR_ERROR_CODES.unknown,
        message: nested.message.trim() || DEFAULT_ERROR.message,
        details: typeof nested.details === 'string' ? nested.details : undefined,
      }
    }
    if (typeof nested === 'string') {
      const parsedNestedMessage = parsePRBindingErrorFromMessage(nested)
      if (parsedNestedMessage) {
        return parsedNestedMessage
      }
    }
    if (nested instanceof Error) {
      const parsedNestedError = parsePRBindingErrorFromMessage(nested.message)
      if (parsedNestedError) {
        return parsedNestedError
      }
    }
    if (nested && typeof nested === 'object') {
      const nestedCandidate = nested as Record<string, unknown>
      if (typeof nestedCandidate.message === 'string') {
        const parsedNestedMessage = parsePRBindingErrorFromMessage(nestedCandidate.message)
        if (parsedNestedMessage) {
          return parsedNestedMessage
        }
      }
    }
  }

  if (typeof candidate.message === 'string') {
    const parsedMessage = parsePRBindingErrorFromMessage(candidate.message)
    if (parsedMessage) {
      return parsedMessage
    }
  }

  return null
}

export function parsePRBindingError(input: unknown): PRBindingError {
  const parsedObject = parsePRBindingErrorFromObject(input)
  if (parsedObject) {
    return parsedObject
  }

  if (typeof input === 'string') {
    const parsedString = parsePRBindingErrorFromMessage(input)
    if (parsedString) {
      return parsedString
    }
  }

  if (input instanceof Error) {
    const parsedError = parsePRBindingErrorFromMessage(input.message)
    if (parsedError) {
      return parsedError
    }
  }

  return DEFAULT_ERROR
}
