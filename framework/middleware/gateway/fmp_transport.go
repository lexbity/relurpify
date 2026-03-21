package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	TransportProfileWebSocketTLS      = "websocket.tls.v1"
	TransportProfileWebSocketLoopback = "websocket.loopback.v1"
)

type FMPTransportPolicy struct {
	RequireNodeTransport  bool
	RequireTLS            bool
	AllowLoopbackInsecure bool
	AllowedProfiles       []string
	SessionTTL            time.Duration
	MaxClockSkew          time.Duration
	NonceStore            TransportNonceStore
	Now                   func() time.Time
}

type TransportNonceStore interface {
	Reserve(ctx context.Context, scope, nonce string, expiresAt time.Time) error
}

type InMemoryTransportNonceStore struct {
	mu      sync.Mutex
	entries map[string]time.Time
	Now     func() time.Time
}

func DefaultFMPTransportPolicy(allowLoopbackInsecure bool) *FMPTransportPolicy {
	profiles := []string{TransportProfileWebSocketTLS}
	if allowLoopbackInsecure {
		profiles = append(profiles, TransportProfileWebSocketLoopback)
	}
	return &FMPTransportPolicy{
		RequireNodeTransport:  true,
		RequireTLS:            !allowLoopbackInsecure,
		AllowLoopbackInsecure: allowLoopbackInsecure,
		AllowedProfiles:       profiles,
		SessionTTL:            30 * time.Minute,
		MaxClockSkew:          30 * time.Second,
		NonceStore:            &InMemoryTransportNonceStore{},
		Now:                   func() time.Time { return time.Now().UTC() },
	}
}

func (s *InMemoryTransportNonceStore) Reserve(_ context.Context, scope, nonce string, expiresAt time.Time) error {
	if strings.TrimSpace(scope) == "" {
		return fmt.Errorf("transport nonce scope required")
	}
	if strings.TrimSpace(nonce) == "" {
		return fmt.Errorf("transport nonce required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entries == nil {
		s.entries = map[string]time.Time{}
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	for key, expiry := range s.entries {
		if !expiry.IsZero() && now.After(expiry) {
			delete(s.entries, key)
		}
	}
	key := scope + ":" + nonce
	if expiry, ok := s.entries[key]; ok && (expiry.IsZero() || now.Before(expiry)) {
		return fmt.Errorf("transport nonce replay detected")
	}
	s.entries[key] = expiresAt.UTC()
	return nil
}

func (p *FMPTransportPolicy) validateNodeConnect(ctx context.Context, frame connectFrame, tlsActive bool) error {
	if frame.Role != "node" || p == nil || !p.RequireNodeTransport {
		return nil
	}
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now().UTC()
	}
	if strings.TrimSpace(frame.TrustDomain) == "" {
		return fmt.Errorf("trust_domain required for node transport")
	}
	if strings.TrimSpace(frame.RuntimeID) == "" {
		return fmt.Errorf("runtime_id required for node transport")
	}
	if strings.TrimSpace(frame.TransportProfile) == "" {
		return fmt.Errorf("transport_profile required for node transport")
	}
	if !containsString(p.AllowedProfiles, frame.TransportProfile) {
		return fmt.Errorf("transport profile %q not allowed", frame.TransportProfile)
	}
	if strings.TrimSpace(frame.SessionNonce) == "" {
		return fmt.Errorf("session_nonce required for node transport")
	}
	if strings.TrimSpace(frame.PeerKeyID) == "" {
		return fmt.Errorf("peer_key_id required for node transport")
	}
	if frame.SessionIssuedAt.IsZero() || frame.SessionExpiresAt.IsZero() {
		return fmt.Errorf("session issuance and expiry required for node transport")
	}
	if !frame.SessionExpiresAt.After(frame.SessionIssuedAt) {
		return fmt.Errorf("session expiry must be after issued_at")
	}
	if p.SessionTTL > 0 && frame.SessionExpiresAt.Sub(frame.SessionIssuedAt) > p.SessionTTL {
		return fmt.Errorf("session ttl exceeds maximum transport policy")
	}
	if p.MaxClockSkew > 0 && frame.SessionIssuedAt.After(now.Add(p.MaxClockSkew)) {
		return fmt.Errorf("session issued_at is too far in the future")
	}
	if p.MaxClockSkew > 0 && frame.SessionExpiresAt.Before(now.Add(-p.MaxClockSkew)) {
		return fmt.Errorf("session has expired")
	}
	switch frame.TransportProfile {
	case TransportProfileWebSocketTLS:
		if p.RequireTLS && !tlsActive {
			return fmt.Errorf("tls required for node transport profile %q", frame.TransportProfile)
		}
	case TransportProfileWebSocketLoopback:
		if !p.AllowLoopbackInsecure {
			return fmt.Errorf("insecure loopback transport profile not allowed")
		}
	default:
		if p.RequireTLS && !tlsActive {
			return fmt.Errorf("tls required for node transport")
		}
	}
	if p.NonceStore != nil {
		if err := p.NonceStore.Reserve(ctx, frame.NodeID+":"+frame.RuntimeID, frame.SessionNonce, frame.SessionExpiresAt); err != nil {
			return err
		}
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
