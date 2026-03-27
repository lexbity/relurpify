package rewoo

import (
	"fmt"

	"github.com/lexcodex/relurpify/framework/capability"
	"github.com/lexcodex/relurpify/framework/contextmgr"
	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/graph"
)

// BuildStaticGraph builds the base ReWOO execution graph structure.
// It creates nodes for planning, execution, aggregation, synthesis, and decision routing.
// The execute section is a placeholder that will be filled with actual step nodes once the plan is known.
// If a checkpoint store is provided, checkpoint nodes are inserted at major phase boundaries.
//
// Topology (with checkpoints):
//
//	plan_node
//	    ↓
//	checkpoint_post_plan (Phase 7)
//	    ↓
//	[step nodes inserted here dynamically]
//	    ↓
//	aggregate_node
//	    ↓
//	checkpoint_post_execute (Phase 7)
//	    ↓
//	replan_node ──→ [replan: loops to plan_node]
//	    ├→ [synthesize: proceeds]
//	    ↓
//	synthesis_node
//	    ↓
//	checkpoint_post_synthesis (Phase 7)
//	    ↓
//	done_node
func BuildStaticGraph(
	model core.LanguageModel,
	registry *capability.Registry,
	task *core.Task,
	toolSpecs []core.LLMToolSpec,
	contextPolicy *contextmgr.ContextPolicy,
	shared *core.SharedContext,
	state *core.Context,
	options RewooOptions,
	permissionManager interface{}, // *authorization.PermissionManager
	debugf func(string, ...interface{}),
) (*graph.Graph, error) {
	return buildStaticGraphWithCheckpoints(model, registry, task, toolSpecs, contextPolicy, shared, state, options, permissionManager, nil, debugf)
}

// buildStaticGraphWithCheckpoints is the internal implementation that accepts an optional checkpoint store.
func buildStaticGraphWithCheckpoints(
	model core.LanguageModel,
	registry *capability.Registry,
	task *core.Task,
	toolSpecs []core.LLMToolSpec,
	contextPolicy *contextmgr.ContextPolicy,
	shared *core.SharedContext,
	state *core.Context,
	options RewooOptions,
	permissionManager interface{}, // *authorization.PermissionManager
	checkpointStore *RewooCheckpointStore,
	debugf func(string, ...interface{}),
) (*graph.Graph, error) {
	g := graph.NewGraph()

	// Create planning node
	planNode := NewPlanNode(
		"rewoo_plan",
		model,
		task,
		toolSpecs,
		contextPolicy,
		shared,
		state,
	)
	// Phase 6: Wire graph components for dynamic step materialization
	planNode.Graph = g
	planNode.Registry = registry
	planNode.PermissionManager = permissionManager
	planNode.Options = options
	planNode.Debugf = debugf
	if err := g.AddNode(planNode); err != nil {
		return nil, fmt.Errorf("graph_builder: add plan node failed: %w", err)
	}

	// Create aggregate node (runs after all steps)
	aggregateNode := NewAggregateNode("rewoo_aggregate", nil)
	aggregateNode.Debugf = debugf
	if err := g.AddNode(aggregateNode); err != nil {
		return nil, fmt.Errorf("graph_builder: add aggregate node failed: %w", err)
	}

	// Create replan decision node
	replanNode := NewReplanNode("rewoo_replan", options.MaxReplanAttempts)
	replanNode.Debugf = debugf
	if options.GraphConfig.MaxParallelSteps > 0 {
		replanNode.ReplanThreshold = 0.25 // Replan if 25%+ fail (more lenient with parallelism)
	}
	if err := g.AddNode(replanNode); err != nil {
		return nil, fmt.Errorf("graph_builder: add replan node failed: %w", err)
	}

	// Create synthesis node
	synthesisNode := NewSynthesisNode(
		"rewoo_synthesis",
		model,
		task,
		contextPolicy,
		shared,
		state,
	)
	synthesisNode.Debugf = debugf
	if err := g.AddNode(synthesisNode); err != nil {
		return nil, fmt.Errorf("graph_builder: add synthesis node failed: %w", err)
	}

	// Create done node
	doneNode := graph.NewTerminalNode("rewoo_done")
	if err := g.AddNode(doneNode); err != nil {
		return nil, fmt.Errorf("graph_builder: add done node failed: %w", err)
	}

	// Phase 7: Create checkpoint nodes if checkpoint store is available
	var checkpointPostExecuteID string = "rewoo_aggregate"
	var checkpointPostSynthesisID string = "rewoo_done"

	if checkpointStore != nil {
		// Checkpoint after execution
		cpPostExec := NewCheckpointNode("rewoo_checkpoint_post_execute", "execute", checkpointStore)
		cpPostExec.Debugf = debugf
		if err := g.AddNode(cpPostExec); err != nil {
			return nil, fmt.Errorf("graph_builder: add checkpoint post-execute failed: %w", err)
		}
		checkpointPostExecuteID = "rewoo_checkpoint_post_execute"

		// Checkpoint after synthesis
		cpPostSynth := NewCheckpointNode("rewoo_checkpoint_post_synthesis", "synthesis", checkpointStore)
		cpPostSynth.Debugf = debugf
		if err := g.AddNode(cpPostSynth); err != nil {
			return nil, fmt.Errorf("graph_builder: add checkpoint post-synthesis failed: %w", err)
		}
		checkpointPostSynthesisID = "rewoo_checkpoint_post_synthesis"
	}

	// NOTE: We intentionally do NOT wire plan → aggregate (or plan → checkpoint)
	// here. MaterializePlanGraph (called by PlanNode during execution) handles
	// all outgoing edges from plan dynamically: plan → step nodes → aggregate
	// when there are steps, or plan → aggregate when the plan is empty.

	// Wire edges: checkpoint → replan (or aggregate → replan if no checkpoint)
	if err := g.AddEdge(checkpointPostExecuteID, "rewoo_replan", nil, false); err != nil {
		return nil, fmt.Errorf("graph_builder: add checkpoint→replan edge failed: %w", err)
	}

	// If checkpoint node exists, also wire aggregate to checkpoint
	if checkpointPostExecuteID != "rewoo_aggregate" {
		if err := g.AddEdge("rewoo_aggregate", checkpointPostExecuteID, nil, false); err != nil {
			return nil, fmt.Errorf("graph_builder: add aggregate→checkpoint edge failed: %w", err)
		}
	}

	// Wire conditional edges from replan:
	// - if "replan" → loop back to plan_node
	if err := g.AddEdge("rewoo_replan", "rewoo_plan", func(result *core.Result, _ *core.Context) bool {
		if result == nil || result.Data == nil {
			return false
		}
		next, ok := result.Data["next_node"]
		return ok && next == "plan"
	}, false); err != nil {
		return nil, fmt.Errorf("graph_builder: add replan→plan edge failed: %w", err)
	}

	// - if "synthesize" → proceed to synthesis
	if err := g.AddEdge("rewoo_replan", "rewoo_synthesis", func(result *core.Result, _ *core.Context) bool {
		if result == nil || result.Data == nil {
			return false
		}
		next, ok := result.Data["next_node"]
		return ok && next == "synthesize"
	}, false); err != nil {
		return nil, fmt.Errorf("graph_builder: add replan→synthesis edge failed: %w", err)
	}

	// Wire edges: synthesis → checkpoint → done (or synthesis → done if no checkpoint)
	if err := g.AddEdge("rewoo_synthesis", checkpointPostSynthesisID, nil, false); err != nil {
		return nil, fmt.Errorf("graph_builder: add synthesis→checkpoint edge failed: %w", err)
	}

	// If checkpoint node exists, also wire it to done
	if checkpointPostSynthesisID != "rewoo_done" {
		if err := g.AddEdge(checkpointPostSynthesisID, "rewoo_done", nil, false); err != nil {
			return nil, fmt.Errorf("graph_builder: add checkpoint→done edge failed: %w", err)
		}
	}

	// Set start node
	if err := g.SetStart("rewoo_plan"); err != nil {
		return nil, fmt.Errorf("graph_builder: set start node failed: %w", err)
	}

	return g, nil
}

// InsertStepNodes inserts nodes for executing plan steps into the graph.
// This is called after planning completes and the plan is known.
// It inserts step nodes between plan_node and aggregate_node based on dependencies.
func InsertStepNodes(
	g *graph.Graph,
	plan *RewooPlan,
	registry *capability.Registry,
	permissionManager interface{}, // *authorization.PermissionManager
	options RewooOptions,
	debugf func(string, ...interface{}),
) error {
	if plan == nil || len(plan.Steps) == 0 {
		// No steps to execute, wire plan directly to aggregate
		return g.AddEdge("rewoo_plan", "rewoo_aggregate", nil, false)
	}

	// Create a node for each step
	stepNodesByID := make(map[string]*StepNode, len(plan.Steps))
	for _, step := range plan.Steps {
		nodeID := fmt.Sprintf("rewoo_step_%s", step.ID)
		stepNode := NewStepNode(nodeID, step, registry, options.OnFailure)
		stepNode.OnPermissionDenied = StepOnFailureAbort
		if permissionManager != nil {
			stepNode.SetPermissionManager(permissionManager)
		}
		stepNode.Debugf = debugf

		if err := g.AddNode(stepNode); err != nil {
			return fmt.Errorf("insert_step_nodes: add node %s failed: %w", nodeID, err)
		}
		stepNodesByID[step.ID] = stepNode
	}

	// Wire step dependencies
	for _, step := range plan.Steps {
		nodeID := fmt.Sprintf("rewoo_step_%s", step.ID)

		if len(step.DependsOn) == 0 {
			// No dependencies: wire from plan_node
			if err := g.AddEdge("rewoo_plan", nodeID, nil, false); err != nil {
				return fmt.Errorf("insert_step_nodes: add plan→%s edge failed: %w", nodeID, err)
			}
		} else {
			// Wire from each dependency
			for _, depID := range step.DependsOn {
				depNodeID := fmt.Sprintf("rewoo_step_%s", depID)
				// TODO(phase-5): Use parallel=true for parallelizable steps
				if err := g.AddEdge(depNodeID, nodeID, nil, false); err != nil {
					return fmt.Errorf("insert_step_nodes: add %s→%s edge failed: %w", depNodeID, nodeID, err)
				}
			}
		}

		// Wire to aggregate node (all steps eventually lead here)
		if err := g.AddEdge(nodeID, "rewoo_aggregate", nil, false); err != nil {
			return fmt.Errorf("insert_step_nodes: add %s→aggregate edge failed: %w", nodeID, err)
		}
	}

	return nil
}
