package modes

import (
	"context"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// VerifyFailureThreshold is the number of consecutive failures before
// proposing a transition to debug mode.
const VerifyFailureThreshold = 2

// VerificationPhase runs verification and handles failure escalation.
// After VerifyFailureThreshold consecutive failures, proposes debug mode transition.
type VerificationPhase struct {
	// RunVerification is an optional callback for actual verification.
	// If nil, a placeholder "passed" result is returned.
	RunVerification func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.ResultContent, error)
}

func (p *VerificationPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	// Emit status.
	statusFrame := interaction.InteractionFrame{
		Kind:  interaction.FrameStatus,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.StatusContent{
			Message: "Running verification...",
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
	if p.RunVerification != nil {
		var err error
		result, err = p.RunVerification(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		result = interaction.ResultContent{Status: "passed"}
	}

	// Track consecutive failures.
	failCount, _ := mc.State["verify.failure_count"].(int)
	if result.Status == "failed" {
		failCount++
	} else {
		failCount = 0
	}

	updates := map[string]any{
		"verify.response":      "",
		"verify.result":        result,
		"verify.failure_count": failCount,
	}

	// Build actions based on result.
	actions := buildVerifyActions(result, failCount)

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

	updates["verify.response"] = resp.ActionID

	switch resp.ActionID {
	case "done":
		return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil

	case "re_verify":
		return interaction.PhaseOutcome{
			Advance:      true,
			JumpTo:       "verify",
			StateUpdates: updates,
		}, nil

	case "fix_gaps":
		return interaction.PhaseOutcome{
			Advance:      true,
			JumpTo:       "execute",
			StateUpdates: updates,
		}, nil

	case "debug":
		updates["verify.escalated"] = true
		return interaction.PhaseOutcome{
			Advance:      true,
			Transition:   "debug",
			StateUpdates: updates,
		}, nil

	case "different_approach":
		updates["execute.paradigm_switch"] = true
		return interaction.PhaseOutcome{
			Advance:      true,
			JumpTo:       "execute",
			StateUpdates: updates,
		}, nil

	default:
		return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
	}
}

// buildVerifyActions constructs actions appropriate for the verification result.
func buildVerifyActions(result interaction.ResultContent, failCount int) []interaction.ActionSlot {
	if result.Status == "passed" {
		return []interaction.ActionSlot{
			{ID: "done", Label: "Done", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
			{ID: "re_verify", Label: "Re-verify", Kind: interaction.ActionConfirm},
		}
	}

	// Failed or partial.
	actions := []interaction.ActionSlot{
		{ID: "fix_gaps", Label: "Fix gaps", Kind: interaction.ActionConfirm, Default: true},
		{ID: "re_verify", Label: "Re-verify", Kind: interaction.ActionConfirm},
		{ID: "different_approach", Label: "Try different approach", Kind: interaction.ActionConfirm},
	}

	// After threshold failures, prominently offer debug transition.
	if failCount >= VerifyFailureThreshold {
		actions = append([]interaction.ActionSlot{
			{ID: "debug", Label: "Switch to debug mode", Kind: interaction.ActionTransition, Default: true},
		}, actions...)
		// Remove default from fix_gaps since debug is now default.
		for i := range actions {
			if actions[i].ID == "fix_gaps" {
				actions[i].Default = false
			}
		}
	} else {
		actions = append(actions, interaction.ActionSlot{
			ID:    "debug",
			Label: "Debug this",
			Kind:  interaction.ActionTransition,
		})
	}

	return actions
}
