package session

import (
	"strings"
	"testing"
	"time"
)

func newServiceForTest(emit func(eventName string, data interface{})) *Service {
	return &Service{
		sessions:  make(map[string]*Session),
		codeIndex: make(map[string]string),
		hostIndex: make(map[string]string),
		emitEvent: emit,
	}
}

func TestCreateSessionAppliesDefaultsAndGeneratesValidCode(t *testing.T) {
	svc := newServiceForTest(nil)
	start := time.Now()

	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if session.Status != StatusWaiting {
		t.Fatalf("status = %q, want %q", session.Status, StatusWaiting)
	}
	if session.Mode != ModeLiveShare {
		t.Fatalf("mode = %q, want %q", session.Mode, ModeLiveShare)
	}
	if session.Config.MaxGuests != 10 {
		t.Fatalf("maxGuests = %d, want 10", session.Config.MaxGuests)
	}
	if session.Config.DefaultPerm != PermReadOnly {
		t.Fatalf("defaultPerm = %q, want %q", session.Config.DefaultPerm, PermReadOnly)
	}
	if session.Config.CodeTTLMinutes != 15 {
		t.Fatalf("codeTTLMinutes = %d, want 15", session.Config.CodeTTLMinutes)
	}
	if !validateCodeFormat(session.Code) {
		t.Fatalf("generated code %q is not in XXX-YY format", session.Code)
	}

	ttl := session.ExpiresAt.Sub(start)
	if ttl < 14*time.Minute || ttl > 16*time.Minute {
		t.Fatalf("ttl = %s, expected around 15m", ttl)
	}

	if _, ok := svc.codeIndex[normalizeCode(session.Code)]; !ok {
		t.Fatalf("code %q was not indexed", session.Code)
	}
	if gotSessionID := svc.hostIndex["host-1"]; gotSessionID != session.ID {
		t.Fatalf("host index = %q, want %q", gotSessionID, session.ID)
	}
}

func TestCreateSessionBlocksSecondActiveSessionForSameHostUntilEnd(t *testing.T) {
	svc := newServiceForTest(nil)

	first, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("first CreateSession() error = %v", err)
	}

	_, err = svc.CreateSession("host-1", SessionConfig{})
	if err == nil {
		t.Fatalf("expected error when host creates second active session")
	}
	if !strings.Contains(err.Error(), "host already has an active session") {
		t.Fatalf("unexpected error = %v", err)
	}

	if err := svc.EndSession(first.ID); err != nil {
		t.Fatalf("EndSession() error = %v", err)
	}

	if _, err := svc.CreateSession("host-1", SessionConfig{}); err != nil {
		t.Fatalf("CreateSession() after EndSession() error = %v", err)
	}
}

func TestJoinSessionNormalizesCodeAndAddsPendingGuest(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{
		DefaultPerm: PermReadWrite,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	joinCode := " " + strings.ToLower(session.Code) + " "
	result, err := svc.JoinSession(joinCode, "guest-1", GuestInfo{Name: "Guest One"})
	if err != nil {
		t.Fatalf("JoinSession() error = %v", err)
	}

	if result.SessionID != session.ID {
		t.Fatalf("join result sessionID = %q, want %q", result.SessionID, session.ID)
	}
	if result.Status != string(GuestPending) {
		t.Fatalf("join result status = %q, want %q", result.Status, GuestPending)
	}

	current, err := svc.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if len(current.Guests) != 1 {
		t.Fatalf("guests len = %d, want 1", len(current.Guests))
	}

	guest := current.Guests[0]
	if guest.UserID != "guest-1" {
		t.Fatalf("guest userID = %q, want guest-1", guest.UserID)
	}
	if guest.Status != GuestPending {
		t.Fatalf("guest status = %q, want %q", guest.Status, GuestPending)
	}
	if guest.Permission != PermReadWrite {
		t.Fatalf("guest permission = %q, want %q", guest.Permission, PermReadWrite)
	}
}

func TestJoinSessionRejectsExpiredCode(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	session.ExpiresAt = time.Now().Add(-1 * time.Second)

	_, err = svc.JoinSession(session.Code, "guest-1", GuestInfo{Name: "Guest One"})
	if err == nil {
		t.Fatalf("expected error for expired code")
	}
	if !strings.Contains(err.Error(), "session code has expired") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestJoinSessionRejectsAnonymousWhenDisabled(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{
		AllowAnonymous: false,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	_, err = svc.JoinSession(session.Code, "anonymous-123", GuestInfo{Name: "Anonymous"})
	if err == nil {
		t.Fatalf("expected anonymous join to fail when allowAnonymous=false")
	}
	if !strings.Contains(err.Error(), "anonymous guests are not allowed") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestApproveGuestKeepsCodeUntilFirstPeerConnected(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := svc.JoinSession(session.Code, "guest-1", GuestInfo{Name: "Guest One"}); err != nil {
		t.Fatalf("JoinSession() error = %v", err)
	}

	if err := svc.ApproveGuest(session.ID, "guest-1"); err != nil {
		t.Fatalf("ApproveGuest() error = %v", err)
	}

	current, err := svc.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if current.Status != StatusWaiting {
		t.Fatalf("session status = %q, want %q before peer_connected", current.Status, StatusWaiting)
	}
	if current.Guests[0].Status != GuestApproved {
		t.Fatalf("guest status = %q, want %q", current.Guests[0].Status, GuestApproved)
	}
	if _, ok := svc.codeIndex[normalizeCode(session.Code)]; !ok {
		t.Fatalf("code should stay valid until peer_connected")
	}

	_, err = svc.JoinSession(session.Code, "guest-2", GuestInfo{Name: "Guest Two"})
	if err != nil {
		t.Fatalf("join before peer_connected should still be allowed: %v", err)
	}

	if err := svc.MarkGuestConnected(session.ID, "guest-1"); err != nil {
		t.Fatalf("MarkGuestConnected() error = %v", err)
	}

	current, err = svc.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if current.Status != StatusActive {
		t.Fatalf("session status = %q, want %q after peer_connected", current.Status, StatusActive)
	}
	if current.Guests[0].Status != GuestConnected {
		t.Fatalf("guest status = %q, want %q after peer_connected", current.Guests[0].Status, GuestConnected)
	}
	if _, ok := svc.codeIndex[normalizeCode(session.Code)]; ok {
		t.Fatalf("code should be invalidated after first peer_connected")
	}

	_, err = svc.JoinSession(session.Code, "guest-3", GuestInfo{Name: "Guest Three"})
	if err == nil {
		t.Fatalf("expected join to fail after code invalidation on peer_connected")
	}
	if !strings.Contains(err.Error(), "session not found for code") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestRejectGuestPreventsRejoin(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := svc.JoinSession(session.Code, "guest-1", GuestInfo{Name: "Guest One"}); err != nil {
		t.Fatalf("JoinSession() error = %v", err)
	}
	if err := svc.RejectGuest(session.ID, "guest-1"); err != nil {
		t.Fatalf("RejectGuest() error = %v", err)
	}

	_, err = svc.JoinSession(session.Code, "guest-1", GuestInfo{Name: "Guest One"})
	if err == nil {
		t.Fatalf("expected rejoin to fail after rejection")
	}
	if !strings.Contains(err.Error(), "you were rejected from this session") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestJoinSessionReturnsSessionFullWhenAtCapacity(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{
		MaxGuests: 1,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Simula uma sessão já no limite para validar a regra de capacidade.
	session.Guests = append(session.Guests, SessionGuest{
		UserID:     "approved-guest",
		Name:       "Approved",
		Permission: PermReadOnly,
		JoinedAt:   time.Now(),
		Status:     GuestApproved,
	})

	_, err = svc.JoinSession(session.Code, "guest-2", GuestInfo{Name: "Guest Two"})
	if err == nil {
		t.Fatalf("expected session full error")
	}
	if !strings.Contains(err.Error(), "session is full (max 1 guests)") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestListPendingGuestsReturnsOnlyPending(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := svc.JoinSession(session.Code, "guest-pending", GuestInfo{Name: "Pending"}); err != nil {
		t.Fatalf("JoinSession(pending) error = %v", err)
	}
	if _, err := svc.JoinSession(session.Code, "guest-rejected", GuestInfo{Name: "Rejected"}); err != nil {
		t.Fatalf("JoinSession(rejected) error = %v", err)
	}
	if err := svc.RejectGuest(session.ID, "guest-rejected"); err != nil {
		t.Fatalf("RejectGuest() error = %v", err)
	}

	pending, err := svc.ListPendingGuests(session.ID)
	if err != nil {
		t.Fatalf("ListPendingGuests() error = %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending len = %d, want 1", len(pending))
	}
	if pending[0].UserID != "guest-pending" {
		t.Fatalf("pending userID = %q, want guest-pending", pending[0].UserID)
	}
}

func TestJoinSessionReturnsApprovalDeadlineAndSessionCode(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := svc.JoinSession(session.Code, "guest-1", GuestInfo{Name: "Guest One"})
	if err != nil {
		t.Fatalf("JoinSession() error = %v", err)
	}

	if result.SessionCode != session.Code {
		t.Fatalf("session code = %q, want %q", result.SessionCode, session.Code)
	}
	if result.ApprovalExpiresAt.IsZero() {
		t.Fatalf("approvalExpiresAt should not be zero for pending join")
	}
	if result.Status != string(GuestPending) {
		t.Fatalf("status = %q, want %q", result.Status, GuestPending)
	}
}

func TestPendingJoinExpiresAfterFiveMinutes(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := svc.JoinSession(session.Code, "guest-1", GuestInfo{Name: "Guest One"}); err != nil {
		t.Fatalf("JoinSession() error = %v", err)
	}

	session.Guests[0].JoinedAt = time.Now().Add(-6 * time.Minute)

	pending, err := svc.ListPendingGuests(session.ID)
	if err != nil {
		t.Fatalf("ListPendingGuests() error = %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending len = %d, want 0", len(pending))
	}

	current, err := svc.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if current.Guests[0].Status != GuestExpired {
		t.Fatalf("guest status = %q, want %q", current.Guests[0].Status, GuestExpired)
	}
}

func TestApproveGuestFailsWhenPendingRequestExpired(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := svc.JoinSession(session.Code, "guest-1", GuestInfo{Name: "Guest One"}); err != nil {
		t.Fatalf("JoinSession() error = %v", err)
	}

	session.Guests[0].JoinedAt = time.Now().Add(-6 * time.Minute)

	err = svc.ApproveGuest(session.ID, "guest-1")
	if err == nil {
		t.Fatalf("expected approve to fail for expired request")
	}
	if !strings.Contains(err.Error(), "approval window expired") {
		t.Fatalf("unexpected error = %v", err)
	}
	if session.Guests[0].Status != GuestExpired {
		t.Fatalf("guest status = %q, want %q", session.Guests[0].Status, GuestExpired)
	}
}

func TestJoinSessionRenewsExpiredRequestWithoutDuplication(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := svc.JoinSession(session.Code, "guest-1", GuestInfo{Name: "Guest One"}); err != nil {
		t.Fatalf("JoinSession() error = %v", err)
	}

	session.Guests[0].JoinedAt = time.Now().Add(-6 * time.Minute)

	result, err := svc.JoinSession(session.Code, "guest-1", GuestInfo{Name: "Guest One"})
	if err != nil {
		t.Fatalf("JoinSession() renew error = %v", err)
	}

	if result.Status != string(GuestPending) {
		t.Fatalf("status = %q, want %q", result.Status, GuestPending)
	}
	if len(session.Guests) != 1 {
		t.Fatalf("guests len = %d, want 1", len(session.Guests))
	}
	if session.Guests[0].Status != GuestPending {
		t.Fatalf("guest status = %q, want %q", session.Guests[0].Status, GuestPending)
	}
	if !session.Guests[0].JoinedAt.After(time.Now().Add(-30 * time.Second)) {
		t.Fatalf("joinedAt should be renewed, got %s", session.Guests[0].JoinedAt)
	}
}
