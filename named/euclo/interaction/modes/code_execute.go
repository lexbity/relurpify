package modes

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// CodeExecutionPhase runs the selected capability and emits status/result frames.
type CodeExecutionPhase struct {
	// RunCapability is an optional callback for actual capability execution.
	// If nil, a placeholder result is returned.
	RunCapability func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.ResultContent, []euclotypes.Artifact, error)
}

func (p *CodeExecutionPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	// Emit status.
	statusFrame := interaction.InteractionFrame{
		Kind:  interaction.FrameStatus,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.StatusContent{
			Message: "Executing code changes...",
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
	var artifacts []euclotypes.Artifact

	if p.RunCapability != nil {
		var err error
		result, artifacts, err = p.RunCapability(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		result = interaction.ResultContent{
			Status: "completed",
		}
	}

	// Emit result.
	resultFrame := interaction.InteractionFrame{
		Kind:    interaction.FrameResult,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: result,
		Actions: []interaction.ActionSlot{
			{ID: "continue", Label: "Continue to verification", Kind: interaction.ActionConfirm, Default: true},
		},
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
		"execute.response": resp.ActionID,
		"execute.result":   result,
	}

	return interaction.PhaseOutcome{
		Advance:      true,
		Artifacts:    artifacts,
		StateUpdates: updates,
	}, nil
}
