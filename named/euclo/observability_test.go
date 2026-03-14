package euclo

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/stretchr/testify/require"
)

type eucloTelemetryRecorder struct {
	events []core.Event
}

func (r *eucloTelemetryRecorder) Emit(event core.Event) {
	r.events = append(r.events, event)
}

func TestBuildActionLogAndProofSurfaceFromState(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.mode_resolution", ModeResolution{ModeID: "code"})
	state.Set("euclo.execution_profile_selection", ExecutionProfileSelection{ProfileID: "edit_verify_repair"})
	state.Set("euclo.capability_family_routing", CapabilityFamilyRouting{PrimaryFamilyID: "implementation"})
	state.Set("euclo.verification", VerificationEvidence{Status: "pass"})
	state.Set("euclo.success_gate", SuccessGateResult{Allowed: true, Reason: "verification_accepted"})
	artifacts := []Artifact{
		{Kind: ArtifactKindWorkflowRetrieval},
		{Kind: ArtifactKindVerification},
	}

	log := BuildActionLog(state, artifacts)
	require.NotEmpty(t, log)

	proof := BuildProofSurface(state, artifacts)
	require.Equal(t, "code", proof.ModeID)
	require.Equal(t, "edit_verify_repair", proof.ProfileID)
	require.Equal(t, "implementation", proof.PrimaryFamilyID)
	require.Equal(t, "pass", proof.VerificationStatus)
	require.True(t, proof.WorkflowRetrievalUsed)
}

func TestAgentExecutePublishesObservabilitySurfacesAndTelemetry(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	recorder := &eucloTelemetryRecorder{}
	agent := New(agentenv.AgentEnvironment{
		Model:    eucloStubModel{},
		Registry: capability.NewRegistry(),
		Memory:   memStore.WithVectorStore(memory.NewInMemoryVectorStore()),
		Config:   &core.Config{Name: "euclo-observe", Model: "stub", MaxIterations: 1, Telemetry: recorder},
	})

	state := core.NewContext()
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "verification passed",
		"checks":  []any{map[string]any{"name": "go test ./...", "status": "pass"}},
	})
	result, err := agent.Execute(context.Background(), &core.Task{
		ID:          "task-observe",
		Instruction: "summarize current status",
		Context:     map[string]any{"workspace": "/tmp/ws"},
	}, state)
	require.NoError(t, err)
	require.NotNil(t, result)

	raw, ok := state.Get("euclo.action_log")
	require.True(t, ok)
	log, ok := raw.([]ActionLogEntry)
	require.True(t, ok)
	require.NotEmpty(t, log)

	raw, ok = state.Get("euclo.proof_surface")
	require.True(t, ok)
	proof, ok := raw.(ProofSurface)
	require.True(t, ok)
	require.Equal(t, "plan_stage_execute", proof.ProfileID)

	require.NotEmpty(t, recorder.events)
	foundStateChange := false
	foundAgentFinish := false
	for _, event := range recorder.events {
		if event.Type == core.EventStateChange {
			foundStateChange = true
		}
		if event.Type == core.EventAgentFinish && event.Message == "euclo proof surface" {
			foundAgentFinish = true
		}
	}
	require.True(t, foundStateChange)
	require.True(t, foundAgentFinish)
}
