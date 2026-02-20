package session

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func connectSignalWS(t *testing.T, serverURL, sessionID, userID, role string) *websocket.Conn {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws/signal" +
		"?session=" + url.QueryEscape(sessionID) +
		"&user=" + url.QueryEscape(userID) +
		"&role=" + url.QueryEscape(role)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial(%s) error = %v", wsURL, err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})
	return conn
}

func dialSignalWSWithOrigin(t *testing.T, serverURL, sessionID, userID, role, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws/signal" +
		"?session=" + url.QueryEscape(sessionID) +
		"&user=" + url.QueryEscape(userID) +
		"&role=" + url.QueryEscape(role)

	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 2 * time.Second,
	}
	return dialer.Dial(wsURL, header)
}

func mustWriteSignal(t *testing.T, conn *websocket.Conn, msg SignalMessage) {
	t.Helper()
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatalf("WriteJSON(%+v) error = %v", msg, err)
	}
}

func mustReadSignal(t *testing.T, conn *websocket.Conn) SignalMessage {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	var msg SignalMessage
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	return msg
}

func TestSignalingForwardsSDPAndICEBetweenHostAndGuest(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	signaling := NewSignalingService(svc)
	server := httptest.NewServer(http.HandlerFunc(signaling.HandleWebSocket))
	t.Cleanup(server.Close)

	hostConn := connectSignalWS(t, server.URL, session.ID, "host-1", "host")
	guestConn := connectSignalWS(t, server.URL, session.ID, "guest-1", "guest")

	mustWriteSignal(t, hostConn, SignalMessage{
		Type:         "sdp_offer",
		TargetUserID: "guest-1",
		Payload:      "offer-1",
	})
	offer := mustReadSignal(t, guestConn)
	if offer.Type != "sdp_offer" || offer.Payload != "offer-1" || offer.FromUserID != "host-1" {
		t.Fatalf("unexpected sdp_offer forwarded: %+v", offer)
	}

	mustWriteSignal(t, guestConn, SignalMessage{
		Type:    "sdp_answer",
		Payload: "answer-1",
	})
	answer := mustReadSignal(t, hostConn)
	if answer.Type != "sdp_answer" || answer.Payload != "answer-1" || answer.FromUserID != "guest-1" {
		t.Fatalf("unexpected sdp_answer forwarded: %+v", answer)
	}

	mustWriteSignal(t, guestConn, SignalMessage{
		Type:    "ice_candidate",
		Payload: "ice-from-guest",
	})
	guestICE := mustReadSignal(t, hostConn)
	if guestICE.Type != "ice_candidate" || guestICE.Payload != "ice-from-guest" || guestICE.FromUserID != "guest-1" {
		t.Fatalf("unexpected guest ice forwarded: %+v", guestICE)
	}

	mustWriteSignal(t, hostConn, SignalMessage{
		Type:         "ice_candidate",
		TargetUserID: "guest-1",
		Payload:      "ice-from-host",
	})
	hostICE := mustReadSignal(t, guestConn)
	if hostICE.Type != "ice_candidate" || hostICE.Payload != "ice-from-host" || hostICE.FromUserID != "host-1" {
		t.Fatalf("unexpected host ice forwarded: %+v", hostICE)
	}

	mustWriteSignal(t, hostConn, SignalMessage{
		Type: "session_ended",
	})
	ended := mustReadSignal(t, guestConn)
	if ended.Type != "session_ended" || ended.FromUserID != "host-1" {
		t.Fatalf("unexpected session_ended forwarded: %+v", ended)
	}
}

func TestSignalingForwardsGuestRequestApproveAndReject(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	signaling := NewSignalingService(svc)
	server := httptest.NewServer(http.HandlerFunc(signaling.HandleWebSocket))
	t.Cleanup(server.Close)

	hostConn := connectSignalWS(t, server.URL, session.ID, "host-1", "host")
	guestConn := connectSignalWS(t, server.URL, session.ID, "guest-1", "guest")

	mustWriteSignal(t, guestConn, SignalMessage{
		Type:    "guest_request",
		Payload: "join please",
	})
	req := mustReadSignal(t, hostConn)
	if req.Type != "guest_request" || req.Payload != "join please" || req.FromUserID != "guest-1" {
		t.Fatalf("unexpected guest_request forwarded: %+v", req)
	}

	mustWriteSignal(t, hostConn, SignalMessage{
		Type:         "guest_approved",
		TargetUserID: "guest-1",
		Payload:      "approved",
	})
	approved := mustReadSignal(t, guestConn)
	if approved.Type != "guest_approved" || approved.Payload != "approved" || approved.FromUserID != "host-1" {
		t.Fatalf("unexpected guest_approved forwarded: %+v", approved)
	}

	mustWriteSignal(t, hostConn, SignalMessage{
		Type:         "guest_rejected",
		TargetUserID: "guest-1",
	})
	rejected := mustReadSignal(t, guestConn)
	if rejected.Type != "guest_rejected" || rejected.FromUserID != "host-1" {
		t.Fatalf("unexpected guest_rejected forwarded: %+v", rejected)
	}
}

func TestNotifyPermissionChangeTargetsGuest(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	signaling := NewSignalingService(svc)
	server := httptest.NewServer(http.HandlerFunc(signaling.HandleWebSocket))
	t.Cleanup(server.Close)

	_ = connectSignalWS(t, server.URL, session.ID, "host-1", "host")
	guestConn := connectSignalWS(t, server.URL, session.ID, "guest-1", "guest")

	signaling.NotifyPermissionChange(session.ID, "guest-1", string(PermReadOnly))

	msg := mustReadSignal(t, guestConn)
	if msg.Type != "permission_change" {
		t.Fatalf("type = %q, want permission_change", msg.Type)
	}
	if msg.Payload != string(PermReadOnly) {
		t.Fatalf("payload = %q, want %q", msg.Payload, PermReadOnly)
	}
	if msg.TargetUserID != "guest-1" {
		t.Fatalf("targetUserID = %q, want guest-1", msg.TargetUserID)
	}
	if msg.FromUserID != "host" {
		t.Fatalf("fromUserID = %q, want host", msg.FromUserID)
	}
}

func TestNotifySessionEndedCleansUpConnections(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	signaling := NewSignalingService(svc)
	server := httptest.NewServer(http.HandlerFunc(signaling.HandleWebSocket))
	t.Cleanup(server.Close)

	_ = connectSignalWS(t, server.URL, session.ID, "host-1", "host")
	_ = connectSignalWS(t, server.URL, session.ID, "guest-1", "guest")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		signaling.mu.RLock()
		count := len(signaling.connections[session.ID])
		signaling.mu.RUnlock()
		if count == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	signaling.NotifySessionEnded(session.ID, "host")

	waitUntil := time.Now().Add(2 * time.Second)
	for time.Now().Before(waitUntil) {
		signaling.mu.RLock()
		connections := len(signaling.connections[session.ID])
		_, sigExists := signaling.sigSessions[session.ID]
		signaling.mu.RUnlock()
		if connections == 0 && !sigExists {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	signaling.mu.RLock()
	connections := len(signaling.connections[session.ID])
	_, sigExists := signaling.sigSessions[session.ID]
	signaling.mu.RUnlock()
	t.Fatalf("expected cleanup, got connections=%d sigSessionExists=%t", connections, sigExists)
}

func TestPeerConnectedMarksApprovedGuestAsConnected(t *testing.T) {
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

	signaling := NewSignalingService(svc)
	server := httptest.NewServer(http.HandlerFunc(signaling.HandleWebSocket))
	t.Cleanup(server.Close)

	guestConn := connectSignalWS(t, server.URL, session.ID, "guest-1", "guest")
	mustWriteSignal(t, guestConn, SignalMessage{Type: "peer_connected"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, getErr := svc.GetSession(session.ID)
		if getErr != nil {
			t.Fatalf("GetSession() error = %v", getErr)
		}
		for _, g := range current.Guests {
			if g.UserID == "guest-1" && g.Status == GuestConnected {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("guest status did not transition to %q after peer_connected", GuestConnected)
}

func TestSignalingReplacesDuplicateIdentityConnection(t *testing.T) {
	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	signaling := NewSignalingService(svc)
	server := httptest.NewServer(http.HandlerFunc(signaling.HandleWebSocket))
	t.Cleanup(server.Close)

	firstHost := connectSignalWS(t, server.URL, session.ID, "host-1", "host")
	secondHost := connectSignalWS(t, server.URL, session.ID, "host-1", "host")
	guestConn := connectSignalWS(t, server.URL, session.ID, "guest-1", "guest")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		signaling.mu.RLock()
		count := len(signaling.connections[session.ID])
		signaling.mu.RUnlock()
		if count == 2 { // host + guest
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	signaling.mu.RLock()
	count := len(signaling.connections[session.ID])
	signaling.mu.RUnlock()
	if count != 2 {
		t.Fatalf("connections count = %d, want 2 (host + guest)", count)
	}

	mustWriteSignal(t, guestConn, SignalMessage{
		Type:    "guest_request",
		Payload: "join please",
	})

	req := mustReadSignal(t, secondHost)
	if req.Type != "guest_request" || req.FromUserID != "guest-1" {
		t.Fatalf("unexpected guest_request on second host: %+v", req)
	}

	staleClosed := false
	waitUntil := time.Now().Add(2 * time.Second)
	for time.Now().Before(waitUntil) {
		if err := firstHost.WriteControl(websocket.PingMessage, []byte("stale"), time.Now().Add(150*time.Millisecond)); err != nil {
			staleClosed = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !staleClosed {
		t.Fatalf("expected first host connection to be closed")
	}
}

func TestSignalingRejectsDisallowedOrigin(t *testing.T) {
	t.Setenv("ORCH_SIGNALING_ALLOWED_ORIGINS", "")

	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	signaling := NewSignalingService(svc)
	server := httptest.NewServer(http.HandlerFunc(signaling.HandleWebSocket))
	t.Cleanup(server.Close)

	conn, resp, err := dialSignalWSWithOrigin(t, server.URL, session.ID, "guest-1", "guest", "https://evil.example")
	if err == nil {
		_ = conn.Close()
		t.Fatalf("expected websocket dial to fail for disallowed origin")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		if resp == nil {
			t.Fatalf("expected HTTP 403, got nil response")
		}
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestSignalingAllowsConfiguredOrigin(t *testing.T) {
	t.Setenv("ORCH_SIGNALING_ALLOWED_ORIGINS", "https://trusted.example")

	svc := newServiceForTest(nil)
	session, err := svc.CreateSession("host-1", SessionConfig{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	signaling := NewSignalingService(svc)
	server := httptest.NewServer(http.HandlerFunc(signaling.HandleWebSocket))
	t.Cleanup(server.Close)

	conn, _, err := dialSignalWSWithOrigin(t, server.URL, session.ID, "guest-1", "guest", "https://trusted.example")
	if err != nil {
		t.Fatalf("expected websocket dial to succeed for configured origin: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})
}
