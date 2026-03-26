package guidance

import (
	"testing"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
	"github.com/stretchr/testify/require"
)

func TestGuidanceRequestToFrame(t *testing.T) {
	req := testRequest()
	req.ID = "guidance-1"

	frame := GuidanceRequestToFrame(req)
	require.Equal(t, interaction.FrameQuestion, frame.Kind)
	require.Equal(t, "guidance", frame.Mode)
	require.Equal(t, string(req.Kind), frame.Phase)
	require.NotEmpty(t, frame.Actions)

	content, ok := frame.Content.(interaction.QuestionContent)
	require.True(t, ok)
	require.NotEmpty(t, content.Question)
	require.Equal(t, req.Description, content.Description)
	require.Len(t, content.Options, len(req.Choices))
	require.True(t, content.AllowFreetext)

	require.Equal(t, req.Choices[0].ID, frame.Actions[0].ID)
	require.Equal(t, interaction.ActionSelect, frame.Actions[0].Kind)
	require.True(t, frame.Actions[0].Default)
	require.Equal(t, interaction.ActionFreetext, frame.Actions[len(frame.Actions)-1].Kind)
}

func TestGuidanceDecisionFromResponseChoice(t *testing.T) {
	req := testRequest()
	req.ID = "guidance-1"

	decision := GuidanceDecisionFromResponse(req, interaction.UserResponse{ActionID: "skip"})
	require.Equal(t, req.ID, decision.RequestID)
	require.Equal(t, "skip", decision.ChoiceID)
	require.Empty(t, decision.Freetext)
	require.Equal(t, "user", decision.DecidedBy)
}

func TestGuidanceDecisionFromResponseFreetext(t *testing.T) {
	req := testRequest()
	req.ID = "guidance-1"

	decision := GuidanceDecisionFromResponse(req, interaction.UserResponse{ActionID: "freetext", Text: "try the smallest safe change"})
	require.Equal(t, req.ID, decision.RequestID)
	require.Empty(t, decision.ChoiceID)
	require.Equal(t, "try the smallest safe change", decision.Freetext)
}
