package session

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/middleware/mcp/protocol"
)

type State string

const (
	StateConnecting   State = "connecting"
	StateInitializing State = "initializing"
	StateInitialized  State = "initialized"
	StateDegraded     State = "degraded"
	StateClosing      State = "closing"
	StateClosed       State = "closed"
	StateFailed       State = "failed"
)

type Config struct {
	ProviderID        string
	SessionID         string
	TransportKind     string
	RemoteTarget      string
	LocalPeer         protocol.PeerInfo
	RequestedVersion  string
	LocalCapabilities map[string]any
	Recoverable       bool
}

type Snapshot struct {
	ProviderID          string
	SessionID           string
	TransportKind       string
	RemoteTarget        string
	State               State
	LocalPeer           protocol.PeerInfo
	RemotePeer          protocol.PeerInfo
	RequestedVersion    string
	NegotiatedVersion   string
	LocalCapabilities   map[string]any
	RemoteCapabilities  map[string]any
	ActiveRequests      int
	ActiveSubscriptions []string
	Recoverable         bool
	FailureReason       string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Session struct {
	mu       sync.RWMutex
	snapshot Snapshot
}

func NewClientSession(cfg Config) (*Session, error) {
	if strings.TrimSpace(cfg.ProviderID) == "" {
		return nil, fmt.Errorf("provider id required")
	}
	if strings.TrimSpace(cfg.SessionID) == "" {
		return nil, fmt.Errorf("session id required")
	}
	if strings.TrimSpace(cfg.TransportKind) == "" {
		return nil, fmt.Errorf("transport kind required")
	}
	if strings.TrimSpace(cfg.RequestedVersion) == "" {
		return nil, fmt.Errorf("requested version required")
	}
	now := time.Now().UTC()
	return &Session{
		snapshot: Snapshot{
			ProviderID:        strings.TrimSpace(cfg.ProviderID),
			SessionID:         strings.TrimSpace(cfg.SessionID),
			TransportKind:     strings.TrimSpace(cfg.TransportKind),
			RemoteTarget:      strings.TrimSpace(cfg.RemoteTarget),
			State:             StateConnecting,
			LocalPeer:         cfg.LocalPeer,
			RequestedVersion:  protocol.NormalizeRevision(cfg.RequestedVersion),
			LocalCapabilities: cloneAnyMap(cfg.LocalCapabilities),
			Recoverable:       cfg.Recoverable,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	}, nil
}

func (s *Session) MarkTransportEstablished() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot.State != StateConnecting {
		return fmt.Errorf("session %s transport establishment invalid from state %s", s.snapshot.SessionID, s.snapshot.State)
	}
	s.snapshot.State = StateInitializing
	s.snapshot.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *Session) ApplyInitializeResult(result protocol.InitializeResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshot.State != StateInitializing && s.snapshot.State != StateDegraded {
		return fmt.Errorf("session %s initialize result invalid from state %s", s.snapshot.SessionID, s.snapshot.State)
	}
	revision := protocol.NormalizeRevision(result.ProtocolVersion)
	if revision == "" {
		return fmt.Errorf("negotiated protocol version required")
	}
	s.snapshot.State = StateInitialized
	s.snapshot.NegotiatedVersion = revision
	s.snapshot.RemotePeer = result.ServerInfo
	s.snapshot.RemoteCapabilities = cloneAnyMap(result.Capabilities)
	s.snapshot.FailureReason = ""
	s.snapshot.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *Session) UpdateRequestCount(delta int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.snapshot.ActiveRequests + delta
	if next < 0 {
		return fmt.Errorf("session %s active request count cannot be negative", s.snapshot.SessionID)
	}
	s.snapshot.ActiveRequests = next
	s.snapshot.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *Session) SetSubscription(uri string, subscribed bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return fmt.Errorf("subscription uri required")
	}
	set := make(map[string]struct{}, len(s.snapshot.ActiveSubscriptions))
	for _, existing := range s.snapshot.ActiveSubscriptions {
		if strings.TrimSpace(existing) == "" {
			continue
		}
		set[existing] = struct{}{}
	}
	if subscribed {
		set[uri] = struct{}{}
	} else {
		delete(set, uri)
	}
	s.snapshot.ActiveSubscriptions = make([]string, 0, len(set))
	for value := range set {
		s.snapshot.ActiveSubscriptions = append(s.snapshot.ActiveSubscriptions, value)
	}
	s.snapshot.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *Session) Degrade(reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.snapshot.State {
	case StateInitializing, StateInitialized:
	default:
		return fmt.Errorf("session %s degrade invalid from state %s", s.snapshot.SessionID, s.snapshot.State)
	}
	s.snapshot.State = StateDegraded
	s.snapshot.FailureReason = strings.TrimSpace(reason)
	s.snapshot.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *Session) Fail(reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.snapshot.State {
	case StateClosing, StateClosed:
		return fmt.Errorf("session %s fail invalid from state %s", s.snapshot.SessionID, s.snapshot.State)
	}
	s.snapshot.State = StateFailed
	s.snapshot.FailureReason = strings.TrimSpace(reason)
	s.snapshot.UpdatedAt = time.Now().UTC()
	return nil
}

func (s *Session) BeginClose() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.snapshot.State {
	case StateClosed:
		return fmt.Errorf("session %s already closed", s.snapshot.SessionID)
	case StateClosing:
		return nil
	default:
		s.snapshot.State = StateClosing
		s.snapshot.UpdatedAt = time.Now().UTC()
		return nil
	}
}

func (s *Session) MarkClosed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.State = StateClosed
	s.snapshot.ActiveRequests = 0
	s.snapshot.UpdatedAt = time.Now().UTC()
}

func (s *Session) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.snapshot
	out.LocalCapabilities = cloneAnyMap(out.LocalCapabilities)
	out.RemoteCapabilities = cloneAnyMap(out.RemoteCapabilities)
	out.ActiveSubscriptions = append([]string(nil), out.ActiveSubscriptions...)
	return out
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
