package plan

import "time"

type InvalidationEvent struct {
	Kind   InvalidationKind
	Target string
	At     time.Time
}

func PropagateInvalidation(plan *LivingPlan, event InvalidationEvent) []string {
	if plan == nil {
		return nil
	}
	graph := NewPlanGraph(plan)
	invalidated := make([]string, 0)
	seen := make(map[string]struct{})
	queue := make([]string, 0)
	for id, step := range plan.Steps {
		if step == nil || !stepMatchesInvalidation(step, event) {
			continue
		}
		queue = append(queue, id)
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		step := plan.Steps[id]
		if step == nil {
			continue
		}
		step.Status = PlanStepInvalidated
		step.UpdatedAt = event.At
		invalidated = append(invalidated, id)
		queue = append(queue, graph.Dependents(id)...)
	}
	return invalidated
}

func stepMatchesInvalidation(step *PlanStep, event InvalidationEvent) bool {
	for _, rule := range step.InvalidatedBy {
		if rule.Kind == event.Kind && rule.Target == event.Target {
			return true
		}
	}
	return false
}
