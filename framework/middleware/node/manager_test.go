package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

type memoryNodeStore struct {
	nodes map[string]core.NodeDescriptor
	creds map[string]core.NodeCredential
	pairs map[string]PendingPairing
}

func (m *memoryNodeStore) GetNode(_ context.Context, id string) (*core.NodeDescriptor, error) {
	if node, ok := m.nodes[id]; ok {
		copy := node
		return &copy, nil
	}
	return nil, nil
}
func (m *memoryNodeStore) ListNodes(_ context.Context) ([]core.NodeDescriptor, error) {
	out := make([]core.NodeDescriptor, 0, len(m.nodes))
	for _, node := range m.nodes {
		out = append(out, node)
	}
	return out, nil
}
func (m *memoryNodeStore) UpsertNode(_ context.Context, node core.NodeDescriptor) error {
	if m.nodes == nil {
		m.nodes = map[string]core.NodeDescriptor{}
	}
	m.nodes[node.ID] = node
	return nil
}
func (m *memoryNodeStore) RemoveNode(_ context.Context, id string) error {
	delete(m.nodes, id)
	return nil
}
func (m *memoryNodeStore) GetCredential(_ context.Context, deviceID string) (*core.NodeCredential, error) {
	if cred, ok := m.creds[deviceID]; ok {
		copy := cred
		return &copy, nil
	}
	return nil, nil
}
func (m *memoryNodeStore) SaveCredential(_ context.Context, cred core.NodeCredential) error {
	if m.creds == nil {
		m.creds = map[string]core.NodeCredential{}
	}
	m.creds[cred.DeviceID] = cred
	return nil
}
func (m *memoryNodeStore) SavePendingPairing(_ context.Context, pairing PendingPairing) error {
	if m.pairs == nil {
		m.pairs = map[string]PendingPairing{}
	}
	m.pairs[pairing.Code] = pairing
	return nil
}
func (m *memoryNodeStore) GetPendingPairing(_ context.Context, code string) (*PendingPairing, error) {
	if pairing, ok := m.pairs[code]; ok {
		copy := pairing
		return &copy, nil
	}
	return nil, nil
}
func (m *memoryNodeStore) ListPendingPairings(_ context.Context) ([]PendingPairing, error) {
	out := make([]PendingPairing, 0, len(m.pairs))
	for _, pairing := range m.pairs {
		out = append(out, pairing)
	}
	return out, nil
}
func (m *memoryNodeStore) DeletePendingPairing(_ context.Context, code string) error {
	delete(m.pairs, code)
	return nil
}
func (m *memoryNodeStore) DeleteExpiredPendingPairings(_ context.Context, before time.Time) (int, error) {
	deleted := 0
	for code, pairing := range m.pairs {
		if !pairing.ExpiresAt.After(before) {
			delete(m.pairs, code)
			deleted++
		}
	}
	return deleted, nil
}

type testConnection struct{ node core.NodeDescriptor }

func (t testConnection) Node() core.NodeDescriptor                 { return t.node }
func (t testConnection) Health() core.NodeHealth                   { return core.NodeHealth{Online: true} }
func (t testConnection) Capabilities() []core.CapabilityDescriptor { return nil }
func (t testConnection) Invoke(context.Context, string, map[string]any) (*core.CapabilityExecutionResult, error) {
	return &core.CapabilityExecutionResult{Success: true}, nil
}
func (t testConnection) Close(context.Context) error { return nil }

func TestManagerConnectDisconnectAndPairing(t *testing.T) {
	store := &memoryNodeStore{}
	manager := &Manager{Store: store}
	node := core.NodeDescriptor{
		ID:         "node-1",
		Name:       "Laptop",
		Platform:   core.NodePlatformLinux,
		TrustClass: core.TrustClassWorkspaceTrusted,
	}

	require.NoError(t, manager.HandleConnect(context.Background(), testConnection{node: node}))
	_, ok := manager.GetConnection("node-1")
	require.True(t, ok)

	cred, _, err := GenerateCredential("node-1")
	require.NoError(t, err)
	code, err := manager.RequestPairing(context.Background(), cred)
	require.NoError(t, err)
	require.NoError(t, manager.ApprovePairing(context.Background(), code))

	stored, err := store.GetCredential(context.Background(), "node-1")
	require.NoError(t, err)
	require.NotNil(t, stored)

	require.NoError(t, manager.HandleDisconnect(context.Background(), "node-1", "bye"))
	_, ok = manager.GetConnection("node-1")
	require.False(t, ok)
}

func TestManagerPairingStatusSweepsExpiredStoreRows(t *testing.T) {
	store := &memoryNodeStore{
		pairs: map[string]PendingPairing{
			"expired": {
				Code: "expired",
				Cred: core.NodeCredential{
					DeviceID: "node-1",
					IssuedAt: time.Now().UTC().Add(-2 * time.Minute),
				},
				ExpiresAt: time.Now().UTC().Add(-time.Minute),
			},
		},
	}
	manager := &Manager{Store: store}

	pairing, ok, err := manager.PairingStatus(context.Background(), "expired")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, pairing)
	require.Empty(t, store.pairs)
}

func TestManagerRequestPairingReturnsEntropyError(t *testing.T) {
	originalRandomRead := randomRead
	randomRead = func([]byte) (int, error) {
		return 0, errors.New("entropy exhausted")
	}
	defer func() {
		randomRead = originalRandomRead
	}()

	store := &memoryNodeStore{}
	manager := &Manager{Store: store}

	cred, _, err := GenerateCredential("node-1")
	require.NoError(t, err)

	code, err := manager.RequestPairing(context.Background(), cred)
	require.Error(t, err)
	require.ErrorContains(t, err, "generate pairing code")
	require.ErrorContains(t, err, "entropy exhausted")
	require.Empty(t, code)
	require.Empty(t, store.pairs)
}

func TestManagerListCapabilitiesForTenantNormalizesProviderMetadata(t *testing.T) {
	store := &memoryNodeStore{}
	manager := &Manager{Store: store}
	node := core.NodeDescriptor{
		ID:         "node-1",
		TenantID:   "tenant-1",
		Name:       "Laptop",
		Platform:   core.NodePlatformLinux,
		TrustClass: core.TrustClassRemoteApproved,
	}
	require.NoError(t, manager.HandleConnect(context.Background(), capabilityConnection{
		node: node,
		caps: []core.CapabilityDescriptor{{
			ID:   "camera.capture",
			Name: "camera.capture",
			Kind: core.CapabilityKindTool,
		}},
	}))

	caps := manager.ListCapabilitiesForTenant("tenant-1")
	require.Len(t, caps, 1)
	require.Equal(t, "node:node-1", caps[0].Source.ProviderID)
	require.Equal(t, core.CapabilityScopeProvider, caps[0].Source.Scope)
	require.Equal(t, core.CapabilityRuntimeFamilyProvider, caps[0].RuntimeFamily)
	require.Equal(t, core.TrustClassRemoteApproved, caps[0].TrustClass)
}

type capabilityConnection struct {
	node core.NodeDescriptor
	caps []core.CapabilityDescriptor
}

func (c capabilityConnection) Node() core.NodeDescriptor { return c.node }
func (c capabilityConnection) Health() core.NodeHealth   { return core.NodeHealth{Online: true} }
func (c capabilityConnection) Capabilities() []core.CapabilityDescriptor {
	return append([]core.CapabilityDescriptor(nil), c.caps...)
}
func (c capabilityConnection) Invoke(context.Context, string, map[string]any) (*core.CapabilityExecutionResult, error) {
	return &core.CapabilityExecutionResult{Success: true}, nil
}
func (c capabilityConnection) Close(context.Context) error { return nil }
