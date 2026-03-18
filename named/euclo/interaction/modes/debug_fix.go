package modes

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// DebugFixProposalPhase presents a fix proposal with regression risk assessment.
// Similar to code mode's EditProposalPhase but framed as a fix.
type DebugFixProposalPhase struct {
	// BuildFixProposal is an optional callback for building the fix proposal.
	// If nil, a default proposal is built from localization results.
	BuildFixProposal func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.DraftContent, error)
}

func (p *DebugFixProposalPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	var content interaction.DraftContent
	if p.BuildFixProposal != nil {
		var err error
		content, err = p.BuildFixProposal(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		content = defaultFixProposal(mc)
	}

	actions := []interaction.ActionSlot{
		{ID: "apply", Label: "Apply fix", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
	}
	for _, item := range content.Items {
		if item.Editable {
			actions = append(actions, interaction.ActionSlot{
				ID:    fmt.Sprintf("edit_%s", item.ID),
				Label: fmt.Sprintf("Edit %s", item.ID),
				Kind:  interaction.ActionFreetext,
			})
		}
	}
	actions = append(actions, interaction.ActionSlot{
		ID:    "reject",
		Label: "Reject, investigate more",
		Kind:  interaction.ActionConfirm,
	})

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameDraft,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: content,
		Actions: actions,
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
		"propose_fix.response": resp.ActionID,
		"propose_fix.items":    content.Items,
	}

	if resp.ActionID == "reject" {
		return interaction.PhaseOutcome{
			Advance:      true,
			JumpTo:       "localize",
			StateUpdates: updates,
		}, nil
	}

	if resp.Text != "" {
		updates["propose_fix.edit_text"] = resp.Text
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

func defaultFixProposal(mc interaction.PhaseMachineContext) interaction.DraftContent {
	location := "unknown"
	if result, ok := mc.State["localize.result"].(interaction.ResultContent); ok {
		for _, e := range result.Evidence {
			if e.Location != "" {
				location = e.Location
				break
			}
		}
	}

	return interaction.DraftContent{
		Kind: "fix_proposal",
		Items: []interaction.DraftItem{
			{ID: "fix-1", Content: fmt.Sprintf("Fix at %s", location), Editable: true, Removable: false},
		},
		Addable: false,
	}
}
