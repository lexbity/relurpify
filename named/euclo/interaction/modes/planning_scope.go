package modes

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// ScopePhase emits a FrameProposal with the system's interpretation
// of the task scope. Always runs (not skippable).
type ScopePhase struct {
	// AnalyzeScope is an optional callback for workspace-aware scope analysis.
	// If nil, a basic scope proposal is built from state.
	AnalyzeScope func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.ProposalContent, error)
}

func (p *ScopePhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	var content interaction.ProposalContent
	if p.AnalyzeScope != nil {
		var err error
		content, err = p.AnalyzeScope(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		content = defaultScopeProposal(mc)
	}

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameProposal,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: content,
		Actions: []interaction.ActionSlot{
			{ID: "confirm", Label: "Confirm scope", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
			{ID: "clarify", Label: "Clarify", Kind: interaction.ActionFreetext},
			{ID: "broaden", Label: "Broaden scope", Kind: interaction.ActionFreetext},
		},
		Continuable: true,
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
		"scope.response": resp.ActionID,
		"scope.proposal": content,
	}

	switch resp.ActionID {
	case "clarify", "broaden":
		updates["scope.correction"] = resp.Text
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

// defaultScopeProposal builds a basic scope proposal from available state.
func defaultScopeProposal(mc interaction.PhaseMachineContext) interaction.ProposalContent {
	instruction, _ := mc.State["instruction"].(string)
	if instruction == "" {
		instruction = "Task instruction not provided"
	}

	scope, _ := mc.State["scope"].([]string)
	approach, _ := mc.State["approach"].(string)
	if approach == "" {
		approach = "plan_stage_execute"
	}

	var constraints []string
	if c, ok := mc.State["constraints"].([]string); ok {
		constraints = c
	}

	return interaction.ProposalContent{
		Interpretation: instruction,
		Scope:          scope,
		Approach:       approach,
		Constraints:    constraints,
	}
}
