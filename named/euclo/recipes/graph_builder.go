package recipe

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	frameworkingestion "codeburg.org/lexbit/relurpify/framework/ingestion"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	eucloingestion "codeburg.org/lexbit/relurpify/named/euclo/ingestion"
)

// BuildRecipeGraph builds an agentgraph.Graph for a spec-shaped execution plan.
//
// The current implementation keeps the topology explicit and deterministic while
// remaining lightweight enough for tests: each step expands into a small chain
// of ingest -> stream -> gate -> step -> fallback nodes.
func BuildRecipeGraph(plan *ExecutionPlan, env agentenv.WorkspaceEnvironment, trigger *contextstream.Trigger, ingestionPipeline *frameworkingestion.Pipeline) (*agentgraph.Graph, error) {
	_ = ingestionPipeline

	if plan == nil {
		return nil, fmt.Errorf("execution plan is nil")
	}
	if len(plan.Steps) == 0 {
		return nil, fmt.Errorf("execution plan has no steps")
	}

	graph := agentgraph.NewGraph()
	steps := make([]stepTopology, 0, len(plan.Steps))

	for i := range plan.Steps {
		step := plan.Steps[i]
		topology := stepTopology{}

		if step.Ingest != nil {
			nodeID := step.ID + ".ingest"
			node := eucloingestion.NewIngestionNode(nodeID, eucloingestion.IngestionSpec{
				Mode:          eucloingestion.IngestionMode(step.Ingest.Mode),
				ExplicitFiles: append([]string(nil), step.Ingest.IncludeGlobs...),
				WorkspaceRoot: step.Ingest.WorkspaceRoot,
				IncludeGlobs:  append([]string(nil), step.Ingest.IncludeGlobs...),
				ExcludeGlobs:  append([]string(nil), step.Ingest.ExcludeGlobs...),
				SinceRef:      "",
			})
			if err := graph.AddNode(node); err != nil {
				return nil, err
			}
			topology.first = nodeID
			topology.last = nodeID
		}

		if step.Stream != nil {
			nodeID := step.ID + ".stream"
			streamData := map[string]any{
				"query_template": step.Stream.QueryTemplate,
				"max_tokens":     step.Stream.MaxTokens,
				"mode":           step.Stream.Mode,
			}
			var streamNode agentgraph.Node
			if trigger != nil {
				query := step.Stream.QueryTemplate
				streamNode = agentgraph.NewContextStreamNode(nodeID, trigger, retrieval.RetrievalQuery{Text: query}, step.Stream.MaxTokens)
				if typed, ok := streamNode.(*agentgraph.StreamTriggerNode); ok {
					typed.Mode = contextstream.Mode(step.Stream.Mode)
					typed.BudgetShortfallPolicy = "emit_partial"
					typed.Metadata = map[string]any{"recipe_step_id": step.ID}
				}
			} else {
				streamNode = newRecipeStageNode(nodeID, agentgraph.NodeTypeStream, "stream", streamData)
			}
			if err := graph.AddNode(streamNode); err != nil {
				return nil, err
			}
			if topology.first == "" {
				topology.first = nodeID
			} else if err := graph.AddEdge(topology.last, nodeID, nil, false); err != nil {
				return nil, err
			}
			topology.last = nodeID
		}

		if step.Mutation == "required" || step.HITL != "" && step.HITL != "never" {
			nodeID := step.ID + ".gate"
			if err := graph.AddNode(newRecipeStageNode(nodeID, agentgraph.NodeTypeSystem, "gate", map[string]any{
				"mutation": step.Mutation,
				"hitl":     step.HITL,
			})); err != nil {
				return nil, err
			}
			if topology.first == "" {
				topology.first = nodeID
			} else if err := graph.AddEdge(topology.last, nodeID, nil, false); err != nil {
				return nil, err
			}
			topology.last = nodeID
		}

		execNodeID := step.ID + ".execute"
		execNode := NewRecipeStepNode(execNodeID, env, step, trigger)
		if err := graph.AddNode(execNode); err != nil {
			return nil, err
		}
		if topology.first == "" {
			topology.first = execNodeID
		} else if err := graph.AddEdge(topology.last, execNodeID, nil, false); err != nil {
			return nil, err
		}
		topology.last = execNodeID

		if step.Fallback != nil {
			fallbackID := step.ID + ".fallback"
			fallbackStep := executionStepFromAgent(fallbackID, step.Fallback)
			if err := graph.AddNode(NewRecipeStepNode(fallbackID, env, fallbackStep, trigger)); err != nil {
				return nil, err
			}
			if err := graph.AddEdge(execNodeID, fallbackID, func(result *agentgraph.Result, env *contextdata.Envelope) bool {
				_ = env
				return result != nil && !result.Success
			}, false); err != nil {
				return nil, err
			}
			topology.fallback = fallbackID
		}

		steps = append(steps, topology)
	}

	terminalID := "euclo.recipe.done"
	merge := agentgraph.NewTerminalNode(terminalID)
	if err := graph.AddNode(merge); err != nil {
		return nil, err
	}

	if err := graph.SetStart(steps[0].first); err != nil {
		return nil, err
	}

	for i, topology := range steps {
		nextFirst := terminalID
		if i+1 < len(steps) && steps[i+1].first != "" {
			nextFirst = steps[i+1].first
		}
		if topology.last != "" {
			if err := graph.AddEdge(topology.last, nextFirst, func(result *agentgraph.Result, env *contextdata.Envelope) bool {
				_ = env
				return result == nil || result.Success
			}, false); err != nil {
				return nil, err
			}
		}
		if topology.fallback != "" {
			if err := graph.AddEdge(topology.fallback, nextFirst, nil, false); err != nil {
				return nil, err
			}
		}
	}

	return graph, nil
}

type stepTopology struct {
	first    string
	last     string
	fallback string
}

func executionStepFromAgent(id string, agent *RecipeStepAgent) ExecutionStep {
	if agent == nil {
		return ExecutionStep{ID: id}
	}
	step := RecipeStep{
		ID:      id,
		Parent:  *agent,
		Context: agent.Context,
	}
	return ExecutionStep{
		ID:       id,
		Paradigm: agent.Paradigm,
		Prompt:   agent.Prompt,
		Stream:   cloneStreamSpec(agent.Context.Stream),
		Ingest:   cloneIngestSpec(agent.Context.Ingest),
		Inherit:  append([]string(nil), agent.Context.Inherit...),
		Capture:  append([]string(nil), agent.Context.Capture...),
		Step:     step,
	}
}

type recipeStageNode struct {
	id   string
	kind string
	op   string
	data map[string]any
}

func newRecipeStageNode(id string, nodeType agentgraph.NodeType, op string, data map[string]any) *recipeStageNode {
	return &recipeStageNode{id: id, kind: string(nodeType), op: op, data: data}
}

func (n *recipeStageNode) ID() string                { return n.id }
func (n *recipeStageNode) Type() agentgraph.NodeType { return agentgraph.NodeType(n.kind) }
func (n *recipeStageNode) Execute(ctx context.Context, env *contextdata.Envelope) (*agentgraph.Result, error) {
	_ = ctx
	if env != nil && n.data != nil {
		for key, value := range n.data {
			env.SetWorkingValue("euclo.recipe.stage."+n.id+"."+key, value, contextdata.MemoryClassTask)
		}
		if n.op == "gate" {
			mutationPermitted := true
			if mutation, _ := n.data["mutation"].(string); strings.TrimSpace(mutation) == "required" {
				mutationPermitted = false
			}
			hitlRequired := false
			if hitl, _ := n.data["hitl"].(string); strings.TrimSpace(hitl) != "" && strings.TrimSpace(hitl) != "never" {
				hitlRequired = true
			}
			env.SetWorkingValue("euclo.policy.mutation_permitted", mutationPermitted, contextdata.MemoryClassTask)
			env.SetWorkingValue("euclo.policy.hitl_required", hitlRequired, contextdata.MemoryClassTask)
			env.SetWorkingValue("euclo.policy.verification_required", mutationPermitted == false || hitlRequired, contextdata.MemoryClassTask)
		}
	}
	return &agentgraph.Result{
		NodeID:  n.id,
		Success: true,
		Data:    n.data,
	}, nil
}
