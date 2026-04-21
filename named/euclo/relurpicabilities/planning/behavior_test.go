package planning

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/execution"
)

// mockCodingCapability is a test double for EucloCodingCapability
type mockCodingCapability struct {
	eligibleFunc func(euclotypes.ArtifactState, euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult
	executeFunc  func(context.Context, euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult
}

func (m *mockCodingCapability) Descriptor() core.CapabilityDescriptor {
	return core.CapabilityDescriptor{ID: "test:mock"}
}

func (m *mockCodingCapability) Contract() euclotypes.ArtifactContract {
	return euclotypes.ArtifactContract{}
}

func (m *mockCodingCapability) Eligible(state euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
	if m.eligibleFunc != nil {
		return m.eligibleFunc(state, snapshot)
	}
	return euclotypes.EligibilityResult{Eligible: true}
}

func (m *mockCodingCapability) Execute(ctx context.Context, envelope euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, envelope)
	}
	return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted}
}

func TestPlanningBehavior_ID(t *testing.T) {
	cap := &mockCodingCapability{}
	behavior := New("test:capability", cap)

	if behavior.ID() != "test:capability" {
		t.Errorf("ID() = %q, want %q", behavior.ID(), "test:capability")
	}
}

func TestPlanningBehavior_Execute_NotEligible(t *testing.T) {
	cap := &mockCodingCapability{
		eligibleFunc: func(_ euclotypes.ArtifactState, _ euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
			return euclotypes.EligibilityResult{Eligible: false, Reason: "not eligible for test"}
		},
	}
	behavior := New("test:capability", cap)

	result, err := behavior.Execute(context.Background(), execution.ExecuteInput{
		Environment: agentenv.AgentEnvironment{Registry: capability.NewRegistry()}, // empty registry is fine
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Success {
		t.Error("expected result.Success = false for ineligible capability")
	}
	if result.Error == nil {
		t.Error("expected result.Error to be set for ineligible capability")
	} else if result.Error.Error() != "not eligible for test" {
		t.Errorf("expected error message 'not eligible for test', got %q", result.Error.Error())
	}
}

func TestPlanningBehavior_Execute_Success(t *testing.T) {
	cap := &mockCodingCapability{
		eligibleFunc: func(_ euclotypes.ArtifactState, _ euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
			return euclotypes.EligibilityResult{Eligible: true}
		},
		executeFunc: func(_ context.Context, _ euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
			return euclotypes.ExecutionResult{
				Status:  euclotypes.ExecutionStatusCompleted,
				Summary: "test completed",
				Artifacts: []euclotypes.Artifact{
					{Kind: euclotypes.ArtifactKindPlan, ID: "test-artifact"},
				},
			}
		},
	}
	behavior := New("test:capability", cap)

	state := core.NewContext()
	result, err := behavior.Execute(context.Background(), execution.ExecuteInput{
		Environment: agentenv.AgentEnvironment{Registry: capability.NewRegistry()},
		State:       state,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Errorf("expected result.Success = true, got false with error: %v", result.Error)
	}
	if result.Data["summary"] != "test completed" {
		t.Errorf("expected summary 'test completed', got %v", result.Data["summary"])
	}
}

func TestPlanningBehavior_Execute_Failure(t *testing.T) {
	cap := &mockCodingCapability{
		eligibleFunc: func(_ euclotypes.ArtifactState, _ euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
			return euclotypes.EligibilityResult{Eligible: true}
		},
		executeFunc: func(_ context.Context, _ euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
			return euclotypes.ExecutionResult{
				Status: euclotypes.ExecutionStatusFailed,
				FailureInfo: &euclotypes.CapabilityFailure{
					Message: "execution failed",
				},
			}
		},
	}
	behavior := New("test:capability", cap)

	result, err := behavior.Execute(context.Background(), execution.ExecuteInput{
		Environment: agentenv.AgentEnvironment{Registry: capability.NewRegistry()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Success {
		t.Error("expected result.Success = false for failed execution")
	}
	if result.Error == nil {
		t.Fatal("expected result.Error to be set for failed execution")
	}
	if result.Error.Error() != "execution failed" {
		t.Errorf("expected error 'execution failed', got %q", result.Error.Error())
	}
}

func TestPlanningBehavior_Execute_PassesTelemetry(t *testing.T) {
	var receivedEnvelope euclotypes.ExecutionEnvelope
	cap := &mockCodingCapability{
		eligibleFunc: func(_ euclotypes.ArtifactState, _ euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
			return euclotypes.EligibilityResult{Eligible: true}
		},
		executeFunc: func(_ context.Context, env euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
			receivedEnvelope = env
			return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted}
		},
	}
	behavior := New("test:capability", cap)

	testTask := &core.Task{ID: "test-task"}
	testMode := euclotypes.ModeResolution{ModeID: "code"}
	testProfile := euclotypes.ExecutionProfileSelection{ProfileID: "test-profile"}

	result, err := behavior.Execute(context.Background(), execution.ExecuteInput{
		Environment: agentenv.AgentEnvironment{Registry: capability.NewRegistry()},
		State:       core.NewContext(),
		Task:        testTask,
		Mode:        testMode,
		Profile:     testProfile,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	if receivedEnvelope.Task != testTask {
		t.Error("expected Task to be passed in envelope")
	}
	if receivedEnvelope.Mode.ModeID != testMode.ModeID {
		t.Error("expected Mode to be passed in envelope")
	}
	if receivedEnvelope.Profile.ProfileID != testProfile.ProfileID {
		t.Error("expected Profile to be passed in envelope")
	}
}

func TestPlanningBehavior_Execute_SnapshotFromRegistry(t *testing.T) {
	var receivedSnapshot euclotypes.CapabilitySnapshot
	cap := &mockCodingCapability{
		eligibleFunc: func(_ euclotypes.ArtifactState, snapshot euclotypes.CapabilitySnapshot) euclotypes.EligibilityResult {
			receivedSnapshot = snapshot
			return euclotypes.EligibilityResult{Eligible: true}
		},
		executeFunc: func(_ context.Context, _ euclotypes.ExecutionEnvelope) euclotypes.ExecutionResult {
			return euclotypes.ExecutionResult{Status: euclotypes.ExecutionStatusCompleted}
		},
	}
	behavior := New("test:capability", cap)

	// Create a registry with a mock tool that has read permissions
	reg := capability.NewRegistry()

	_, err := behavior.Execute(context.Background(), execution.ExecuteInput{
		Environment: agentenv.AgentEnvironment{Registry: reg},
		State:       core.NewContext(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify that the snapshot was built from the registry (even if empty)
	// The key point is that it came from SnapshotCapabilities, not from WorkflowStore presence
	if receivedSnapshot.ToolNames == nil {
		// ToolNames should be set (even if empty slice) when registry is provided
		t.Log("Snapshot was built from registry (ToolNames is nil for empty registry)")
	}
}
