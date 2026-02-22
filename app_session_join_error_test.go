package main

import (
	"strings"
	"testing"
	"time"

	"orch/internal/auth"
	"orch/internal/session"
)

func newJoinTestApp() *App {
	app := NewApp()
	app.auth = auth.NewService(nil)
	app.auth.SetCurrentUserForTesting(&auth.User{
		ID:        "join-test-user",
		Email:     "join-test-user@example.com",
		Name:      "Join Test User",
		AvatarURL: "https://avatars.githubusercontent.com/u/3?v=4",
		Provider:  "github",
	})
	app.session = session.NewService(nil)
	app.sessionGatewayOwner = true
	return app
}

func TestSessionJoinReturnsGenericErrorForInvalidCode(t *testing.T) {
	app := newJoinTestApp()

	_, err := app.SessionJoin("bad", "Guest", "")
	if err == nil {
		t.Fatalf("expected error for invalid code")
	}
	if err.Error() != genericSessionJoinCodeError {
		t.Fatalf("error = %q, want %q", err.Error(), genericSessionJoinCodeError)
	}
}

func TestSessionJoinReturnsGenericErrorForUnknownCode(t *testing.T) {
	app := newJoinTestApp()

	_, err := app.SessionJoin("ABCD-EFG", "Guest", "")
	if err == nil {
		t.Fatalf("expected error for unknown code")
	}
	if err.Error() != genericSessionJoinCodeError {
		t.Fatalf("error = %q, want %q", err.Error(), genericSessionJoinCodeError)
	}
}

func TestSessionJoinReturnsGenericErrorForExpiredCode(t *testing.T) {
	app := newJoinTestApp()

	created, err := app.session.CreateSession("host-1", session.SessionConfig{AllowAnonymous: true})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	created.ExpiresAt = time.Now().Add(-1 * time.Second)

	_, err = app.SessionJoin(created.Code, "Guest", "")
	if err == nil {
		t.Fatalf("expected error for expired code")
	}
	if err.Error() != genericSessionJoinCodeError {
		t.Fatalf("error = %q, want %q", err.Error(), genericSessionJoinCodeError)
	}
}

func TestSessionJoinPreservesNonCodeErrors(t *testing.T) {
	app := newJoinTestApp()
	app.auth.SetCurrentUserForTesting(&auth.User{
		ID:        "anonymous-join-test",
		Email:     "anonymous-join-test@example.com",
		Name:      "Anonymous Join",
		AvatarURL: "https://avatars.githubusercontent.com/u/4?v=4",
		Provider:  "github",
	})

	created, err := app.session.CreateSession("host-1", session.SessionConfig{AllowAnonymous: false})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	_, err = app.SessionJoin(created.Code, "Guest", "")
	if err == nil {
		t.Fatalf("expected anonymous guard error")
	}
	if err.Error() == genericSessionJoinCodeError {
		t.Fatalf("non-code error should not be sanitized")
	}
	if !strings.Contains(err.Error(), "anonymous guests are not allowed") {
		t.Fatalf("unexpected error = %v", err)
	}
}
