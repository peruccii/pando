package gitpanel

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	CodeServiceUnavailable = "E_SERVICE_UNAVAILABLE"
	CodeGitUnavailable     = "E_GIT_UNAVAILABLE"
	CodeRepoNotResolved    = "E_REPO_NOT_RESOLVED"
	CodeRepoNotFound       = "E_REPO_NOT_FOUND"
	CodeRepoNotGit         = "E_REPO_NOT_GIT"
	CodeRepoOutOfScope     = "E_REPO_OUT_OF_SCOPE"
	CodeInvalidPath        = "E_INVALID_PATH"
	CodeInvalidCursor      = "E_INVALID_CURSOR"
	CodePatchInvalid       = "E_PATCH_INVALID"
	CodeCommandFailed      = "E_COMMAND_FAILED"
	CodeTimeout            = "E_TIMEOUT"
	CodeCanceled           = "E_CANCELED"
	CodeUnknown            = "E_UNKNOWN"
)

// BindingError implementa contrato normalizado de erro para bindings.
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
			bindingErr.Message = "Falha ao executar operação do Git Panel"
		}
		if strings.TrimSpace(bindingErr.Code) == "" {
			bindingErr.Code = CodeUnknown
		}
		return bindingErr
	}

	return NewBindingError(
		CodeUnknown,
		"Falha ao executar operação do Git Panel",
		err.Error(),
	)
}

func sanitizeJSONText(input string) string {
	output := strings.ReplaceAll(input, `"`, `'`)
	output = strings.ReplaceAll(output, "\n", " ")
	return strings.TrimSpace(output)
}
