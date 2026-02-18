package terminal

import (
	"context"
	"encoding/base64"
	"log"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type OutputObserver func(sessionID string, data []byte)

// Bridge conecta o PTYManager ao frontend via Wails Events
type Bridge struct {
	ctx     context.Context
	manager *PTYManager
	seq     atomic.Uint64

	observersMu sync.RWMutex
	observers   []OutputObserver
}

// NewBridge cria um novo Terminal Bridge
func NewBridge(ctx context.Context, manager *PTYManager) *Bridge {
	return &Bridge{
		ctx:     ctx,
		manager: manager,
	}
}

// CreateTerminal cria um novo terminal e configura o streaming de I/O
func (b *Bridge) CreateTerminal(config PTYConfig) (string, error) {
	sessionID, err := b.manager.Create(config)
	if err != nil {
		return "", err
	}

	// Registrar handler de output que emite eventos para o frontend
	b.manager.OnOutput(sessionID, func(data []byte) {
		seq := b.seq.Add(1)
		msg := OutputMessage{
			SessionID: sessionID,
			Data:      base64.StdEncoding.EncodeToString(data),
			Timestamp: 0,
			Sequence:  seq,
		}
		runtime.EventsEmit(b.ctx, "terminal:output", msg)
		b.notifyOutputObservers(sessionID, data)
	})

	// Emitir evento de terminal criado
	runtime.EventsEmit(b.ctx, "terminal:created", map[string]interface{}{
		"sessionID": sessionID,
		"config":    config,
	})

	log.Printf("[Bridge] Terminal created and streaming: %s", sessionID)
	return sessionID, nil
}

// WriteTerminal envia dados para o stdin de um terminal
func (b *Bridge) WriteTerminal(sessionID string, data string) error {
	// Decodificar base64 do frontend
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		// Se não for base64, usar como texto raw
		decoded = []byte(data)
	}
	return b.manager.Write(sessionID, decoded)
}

// ResizeTerminal altera o tamanho de um terminal
func (b *Bridge) ResizeTerminal(sessionID string, cols, rows uint16) error {
	return b.manager.Resize(sessionID, cols, rows)
}

// DestroyTerminal encerra um terminal
func (b *Bridge) DestroyTerminal(sessionID string) error {
	err := b.manager.Destroy(sessionID)
	if err != nil {
		return err
	}

	runtime.EventsEmit(b.ctx, "terminal:destroyed", map[string]string{
		"sessionID": sessionID,
	})

	return nil
}

// GetTerminals retorna informações de todos os terminais ativos
func (b *Bridge) GetTerminals() []SessionInfo {
	return b.manager.GetSessions()
}

// IsTerminalAlive verifica se um terminal está ativo
func (b *Bridge) IsTerminalAlive(sessionID string) bool {
	return b.manager.IsAlive(sessionID)
}

// RegisterOutputObserver adiciona um observador para output bruto do PTY.
func (b *Bridge) RegisterOutputObserver(observer OutputObserver) {
	if observer == nil {
		return
	}
	b.observersMu.Lock()
	defer b.observersMu.Unlock()
	b.observers = append(b.observers, observer)
}

func (b *Bridge) notifyOutputObservers(sessionID string, data []byte) {
	b.observersMu.RLock()
	defer b.observersMu.RUnlock()

	for _, obs := range b.observers {
		obs(sessionID, data)
	}
}
