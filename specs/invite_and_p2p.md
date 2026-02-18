# Spec: Sistema de Convite & Conexão P2P (WebRTC)

> **Módulo**: 6 — Invite & P2P  
> **Status**: Draft  
> **PRD Ref**: Seção 12  
> **Última Atualização**: 12 de Fevereiro de 2026

---

## 1. Objetivo

Implementar sistema de convite via código curto + conexão direta P2P via WebRTC para streaming de terminal e dados de colaboração. O backend atua como **Signaling Server** temporário; após o handshake, o backend sai da jogada.

---

## 2. Fluxo de Convite — "Handshake"

```
1. Host clica "Start Session"
2. Backend gera Short Code (ex: X92-B4) + registra sessão
3. Host envia código para Guest (Slack/WhatsApp/etc)
4. Guest abre app → "Join Session" → digita X92-B4
5. Backend valida código + envia evento para Host: "Fulano quer entrar"
6. Host aprova no "Waiting Room"
7. Backend troca SDP Offer/Answer entre Host e Guest
8. Conexão WebRTC P2P estabelecida
9. Backend sai da jogada para dados pesados
```

---

## 3. Session Service (Backend Go)

### 3.1 Interface

```go
type ISessionService interface {
    // Host
    CreateSession(hostUserID string, config SessionConfig) (*Session, error)
    ApproveGuest(sessionID, guestUserID string) error
    RejectGuest(sessionID, guestUserID string) error
    EndSession(sessionID string) error

    // Guest
    JoinSession(code string, guestUserID string) (*JoinRequest, error)

    // Query
    GetSession(sessionID string) (*Session, error)
    GetActiveSession(userID string) (*Session, error)
    ListPendingGuests(sessionID string) ([]GuestRequest, error)
}

type Session struct {
    ID          string
    Code        string          // "X92-B4"
    HostUserID  string
    Status      string          // "waiting", "active", "ended"
    Mode        string          // "docker", "liveshare"
    Guests      []SessionGuest
    CreatedAt   time.Time
    ExpiresAt   time.Time       // Code expira em 15 min
    Config      SessionConfig
}

type SessionConfig struct {
    MaxGuests        int    // Default: 10
    DefaultPerm      string // "read_only"
    AllowAnonymous   bool   // Guests sem login GitHub
    DockerImage      string // Se mode=docker
    ProjectPath      string
}

type SessionGuest struct {
    UserID     string
    Name       string
    AvatarURL  string
    Permission string // "read_only", "read_write"
    JoinedAt   time.Time
    Status     string // "pending", "approved", "rejected", "connected"
}

type GuestRequest struct {
    UserID    string
    Name      string
    Email     string
    AvatarURL string
    RequestAt time.Time
}
```

### 3.2 Short Code Generator

```go
func generateShortCode() string {
    // Formato: XXX-YY (fácil de ditar por voz)
    chars := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Sem 0/O/1/I
    
    part1 := make([]byte, 3)
    part2 := make([]byte, 2)
    
    for i := range part1 {
        n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
        part1[i] = chars[n.Int64()]
    }
    for i := range part2 {
        n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
        part2[i] = chars[n.Int64()]
    }
    
    return string(part1) + "-" + string(part2)
}
```

### 3.3 Regras do Código

| Requisito      | Especificação                                |
| --------------- | -------------------------------------------- |
| Formato         | `XXX-YY` (letras/números, sem ambíguos)       |
| Expiração       | 15 min após criação (configurável)            |
| Uso Único       | Invalidado após conexão bem-sucedida          |
| Charset         | `ABCDEFGHJKLMNPQRSTUVWXYZ23456789`            |
| Case-insensitive| Guest pode digitar em minúsculas              |

---

## 4. Signaling Server (WebRTC)

### 4.1 Fluxo SDP

```
Host                    Backend                   Guest
  │                        │                        │
  │── CreateSession ──────▶│                        │
  │◀── Session + Code ─────│                        │
  │                        │                        │
  │   SDP Offer            │                        │
  │── StoreSDP ───────────▶│                        │
  │                        │                        │
  │                        │◀── JoinSession(code) ──│
  │                        │                        │
  │◀── "Guest quer entrar" │                        │
  │                        │                        │
  │── ApproveGuest ───────▶│                        │
  │                        │── SDP Offer ──────────▶│
  │                        │                        │
  │                        │◀── SDP Answer ─────────│
  │◀── SDP Answer ─────────│                        │
  │                        │                        │
  │◄══════════════ WebRTC P2P ═════════════════════▶│
  │         (Backend sai da jogada)                  │
```

### 4.2 ICE Candidates

```go
type SignalingService struct {
    sessions   map[string]*SignalingSession
    stunServer string // "stun:stun.l.google.com:19302"
    turnServer *TURNConfig // Fallback para NAT restritivo
}

type TURNConfig struct {
    URL        string
    Username   string
    Credential string
}

type SignalingSession struct {
    SessionID    string
    HostSDP      string // SDP Offer do Host
    GuestSDPs    map[string]string // userID → SDP Answer
    ICECandidates map[string][]string // userID → ICE candidates
}
```

### 4.3 WebSocket para Sinalização

```go
// Endpoints WebSocket para troca de sinais em tempo real
// ws://localhost:PORT/ws/signal?session=SESSION_ID&user=USER_ID

func (s *SignalingService) HandleWebSocket(conn *websocket.Conn, sessionID, userID string) {
    for {
        var msg SignalMessage
        err := conn.ReadJSON(&msg)
        if err != nil { break }

        switch msg.Type {
        case "sdp_offer":
            s.sessions[sessionID].HostSDP = msg.Payload
        case "sdp_answer":
            s.sessions[sessionID].GuestSDPs[userID] = msg.Payload
            // Notificar Host
            s.notifyHost(sessionID, msg)
        case "ice_candidate":
            s.sessions[sessionID].ICECandidates[userID] = append(
                s.sessions[sessionID].ICECandidates[userID], msg.Payload)
            // Forward para o peer
            s.forwardToPeer(sessionID, userID, msg)
        case "guest_request":
            s.notifyHost(sessionID, msg)
        case "guest_approved":
            s.notifyGuest(sessionID, msg.TargetUserID, msg)
        }
    }
}

type SignalMessage struct {
    Type         string `json:"type"`
    Payload      string `json:"payload"`
    TargetUserID string `json:"targetUserID,omitempty"`
}
```

---

## 5. WebRTC Data Channels (Frontend)

### 5.1 Configuração

```typescript
const rtcConfig: RTCConfiguration = {
    iceServers: [
        { urls: 'stun:stun.l.google.com:19302' },
        { urls: 'stun:stun1.l.google.com:19302' },
        // TURN server como fallback
        {
            urls: 'turn:turn.orch.app:3478',
            username: 'orch',
            credential: 'secret',
        },
    ],
}
```

### 5.2 Data Channels

| Channel             | Uso                                      | Prioridade |
| -------------------- | ---------------------------------------- | ---------- |
| `terminal-io`        | Stream de stdin/stdout do terminal       | Alta       |
| `github-state`       | Hydrated State do GitHub                  | Média      |
| `cursor-awareness`   | Posição dos cursores dos guests           | Baixa      |
| `control`            | Permissões, resize, scroll sync           | Alta       |
| `chat`               | Chat textual entre participantes          | Baixa      |

### 5.3 Reconexão Automática

```typescript
class P2PConnection {
    private maxRetries = 5
    private retryDelay = 1000 // ms, com backoff exponencial

    async reconnect() {
        for (let i = 0; i < this.maxRetries; i++) {
            try {
                await this.connect()
                return // Sucesso
            } catch (err) {
                const delay = this.retryDelay * Math.pow(2, i)
                await sleep(delay)
            }
        }
        // Fallback: pedir novo código ao Host
        this.emit('reconnect:failed')
    }
}
```

---

## 6. Waiting Room (UX)

### 6.1 Host View

```
┌──────────────────────────────────────┐
│  🔔 Novo pedido de entrada           │
│                                      │
│  👤 fulano@gmail.com                 │
│     Fulano da Silva                  │
│                                      │
│  [ ✅ Aprovar ]  [ ❌ Rejeitar ]      │
└──────────────────────────────────────┘
```

### 6.2 Guest View (Aguardando)

```
┌──────────────────────────────────────┐
│  ⏳ Aguardando aprovação do Host...   │
│                                      │
│  Sessão: X92-B4                      │
│  Host: perucci                       │
│                                      │
│  [ Cancelar ]                        │
└──────────────────────────────────────┘
```

---

## 7. Escalabilidade

| Nº Guests | Topologia                              |
| ---------- | -------------------------------------- |
| 1-4        | Full Mesh (cada um conecta ao Host)    |
| 5-10       | Star (Host no centro, broadcast)       |
| 10+        | Considerar SFU (Selective Forwarding)  |

---

## 8. Métricas

| Operação                       | Meta          |
| ------------------------------- | ------------- |
| Geração de código               | < 10ms        |
| Handshake completo (LAN)        | < 500ms       |
| Handshake completo (WAN)        | < 3s          |
| Reconexão automática            | < 5s          |
| Latência P2P (local network)    | < 20ms        |
| Latência P2P (internet)         | < 150ms       |

---

## 9. Dependências

| Dependência                | Tipo       | Spec Relacionada       |
| --------------------------- | ---------- | ---------------------- |
| WebSocket (signaling)       | Bloqueador | —                      |
| auth_and_persistence        | Bloqueador | auth_and_persistence   |
| terminal_sharing             | Bloqueador | terminal_sharing       |
| STUN/TURN servers            | Bloqueador | —                      |
