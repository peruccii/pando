package auth

import (
	"strings"
	"testing"
)

func TestSummarizeAuthErrorBodyPrefersSafeFields(t *testing.T) {
	body := []byte(`{"error":"invalid_grant","error_description":"code verifier mismatch"}`)
	message := summarizeAuthErrorBody(body)
	if message != "code verifier mismatch" {
		t.Fatalf("unexpected summarized message: got=%q want=%q", message, "code verifier mismatch")
	}
}

func TestSummarizeAuthErrorBodyNeverEchoesTokenPayload(t *testing.T) {
	body := []byte(`{"access_token":"secret-token-value","refresh_token":"secret-refresh-token"}`)
	message := summarizeAuthErrorBody(body)
	if message != "authentication provider returned an error" {
		t.Fatalf("unexpected fallback summary: got=%q", message)
	}
	if strings.Contains(message, "secret-token-value") || strings.Contains(message, "secret-refresh-token") {
		t.Fatalf("summary leaked token payload: %q", message)
	}
}
