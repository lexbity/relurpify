package plans

import (
	"context"

	frameworkplan "codeburg.org/lexbit/relurpify/framework/plan"
)

func (s Service) CompareVersions(ctx context.Context, workflowID string, fromVersion, toVersion int) (map[string]any, error) {
	from, err := s.LoadVersion(ctx, workflowID, fromVersion)
	if err != nil {
		return nil, err
	}
	to, err := s.LoadVersion(ctx, workflowID, toVersion)
	if err != nil {
		return nil, err
	}
	if from == nil || to == nil {
		return nil, nil
	}
	return map[string]any{
		"from_version":       from.Version,
		"to_version":         to.Version,
		"from_plan_id":       from.Plan.ID,
		"to_plan_id":         to.Plan.ID,
		"same_plan_id":       from.Plan.ID == to.Plan.ID,
		"from_status":        from.Status,
		"to_status":          to.Status,
		"from_revision":      from.BasedOnRevision,
		"to_revision":        to.BasedOnRevision,
		"same_revision":      from.BasedOnRevision == to.BasedOnRevision,
		"from_snapshot_ref":  from.SemanticSnapshotRef,
		"to_snapshot_ref":    to.SemanticSnapshotRef,
		"step_count_delta":   len(to.Plan.StepOrder) - len(from.Plan.StepOrder),
		"step_ids_added":     diffStrings(to.Plan.StepOrder, from.Plan.StepOrder),
		"step_ids_removed":   diffStrings(from.Plan.StepOrder, to.Plan.StepOrder),
		"step_ids_changed":   changedStepIDs(from.Plan, to.Plan),
		"step_diffs":         stepDiffs(from.Plan, to.Plan),
		"pattern_refs_added": diffStrings(to.PatternRefs, from.PatternRefs),
		"anchor_refs_added":  diffStrings(to.AnchorRefs, from.AnchorRefs),
		"tension_refs_added": diffStrings(to.TensionRefs, from.TensionRefs),
		"comment_refs_added": diffStrings(to.CommentRefs, from.CommentRefs),
	}, nil
}

func diffStrings(values, baseline []string) []string {
	seen := make(map[string]struct{}, len(baseline))
	for _, value := range baseline {
		seen[value] = struct{}{}
	}
	var out []string
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		out = append(out, value)
	}
	return out
}

func changedStepIDs(from, to frameworkplan.LivingPlan) []string {
	ids := make([]string, 0)
	for stepID, fromStep := range from.Steps {
		toStep, ok := to.Steps[stepID]
		if !ok || fromStep == nil || toStep == nil {
			continue
		}
		if !stepsEqual(fromStep, toStep) {
			ids = append(ids, stepID)
		}
	}
	return ids
}

func stepDiffs(from, to frameworkplan.LivingPlan) map[string]any {
	out := make(map[string]any)
	for _, stepID := range diffStrings(to.StepOrder, from.StepOrder) {
		if step := to.Steps[stepID]; step != nil {
			out[stepID] = map[string]any{"change": "added", "to": stepSummary(step)}
		}
	}
	for _, stepID := range diffStrings(from.StepOrder, to.StepOrder) {
		if step := from.Steps[stepID]; step != nil {
			out[stepID] = map[string]any{"change": "removed", "from": stepSummary(step)}
		}
	}
	for stepID, fromStep := range from.Steps {
		toStep, ok := to.Steps[stepID]
		if !ok || fromStep == nil || toStep == nil || stepsEqual(fromStep, toStep) {
			continue
		}
		out[stepID] = map[string]any{
			"change": "modified",
			"from":   stepSummary(fromStep),
			"to":     stepSummary(toStep),
		}
	}
	return out
}

func stepsEqual(a, b *frameworkplan.PlanStep) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Description != b.Description || a.Status != b.Status || a.ConfidenceScore != b.ConfidenceScore {
		return false
	}
	if !stringSlicesEqual(a.Scope, b.Scope) || !stringSlicesEqual(a.DependsOn, b.DependsOn) || !stringSlicesEqual(a.AnchorDependencies, b.AnchorDependencies) {
		return false
	}
	if (a.EvidenceGate == nil) != (b.EvidenceGate == nil) {
		return false
	}
	if a.EvidenceGate != nil && b.EvidenceGate != nil {
		if a.EvidenceGate.MaxTotalLoss != b.EvidenceGate.MaxTotalLoss ||
			!stringSlicesEqual(a.EvidenceGate.RequiredAnchors, b.EvidenceGate.RequiredAnchors) ||
			!stringSlicesEqual(a.EvidenceGate.RequiredSymbols, b.EvidenceGate.RequiredSymbols) {
			return false
		}
	}
	return true
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stepSummary(step *frameworkplan.PlanStep) map[string]any {
	if step == nil {
		return nil
	}
	return map[string]any{
		"id":                  step.ID,
		"description":         step.Description,
		"status":              step.Status,
		"confidence_score":    step.ConfidenceScore,
		"scope":               append([]string(nil), step.Scope...),
		"depends_on":          append([]string(nil), step.DependsOn...),
		"anchor_dependencies": append([]string(nil), step.AnchorDependencies...),
	}
}
