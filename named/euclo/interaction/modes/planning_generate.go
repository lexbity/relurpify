package modes

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// GeneratePhase produces plan candidates and presents them for selection.
type GeneratePhase struct {
	// GenerateCandidates is an optional callback for capability-driven plan generation.
	// If nil, a single placeholder candidate is created.
	GenerateCandidates func(ctx context.Context, mc interaction.PhaseMachineContext) ([]interaction.Candidate, error)
}

func (p *GeneratePhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	// Emit status while generating.
	statusFrame := interaction.InteractionFrame{
		Kind:  interaction.FrameStatus,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.StatusContent{
			Message: "Generating plan candidates...",
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

	var candidates []interaction.Candidate
	if p.GenerateCandidates != nil {
		var err error
		candidates, err = p.GenerateCandidates(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	}
	if len(candidates) == 0 {
		candidates = []interaction.Candidate{
			{ID: "plan-1", Summary: "Default plan", Properties: map[string]string{"approach": "standard"}},
		}
	}

	recommendedID := candidates[0].ID
	content := interaction.CandidatesContent{
		Candidates:    candidates,
		RecommendedID: recommendedID,
	}

	actions := make([]interaction.ActionSlot, 0, len(candidates)+2)
	for i, c := range candidates {
		actions = append(actions, interaction.ActionSlot{
			ID:       c.ID,
			Label:    c.Summary,
			Kind:     interaction.ActionSelect,
			Default:  i == 0,
			Shortcut: fmt.Sprintf("%d", i+1),
		})
	}
	actions = append(actions, interaction.ActionSlot{
		ID:    "merge",
		Label: "Merge ideas from multiple",
		Kind:  interaction.ActionFreetext,
	})
	actions = append(actions, interaction.ActionSlot{
		ID:    "more",
		Label: "Show more alternatives",
		Kind:  interaction.ActionConfirm,
	})

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameCandidates,
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
		"generate.response":        resp.ActionID,
		"generate.candidate_count": len(candidates),
		"generate.candidates":      candidates,
	}

	switch resp.ActionID {
	case "more":
		// Re-run generation — jump back to generate.
		return interaction.PhaseOutcome{
			Advance:      true,
			JumpTo:       "generate",
			StateUpdates: updates,
		}, nil
	case "merge":
		updates["generate.merge_request"] = resp.Text
		updates["generate.selected"] = recommendedID
		updates["generate.selected_summary"] = "merged plan"
	default:
		// User selected a candidate by ID.
		updates["generate.selected"] = resp.ActionID
		for _, c := range candidates {
			if c.ID == resp.ActionID {
				updates["generate.selected_summary"] = c.Summary
				break
			}
		}
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}
