package session

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const guestApprovalTimeout = 5 * time.Minute

const (
	joinRateLimitWindow      = 30 * time.Second
	joinRateLimitMaxAttempts = 5

	invalidJoinAttemptWindow      = 2 * time.Minute
	invalidJoinLockDuration       = 2 * time.Minute
	invalidJoinLockMaxAttempts    = 5
	minimumRetrySecondsOnLockHint = 1

	joinInvalidReasonInvalidFormat = "invalid_code_format"
	joinInvalidReasonUnknownCode   = "unknown_session_code"
	joinInvalidReasonMissingSess   = "missing_session"
	joinBlockedReasonActiveLock    = "invalid_attempt_lock_active"
)

type joinRateLimitState struct {
	windowStart time.Time
	attempts    int
}

type invalidJoinAttemptState struct {
	windowStart time.Time
	attempts    int
	lockUntil   time.Time
}

// Service implementa ISessionService
type Service struct {
	sessions            map[string]*Session                // sessionID → Session
	codeIndex           map[string]string                  // normalizedCode → sessionID
	hostIndex           map[string]string                  // hostUserID → sessionID (sessão ativa)
	joinRateLimits      map[string]joinRateLimitState      // sessionID|guestUserID -> janela de tentativas de join
	invalidJoinAttempts map[string]invalidJoinAttemptState // guestUserID -> tentativas inválidas + lock temporário
	joinSecurityMetrics JoinSecurityMetrics
	emitEvent           func(eventName string, data interface{})
	mu                  sync.RWMutex
}

// NewService cria um novo SessionService
func NewService(emitEvent func(eventName string, data interface{})) *Service {
	s := &Service{
		sessions:            make(map[string]*Session),
		codeIndex:           make(map[string]string),
		hostIndex:           make(map[string]string),
		joinRateLimits:      make(map[string]joinRateLimitState),
		invalidJoinAttempts: make(map[string]invalidJoinAttemptState),
		emitEvent:           emitEvent,
	}

	// Iniciar goroutine para limpar sessões expiradas
	go s.cleanupLoop()

	return s
}

func guestApprovalExpiresAt(joinedAt time.Time) time.Time {
	return joinedAt.Add(guestApprovalTimeout)
}

func isAnonymousGuestID(guestUserID string) bool {
	normalized := strings.ToLower(strings.TrimSpace(guestUserID))
	return normalized == "" || strings.HasPrefix(normalized, "anonymous-")
}

func buildJoinResult(session *Session, guest SessionGuest) *JoinResult {
	result := &JoinResult{
		SessionID:     session.ID,
		SessionCode:   session.Code,
		HostName:      session.HostName,
		Status:        string(guest.Status),
		GuestUserID:   guest.UserID,
		WorkspaceName: session.Config.WorkspaceName,
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

func buildJoinRateLimitKey(sessionID, guestUserID string) string {
	return sessionID + "|" + strings.ToLower(strings.TrimSpace(guestUserID))
}

func normalizeGuestID(guestUserID string) string {
	return strings.ToLower(strings.TrimSpace(guestUserID))
}

func invalidJoinStateKey(guestUserID string) string {
	key := normalizeGuestID(guestUserID)
	if key == "" {
		return "_anonymous"
	}
	return key
}

func (s *Service) consumeJoinRateLimitAttemptLocked(sessionID, guestUserID string, now time.Time) (time.Duration, bool) {
	key := buildJoinRateLimitKey(sessionID, guestUserID)
	state, exists := s.joinRateLimits[key]

	if !exists || now.Sub(state.windowStart) >= joinRateLimitWindow {
		s.joinRateLimits[key] = joinRateLimitState{
			windowStart: now,
			attempts:    1,
		}
		return 0, true
	}

	if state.attempts >= joinRateLimitMaxAttempts {
		retryAfter := joinRateLimitWindow - now.Sub(state.windowStart)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return retryAfter, false
	}

	state.attempts++
	s.joinRateLimits[key] = state
	return 0, true
}

func (s *Service) clearJoinRateLimitStateForSessionLocked(sessionID string) {
	prefix := sessionID + "|"
	for key := range s.joinRateLimits {
		if strings.HasPrefix(key, prefix) {
			delete(s.joinRateLimits, key)
		}
	}
}

func (s *Service) consumeInvalidJoinAttemptLocked(guestUserID string, now time.Time) (time.Duration, bool) {
	key := invalidJoinStateKey(guestUserID)
	state, exists := s.invalidJoinAttempts[key]
	if exists && now.Before(state.lockUntil) {
		retryAfter := state.lockUntil.Sub(now)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return retryAfter, false
	}

	lockExpired := !state.lockUntil.IsZero() && now.After(state.lockUntil)
	if !exists || now.Sub(state.windowStart) >= invalidJoinAttemptWindow || lockExpired {
		state = invalidJoinAttemptState{
			windowStart: now,
			attempts:    1,
		}
		if state.attempts >= invalidJoinLockMaxAttempts {
			state.lockUntil = now.Add(invalidJoinLockDuration)
			s.invalidJoinAttempts[key] = state
			return invalidJoinLockDuration, false
		}
		s.invalidJoinAttempts[key] = state
		return 0, true
	}

	state.attempts++
	if state.attempts >= invalidJoinLockMaxAttempts {
		state.lockUntil = now.Add(invalidJoinLockDuration)
		s.invalidJoinAttempts[key] = state
		return invalidJoinLockDuration, false
	}

	s.invalidJoinAttempts[key] = state
	return 0, true
}

func (s *Service) getInvalidJoinLockRemainingLocked(guestUserID string, now time.Time) (time.Duration, bool) {
	key := invalidJoinStateKey(guestUserID)
	state, exists := s.invalidJoinAttempts[key]
	if !exists || now.After(state.lockUntil) {
		return 0, false
	}

	retryAfter := state.lockUntil.Sub(now)
	if retryAfter < 0 {
		retryAfter = 0
	}
	return retryAfter, true
}

func (s *Service) clearInvalidJoinAttemptsLocked(guestUserID string) {
	delete(s.invalidJoinAttempts, invalidJoinStateKey(guestUserID))
}

func (s *Service) getInvalidJoinAttemptStateLocked(guestUserID string) (invalidJoinAttemptState, bool) {
	state, exists := s.invalidJoinAttempts[invalidJoinStateKey(guestUserID)]
	return state, exists
}

func (s *Service) resolveSessionIDByCodeLocked(normalizedCode string) string {
	if normalizedCode == "" || !validateCodeFormat(normalizedCode) {
		return ""
	}
	sessionID, ok := s.codeIndex[normalizedCode]
	if !ok {
		return ""
	}
	return sessionID
}

func (s *Service) recordInvalidJoinMetricLocked(reason string, now time.Time) {
	s.joinSecurityMetrics.InvalidAttemptsTotal++
	s.joinSecurityMetrics.LastInvalidAttemptAt = now

	switch reason {
	case joinInvalidReasonInvalidFormat:
		s.joinSecurityMetrics.InvalidFormatAttemptsTotal++
	case joinInvalidReasonUnknownCode:
		s.joinSecurityMetrics.UnknownCodeAttemptsTotal++
	case joinInvalidReasonMissingSess:
		s.joinSecurityMetrics.MissingSessionAttemptsTotal++
	}
}

func (s *Service) recordBlockedJoinMetricLocked(now time.Time, lockout bool) {
	s.joinSecurityMetrics.BlockedAttemptsTotal++
	s.joinSecurityMetrics.LastBlockedAt = now
	if lockout {
		s.joinSecurityMetrics.LockoutsTotal++
	}
}

func (s *Service) emitJoinInvalidAttemptLocked(
	sessionID string,
	guestUserID string,
	reason string,
	attempt int,
	retryAfter time.Duration,
	locked bool,
	now time.Time,
) {
	if s.emitEvent == nil {
		return
	}
	s.emitEvent(JoinSecurityEventInvalidAttempt, JoinSecurityEvent{
		SessionID:         sessionID,
		GuestUserID:       guestUserID,
		Reason:            reason,
		Attempt:           attempt,
		MaxAttempts:       invalidJoinLockMaxAttempts,
		RetryAfterSeconds: retrySecondsHint(retryAfter),
		Locked:            locked,
		OccurredAt:        now,
	})
}

func (s *Service) emitJoinBlockedLocked(
	sessionID string,
	guestUserID string,
	reason string,
	attempt int,
	retryAfter time.Duration,
	now time.Time,
) {
	if s.emitEvent == nil {
		return
	}
	s.emitEvent(JoinSecurityEventBlocked, JoinSecurityEvent{
		SessionID:         sessionID,
		GuestUserID:       guestUserID,
		Reason:            reason,
		Attempt:           attempt,
		MaxAttempts:       invalidJoinLockMaxAttempts,
		RetryAfterSeconds: retrySecondsHint(retryAfter),
		Locked:            true,
		OccurredAt:        now,
	})
}

func (s *Service) countActiveInvalidLocksLocked(now time.Time) int {
	count := 0
	for _, state := range s.invalidJoinAttempts {
		if now.Before(state.lockUntil) {
			count++
		}
	}
	return count
}

func retrySecondsHint(duration time.Duration) int {
	seconds := int(duration.Seconds())
	if duration > 0 && seconds == 0 {
		return minimumRetrySecondsOnLockHint
	}
	return seconds
}

func (s *Service) cleanupInvalidJoinAttemptsLocked(now time.Time) {
	for key, state := range s.invalidJoinAttempts {
		if now.Before(state.lockUntil) {
			continue
		}
		if now.Sub(state.windowStart) < invalidJoinAttemptWindow {
			continue
		}
		delete(s.invalidJoinAttempts, key)
	}
}

// GetJoinSecurityMetrics retorna métricas agregadas de tentativas inválidas e bloqueios.
func (s *Service) GetJoinSecurityMetrics() JoinSecurityMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := s.joinSecurityMetrics
	metrics.ActiveLocks = s.countActiveInvalidLocksLocked(time.Now())
	return metrics
}

func (s *Service) generateUniqueShortCodeLocked() (string, error) {
	for i := 0; i < 10; i++ {
		code, err := generateShortCode()
		if err != nil {
			return "", fmt.Errorf("generating short code: %w", err)
		}
		normalized := normalizeCode(code)
		if _, exists := s.codeIndex[normalized]; !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("failed to generate unique short code after 10 attempts")
}

func (s *Service) deactivateJoinCodeLocked(session *Session, clearCode bool) {
	if session == nil {
		return
	}
	if session.Code != "" {
		delete(s.codeIndex, normalizeCode(session.Code))
	}
	session.AllowNewJoins = false
	if clearCode {
		session.Code = ""
	}
}

func (s *Service) activateJoinCodeLocked(session *Session, now time.Time, forceRotate bool) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	if session.Status == StatusEnded {
		return fmt.Errorf("session has ended")
	}

	if !forceRotate && session.Code != "" && now.Before(session.ExpiresAt) {
		normalized := normalizeCode(session.Code)
		if current, exists := s.codeIndex[normalized]; !exists || current == session.ID {
			s.codeIndex[normalized] = session.ID
			session.AllowNewJoins = true
			return nil
		}
	}

	s.deactivateJoinCodeLocked(session, false)

	code, err := s.generateUniqueShortCodeLocked()
	if err != nil {
		return err
	}
	if session.Config.CodeTTLMinutes <= 0 {
		session.Config.CodeTTLMinutes = 15
	}

	session.Code = code
	session.ExpiresAt = now.Add(time.Duration(session.Config.CodeTTLMinutes) * time.Minute)
	session.AllowNewJoins = true
	s.codeIndex[normalizeCode(code)] = session.ID
	return nil
}

func (s *Service) refreshAllowNewJoinsLocked(session *Session, now time.Time) {
	if session == nil {
		return
	}
	if session.Status == StatusEnded || session.Code == "" || !now.Before(session.ExpiresAt) {
		session.AllowNewJoins = false
		return
	}
	current, exists := s.codeIndex[normalizeCode(session.Code)]
	session.AllowNewJoins = exists && current == session.ID
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

	code, err := s.generateUniqueShortCodeLocked()
	if err != nil {
		return nil, err
	}

	session := &Session{
		ID:            uuid.New().String(),
		Code:          code,
		AllowNewJoins: true,
		HostUserID:    hostUserID,
		HostName:      hostUserID, // será substituído pelo nome real
		Status:        StatusWaiting,
		Mode:          config.Mode,
		Guests:        []SessionGuest{},
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(time.Duration(config.CodeTTLMinutes) * time.Minute),
		Config:        config,
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

	if retryAfter, locked := s.getInvalidJoinLockRemainingLocked(guestUserID, now); locked {
		sessionID := s.resolveSessionIDByCodeLocked(normalized)
		attempt := 0
		if state, exists := s.getInvalidJoinAttemptStateLocked(guestUserID); exists {
			attempt = state.attempts
		}
		s.recordBlockedJoinMetricLocked(now, false)
		s.emitJoinBlockedLocked(sessionID, guestUserID, joinBlockedReasonActiveLock, attempt, retryAfter, now)
		return nil, fmt.Errorf("too many invalid join attempts, try again in %ds", retrySecondsHint(retryAfter))
	}

	if !validateCodeFormat(normalized) {
		retryAfter, allowed := s.consumeInvalidJoinAttemptLocked(guestUserID, now)
		attempt := 0
		if state, exists := s.getInvalidJoinAttemptStateLocked(guestUserID); exists {
			attempt = state.attempts
		}
		s.recordInvalidJoinMetricLocked(joinInvalidReasonInvalidFormat, now)
		s.emitJoinInvalidAttemptLocked("", guestUserID, joinInvalidReasonInvalidFormat, attempt, retryAfter, !allowed, now)
		if !allowed {
			s.recordBlockedJoinMetricLocked(now, true)
			s.emitJoinBlockedLocked("", guestUserID, joinInvalidReasonInvalidFormat, attempt, retryAfter, now)
			return nil, fmt.Errorf("too many invalid join attempts, try again in %ds", retrySecondsHint(retryAfter))
		}
		return nil, fmt.Errorf("invalid code format: expected XXXX-XXX")
	}

	sessionID, ok := s.codeIndex[normalized]
	if !ok {
		retryAfter, allowed := s.consumeInvalidJoinAttemptLocked(guestUserID, now)
		attempt := 0
		if state, exists := s.getInvalidJoinAttemptStateLocked(guestUserID); exists {
			attempt = state.attempts
		}
		s.recordInvalidJoinMetricLocked(joinInvalidReasonUnknownCode, now)
		s.emitJoinInvalidAttemptLocked("", guestUserID, joinInvalidReasonUnknownCode, attempt, retryAfter, !allowed, now)
		if !allowed {
			s.recordBlockedJoinMetricLocked(now, true)
			s.emitJoinBlockedLocked("", guestUserID, joinInvalidReasonUnknownCode, attempt, retryAfter, now)
			return nil, fmt.Errorf("too many invalid join attempts, try again in %ds", retrySecondsHint(retryAfter))
		}
		return nil, fmt.Errorf("session not found for code: %s", code)
	}

	session, ok := s.sessions[sessionID]
	if !ok {
		retryAfter, allowed := s.consumeInvalidJoinAttemptLocked(guestUserID, now)
		attempt := 0
		if state, exists := s.getInvalidJoinAttemptStateLocked(guestUserID); exists {
			attempt = state.attempts
		}
		s.recordInvalidJoinMetricLocked(joinInvalidReasonMissingSess, now)
		s.emitJoinInvalidAttemptLocked(sessionID, guestUserID, joinInvalidReasonMissingSess, attempt, retryAfter, !allowed, now)
		if !allowed {
			s.recordBlockedJoinMetricLocked(now, true)
			s.emitJoinBlockedLocked(sessionID, guestUserID, joinInvalidReasonMissingSess, attempt, retryAfter, now)
			return nil, fmt.Errorf("too many invalid join attempts, try again in %ds", retrySecondsHint(retryAfter))
		}
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	s.clearInvalidJoinAttemptsLocked(guestUserID)

	// Verificar se o código expirou
	if now.After(session.ExpiresAt) {
		return nil, fmt.Errorf("session code has expired")
	}

	// Verificar se a sessão ainda aceita guests
	if session.Status == StatusEnded {
		return nil, fmt.Errorf("session has ended")
	}
	s.refreshAllowNewJoinsLocked(session, now)
	if !session.AllowNewJoins {
		return nil, fmt.Errorf("session not found for code: %s", code)
	}
	if !session.Config.AllowAnonymous && isAnonymousGuestID(guestUserID) {
		return nil, fmt.Errorf("anonymous guests are not allowed in this session")
	}

	if retryAfter, allowed := s.consumeJoinRateLimitAttemptLocked(session.ID, guestUserID, now); !allowed {
		retrySeconds := int(retryAfter.Seconds())
		if retryAfter > 0 && retrySeconds == 0 {
			retrySeconds = 1
		}
		return nil, fmt.Errorf("join rate limit exceeded for this session and guestUserID, retry in %ds", retrySeconds)
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

// MarkGuestConnected marca um guest aprovado como conectado e invalida o código após a primeira conexão real.
func (s *Service) MarkGuestConnected(sessionID, guestUserID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	hasConnectedGuest := false
	for _, guest := range session.Guests {
		if guest.Status == GuestConnected {
			hasConnectedGuest = true
			break
		}
	}

	for i, guest := range session.Guests {
		if guest.UserID != guestUserID {
			continue
		}

		switch guest.Status {
		case GuestConnected:
			return nil
		case GuestApproved:
			// fluxo válido.
		case GuestPending:
			return fmt.Errorf("guest %s is pending approval", guestUserID)
		case GuestRejected:
			return fmt.Errorf("guest %s was rejected", guestUserID)
		case GuestExpired:
			return fmt.Errorf("guest %s request has expired", guestUserID)
		default:
			return fmt.Errorf("guest cannot connect from status: %s", guest.Status)
		}

		session.Guests[i].Status = GuestConnected
		if session.Status == StatusWaiting {
			session.Status = StatusActive
		}
		if !hasConnectedGuest {
			delete(s.codeIndex, normalizeCode(session.Code))
			session.AllowNewJoins = false
		}

		log.Printf("[SESSION] Guest %s connected in session %s", guestUserID, session.Code)
		if s.emitEvent != nil {
			s.emitEvent("session:guest_connected", map[string]interface{}{
				"sessionID":   sessionID,
				"guestUserID": guestUserID,
			})
		}
		return nil
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
	session.AllowNewJoins = false
	delete(s.hostIndex, session.HostUserID)
	s.clearJoinRateLimitStateForSessionLocked(sessionID)

	log.Printf("[SESSION] Session %s ended", session.Code)

	// Notificar todos os participantes
	if s.emitEvent != nil {
		s.emitEvent("session:ended", map[string]interface{}{
			"sessionID": sessionID,
		})
	}

	return nil
}

// RegenerateCode gera um novo código para a sessão e reabre o join.
func (s *Service) RegenerateCode(sessionID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	now := time.Now()
	if err := s.activateJoinCodeLocked(session, now, true); err != nil {
		return nil, err
	}

	log.Printf("[SESSION] Session %s regenerated join code", sessionID)
	return session, nil
}

// RevokeCode invalida o código atual da sessão.
func (s *Service) RevokeCode(sessionID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if session.Status == StatusEnded {
		return nil, fmt.Errorf("session has ended")
	}

	s.deactivateJoinCodeLocked(session, true)
	log.Printf("[SESSION] Session %s join code revoked", sessionID)
	return session, nil
}

// SetAllowNewJoins habilita/desabilita novos joins na sessão.
func (s *Service) SetAllowNewJoins(sessionID string, allow bool) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if session.Status == StatusEnded {
		return nil, fmt.Errorf("session has ended")
	}

	now := time.Now()
	if allow {
		if err := s.activateJoinCodeLocked(session, now, false); err != nil {
			return nil, err
		}
		log.Printf("[SESSION] Session %s enabled new joins", sessionID)
		return session, nil
	}

	s.deactivateJoinCodeLocked(session, false)
	log.Printf("[SESSION] Session %s disabled new joins", sessionID)
	return session, nil
}

// GetSession retorna detalhes de uma sessão
func (s *Service) GetSession(sessionID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	now := time.Now()
	s.expirePendingGuestsLocked(session, now)
	s.refreshAllowNewJoinsLocked(session, now)

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

	// Compatibilidade: sessões antigas em waiting não tinham allowNewJoins persistido.
	// Nesse caso, mantém join habilitado enquanto o código ainda for válido.
	now := time.Now()
	shouldRestoreCode := restored.Code != "" &&
		now.Before(restored.ExpiresAt) &&
		(restored.Status == StatusWaiting || restored.AllowNewJoins)
	if shouldRestoreCode {
		s.codeIndex[normalizeCode(restored.Code)] = restored.ID
	}
	s.refreshAllowNewJoinsLocked(restored, now)

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

	now := time.Now()
	s.expirePendingGuestsLocked(session, now)
	s.refreshAllowNewJoinsLocked(session, now)

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
				s.clearJoinRateLimitStateForSessionLocked(id)
				delete(s.sessions, id)
				continue
			}

			if session.Code != "" && now.After(session.ExpiresAt) {
				delete(s.codeIndex, normalizeCode(session.Code))
				session.AllowNewJoins = false
			}

			// Limpar sessões encerradas há mais de 1 hora
			if session.Status == StatusEnded && now.After(session.CreatedAt.Add(1*time.Hour)) {
				s.clearJoinRateLimitStateForSessionLocked(id)
				delete(s.sessions, id)
			}
		}
		s.cleanupInvalidJoinAttemptsLocked(now)
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
