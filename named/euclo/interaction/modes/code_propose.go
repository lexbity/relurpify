package modes

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// EditProposalPhase presents the proposed edits for user review before execution.
// This replaces the hardcoded implicitApproval in runtime/edit.go.
// Skipped when the change is small or user said "just do it".
type EditProposalPhase struct {
	// BuildProposal is an optional callback that builds edit proposals.
	// If nil, a placeholder proposal is built from state.
	BuildProposal func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.DraftContent, error)
}

func (p *EditProposalPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	var content interaction.DraftContent
	if p.BuildProposal != nil {
		var err error
		content, err = p.BuildProposal(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		content = defaultEditProposal(mc)
	}

	actions := []interaction.ActionSlot{
		{ID: "apply", Label: "Apply", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
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
		ID:          "skip",
		Label:       "Skip to verify",
		Kind:        interaction.ActionConfirm,
		TargetPhase: "verify",
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
		"propose.response": resp.ActionID,
		"propose.items":    content.Items,
	}

	if resp.ActionID == "skip" {
		return interaction.PhaseOutcome{
			Advance:      true,
			JumpTo:       "verify",
			StateUpdates: updates,
		}, nil
	}

	if resp.Text != "" {
		updates["propose.edit_text"] = resp.Text
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

func defaultEditProposal(mc interaction.PhaseMachineContext) interaction.DraftContent {
	scope, _ := mc.State["scope"].([]string)
	items := make([]interaction.DraftItem, 0, len(scope))
	for i, file := range scope {
		items = append(items, interaction.DraftItem{
			ID:        fmt.Sprintf("file-%d", i+1),
			Content:   fmt.Sprintf("Edit %s", file),
			Editable:  true,
			Removable: true,
		})
	}
	if len(items) == 0 {
		items = []interaction.DraftItem{
			{ID: "edit-1", Content: "Proposed edit", Editable: true, Removable: false},
		}
	}
	return interaction.DraftContent{
		Kind:    "edit_proposal",
		Items:   items,
		Addable: false,
	}
}
