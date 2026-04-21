package modes

import (
	"context"
	"fmt"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// RefinePhase presents the selected plan as an editable draft.
// Skipped when user said "just plan it".
type RefinePhase struct {
	// BuildDraft is an optional callback to build a custom draft from the selected plan.
	// If nil, a default single-item draft is created.
	BuildDraft func(mc interaction.PhaseMachineContext) interaction.DraftContent
}

func (p *RefinePhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	var content interaction.DraftContent
	if p.BuildDraft != nil {
		content = p.BuildDraft(mc)
	} else {
		content = defaultDraft(mc)
	}

	actions := []interaction.ActionSlot{
		{ID: "commit", Label: "Commit plan", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
	}
	for i, item := range content.Items {
		if item.Editable {
			actions = append(actions, interaction.ActionSlot{
				ID:    fmt.Sprintf("edit_%s", item.ID),
				Label: fmt.Sprintf("Edit step %d", i+1),
				Kind:  interaction.ActionFreetext,
			})
		}
	}
	if content.Addable {
		actions = append(actions, interaction.ActionSlot{
			ID:    "add",
			Label: "Add step",
			Kind:  interaction.ActionFreetext,
		})
	}
	for _, item := range content.Items {
		if item.Removable {
			actions = append(actions, interaction.ActionSlot{
				ID:    fmt.Sprintf("remove_%s", item.ID),
				Label: fmt.Sprintf("Remove %s", item.ID),
				Kind:  interaction.ActionConfirm,
			})
		}
	}

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
		"refine.response": resp.ActionID,
		"refine.items":    content.Items,
	}
	if resp.Text != "" {
		updates["refine.edit_text"] = resp.Text
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

func defaultDraft(mc interaction.PhaseMachineContext) interaction.DraftContent {
	summary, _ := mc.State["generate.selected_summary"].(string)
	if summary == "" {
		summary = "Plan step 1"
	}
	return interaction.DraftContent{
		Kind: "plan",
		Items: []interaction.DraftItem{
			{ID: "step-1", Content: summary, Editable: true, Removable: false},
		},
		Addable: true,
	}
}
