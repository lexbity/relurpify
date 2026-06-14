package registry

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/capability/agentspec"
	"codeburg.org/lexbit/relurpify/capability/descriptor"
	"codeburg.org/lexbit/relurpify/governance/permissions"
	"codeburg.org/lexbit/relurpify/governance/policy"
)

type mockApprovalManager struct {
	mu        sync.Mutex
	requests  []permissions.PermissionDescriptor
	approvals int
}

func (m *mockApprovalManager) RequireApproval(_ context.Context, _ string, desc permissions.PermissionDescriptor, _ string, _ policy.GrantScope, _ policy.RiskLevel, _ time.Duration) error {
	m.mu.Lock()
	m.requests = append(m.requests, desc)
	if desc.RequiresHITL {
		m.approvals++
	}
	m.mu.Unlock()
	return nil
}

func (m *mockApprovalManager) AuthorizeTool(_ context.Context, _ string, _ any, _ map[string]any) error {
	return nil
}

func (m *mockApprovalManager) approvalCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.approvals
}

func newTestState(manager PermissionManagerHandle, toolPolicy map[string]agentspec.ToolPolicy) executionRuntimeState {
	spec := &agentspec.AgentRuntimeSpec{
		ToolExecutionPolicy: toolPolicy,
	}
	return executionRuntimeState{
		agentID: "test-agent",
		manager: manager,
		policy:  compileRuntimePolicy(spec, toolPolicy, nil, nil, nil),
	}
}

const testToolDenyName = "_test_tool"

func TestAskPath_TriggersApproval(t *testing.T) {
	manager := &mockApprovalManager{}
	state := newTestState(manager, map[string]agentspec.ToolPolicy{
		testToolDenyName: {Execute: agentspec.AgentPermissionAsk},
	})
	desc := descriptor.CapabilityDescriptor{
		ID:            testToolDenyName,
		Name:          testToolDenyName,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool,
	}

	if err := enforceDescriptorExecutionPolicies(context.Background(), desc, state, nil); err != nil {
		t.Fatalf("ask should pass after approval: %v", err)
	}
	if manager.approvalCount() != 1 {
		t.Fatalf("expected 1 approval request for ask, got %d", manager.approvalCount())
	}
}

func TestDenyPath_BlocksExecution(t *testing.T) {
	manager := &mockApprovalManager{}
	state := newTestState(manager, map[string]agentspec.ToolPolicy{
		testToolDenyName: {Execute: agentspec.AgentPermissionDeny},
	})
	desc := descriptor.CapabilityDescriptor{
		ID:            testToolDenyName,
		Name:          testToolDenyName,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool,
	}

	err := enforceDescriptorExecutionPolicies(context.Background(), desc, state, nil)
	if err == nil {
		t.Fatal("deny should return an error")
	}
	if manager.approvalCount() != 0 {
		t.Fatalf("expected 0 approvals for deny, got %d", manager.approvalCount())
	}
	if !strings.Contains(err.Error(), "tool policy") {
		t.Fatalf("deny error %q should contain rejecting layer 'tool policy'", err.Error())
	}
}

func TestDenyPath_NormalizedError(t *testing.T) {
	// Verify the error from deny goes through normalizeToolExecutionPolicyError
	// and produces a user-facing error mentioning the tool name.
	innerErr := enforceDescriptorExecutionPolicies(context.Background(),
		descriptor.CapabilityDescriptor{
			ID:            testToolDenyName,
			Name:          testToolDenyName,
			Kind:          agentspec.CapabilityKindTool,
			RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool,
		},
		newTestState(&mockApprovalManager{}, map[string]agentspec.ToolPolicy{
			testToolDenyName: {Execute: agentspec.AgentPermissionDeny},
		}),
		nil,
	)
	normalized := normalizeToolExecutionPolicyError(testToolDenyName, innerErr)
	if normalized == nil {
		t.Fatal("normalized error should not be nil")
	}
	msg := normalized.Error()
	if !strings.Contains(msg, testToolDenyName) {
		t.Fatalf("normalized error %q should mention tool name %q", msg, testToolDenyName)
	}
	if !strings.Contains(msg, "blocked") {
		t.Fatalf("normalized error %q should mention 'blocked'", msg)
	}
}

func TestAllowPath_PassesWithoutApproval(t *testing.T) {
	manager := &mockApprovalManager{}
	state := newTestState(manager, map[string]agentspec.ToolPolicy{
		testToolDenyName: {Execute: agentspec.AgentPermissionAllow},
	})
	desc := descriptor.CapabilityDescriptor{
		ID:            testToolDenyName,
		Name:          testToolDenyName,
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool,
	}

	if err := enforceDescriptorExecutionPolicies(context.Background(), desc, state, nil); err != nil {
		t.Fatalf("allow should pass: %v", err)
	}
	if manager.approvalCount() != 0 {
		t.Fatalf("expected 0 approvals for allow, got %d", manager.approvalCount())
	}
}

func TestUnsetPolicy_DefaultBehavior(t *testing.T) {
	manager := &mockApprovalManager{}
	state := newTestState(manager, map[string]agentspec.ToolPolicy{})
	desc := descriptor.CapabilityDescriptor{
		ID:            "_test_tool",
		Name:          "_test_tool",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool,
	}

	if err := enforceDescriptorExecutionPolicies(context.Background(), desc, state, nil); err != nil {
		t.Fatalf("unset should pass: %v", err)
	}
	if manager.approvalCount() != 0 {
		t.Fatalf("expected 0 approvals for unset, got %d", manager.approvalCount())
	}
}
