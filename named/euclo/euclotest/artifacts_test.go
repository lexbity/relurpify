package euclotest

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/stretchr/testify/require"
)

func TestCollectArtifactsFromStateNormalizesPipelineAndRetrievalState(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.envelope", eucloruntime.TaskEnvelope{TaskID: "task-1", Instruction: "fix bug", ResolvedMode: "code", ExecutionProfile: "edit_verify_repair"})
	state.Set("euclo.classification", eucloruntime.TaskClassification{RecommendedMode: "code", IntentFamilies: []string{"code"}})
	state.Set("euclo.mode_resolution", euclotypes.ModeResolution{ModeID: "code", Source: "explicit"})
	state.Set("euclo.execution_profile_selection", euclotypes.ExecutionProfileSelection{ProfileID: "edit_verify_repair"})
	state.Set("euclo.retrieval_policy", eucloruntime.RetrievalPolicy{ModeID: "code", LocalPathsFirst: true})
	state.Set("euclo.context_expansion", eucloruntime.ContextExpansion{LocalPaths: []string{"main.go"}, Summary: "local_paths=1"})
	state.Set("euclo.capability_family_routing", eucloruntime.CapabilityFamilyRouting{ModeID: "code", PrimaryFamilyID: "implementation"})
	state.Set("euclo.verification_policy", eucloruntime.VerificationPolicy{PolicyID: "code/edit_verify_repair", RequiresVerification: true})
	state.Set("euclo.success_gate", eucloruntime.SuccessGateResult{Allowed: true, Reason: "verification_accepted"})
	state.Set("euclo.action_log", []eucloruntime.ActionLogEntry{{Kind: "mode_resolution", Message: "resolved execution mode"}})
	state.Set("euclo.proof_surface", eucloruntime.ProofSurface{ModeID: "code", ProfileID: "edit_verify_repair"})
	state.Set("pipeline.workflow_retrieval", map[string]any{
		"query":          "fix bug",
		"scope":          "workflow:wf-1",
		"summary":        "prior workflow summary",
		"citation_count": 2,
	})
	state.Set("pipeline.plan", map[string]any{
		"strategy": "minimal patch",
		"steps":    []map[string]any{{"id": "s1"}},
	})
	state.Set("pipeline.code", map[string]any{
		"summary": "requested one edit",
		"edits":   []map[string]any{{"path": "main.go", "action": "update"}},
	})
	state.Set("euclo.edit_execution", eucloruntime.EditExecutionRecord{
		Requested: []eucloruntime.EditOperationRecord{{Path: "main.go", Action: "update", Status: "requested", Requested: true}},
		Executed:  []eucloruntime.EditOperationRecord{{Path: "main.go", Action: "update", Status: "executed", Requested: true}},
		Summary:   "requested=1 executed=1",
	})
	state.Set("pipeline.verify", map[string]any{
		"status":  "pass",
		"summary": "tests passed",
	})

	artifacts := euclotypes.CollectArtifactsFromState(state)
	require.Len(t, artifacts, 16)
	require.Equal(t, euclotypes.ArtifactKindIntake, artifacts[0].Kind)
	require.Equal(t, euclotypes.ArtifactKindClassification, artifacts[1].Kind)
	require.Equal(t, euclotypes.ArtifactKindModeResolution, artifacts[2].Kind)
	require.Equal(t, euclotypes.ArtifactKindExecutionProfile, artifacts[3].Kind)
	require.Equal(t, euclotypes.ArtifactKindRetrievalPolicy, artifacts[4].Kind)
	require.Equal(t, euclotypes.ArtifactKindContextExpansion, artifacts[5].Kind)
	require.Equal(t, euclotypes.ArtifactKindCapabilityRouting, artifacts[6].Kind)
	require.Equal(t, euclotypes.ArtifactKindVerificationPolicy, artifacts[7].Kind)
	require.Equal(t, euclotypes.ArtifactKindSuccessGate, artifacts[8].Kind)
	require.Equal(t, euclotypes.ArtifactKindActionLog, artifacts[9].Kind)
	require.Equal(t, euclotypes.ArtifactKindProofSurface, artifacts[10].Kind)
	require.Equal(t, euclotypes.ArtifactKindWorkflowRetrieval, artifacts[11].Kind)
	require.Equal(t, "prior workflow summary", artifacts[11].Summary)
	require.Equal(t, euclotypes.ArtifactKindPlan, artifacts[12].Kind)
	require.Equal(t, "minimal patch", artifacts[12].Summary)
	require.Equal(t, euclotypes.ArtifactKindEditIntent, artifacts[13].Kind)
	require.Equal(t, true, artifacts[13].Metadata["intent_only"])
	require.Equal(t, euclotypes.ArtifactKindEditExecution, artifacts[14].Kind)
	require.Equal(t, euclotypes.ArtifactKindVerification, artifacts[15].Kind)
}

func TestCollectArtifactsFromStateNormalizesLegacyRetrievalString(t *testing.T) {
	state := core.NewContext()
	state.Set("pipeline.workflow_retrieval", "legacy retrieval summary")

	artifacts := euclotypes.CollectArtifactsFromState(state)
	require.Len(t, artifacts, 1)
	require.Equal(t, euclotypes.ArtifactKindWorkflowRetrieval, artifacts[0].Kind)
	require.Equal(t, "legacy retrieval summary", artifacts[0].Summary)

	payload, ok := artifacts[0].Payload.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "legacy retrieval summary", payload["summary"])
}

func TestPersistWorkflowArtifactsWritesNormalizedRecords(t *testing.T) {
	writer := &workflowArtifactWriterStub{}
	artifacts := []euclotypes.Artifact{
		{
			ID:      "euclo_plan",
			Kind:    euclotypes.ArtifactKindPlan,
			Summary: "minimal patch",
			Metadata: map[string]any{
				"source_key": "pipeline.plan",
			},
			Payload: map[string]any{"strategy": "minimal patch"},
		},
		{
			Kind:    euclotypes.ArtifactKindVerification,
			Summary: "tests passed",
			Payload: map[string]any{"status": "pass"},
		},
	}

	err := euclotypes.PersistWorkflowArtifacts(context.Background(), writer, "wf-1", "run-1", artifacts)
	require.NoError(t, err)
	require.Len(t, writer.records, 2)
	require.Equal(t, "wf-1", writer.records[0].WorkflowID)
	require.Equal(t, "run-1", writer.records[0].RunID)
	require.Equal(t, string(euclotypes.ArtifactKindPlan), writer.records[0].Kind)
	require.Equal(t, "minimal patch", writer.records[0].SummaryText)
	require.Equal(t, "application/json", writer.records[0].ContentType)
	require.NotEmpty(t, writer.records[1].ArtifactID)
}
