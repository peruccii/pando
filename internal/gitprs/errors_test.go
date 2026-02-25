package gitprs

import (
	"errors"
	"testing"
)

func TestCodeForHTTPStatus(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{status: 401, want: CodeUnauthorized},
		{status: 403, want: CodeForbidden},
		{status: 404, want: CodeNotFound},
		{status: 409, want: CodeConflict},
		{status: 422, want: CodeValidationFailed},
		{status: 429, want: CodeRateLimited},
		{status: 500, want: CodeUnknown},
	}

	for _, tt := range tests {
		if got := CodeForHTTPStatus(tt.status); got != tt.want {
			t.Fatalf("status=%d unexpected code: got=%s want=%s", tt.status, got, tt.want)
		}
	}
}

func TestNormalizeBindingError(t *testing.T) {
	raw := errors.New("some failure")
	normalized := NormalizeBindingError(raw)
	if normalized == nil {
		t.Fatalf("expected normalized error")
	}
	if normalized.Code != CodeUnknown {
		t.Fatalf("unexpected code: got=%s want=%s", normalized.Code, CodeUnknown)
	}
	if normalized.Message == "" {
		t.Fatalf("expected message to be filled")
	}
	if normalized.Details != "some failure" {
		t.Fatalf("unexpected details: got=%q", normalized.Details)
	}
}
