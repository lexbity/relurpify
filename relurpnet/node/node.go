package node

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
)

// NodeStore persists paired nodes and credentials.
type NodeStore interface {
	GetNode(ctx context.Context, id string) (*core.NodeDescriptor, error)
	ListNodes(ctx context.Context) ([]core.NodeDescriptor, error)
	UpsertNode(ctx context.Context, node core.NodeDescriptor) error
	RemoveNode(ctx context.Context, id string) error
	GetCredential(ctx context.Context, deviceID string) (*core.NodeCredential, error)
	SaveCredential(ctx context.Context, cred core.NodeCredential) error
	SavePendingPairing(ctx context.Context, pairing PendingPairing) error
	GetPendingPairing(ctx context.Context, code string) (*PendingPairing, error)
	ListPendingPairings(ctx context.Context) ([]PendingPairing, error)
	DeletePendingPairing(ctx context.Context, code string) error
	DeleteExpiredPendingPairings(ctx context.Context, before time.Time) (int, error)
}

// Connection is the framework view of an active node connection.
type Connection interface {
	Node() core.NodeDescriptor
	Health() core.NodeHealth
	Capabilities() []core.CapabilityDescriptor
	Invoke(ctx context.Context, capabilityID string, args map[string]any) (*core.CapabilityExecutionResult, error)
	Close(ctx context.Context) error
}

// maxPairingFailures is the number of failed pairing-code lookups allowed within
// pairingFailureWindow before the manager stops accepting approve/reject requests.
const maxPairingFailures = 10

// pairingFailureWindow is the rolling window for counting failed pairing attempts.
const pairingFailureWindow = 5 * time.Minute

type PairingConfig struct {
	AutoApproveLocal bool
	PairingCodeTTL   time.Duration
	RequireSignature bool
}

type pairingRequest struct {
	cred      core.NodeCredential
	expiresAt time.Time
}

// pairingFailureBucket tracks consecutive failed pairing-code lookups within a
// rolling time window to defend against brute-force enumeration of pairing codes.
type pairingFailureBucket struct {
	count    int
	windowAt time.Time
}

type PendingPairing struct {
	Code      string
	Cred      core.NodeCredential
	ExpiresAt time.Time
}

// Manager manages connected nodes and pairing state.
type Manager struct {
	Store   NodeStore
	Log     event.Log
	Pairing PairingConfig

	mu              sync.RWMutex
	connections     map[string]Connection
	pending         map[string]pairingRequest
	pairingFailures pairingFailureBucket
}

func (m *Manager) HandleConnect(ctx context.Context, conn Connection) error {
	if conn == nil {
		return errors.New("connection required")
	}
	node := conn.Node()
	if err := node.Validate(); err != nil {
		return err
	}
	if m.Store != nil {
		if err := m.Store.UpsertNode(ctx, node); err != nil {
			return err
		}
	}
	m.mu.Lock()
	if m.connections == nil {
		m.connections = map[string]Connection{}
	}
	m.connections[node.ID] = conn
	m.mu.Unlock()
	m.emit(ctx, node.ID, core.FrameworkEventNodeConnected, map[string]any{
		"node_id":   node.ID,
		"name":      node.Name,
		"platform":  node.Platform,
		"connected": true,
	})
	return nil
}

func (m *Manager) HandleDisconnect(ctx context.Context, nodeID string, reason string) error {
	m.mu.Lock()
	delete(m.connections, nodeID)
	m.mu.Unlock()
	m.emit(ctx, nodeID, core.FrameworkEventNodeDisconnected, map[string]any{
		"node_id": nodeID,
		"reason":  reason,
	})
	return nil
}

func (m *Manager) RequestPairing(ctx context.Context, cred core.NodeCredential) (string, error) {
	if err := cred.Validate(); err != nil {
		return "", err
	}
	if err := m.sweepExpiredPendingPairings(ctx); err != nil {
		return "", err
	}
	code, err := generatePairingCode()
	if err != nil {
		return "", err
	}
	ttl := m.Pairing.PairingCodeTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	m.mu.Lock()
	if m.pending == nil {
		m.pending = map[string]pairingRequest{}
	}
	expiresAt := time.Now().UTC().Add(ttl)
	m.pending[code] = pairingRequest{cred: cred, expiresAt: expiresAt}
	m.mu.Unlock()
	if m.Store != nil {
		if err := m.Store.SavePendingPairing(ctx, PendingPairing{
			Code:      code,
			Cred:      cred,
			ExpiresAt: expiresAt,
		}); err != nil {
			return "", err
		}
	}
	m.emit(ctx, cred.DeviceID, core.FrameworkEventNodePairingRequested, map[string]any{
		"device_id": cred.DeviceID,
		"code":      code,
	})
	return code, nil
}

// checkPairingRateLimit records a failed lookup and returns an error if the
// failure rate exceeds maxPairingFailures within pairingFailureWindow. Must be
// called while holding m.mu.
func (m *Manager) recordPairingFailureLocked() error {
	now := time.Now().UTC()
	if now.Sub(m.pairingFailures.windowAt) >= pairingFailureWindow {
		m.pairingFailures = pairingFailureBucket{count: 1, windowAt: now}
		return nil
	}
	m.pairingFailures.count++
	if m.pairingFailures.count > maxPairingFailures {
		return fmt.Errorf("pairing rate limit exceeded: too many failed attempts, try again in %s", pairingFailureWindow)
	}
	return nil
}

// checkPairingRateLimit returns an error when the failure window is saturated,
// without incrementing the counter (used for pre-check before lookup).
func (m *Manager) checkPairingRateLimitLocked() error {
	now := time.Now().UTC()
	if now.Sub(m.pairingFailures.windowAt) >= pairingFailureWindow {
		return nil
	}
	if m.pairingFailures.count > maxPairingFailures {
		return fmt.Errorf("pairing rate limit exceeded: too many failed attempts, try again in %s", pairingFailureWindow)
	}
	return nil
}

func (m *Manager) ApprovePairing(ctx context.Context, pairingCode string) error {
	m.mu.Lock()
	if err := m.checkPairingRateLimitLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()
	req, ok := m.pendingRequest(ctx, pairingCode)
	if !ok {
		m.mu.Lock()
		_ = m.recordPairingFailureLocked()
		m.mu.Unlock()
		return errors.New("pairing request not found")
	}
	if m.Store != nil {
		if err := m.Store.SaveCredential(ctx, req.cred); err != nil {
			return err
		}
		if err := m.Store.DeletePendingPairing(ctx, pairingCode); err != nil {
			return err
		}
	}
	m.mu.Lock()
	delete(m.pending, pairingCode)
	m.mu.Unlock()
	m.emit(ctx, req.cred.DeviceID, core.FrameworkEventNodePairingApproved, map[string]any{
		"device_id": req.cred.DeviceID,
	})
	return nil
}

func (m *Manager) RejectPairing(ctx context.Context, pairingCode string) error {
	m.mu.Lock()
	if err := m.checkPairingRateLimitLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()
	req, ok := m.pendingRequest(ctx, pairingCode)
	if !ok {
		m.mu.Lock()
		_ = m.recordPairingFailureLocked()
		m.mu.Unlock()
		return errors.New("pairing request not found")
	}
	m.mu.Lock()
	delete(m.pending, pairingCode)
	m.mu.Unlock()
	if m.Store != nil {
		if err := m.Store.DeletePendingPairing(ctx, pairingCode); err != nil {
			return err
		}
	}
	m.emit(ctx, req.cred.DeviceID, core.FrameworkEventNodePairingRejected, map[string]any{
		"device_id": req.cred.DeviceID,
	})
	return nil
}

func (m *Manager) GetConnection(nodeID string) (Connection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.connections[nodeID]
	return conn, ok
}

func (m *Manager) ListConnections() []Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Connection, 0, len(m.connections))
	for _, conn := range m.connections {
		out = append(out, conn)
	}
	return out
}

// ListCapabilitiesForTenant returns all capabilities from connected nodes scoped
// to the given tenant. An empty tenantID matches nodes with no tenant set.
// Capabilities are normalized: source is set to "node:<nodeID>" and RuntimeFamily
// is set to CapabilityRuntimeFamilyProvider. TrustClass inherits from the node
// if not already set on the capability.
func (m *Manager) ListCapabilitiesForTenant(tenantID string) []core.CapabilityDescriptor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []core.CapabilityDescriptor
	for _, conn := range m.connections {
		node := conn.Node()
		if node.TenantID != "" && !strings.EqualFold(node.TenantID, tenantID) {
			continue
		}
		for _, desc := range conn.Capabilities() {
			normalized := desc
			normalized.Source.ProviderID = "node:" + node.ID
			normalized.Source.Scope = core.CapabilityScopeProvider
			normalized.RuntimeFamily = core.CapabilityRuntimeFamilyProvider
			if normalized.TrustClass == "" {
				normalized.TrustClass = node.TrustClass
			}
			out = append(out, normalized)
		}
	}
	return out
}

// InvokeCapabilityForTenant invokes a capability by ID or name on a connected node
// that belongs to the given tenant. Returns an error if no matching capability is found.
func (m *Manager) InvokeCapabilityForTenant(ctx context.Context, tenantID, capabilityID string, args map[string]any) (*core.CapabilityExecutionResult, error) {
	m.mu.RLock()
	var target Connection
	var targetCapID string
	for _, conn := range m.connections {
		node := conn.Node()
		if node.TenantID != "" && !strings.EqualFold(node.TenantID, tenantID) {
			continue
		}
		for _, desc := range conn.Capabilities() {
			if desc.ID == capabilityID || desc.Name == capabilityID {
				target = conn
				targetCapID = desc.ID
				break
			}
		}
		if target != nil {
			break
		}
	}
	m.mu.RUnlock()
	if target == nil {
		return nil, fmt.Errorf("capability %s not available for tenant %s", capabilityID, tenantID)
	}
	return target.Invoke(ctx, targetCapID, args)
}

func (m *Manager) PairingStatus(ctx context.Context, code string) (*PendingPairing, bool, error) {
	if err := m.sweepExpiredPendingPairings(ctx); err != nil {
		return nil, false, err
	}
	req, ok := m.pendingRequest(ctx, code)
	if !ok {
		return nil, false, nil
	}
	return &PendingPairing{
		Code:      code,
		Cred:      req.cred,
		ExpiresAt: req.expiresAt,
	}, true, nil
}

func (m *Manager) pendingRequest(ctx context.Context, code string) (pairingRequest, bool) {
	m.mu.RLock()
	req, ok := m.pending[code]
	m.mu.RUnlock()
	if !ok && m.Store != nil {
		_, _ = m.Store.DeleteExpiredPendingPairings(ctx, time.Now().UTC())
		stored, err := m.Store.GetPendingPairing(ctx, code)
		if err == nil && stored != nil {
			req = pairingRequest{cred: stored.Cred, expiresAt: stored.ExpiresAt}
			ok = true
		}
	}
	if !ok {
		return pairingRequest{}, false
	}
	if time.Now().UTC().After(req.expiresAt) {
		m.mu.Lock()
		delete(m.pending, code)
		m.mu.Unlock()
		if m.Store != nil {
			_ = m.Store.DeletePendingPairing(ctx, code)
		}
		return pairingRequest{}, false
	}
	return req, true
}

func (m *Manager) sweepExpiredPendingPairings(ctx context.Context) error {
	if m == nil || m.Store == nil {
		return nil
	}
	_, err := m.Store.DeleteExpiredPendingPairings(ctx, time.Now().UTC())
	return err
}

func (m *Manager) emit(ctx context.Context, actorID, eventType string, payload map[string]any) {
	if m == nil || m.Log == nil {
		return
	}
	data := mustJSON(payload)
	_, _ = m.Log.Append(ctx, "local", []core.FrameworkEvent{{
		Timestamp: time.Now().UTC(),
		Type:      eventType,
		Payload:   data,
		Actor:     core.EventActor{Kind: "node", ID: actorID},
		Partition: "local",
	}})
}

func generatePairingCode() (string, error) {
	b := make([]byte, 16)
	if _, err := randomRead(b); err != nil {
		return "", fmt.Errorf("generate pairing code: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func mustJSON(v any) []byte {
	data, _ := jsonMarshal(v)
	return data
}

var jsonMarshal = func(v any) ([]byte, error) {
	return json.Marshal(v)
}

var randomRead = rand.Read
