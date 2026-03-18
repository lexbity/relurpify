package modes

import (
	"context"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// CodePresentPhase emits a summary of changes, verification, and artifacts.
type CodePresentPhase struct{}

func (p *CodePresentPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	// Build summary from accumulated state.
	var description string
	if result, ok := mc.State["verify.result"].(interaction.ResultContent); ok && result.Status == "passed" {
		description = "Changes applied and verified successfully"
	} else {
		description = "Changes applied"
	}

	var artifactRefs []string
	for _, a := range mc.Artifacts.All() {
		artifactRefs = append(artifactRefs, string(a.Kind))
	}

	var changes []string
	if scope, ok := mc.State["scope"].([]string); ok {
		changes = scope
	}

	content := interaction.SummaryContent{
		Description: description,
		Artifacts:   artifactRefs,
		Changes:     changes,
	}

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameSummary,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: content,
		Actions: []interaction.ActionSlot{
			{ID: "accept", Label: "Accept", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
			{ID: "undo", Label: "Undo changes", Kind: interaction.ActionConfirm},
			{ID: "review", Label: "Review", Kind: interaction.ActionTransition, TargetPhase: "review"},
		},
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}

	if err := mc.Emitter.Emit(ctx, frame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return interaction.PhaseOutcome{}, err
	}

	updates := map[string]any{
		"present.response": resp.ActionID,
	}

	outcome := interaction.PhaseOutcome{
		Advance:      true,
		StateUpdates: updates,
	}

	switch resp.ActionID {
	case "undo":
		updates["present.undo"] = true
	case "review":
		outcome.Transition = "review"
	}

	return outcome, nil
}
