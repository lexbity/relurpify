package authorization

import (
	"context"
	"fmt"
	"testing"

	"codeburg.org/lexbit/relurpify/capability/ports"
	"codeburg.org/lexbit/relurpify/governance/classification"
	policy "codeburg.org/lexbit/relurpify/governance/policy"
	governanceports "codeburg.org/lexbit/relurpify/governance/ports"
	"codeburg.org/lexbit/relurpify/governance/risk"
)

// mockDelegationRegistry implements DelegationCapabilityRegistry for testing.
type mockDelegationRegistry struct {
	invokeCalled bool
	invokeState  governanceports.InvocationState
	invokeResult any
	invokeErr    error
}

func (r *mockDelegationRegistry) GetCoordinationTarget(idOrName string) (governanceports.DescriptorView, bool) {
	return &mockDescriptor{id: idOrName, name: idOrName}, true
}

func (r *mockDelegationRegistry) CoordinationTargets(selectors ...governanceports.CapabilitySelectorView) []governanceports.DescriptorView {
	return []governanceports.DescriptorView{&mockDescriptor{id: "target-1", name: "target-1"}}
}

func (r *mockDelegationRegistry) InvokeCapability(ctx context.Context, state governanceports.InvocationState, idOrName string, args map[string]any) (any, error) {
	r.invokeCalled = true
	r.invokeState = state
	return r.invokeResult, r.invokeErr
}

func (r *mockDelegationRegistry) CapturePolicySnapshot() *policy.PolicySnapshot {
	return &policy.PolicySnapshot{ID: "snapshot-1"}
}

func (r *mockDelegationRegistry) EffectiveCoordination(spec governanceports.SpecView) governanceports.CoordinationSpecView {
	return governanceports.CoordinationSpecView{
		MaxDelegationDepth:        3,
		AllowRemoteDelegation:     false,
		AllowBackgroundDelegation: false,
	}
}

func (r *mockDelegationRegistry) BuildDelegationResult(request policy.DelegationRequest, target governanceports.DescriptorView, result any, invokeErr error, snapshot *policy.PolicySnapshot, spec governanceports.SpecView, callerTrust string) *policy.DelegationResult {
	state := policy.DelegationStateSucceeded
	if invokeErr != nil {
		state = policy.DelegationStateFailed
	}
	return policy.NewDelegationResult(request, target.CapabilityID(), target.SourceProviderID(), target.SourceSessionID(), state, invokeErr == nil, nil)
}

type mockDescriptor struct {
	id   string
	name string
}

func (d *mockDescriptor) CapabilityID() string                              { return d.id }
func (d *mockDescriptor) CapabilityName() string                            { return d.name }
func (d *mockDescriptor) CapabilityKind() string                            { return "tool" }
func (d *mockDescriptor) RuntimeFamily() string                             { return "local-tool" }
func (d *mockDescriptor) Description() string                               { return "mock" }
func (d *mockDescriptor) Version() string                                   { return "1.0" }
func (d *mockDescriptor) Category() string                                  { return "" }
func (d *mockDescriptor) Tags() []string                                    { return nil }
func (d *mockDescriptor) TrustClass() string                                { return "builtin-trusted" }
func (d *mockDescriptor) RiskClasses() []risk.RiskClass                      { return nil }
func (d *mockDescriptor) EffectClasses() []classification.EffectClass        { return nil }
func (d *mockDescriptor) SourceProviderID() string                          { return "provider-1" }
func (d *mockDescriptor) SourceScope() string                               { return "builtin" }
func (d *mockDescriptor) SourceSessionID() string                           { return "" }
func (d *mockDescriptor) CoordinationRole() string                          { return "" }
func (d *mockDescriptor) CoordinationTaskTypes() []string                    { return nil }
func (d *mockDescriptor) CoordinationExecutionModes() []string               { return nil }
func (d *mockDescriptor) CoordinationLongRunning() int32                     { return 0 }
func (d *mockDescriptor) CoordinationDirectInsertionAllowed() int32          { return 0 }
func (d *mockDescriptor) CapabilityExecutionModes() []string                  { return nil }
func (d *mockDescriptor) CoordinationTarget() bool                           { return true }
func (d *mockDescriptor) CoordinationMaxDepth() int                          { return 3 }
func (d *mockDescriptor) CoordinationMaxRuntimeSeconds() int                  { return 0 }
func (d *mockDescriptor) GetToolExecutionPolicy() map[string]governanceports.ToolPolicyView { return nil }
func (d *mockDescriptor) GetCapabilityPolicies() []governanceports.CapabilityPolicyView { return nil }
func (d *mockDescriptor) GetProviderPolicies() map[string]governanceports.ProviderPolicyView { return nil }
func (d *mockDescriptor) GetSessionPolicies() []governanceports.SessionPolicyView { return nil }
func (d *mockDescriptor) GetGlobalPolicies() map[string]string              { return nil }
func (d *mockDescriptor) GetAllowedCapabilities() []governanceports.CapabilitySelectorView { return nil }
func (d *mockDescriptor) GetBrowser() governanceports.BrowserSpecView         { return governanceports.BrowserSpecView{} }
func (d *mockDescriptor) GetOrchestration() governanceports.OrchestrationConfigView { return governanceports.OrchestrationConfigView{} }
func (d *mockDescriptor) ProviderSecurityOrigin() string                    { return "" }

// mockInvocationState implements ports.State via the InvocationState pass-through.
type mockInvocationState struct {
	taskID    string
	sessionID string
}

func (s *mockInvocationState) GetWorkingValue(key string) (any, bool) { return nil, false }
func (s *mockInvocationState) SetWorkingValue(key string, value any)  {}
func (s *mockInvocationState) DeleteWorkingValue(key string)          {}
func (s *mockInvocationState) ClearWorkingData()                     {}
func (s *mockInvocationState) WorkingMemoryKeys() []string            { return nil }
func (s *mockInvocationState) Snapshot() map[string]any               { return nil }
func (s *mockInvocationState) TaskID() string                         { return s.taskID }
func (s *mockInvocationState) SessionID() string                      { return s.sessionID }

// TestACDELEG_DelegatedInvoke verifies that a delegated capability invocation
// flows through the DelegationCapabilityRegistry with the correct state type.
func TestACDELEG_DelegatedInvoke(t *testing.T) {
	mgr := NewDelegationManager()
	reg := &mockDelegationRegistry{
		invokeResult: map[string]any{"result": "ok"},
	}

	state := &mockInvocationState{taskID: "task-1", sessionID: "session-1"}
	var stateAsInvocation governanceports.InvocationState = state

	snapshot, err := mgr.ExecuteDelegation(context.Background(), policy.DelegationRequest{
		ID:                 "deleg-1",
		TaskType:           "test",
		Instruction:        "do something",
		TargetCapabilityID: "target-1",
	}, DelegationExecutionOptions{
		Registry:  reg,
		AgentSpec: nil,
		State:     stateAsInvocation,
	})
	if err != nil {
		t.Fatalf("ExecuteDelegation: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snapshot.State != policy.DelegationStateSucceeded {
		t.Errorf("expected state Succeeded, got %s", snapshot.State)
	}
	if !reg.invokeCalled {
		t.Fatal("InvokeCapability was not called")
	}
	// Verify the state passed through the InvocationState interface
	ps, ok := reg.invokeState.(ports.State)
	if !ok {
		t.Fatal("invokeState does not implement ports.State")
	}
	if ps.TaskID() != "task-1" {
		t.Errorf("expected TaskID task-1, got %s", ps.TaskID())
	}
	if ps.SessionID() != "session-1" {
		t.Errorf("expected SessionID session-1, got %s", ps.SessionID())
	}
}

// TestACDELEG_NilState verifies that a nil InvocationState is handled.
func TestACDELEG_NilState(t *testing.T) {
	mgr := NewDelegationManager()
	reg := &mockDelegationRegistry{
		invokeResult: map[string]any{"result": "ok"},
	}

	if _, err := mgr.ExecuteDelegation(context.Background(), policy.DelegationRequest{
		ID:                 "deleg-nil",
		TaskType:           "test",
		Instruction:        "do something",
		TargetCapabilityID: "target-1",
	}, DelegationExecutionOptions{
		Registry:  reg,
		AgentSpec: nil,
		State:     nil,
	}); err != nil {
		t.Fatalf("ExecuteDelegation: %v", err)
	}
	if !reg.invokeCalled {
		t.Fatal("InvokeCapability was not called")
	}
	if reg.invokeState != nil {
		t.Errorf("expected nil state passed to registry, got %v", reg.invokeState)
	}
}

// TestACDELEG_FailedInvoke verifies that a failed capability invocation
// produces the correct delegation state transition.
func TestACDELEG_FailedInvoke(t *testing.T) {
	mgr := NewDelegationManager()
	reg := &mockDelegationRegistry{
		invokeErr: fmt.Errorf("capability error"),
	}

	// ExecuteDelegation returns both the completed snapshot and the invokeError.
	// We check that the snapshot contains the Failed state.
	completed, err := mgr.ExecuteDelegation(context.Background(), policy.DelegationRequest{
		ID:                 "deleg-fail",
		TaskType:           "test",
		Instruction:        "do something",
		TargetCapabilityID: "target-1",
	}, DelegationExecutionOptions{
		Registry:  reg,
		AgentSpec: nil,
	})
	if err == nil {
		t.Fatal("expected error for failed capability invocation")
	}
	if completed != nil && completed.State != policy.DelegationStateFailed {
		t.Errorf("expected state Failed, got %s", completed.State)
	}
}

// TestACDELEG_StateTransitions verifies the delegation lifecycle transitions.
func TestACDELEG_StateTransitions(t *testing.T) {
	mgr := NewDelegationManager()
	reg := &mockDelegationRegistry{
		invokeResult: map[string]any{"result": "ok"},
	}

	started, err := mgr.ExecuteDelegation(context.Background(), policy.DelegationRequest{
		ID:                 "deleg-trans",
		TaskType:           "test",
		Instruction:        "do something",
		TargetCapabilityID: "target-1",
	}, DelegationExecutionOptions{
		Registry:  reg,
		AgentSpec: nil,
	})
	if err != nil {
		t.Fatalf("ExecuteDelegation: %v", err)
	}
	if started.State != policy.DelegationStateSucceeded {
		t.Errorf("expected state Succeeded, got %s", started.State)
	}
	if started.StartedAt.IsZero() {
		t.Error("expected non-zero StartedAt")
	}
	if started.UpdatedAt.IsZero() {
		t.Error("expected non-zero UpdatedAt")
	}

	fetched, err := mgr.GetDelegation(started.Request.ID)
	if err != nil {
		t.Fatalf("GetDelegation: %v", err)
	}
	if fetched.Request.ID != started.Request.ID {
		t.Errorf("expected ID %s, got %s", started.Request.ID, fetched.Request.ID)
	}
}
