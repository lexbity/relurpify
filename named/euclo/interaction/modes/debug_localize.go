package modes

import (
	"context"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// LocalizationPhase presents an evidence chain pointing to the root cause.
// Not skippable — localization is always required before proposing a fix.
type LocalizationPhase struct {
	// RunLocalization is an optional callback for actual localization.
	// If nil, a placeholder evidence chain is returned.
	RunLocalization func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.ResultContent, error)
}

func (p *LocalizationPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	// Emit status.
	statusFrame := interaction.InteractionFrame{
		Kind:  interaction.FrameStatus,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.StatusContent{
			Message: "Localizing root cause...",
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
	if p.RunLocalization != nil {
		var err error
		result, err = p.RunLocalization(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		result = interaction.ResultContent{
			Status: "localized",
			Evidence: []interaction.EvidenceItem{
				{Kind: "code_read", Detail: "Root cause identified", Confidence: 0.8},
			},
		}
	}

	actions := []interaction.ActionSlot{
		{ID: "fix", Label: "Fix this", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
		{ID: "deeper", Label: "Investigate deeper", Kind: interaction.ActionConfirm},
		{ID: "regression", Label: "Show recent changes", Kind: interaction.ActionConfirm,
			CapabilityTrigger: "euclo:debug.investigate_regression"},
		{ID: "override", Label: "It's in a different location", Kind: interaction.ActionFreetext},
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
		"localize.response": resp.ActionID,
		"localize.result":   result,
	}

	switch resp.ActionID {
	case "deeper":
		// Re-run localization with deeper investigation.
		return interaction.PhaseOutcome{
			Advance:      true,
			JumpTo:       "localize",
			StateUpdates: updates,
		}, nil
	case "override":
		updates["localize.override_location"] = resp.Text
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}
