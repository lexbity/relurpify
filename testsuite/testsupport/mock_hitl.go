package testsupport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"codeburg.org/lexbit/relurpify/governance/authorization"
)

// MockHITLBroker is a configurable test double for the euclo policy.HITLBroker
// interface. It records every request and returns a configured response. With
// no response configured it fails closed (returns an error), matching the
// production posture; tests that need approval set Default or Responses.
type MockHITLBroker struct {
	mu        sync.Mutex
	requests  []authorization.PermissionRequest
	Responses map[string]*authorization.PermissionGrant
	Default   *authorization.PermissionGrant
}

// NewMockHITLBroker returns a fail-closed mock: requests are recorded and an
// error is returned until a response is configured.
func NewMockHITLBroker() *MockHITLBroker {
	return &MockHITLBroker{Responses: map[string]*authorization.PermissionGrant{}}
}

// NewAutoApprovingBroker returns a mock that approves every request. It is
// intended for tests that exercise non-HITL behavior but still run the policy
// gate. The production HITLBroker contains no auto-approve switch (D13); this
// explicit fake is its test-side replacement.
func NewAutoApprovingBroker() *MockHITLBroker {
	now := time.Now().UTC()
	return &MockHITLBroker{
		Responses: map[string]*authorization.PermissionGrant{},
		Default: &authorization.PermissionGrant{
			ID:         "mock-auto-approve",
			ApprovedBy: "testsupport",
			Conditions: map[string]string{},
			GrantedAt:  now,
			ExpiresAt:  now.Add(5 * time.Minute),
		},
	}
}

// RequestPermission records the request and returns the configured response.
func (m *MockHITLBroker) RequestPermission(_ context.Context, req authorization.PermissionRequest) (*authorization.PermissionGrant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	if grant, ok := m.Responses[req.Permission.Action]; ok {
		return grant, nil
	}
	if m.Default != nil {
		return m.Default, nil
	}
	return nil, fmt.Errorf("mock HITL broker: no response configured for %q", req.Permission.Action)
}

// Requests returns a copy of every permission request received.
func (m *MockHITLBroker) Requests() []authorization.PermissionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]authorization.PermissionRequest(nil), m.requests...)
}
