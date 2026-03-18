package interaction

import (
	"context"
	"fmt"
	"time"
)

// ConfirmationPhase emits a Proposal frame and awaits confirm/reject/freetext correction.
type ConfirmationPhase struct {
	BuildProposal func(mc PhaseMachineContext) ProposalContent
}

func (p *ConfirmationPhase) Execute(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
	content := p.BuildProposal(mc)

	frame := InteractionFrame{
		Kind:  FrameProposal,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: content,
		Actions: []ActionSlot{
			{ID: "confirm", Label: "Confirm", Shortcut: "y", Kind: ActionConfirm, Default: true},
			{ID: "reject", Label: "Reject", Shortcut: "n", Kind: ActionConfirm},
			{ID: "correct", Label: "Correct", Kind: ActionFreetext},
		},
		Continuable: true,
		Metadata: FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}

	if err := mc.Emitter.Emit(ctx, frame); err != nil {
		return PhaseOutcome{}, err
	}

	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return PhaseOutcome{}, err
	}

	updates := map[string]any{
		mc.Phase + ".response":     resp.ActionID,
		mc.Phase + ".proposal":     content,
	}

	switch resp.ActionID {
	case "confirm":
		return PhaseOutcome{Advance: true, StateUpdates: updates}, nil
	case "reject":
		return PhaseOutcome{Advance: false, StateUpdates: updates}, nil
	case "correct":
		updates[mc.Phase+".correction"] = resp.Text
		return PhaseOutcome{Advance: true, StateUpdates: updates}, nil
	default:
		return PhaseOutcome{Advance: true, StateUpdates: updates}, nil
	}
}

// QuestionPhase emits a Question frame with options and collects the answer.
type QuestionPhase struct {
	BuildQuestion func(mc PhaseMachineContext) QuestionContent
}

func (p *QuestionPhase) Execute(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
	content := p.BuildQuestion(mc)

	actions := make([]ActionSlot, 0, len(content.Options)+2)
	for i, opt := range content.Options {
		actions = append(actions, ActionSlot{
			ID:       opt.ID,
			Label:    opt.Label,
			Kind:     ActionSelect,
			Default:  i == 0,
			Shortcut: fmt.Sprintf("%d", i+1),
		})
	}
	if content.AllowFreetext {
		actions = append(actions, ActionSlot{
			ID:    "freetext",
			Label: "Type answer",
			Kind:  ActionFreetext,
		})
	}
	actions = append(actions, ActionSlot{
		ID:    "skip",
		Label: "Skip",
		Kind:  ActionConfirm,
	})

	frame := InteractionFrame{
		Kind:    FrameQuestion,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: content,
		Actions: actions,
		Metadata: FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}

	if err := mc.Emitter.Emit(ctx, frame); err != nil {
		return PhaseOutcome{}, err
	}

	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return PhaseOutcome{}, err
	}

	updates := map[string]any{
		mc.Phase + ".response": resp.ActionID,
	}
	if resp.Text != "" {
		updates[mc.Phase+".text"] = resp.Text
	}
	if len(resp.Selections) > 0 {
		updates[mc.Phase+".selections"] = resp.Selections
	}

	return PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

// ExecutionPhase runs a capability and emits Status/Result frames.
// The actual capability execution is provided via the RunFunc callback.
type ExecutionPhase struct {
	// RunFunc performs the actual work. It receives the machine context and
	// should return a ResultContent describing what happened.
	RunFunc func(ctx context.Context, mc PhaseMachineContext) (ResultContent, error)
}

func (p *ExecutionPhase) Execute(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
	// Emit status frame before execution.
	statusFrame := InteractionFrame{
		Kind:  FrameStatus,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: StatusContent{
			Message: "Executing...",
			Phase:   mc.Phase,
		},
		Metadata: FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	if err := mc.Emitter.Emit(ctx, statusFrame); err != nil {
		return PhaseOutcome{}, err
	}

	result, err := p.RunFunc(ctx, mc)
	if err != nil {
		return PhaseOutcome{}, err
	}

	// Emit result frame.
	resultFrame := InteractionFrame{
		Kind:    FrameResult,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: result,
		Actions: []ActionSlot{
			{ID: "continue", Label: "Continue", Kind: ActionConfirm, Default: true},
		},
		Metadata: FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}
	if err := mc.Emitter.Emit(ctx, resultFrame); err != nil {
		return PhaseOutcome{}, err
	}

	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return PhaseOutcome{}, err
	}

	updates := map[string]any{
		mc.Phase + ".result":   result,
		mc.Phase + ".response": resp.ActionID,
	}

	return PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

// SummaryPhase emits a Summary frame with all produced artifacts.
type SummaryPhase struct {
	BuildSummary func(mc PhaseMachineContext) SummaryContent
}

func (p *SummaryPhase) Execute(ctx context.Context, mc PhaseMachineContext) (PhaseOutcome, error) {
	content := p.BuildSummary(mc)

	frame := InteractionFrame{
		Kind:    FrameSummary,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: content,
		Actions: []ActionSlot{
			{ID: "done", Label: "Done", Kind: ActionConfirm, Default: true},
		},
		Metadata: FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}

	if err := mc.Emitter.Emit(ctx, frame); err != nil {
		return PhaseOutcome{}, err
	}

	resp, err := mc.Emitter.AwaitResponse(ctx)
	if err != nil {
		return PhaseOutcome{}, err
	}

	updates := map[string]any{
		mc.Phase + ".response": resp.ActionID,
	}

	return PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}
