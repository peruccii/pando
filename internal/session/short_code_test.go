package session

import "testing"

func TestGenerateShortCodeUsesCurrentFormat(t *testing.T) {
	code, err := generateShortCode()
	if err != nil {
		t.Fatalf("generateShortCode() error = %v", err)
	}

	if len(code) != 8 {
		t.Fatalf("code len = %d, want 8 (XXXX-XXX)", len(code))
	}
	if code[4] != '-' {
		t.Fatalf("code separator = %q, want '-'", code[4])
	}
	if !matchesCodeFormat(code, shortCodePart1Len, shortCodePart2Len) {
		t.Fatalf("code %q is not in XXXX-XXX format", code)
	}
}

func TestValidateCodeFormatSupportsCurrentAndLegacy(t *testing.T) {
	tests := []struct {
		name  string
		code  string
		valid bool
	}{
		{
			name:  "current_format_upper",
			code:  "ABCD-EFG",
			valid: true,
		},
		{
			name:  "current_format_lower_trimmed",
			code:  " abcd-efg ",
			valid: true,
		},
		{
			name:  "legacy_format_still_allowed",
			code:  "ABC-DE",
			valid: true,
		},
		{
			name:  "missing_separator",
			code:  "ABCDEFG",
			valid: false,
		},
		{
			name:  "wrong_lengths",
			code:  "ABCD-EF",
			valid: false,
		},
		{
			name:  "invalid_character",
			code:  "ABCD-E0G",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateCodeFormat(tt.code)
			if got != tt.valid {
				t.Fatalf("validateCodeFormat(%q) = %v, want %v", tt.code, got, tt.valid)
			}
		})
	}
}
