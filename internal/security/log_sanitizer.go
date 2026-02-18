package security

import "regexp"

// LogSanitizer remove credenciais e segredos antes de persistir logs/auditoria.
type LogSanitizer struct {
	patterns []*regexp.Regexp
}

func NewLogSanitizer() *LogSanitizer {
	return &LogSanitizer{
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|authorization)\s*[:=]\s*['"]?[\w\-\.]+['"]?`),
			regexp.MustCompile(`(?i)bearer\s+[\w\-\.=]+`),
			regexp.MustCompile(`gh[pousr]_[a-zA-Z0-9]{20,}`),
			regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`),
			regexp.MustCompile(`AIza[a-zA-Z0-9_\-]{35}`),
			regexp.MustCompile(`(?i)(cookie|set-cookie):\s*[^\s;]+`),
		},
	}
}

func (s *LogSanitizer) Sanitize(message string) string {
	if s == nil {
		return message
	}

	clean := message
	for _, p := range s.patterns {
		clean = p.ReplaceAllString(clean, "[REDACTED]")
	}
	return clean
}
