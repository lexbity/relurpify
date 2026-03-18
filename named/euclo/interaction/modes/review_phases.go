package modes

import (
	"context"
	"fmt"
	"time"

	"github.com/lexcodex/relurpify/named/euclo/interaction"
)

// ReviewScopePhase proposes the review scope — detected from recent changes,
// full file, or PR diff.
type ReviewScopePhase struct {
	// AnalyzeScope is an optional callback for workspace-aware scope detection.
	AnalyzeScope func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.ProposalContent, error)
}

func (p *ReviewScopePhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	var content interaction.ProposalContent
	if p.AnalyzeScope != nil {
		var err error
		content, err = p.AnalyzeScope(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		content = defaultReviewScope(mc)
	}

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameProposal,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: content,
		Actions: []interaction.ActionSlot{
			{ID: "confirm", Label: "Review this scope", Shortcut: "y", Kind: interaction.ActionConfirm, Default: true},
			{ID: "narrow", Label: "Narrow to file", Kind: interaction.ActionFreetext},
			{ID: "focus", Label: "Add focus lens", Kind: interaction.ActionFreetext},
			{ID: "compat", Label: "Check compatibility", Kind: interaction.ActionConfirm,
				CapabilityTrigger: "euclo:review.compatibility"},
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
	case "narrow":
		updates["scope.narrow_file"] = resp.Text
	case "focus":
		updates["scope.focus_lens"] = resp.Text
	case "compat":
		updates["scope.check_compatibility"] = true
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

func defaultReviewScope(mc interaction.PhaseMachineContext) interaction.ProposalContent {
	scope, _ := mc.State["scope"].([]string)
	instruction, _ := mc.State["instruction"].(string)
	if instruction == "" {
		instruction = "Review recent changes"
	}
	return interaction.ProposalContent{
		Interpretation: instruction,
		Scope:          scope,
		Approach:       "review_suggest_implement",
	}
}

// ReviewSweepPhase runs the review capability and streams progress.
type ReviewSweepPhase struct {
	// RunReview is an optional callback for actual review execution.
	RunReview func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.FindingsContent, error)
}

func (p *ReviewSweepPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	statusFrame := interaction.InteractionFrame{
		Kind:  interaction.FrameStatus,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.StatusContent{
			Message: "Reviewing code...",
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

	var findings interaction.FindingsContent
	if p.RunReview != nil {
		var err error
		findings, err = p.RunReview(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		findings = interaction.FindingsContent{}
	}

	updates := map[string]any{
		"sweep.findings": findings,
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

// TriagePhase presents findings grouped by severity and lets the user
// decide which to fix.
type TriagePhase struct{}

func (p *TriagePhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	findings, _ := mc.State["sweep.findings"].(interaction.FindingsContent)

	totalFindings := len(findings.Critical) + len(findings.Warning) + len(findings.Info)
	if totalFindings == 0 {
		// No findings — emit summary and mark no fixes needed.
		frame := interaction.InteractionFrame{
			Kind:  interaction.FrameResult,
			Mode:  mc.Mode,
			Phase: mc.Phase,
			Content: interaction.ResultContent{
				Status: "passed",
			},
			Actions: []interaction.ActionSlot{
				{ID: "done", Label: "Done", Kind: interaction.ActionConfirm, Default: true},
			},
			Metadata: interaction.FrameMetadata{
				Timestamp:  time.Now(),
				PhaseIndex: mc.PhaseIndex,
				PhaseCount: mc.PhaseCount,
			},
		}
		if err := mc.Emitter.Emit(ctx, frame); err != nil {
			return interaction.PhaseOutcome{}, err
		}
		if _, err := mc.Emitter.AwaitResponse(ctx); err != nil {
			return interaction.PhaseOutcome{}, err
		}
		return interaction.PhaseOutcome{
			Advance:      true,
			StateUpdates: map[string]any{"triage.no_fixes": true},
		}, nil
	}

	// Present findings.
	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameResult,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: findings,
		Actions: buildTriageActions(findings),
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
		"triage.response": resp.ActionID,
		"triage.findings": findings,
	}

	switch resp.ActionID {
	case "no_fixes":
		updates["triage.no_fixes"] = true
	case "fix_critical":
		updates["triage.fix_scope"] = "critical"
	case "fix_all":
		updates["triage.fix_scope"] = "all"
	case "pick":
		updates["triage.fix_scope"] = "selected"
		updates["triage.selections"] = resp.Selections
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

func buildTriageActions(findings interaction.FindingsContent) []interaction.ActionSlot {
	actions := []interaction.ActionSlot{}

	if len(findings.Critical) > 0 {
		actions = append(actions, interaction.ActionSlot{
			ID:      "fix_critical",
			Label:   fmt.Sprintf("Fix all critical (%d)", len(findings.Critical)),
			Kind:    interaction.ActionBatch,
			Default: true,
		})
	}

	total := len(findings.Critical) + len(findings.Warning) + len(findings.Info)
	actions = append(actions, interaction.ActionSlot{
		ID:    "fix_all",
		Label: fmt.Sprintf("Fix all (%d)", total),
		Kind:  interaction.ActionBatch,
	})
	actions = append(actions, interaction.ActionSlot{
		ID:    "pick",
		Label: "Pick which to fix",
		Kind:  interaction.ActionToggle,
	})
	actions = append(actions, interaction.ActionSlot{
		ID:    "no_fixes",
		Label: "No fixes needed",
		Kind:  interaction.ActionConfirm,
	})

	// If no critical, make fix_all the default.
	if len(findings.Critical) == 0 && len(actions) > 0 {
		actions[0].Default = true
	}

	return actions
}

// BatchFixPhase applies selected fixes and presents the result.
type BatchFixPhase struct {
	// RunFixes is an optional callback for applying batch fixes.
	RunFixes func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.ResultContent, error)
}

func (p *BatchFixPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	statusFrame := interaction.InteractionFrame{
		Kind:  interaction.FrameStatus,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.StatusContent{
			Message: "Applying fixes...",
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
	if p.RunFixes != nil {
		var err error
		result, err = p.RunFixes(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		fixScope, _ := mc.State["triage.fix_scope"].(string)
		result = interaction.ResultContent{
			Status: "completed",
			Evidence: []interaction.EvidenceItem{
				{Kind: "batch_fix", Detail: fmt.Sprintf("Fixes applied (scope: %s)", fixScope)},
			},
		}
	}

	frame := interaction.InteractionFrame{
		Kind:    interaction.FrameResult,
		Mode:    mc.Mode,
		Phase:   mc.Phase,
		Content: result,
		Actions: []interaction.ActionSlot{
			{ID: "re_review", Label: "Re-review", Kind: interaction.ActionConfirm, Default: true},
			{ID: "accept", Label: "Accept without re-review", Kind: interaction.ActionConfirm},
		},
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
		"act.response": resp.ActionID,
		"act.result":   result,
	}

	if resp.ActionID == "accept" {
		updates["act.skip_re_review"] = true
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}

// ReReviewPhase runs the review again after fixes to verify no new issues.
type ReReviewPhase struct {
	// RunReview is an optional callback for re-review execution.
	RunReview func(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.FindingsContent, error)
}

func (p *ReReviewPhase) Execute(ctx context.Context, mc interaction.PhaseMachineContext) (interaction.PhaseOutcome, error) {
	statusFrame := interaction.InteractionFrame{
		Kind:  interaction.FrameStatus,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.StatusContent{
			Message: "Re-reviewing after fixes...",
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

	var findings interaction.FindingsContent
	if p.RunReview != nil {
		var err error
		findings, err = p.RunReview(ctx, mc)
		if err != nil {
			return interaction.PhaseOutcome{}, err
		}
	} else {
		findings = interaction.FindingsContent{}
	}

	totalFindings := len(findings.Critical) + len(findings.Warning) + len(findings.Info)

	var resultStatus string
	if totalFindings == 0 {
		resultStatus = "passed"
	} else {
		resultStatus = "partial"
	}

	frame := interaction.InteractionFrame{
		Kind:  interaction.FrameResult,
		Mode:  mc.Mode,
		Phase: mc.Phase,
		Content: interaction.ResultContent{
			Status: resultStatus,
			Evidence: []interaction.EvidenceItem{
				{Kind: "re_review", Detail: fmt.Sprintf("%d findings after re-review", totalFindings)},
			},
		},
		Actions: []interaction.ActionSlot{
			{ID: "done", Label: "Done", Kind: interaction.ActionConfirm, Default: true},
		},
		Metadata: interaction.FrameMetadata{
			Timestamp:  time.Now(),
			PhaseIndex: mc.PhaseIndex,
			PhaseCount: mc.PhaseCount,
		},
	}

	if err := mc.Emitter.Emit(ctx, frame); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	if _, err := mc.Emitter.AwaitResponse(ctx); err != nil {
		return interaction.PhaseOutcome{}, err
	}

	updates := map[string]any{
		"re_review.findings": findings,
		"re_review.status":   resultStatus,
	}

	return interaction.PhaseOutcome{Advance: true, StateUpdates: updates}, nil
}
