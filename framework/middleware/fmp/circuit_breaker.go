package fmp

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"   // normal operation
	CircuitOpen     CircuitState = "open"      // tripped, rejecting traffic
	CircuitHalfOpen CircuitState = "half_open" // probing for recovery
)

// CircuitBreakerConfig configures a per-trust-domain circuit breaker.
type CircuitBreakerConfig struct {
	TrustDomain      string        `json:"trust_domain" yaml:"trust_domain"`
	ErrorThreshold   float64       `json:"error_threshold" yaml:"error_threshold"` // 0.0-1.0 fraction
	MinRequests      int           `json:"min_requests" yaml:"min_requests"`       // minimum samples before tripping
	WindowDuration   time.Duration `json:"window_duration" yaml:"window_duration"`
	RecoveryDuration time.Duration `json:"recovery_duration" yaml:"recovery_duration"`
}

// CircuitBreakerStatus represents the current status of a circuit breaker.
type CircuitBreakerStatus struct {
	TrustDomain string       `json:"trust_domain"`
	State       CircuitState `json:"state"`
	ErrorRate   float64      `json:"error_rate"`
	Requests    int          `json:"requests"`
	TrippedAt   *time.Time   `json:"tripped_at,omitempty"`
	RecoveryAt  *time.Time   `json:"recovery_at,omitempty"`
}

// CircuitBreakerStore manages circuit breaker state per trust domain.
type CircuitBreakerStore interface {
	GetState(ctx context.Context, trustDomain string) (CircuitState, error)
	RecordSuccess(ctx context.Context, trustDomain string, now time.Time) error
	RecordFailure(ctx context.Context, trustDomain string, now time.Time) error
	Trip(ctx context.Context, trustDomain string, now time.Time) error
	Reset(ctx context.Context, trustDomain string, now time.Time) error
	ListStates(ctx context.Context) ([]CircuitBreakerStatus, error)
	SetConfig(ctx context.Context, cfg CircuitBreakerConfig) error
}

// InMemoryCircuitBreakerStore provides in-memory circuit breaker state management.
type InMemoryCircuitBreakerStore struct {
	mu       sync.RWMutex
	states   map[string]*circuitEntry
	configs  map[string]CircuitBreakerConfig
	windows  map[string]time.Time // trust domain -> window start time
}

type circuitEntry struct {
	state       CircuitState
	requests    int
	failures    int
	trippedAt   *time.Time
	recoveryAt  *time.Time
}

// GetState returns the current circuit state, automatically transitioning half-open to closed on success probes.
func (s *InMemoryCircuitBreakerStore) GetState(ctx context.Context, trustDomain string) (CircuitState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.states[trustDomain]
	if !ok {
		return CircuitClosed, nil // default state for unknown domain
	}

	// Auto-transition from open to half-open when recovery window expires
	if entry.state == CircuitOpen && entry.recoveryAt != nil && time.Now().UTC().After(*entry.recoveryAt) {
		// Caller should handle half-open state by sending probe requests
		return CircuitHalfOpen, nil
	}

	return entry.state, nil
}

// RecordSuccess records a successful operation, transitioning half-open to closed.
func (s *InMemoryCircuitBreakerStore) RecordSuccess(ctx context.Context, trustDomain string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.getOrCreateEntry(trustDomain)

	// Half-open probe succeeded: reset to closed
	if entry.state == CircuitHalfOpen {
		entry.state = CircuitClosed
		entry.requests = 0
		entry.failures = 0
		entry.trippedAt = nil
		entry.recoveryAt = nil
		return nil
	}

	// Closed state: track request
	entry.requests++
	return nil
}

// RecordFailure records a failed operation, potentially tripping the breaker.
func (s *InMemoryCircuitBreakerStore) RecordFailure(ctx context.Context, trustDomain string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.getOrCreateEntry(trustDomain)
	cfg := s.configs[trustDomain]

	entry.failures++
	entry.requests++

	// Check if threshold exceeded
	if entry.requests >= cfg.MinRequests {
		errorRate := float64(entry.failures) / float64(entry.requests)
		if errorRate >= cfg.ErrorThreshold {
			// Trip the breaker
			entry.state = CircuitOpen
			now := time.Now().UTC()
			entry.trippedAt = &now
			recoveryTime := now.Add(cfg.RecoveryDuration)
			entry.recoveryAt = &recoveryTime
		}
	}

	return nil
}

// Trip manually opens the circuit breaker.
func (s *InMemoryCircuitBreakerStore) Trip(ctx context.Context, trustDomain string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.getOrCreateEntry(trustDomain)
	entry.state = CircuitOpen
	entry.trippedAt = &now
	cfg := s.configs[trustDomain]
	recoveryTime := now.Add(cfg.RecoveryDuration)
	entry.recoveryAt = &recoveryTime

	return nil
}

// Reset closes the circuit breaker and clears failure counts.
func (s *InMemoryCircuitBreakerStore) Reset(ctx context.Context, trustDomain string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.getOrCreateEntry(trustDomain)
	entry.state = CircuitClosed
	entry.requests = 0
	entry.failures = 0
	entry.trippedAt = nil
	entry.recoveryAt = nil

	return nil
}

// ListStates returns the status of all circuit breakers.
func (s *InMemoryCircuitBreakerStore) ListStates(ctx context.Context) ([]CircuitBreakerStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []CircuitBreakerStatus
	for domain, entry := range s.states {
		errorRate := 0.0
		if entry.requests > 0 {
			errorRate = float64(entry.failures) / float64(entry.requests)
		}
		out = append(out, CircuitBreakerStatus{
			TrustDomain: domain,
			State:       entry.state,
			ErrorRate:   errorRate,
			Requests:    entry.requests,
			TrippedAt:   entry.trippedAt,
			RecoveryAt:  entry.recoveryAt,
		})
	}
	return out, nil
}

// SetConfig sets or updates the configuration for a trust domain.
func (s *InMemoryCircuitBreakerStore) SetConfig(ctx context.Context, cfg CircuitBreakerConfig) error {
	if cfg.TrustDomain == "" {
		return fmt.Errorf("trust domain required")
	}
	if cfg.ErrorThreshold < 0 || cfg.ErrorThreshold > 1 {
		return fmt.Errorf("error threshold must be 0.0-1.0")
	}
	if cfg.MinRequests < 1 {
		cfg.MinRequests = 10 // default
	}
	if cfg.WindowDuration <= 0 {
		cfg.WindowDuration = 1 * time.Minute // default
	}
	if cfg.RecoveryDuration <= 0 {
		cfg.RecoveryDuration = 30 * time.Second // default
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.configs == nil {
		s.configs = map[string]CircuitBreakerConfig{}
	}
	s.configs[cfg.TrustDomain] = cfg

	return nil
}

func (s *InMemoryCircuitBreakerStore) getOrCreateEntry(trustDomain string) *circuitEntry {
	if s.states == nil {
		s.states = map[string]*circuitEntry{}
	}
	entry, ok := s.states[trustDomain]
	if !ok {
		entry = &circuitEntry{state: CircuitClosed}
		s.states[trustDomain] = entry
		if s.configs == nil {
			s.configs = map[string]CircuitBreakerConfig{}
		}
		// Set default config if not already present
		if _, ok := s.configs[trustDomain]; !ok {
			s.configs[trustDomain] = CircuitBreakerConfig{
				TrustDomain:      trustDomain,
				ErrorThreshold:   0.5,
				MinRequests:      10,
				WindowDuration:   1 * time.Minute,
				RecoveryDuration: 30 * time.Second,
			}
		}
	}
	return entry
}
