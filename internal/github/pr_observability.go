package github

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

// PRRequestTelemetry representa metrica minima de request de PR REST.
type PRRequestTelemetry struct {
	Method        string `json:"method"`
	Endpoint      string `json:"endpoint"`
	StatusCode    int    `json:"statusCode"`
	DurationMs    int64  `json:"durationMs"`
	Cache         string `json:"cache"` // hit | miss
	RateRemaining int    `json:"rateRemaining"`
}

// PRCacheTelemetry representa evento simplificado de cache para PR REST.
type PRCacheTelemetry struct {
	Method   string `json:"method"`
	Endpoint string `json:"endpoint"`
	Cache    string `json:"cache"` // hit | miss
}

const (
	prActionCreate              = "create"
	prActionUpdate              = "update"
	prActionMerge               = "merge"
	prActionUpdateBranch        = "update_branch"
	prActionLabelCreate         = "label_create"
	prActionInlineCommentCreate = "inline_comment_create"
)

// PRActionResultTelemetry representa resultado de acoes mutaveis de PR REST.
type PRActionResultTelemetry struct {
	Action        string `json:"action"`
	Method        string `json:"method"`
	Endpoint      string `json:"endpoint"`
	StatusCode    int    `json:"statusCode"`
	DurationMs    int64  `json:"durationMs"`
	Success       bool   `json:"success"`
	RateRemaining int    `json:"rateRemaining"`
	ErrorType     string `json:"errorType,omitempty"`
}

// SetTelemetryEmitter registra callback para emitir telemetria de PR REST.
func (s *Service) SetTelemetryEmitter(emitter func(eventName string, payload interface{})) {
	if s == nil {
		return
	}
	s.telemetry = emitter
}

func (s *Service) emitTelemetry(eventName string, payload interface{}) {
	if s == nil {
		return
	}
	emit := s.telemetry
	if emit == nil {
		return
	}
	if strings.TrimSpace(eventName) == "" {
		return
	}
	emit(eventName, payload)
}

func (s *Service) emitPRReadTelemetry(method, endpoint string, statusCode int, startedAt time.Time, cacheResult string) {
	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	if normalizedMethod == "" {
		normalizedMethod = http.MethodGet
	}

	normalizedEndpoint := strings.TrimSpace(endpoint)
	if normalizedEndpoint == "" {
		normalizedEndpoint = "/"
	}

	normalizedCache := normalizeCacheTelemetryResult(cacheResult)
	durationMs := int64(0)
	if !startedAt.IsZero() {
		durationMs = time.Since(startedAt).Milliseconds()
		if durationMs < 0 {
			durationMs = 0
		}
	}

	s.emitTelemetry("gitpanel:prs_cache", PRCacheTelemetry{
		Method:   normalizedMethod,
		Endpoint: normalizedEndpoint,
		Cache:    normalizedCache,
	})

	s.emitTelemetry("gitpanel:prs_request", PRRequestTelemetry{
		Method:        normalizedMethod,
		Endpoint:      normalizedEndpoint,
		StatusCode:    statusCode,
		DurationMs:    durationMs,
		Cache:         normalizedCache,
		RateRemaining: s.rateLeft,
	})
}

func (s *Service) emitPRActionResultTelemetry(action, method, endpoint string, statusCode int, startedAt time.Time, err error) {
	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	if normalizedMethod == "" {
		normalizedMethod = "UNKNOWN"
	}

	normalizedEndpoint := strings.TrimSpace(endpoint)
	if normalizedEndpoint == "" {
		normalizedEndpoint = "/"
	}

	if statusCode <= 0 {
		statusCode = statusCodeFromGitHubError(err)
	}
	if statusCode < 0 {
		statusCode = 0
	}

	durationMs := int64(0)
	if !startedAt.IsZero() {
		durationMs = time.Since(startedAt).Milliseconds()
		if durationMs < 0 {
			durationMs = 0
		}
	}

	errorType := ""
	if err != nil {
		var githubErr *GitHubError
		if errors.As(err, &githubErr) && githubErr != nil {
			errorType = strings.TrimSpace(githubErr.Type)
		} else {
			errorType = "unknown"
		}
	}

	success := err == nil && statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices

	s.emitTelemetry("gitpanel:prs_action_result", PRActionResultTelemetry{
		Action:        normalizePRActionTelemetryName(action),
		Method:        normalizedMethod,
		Endpoint:      normalizedEndpoint,
		StatusCode:    statusCode,
		DurationMs:    durationMs,
		Success:       success,
		RateRemaining: s.rateLeft,
		ErrorType:     errorType,
	})
}

func normalizeCacheTelemetryResult(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hit":
		return "hit"
	default:
		return "miss"
	}
}

func normalizePRActionTelemetryName(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "unknown"
	}
	normalized = strings.ReplaceAll(normalized, " ", "_")
	return normalized
}

func statusCodeFromGitHubError(err error) int {
	if err == nil {
		return 0
	}

	var githubErr *GitHubError
	if errors.As(err, &githubErr) && githubErr != nil {
		return githubErr.StatusCode
	}

	return 0
}
