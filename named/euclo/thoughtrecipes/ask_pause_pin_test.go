package thoughtrecipe

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	euclostate "codeburg.org/lexbit/relurpify/named/euclo/state"
)

// TestAskNodePauseRulingPin is the R-2 pin: the DSL ask pause is production
// surface — the ask node emits the frame, stores the resume node, and its
// result metadata carries euclo.interaction.pause (the key the graph
// short-circuits on). A future change that deletes the pause key fails HERE
// before it silently changes the ruled ask semantics.
func TestAskNodePauseRulingPin(t *testing.T) {
	step := ExecutionStep{
		ID:       "review.ask",
		Kind:     StepKindAsk,
		Question: "proceed with the refactor?",
		Choices:  []string{"yes", "no"},
	}
	node := NewAskNode("review.ask", goldenDeps(t), step)
	env := contextdata.NewEnvelope("task-ask", "session-ask")

	result, err := node.Execute(context.Background(), env)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)

	require.NotNil(t, result.Metadata, "the pause outcome carries metadata")
	pause, ok := result.Metadata["euclo.interaction.pause"].(bool)
	require.True(t, ok, "the pause key must be present (R-2 pin)")
	require.True(t, pause, "the first execution of an unanswered ask pauses")

	fields := execution.ResultFields(result.Data)
	require.Equal(t, true, fields["paused"])
	require.NotEmpty(t, fields["frame_id"])

	// The resume node is stored for the resume path (R-2's re-entry seam).
	require.Equal(t, "review.ask", euclostate.GetInteractionResumeNodeID(env))
}
