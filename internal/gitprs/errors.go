package gitprs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	CodeServiceUnavailable = "E_PR_SERVICE_UNAVAILABLE"
	CodeRepoPathRequired   = "E_PR_REPO_PATH_REQUIRED"
	CodeRepoUnavailable    = "E_PR_REPO_UNAVAILABLE"
	CodeRepoResolveFailed  = "E_PR_REPO_RESOLVE_FAILED"
	CodeManualRepoInvalid  = "E_PR_MANUAL_REPO_INVALID"
	CodeRepoTargetMismatch = "E_PR_REPO_TARGET_MISMATCH"
	CodeUnauthorized       = "E_PR_UNAUTHORIZED"
	CodeForbidden          = "E_PR_FORBIDDEN"
	CodeNotFound           = "E_PR_NOT_FOUND"
	CodeConflict           = "E_PR_CONFLICT"
	CodeValidationFailed   = "E_PR_VALIDATION_FAILED"
	CodeRateLimited        = "E_PR_RATE_LIMITED"
	CodeUnknown            = "E_PR_UNKNOWN"
)

// BindingError implementa contrato normalizado de erro para bindings de PR REST.
type BindingError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *BindingError) Error() string {
	if e == nil {
		return ""
	}
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Sprintf(`{"code":"%s","message":"%s","details":"%s"}`, e.Code, sanitizeJSONText(e.Message), sanitizeJSONText(e.Details))
	}
	return string(payload)
}

func NewBindingError(code, message, details string) *BindingError {
	return &BindingError{
		Code:    strings.TrimSpace(code),
		Message: strings.TrimSpace(message),
		Details: strings.TrimSpace(details),
	}
}

func AsBindingError(err error) *BindingError {
	if err == nil {
		return nil
	}

	var bindingErr *BindingError
	if errors.As(err, &bindingErr) && bindingErr != nil {
		return bindingErr
	}

	raw := strings.TrimSpace(err.Error())
	if raw == "" {
		return nil
	}

	var parsed BindingError
	if parseErr := json.Unmarshal([]byte(raw), &parsed); parseErr == nil && strings.TrimSpace(parsed.Code) != "" {
		return &parsed
	}

	return nil
}

func NormalizeBindingError(err error) *BindingError {
	if err == nil {
		return nil
	}

	if bindingErr := AsBindingError(err); bindingErr != nil {
		if strings.TrimSpace(bindingErr.Message) == "" {
			bindingErr.Message = "Falha ao executar operacao de Pull Requests."
		}
		if strings.TrimSpace(bindingErr.Code) == "" {
			bindingErr.Code = CodeUnknown
		}
		return bindingErr
	}

	return NewBindingError(
		CodeUnknown,
		"Falha ao executar operacao de Pull Requests.",
		err.Error(),
	)
}

// NewHTTPBindingError converte status HTTP do GitHub em erro de dominio padrao.
func NewHTTPBindingError(statusCode int, details string) *BindingError {
	code := CodeForHTTPStatus(statusCode)
	return NewBindingError(code, messageForHTTPCode(code), details)
}

// CodeForHTTPStatus mapeia status HTTP para o contrato final de erro.
func CodeForHTTPStatus(statusCode int) string {
	switch statusCode {
	case 401:
		return CodeUnauthorized
	case 403:
		return CodeForbidden
	case 404:
		return CodeNotFound
	case 409:
		return CodeConflict
	case 422:
		return CodeValidationFailed
	case 429:
		return CodeRateLimited
	default:
		return CodeUnknown
	}
}

func messageForHTTPCode(code string) string {
	switch code {
	case CodeUnauthorized:
		return "Sessao GitHub invalida ou expirada."
	case CodeForbidden:
		return "Operacao bloqueada por permissao ou rate limit."
	case CodeNotFound:
		return "Recurso de Pull Request nao encontrado."
	case CodeConflict:
		return "Conflito detectado na operacao de Pull Request."
	case CodeValidationFailed:
		return "Payload invalido para operacao de Pull Request."
	case CodeRateLimited:
		return "Rate limit excedido para operacoes de Pull Request."
	default:
		return "Falha ao executar operacao de Pull Requests."
	}
}

func sanitizeJSONText(input string) string {
	output := strings.ReplaceAll(input, `"`, `'`)
	output = strings.ReplaceAll(output, "\n", " ")
	return strings.TrimSpace(output)
}
