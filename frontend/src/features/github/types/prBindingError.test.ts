import { describe, expect, it } from 'vitest'
import { PR_ERROR_CODES, parsePRBindingError } from './prBindingError'

describe('parsePRBindingError', () => {
  it('parses binding error from plain object payload', () => {
    const parsed = parsePRBindingError({
      code: PR_ERROR_CODES.validationFailed,
      message: 'Payload invalido para operacao de Pull Request.',
      details: 'Campo "head" deve ser preenchido.',
    })

    expect(parsed).toEqual({
      code: PR_ERROR_CODES.validationFailed,
      message: 'Payload invalido para operacao de Pull Request.',
      details: 'Campo "head" deve ser preenchido.',
    })
  })

  it('parses binding error from rejected string JSON payload', () => {
    const parsed = parsePRBindingError(
      '{"code":"E_PR_UNAUTHORIZED","message":"Sessao GitHub invalida ou expirada.","details":"GitHub token expired or invalid (type=auth)"}',
    )

    expect(parsed).toEqual({
      code: PR_ERROR_CODES.unauthorized,
      message: 'Sessao GitHub invalida ou expirada.',
      details: 'GitHub token expired or invalid (type=auth)',
    })
  })

  it('parses binding error from Error with JSON message', () => {
    const parsed = parsePRBindingError(new Error(
      '{"code":"E_PR_VALIDATION_FAILED","message":"Payload invalido para operacao de Pull Request.","details":"Validation failed: No commits between base and head (type=validation)"}',
    ))

    expect(parsed).toEqual({
      code: PR_ERROR_CODES.validationFailed,
      message: 'Payload invalido para operacao de Pull Request.',
      details: 'Validation failed: No commits between base and head (type=validation)',
    })
  })

  it('parses binding error from wrapped cause payload', () => {
    const parsed = parsePRBindingError({
      message: 'Falha ao executar operacao de Pull Requests.',
      cause: {
        code: PR_ERROR_CODES.forbidden,
        message: 'Operacao bloqueada por permissao ou rate limit.',
        details: 'Permission denied: missing pull_requests:write scope (type=permission)',
      },
    })

    expect(parsed).toEqual({
      code: PR_ERROR_CODES.forbidden,
      message: 'Operacao bloqueada por permissao ou rate limit.',
      details: 'Permission denied: missing pull_requests:write scope (type=permission)',
    })
  })

  it('returns unknown error for plain message when payload is not structured', () => {
    const parsed = parsePRBindingError('Falha inesperada de rede')

    expect(parsed).toEqual({
      code: PR_ERROR_CODES.unknown,
      message: 'Falha inesperada de rede',
      details: 'Falha inesperada de rede',
    })
  })
})
