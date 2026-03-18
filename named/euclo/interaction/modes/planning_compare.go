package modes

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// ComparePhase presents a side-by-side comparison of candidates.
// Skipped when there's only one candidate or the user already selected.
type ComparePhase struct {
	// BuildComparison is an optional callback to build a custom comparison matrix.
	// If nil, a default comparison is built from candidate properties.
	BuildComparison func(candidates []interaction.Candidate) interaction.ComparisonContent
}

func (p *ComparePhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	candidates, _ := mc.State["generate.candidates"].([]interaction.Candidate)
	if len(candidates) == 0 {
		return interaction.PhaseOutcome{Advance: true}, nil
	}

	var content interaction.ComparisonContent
	if p.BuildComparison != nil {
		content = p.BuildComparison(candidates)
	} else {
		content = defaultComparison(candidates)
	}

	recommendedID, _ := mc.State["generate.candidates"].([]interaction.Candidate)
	recID := ""
	if len(recommendedID) > 0 {
		recID = recommendedID[0].ID
	}

	actions := make([]interaction.ActionSlot, 0, len(candidates)+1)
	for i, c := range candidates {
		actions = append(actions, interaction.ActionSlot{
			ID:       c.ID,
			Label:    fmt.Sprintf("Select: %s", c.Summary),
			Kind:     interaction.ActionSelect,
			Default:  i == 0,
			Shortcut: fmt.Sprintf("%d", i+1),
		})
	}
	if recID != "" {
		actions = append(actions, interaction.ActionSlot{
			ID:    "recommended",
			Label: "Go with recommended",
			Kind:  interaction.ActionConfirm,
		})
	}

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameComparison,
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

	selected := resp.ActionID
	if selected == "recommended" {
		selected = recID
	}

	updates := map[string]any{
		"compare.response": resp.ActionID,
		"generate.selected": selected,
	}

	for _, c := range candidates {
		if c.ID == selected {
			updates["generate.selected_summary"] = c.Summary
			break
		}
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

// defaultComparison builds a comparison matrix from candidate properties.
func defaultComparison(candidates []interaction.Candidate) interaction.ComparisonContent {
	// Collect all dimension keys.
	dimSet := map[string]bool{}
	for _, c := range candidates {
		for k := range c.Properties {
			dimSet[k] = true
		}
	}

	dimensions := make([]string, 0, len(dimSet))
	for k := range dimSet {
		dimensions = append(dimensions, k)
	}

	// Build matrix: candidates × dimensions.
	matrix := make([][]string, len(candidates))
	for i, c := range candidates {
		row := make([]string, len(dimensions))
		for j, dim := range dimensions {
			row[j] = c.Properties[dim]
			if row[j] == "" {
				row[j] = "-"
			}
		}
		matrix[i] = row
	}

	return interaction.ComparisonContent{
		Dimensions: dimensions,
		Matrix:     matrix,
	}
}
