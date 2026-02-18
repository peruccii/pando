package session

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// SignalingService gerencia a troca de SDP e ICE candidates via WebSocket
type SignalingService struct {
	sessionService *Service
	connections    map[string][]*wsConnection // sessionID → connections
	sigSessions    map[string]*SignalingSession
	stunServers    []string
	turnConfig     *TURNConfig
	connObserver   func(sessionID, userID string, isHost bool, connected bool)
	upgrader       websocket.Upgrader
	mu             sync.RWMutex
}

// NewSignalingService cria um novo SignalingService
func NewSignalingService(sessionService *Service) *SignalingService {
	return &SignalingService{
		sessionService: sessionService,
		connections:    make(map[string][]*wsConnection),
		sigSessions:    make(map[string]*SignalingSession),
		stunServers: []string{
			"stun:stun.l.google.com:19302",
			"stun:stun1.l.google.com:19302",
		},
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true }, // TODO: restringir em produção
		},
	}
}

// HandleWebSocket trata conexões WebSocket para signaling
// Endpoint: ws://localhost:PORT/ws/signal?session=SESSION_ID&user=USER_ID&role=host|guest
func (s *SignalingService) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	userID := r.URL.Query().Get("user")
	role := r.URL.Query().Get("role")

	if sessionID == "" || userID == "" {
		http.Error(w, "missing session or user query param", http.StatusBadRequest)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[SIGNALING] Upgrade error: %v", err)
		return
	}

	wsConn := &wsConnection{
		conn:      conn,
		userID:    userID,
		sessionID: sessionID,
		isHost:    role == "host",
	}

	s.registerConnection(sessionID, wsConn)
	defer s.unregisterConnection(sessionID, wsConn)

	log.Printf("[SIGNALING] %s connected to session %s (role: %s)", userID, sessionID, role)

	// Garantir que existe um SignalingSession
	s.ensureSignalingSession(sessionID)

	for {
		_, rawMsg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[SIGNALING] Read error: %v", err)
			}
			break
		}

		var msg SignalMessage
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			log.Printf("[SIGNALING] Invalid message: %v", err)
			continue
		}

		msg.FromUserID = userID
		msg.SessionID = sessionID

		s.handleMessage(sessionID, userID, wsConn.isHost, msg)
	}
}

// handleMessage processa uma mensagem de signaling
func (s *SignalingService) handleMessage(sessionID, userID string, isHost bool, msg SignalMessage) {
	s.mu.Lock()
	sigSession := s.sigSessions[sessionID]
	s.mu.Unlock()

	if sigSession == nil {
		log.Printf("[SIGNALING] No signaling session for %s", sessionID)
		return
	}

	switch msg.Type {
	case "sdp_offer":
		// Host envia SDP Offer
		if !isHost {
			log.Printf("[SIGNALING] Non-host tried to send SDP offer")
			return
		}
		sigSession.HostSDP = msg.Payload
		log.Printf("[SIGNALING] Stored SDP offer from host for session %s", sessionID)

		// Se há um guest aprovado esperando, enviar o offer
		if msg.TargetUserID != "" {
			s.sendToUser(sessionID, msg.TargetUserID, SignalMessage{
				Type:       "sdp_offer",
				Payload:    msg.Payload,
				FromUserID: userID,
			})
		}

	case "sdp_answer":
		// Guest envia SDP Answer
		sigSession.GuestSDPs[userID] = msg.Payload
		log.Printf("[SIGNALING] Stored SDP answer from guest %s", userID)

		// Enviar para o Host
		s.sendToHost(sessionID, SignalMessage{
			Type:       "sdp_answer",
			Payload:    msg.Payload,
			FromUserID: userID,
		})

	case "ice_candidate":
		// Forward ICE candidate para o peer apropriado
		sigSession.ICECandidates[userID] = append(sigSession.ICECandidates[userID], msg.Payload)

		if isHost && msg.TargetUserID != "" {
			// Host → Guest específico
			s.sendToUser(sessionID, msg.TargetUserID, SignalMessage{
				Type:       "ice_candidate",
				Payload:    msg.Payload,
				FromUserID: userID,
			})
		} else if !isHost {
			// Guest → Host
			s.sendToHost(sessionID, SignalMessage{
				Type:       "ice_candidate",
				Payload:    msg.Payload,
				FromUserID: userID,
			})
		}

	case "guest_request":
		// Guest quer entrar — notificar o Host
		s.sendToHost(sessionID, SignalMessage{
			Type:       "guest_request",
			Payload:    msg.Payload,
			FromUserID: userID,
		})

	case "guest_approved":
		// Host aprovou guest — notificar o guest
		s.sendToUser(sessionID, msg.TargetUserID, SignalMessage{
			Type:       "guest_approved",
			FromUserID: userID,
			Payload:    msg.Payload,
		})

	case "guest_rejected":
		// Host rejeitou guest
		s.sendToUser(sessionID, msg.TargetUserID, SignalMessage{
			Type:       "guest_rejected",
			FromUserID: userID,
		})

	case "peer_connected":
		// Notificar que a conexão P2P foi estabelecida
		log.Printf("[SIGNALING] P2P connection established for session %s, user %s", sessionID, userID)

		// Atualizar status do guest se necessário
		if !isHost {
			if session, err := s.sessionService.GetSession(sessionID); err == nil {
				for i, g := range session.Guests {
					if g.UserID == userID {
						session.mu.Lock()
						session.Guests[i].Status = GuestConnected
						session.mu.Unlock()
						break
					}
				}
			}
		}

	case "session_ended":
		// Host encerrou sessão — notificar todos
		s.broadcastToSession(sessionID, userID, SignalMessage{
			Type:       "session_ended",
			FromUserID: userID,
		})

	default:
		log.Printf("[SIGNALING] Unknown message type: %s", msg.Type)
	}
}

// registerConnection registra uma conexão WebSocket
func (s *SignalingService) registerConnection(sessionID string, conn *wsConnection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections[sessionID] = append(s.connections[sessionID], conn)

	if s.connObserver != nil {
		s.connObserver(sessionID, conn.userID, conn.isHost, true)
	}
}

// unregisterConnection remove uma conexão WebSocket
func (s *SignalingService) unregisterConnection(sessionID string, conn *wsConnection) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conns := s.connections[sessionID]
	for i, c := range conns {
		if c == conn {
			s.connections[sessionID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}

	conn.conn.Close()

	if s.connObserver != nil {
		s.connObserver(sessionID, conn.userID, conn.isHost, false)
	}
}

// ensureSignalingSession garante que existe um SignalingSession
func (s *SignalingService) ensureSignalingSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sigSessions[sessionID]; !ok {
		s.sigSessions[sessionID] = &SignalingSession{
			SessionID:     sessionID,
			GuestSDPs:     make(map[string]string),
			ICECandidates: make(map[string][]string),
		}
	}
}

// sendToHost envia uma mensagem para o Host de uma sessão
func (s *SignalingService) sendToHost(sessionID string, msg SignalMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, conn := range s.connections[sessionID] {
		if conn.isHost {
			s.writeJSON(conn, msg)
			return
		}
	}
}

// sendToUser envia uma mensagem para um user específico
func (s *SignalingService) sendToUser(sessionID, targetUserID string, msg SignalMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, conn := range s.connections[sessionID] {
		if conn.userID == targetUserID {
			s.writeJSON(conn, msg)
			return
		}
	}
}

// broadcastToSession envia mensagem para todos na sessão, exceto o sender
func (s *SignalingService) broadcastToSession(sessionID, senderUserID string, msg SignalMessage) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, conn := range s.connections[sessionID] {
		if conn.userID != senderUserID {
			s.writeJSON(conn, msg)
		}
	}
}

// writeJSON escreve uma mensagem JSON no WebSocket com lock
func (s *SignalingService) writeJSON(conn *wsConnection, msg SignalMessage) {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	if err := conn.conn.WriteJSON(msg); err != nil {
		log.Printf("[SIGNALING] Write error to %s: %v", conn.userID, err)
	}
}

// CleanupSession limpa recursos de signaling de uma sessão
func (s *SignalingService) CleanupSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Fechar todas as conexões
	for _, conn := range s.connections[sessionID] {
		conn.conn.Close()
	}
	delete(s.connections, sessionID)
	delete(s.sigSessions, sessionID)
}

// StartSignalingServer inicia o servidor WebSocket de sinalização
// Retorna a porta utilizada
func (s *SignalingService) StartSignalingServer(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/signal", s.HandleWebSocket)

	addr := fmt.Sprintf(":%d", port)
	log.Printf("[SIGNALING] Starting signaling server on %s", addr)

	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("[SIGNALING] Server error: %v", err)
		}
	}()

	return nil
}

// SetConnectionObserver registra callback para entrada/saída de peers na sessão.
func (s *SignalingService) SetConnectionObserver(observer func(sessionID, userID string, isHost bool, connected bool)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connObserver = observer
}

// NotifyPermissionChange envia alteração de permissão em tempo real para um guest.
func (s *SignalingService) NotifyPermissionChange(sessionID, guestUserID, permission string) {
	if sessionID == "" || guestUserID == "" {
		return
	}

	s.sendToUser(sessionID, guestUserID, SignalMessage{
		Type:         "permission_change",
		TargetUserID: guestUserID,
		Payload:      permission,
		FromUserID:   "host",
	})
}
