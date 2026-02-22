package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// GatewayServer expõe o SessionService via HTTP local para múltiplas instâncias do app.
type GatewayServer struct {
	service *Service
	addr    string
	server  *http.Server

	onSessionChanged func(sessionID string)
	onSessionDeleted func(sessionID string)
}

func NewGatewayServer(service *Service, addr string) *GatewayServer {
	return &GatewayServer{
		service: service,
		addr:    addr,
	}
}

func (g *GatewayServer) Start() error {
	if g == nil || g.service == nil {
		return fmt.Errorf("session gateway service is nil")
	}
	if g.addr == "" {
		return fmt.Errorf("session gateway addr is empty")
	}
	if g.server != nil {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", g.handleHealthz)
	mux.HandleFunc("/api/session/create", g.handleCreateSession)
	mux.HandleFunc("/api/session/join", g.handleJoinSession)
	mux.HandleFunc("/api/session/approve", g.handleApproveGuest)
	mux.HandleFunc("/api/session/reject", g.handleRejectGuest)
	mux.HandleFunc("/api/session/end", g.handleEndSession)
	mux.HandleFunc("/api/session/get", g.handleGetSession)
	mux.HandleFunc("/api/session/active", g.handleGetActiveSession)
	mux.HandleFunc("/api/session/pending", g.handleListPendingGuests)
	mux.HandleFunc("/api/session/permission", g.handleSetGuestPermission)
	mux.HandleFunc("/api/session/kick", g.handleKickGuest)
	mux.HandleFunc("/api/session/code/regenerate", g.handleRegenerateCode)
	mux.HandleFunc("/api/session/code/revoke", g.handleRevokeCode)
	mux.HandleFunc("/api/session/allow-joins", g.handleSetAllowNewJoins)
	mux.HandleFunc("/api/session/metrics/join-security", g.handleGetJoinSecurityMetrics)
	mux.HandleFunc("/api/session/ice", g.handleGetICEServers)

	listener, err := net.Listen("tcp", g.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", g.addr, err)
	}

	g.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if serveErr := g.server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Printf("[SESSION][GATEWAY] serve error: %v", serveErr)
		}
	}()

	return nil
}

func (g *GatewayServer) Stop(ctx context.Context) error {
	if g == nil || g.server == nil {
		return nil
	}
	return g.server.Shutdown(ctx)
}

func (g *GatewayServer) SetObservers(onSessionChanged func(sessionID string), onSessionDeleted func(sessionID string)) {
	g.onSessionChanged = onSessionChanged
	g.onSessionDeleted = onSessionDeleted
}

func (g *GatewayServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeGatewayJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type createSessionRequest struct {
	HostUserID    string        `json:"hostUserID"`
	HostName      string        `json:"hostName,omitempty"`
	HostAvatarURL string        `json:"hostAvatarUrl,omitempty"`
	Config        SessionConfig `json:"config"`
}

func (g *GatewayServer) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req createSessionRequest
	if err := decodeGatewayJSON(r, &req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.HostUserID == "" {
		writeGatewayError(w, http.StatusBadRequest, "hostUserID is required")
		return
	}

	session, err := g.service.CreateSession(req.HostUserID, req.Config)
	if err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if hostName := strings.TrimSpace(req.HostName); hostName != "" {
		session.HostName = hostName
	}
	if hostAvatarURL := strings.TrimSpace(req.HostAvatarURL); hostAvatarURL != "" {
		session.HostAvatarURL = hostAvatarURL
	}
	if g.onSessionChanged != nil {
		g.onSessionChanged(session.ID)
	}
	writeGatewayJSON(w, http.StatusOK, session)
}

type joinSessionRequest struct {
	Code        string    `json:"code"`
	GuestUserID string    `json:"guestUserID"`
	GuestInfo   GuestInfo `json:"guestInfo"`
}

func (g *GatewayServer) handleJoinSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req joinSessionRequest
	if err := decodeGatewayJSON(r, &req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Code == "" || req.GuestUserID == "" {
		writeGatewayError(w, http.StatusBadRequest, "code and guestUserID are required")
		return
	}

	result, err := g.service.JoinSession(req.Code, req.GuestUserID, req.GuestInfo)
	if err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if g.onSessionChanged != nil {
		g.onSessionChanged(result.SessionID)
	}
	writeGatewayJSON(w, http.StatusOK, result)
}

type sessionGuestActionRequest struct {
	SessionID   string `json:"sessionID"`
	GuestUserID string `json:"guestUserID"`
}

func (g *GatewayServer) handleApproveGuest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req sessionGuestActionRequest
	if err := decodeGatewayJSON(r, &req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := g.service.ApproveGuest(req.SessionID, req.GuestUserID); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if g.onSessionChanged != nil {
		g.onSessionChanged(req.SessionID)
	}
	writeGatewayJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (g *GatewayServer) handleRejectGuest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req sessionGuestActionRequest
	if err := decodeGatewayJSON(r, &req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := g.service.RejectGuest(req.SessionID, req.GuestUserID); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if g.onSessionChanged != nil {
		g.onSessionChanged(req.SessionID)
	}
	writeGatewayJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type endSessionRequest struct {
	SessionID string `json:"sessionID"`
}

func (g *GatewayServer) handleEndSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req endSessionRequest
	if err := decodeGatewayJSON(r, &req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := g.service.EndSession(req.SessionID); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if g.onSessionDeleted != nil {
		g.onSessionDeleted(req.SessionID)
	}
	writeGatewayJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (g *GatewayServer) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sessionID := r.URL.Query().Get("sessionID")
	if sessionID == "" {
		writeGatewayError(w, http.StatusBadRequest, "sessionID is required")
		return
	}
	session, err := g.service.GetSession(sessionID)
	if err != nil {
		writeGatewayError(w, http.StatusNotFound, err.Error())
		return
	}
	writeGatewayJSON(w, http.StatusOK, session)
}

func (g *GatewayServer) handleGetActiveSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID := r.URL.Query().Get("userID")
	if userID == "" {
		writeGatewayError(w, http.StatusBadRequest, "userID is required")
		return
	}
	session, err := g.service.GetActiveSession(userID)
	if err != nil {
		writeGatewayError(w, http.StatusNotFound, err.Error())
		return
	}
	writeGatewayJSON(w, http.StatusOK, session)
}

func (g *GatewayServer) handleListPendingGuests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sessionID := r.URL.Query().Get("sessionID")
	if sessionID == "" {
		writeGatewayError(w, http.StatusBadRequest, "sessionID is required")
		return
	}
	pending, err := g.service.ListPendingGuests(sessionID)
	if err != nil {
		writeGatewayError(w, http.StatusNotFound, err.Error())
		return
	}
	writeGatewayJSON(w, http.StatusOK, pending)
}

type setPermissionRequest struct {
	SessionID   string `json:"sessionID"`
	GuestUserID string `json:"guestUserID"`
	Permission  string `json:"permission"`
}

func (g *GatewayServer) handleSetGuestPermission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req setPermissionRequest
	if err := decodeGatewayJSON(r, &req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := g.service.SetGuestPermission(req.SessionID, req.GuestUserID, req.Permission); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if g.onSessionChanged != nil {
		g.onSessionChanged(req.SessionID)
	}
	writeGatewayJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (g *GatewayServer) handleKickGuest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req sessionGuestActionRequest
	if err := decodeGatewayJSON(r, &req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := g.service.KickGuest(req.SessionID, req.GuestUserID); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if g.onSessionChanged != nil {
		g.onSessionChanged(req.SessionID)
	}
	writeGatewayJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (g *GatewayServer) handleRegenerateCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req endSessionRequest
	if err := decodeGatewayJSON(r, &req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	session, err := g.service.RegenerateCode(req.SessionID)
	if err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if g.onSessionChanged != nil {
		g.onSessionChanged(req.SessionID)
	}
	writeGatewayJSON(w, http.StatusOK, session)
}

func (g *GatewayServer) handleRevokeCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req endSessionRequest
	if err := decodeGatewayJSON(r, &req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	session, err := g.service.RevokeCode(req.SessionID)
	if err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if g.onSessionChanged != nil {
		g.onSessionChanged(req.SessionID)
	}
	writeGatewayJSON(w, http.StatusOK, session)
}

type setAllowNewJoinsRequest struct {
	SessionID string `json:"sessionID"`
	Allow     bool   `json:"allow"`
}

func (g *GatewayServer) handleSetAllowNewJoins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req setAllowNewJoinsRequest
	if err := decodeGatewayJSON(r, &req); err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	session, err := g.service.SetAllowNewJoins(req.SessionID, req.Allow)
	if err != nil {
		writeGatewayError(w, http.StatusBadRequest, err.Error())
		return
	}
	if g.onSessionChanged != nil {
		g.onSessionChanged(req.SessionID)
	}
	writeGatewayJSON(w, http.StatusOK, session)
}

func (g *GatewayServer) handleGetICEServers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeGatewayJSON(w, http.StatusOK, g.service.GetICEServers())
}

func (g *GatewayServer) handleGetJoinSecurityMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeGatewayError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeGatewayJSON(w, http.StatusOK, g.service.GetJoinSecurityMetrics())
}

func decodeGatewayJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid json payload: %w", err)
	}
	return nil
}

func writeGatewayJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeGatewayError(w http.ResponseWriter, status int, message string) {
	writeGatewayJSON(w, status, map[string]string{"error": message})
}
