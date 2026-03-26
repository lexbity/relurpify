package plan

import "fmt"

type PlanGraph struct {
	plan *LivingPlan
}

func NewPlanGraph(plan *LivingPlan) *PlanGraph {
	return &PlanGraph{plan: plan}
}

func (g *PlanGraph) TopologicalOrder() ([]string, error) {
	if g == nil || g.plan == nil {
		return nil, nil
	}
	inDegree := make(map[string]int, len(g.plan.Steps))
	dependents := make(map[string][]string, len(g.plan.Steps))
	for id := range g.plan.Steps {
		inDegree[id] = 0
	}
	for id, step := range g.plan.Steps {
		if step == nil {
			continue
		}
		for _, dep := range step.DependsOn {
			if _, ok := g.plan.Steps[dep]; !ok {
				continue
			}
			inDegree[id]++
			dependents[dep] = append(dependents[dep], id)
		}
	}
	queue := make([]string, 0, len(inDegree))
	for _, id := range g.seedOrder() {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	order := make([]string, 0, len(g.plan.Steps))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)
		for _, dep := range dependents[id] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}
	if len(order) != len(g.plan.Steps) {
		return nil, fmt.Errorf("plan graph contains a cycle")
	}
	return order, nil
}

func (g *PlanGraph) ReadySteps() []*PlanStep {
	if g == nil || g.plan == nil {
		return nil
	}
	out := make([]*PlanStep, 0)
	for _, id := range g.seedOrder() {
		step := g.plan.Steps[id]
		if step == nil || step.Status != PlanStepPending {
			continue
		}
		ready := true
		for _, dep := range step.DependsOn {
			depStep := g.plan.Steps[dep]
			if depStep == nil || depStep.Status != PlanStepCompleted {
				ready = false
				break
			}
		}
		if ready {
			out = append(out, step)
		}
	}
	return out
}

func (g *PlanGraph) Dependents(stepID string) []string {
	if g == nil || g.plan == nil {
		return nil
	}
	direct := make(map[string][]string, len(g.plan.Steps))
	for id, step := range g.plan.Steps {
		if step == nil {
			continue
		}
		for _, dep := range step.DependsOn {
			direct[dep] = append(direct[dep], id)
		}
	}
	seen := make(map[string]struct{})
	queue := append([]string(nil), direct[stepID]...)
	out := make([]string, 0)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		queue = append(queue, direct[id]...)
	}
	return out
}

func (g *PlanGraph) seedOrder() []string {
	if g == nil || g.plan == nil {
		return nil
	}
	if len(g.plan.StepOrder) > 0 {
		return append([]string(nil), g.plan.StepOrder...)
	}
	out := make([]string, 0, len(g.plan.Steps))
	for id := range g.plan.Steps {
		out = append(out, id)
	}
	return out
}
