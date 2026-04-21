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
	TransportProfileHTTPTLS           = "http.tls.v1"
	TransportProfileHTTPLoopback      = "http.loopback.v1"

	HeaderFMPTransportProfile = "X-FMP-Transport-Profile"
	HeaderFMPSessionNonce     = "X-FMP-Session-Nonce"
	HeaderFMPSessionIssuedAt  = "X-FMP-Session-Issued-At"
	HeaderFMPSessionExpiresAt = "X-FMP-Session-Expires-At"
	HeaderFMPPeerKeyID        = "X-FMP-Peer-Key-ID"
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

type FederationTransportFrame struct {
	TrustDomain      string
	TransportProfile string
	SessionNonce     string
	SessionIssuedAt  time.Time
	SessionExpiresAt time.Time
	PeerKeyID        string
}

func DefaultFMPTransportPolicy(allowLoopbackInsecure bool) *FMPTransportPolicy {
	profiles := []string{TransportProfileWebSocketTLS, TransportProfileHTTPTLS}
	if allowLoopbackInsecure {
		profiles = append(profiles, TransportProfileWebSocketLoopback, TransportProfileHTTPLoopback)
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
	return p.validateTransportFrame(ctx, transportValidationFrame{
		Scope:            frame.NodeID + ":" + frame.RuntimeID,
		TrustDomain:      frame.TrustDomain,
		RuntimeID:        frame.RuntimeID,
		TransportProfile: frame.TransportProfile,
		SessionNonce:     frame.SessionNonce,
		SessionIssuedAt:  frame.SessionIssuedAt,
		SessionExpiresAt: frame.SessionExpiresAt,
		PeerKeyID:        frame.PeerKeyID,
		RequireTrust:     true,
		RequireRuntime:   true,
		Subject:          "node transport",
	}, tlsActive)
}

func (p *FMPTransportPolicy) ValidateFederationForward(ctx context.Context, frame FederationTransportFrame, tlsActive bool) error {
	if p == nil {
		return nil
	}
	return p.validateTransportFrame(ctx, transportValidationFrame{
		Scope:            strings.TrimSpace(frame.TrustDomain) + ":" + strings.TrimSpace(frame.PeerKeyID),
		TrustDomain:      frame.TrustDomain,
		TransportProfile: frame.TransportProfile,
		SessionNonce:     frame.SessionNonce,
		SessionIssuedAt:  frame.SessionIssuedAt,
		SessionExpiresAt: frame.SessionExpiresAt,
		PeerKeyID:        frame.PeerKeyID,
		RequireTrust:     true,
		Subject:          "federation transport",
	}, tlsActive)
}

type transportValidationFrame struct {
	Scope            string
	TrustDomain      string
	RuntimeID        string
	TransportProfile string
	SessionNonce     string
	SessionIssuedAt  time.Time
	SessionExpiresAt time.Time
	PeerKeyID        string
	RequireTrust     bool
	RequireRuntime   bool
	Subject          string
}

func (p *FMPTransportPolicy) validateTransportFrame(ctx context.Context, frame transportValidationFrame, tlsActive bool) error {
	now := time.Now().UTC()
	if p.Now != nil {
		now = p.Now().UTC()
	}
	subject := strings.TrimSpace(frame.Subject)
	if subject == "" {
		subject = "transport"
	}
	if frame.RequireTrust && strings.TrimSpace(frame.TrustDomain) == "" {
		return fmt.Errorf("trust_domain required for %s", subject)
	}
	if frame.RequireRuntime && strings.TrimSpace(frame.RuntimeID) == "" {
		return fmt.Errorf("runtime_id required for %s", subject)
	}
	if strings.TrimSpace(frame.TransportProfile) == "" {
		return fmt.Errorf("transport_profile required for %s", subject)
	}
	if !containsString(p.AllowedProfiles, frame.TransportProfile) {
		return fmt.Errorf("transport profile %q not allowed", frame.TransportProfile)
	}
	if strings.TrimSpace(frame.SessionNonce) == "" {
		return fmt.Errorf("session_nonce required for %s", subject)
	}
	if strings.TrimSpace(frame.PeerKeyID) == "" {
		return fmt.Errorf("peer_key_id required for %s", subject)
	}
	if frame.SessionIssuedAt.IsZero() || frame.SessionExpiresAt.IsZero() {
		return fmt.Errorf("session issuance and expiry required for %s", subject)
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
	case TransportProfileHTTPTLS:
		if p.RequireTLS && !tlsActive {
			return fmt.Errorf("tls required for federation transport profile %q", frame.TransportProfile)
		}
	case TransportProfileHTTPLoopback:
		if !p.AllowLoopbackInsecure {
			return fmt.Errorf("insecure loopback transport profile not allowed")
		}
	default:
		if p.RequireTLS && !tlsActive {
			return fmt.Errorf("tls required for node transport")
		}
	}
	if p.NonceStore != nil {
		if err := p.NonceStore.Reserve(ctx, strings.TrimSpace(frame.Scope), frame.SessionNonce, frame.SessionExpiresAt); err != nil {
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
