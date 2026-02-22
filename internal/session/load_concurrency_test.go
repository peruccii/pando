package session

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestCreateSessionConcurrentLoadGeneratesUniqueCodes(t *testing.T) {
	svc := newServiceForTest(nil)

	const sessionsToCreate = 200
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		errs  []error
		codes = make(map[string]struct{}, sessionsToCreate)
	)

	for i := 0; i < sessionsToCreate; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			hostID := fmt.Sprintf("host-load-%03d", i)
			created, err := svc.CreateSession(hostID, SessionConfig{})
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}
			if !validateCodeFormat(created.Code) {
				mu.Lock()
				errs = append(errs, fmt.Errorf("invalid code format generated: %q", created.Code))
				mu.Unlock()
				return
			}

			mu.Lock()
			if _, exists := codes[created.Code]; exists {
				errs = append(errs, fmt.Errorf("duplicate code generated under load: %s", created.Code))
				mu.Unlock()
				return
			}
			codes[created.Code] = struct{}{}
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("concurrent session creation returned %d errors (first: %v)", len(errs), errs[0])
	}
	if len(codes) != sessionsToCreate {
		t.Fatalf("unique codes = %d, want %d", len(codes), sessionsToCreate)
	}
}

func TestRegenerateCodeConcurrentLoadKeepsValidCode(t *testing.T) {
	svc := newServiceForTest(nil)
	created, err := svc.CreateSession("host-regen-load", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	const regenerations = 120
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	for i := 0; i < regenerations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, regenErr := svc.RegenerateCode(created.ID)
			if regenErr != nil {
				mu.Lock()
				errs = append(errs, regenErr)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("concurrent regenerate returned %d errors (first: %v)", len(errs), errs[0])
	}

	current, err := svc.GetSession(created.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if !current.AllowNewJoins {
		t.Fatalf("allowNewJoins should remain true after concurrent regenerate")
	}
	if current.Code == "" || !validateCodeFormat(current.Code) {
		t.Fatalf("current code invalid after concurrent regenerate: %q", current.Code)
	}
	indexedSessionID, indexed := svc.codeIndex[normalizeCode(current.Code)]
	if !indexed || indexedSessionID != current.ID {
		t.Fatalf("current code not indexed correctly, indexed=%t sessionID=%q", indexed, indexedSessionID)
	}
}

func TestJoinSessionConcurrentAbuseSingleGuest(t *testing.T) {
	svc := newServiceForTest(nil)

	const (
		guestID  = "abuser-1"
		attempts = 80
	)

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.JoinSession("bad", guestID, GuestInfo{Name: "Abuser"})
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		}()
	}
	wg.Wait()

	invalidFormatErrors := 0
	blockedErrors := 0
	for _, err := range errs {
		if err == nil {
			t.Fatalf("expected error for abusive join attempts")
		}
		msg := err.Error()
		switch {
		case strings.Contains(msg, "invalid code format"):
			invalidFormatErrors++
		case strings.Contains(msg, "too many invalid join attempts"):
			blockedErrors++
		default:
			t.Fatalf("unexpected abuse error: %v", err)
		}
	}

	expectedInvalidFormat := invalidJoinLockMaxAttempts - 1
	expectedBlocked := attempts - expectedInvalidFormat
	if invalidFormatErrors != expectedInvalidFormat {
		t.Fatalf("invalidFormatErrors = %d, want %d", invalidFormatErrors, expectedInvalidFormat)
	}
	if blockedErrors != expectedBlocked {
		t.Fatalf("blockedErrors = %d, want %d", blockedErrors, expectedBlocked)
	}

	metrics := svc.GetJoinSecurityMetrics()
	if metrics.InvalidAttemptsTotal != invalidJoinLockMaxAttempts {
		t.Fatalf("invalidAttemptsTotal = %d, want %d", metrics.InvalidAttemptsTotal, invalidJoinLockMaxAttempts)
	}
	if metrics.InvalidFormatAttemptsTotal != invalidJoinLockMaxAttempts {
		t.Fatalf("invalidFormatAttemptsTotal = %d, want %d", metrics.InvalidFormatAttemptsTotal, invalidJoinLockMaxAttempts)
	}
	if metrics.BlockedAttemptsTotal != expectedBlocked {
		t.Fatalf("blockedAttemptsTotal = %d, want %d", metrics.BlockedAttemptsTotal, expectedBlocked)
	}
	if metrics.LockoutsTotal != 1 {
		t.Fatalf("lockoutsTotal = %d, want 1", metrics.LockoutsTotal)
	}
	if metrics.ActiveLocks != 1 {
		t.Fatalf("activeLocks = %d, want 1", metrics.ActiveLocks)
	}
}

func TestJoinSessionConcurrentAbuseDistributedGuests(t *testing.T) {
	svc := newServiceForTest(nil)
	validSession, err := svc.CreateSession("host-safe-join", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	const (
		attackers        = 16
		attemptsPerGuest = invalidJoinLockMaxAttempts + 2
	)

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)

	for i := 0; i < attackers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			guestID := fmt.Sprintf("attacker-%02d", i)
			for j := 0; j < attemptsPerGuest; j++ {
				_, joinErr := svc.JoinSession("bad", guestID, GuestInfo{Name: guestID})
				mu.Lock()
				errs = append(errs, joinErr)
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err == nil {
			t.Fatalf("expected error for abusive distributed attempts")
		}
		msg := err.Error()
		if !strings.Contains(msg, "invalid code format") && !strings.Contains(msg, "too many invalid join attempts") {
			t.Fatalf("unexpected distributed abuse error: %v", err)
		}
	}

	metrics := svc.GetJoinSecurityMetrics()
	expectedInvalidAttempts := attackers * invalidJoinLockMaxAttempts
	expectedBlockedPerGuest := attemptsPerGuest - (invalidJoinLockMaxAttempts - 1)
	expectedBlocked := attackers * expectedBlockedPerGuest
	if metrics.InvalidAttemptsTotal != expectedInvalidAttempts {
		t.Fatalf("invalidAttemptsTotal = %d, want %d", metrics.InvalidAttemptsTotal, expectedInvalidAttempts)
	}
	if metrics.InvalidFormatAttemptsTotal != expectedInvalidAttempts {
		t.Fatalf("invalidFormatAttemptsTotal = %d, want %d", metrics.InvalidFormatAttemptsTotal, expectedInvalidAttempts)
	}
	if metrics.BlockedAttemptsTotal != expectedBlocked {
		t.Fatalf("blockedAttemptsTotal = %d, want %d", metrics.BlockedAttemptsTotal, expectedBlocked)
	}
	if metrics.LockoutsTotal != attackers {
		t.Fatalf("lockoutsTotal = %d, want %d", metrics.LockoutsTotal, attackers)
	}
	if metrics.ActiveLocks != attackers {
		t.Fatalf("activeLocks = %d, want %d", metrics.ActiveLocks, attackers)
	}

	if _, err := svc.JoinSession(validSession.Code, "healthy-guest", GuestInfo{Name: "Healthy"}); err != nil {
		t.Fatalf("healthy guest should still join with valid code, got: %v", err)
	}
}
