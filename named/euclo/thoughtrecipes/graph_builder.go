package thoughtrecipe

import (
	"context"
	"fmt"
	"strings"

	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	frameworkingestion "codeburg.org/lexbit/relurpify/framework/ingestion"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	eucloingestion "codeburg.org/lexbit/relurpify/named/euclo/ingestion"
	"codeburg.org/lexbit/relurpify/named/euclo/surface"
)

// BuildThoughtRecipeGraph builds an agentgraph.Graph for a compiled execution plan.
//
// The graph preserves the linear thoughtrecipe steps and also materializes parallel and
// conditional groups using the primitives available in framework/agentgraph.
func BuildThoughtRecipeGraph(plan *ExecutionPlan, env agentenv.WorkspaceEnvironment, ingestionPipeline *frameworkingestion.Pipeline) (*agentgraph.Graph, error) {
	_ = ingestionPipeline // retained for future plumbing; step nodes currently own ingestion.

	if plan == nil {
		return nil, fmt.Errorf("execution plan is nil")
	}

	graph := agentgraph.NewGraph()
	sections := make([]graphSection, 0, 1+len(plan.Parallel)+len(plan.Conditional))

	if len(plan.Steps) > 0 {
		section, err := buildLinearSection(graph, env, plan.Steps)
		if err != nil {
			return nil, err
		}
		sections = append(sections, section)
	}

	for _, group := range plan.Parallel {
		section, err := buildParallelSection(graph, env, group)
		if err != nil {
			return nil, err
		}
		sections = append(sections, section)
	}

	for _, group := range plan.Conditional {
		section, err := buildConditionalSection(graph, env, group)
		if err != nil {
			return nil, err
		}
		sections = append(sections, section)
	}

	for _, group := range plan.Routes {
		section, err := buildRouteSection(graph, env, group)
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

func buildLinearSection(graph *agentgraph.Graph, env agentenv.WorkspaceEnvironment, steps []ExecutionStep) (graphSection, error) {
	artifacts := make([]stepArtifacts, 0, len(steps))
	for _, step := range steps {
		artifact, err := addExecutionStep(graph, env, step)
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

func buildParallelSection(graph *agentgraph.Graph, env agentenv.WorkspaceEnvironment, group CompiledParallelGroup) (graphSection, error) {
	groupID := scopedGroupNodeID(group.Group.ID, "parallel")
	if err := graph.AddNode(newThoughtRecipeStageNode(groupID, agentgraph.NodeTypeSystem, "parallel", map[string]any{
		"group_id": group.Group.ID,
		"merge":    group.Merge,
	})); err != nil {
		return graphSection{}, err
	}

	for _, step := range group.Steps {
		artifact, err := addExecutionStep(graph, env, ExecutionStep{
			ID:                  step.Step.ID,
			Type:                step.Type,
			Paradigm:            executionParadigmForStep(*step.Step),
			CapabilityID:        step.Step.CapabilityID,
			Prompt:              step.Step.Prompt,
			PromptID:            step.Step.PromptID,
			Mutation:            step.Step.Mutation,
			HITL:                step.Step.HITL,
			Stream:              cloneStreamSpec(step.Step.Parent.Context.Stream),
			Ingest:              cloneIngestSpec(step.Step.Parent.Context.Ingest),
			Fallback:            cloneStepAgent(step.Step.Fallback),
			Inherit:             append([]string(nil), step.Step.Parent.Context.Inherit...),
			Capture:             append([]string(nil), step.Step.Parent.Context.Capture...),
			Dependencies:        append([]string(nil), step.Step.Dependencies...),
			ClarificationConfig: cloneClarificationStepConfig(step.ClarificationConfig),
			Step:                *step.Step,
		})
		if err != nil {
			return graphSection{}, err
		}
		if err := graph.AddEdge(groupID, artifact.entry, nil, true); err != nil {
			return graphSection{}, err
		}
	}

	return graphSection{
		entry: groupID,
		tail:  groupID,
	}, nil
}

func buildConditionalSection(graph *agentgraph.Graph, env agentenv.WorkspaceEnvironment, group CompiledConditionalGroup) (graphSection, error) {
	condID := scopedGroupNodeID(group.Group.ID, "conditional")
	joinID := scopedGroupNodeID(group.Group.ID, "join")

	if err := graph.AddNode(newThoughtRecipeStageNode(condID, agentgraph.NodeTypeConditional, "condition", map[string]any{
		"group_id":  group.Group.ID,
		"condition": group.Condition,
	})); err != nil {
		return graphSection{}, err
	}
	if err := graph.AddNode(newThoughtRecipeStageNode(joinID, agentgraph.NodeTypeSystem, "join", map[string]any{
		"group_id": group.Group.ID,
	})); err != nil {
		return graphSection{}, err
	}

	ifEntry, err := buildBranchSequence(graph, env, group.IfSteps, joinID)
	if err != nil {
		return graphSection{}, err
	}
	elseEntry, err := buildBranchSequence(graph, env, group.ElseSteps, joinID)
	if err != nil {
		return graphSection{}, err
	}

	if ifEntry != "" {
		if err := graph.AddEdge(condID, ifEntry, conditionTrue(group.Group.ID), false); err != nil {
			return graphSection{}, err
		}
	}
	if elseEntry != "" {
		if err := graph.AddEdge(condID, elseEntry, conditionFalse(group.Group.ID), false); err != nil {
			return graphSection{}, err
		}
	}

	return graphSection{
		entry:    condID,
		tail:     joinID,
		fallback: "",
	}, nil
}

func buildRouteSection(graph *agentgraph.Graph, env agentenv.WorkspaceEnvironment, group CompiledRouteGroup) (graphSection, error) {
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

	nextEntry := joinID
	for i := len(group.Branches) - 1; i >= 0; i-- {
		branch := group.Branches[i]
		bodyEntry, err := buildExecutionSequence(graph, env, branch.Steps, joinID)
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
		if err := graph.AddNode(newThoughtRecipeStageNode(branchID, agentgraph.NodeTypeConditional, "route_condition", map[string]any{
			"group_id":  group.Group.ID,
			"branch_id": branchID,
			"predicate": branch.Predicate,
		})); err != nil {
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
		tail:  joinID,
	}, nil
}

func buildExecutionSequence(graph *agentgraph.Graph, env agentenv.WorkspaceEnvironment, steps []ExecutionStep, continuation string) (string, error) {
	if len(steps) == 0 {
		if strings.TrimSpace(continuation) != "" {
			return continuation, nil
		}
		return "", nil
	}

	artifacts := make([]stepArtifacts, 0, len(steps))
	for _, step := range steps {
		artifact, err := addExecutionStep(graph, env, step)
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

func buildBranchSequence(graph *agentgraph.Graph, env agentenv.WorkspaceEnvironment, steps []CompiledStep, continuation string) (string, error) {
	if len(steps) == 0 {
		if strings.TrimSpace(continuation) != "" {
			return continuation, nil
		}
		return "", nil
	}

	artifacts := make([]stepArtifacts, 0, len(steps))
	for _, step := range steps {
		execStep := ExecutionStep{
			ID:                  step.Step.ID,
			Type:                step.Type,
			Paradigm:            executionParadigmForStep(*step.Step),
			CapabilityID:        step.Step.CapabilityID,
			Prompt:              step.Step.Prompt,
			PromptID:            step.Step.PromptID,
			Mutation:            step.Step.Mutation,
			HITL:                step.Step.HITL,
			Stream:              cloneStreamSpec(step.Step.Parent.Context.Stream),
			Ingest:              cloneIngestSpec(step.Step.Parent.Context.Ingest),
			Fallback:            cloneStepAgent(step.Step.Fallback),
			Inherit:             append([]string(nil), step.Step.Parent.Context.Inherit...),
			Capture:             append([]string(nil), step.Step.Parent.Context.Capture...),
			Dependencies:        append([]string(nil), step.Step.Dependencies...),
			ClarificationConfig: cloneClarificationStepConfig(step.ClarificationConfig),
			Step:                *step.Step,
		}
		artifact, err := addExecutionStep(graph, env, execStep)
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

func executionStepFromAgent(id string, agent *surface.ThoughtRecipeStepAgent, parent ExecutionStep) ExecutionStep {
	if agent == nil {
		return inheritExecutionStepScope(ExecutionStep{ID: id}, parent)
	}
	step := surface.ThoughtRecipeStep{
		ID:      id,
		Parent:  *agent,
		Context: agent.Context,
	}
	return inheritExecutionStepScope(ExecutionStep{
		ID:       id,
		Paradigm: agent.Paradigm,
		Prompt:   agent.Prompt,
		Stream:   cloneStreamSpec(agent.Context.Stream),
		Ingest:   cloneIngestSpec(agent.Context.Ingest),
		Inherit:  append([]string(nil), agent.Context.Inherit...),
		Capture:  append([]string(nil), agent.Context.Capture...),
		Step:     step,
	}, parent)
}

func inheritExecutionStepScope(step, parent ExecutionStep) ExecutionStep {
	if len(step.ToolScopes) == 0 && len(parent.ToolScopes) > 0 {
		step.ToolScopes = append([]ToolScopeFrame(nil), parent.ToolScopes...)
	}
	if len(step.EffectiveToolNames) == 0 && len(parent.EffectiveToolNames) > 0 {
		step.EffectiveToolNames = append([]string(nil), parent.EffectiveToolNames...)
	}
	if step.Step.Config == nil && (len(step.ToolScopes) > 0 || len(step.EffectiveToolNames) > 0) {
		step.Step.Config = map[string]any{}
	}
	if step.Step.Config != nil {
		if len(step.ToolScopes) > 0 {
			step.Step.Config["tool_scopes"] = summarizeToolScopeFrames(step.ToolScopes)
		}
		if len(step.EffectiveToolNames) > 0 {
			step.Step.Config["effective_tool_names"] = append([]string(nil), step.EffectiveToolNames...)
		}
	}
	return step
}

func addExecutionStep(graph *agentgraph.Graph, env agentenv.WorkspaceEnvironment, step ExecutionStep) (stepArtifacts, error) {
	if strings.EqualFold(strings.TrimSpace(step.Type), "pipeline") {
		return addPipelineStep(graph, env, step)
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
			"execution_step_type": step.Type,
		}
		if cfg := cloneClarificationStepConfig(step.ClarificationConfig); cfg != nil {
			streamNode.Metadata["execution_clarification_type"] = step.Type
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
			"step_type":          step.Type,
			"mutation":           step.Mutation,
			"hitl":               step.HITL,
			"clarification_type": step.Type,
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
	if err := graph.AddNode(NewThoughtRecipeStepNode(execNodeID, env, step)); err != nil {
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
		fallbackStep := executionStepFromAgent(fallbackID, step.Fallback, step)
		if err := graph.AddNode(NewThoughtRecipeStepNode(fallbackID, env, fallbackStep)); err != nil {
			return stepArtifacts{}, err
		}
		if err := graph.AddEdge(execNodeID, fallbackID, func(result *core.Result, env *contextdata.Envelope) bool {
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

func addPipelineStep(graph *agentgraph.Graph, env agentenv.WorkspaceEnvironment, step ExecutionStep) (stepArtifacts, error) {
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
		bodyEntry, err := buildExecutionSequence(graph, env, stage.Steps, nextEntry)
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

func successCondition(result *core.Result, env *contextdata.Envelope) bool {
	_ = env
	return result == nil || result.Success
}

func conditionTrue(groupID string) agentgraph.ConditionFunc {
	key := conditionResultKey(groupID)
	return func(result *core.Result, env *contextdata.Envelope) bool {
		_ = result
		return envBool(env, key)
	}
}

func conditionFalse(groupID string) agentgraph.ConditionFunc {
	key := conditionResultKey(groupID)
	return func(result *core.Result, env *contextdata.Envelope) bool {
		_ = result
		return !envBool(env, key)
	}
}

func conditionResultKey(groupID string) string {
	return "euclo.execution.group." + sanitizeComponent(groupID) + ".condition_met"
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
	id   string
	kind string
	op   string
	data map[string]any
}

func newThoughtRecipeStageNode(id string, nodeType agentgraph.NodeType, op string, data map[string]any) *thoughtrecipeStageNode {
	return &thoughtrecipeStageNode{id: id, kind: string(nodeType), op: op, data: data}
}

func (n *thoughtrecipeStageNode) ID() string                { return n.id }
func (n *thoughtrecipeStageNode) Type() agentgraph.NodeType { return agentgraph.NodeType(n.kind) }
func (n *thoughtrecipeStageNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
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
		case "condition":
			matched := evaluateThoughtRecipeCondition(env, fmt.Sprint(n.data["condition"]))
			// envelope: intentional dynamic key — condition result is scoped by group ID.
			contextdata.SetTyped(env, conditionResultKey(fmt.Sprint(n.data["group_id"])), matched)
			n.data["condition_met"] = matched
		case "route":
			// envelope: intentional dynamic key — route state is scoped by group ID.
			contextdata.SetTyped(env, "euclo.execution.route."+sanitizeComponent(fmt.Sprint(n.data["group_id"])), true)
		case "route_condition":
			matched := evaluateRoutePredicate(env, n.data["predicate"])
			branchID := fmt.Sprint(n.data["branch_id"])
			// envelope: intentional dynamic key — route condition results are scoped by group and branch.
			contextdata.SetTyped(env, routeConditionResultKey(fmt.Sprint(n.data["group_id"]), branchID), matched)
			n.data["condition_met"] = matched
		}
	}
	return &core.Result{
		NodeID:  n.id,
		Success: true,
		Data:    core.NewToolResultPayload(n.data),
	}, nil
}

func evaluateThoughtRecipeCondition(env *contextdata.Envelope, expr string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	switch strings.ToLower(expr) {
	case "true", "yes", "on", "1":
		return true
	case "false", "no", "off", "0":
		return false
	}
	if strings.HasPrefix(expr, "!") {
		return !evaluateThoughtRecipeCondition(env, strings.TrimSpace(expr[1:]))
	}
	if key, want, ok := strings.Cut(expr, "=="); ok {
		return strings.TrimSpace(fmt.Sprint(lookupThoughtRecipeConditionValue(env, key))) == strings.TrimSpace(want)
	}
	if key, want, ok := strings.Cut(expr, "!="); ok {
		return strings.TrimSpace(fmt.Sprint(lookupThoughtRecipeConditionValue(env, key))) != strings.TrimSpace(want)
	}
	return truthy(lookupThoughtRecipeConditionValue(env, expr))
}

func evaluateRoutePredicate(env *contextdata.Envelope, value any) bool {
	pred, ok := value.(*RoutePredicate)
	if !ok || pred == nil {
		if predValue, ok := value.(RoutePredicate); ok {
			pred = &predValue
		} else {
			return false
		}
	}
	subject := valueExprRaw(pred.Subject)
	switch pred.Kind {
	case "missing":
		return !truthy(lookupThoughtRecipeConditionValue(env, subject))
	case "present":
		return truthy(lookupThoughtRecipeConditionValue(env, subject))
	case "is":
		return strings.TrimSpace(fmt.Sprint(lookupThoughtRecipeConditionValue(env, subject))) == strings.TrimSpace(valueExprRaw(pred.Value))
	case "contains":
		have := lookupThoughtRecipeConditionValue(env, subject)
		want := strings.TrimSpace(valueExprRaw(pred.Value))
		switch v := have.(type) {
		case string:
			return strings.Contains(v, want)
		case []string:
			for _, entry := range v {
				if strings.TrimSpace(entry) == want {
					return true
				}
			}
		case []any:
			for _, entry := range v {
				if strings.TrimSpace(fmt.Sprint(entry)) == want {
					return true
				}
			}
		default:
			return strings.Contains(strings.TrimSpace(fmt.Sprint(have)), want)
		}
		return false
	case "confidence_below":
		have := lookupThoughtRecipeConditionValue(env, subject)
		threshold := routeConfidenceThreshold(pred.Value)
		if threshold < 0 {
			return false
		}
		if confidence, ok := routeConfidenceValue(have); ok {
			return confidence < threshold
		}
		confidence, ok := routeConfidenceValue(lookupThoughtRecipeConditionValue(env, subject+"_confidence"))
		if ok {
			return confidence < threshold
		}
		confidence, ok = routeConfidenceValue(lookupThoughtRecipeConditionValue(env, subject+".confidence"))
		return ok && confidence < threshold
	default:
		return false
	}
}

func routeConfidenceThreshold(value ValueExpr) int {
	raw := strings.TrimSpace(valueExprRaw(value))
	raw = strings.TrimSuffix(raw, "%")
	if raw == "" {
		return -1
	}
	var threshold int
	if _, err := fmt.Sscanf(raw, "%d", &threshold); err != nil {
		return -1
	}
	return threshold
}

func routeConfidenceValue(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSuffix(strings.TrimSpace(v), "%"), "%d", &parsed); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func routeConditionResultKey(groupID, branchID string) string {
	return "euclo.execution.group." + sanitizeComponent(groupID) + ".branch." + sanitizeComponent(branchID) + ".condition_met"
}

func routeConditionTrue(groupID, branchID string) agentgraph.ConditionFunc {
	key := routeConditionResultKey(groupID, branchID)
	return func(result *core.Result, env *contextdata.Envelope) bool {
		_ = result
		return envBool(env, key)
	}
}

func routeConditionFalse(groupID, branchID string) agentgraph.ConditionFunc {
	key := routeConditionResultKey(groupID, branchID)
	return func(result *core.Result, env *contextdata.Envelope) bool {
		_ = result
		return !envBool(env, key)
	}
}

func lookupThoughtRecipeConditionValue(env *contextdata.Envelope, key string) any {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if value, ok := contextdata.GetTyped[any](env, key); ok {
		return value
	}
	return nil
}
