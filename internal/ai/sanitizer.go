package ai

import "regexp"

// SecretSanitizer remove segredos/tokens do texto antes de enviar para LLM.
type SecretSanitizer struct {
	patterns []*regexp.Regexp
}

// NewSecretSanitizer cria sanitizador com padrões comuns de segredo.
func NewSecretSanitizer() *SecretSanitizer {
	return &SecretSanitizer{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|auth)\s*[:=]\s*['"]?[\w\-\.]+['"]?`),
			regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`),    // GitHub PAT
			regexp.MustCompile(`gho_[a-zA-Z0-9]{36}`),    // GitHub OAuth token
			regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`),    // OpenAI
			regexp.MustCompile(`AIza[a-zA-Z0-9_\-]{35}`), // Google API
			regexp.MustCompile(`Bearer\s+[\w\-\.]+`),     // Bearer tokens
		},
	}
}

// Clean substitui padrões sensíveis por [REDACTED].
func (s *SecretSanitizer) Clean(text string) string {
	clean := text
	for _, p := range s.patterns {
		clean = p.ReplaceAllString(clean, "[REDACTED]")
	}
	return clean
}
