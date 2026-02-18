package gitactivity

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxEvents    = 200
	defaultDedupeWindow = 750 * time.Millisecond
	maxListLimit        = 500
)

// Service mantém um buffer em memória com deduplicação de eventos.
type Service struct {
	mu           sync.RWMutex
	events       []Event
	lastSeen     map[string]time.Time
	maxEvents    int
	dedupeWindow time.Duration
	seq          uint64
}

// NewService cria o serviço com defaults seguros para UI.
func NewService(maxEvents int, dedupeWindow time.Duration) *Service {
	if maxEvents <= 0 {
		maxEvents = defaultMaxEvents
	}
	if dedupeWindow <= 0 {
		dedupeWindow = defaultDedupeWindow
	}

	return &Service{
		events:       make([]Event, 0, maxEvents),
		lastSeen:     make(map[string]time.Time),
		maxEvents:    maxEvents,
		dedupeWindow: dedupeWindow,
	}
}

// AppendEvent adiciona um evento ao buffer; retorna false se foi deduplicado.
func (s *Service) AppendEvent(event Event) (Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := event.Timestamp
	if now.IsZero() {
		now = time.Now()
	}
	event.Timestamp = now

	if event.Type == "" {
		event.Type = EventTypeUnknown
	}
	if event.RepoPath != "" {
		event.RepoPath = filepath.Clean(event.RepoPath)
	}
	if event.RepoName == "" && event.RepoPath != "" {
		event.RepoName = filepath.Base(event.RepoPath)
	}

	dedupeKey := strings.TrimSpace(event.DedupeKey)
	if dedupeKey == "" {
		dedupeKey = buildDedupeKey(event)
	}
	event.DedupeKey = dedupeKey

	if dedupeKey != "" {
		if last, ok := s.lastSeen[dedupeKey]; ok {
			if now.Sub(last) <= s.dedupeWindow {
				return Event{}, false
			}
		}
		s.lastSeen[dedupeKey] = now
	}

	s.seq++
	event.ID = fmt.Sprintf("gae_%d_%d", now.UnixMilli(), s.seq)
	s.events = append(s.events, event)
	if len(s.events) > s.maxEvents {
		overflow := len(s.events) - s.maxEvents
		s.events = append([]Event(nil), s.events[overflow:]...)
	}

	s.pruneLastSeen(now)

	return cloneEvent(event), true
}

// ListEvents retorna eventos em ordem reversa cronológica (mais recente primeiro).
func (s *Service) ListEvents(opts ListOptions) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	repoFilter := ""
	if opts.RepoPath != "" {
		repoFilter = filepath.Clean(opts.RepoPath)
	}

	result := make([]Event, 0, limit)
	for i := len(s.events) - 1; i >= 0; i-- {
		event := s.events[i]
		if opts.Type != "" && event.Type != opts.Type {
			continue
		}
		if repoFilter != "" && filepath.Clean(event.RepoPath) != repoFilter {
			continue
		}

		result = append(result, cloneEvent(event))
		if len(result) >= limit {
			break
		}
	}

	return result
}

// GetEvent retorna um evento pelo ID.
func (s *Service) GetEvent(eventID string) (*Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := len(s.events) - 1; i >= 0; i-- {
		if s.events[i].ID == eventID {
			event := cloneEvent(s.events[i])
			return &event, true
		}
	}
	return nil, false
}

// Clear remove todos os eventos do buffer.
func (s *Service) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = s.events[:0]
	s.lastSeen = make(map[string]time.Time)
}

// Count retorna quantidade de eventos armazenados.
func (s *Service) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}

func (s *Service) pruneLastSeen(now time.Time) {
	if len(s.lastSeen) == 0 {
		return
	}

	expiration := s.dedupeWindow * 6
	if expiration < 2*time.Second {
		expiration = 2 * time.Second
	}
	for key, ts := range s.lastSeen {
		if now.Sub(ts) > expiration {
			delete(s.lastSeen, key)
		}
	}
}

func buildDedupeKey(event Event) string {
	typePart := string(event.Type)
	branchPart := strings.TrimSpace(event.Branch)
	repoPart := strings.TrimSpace(event.RepoPath)
	msgPart := strings.TrimSpace(event.Message)
	refPart := strings.TrimSpace(event.Details.Ref)
	return strings.Join([]string{typePart, repoPart, branchPart, refPart, msgPart}, "|")
}

func cloneEvent(event Event) Event {
	cloned := event
	if event.Details.Extra != nil {
		cloned.Details.Extra = make(map[string]string, len(event.Details.Extra))
		for k, v := range event.Details.Extra {
			cloned.Details.Extra[k] = v
		}
	}
	if len(event.Details.Files) > 0 {
		cloned.Details.Files = append([]EventFile(nil), event.Details.Files...)
	}
	return cloned
}
