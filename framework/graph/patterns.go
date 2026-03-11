package graph

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

// BuildPlanExecuteSummarizeVerifyGraph wires plan -> execute -> summarize -> verify -> done.
func BuildPlanExecuteSummarizeVerifyGraph(plan, execute, summarize, verify Node, doneID string) (*Graph, error) {
	if plan == nil || execute == nil || summarize == nil || verify == nil {
		return nil, fmt.Errorf("plan/execute/summarize/verify nodes required")
	}
	if doneID == "" {
		doneID = "plan_done"
	}
	done := NewTerminalNode(doneID)
	g := NewGraph()
	for _, node := range []Node{plan, execute, summarize, verify, done} {
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
	if err := g.AddEdge(execute.ID(), summarize.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(summarize.ID(), verify.ID(), nil, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(verify.ID(), done.ID(), nil, false); err != nil {
		return nil, err
	}
	return g, nil
}

// WrapWithCheckpointing inserts an explicit checkpoint node before the terminal node.
func WrapWithCheckpointing(g *Graph, beforeNodeID string, checkpoint *CheckpointNode, doneNodeID string) error {
	if g == nil || checkpoint == nil {
		return fmt.Errorf("graph and checkpoint node required")
	}
	if beforeNodeID == "" || doneNodeID == "" {
		return fmt.Errorf("before/done node ids required")
	}
	if err := g.AddNode(checkpoint); err != nil {
		return err
	}
	g.mu.Lock()
	edges := append([]Edge(nil), g.edges[beforeNodeID]...)
	filtered := make([]Edge, 0, len(edges))
	for _, edge := range edges {
		if edge.To == doneNodeID && !edge.Parallel {
			continue
		}
		filtered = append(filtered, edge)
	}
	g.edges[beforeNodeID] = filtered
	g.mu.Unlock()
	if err := g.AddEdge(beforeNodeID, checkpoint.ID(), nil, false); err != nil {
		return err
	}
	return g.AddEdge(checkpoint.ID(), doneNodeID, nil, false)
}

// WrapWithPeriodicSummaries inserts a summarize node before the terminal node.
func WrapWithPeriodicSummaries(g *Graph, beforeNodeID string, summarize *SummarizeContextNode, doneNodeID string) error {
	if g == nil || summarize == nil {
		return fmt.Errorf("graph and summarize node required")
	}
	if beforeNodeID == "" || doneNodeID == "" {
		return fmt.Errorf("before/done node ids required")
	}
	if err := g.AddNode(summarize); err != nil {
		return err
	}
	g.mu.Lock()
	edges := append([]Edge(nil), g.edges[beforeNodeID]...)
	filtered := make([]Edge, 0, len(edges))
	for _, edge := range edges {
		if edge.To == doneNodeID && !edge.Parallel {
			continue
		}
		filtered = append(filtered, edge)
	}
	g.edges[beforeNodeID] = filtered
	g.mu.Unlock()
	if err := g.AddEdge(beforeNodeID, summarize.ID(), nil, false); err != nil {
		return err
	}
	return g.AddEdge(summarize.ID(), doneNodeID, nil, false)
}

// WrapWithDeclarativeRetrieval inserts a declarative retrieval node ahead of the current start.
func WrapWithDeclarativeRetrieval(g *Graph, retrieve *RetrieveDeclarativeMemoryNode) error {
	if g == nil || retrieve == nil {
		return fmt.Errorf("graph and retrieve node required")
	}
	startID := g.startNodeID
	if startID == "" {
		return fmt.Errorf("graph start node required")
	}
	if err := g.AddNode(retrieve); err != nil {
		return err
	}
	if err := g.AddEdge(retrieve.ID(), startID, nil, false); err != nil {
		return err
	}
	return g.SetStart(retrieve.ID())
}

// WrapWithProceduralRetrieval inserts a procedural retrieval node ahead of the current start.
func WrapWithProceduralRetrieval(g *Graph, retrieve *RetrieveProceduralMemoryNode) error {
	if g == nil || retrieve == nil {
		return fmt.Errorf("graph and retrieve node required")
	}
	startID := g.startNodeID
	if startID == "" {
		return fmt.Errorf("graph start node required")
	}
	if err := g.AddNode(retrieve); err != nil {
		return err
	}
	if err := g.AddEdge(retrieve.ID(), startID, nil, false); err != nil {
		return err
	}
	return g.SetStart(retrieve.ID())
}
