package react

import (
	"context"
	"fmt"

	"codeburg.org/lexbit/relurpify/context/contextdata"
	execution "codeburg.org/lexbit/relurpify/execution"
	graph "codeburg.org/lexbit/relurpify/execution/agentgraph"
)

// BuildGraph constructs the ReAct workflow.
func (a *ReActAgent) BuildGraph(ctx context.Context, task *execution.Task) (*graph.Graph, error) {
	if a.Model == nil {
		return nil, fmt.Errorf("react agent missing language model")
	}
	think := &reactThinkNode{
		id:    "react_think",
		agent: a,
		task:  task,
	}
	act := &reactActNode{
		id:    "react_act",
		agent: a,
		task:  task,
	}
	stream := a.streamTriggerNode(task)
	observe := &reactObserveNode{
		id:    "react_observe",
		agent: a,
		task:  task,
	}
	done := graph.NewTerminalNode("react_done")
	g := graph.NewGraph()
	if catalog := a.executionCapabilityCatalog(ctx); catalog != nil && len(catalog.InspectableCapabilities()) > 0 {
		g.SetCapabilityCatalog(catalog)
	}
	if err := g.AddNode(think); err != nil {
		return nil, err
	}
	if err := g.SetStart(think.ID()); err != nil {
		return nil, err
	}
	for _, node := range []graph.Node{act, observe, done} {
		if err := g.AddNode(node); err != nil {
			return nil, err
		}
	}
	if stream != nil {
		if err := g.AddNode(stream); err != nil {
			return nil, err
		}
	}
	if err := g.AddEdge(think.ID(), act.ID(), nil, false); err != nil {
		return nil, err
	}
	// Connect act -> stream -> observe if streaming is enabled, otherwise act -> observe directly
	if stream != nil {
		if err := g.AddEdge(act.ID(), stream.ID(), nil, false); err != nil {
			return nil, err
		}
		if err := g.AddEdge(stream.ID(), observe.ID(), nil, false); err != nil {
			return nil, err
		}
	} else {
		if err := g.AddEdge(act.ID(), observe.ID(), nil, false); err != nil {
			return nil, err
		}
	}
	if err := g.AddEdge(observe.ID(), think.ID(), func(result *execution.Result, env *contextdata.Envelope) bool {
		done, _ := contextdata.GetTyped[any](env, "react.done")
		return done == false || done == nil
	}, false); err != nil {
		return nil, err
	}
	if err := g.AddEdge(observe.ID(), done.ID(), func(result *execution.Result, env *contextdata.Envelope) bool {
		done, _ := contextdata.GetTyped[any](env, "react.done")
		return done == true
	}, false); err != nil {
		return nil, err
	}
	return g, nil
}
