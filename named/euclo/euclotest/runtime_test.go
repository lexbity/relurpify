package euclotest

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	eucloruntime "github.com/lexcodex/relurpify/named/euclo/runtime"
	"github.com/stretchr/testify/require"
)

func TestResolveModePrefersExplicitHintOverResumedState(t *testing.T) {
	envelope := eucloruntime.TaskEnvelope{
		ModeHint:           "debug",
		ResumedMode:        "planning",
		EditPermitted:      true,
		Instruction:        "fix the failing test",
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{HasWriteTools: true},
	}
	classification := eucloruntime.ClassifyTask(envelope)

	resolution := eucloruntime.ResolveMode(envelope, classification, euclotypes.DefaultModeRegistry())
	require.Equal(t, "debug", resolution.ModeID)
	require.Equal(t, "explicit", resolution.Source)
}

func TestSelectExecutionProfileFallsBackToNonMutatingWhenWritesUnavailable(t *testing.T) {
	envelope := eucloruntime.TaskEnvelope{
		Instruction:        "implement the requested change",
		EditPermitted:      false,
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{},
	}
	classification := eucloruntime.ClassifyTask(envelope)
	mode := eucloruntime.ResolveMode(envelope, classification, euclotypes.DefaultModeRegistry())

	selection := eucloruntime.SelectExecutionProfile(envelope, classification, mode, euclotypes.DefaultExecutionProfileRegistry())
	require.NotEmpty(t, selection.ProfileID)
	require.False(t, selection.MutationAllowed)
	require.Contains(t, selection.PhaseRoutes, "plan")
}

func TestSelectExecutionProfileRequiresEvidenceFirstForDebugLikeTask(t *testing.T) {
	envelope := eucloruntime.TaskEnvelope{
		Instruction:        "fix the failing test and diagnose the root cause",
		EditPermitted:      true,
		CapabilitySnapshot: euclotypes.CapabilitySnapshot{HasWriteTools: true, HasVerificationTools: true},
	}
	classification := eucloruntime.ClassifyTask(envelope)
	mode := eucloruntime.ResolveMode(envelope, classification, euclotypes.DefaultModeRegistry())

	selection := eucloruntime.SelectExecutionProfile(envelope, classification, mode, euclotypes.DefaultExecutionProfileRegistry())
	require.NotEmpty(t, selection.ProfileID)
	require.True(t, classification.RequiresEvidenceBeforeMutation)
	require.True(t, selection.VerificationRequired)
	require.Contains(t, selection.PhaseRoutes, "verify")
}

func TestNormalizeTaskEnvelopeReadsModeHintsAndPriorArtifacts(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.mode", "planning")
	state.Set("euclo.artifacts", []euclotypes.Artifact{{Kind: euclotypes.ArtifactKindPlan}})

	envelope := eucloruntime.NormalizeTaskEnvelope(&core.Task{
		ID:          "task-1",
		Instruction: "review the patch",
		Context:     map[string]any{"workspace": "/tmp/ws", "mode": "review"},
	}, state, nil)

	require.Equal(t, "task-1", envelope.TaskID)
	require.Equal(t, "/tmp/ws", envelope.Workspace)
	require.Equal(t, "review", envelope.ModeHint)
	require.Equal(t, "planning", envelope.ResumedMode)
	require.Equal(t, []string{string(euclotypes.ArtifactKindPlan)}, envelope.PreviousArtifactKinds)
}
