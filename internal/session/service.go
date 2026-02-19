package session

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

const guestApprovalTimeout = 5 * time.Minute

// Service implementa ISessionService
type Service struct {
	sessions  map[string]*Session // sessionID → Session
	codeIndex map[string]string   // normalizedCode → sessionID
	hostIndex map[string]string   // hostUserID → sessionID (sessão ativa)
	emitEvent func(eventName string, data interface{})
	mu        sync.RWMutex
}

// NewService cria um novo SessionService
func NewService(emitEvent func(eventName string, data interface{})) *Service {
	s := &Service{
		sessions:  make(map[string]*Session),
		codeIndex: make(map[string]string),
		hostIndex: make(map[string]string),
		emitEvent: emitEvent,
	}

	// Iniciar goroutine para limpar sessões expiradas
	go s.cleanupLoop()

	return s
}

func guestApprovalExpiresAt(joinedAt time.Time) time.Time {
	return joinedAt.Add(guestApprovalTimeout)
}

func buildJoinResult(session *Session, guest SessionGuest) *JoinResult {
	result := &JoinResult{
		SessionID:   session.ID,
		SessionCode: session.Code,
		HostName:    session.HostName,
		Status:      string(guest.Status),
		GuestUserID: guest.UserID,
	}
	if guest.Status == GuestPending {
		result.ApprovalExpiresAt = guestApprovalExpiresAt(guest.JoinedAt)
	}
	return result
}

func (s *Service) emitGuestExpired(sessionID, guestUserID string) {
	if s.emitEvent == nil {
		return
	}
	s.emitEvent("session:guest_expired", map[string]interface{}{
		"sessionID":   sessionID,
		"guestUserID": guestUserID,
	})
}

func (s *Service) expirePendingGuestsLocked(session *Session, now time.Time) {
	for i := range session.Guests {
		if session.Guests[i].Status != GuestPending {
			continue
		}
		if now.Before(guestApprovalExpiresAt(session.Guests[i].JoinedAt)) {
			continue
		}
		session.Guests[i].Status = GuestExpired
		log.Printf("[SESSION] Guest %s request expired in session %s", session.Guests[i].UserID, session.Code)
		s.emitGuestExpired(session.ID, session.Guests[i].UserID)
	}
}

// CreateSession cria uma nova sessão de colaboração
func (s *Service) CreateSession(hostUserID string, config SessionConfig) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verificar se o host já tem uma sessão ativa
	if existingID, ok := s.hostIndex[hostUserID]; ok {
		if existing, ok := s.sessions[existingID]; ok && existing.Status != StatusEnded {
			return nil, fmt.Errorf("host already has an active session: %s", existing.Code)
		}
	}

	// Aplicar defaults
	if config.MaxGuests <= 0 {
		config.MaxGuests = 10
	}
	if config.DefaultPerm == "" {
		config.DefaultPerm = PermReadOnly
	}
	if config.Mode == "" {
		config.Mode = ModeLiveShare
	}
	if config.CodeTTLMinutes <= 0 {
		config.CodeTTLMinutes = 15
	}

	// Gerar código único
	var code string
	for i := 0; i < 10; i++ { // máx 10 tentativas
		c, err := generateShortCode()
		if err != nil {
			return nil, fmt.Errorf("generating short code: %w", err)
		}
		normalized := normalizeCode(c)
		if _, exists := s.codeIndex[normalized]; !exists {
			code = c
			break
		}
	}
	if code == "" {
		return nil, fmt.Errorf("failed to generate unique short code after 10 attempts")
	}

	session := &Session{
		ID:         uuid.New().String(),
		Code:       code,
		HostUserID: hostUserID,
		HostName:   hostUserID, // será substituído pelo nome real
		Status:     StatusWaiting,
		Mode:       config.Mode,
		Guests:     []SessionGuest{},
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(time.Duration(config.CodeTTLMinutes) * time.Minute),
		Config:     config,
	}

	s.sessions[session.ID] = session
	s.codeIndex[normalizeCode(code)] = session.ID
	s.hostIndex[hostUserID] = session.ID

	log.Printf("[SESSION] Created session %s (code: %s) for host %s", session.ID, code, hostUserID)

	// Emitir evento para o frontend do host
	if s.emitEvent != nil {
		s.emitEvent("session:created", session)
	}

	return session, nil
}

// JoinSession permite que um guest entre na sessão via código
func (s *Service) JoinSession(code string, guestUserID string, guestInfo GuestInfo) (*JoinResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	normalized := normalizeCode(code)

	if !validateCodeFormat(normalized) {
		return nil, fmt.Errorf("invalid code format: expected XXX-YY")
	}

	sessionID, ok := s.codeIndex[normalized]
	if !ok {
		return nil, fmt.Errorf("session not found for code: %s", code)
	}

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// Verificar se o código expirou
	if now.After(session.ExpiresAt) {
		return nil, fmt.Errorf("session code has expired")
	}

	// Verificar se a sessão ainda aceita guests
	if session.Status == StatusEnded {
		return nil, fmt.Errorf("session has ended")
	}

	// Expirar pedidos pendentes com mais de 5 minutos.
	s.expirePendingGuestsLocked(session, now)

	// Verificar se o guest já está na sessão
	existingGuestIndex := -1
	for i, g := range session.Guests {
		if g.UserID == guestUserID {
			existingGuestIndex = i
			if g.Status == GuestRejected {
				return nil, fmt.Errorf("you were rejected from this session")
			}
			if g.Status != GuestExpired {
				return buildJoinResult(session, g), nil
			}
			break
		}
	}

	// Verificar max guests
	activeGuests := 0
	for _, g := range session.Guests {
		if g.Status == GuestApproved || g.Status == GuestConnected {
			activeGuests++
		}
	}
	if activeGuests >= session.Config.MaxGuests {
		return nil, fmt.Errorf("session is full (max %d guests)", session.Config.MaxGuests)
	}

	joinedAt := time.Now()
	guest := SessionGuest{
		UserID:     guestUserID,
		Name:       guestInfo.Name,
		AvatarURL:  guestInfo.AvatarURL,
		Permission: session.Config.DefaultPerm,
		JoinedAt:   joinedAt,
		Status:     GuestPending,
	}

	if existingGuestIndex >= 0 {
		session.Guests[existingGuestIndex] = guest
		log.Printf("[SESSION] Guest %s (%s) renewed join request for session %s", guestUserID, guestInfo.Name, session.Code)
	} else {
		session.Guests = append(session.Guests, guest)
		log.Printf("[SESSION] Guest %s (%s) requesting to join session %s", guestUserID, guestInfo.Name, session.Code)
	}

	// Notificar o host que um guest quer entrar
	if s.emitEvent != nil {
		s.emitEvent("session:guest_request", GuestRequest{
			UserID:    guestUserID,
			Name:      guestInfo.Name,
			Email:     guestInfo.Email,
			AvatarURL: guestInfo.AvatarURL,
			RequestAt: joinedAt,
		})
	}

	return buildJoinResult(session, guest), nil
}

// ApproveGuest aprova um guest para entrar na sessão
func (s *Service) ApproveGuest(sessionID, guestUserID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	s.expirePendingGuestsLocked(session, time.Now())

	for i, g := range session.Guests {
		if g.UserID == guestUserID {
			switch g.Status {
			case GuestExpired:
				return fmt.Errorf("guest request approval window expired")
			case GuestRejected:
				return fmt.Errorf("guest request was rejected")
			case GuestApproved, GuestConnected:
				return nil
			case GuestPending:
				// segue para aprovação.
			default:
				return fmt.Errorf("guest cannot be approved from status: %s", g.Status)
			}

			session.Guests[i].Status = GuestApproved
			log.Printf("[SESSION] Guest %s approved in session %s", guestUserID, session.Code)

			// Ativar sessão se era a primeira aprovação
			if session.Status == StatusWaiting {
				session.Status = StatusActive
				// Invalidar o código (uso único)
				delete(s.codeIndex, normalizeCode(session.Code))
			}

			// Notificar o guest que foi aprovado
			if s.emitEvent != nil {
				s.emitEvent("session:guest_approved", map[string]interface{}{
					"sessionID":   sessionID,
					"guestUserID": guestUserID,
					"session":     session,
				})
			}

			return nil
		}
	}

	return fmt.Errorf("guest not found: %s", guestUserID)
}

// RejectGuest rejeita um guest
func (s *Service) RejectGuest(sessionID, guestUserID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	s.expirePendingGuestsLocked(session, time.Now())

	for i, g := range session.Guests {
		if g.UserID == guestUserID {
			session.Guests[i].Status = GuestRejected
			log.Printf("[SESSION] Guest %s rejected in session %s", guestUserID, session.Code)

			// Notificar guest
			if s.emitEvent != nil {
				s.emitEvent("session:guest_rejected", map[string]interface{}{
					"sessionID":   sessionID,
					"guestUserID": guestUserID,
				})
			}

			return nil
		}
	}

	return fmt.Errorf("guest not found: %s", guestUserID)
}

// EndSession encerra a sessão e desconecta todos
func (s *Service) EndSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.Status = StatusEnded

	// Limpar índices
	delete(s.codeIndex, normalizeCode(session.Code))
	delete(s.hostIndex, session.HostUserID)

	log.Printf("[SESSION] Session %s ended", session.Code)

	// Notificar todos os participantes
	if s.emitEvent != nil {
		s.emitEvent("session:ended", map[string]interface{}{
			"sessionID": sessionID,
		})
	}

	return nil
}

// GetSession retorna detalhes de uma sessão
func (s *Service) GetSession(sessionID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	s.expirePendingGuestsLocked(session, time.Now())

	return session, nil
}

// RestoreSession restaura uma sessão previamente persistida no estado em memória.
func (s *Service) RestoreSession(restored *Session) error {
	if restored == nil {
		return fmt.Errorf("restored session is nil")
	}
	if restored.ID == "" {
		return fmt.Errorf("restored session has empty id")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[restored.ID] = restored

	// Limpar índices antigos apontando para esta sessão.
	for code, sessionID := range s.codeIndex {
		if sessionID == restored.ID {
			delete(s.codeIndex, code)
		}
	}

	if restored.Status != StatusEnded && restored.HostUserID != "" {
		s.hostIndex[restored.HostUserID] = restored.ID
	}

	// Código só deve voltar a ser aceito se ainda estiver em waiting e não expirado.
	if restored.Status == StatusWaiting && restored.Code != "" && time.Now().Before(restored.ExpiresAt) {
		s.codeIndex[normalizeCode(restored.Code)] = restored.ID
	}

	return nil
}

// GetActiveSession retorna a sessão ativa de um usuário (host)
func (s *Service) GetActiveSession(userID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID, ok := s.hostIndex[userID]
	if !ok {
		return nil, fmt.Errorf("no active session for user: %s", userID)
	}

	session, ok := s.sessions[sessionID]
	if !ok || session.Status == StatusEnded {
		return nil, fmt.Errorf("no active session for user: %s", userID)
	}

	s.expirePendingGuestsLocked(session, time.Now())

	return session, nil
}

// ListPendingGuests lista pedidos de entrada pendentes
func (s *Service) ListPendingGuests(sessionID string) ([]GuestRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	s.expirePendingGuestsLocked(session, time.Now())

	var pending []GuestRequest
	for _, g := range session.Guests {
		if g.Status == GuestPending {
			pending = append(pending, GuestRequest{
				UserID:    g.UserID,
				Name:      g.Name,
				AvatarURL: g.AvatarURL,
				RequestAt: g.JoinedAt,
			})
		}
	}

	return pending, nil
}

// SetGuestPermission altera a permissão de um guest
func (s *Service) SetGuestPermission(sessionID, guestUserID, permission string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	perm := Permission(permission)
	if perm != PermReadOnly && perm != PermReadWrite {
		return fmt.Errorf("invalid permission: %s", permission)
	}

	for i, g := range session.Guests {
		if g.UserID == guestUserID {
			session.Guests[i].Permission = perm
			log.Printf("[SESSION] Guest %s permission set to %s in session %s", guestUserID, permission, session.Code)

			if s.emitEvent != nil {
				s.emitEvent("session:permission_changed", map[string]interface{}{
					"sessionID":   sessionID,
					"guestUserID": guestUserID,
					"permission":  permission,
				})
			}

			return nil
		}
	}

	return fmt.Errorf("guest not found: %s", guestUserID)
}

// KickGuest remove um guest da sessão
func (s *Service) KickGuest(sessionID, guestUserID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	newGuests := make([]SessionGuest, 0, len(session.Guests))
	found := false
	for _, g := range session.Guests {
		if g.UserID == guestUserID {
			found = true
			continue
		}
		newGuests = append(newGuests, g)
	}

	if !found {
		return fmt.Errorf("guest not found: %s", guestUserID)
	}

	session.Guests = newGuests
	log.Printf("[SESSION] Guest %s kicked from session %s", guestUserID, session.Code)

	if s.emitEvent != nil {
		s.emitEvent("session:guest_kicked", map[string]interface{}{
			"sessionID":   sessionID,
			"guestUserID": guestUserID,
		})
	}

	return nil
}

// cleanupLoop limpa sessões expiradas periodicamente
func (s *Service) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, session := range s.sessions {
			s.expirePendingGuestsLocked(session, now)

			// Limpar sessões com código expirado que nunca se tornaram ativas
			if session.Status == StatusWaiting && now.After(session.ExpiresAt) {
				log.Printf("[SESSION] Cleaning up expired session %s (code: %s)", id, session.Code)
				delete(s.codeIndex, normalizeCode(session.Code))
				delete(s.hostIndex, session.HostUserID)
				delete(s.sessions, id)
				continue
			}
			// Limpar sessões encerradas há mais de 1 hora
			if session.Status == StatusEnded && now.After(session.CreatedAt.Add(1*time.Hour)) {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

// GetICEServers retorna a configuração de ICE servers para o frontend
func (s *Service) GetICEServers() []ICEServerConfig {
	return []ICEServerConfig{
		{
			URLs: []string{
				"stun:stun.l.google.com:19302",
				"stun:stun1.l.google.com:19302",
			},
		},
		// TURN fallback — pode ser configurado via env vars
		// {
		//     URLs:       []string{"turn:turn.orch.app:3478"},
		//     Username:   "orch",
		//     Credential: "secret",
		// },
	}
}
