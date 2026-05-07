package recipe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"

	blackboardagent "codeburg.org/lexbit/relurpify/agents/blackboard"
	chaineragent "codeburg.org/lexbit/relurpify/agents/chainer"
	goalconagent "codeburg.org/lexbit/relurpify/agents/goalcon"
	htnagent "codeburg.org/lexbit/relurpify/agents/htn"
	pipelineagent "codeburg.org/lexbit/relurpify/agents/pipeline"
	planneragent "codeburg.org/lexbit/relurpify/agents/planner"
	reactagent "codeburg.org/lexbit/relurpify/agents/react"
	reflectionagent "codeburg.org/lexbit/relurpify/agents/reflection"
	rewooagent "codeburg.org/lexbit/relurpify/agents/rewoo"
	"codeburg.org/lexbit/relurpify/framework/agentenv"
	"codeburg.org/lexbit/relurpify/framework/agentgraph"
	"codeburg.org/lexbit/relurpify/framework/capability"
	"codeburg.org/lexbit/relurpify/framework/contextdata"
	"codeburg.org/lexbit/relurpify/framework/contextstream"
	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/knowledge"
	"codeburg.org/lexbit/relurpify/framework/prompt"
	"codeburg.org/lexbit/relurpify/framework/retrieval"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/platform/contracts"
)

// RecipeStepNode executes a compiled recipe step by delegating to the matching
// /agents constructor for the step's paradigm.
type RecipeStepNode struct {
	id   string
	env  agentenv.WorkspaceEnvironment
	step ExecutionStep
}

// NewRecipeStepNode creates a new agent-backed recipe step node.
func NewRecipeStepNode(id string, env agentenv.WorkspaceEnvironment, step ExecutionStep) *RecipeStepNode {
	return &RecipeStepNode{
		id:   id,
		env:  env,
		step: step,
	}
}

// ID implements agentgraph.Node.
func (n *RecipeStepNode) ID() string { return n.id }

// Type implements agentgraph.Node.
func (n *RecipeStepNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeTool }

// Execute builds the selected paradigm agent, runs it, and writes captures back
// to the envelope.
func (n *RecipeStepNode) Execute(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	if n == nil {
		return nil, fmt.Errorf("recipe step node is nil")
	}
	if env == nil {
		return nil, fmt.Errorf("recipe step node %q missing envelope", n.id)
	}

	if strings.TrimSpace(n.step.CapabilityID) != "" {
		return n.executeCapability(ctx, env)
	}

	task, err := n.buildTask(env)
	if err != nil {
		return nil, err
	}
	agent, err := n.buildAgent(task)
	if err != nil {
		return nil, err
	}

	result, execErr := agent.Execute(ctx, task, env)
	if result == nil {
		result = &core.Result{
			NodeID:  n.id,
			Success: execErr == nil,
			Data:    map[string]any{},
		}
	}
	if result.Data == nil {
		result.Data = map[string]any{}
	}
	if execErr != nil {
		result.Success = false
		result.Error = execErr.Error()
	}

	captured := n.captureValues(result)
	for key, value := range captured {
		env.SetWorkingValue(key, value, contextdata.MemoryClassTask)
	}
	env.SetWorkingValue("euclo.recipe.step."+n.step.ID+".result", result.Data, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.recipe.step."+n.step.ID+".success", result.Success, contextdata.MemoryClassTask)
	if result.Error != "" {
		env.SetWorkingValue("euclo.recipe.step."+n.step.ID+".error", result.Error, contextdata.MemoryClassTask)
	}

	if execErr != nil {
		// Preserve the failure in the result while keeping graph control flow
		// conditional instead of aborting immediately.
		return result, nil
	}
	return result, nil
}

func (n *RecipeStepNode) executeCapability(ctx context.Context, env *contextdata.Envelope) (*core.Result, error) {
	if n == nil {
		return nil, fmt.Errorf("recipe step node is nil")
	}
	if env == nil {
		return nil, fmt.Errorf("recipe step node %q missing envelope", n.id)
	}
	n.writeStepMetadata(env)

	reg := n.env.Registry
	if reg == nil {
		return nil, fmt.Errorf("recipe step %q: capability_id requires a registry", n.id)
	}
	if scoped := n.scopedRegistry(); scoped != nil {
		reg = scoped
	}

	args := n.buildCapabilityArgs(env)
	toolResult, err := reg.InvokeCapability(ctx, env, n.step.CapabilityID, args)

	data := map[string]any{
		"capability_id": n.step.CapabilityID,
	}
	success := err == nil
	if toolResult != nil {
		data["output"] = toolResult.Data
		if toolResult.Metadata != nil {
			data["metadata"] = toolResult.Metadata
		}
		if strings.TrimSpace(toolResult.Error) != "" {
			data["error"] = toolResult.Error
		}
		if !toolResult.Success {
			success = false
		}
	}
	if err != nil {
		data["error"] = err.Error()
	}

	if n.env.IngestOutputs && n.env.OutputIngester != nil {
		if payload, marshalErr := json.Marshal(data); marshalErr == nil {
			knowledge.IngestToolResultAsync(contextdata.WithEnvelope(ctx, env), n.env.OutputIngester, n.step.CapabilityID, payload)
		}
	}

	result := &core.Result{
		NodeID:  n.id,
		Success: success,
		Data:    data,
	}
	if msg, ok := data["error"].(string); ok && strings.TrimSpace(msg) != "" {
		result.Error = msg
	}

	captured := n.captureValues(result)
	for key, value := range captured {
		env.SetWorkingValue(key, value, contextdata.MemoryClassTask)
	}
	env.SetWorkingValue("euclo.recipe.step."+n.step.ID+".result", data, contextdata.MemoryClassTask)
	env.SetWorkingValue("euclo.recipe.step."+n.step.ID+".success", success, contextdata.MemoryClassTask)
	if result.Error != "" {
		env.SetWorkingValue("euclo.recipe.step."+n.step.ID+".error", result.Error, contextdata.MemoryClassTask)
	}

	return result, nil
}

func (n *RecipeStepNode) buildTask(env *contextdata.Envelope) (*core.Task, error) {
	data := recipeTemplateData(env, n.step)
	n.writeStepMetadata(env)

	var instruction string
	if n.step.PromptID != "" {
		if n.env.PromptRegistry == nil {
			return nil, fmt.Errorf("recipe step %q: prompt_id requires a registry", n.step.ID)
		}
		var err error
		instruction, err = n.resolveFromRegistry(env)
		if err != nil {
			return nil, err
		}
	}

	if instruction == "" {
		instruction = n.renderTemplate(n.step.Prompt, data)
		if instruction == "" {
			instruction = n.step.Prompt
		}
	}

	task := &core.Task{
		ID:          n.id,
		Type:        n.step.Paradigm,
		Instruction: instruction,
		Data:        make(map[string]interface{}),
		Context:     data,
		Metadata:    n.stepMetadata(),
	}
	if len(n.step.Bindings) > 0 {
		for key, ref := range n.step.Bindings {
			if value, ok := lookupTemplateValue(data, ref); ok {
				task.Context[key] = value
			}
		}
	}

	// Set prompt_id in task context for downstream agents (like react think node)
	if n.step.PromptID != "" {
		task.Context["prompt_id"] = n.step.PromptID
	}

	return task, nil
}

// resolveFromRegistry resolves the prompt from the registry using the PromptID.
func (n *RecipeStepNode) resolveFromRegistry(env *contextdata.Envelope) (string, error) {
	if n.env.PromptRegistry == nil {
		return "", fmt.Errorf("no prompt registry available")
	}

	// Build runtime context for prompt resolution
	rctx := n.buildRuntimeContext(env)

	// Resolve the prompt from registry
	return n.env.PromptRegistry.Resolve(n.step.PromptID, rctx)
}

// buildRuntimeContext creates a prompt.RuntimeContext for recipe step resolution.
func (n *RecipeStepNode) buildRuntimeContext(env *contextdata.Envelope) prompt.RuntimeContext {
	data := recipeTemplateData(env, n.step)
	runtime := prompt.NewRuntimeContext(env, n.step.Paradigm, "euclo").
		WithVariable("instruction", n.renderTemplate(n.step.Prompt, data)).
		WithVariable("question", n.step.Prompt).
		WithVariable("prompt_id", n.step.PromptID).
		WithStateMap(clarificationRuntimeState(env)).
		WithStateMap(n.stepRuntimeState(data))

	runtime.Task = &core.Task{
		ID:          n.id,
		Type:        n.step.Paradigm,
		Instruction: n.renderTemplate(n.step.Prompt, data),
		Context:     data,
	}
	runtime.Tools = []contracts.Tool{}
	runtime.Capabilities = []core.CapabilityDescriptor{}
	runtime.AgentSpec = nil
	return runtime
}

func (n *RecipeStepNode) stepMetadata() map[string]interface{} {
	metadata := map[string]interface{}{
		"recipe_step_id":   n.step.ID,
		"recipe_step_type": n.step.Type,
		"recipe_paradigm":  n.step.Paradigm,
		"recipe_mutation":  n.step.Mutation,
		"recipe_hitl":      n.step.HITL,
	}
	if cfg := cloneClarificationStepConfig(n.step.ClarificationConfig); cfg != nil {
		metadata["recipe_clarification_type"] = n.step.Type
		metadata["recipe_clarification_config"] = cfg
	}
	return metadata
}

func (n *RecipeStepNode) stepRuntimeState(data map[string]any) map[string]any {
	state := map[string]any{
		"recipe_step_id":     n.step.ID,
		"recipe_step_type":   n.step.Type,
		"recipe_paradigm":    n.step.Paradigm,
		"recipe_prompt_id":   n.step.PromptID,
		"recipe_instruction": n.renderTemplate(n.step.Prompt, data),
	}
	if cfg := cloneClarificationStepConfig(n.step.ClarificationConfig); cfg != nil {
		state["recipe_clarification_type"] = n.step.Type
		state["recipe_clarification_config"] = cfg
		state["recipe_clarification_schema_id"] = cfg.OutputSchemaID
		state["recipe_clarification_validation_mode"] = cfg.ValidationMode
		state["recipe_clarification_required_fields"] = append([]string(nil), cfg.RequiredFields...)
		state["recipe_clarification_allowed_statuses"] = append([]intentcontext.ClarificationStepStatus(nil), cfg.AllowedStatuses...)
		state["recipe_clarification_state_write_keys"] = append([]string(nil), cfg.StateWriteKeys...)
		state["recipe_clarification_projection_policy"] = cfg.ProjectionPolicy
		state["recipe_clarification_requery_on_success"] = cfg.RequeryOnSuccess
	}
	return state
}

func (n *RecipeStepNode) writeStepMetadata(env *contextdata.Envelope) {
	if env == nil {
		return
	}
	base := "euclo.recipe.step." + n.step.ID
	env.SetWorkingValue(base+".id", n.step.ID, contextdata.MemoryClassTask)
	env.SetWorkingValue(base+".type", n.step.Type, contextdata.MemoryClassTask)
	env.SetWorkingValue(base+".paradigm", n.step.Paradigm, contextdata.MemoryClassTask)
	env.SetWorkingValue(base+".prompt_id", n.step.PromptID, contextdata.MemoryClassTask)
	env.SetWorkingValue(base+".mutation", n.step.Mutation, contextdata.MemoryClassTask)
	env.SetWorkingValue(base+".hitl", n.step.HITL, contextdata.MemoryClassTask)
	if cfg := cloneClarificationStepConfig(n.step.ClarificationConfig); cfg != nil {
		env.SetWorkingValue(base+".clarification_type", n.step.Type, contextdata.MemoryClassTask)
		env.SetWorkingValue(base+".clarification_config", cfg, contextdata.MemoryClassTask)
		env.SetWorkingValue(base+".clarification_schema_id", cfg.OutputSchemaID, contextdata.MemoryClassTask)
		env.SetWorkingValue(base+".clarification_validation_mode", cfg.ValidationMode, contextdata.MemoryClassTask)
		env.SetWorkingValue(base+".clarification_required_fields", append([]string(nil), cfg.RequiredFields...), contextdata.MemoryClassTask)
		env.SetWorkingValue(base+".clarification_allowed_statuses", append([]intentcontext.ClarificationStepStatus(nil), cfg.AllowedStatuses...), contextdata.MemoryClassTask)
		env.SetWorkingValue(base+".clarification_state_write_keys", append([]string(nil), cfg.StateWriteKeys...), contextdata.MemoryClassTask)
		env.SetWorkingValue(base+".clarification_projection_policy", cfg.ProjectionPolicy, contextdata.MemoryClassTask)
		env.SetWorkingValue(base+".clarification_requery_on_success", cfg.RequeryOnSuccess, contextdata.MemoryClassTask)
	}
}

func clarificationRuntimeState(env *contextdata.Envelope) map[string]any {
	if env == nil {
		return map[string]any{}
	}

	state := make(map[string]any)
	if current, err := intentcontext.NewStateStore().Read(context.Background(), env); err == nil && current != nil {
		state[intentcontext.ClarificationStateKey] = current.Clone()
		state["euclo.intent.clarification.state_version"] = current.StateVersion
		state["euclo.intent.clarification.current_turn_id"] = current.CurrentTurnID
		state["euclo.intent.clarification.active_recipe_id"] = current.ActiveRecipeID
		state["euclo.intent.clarification.last_checkpoint_id"] = current.LastCheckpointID
		state["euclo.intent.clarification.last_checkpoint_seq"] = current.LastCheckpointSeq
		state["euclo.intent.clarification.confirmed_entity_ids"] = stableEntityIDs(current.ConfirmedEntities)
		state["euclo.intent.clarification.confirmed_scope_ids"] = stableScopeIDs(current.ConfirmedScopes)
		state["euclo.intent.clarification.pending_projection_ids"] = stableProjectionIDs(current.PendingProjection)
		state["euclo.intent.clarification.grounded_anchor_ids"] = anchorIDs(current.GroundedAnchors)
		if current.Ambiguity != nil {
			state["euclo.intent.clarification.ambiguity_kind"] = string(current.Ambiguity.Kind)
			state["euclo.intent.clarification.ambiguity_confidence"] = current.Ambiguity.Confidence
			state["euclo.intent.clarification.ambiguity_rationale"] = current.Ambiguity.Rationale
		}
		if len(current.PendingQuestions) > 0 {
			state["euclo.intent.clarification.pending_questions"] = append([]intentcontext.ClarificationQuestion(nil), current.PendingQuestions...)
		}
		if len(current.Turns) > 0 {
			state["euclo.intent.clarification.turn_ids"] = turnIDs(current.Turns)
		}
	}
	return state
}

func anchorIDs(anchors []retrieval.AnchorRef) []string {
	if len(anchors) == 0 {
		return nil
	}
	ids := make([]string, 0, len(anchors))
	for _, anchor := range anchors {
		if strings.TrimSpace(anchor.AnchorID) != "" {
			ids = append(ids, strings.TrimSpace(anchor.AnchorID))
		}
	}
	return ids
}

func stableEntityIDs(entities []intentcontext.ConfirmedEntity) []string {
	if len(entities) == 0 {
		return nil
	}
	ids := make([]string, 0, len(entities))
	for _, entity := range entities {
		if strings.TrimSpace(entity.StableID) != "" {
			ids = append(ids, strings.TrimSpace(entity.StableID))
		}
	}
	return ids
}

func stableScopeIDs(scopes []intentcontext.ConfirmedScope) []string {
	if len(scopes) == 0 {
		return nil
	}
	ids := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if strings.TrimSpace(scope.StableID) != "" {
			ids = append(ids, strings.TrimSpace(scope.StableID))
		}
	}
	return ids
}

func stableProjectionIDs(intents []intentcontext.ProjectionIntent) []string {
	if len(intents) == 0 {
		return nil
	}
	ids := make([]string, 0, len(intents))
	for _, intent := range intents {
		if strings.TrimSpace(intent.StableID) != "" {
			ids = append(ids, strings.TrimSpace(intent.StableID))
		}
	}
	return ids
}

func turnIDs(turns []intentcontext.ClarificationTurn) []string {
	if len(turns) == 0 {
		return nil
	}
	ids := make([]string, 0, len(turns))
	for _, turn := range turns {
		if strings.TrimSpace(turn.TurnID) != "" {
			ids = append(ids, strings.TrimSpace(turn.TurnID))
		}
	}
	return ids
}

func (n *RecipeStepNode) buildCapabilityArgs(env *contextdata.Envelope) map[string]any {
	data := recipeTemplateData(env, n.step)
	args := make(map[string]any)
	for key, ref := range n.step.Bindings {
		if value, ok := lookupTemplateValue(data, ref); ok {
			args[key] = value
		}
	}
	for key, value := range n.step.Step.Config {
		if _, exists := args[key]; !exists {
			args[key] = value
		}
	}
	return args
}

func (n *RecipeStepNode) buildAgent(task *core.Task) (agentgraph.WorkflowExecutor, error) {
	scopedEnv := n.env
	if scopedRegistry := n.scopedRegistry(); scopedRegistry != nil {
		scopedEnv = scopedEnv.WithRegistry(scopedRegistry)
	}

	switch strings.ToLower(strings.TrimSpace(n.step.Paradigm)) {
	case "react":
		return reactagent.New(&scopedEnv, n.streamOptions()...), nil
	case "planner":
		return planneragent.New(&scopedEnv), nil
	case "htn":
		primitive := reactagent.New(&scopedEnv, n.streamOptions()...)
		return htnagent.New(&scopedEnv, htnagent.NewMethodLibrary(), append([]htnagent.Option{
			htnagent.WithPrimitiveExec(primitive),
		}, n.streamOptionsHTN()...)...), nil
	case "reflection":
		delegate := reactagent.New(&scopedEnv, n.streamOptions()...)
		return reflectionagent.New(&scopedEnv, delegate), nil
	case "blackboard":
		return blackboardagent.New(&scopedEnv, n.streamOptionsBlackboard()...), nil
	case "chainer":
		return chaineragent.New(&scopedEnv, n.streamOptionsChainer()...), nil
	case "pipeline":
		return pipelineagent.New(&scopedEnv, n.streamOptionsPipeline()...), nil
	case "rewoo":
		agent := rewooagent.New(&scopedEnv)
		agent.Options = n.rewooOptions()
		return agent, nil
	case "goalcon":
		agent := goalconagent.New(&scopedEnv, goalconagent.DefaultOperatorRegistry(), n.streamOptionsGoalCon()...)
		if agent != nil && agent.PlanExecutor == nil {
			agent.PlanExecutor = reactagent.New(&scopedEnv, n.streamOptions()...)
		}
		return agent, nil
	default:
		return nil, fmt.Errorf("recipe step %q has unsupported paradigm %q", n.step.ID, n.step.Paradigm)
	}
}

func (n *RecipeStepNode) scopedRegistry() *capability.Registry {
	if n == nil || n.env.Registry == nil {
		return nil
	}
	allowed := extractAllowedCapabilities(n.step.Step)
	if len(allowed) == 0 {
		return nil
	}
	return n.env.Registry.WithAllowlist(allowed)
}

func (n *RecipeStepNode) streamOptions() []reactagent.Option {
	opts := make([]reactagent.Option, 0, 3)
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts = append(opts, reactagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, reactagent.WithContextStreamQuery(query))
		}
		if n.step.Stream.MaxTokens > 0 {
			opts = append(opts, reactagent.WithContextStreamMaxTokens(n.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (n *RecipeStepNode) streamOptionsHTN() []htnagent.Option {
	opts := make([]htnagent.Option, 0, 3)
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts = append(opts, htnagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, htnagent.WithContextStreamQuery(query))
		}
		if n.step.Stream.MaxTokens > 0 {
			opts = append(opts, htnagent.WithContextStreamMaxTokens(n.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (n *RecipeStepNode) streamOptionsBlackboard() []blackboardagent.Option {
	opts := make([]blackboardagent.Option, 0, 3)
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts = append(opts, blackboardagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, blackboardagent.WithContextStreamQuery(query))
		}
		if n.step.Stream.MaxTokens > 0 {
			opts = append(opts, blackboardagent.WithContextStreamMaxTokens(n.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (n *RecipeStepNode) streamOptionsChainer() []chaineragent.Option {
	opts := make([]chaineragent.Option, 0, 3)
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts = append(opts, chaineragent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, chaineragent.WithContextStreamQuery(query))
		}
		if n.step.Stream.MaxTokens > 0 {
			opts = append(opts, chaineragent.WithContextStreamMaxTokens(n.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (n *RecipeStepNode) streamOptionsPipeline() []pipelineagent.Option {
	opts := make([]pipelineagent.Option, 0, 3)
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts = append(opts, pipelineagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, pipelineagent.WithContextStreamQuery(query))
		}
		if n.step.Stream.MaxTokens > 0 {
			opts = append(opts, pipelineagent.WithContextStreamMaxTokens(n.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (n *RecipeStepNode) streamOptionsGoalCon() []goalconagent.Option {
	opts := make([]goalconagent.Option, 0, 3)
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts = append(opts, goalconagent.WithContextStreamMode(contextstream.Mode(mode)))
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts = append(opts, goalconagent.WithContextStreamQuery(query))
		}
		if n.step.Stream.MaxTokens > 0 {
			opts = append(opts, goalconagent.WithContextStreamMaxTokens(n.step.Stream.MaxTokens))
		}
	}
	return opts
}

func (n *RecipeStepNode) rewooOptions() rewooagent.RewooOptions {
	opts := rewooagent.RewooOptions{}
	if n.step.Stream != nil {
		if mode := strings.TrimSpace(n.step.Stream.Mode); mode != "" {
			opts.StreamMode = contextstream.Mode(mode)
		}
		if query := strings.TrimSpace(n.step.Stream.QueryTemplate); query != "" {
			opts.StreamQuery = query
		}
		if n.step.Stream.MaxTokens > 0 {
			opts.StreamMaxTokens = n.step.Stream.MaxTokens
		}
	}
	return opts
}

func (n *RecipeStepNode) captureValues(result *core.Result) map[string]any {
	if n == nil || result == nil {
		return nil
	}
	captures := n.step.Captures
	if len(captures) == 0 {
		return nil
	}
	out := make(map[string]any, len(captures))
	for alias, key := range captures {
		value, ok := lookupCaptureValue(result.Data, alias)
		if !ok {
			if len(result.Data) == 1 {
				for _, candidate := range result.Data {
					value = candidate
					ok = true
				}
			}
		}
		if !ok {
			value = result.Data
		}
		out[key] = value
	}
	return out
}

func recipeTemplateData(env *contextdata.Envelope, step ExecutionStep) map[string]any {
	data := map[string]any{
		"TaskID":    "",
		"SessionID": "",
		"StepID":    step.ID,
		"Paradigm":  step.Paradigm,
		"Prompt":    step.Prompt,
	}
	if env != nil {
		data["TaskID"] = env.TaskID
		data["SessionID"] = env.SessionID
		for key, value := range env.Snapshot() {
			data[key] = value
			suffix := aliasFromEnvelopeKey(key)
			if suffix != "" {
				if _, exists := data[suffix]; !exists {
					data[suffix] = value
				}
			}
			normalized := normalizeTemplateKey(key)
			if normalized != "" {
				if _, exists := data[normalized]; !exists {
					data[normalized] = value
				}
			}
		}
	}
	return data
}

func (n *RecipeStepNode) renderTemplate(src string, data map[string]any) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	src = normalizeRecipeTemplate(src)
	tpl, err := template.New("recipe-step").Option("missingkey=zero").Parse(src)
	if err != nil {
		return src
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return src
	}
	return buf.String()
}

func lookupTemplateValue(data map[string]any, ref string) (any, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" || data == nil {
		return nil, false
	}
	if value, ok := data[ref]; ok {
		return value, true
	}
	if value, ok := data[aliasFromEnvelopeKey(ref)]; ok {
		return value, true
	}
	return nil, false
}

func lookupCaptureValue(data map[string]any, alias string) (any, bool) {
	alias = strings.TrimSpace(alias)
	if alias == "" || data == nil {
		return nil, false
	}
	if value, ok := data[alias]; ok {
		return value, true
	}
	if value, ok := data[normalizeTemplateKey(alias)]; ok {
		return value, true
	}
	return nil, false
}

func aliasFromEnvelopeKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	parts := strings.FieldsFunc(key, func(r rune) bool {
		switch r {
		case '.', '/', ':':
			return true
		default:
			return false
		}
	})
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func normalizeTemplateKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	replacer := strings.NewReplacer(".", "_", "-", "_", "/", "_", ":", "_")
	return replacer.Replace(key)
}

var simpleTemplatePattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

func normalizeRecipeTemplate(src string) string {
	if src == "" {
		return ""
	}
	return simpleTemplatePattern.ReplaceAllString(src, "{{.$1}}")
}

func extractAllowedCapabilities(step RecipeStep) []string {
	if len(step.Config) == 0 {
		return nil
	}
	return extractAllowedCapabilitiesFromMap(step.Config)
}

func extractAllowedCapabilitiesFromMap(data map[string]any) []string {
	if len(data) == 0 {
		return nil
	}
	if nested, ok := data["capabilities"].(map[string]any); ok {
		if allowed := stringsFromAny(nested["allowed"]); len(allowed) > 0 {
			return allowed
		}
	}
	if allowed := stringsFromAny(data["capabilities.allowed"]); len(allowed) > 0 {
		return allowed
	}
	if allowed := stringsFromAny(data["allowed_capabilities"]); len(allowed) > 0 {
		return allowed
	}
	return nil
}

func stringsFromAny(v any) []string {
	switch typed := v.(type) {
	case nil:
		return nil
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	default:
		return nil
	}
}

func sortedKeys(data map[string]any) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
