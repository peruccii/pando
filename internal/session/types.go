package session

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ISessionService define a interface do serviço de sessões P2P
type ISessionService interface {
	// Host
	CreateSession(hostUserID string, config SessionConfig) (*Session, error)
	ApproveGuest(sessionID, guestUserID string) error
	RejectGuest(sessionID, guestUserID string) error
	EndSession(sessionID string) error

	// Guest
	JoinSession(code string, guestUserID string, guestInfo GuestInfo) (*JoinResult, error)
	MarkGuestConnected(sessionID, guestUserID string) error

	// Query
	GetSession(sessionID string) (*Session, error)
	GetActiveSession(userID string) (*Session, error)
	ListPendingGuests(sessionID string) ([]GuestRequest, error)

	// Permissions
	SetGuestPermission(sessionID, guestUserID, permission string) error
	KickGuest(sessionID, guestUserID string) error
}

// SessionStatus representa o estado de uma sessão
type SessionStatus string

const (
	StatusWaiting SessionStatus = "waiting"
	StatusActive  SessionStatus = "active"
	StatusEnded   SessionStatus = "ended"
)

// SessionMode define o modo de operação da sessão
type SessionMode string

const (
	ModeDocker    SessionMode = "docker"
	ModeLiveShare SessionMode = "liveshare"
)

// GuestStatus define o estado de um guest na sessão
type GuestStatus string

const (
	GuestPending   GuestStatus = "pending"
	GuestApproved  GuestStatus = "approved"
	GuestRejected  GuestStatus = "rejected"
	GuestConnected GuestStatus = "connected"
	GuestExpired   GuestStatus = "expired"
)

// Permission define o nível de permissão
type Permission string

const (
	PermReadOnly  Permission = "read_only"
	PermReadWrite Permission = "read_write"
)

// Session representa uma sessão de colaboração P2P
type Session struct {
	ID         string         `json:"id"`
	Code       string         `json:"code"` // Short code ex: "X92-B4"
	HostUserID string         `json:"hostUserID"`
	HostName   string         `json:"hostName"`
	Status     SessionStatus  `json:"status"`
	Mode       SessionMode    `json:"mode"`
	Guests     []SessionGuest `json:"guests"`
	CreatedAt  time.Time      `json:"createdAt"`
	ExpiresAt  time.Time      `json:"expiresAt"` // Code expira
	Config     SessionConfig  `json:"config"`

	mu sync.RWMutex `json:"-"`
}

// SessionConfig configura uma sessão
type SessionConfig struct {
	MaxGuests      int         `json:"maxGuests"`      // Default: 10
	DefaultPerm    Permission  `json:"defaultPerm"`    // "read_only"
	AllowAnonymous bool        `json:"allowAnonymous"` // Guests sem login GitHub
	Mode           SessionMode `json:"mode"`           // "docker" ou "liveshare"
	WorkspaceID    uint        `json:"workspaceID"`
	WorkspaceName  string      `json:"workspaceName,omitempty"`
	DockerImage    string      `json:"dockerImage,omitempty"`
	ProjectPath    string      `json:"projectPath,omitempty"`
	CodeTTLMinutes int         `json:"codeTTLMinutes"` // Default: 15
}

// SessionGuest representa um guest conectado/pendente
type SessionGuest struct {
	UserID     string      `json:"userID"`
	Name       string      `json:"name"`
	AvatarURL  string      `json:"avatarUrl,omitempty"`
	Permission Permission  `json:"permission"`
	JoinedAt   time.Time   `json:"joinedAt"`
	Status     GuestStatus `json:"status"`
}

// GuestInfo são as informações que o Guest envia ao fazer Join
type GuestInfo struct {
	Name      string `json:"name"`
	Email     string `json:"email,omitempty"`
	AvatarURL string `json:"avatarUrl,omitempty"`
}

// GuestRequest é um pedido de entrada exibido na Waiting Room do Host
type GuestRequest struct {
	UserID    string    `json:"userID"`
	Name      string    `json:"name"`
	Email     string    `json:"email,omitempty"`
	AvatarURL string    `json:"avatarUrl,omitempty"`
	RequestAt time.Time `json:"requestAt"`
}

// JoinResult é o resultado de um JoinSession
type JoinResult struct {
	SessionID         string    `json:"sessionID"`
	SessionCode       string    `json:"sessionCode"`
	HostName          string    `json:"hostName"`
	Status            string    `json:"status"` // "pending" — guest deve aguardar aprovação
	GuestUserID       string    `json:"guestUserID"`
	ApprovalExpiresAt time.Time `json:"approvalExpiresAt,omitempty"`
	WorkspaceName     string    `json:"workspaceName,omitempty"`
}

// SignalMessage é uma mensagem trocada via WebSocket para signaling WebRTC
type SignalMessage struct {
	Type         string `json:"type"` // "sdp_offer", "sdp_answer", "ice_candidate", "guest_request", "guest_approved", "guest_rejected", "session_ended", "permission_change"
	Payload      string `json:"payload,omitempty"`
	TargetUserID string `json:"targetUserID,omitempty"`
	FromUserID   string `json:"fromUserID,omitempty"`
	SessionID    string `json:"sessionID,omitempty"`
}

// SignalingSession mantém estado de signaling WebRTC para uma sessão
type SignalingSession struct {
	SessionID     string
	HostSDP       string              // SDP Offer do Host
	GuestSDPs     map[string]string   // userID → SDP Answer
	ICECandidates map[string][]string // userID → ICE candidates
}

// TURNConfig configura servidor TURN para fallback NAT
type TURNConfig struct {
	URL        string `json:"url"`
	Username   string `json:"username"`
	Credential string `json:"credential"`
}

// ICEServerConfig é a configuração de ICE servers para o frontend
type ICEServerConfig struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// wsConnection wrappea uma conexão WebSocket com metadata
type wsConnection struct {
	conn      *websocket.Conn
	userID    string
	sessionID string
	isHost    bool
	mu        sync.Mutex
}

// SessionEvent é um evento emitido pelo SessionService
type SessionEvent struct {
	Type      string      `json:"type"`
	SessionID string      `json:"sessionID"`
	Data      interface{} `json:"data,omitempty"`
}
