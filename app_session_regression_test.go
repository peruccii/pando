package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"orch/internal/database"
	"orch/internal/session"

	"github.com/gorilla/websocket"
)

func reserveGatewayAddress(t *testing.T) (string, string) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve tcp port: %v", err)
	}
	addr := l.Addr().(*net.TCPAddr)
	hostAddr := fmt.Sprintf("127.0.0.1:%d", addr.Port)
	if err := l.Close(); err != nil {
		t.Fatalf("failed to release reserved tcp port: %v", err)
	}
	return hostAddr, "http://" + hostAddr
}

func newSessionHostAppWithDB(t *testing.T) (*App, *database.Service) {
	t.Helper()

	db, err := database.NewService()
	if err != nil {
		t.Fatalf("failed to init test database: %v", err)
	}

	app := NewApp()
	app.db = db
	app.session = session.NewService(nil)
	app.sessionGatewayOwner = true
	return app, db
}

func waitForGuestStatus(t *testing.T, app *App, sessionID, guestUserID string, expected session.GuestStatus, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		current, err := app.SessionGetSession(sessionID)
		if err == nil && current != nil {
			for _, g := range current.Guests {
				if g.UserID == guestUserID && g.Status == expected {
					return
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("guest %s did not reach status %q in session %s", guestUserID, expected, sessionID)
}

func TestSessionRegression_TwoInstances_CreateJoinApproveRestartRestoreRejoin(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("HOME", tempRoot)
	t.Setenv("ORCH_DB_PATH", fmt.Sprintf("%s/orch-regression.db", tempRoot))

	gatewayAddr, gatewayURL := reserveGatewayAddress(t)

	host1, db1 := newSessionHostAppWithDB(t)
	gateway1 := session.NewGatewayServer(host1.session, gatewayAddr)
	gateway1.SetObservers(host1.persistSessionState, host1.deletePersistedSessionState)
	if err := gateway1.Start(); err != nil {
		t.Fatalf("failed to start gateway1: %v", err)
	}

	guest := NewApp()
	guest.sessionGatewayOwner = false
	guest.sessionGatewayURL = gatewayURL
	guest.session = nil

	created, err := host1.SessionCreate(2, string(session.ModeLiveShare), true, 1)
	if err != nil {
		t.Fatalf("SessionCreate() error: %v", err)
	}
	if created == nil {
		t.Fatalf("SessionCreate() returned nil session")
	}

	joinResult, err := guest.SessionJoin(created.Code, "Guest QA", "qa@example.com")
	if err != nil {
		t.Fatalf("guest SessionJoin() error: %v", err)
	}
	if joinResult == nil || joinResult.GuestUserID == "" {
		t.Fatalf("invalid join result: %+v", joinResult)
	}

	pending, err := host1.SessionListPendingGuests(created.ID)
	if err != nil {
		t.Fatalf("SessionListPendingGuests() error: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending requests = %d, want 1", len(pending))
	}
	if pending[0].UserID != joinResult.GuestUserID {
		t.Fatalf("pending userID = %q, want %q", pending[0].UserID, joinResult.GuestUserID)
	}

	if err := host1.SessionApproveGuest(created.ID, joinResult.GuestUserID); err != nil {
		t.Fatalf("SessionApproveGuest() error: %v", err)
	}
	waitForGuestStatus(t, host1, created.ID, joinResult.GuestUserID, session.GuestApproved, 2*time.Second)

	stopCtx1, cancel1 := context.WithTimeout(context.Background(), 2*time.Second)
	if err := gateway1.Stop(stopCtx1); err != nil {
		t.Fatalf("failed to stop gateway1: %v", err)
	}
	cancel1()
	if err := db1.Close(); err != nil {
		t.Fatalf("failed to close db1: %v", err)
	}

	host2, db2 := newSessionHostAppWithDB(t)
	t.Cleanup(func() {
		_ = db2.Close()
	})
	host2.restorePersistedSessionStates()

	gateway2 := session.NewGatewayServer(host2.session, gatewayAddr)
	gateway2.SetObservers(host2.persistSessionState, host2.deletePersistedSessionState)
	if err := gateway2.Start(); err != nil {
		t.Fatalf("failed to start gateway2: %v", err)
	}
	t.Cleanup(func() {
		stopCtx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
		_ = gateway2.Stop(stopCtx2)
		cancel2()
	})

	active, err := host2.SessionGetActive()
	if err != nil {
		t.Fatalf("SessionGetActive() after restore error: %v", err)
	}
	if active == nil {
		t.Fatalf("SessionGetActive() returned nil after restore")
	}
	if active.ID != created.ID {
		t.Fatalf("restored active session id = %q, want %q", active.ID, created.ID)
	}

	waitForGuestStatus(t, host2, active.ID, joinResult.GuestUserID, session.GuestApproved, 2*time.Second)

	signaling := session.NewSignalingService(host2.session)
	signalingServer := httptest.NewServer(http.HandlerFunc(signaling.HandleWebSocket))
	t.Cleanup(signalingServer.Close)

	wsURL := "ws" + strings.TrimPrefix(signalingServer.URL, "http") + "/ws/signal" +
		"?session=" + url.QueryEscape(active.ID) +
		"&user=" + url.QueryEscape(joinResult.GuestUserID) +
		"&role=guest"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect signaling websocket: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	if err := conn.WriteJSON(session.SignalMessage{Type: "peer_connected"}); err != nil {
		t.Fatalf("failed to send peer_connected: %v", err)
	}

	waitForGuestStatus(t, host2, active.ID, joinResult.GuestUserID, session.GuestConnected, 2*time.Second)
}
