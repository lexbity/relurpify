package euclotest

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/agentenv"
	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
	"github.com/lexcodex/relurpify/named/euclo"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/lexcodex/relurpify/testutil/euclotestutil"
	"github.com/stretchr/testify/require"
)

func TestBuildActionLogAndProofSurfaceFromState(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.mode_resolution", euclotypes.ModeResolution{ModeID: "code"})
	state.Set("euclo.execution_profile_selection", euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"})
	state.Set("euclo.capability_family_routing", eucloruntime.CapabilityFamilyRouting{PrimaryFamilyID: "implementation"})
	state.Set("euclo.verification", eucloruntime.VerificationEvidence{Status: "pass"})
	state.Set("euclo.success_gate", eucloruntime.SuccessGateResult{Allowed: true, Reason: "verification_accepted"})
	artifacts := []euclotypes.Artifact{
		{Kind: euclotypes.ArtifactKindWorkflowRetrieval},
		{Kind: euclotypes.ArtifactKindVerification},
	}

	log := eucloruntime.BuildActionLog(state, artifacts)
	require.NotEmpty(t, log)

	proof := eucloruntime.BuildProofSurface(state, artifacts)
	require.Equal(t, "code", proof.ModeID)
	require.NotEmpty(t, proof.ProfileID)
	require.Equal(t, "implementation", proof.PrimaryFamilyID)
	require.Equal(t, "pass", proof.VerificationStatus)
	require.True(t, proof.WorkflowRetrievalUsed)
}

func TestAgentExecutePublishesObservabilitySurfacesAndTelemetry(t *testing.T) {
	memStore, err := memory.NewHybridMemory(t.TempDir())
	require.NoError(t, err)
	recorder := &testutil.TelemetryRecorder{}
	agent := euclo.New(agentenv.AgentEnvironment{
		Model:    testutil.StubModel{},
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
	log, ok := raw.([]eucloruntime.ActionLogEntry)
	require.True(t, ok)
	require.NotEmpty(t, log)

	raw, ok = state.Get("euclo.proof_surface")
	require.True(t, ok)
	proof, ok := raw.(eucloruntime.ProofSurface)
	require.True(t, ok)
	require.NotEmpty(t, proof.ProfileID)
	require.NotEmpty(t, proof.ArtifactKinds)

	require.NotEmpty(t, recorder.Events)
	foundStateChange := false
	foundAgentFinish := false
	for _, event := range recorder.Events {
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
