package registry

import (
	"context"
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

func TestAskPath_TriggersApproval(t *testing.T) {
	manager := &mockApprovalManager{}
	state := newTestState(manager, map[string]agentspec.ToolPolicy{
		"_test_tool": {Execute: agentspec.AgentPermissionAsk},
	})
	desc := descriptor.CapabilityDescriptor{
		ID:            "_test_tool",
		Name:          "_test_tool",
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
		"_test_tool": {Execute: agentspec.AgentPermissionDeny},
	})
	desc := descriptor.CapabilityDescriptor{
		ID:            "_test_tool",
		Name:          "_test_tool",
		Kind:          agentspec.CapabilityKindTool,
		RuntimeFamily: agentspec.CapabilityRuntimeFamilyLocalTool,
	}

	if err := enforceDescriptorExecutionPolicies(context.Background(), desc, state, nil); err == nil {
		t.Fatal("deny should return an error")
	}
	if manager.approvalCount() != 0 {
		t.Fatalf("expected 0 approvals for deny, got %d", manager.approvalCount())
	}
}

func TestAllowPath_PassesWithoutApproval(t *testing.T) {
	manager := &mockApprovalManager{}
	state := newTestState(manager, map[string]agentspec.ToolPolicy{
		"_test_tool": {Execute: agentspec.AgentPermissionAllow},
	})
	desc := descriptor.CapabilityDescriptor{
		ID:            "_test_tool",
		Name:          "_test_tool",
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
