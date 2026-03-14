package euclo

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/stretchr/testify/require"
)

func TestResolveModePrefersExplicitHintOverResumedState(t *testing.T) {
	envelope := TaskEnvelope{
		ModeHint:           "debug",
		ResumedMode:        "planning",
		EditPermitted:      true,
		Instruction:        "fix the failing test",
		CapabilitySnapshot: CapabilitySnapshot{HasWriteTools: true},
	}
	classification := ClassifyTask(envelope)

	resolution := ResolveMode(envelope, classification, DefaultModeRegistry())
	require.Equal(t, "debug", resolution.ModeID)
	require.Equal(t, "explicit", resolution.Source)
}

func TestSelectExecutionProfileFallsBackToNonMutatingWhenWritesUnavailable(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:        "implement the requested change",
		EditPermitted:      false,
		CapabilitySnapshot: CapabilitySnapshot{},
	}
	classification := ClassifyTask(envelope)
	mode := ResolveMode(envelope, classification, DefaultModeRegistry())

	selection := SelectExecutionProfile(envelope, classification, mode, DefaultExecutionProfileRegistry())
	require.Equal(t, "plan_stage_execute", selection.ProfileID)
	require.False(t, selection.MutationAllowed)
}

func TestSelectExecutionProfileUpgradesCodeToEvidenceFirstForDebugLikeTask(t *testing.T) {
	envelope := TaskEnvelope{
		Instruction:        "fix the failing test and diagnose the root cause",
		EditPermitted:      true,
		CapabilitySnapshot: CapabilitySnapshot{HasWriteTools: true, HasVerificationTools: true},
	}
	classification := ClassifyTask(envelope)
	mode := ResolveMode(envelope, classification, DefaultModeRegistry())

	selection := SelectExecutionProfile(envelope, classification, mode, DefaultExecutionProfileRegistry())
	require.Equal(t, "reproduce_localize_patch", selection.ProfileID)
	require.True(t, selection.VerificationRequired)
}

func TestNormalizeTaskEnvelopeReadsModeHintsAndPriorArtifacts(t *testing.T) {
	state := core.NewContext()
	state.Set("euclo.mode", "planning")
	state.Set("euclo.artifacts", []Artifact{{Kind: ArtifactKindPlan}})

	envelope := NormalizeTaskEnvelope(&core.Task{
		ID:          "task-1",
		Instruction: "review the patch",
		Context:     map[string]any{"workspace": "/tmp/ws", "mode": "review"},
	}, state, nil)

	require.Equal(t, "task-1", envelope.TaskID)
	require.Equal(t, "/tmp/ws", envelope.Workspace)
	require.Equal(t, "review", envelope.ModeHint)
	require.Equal(t, "planning", envelope.ResumedMode)
	require.Equal(t, []string{string(ArtifactKindPlan)}, envelope.PreviousArtifactKinds)
}
