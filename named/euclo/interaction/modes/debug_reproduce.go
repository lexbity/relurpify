package modes

import (
	"context"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// ReproductionPhase attempts to reproduce the reported issue and presents
// the result. Skipped when user said "skip reproduction" or "I know the cause".
type ReproductionPhase struct {
	// RunReproduction is an optional callback for actual reproduction.
	// If nil, a placeholder result is returned.
	RunReproduction func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.ResultContent, error)
}

func (p *ReproductionPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	// Emit status.
	statusFrame := interaction.InteractionFrame{
		Kind:  interaction.FrameStatus,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.StatusContent{
			Message: "Attempting to reproduce the issue...",
			Phase:   mc.Phase,
		},
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	if err := mc.Emitter.Emit(ctx, statusFrame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	var result interaction.ResultContent
	if p.RunReproduction != nil {
		var err error
		result, err = p.RunReproduction(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		result = interaction.ResultContent{
			Status: "reproduced",
			Evidence: []interaction.EvidenceItem{
				{Kind: "reproduction", Detail: "Issue reproduced successfully"},
			},
		}
	}

	actions := []interaction.ActionSlot{
		{ID: "continue", Label: "Continue to localization", Kind: interaction.ActionConfirm, Default: true},
		{ID: "wrong_error", Label: "Not the right error", Kind: interaction.ActionConfirm, TargetPhase: "intake"},
		{ID: "skip", Label: "I know the cause", Kind: interaction.ActionFreetext},
	}

	resultFrame := interaction.InteractionFrame{
		Kind:    interaction.FrameResult,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: result,
		Actions: actions,
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	if err := mc.Emitter.Emit(ctx, resultFrame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return interaction.PhaseOutcome{}, err
	}

	updates := map[string]any{
		"reproduce.response": resp.ActionID,
		"reproduce.result":   result,
	}

	switch resp.ActionID {
	case "wrong_error":
		return interaction.PhaseOutcome{
			Advance:      true,
			JumpTo:       "intake",
			StateUpdates: updates,
		}, nil
	case "skip":
		if resp.Text != "" {
			updates["reproduce.known_cause"] = resp.Text
		}
		updates["known_cause"] = true
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}
