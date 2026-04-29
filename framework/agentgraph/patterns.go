package agentgraph

import "fmt"

// BuildPlanExecuteVerifyGraph wires plan -> execute -> verify -> done.
func BuildPlanExecuteVerifyGraph(plan, execute, verify Node, doneID string) (*Graph, error) {
	if plan == nil || execute == nil || verify == nil {
		return nil, fmt.Errorf("plan/execute/verify nodes required")
	}
	if doneID == "" {
		doneID = "plan_done"
	}
	done := NewTerminalNode(doneID)
	g := NewGraph()
	for _, node := range []Node{plan, execute, verify, done} {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := g.SetStart(plan.ID()); err != nil {
		return nil, err
	}
	if err := g.AddEdge(plan.ID(), execute.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(execute.ID(), verify.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(verify.ID(), done.ID(), nil, false); err != nil {
		return nil, err
	}
	return g, nil
}

// BuildThinkActObserveGraph wires think -> act -> observe with loop conditions.
func BuildThinkActObserveGraph(think, act, observe Node, continueCond, doneCond ConditionFunc, doneID string) (*Graph, error) {
	if think == nil || act == nil || observe == nil {
		return nil, fmt.Errorf("think/act/observe nodes required")
	}
	if doneID == "" {
		doneID = "react_done"
	}
	done := NewTerminalNode(doneID)
	g := NewGraph()
	for _, node := range []Node{think, act, observe, done} {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := g.SetStart(think.ID()); err != nil {
		return nil, err
	}
	if err := g.AddEdge(think.ID(), act.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(act.ID(), observe.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(observe.ID(), think.ID(), continueCond, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(observe.ID(), done.ID(), doneCond, false); err != nil {
		return nil, err
	}
	return g, nil
}

// BuildReviewIterateGraph wires execute -> review with loop conditions.
func BuildReviewIterateGraph(execute, review Node, continueCond, doneCond ConditionFunc, doneID string) (*Graph, error) {
	if execute == nil || review == nil {
		return nil, fmt.Errorf("execute/review nodes required")
	}
	if doneID == "" {
		doneID = "review_done"
	}
	done := NewTerminalNode(doneID)
	g := NewGraph()
	for _, node := range []Node{execute, review, done} {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := g.SetStart(execute.ID()); err != nil {
		return nil, err
	}
	if err := g.AddEdge(execute.ID(), review.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(review.ID(), execute.ID(), continueCond, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(review.ID(), done.ID(), doneCond, false); err != nil {
		return nil, err
	}
	return g, nil
}
