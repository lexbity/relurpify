package thoughtrecipe

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	frameworkingestion "codeburg.org/lexbit/relurpify/context/knowledge/ingestion"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	eucloingestion "codeburg.org/lexbit/relurpify/named/euclo/ingestion"
)

// BuildThoughtRecipeGraph builds an agentgraph.Graph for a compiled execution plan.
//
// The graph preserves the linear thoughtrecipe steps and also materializes parallel and
// conditional groups using the primitives available in framework/agentgraph.
func BuildThoughtRecipeGraph(plan *ExecutionPlan, deps *paradigm.Deps, ingestionPipeline *frameworkingestion.Pipeline) (*agentgraph.Graph, error) {
	_ = ingestionPipeline // retained for future plumbing; step nodes currently own ingestion.

	if plan == nil {
		return nil, fmt.Errorf("execution plan is nil")
	}

	graph := agentgraph.NewGraph()
	sections := make([]graphSection, 0, 1+len(plan.Routes))

	if len(plan.Steps) > 0 {
		section, err := buildLinearSection(graph, deps, plan.Steps)
		if err != nil {
			return nil, err
		}
		sections = append(sections, section)
	}

	for _, group := range plan.Routes {
		section, err := buildRouteSection(graph, deps, group)
		if err != nil {
			return nil, err
		}
		sections = append(sections, section)
	}

	if len(sections) == 0 {
		return nil, fmt.Errorf("execution plan has no steps")
	}

	terminalID := "euclo.execution.done"
	if err := graph.AddNode(agentgraph.NewTerminalNode(terminalID)); err != nil {
		return nil, err
	}

	for i, section := range sections {
		nextID := terminalID
		if i+1 < len(sections) && strings.TrimSpace(sections[i+1].entry) != "" {
			nextID = sections[i+1].entry
		}
		if strings.TrimSpace(section.tail) != "" && strings.TrimSpace(nextID) != "" {
			if err := graph.AddEdge(section.tail, nextID, successCondition, false); err != nil {
				return nil, err
			}
		}
		if strings.TrimSpace(section.fallback) != "" && strings.TrimSpace(nextID) != "" {
			if err := graph.AddEdge(section.fallback, nextID, nil, false); err != nil {
				return nil, err
			}
		}
	}

	if err := graph.SetStart(sections[0].entry); err != nil {
		return nil, err
	}

	return graph, nil
}

type graphSection struct {
	entry    string
	tail     string
	fallback string
}

type stepArtifacts struct {
	entry    string
	tail     string
	fallback string
}

func buildLinearSection(graph *agentgraph.Graph, deps *paradigm.Deps, steps []ExecutionStep) (graphSection, error) {
	artifacts := make([]stepArtifacts, 0, len(steps))
	for _, step := range steps {
		artifact, err := addExecutionStep(graph, deps, step)
		if err != nil {
			return graphSection{}, err
		}
		artifacts = append(artifacts, artifact)
	}
	if len(artifacts) == 0 {
		return graphSection{}, nil
	}

	for i := 0; i < len(artifacts)-1; i++ {
		if err := wireStepTransitions(graph, artifacts[i], artifacts[i+1].entry); err != nil {
			return graphSection{}, err
		}
	}

	return graphSection{
		entry:    artifacts[0].entry,
		tail:     artifacts[len(artifacts)-1].tail,
		fallback: artifacts[len(artifacts)-1].fallback,
	}, nil
}

func buildRouteSection(graph *agentgraph.Graph, deps *paradigm.Deps, group CompiledRouteGroup) (graphSection, error) {
	routeID := scopedGroupNodeID(group.Group.ID, "route")
	joinID := scopedGroupNodeID(group.Group.ID, "join")

	if err := graph.AddNode(newThoughtRecipeStageNode(routeID, agentgraph.NodeTypeConditional, "route", map[string]any{
		"group_id": group.Group.ID,
	})); err != nil {
		return graphSection{}, err
	}
	if err := graph.AddNode(newThoughtRecipeStageNode(joinID, agentgraph.NodeTypeSystem, "join", map[string]any{
		"group_id": group.Group.ID,
	})); err != nil {
		return graphSection{}, err
	}

	hasElse := false
	for _, br := range group.Branches {
		if br.IsElse {
			hasElse = true
			break
		}
	}

	// FR-8: unmatched route without otherwise must halt.
	noMatchID := joinID
	if !hasElse {
		noMatchID = scopedGroupNodeID(group.Group.ID, "no-match")
		if err := graph.AddNode(newThoughtRecipeStageNode(noMatchID, agentgraph.NodeTypeSystem, "no_match", map[string]any{
			"group_id": group.Group.ID,
		})); err != nil {
			return graphSection{}, err
		}
		// Dead-end edge: no-match node never transitions to join.
		if err := graph.AddEdge(noMatchID, joinID, func(_ *execution.Result, _ *contextdata.Envelope) bool { return false }, false); err != nil {
			return graphSection{}, err
		}
	}

	nextEntry := noMatchID
	for i := len(group.Branches) - 1; i >= 0; i-- {
		branch := group.Branches[i]
		bodyEntry, err := buildExecutionSequence(graph, deps, branch.Steps, joinID)
		if err != nil {
			return graphSection{}, err
		}
		if branch.IsElse {
			if bodyEntry != "" {
				nextEntry = bodyEntry
			}
			continue
		}
		branchID := scopedGroupNodeID(group.Group.ID, fmt.Sprintf("branch.%d", i))
		condNode := newThoughtRecipeStageNode(branchID, agentgraph.NodeTypeConditional, "route_condition", map[string]any{
			"group_id":  group.Group.ID,
			"branch_id": branchID,
		})
		if branch.Predicate != nil {
			condNode.condFunc = compilePredicate(*branch.Predicate)
			condNode.data["predicate"] = branch.Predicate // envelope metadata
		}
		if err := graph.AddNode(condNode); err != nil {
			return graphSection{}, err
		}
		if strings.TrimSpace(nextEntry) != "" {
			if err := graph.AddEdge(branchID, nextEntry, routeConditionFalse(group.Group.ID, branchID), false); err != nil {
				return graphSection{}, err
			}
		}
		if bodyEntry != "" {
			if err := graph.AddEdge(branchID, bodyEntry, routeConditionTrue(group.Group.ID, branchID), false); err != nil {
				return graphSection{}, err
			}
		} else {
			if err := graph.AddEdge(branchID, joinID, routeConditionTrue(group.Group.ID, branchID), false); err != nil {
				return graphSection{}, err
			}
		}
		nextEntry = branchID
	}

	if err := graph.AddEdge(routeID, nextEntry, nil, false); err != nil {
		return graphSection{}, err
	}

	return graphSection{
		entry: routeID,
		tail:  noMatchID,
	}, nil
}

func buildExecutionSequence(graph *agentgraph.Graph, deps *paradigm.Deps, steps []ExecutionStep, continuation string) (string, error) {
	if len(steps) == 0 {
		if strings.TrimSpace(continuation) != "" {
			return continuation, nil
		}
		return "", nil
	}

	artifacts := make([]stepArtifacts, 0, len(steps))
	for _, step := range steps {
		artifact, err := addExecutionStep(graph, deps, step)
		if err != nil {
			return "", err
		}
		artifacts = append(artifacts, artifact)
	}
	for i := 0; i < len(artifacts)-1; i++ {
		if err := wireStepTransitions(graph, artifacts[i], artifacts[i+1].entry); err != nil {
			return "", err
		}
	}
	if strings.TrimSpace(continuation) != "" {
		if err := wireStepTransitions(graph, artifacts[len(artifacts)-1], continuation); err != nil {
			return "", err
		}
	}
	return artifacts[0].entry, nil
}

func addExecutionStep(graph *agentgraph.Graph, deps *paradigm.Deps, step ExecutionStep) (stepArtifacts, error) {
	if step.Kind == StepKindPipelineStage {
		return addPipelineStep(graph, deps, step)
	}

	entry := ""
	tail := ""

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
			return stepArtifacts{}, err
		}
		entry = nodeID
		tail = nodeID
	}

	if step.CapabilityID == "" && step.Stream != nil {
		nodeID := step.ID + ".stream"
		streamNode := agentgraph.NewContextStreamNode(nodeID, retrieval.RetrievalQuery{Text: step.Stream.QueryTemplate}, step.Stream.MaxTokens)
		streamNode.Mode = contextstream.Mode(step.Stream.Mode)
		streamNode.BudgetShortfallPolicy = "emit_partial"
		streamNode.Metadata = map[string]any{
			"execution_step_id":   step.ID,
			"execution_step_type": step.Kind.String(),
		}
		if cfg := cloneClarificationStepConfig(step.ClarificationConfig); cfg != nil {
			streamNode.Metadata["execution_clarification_type"] = step.Kind.String()
			streamNode.Metadata["execution_clarification_schema_id"] = cfg.OutputSchemaID
		}
		if err := graph.AddNode(streamNode); err != nil {
			return stepArtifacts{}, err
		}
		if entry == "" {
			entry = nodeID
		} else if err := graph.AddEdge(tail, nodeID, nil, false); err != nil {
			return stepArtifacts{}, err
		}
		tail = nodeID
	}

	if step.CapabilityID == "" && (step.Mutation == "required" || (step.HITL != "" && step.HITL != "never")) {
		nodeID := step.ID + ".gate"
		if err := graph.AddNode(newThoughtRecipeStageNode(nodeID, agentgraph.NodeTypeSystem, "gate", map[string]any{
			"step_id":            step.ID,
			"step_type":          step.Kind.String(),
			"mutation":           step.Mutation,
			"hitl":               step.HITL,
			"clarification_type": step.Kind.String(),
			"clarification_schema_id": func() string {
				if step.ClarificationConfig != nil {
					return step.ClarificationConfig.OutputSchemaID
				}
				return ""
			}(),
		})); err != nil {
			return stepArtifacts{}, err
		}
		if entry == "" {
			entry = nodeID
		} else if err := graph.AddEdge(tail, nodeID, nil, false); err != nil {
			return stepArtifacts{}, err
		}
		tail = nodeID
	}

	execNodeID := step.ID + ".execute"
	execNode := newNodeForStep(execNodeID, deps, step)
	if err := graph.AddNode(execNode); err != nil {
		return stepArtifacts{}, err
	}
	if entry == "" {
		entry = execNodeID
	} else if err := graph.AddEdge(tail, execNodeID, nil, false); err != nil {
		return stepArtifacts{}, err
	}
	tail = execNodeID

	var fallbackID string
	if step.Fallback != nil {
		fallbackID = step.ID + ".fallback"
		agent := step.Fallback
		fallbackStep := ExecutionStep{
			ID:       fallbackID,
			Kind:     step.Kind,
			Scope:    step.Scope,
			Paradigm: agent.Paradigm,
			Prompt:   agent.Prompt,
			Stream:   cloneStreamSpec(agent.Context.Stream),
			Ingest:   cloneIngestSpec(agent.Context.Ingest),
			Inherit:  append([]string(nil), agent.Context.Inherit...),
			Capture:  append([]string(nil), agent.Context.Capture...),
		}
		if err := graph.AddNode(newNodeForStep(fallbackID, deps, fallbackStep)); err != nil {
			return stepArtifacts{}, err
		}
		if err := graph.AddEdge(execNodeID, fallbackID, func(result *execution.Result, env *contextdata.Envelope) bool {
			_ = env
			return result != nil && !result.Success
		}, false); err != nil {
			return stepArtifacts{}, err
		}
	}

	return stepArtifacts{
		entry:    entry,
		tail:     tail,
		fallback: fallbackID,
	}, nil
}

func addPipelineStep(graph *agentgraph.Graph, deps *paradigm.Deps, step ExecutionStep) (stepArtifacts, error) {
	entry := step.ID + ".pipeline"
	joinID := step.ID + ".join"
	if err := graph.AddNode(newThoughtRecipeStageNode(entry, agentgraph.NodeTypeSystem, "pipeline", map[string]any{
		"step_id":     step.ID,
		"pipeline_id": step.ID,
		"stage_count": len(step.PipelineStages),
		"stage_names": summarizePipelineStageSpecs(step.PipelineStages),
	})); err != nil {
		return stepArtifacts{}, err
	}
	if err := graph.AddNode(newThoughtRecipeStageNode(joinID, agentgraph.NodeTypeSystem, "join", map[string]any{
		"step_id":     step.ID,
		"pipeline_id": step.ID,
	})); err != nil {
		return stepArtifacts{}, err
	}

	nextEntry := joinID
	for i := len(step.PipelineStages) - 1; i >= 0; i-- {
		stage := step.PipelineStages[i]
		stageName := strings.TrimSpace(stage.Name)
		stageID := step.ID + ".stage." + fmt.Sprint(i)
		if stageName != "" {
			stageID += "." + sanitizeComponent(stageName)
		}
		if err := graph.AddNode(newThoughtRecipeStageNode(stageID, agentgraph.NodeTypeSystem, "pipeline_stage", map[string]any{
			"step_id":     step.ID,
			"pipeline_id": step.ID,
			"stage_name":  stageName,
			"stage_index": i,
		})); err != nil {
			return stepArtifacts{}, err
		}
		bodyEntry, err := buildExecutionSequence(graph, deps, stage.Steps, nextEntry)
		if err != nil {
			return stepArtifacts{}, err
		}
		target := nextEntry
		if strings.TrimSpace(bodyEntry) != "" {
			target = bodyEntry
		}
		if err := graph.AddEdge(stageID, target, nil, false); err != nil {
			return stepArtifacts{}, err
		}
		nextEntry = stageID
	}

	if err := graph.AddEdge(entry, nextEntry, nil, false); err != nil {
		return stepArtifacts{}, err
	}

	return stepArtifacts{
		entry: entry,
		tail:  joinID,
	}, nil
}

func wireStepTransitions(graph *agentgraph.Graph, artifact stepArtifacts, next string) error {
	if strings.TrimSpace(artifact.tail) != "" && strings.TrimSpace(next) != "" {
		if err := graph.AddEdge(artifact.tail, next, successCondition, false); err != nil {
			return err
		}
	}
	if strings.TrimSpace(artifact.fallback) != "" && strings.TrimSpace(next) != "" {
		if err := graph.AddEdge(artifact.fallback, next, nil, false); err != nil {
			return err
		}
	}
	return nil
}

func successCondition(result *execution.Result, env *contextdata.Envelope) bool {
	_ = env
	return result == nil || result.Success
}

func scopedGroupNodeID(groupID, kind string) string {
	base := sanitizeComponent(groupID)
	if strings.TrimSpace(base) == "" {
		base = "group"
	}
	kind = sanitizeComponent(kind)
	if strings.TrimSpace(kind) == "" {
		kind = "node"
	}
	return "euclo.execution.group." + base + "." + kind
}

func envBool(env *contextdata.Envelope, key string) bool {
	value, ok := contextdata.GetTyped[any](env, key)
	if !ok {
		return false
	}
	return truthy(value)
}

func truthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "0", "false", "no", "off", "nil", "null":
			return false
		default:
			return true
		}
	case fmt.Stringer:
		return truthy(v.String())
	case int, int8, int16, int32, int64:
		return fmt.Sprint(v) != "0"
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(v) != "0"
	case float32, float64:
		return fmt.Sprint(v) != "0" && fmt.Sprint(v) != "0.0"
	default:
		return value != nil
	}
}

type thoughtrecipeStageNode struct {
	id       string
	kind     string
	op       string
	data     map[string]any
	condFunc agentgraph.ConditionFunc // for route_condition nodes (compiled predicate)
}

func newThoughtRecipeStageNode(id string, nodeType agentgraph.NodeType, op string, data map[string]any) *thoughtrecipeStageNode {
	return &thoughtrecipeStageNode{id: id, kind: string(nodeType), op: op, data: data}
}

func (n *thoughtrecipeStageNode) ID() string                { return n.id }
func (n *thoughtrecipeStageNode) Type() agentgraph.NodeType { return agentgraph.NodeType(n.kind) }
func (n *thoughtrecipeStageNode) Execute(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	_ = ctx
	if env != nil && n.data != nil {
		// envelope: intentional dynamic key — stage metadata is keyed by stage ID and field name.
		for key, value := range n.data {
			contextdata.SetTyped(env, "euclo.execution.stage."+n.id+"."+key, value)
		}
		switch n.op {
		case "gate":
			mutationPermitted := true
			if mutation, _ := n.data["mutation"].(string); strings.TrimSpace(mutation) == "required" {
				mutationPermitted = false
			}
			hitlRequired := false
			if hitl, _ := n.data["hitl"].(string); strings.TrimSpace(hitl) != "" && strings.TrimSpace(hitl) != "never" {
				hitlRequired = true
			}
			contextdata.SetTyped(env, "euclo.policy.mutation_permitted", mutationPermitted)
			contextdata.SetTyped(env, "euclo.policy.hitl_required", hitlRequired)
			contextdata.SetTyped(env, "euclo.policy.verification_required", !mutationPermitted || hitlRequired)
		case "route":
			// envelope: intentional dynamic key — route state is scoped by group ID.
			contextdata.SetTyped(env, "euclo.execution.route."+sanitizeComponent(fmt.Sprint(n.data["group_id"])), true)
		case "no_match":
			return &execution.Result{
				NodeID:  n.id,
				Success: false,
				Error:   "no-route: no matching branch and no otherwise",
				Data:    execution.NewToolResultPayload(n.data),
			}, nil
		case "route_condition":
			matched := n.condFunc(nil, env)
			branchID := fmt.Sprint(n.data["branch_id"])
			// envelope: intentional dynamic key — route condition results are scoped by group and branch.
			contextdata.SetTyped(env, routeConditionResultKey(fmt.Sprint(n.data["group_id"]), branchID), matched)
			n.data["condition_met"] = matched
		}
	}
	return &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data:    execution.NewToolResultPayload(n.data),
	}, nil
}

func routeConditionResultKey(groupID, branchID string) string {
	return "euclo.execution.group." + sanitizeComponent(groupID) + ".branch." + sanitizeComponent(branchID) + ".condition_met"
}

func routeConditionTrue(groupID, branchID string) agentgraph.ConditionFunc {
	key := routeConditionResultKey(groupID, branchID)
	return func(result *execution.Result, env *contextdata.Envelope) bool {
		_ = result
		return envBool(env, key)
	}
}

func routeConditionFalse(groupID, branchID string) agentgraph.ConditionFunc {
	key := routeConditionResultKey(groupID, branchID)
	return func(result *execution.Result, env *contextdata.Envelope) bool {
		_ = result
		return !envBool(env, key)
	}
}

