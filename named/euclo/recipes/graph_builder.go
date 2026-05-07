package recipe

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
)

// BuildRecipeGraph builds an agentgraph.Graph for a compiled execution plan.
//
// The graph preserves the linear recipe steps and also materializes parallel and
// conditional groups using the primitives available in framework/agentgraph.
func BuildRecipeGraph(plan *ExecutionPlan, env agentenv.WorkspaceEnvironment, ingestionPipeline *frameworkingestion.Pipeline) (*agentgraph.Graph, error) {
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

	if len(sections) == 0 {
		return nil, fmt.Errorf("execution plan has no steps")
	}

	terminalID := "euclo.recipe.done"
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
	if err := graph.AddNode(newRecipeStageNode(groupID, agentgraph.NodeTypeSystem, "parallel", map[string]any{
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
			Bindings:            step.Bindings,
			Captures:            step.Captures,
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

	if err := graph.AddNode(newRecipeStageNode(condID, agentgraph.NodeTypeConditional, "condition", map[string]any{
		"group_id":  group.Group.ID,
		"condition": group.Condition,
	})); err != nil {
		return graphSection{}, err
	}
	if err := graph.AddNode(newRecipeStageNode(joinID, agentgraph.NodeTypeSystem, "join", map[string]any{
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
			Bindings:            step.Bindings,
			Captures:            step.Captures,
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

func addExecutionStep(graph *agentgraph.Graph, env agentenv.WorkspaceEnvironment, step ExecutionStep) (stepArtifacts, error) {
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
			"recipe_step_id":   step.ID,
			"recipe_step_type": step.Type,
		}
		if cfg := cloneClarificationStepConfig(step.ClarificationConfig); cfg != nil {
			streamNode.Metadata["recipe_clarification_type"] = step.Type
			streamNode.Metadata["recipe_clarification_schema_id"] = cfg.OutputSchemaID
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
		if err := graph.AddNode(newRecipeStageNode(nodeID, agentgraph.NodeTypeSystem, "gate", map[string]any{
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
	if err := graph.AddNode(NewRecipeStepNode(execNodeID, env, step)); err != nil {
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
		fallbackStep := executionStepFromAgent(fallbackID, step.Fallback)
		if err := graph.AddNode(NewRecipeStepNode(fallbackID, env, fallbackStep)); err != nil {
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
	return "euclo.recipe.group." + sanitizeAliasComponent(groupID) + ".condition_met"
}

func scopedGroupNodeID(groupID, kind string) string {
	base := sanitizeAliasComponent(groupID)
	if strings.TrimSpace(base) == "" {
		base = "group"
	}
	kind = sanitizeAliasComponent(kind)
	if strings.TrimSpace(kind) == "" {
		kind = "node"
	}
	return "euclo.recipe.group." + base + "." + kind
}

func envBool(env *contextdata.Envelope, key string) bool {
	if env == nil {
		return false
	}
	value, ok := env.GetWorkingValue(key)
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
func (n *recipeStageNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	_ = ctx
	if env != nil && n.data != nil {
		for key, value := range n.data {
			env.SetWorkingValue("euclo.recipe.stage."+n.id+"."+key, value, contextdata.MemoryClassTask)
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
			env.SetWorkingValue("euclo.policy.mutation_permitted", mutationPermitted, contextdata.MemoryClassTask)
			env.SetWorkingValue("euclo.policy.hitl_required", hitlRequired, contextdata.MemoryClassTask)
			env.SetWorkingValue("euclo.policy.verification_required", !mutationPermitted || hitlRequired, contextdata.MemoryClassTask)
		case "condition":
			matched := evaluateRecipeCondition(env, fmt.Sprint(n.data["condition"]))
			env.SetWorkingValue(conditionResultKey(fmt.Sprint(n.data["group_id"])), matched, contextdata.MemoryClassTask)
			n.data["condition_met"] = matched
		}
	}
	return &core.Result{
		NodeID:  n.id,
		Success: true,
		Data:    n.data,
	}, nil
}

func evaluateRecipeCondition(env *contextdata.Envelope, expr string) bool {
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
		return !evaluateRecipeCondition(env, strings.TrimSpace(expr[1:]))
	}
	if key, want, ok := strings.Cut(expr, "=="); ok {
		return strings.TrimSpace(fmt.Sprint(lookupRecipeConditionValue(env, key))) == strings.TrimSpace(want)
	}
	if key, want, ok := strings.Cut(expr, "!="); ok {
		return strings.TrimSpace(fmt.Sprint(lookupRecipeConditionValue(env, key))) != strings.TrimSpace(want)
	}
	return truthy(lookupRecipeConditionValue(env, expr))
}

func lookupRecipeConditionValue(env *contextdata.Envelope, key string) any {
	if env == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if value, ok := env.GetWorkingValue(key); ok {
		return value
	}
	return nil
}
