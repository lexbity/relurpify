package archaeology

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/named/euclo/euclotypes"
	"github.com/lexcodex/relurpify/named/euclo/execution"
	euclorelurpic "github.com/lexcodex/relurpify/named/euclo/relurpicabilities"
)

type patternSurfaceRoutine struct{}
type prospectiveAssessRoutine struct{}
type convergenceGuardRoutine struct{}
type coherenceAssessRoutine struct{}
type scopeExpandRoutine struct{}

func NewSupportingRoutines() []euclorelurpic.SupportingRoutine {
	return []euclorelurpic.SupportingRoutine{
		patternSurfaceRoutine{},
		prospectiveAssessRoutine{},
		convergenceGuardRoutine{},
		coherenceAssessRoutine{},
		scopeExpandRoutine{},
	}
}

func (patternSurfaceRoutine) ID() string { return PatternSurface }

func (patternSurfaceRoutine) Execute(ctx context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	patternRefs := append([]string(nil), in.Work.PatternRefs...)
	tensions := []any{}
	if bundle, ok := archaeologyBundle(in); ok && bundle.Archaeo != nil && strings.TrimSpace(workflowIDFromTask(in.Task)) != "" {
		if records, err := bundle.Archaeo.TensionsByWorkflow(ctx, workflowIDFromTask(in.Task)); err == nil {
			tensions = make([]any, 0, len(records))
			for _, record := range records {
				tensions = append(tensions, map[string]any{
					"id":          record.ID,
					"kind":        record.Kind,
					"description": record.Description,
					"severity":    record.Severity,
					"status":      record.Status,
					"symbols":     append([]string(nil), record.SymbolScope...),
				})
				patternRefs = append(patternRefs, record.PatternIDs...)
			}
		}
	}
	payload := map[string]any{
		"pattern_refs": execution.UniqueStrings(patternRefs),
		"tensions":     tensions,
		"summary":      "pattern-surface routine grounded archaeology exploration in surfaced patterns and tensions",
	}
	return []euclotypes.Artifact{{
		ID:         "archaeology_pattern_surface",
		Kind:       euclotypes.ArtifactKindExplore,
		Summary:    "pattern-surface routine grounded archaeology exploration in surfaced codebase patterns",
		Payload:    payload,
		ProducerID: PatternSurface,
		Status:     "produced",
	}}, nil
}

func (prospectiveAssessRoutine) ID() string { return ProspectiveAssess }

func (prospectiveAssessRoutine) Execute(ctx context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	requestSummary := map[string]any{}
	if bundle, ok := archaeologyBundle(in); ok && bundle.Archaeo != nil && strings.TrimSpace(workflowIDFromTask(in.Task)) != "" {
		if history, err := bundle.Archaeo.RequestHistory(ctx, workflowIDFromTask(in.Task)); err == nil && history != nil {
			requestSummary = map[string]any{
				"pending":   history.Pending,
				"running":   history.Running,
				"completed": history.Completed,
				"failed":    history.Failed,
			}
		}
	}
	payload := map[string]any{
		"prospective_refs": append([]string(nil), in.Work.ProspectiveRefs...),
		"pattern_refs":     append([]string(nil), in.Work.PatternRefs...),
		"request_history":  requestSummary,
		"operation":        ProspectiveAssess,
		"summary":          "prospective-assess routine shaped candidate engineering directions",
	}
	return []euclotypes.Artifact{{
		ID:         "archaeology_prospective_assess",
		Kind:       euclotypes.ArtifactKindPlanCandidates,
		Summary:    "prospective-assess routine shaped candidate engineering directions",
		Payload:    payload,
		ProducerID: ProspectiveAssess,
		Status:     "produced",
	}}, nil
}

func (convergenceGuardRoutine) ID() string { return ConvergenceGuard }

func (convergenceGuardRoutine) Execute(ctx context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	learning := map[string]any{}
	if bundle, ok := archaeologyBundle(in); ok && bundle.Archaeo != nil && strings.TrimSpace(workflowIDFromTask(in.Task)) != "" {
		if queue, err := bundle.Archaeo.LearningQueue(ctx, workflowIDFromTask(in.Task)); err == nil && queue != nil {
			learning = map[string]any{
				"pending_ids": append([]string(nil), queue.PendingGuidanceIDs...),
				"blocking":    append([]string(nil), queue.BlockingLearning...),
				"count":       len(queue.PendingLearning),
			}
		}
	}
	payload := map[string]any{
		"convergence_refs": append([]string(nil), in.Work.ConvergenceRefs...),
		"learning_queue":   learning,
		"operation":        ConvergenceGuard,
		"summary":          "convergence-guard routine checked candidate plans for unresolved divergence",
	}
	return []euclotypes.Artifact{{
		ID:         "archaeology_convergence_guard",
		Kind:       euclotypes.ArtifactKindPlanCandidates,
		Summary:    "convergence-guard routine checked candidate plans for unresolved divergence",
		Payload:    payload,
		ProducerID: ConvergenceGuard,
		Status:     "produced",
	}}, nil
}

func (coherenceAssessRoutine) ID() string { return CoherenceAssess }

func (coherenceAssessRoutine) Execute(ctx context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	summaryPayload := map[string]any{}
	if bundle, ok := archaeologyBundle(in); ok && bundle.Archaeo != nil && strings.TrimSpace(workflowIDFromTask(in.Task)) != "" {
		if summary, err := bundle.Archaeo.TensionSummaryByWorkflow(ctx, workflowIDFromTask(in.Task)); err == nil && summary != nil {
			summaryPayload = map[string]any{
				"total":      summary.Total,
				"active":     summary.Active,
				"accepted":   summary.Accepted,
				"resolved":   summary.Resolved,
				"unresolved": summary.Unresolved,
			}
		}
	}
	payload := map[string]any{
		"pattern_refs":    append([]string(nil), in.Work.PatternRefs...),
		"tension_refs":    append([]string(nil), in.Work.TensionRefs...),
		"tension_summary": summaryPayload,
		"operation":       CoherenceAssess,
		"summary":         "coherence-assess routine checked whether discovered structures fit together coherently",
	}
	return []euclotypes.Artifact{{
		ID:         "archaeology_coherence_assess",
		Kind:       euclotypes.ArtifactKindAnalyze,
		Summary:    "coherence-assess routine checked whether discovered structures fit together coherently",
		Payload:    payload,
		ProducerID: CoherenceAssess,
		Status:     "produced",
	}}, nil
}

func (scopeExpandRoutine) ID() string { return ScopeExpansionAssess }

func (scopeExpandRoutine) Execute(ctx context.Context, in euclorelurpic.RoutineInput) ([]euclotypes.Artifact, error) {
	activePlan := map[string]any{}
	if bundle, ok := archaeologyBundle(in); ok && bundle.Archaeo != nil && strings.TrimSpace(workflowIDFromTask(in.Task)) != "" {
		if plan, err := bundle.Archaeo.ActivePlan(ctx, workflowIDFromTask(in.Task)); err == nil && plan != nil && plan.ActivePlan != nil {
			activePlan = map[string]any{
				"plan_id":      plan.ActivePlan.PlanID,
				"version":      plan.ActivePlan.Version,
				"active_step":  plan.ActiveStepID,
				"pattern_refs": append([]string(nil), plan.ActivePlan.PatternRefs...),
				"tension_refs": append([]string(nil), plan.ActivePlan.TensionRefs...),
				"step_count":   len(plan.ActivePlan.Plan.StepOrder),
			}
		}
	}
	payload := map[string]any{
		"pattern_refs": append([]string(nil), in.Work.PatternRefs...),
		"active_plan":  activePlan,
		"operation":    ScopeExpansionAssess,
		"summary":      "scope-expansion routine identified adjacent system areas implicated by the current exploration",
	}
	return []euclotypes.Artifact{{
		ID:         "archaeology_scope_expansion",
		Kind:       euclotypes.ArtifactKindContextExpansion,
		Summary:    "scope-expansion routine identified adjacent system areas implicated by the current exploration",
		Payload:    payload,
		ProducerID: ScopeExpansionAssess,
		Status:     "produced",
	}}, nil
}

func workflowIDFromTask(task *core.Task) string {
	if task == nil || task.Context == nil {
		return ""
	}
	if workflowID, ok := task.Context["workflow_id"]; ok {
		return strings.TrimSpace(fmt.Sprint(workflowID))
	}
	return ""
}

func archaeologyBundle(in euclorelurpic.RoutineInput) (execution.ServiceBundle, bool) {
	bundle, ok := in.ServiceBundle.(execution.ServiceBundle)
	return bundle, ok
}
