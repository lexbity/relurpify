package modes

import (
	"context"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// IntentPhase emits a FrameProposal with the system's interpretation of the
// user's coding intent: scope, approach, and whether mutation is involved.
type IntentPhase struct {
	// AnalyzeIntent is an optional callback for classification-driven intent analysis.
	// If nil, a basic intent proposal is built from state.
	AnalyzeIntent func(ctx context.Context, mc interaction.PhaseMachineContext) (IntentAnalysis, error)
}

// IntentAnalysis holds the result of intent analysis for the proposal frame.
type IntentAnalysis struct {
	Interpretation string
	Scope          []string
	Approach       string
	Constraints    []string
	SmallChange    bool
	MutationFlag   bool
}

func (p *IntentPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	var analysis IntentAnalysis
	if p.AnalyzeIntent != nil {
		var err error
		analysis, err = p.AnalyzeIntent(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		analysis = defaultIntentAnalysis(mc)
	}

	content := interaction.ProposalContent{
		Interpretation: analysis.Interpretation,
		Scope:          analysis.Scope,
		Approach:       analysis.Approach,
		Constraints:    analysis.Constraints,
	}

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameProposal,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: content,
		Actions: []interaction.ActionSlot{
			{ID: "confirm", Label: "Confirm", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
			{ID: "correct", Label: "Correct", Kind: interaction.ActionFreetext},
			{ID: "plan_first", Label: "Plan first", Kind: interaction.ActionTransition, TargetPhase: "planning"},
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
		"understand.response":     resp.ActionID,
		"understand.proposal":     content,
		"understand.small_change": analysis.SmallChange,
		"understand.mutation":     analysis.MutationFlag,
	}

	switch resp.ActionID {
	case "correct":
		updates["understand.correction"] = resp.Text
	case "plan_first":
		return interaction.PhaseOutcome{
			Advance:      true,
			Transition:   "planning",
			StateUpdates: updates,
		}, nil
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

func defaultIntentAnalysis(mc interaction.PhaseMachineContext) IntentAnalysis {
	instruction, _ := mc.State["instruction"].(string)
	if instruction == "" {
		instruction = "Code modification"
	}

	scope, _ := mc.State["scope"].([]string)
	approach, _ := mc.State["approach"].(string)
	if approach == "" {
		approach = "edit_verify_repair"
	}

	var constraints []string
	if c, ok := mc.State["constraints"].([]string); ok {
		constraints = c
	}

	smallChange, _ := mc.State["small_change"].(bool)

	return IntentAnalysis{
		Interpretation: instruction,
		Scope:          scope,
		Approach:       approach,
		Constraints:    constraints,
		SmallChange:    smallChange,
		MutationFlag:   true,
	}
}
