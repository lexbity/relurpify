package modes

import (
	"context"
	"time"

	"codeburg.org/lexbit/relurpify/named/euclo/euclotypes"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
)

// CommitPhase produces the final plan artifact and presents a summary.
type CommitPhase struct{}

func (p *CommitPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	artifact := planArtifact(mc.State)

	summary, _ := mc.State["generate.selected_summary"].(string)
	if summary == "" {
		summary = "Plan committed"
	}

	var artifactRefs []string
	for _, a := range mc.Artifacts.All() {
		artifactRefs = append(artifactRefs, string(a.Kind))
	}
	artifactRefs = append(artifactRefs, string(euclotypes.ArtifactKindPlan))

	content := interaction.SummaryContent{
		Description: summary,
		Artifacts:   artifactRefs,
	}

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameSummary,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: content,
		Actions: []interaction.ActionSlot{
			{ID: "execute", Label: "Execute plan", Shortcut: "y", Kind: interaction.ActionTransition, Default: true, TargetPhase: "code"},
			{ID: "save", Label: "Save plan only", Kind: interaction.ActionConfirm},
		},
		Metadata: interaction.FrameMetadata{
			Timestamp:    time.Now(),
			PhaseIndex:   mc.PhaseIndex,
			PhaseCount:   mc.PhaseCount,
			ArtifactRefs: []string{artifact.ID},
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
		"commit.response": resp.ActionID,
	}

	outcome := interaction.PhaseOutcome{
		Advance:      true,
		Artifacts:    []euclotypes.Artifact{artifact},
		StateUpdates: updates,
	}

	if resp.ActionID == "execute" {
		outcome.Transition = "code"
	}

	return outcome, nil
}
