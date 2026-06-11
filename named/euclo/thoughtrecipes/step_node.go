package thoughtrecipe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	registry "codeburg.org/lexbit/relurpify/capability/registry"
	blackboardagent "codeburg.org/lexbit/relurpify/cognitionzoo/blackboard"
	chaineragent "codeburg.org/lexbit/relurpify/cognitionzoo/chainer"
	goalconagent "codeburg.org/lexbit/relurpify/cognitionzoo/goalcon"
	htnagent "codeburg.org/lexbit/relurpify/cognitionzoo/htn"
	"codeburg.org/lexbit/relurpify/cognitionzoo/paradigm"
	pipelineagent "codeburg.org/lexbit/relurpify/cognitionzoo/pipeline"
	planneragent "codeburg.org/lexbit/relurpify/cognitionzoo/planner"
	reactagent "codeburg.org/lexbit/relurpify/cognitionzoo/react"
	reflectionagent "codeburg.org/lexbit/relurpify/cognitionzoo/reflection"
	rewooagent "codeburg.org/lexbit/relurpify/cognitionzoo/rewoo"
	"codeburg.org/lexbit/relurpify/context/contextdata"
	"codeburg.org/lexbit/relurpify/context/contextstream"
	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/context/knowledge/retrieval"
	execution "codeburg.org/lexbit/relurpify/execution"
	"codeburg.org/lexbit/relurpify/execution/agentgraph"
	"codeburg.org/lexbit/relurpify/execution/prompt"
	"codeburg.org/lexbit/relurpify/named/euclo/intentcontext"
	"codeburg.org/lexbit/relurpify/named/euclo/interaction"
	"codeburg.org/lexbit/relurpify/named/euclo/reporting"
	"codeburg.org/lexbit/relurpify/named/euclo/state"
	telemetry "codeburg.org/lexbit/relurpify/telemetry"
)

const (
	executionCapabilityIDKey = "execution_capability_id"
	eucloCapabilityIDKey     = "euclo.execution.capability_id"
)

// ThoughtRecipeStepNode executes a compiled thoughtrecipe step by delegating to the matching
// cognitionzoo constructor for the step's paradigm.
type ThoughtRecipeStepNode struct {
	id   string
	deps *paradigm.Deps
	step ExecutionStep
}

// NewThoughtRecipeStepNode creates a new agent-backed thoughtrecipe step node.
func NewThoughtRecipeStepNode(id string, deps *paradigm.Deps, step ExecutionStep) *ThoughtRecipeStepNode {
	return &ThoughtRecipeStepNode{
		id:   id,
		deps: deps,
		step: step,
	}
}

// ID implements agentgraph.Node.
func (n *ThoughtRecipeStepNode) ID() string { return n.id }

// Type implements agentgraph.Node.
func (n *ThoughtRecipeStepNode) Type() agentgraph.NodeType { return agentgraph.NodeTypeTool }

// Execute builds the selected paradigm agent, runs it, and writes captures back
// to the envelope.
func (n *ThoughtRecipeStepNode) Execute(ctx context.Context, env *contextdata.Envelope) (retResult *execution.Result, retErr error) {
	if n == nil {
		return nil, fmt.Errorf("thoughtrecipe step node is nil")
	}

	start := time.Now()
	emitStepStarted(ctx, env, n.step)

	defer func() {
		success := retResult != nil && retResult.Success && retErr == nil
		dur := time.Since(start)
		emitStepCompleted(ctx, env, n.step, success, dur)
	}()

	if strings.TrimSpace(n.step.CapabilityID) != "" {
		return n.executeCapability(ctx, env)
	}
	if strings.EqualFold(strings.TrimSpace(n.step.Type), "ask") {
		return n.executeAsk(ctx, env)
	}
	if strings.EqualFold(strings.TrimSpace(n.step.Type), "delegate") {
		return n.executeDelegation(ctx, env)
	}

	task, err := n.buildTask(ctx, env)
	if err != nil {
		return nil, err
	}
	agent, err := n.buildAgent(task)
	if err != nil {
		return nil, err
	}

	result, execErr := agent.Execute(ctx, task, env)
	if result == nil {
		result = &execution.Result{
			NodeID:  n.id,
			Success: execErr == nil,
			Data:    execution.NewToolResultPayload(map[string]any{}),
		}
	}
	if result.Data == nil {
		result.Data = execution.NewToolResultPayload(map[string]any{})
	}
	if execErr != nil {
		result.Success = false
		result.Error = execErr.Error()
	}

	if err := n.writeCaptures(env, result); err != nil {
		return result, err
	}
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".result", result.Data)
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".success", result.Success)
	if result.Error != "" {
		contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".error", result.Error)
	}

	if execErr != nil {
		return result, nil
	}
	return result, nil
}

func emitStepStarted(ctx context.Context, env *contextdata.Envelope, step ExecutionStep) {
	total, _ := contextdata.GetTyped[int](env, "euclo.execution.step_total")
	idx, _ := contextdata.GetTyped[int](env, "euclo.execution.step_index")
	contextdata.SetTyped(env, "euclo.execution.step_index", idx+1)

	tel := reporting.NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
	tel.EmitStepStarted(ctx, reporting.EventStepStarted{
		EventHeader: reporting.EventHeader{
			TaskID:     env.TaskID,
			SessionID:  env.SessionID,
			OccurredAt: time.Now().UTC(),
		},
		StepID:          step.ID,
		ThoughtRecipeID: "",
		Paradigm:        step.Paradigm,
		Index:           idx,
		Total:           total,
	})
}

func emitStepCompleted(ctx context.Context, env *contextdata.Envelope, step ExecutionStep, success bool, dur time.Duration) {
	total, _ := contextdata.GetTyped[int](env, "euclo.execution.step_total")
	idx, _ := contextdata.GetTyped[int](env, "euclo.execution.step_index")

	tel := reporting.NewEucloTelemetry(telemetry.TelemetryFromContext(ctx))
	tel.EmitStepCompleted(ctx, reporting.EventStepCompleted{
		EventHeader: reporting.EventHeader{
			TaskID:     env.TaskID,
			SessionID:  env.SessionID,
			OccurredAt: time.Now().UTC(),
		},
		StepID:          step.ID,
		ThoughtRecipeID: "",
		Paradigm:        step.Paradigm,
		Success:         success,
		DurationMs:      dur.Milliseconds(),
		Index:           idx - 1,
		Total:           total,
	})
}

func (n *ThoughtRecipeStepNode) executeDelegation(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n == nil {
		return nil, fmt.Errorf("thoughtrecipe step node is nil")
	}

	childEnv := n.buildDelegationEnvelope(env)
	task, err := n.buildTask(ctx, childEnv)
	if err != nil {
		return nil, err
	}
	agent, err := n.buildAgent(task)
	if err != nil {
		return nil, err
	}

	result, execErr := agent.Execute(ctx, task, childEnv)
	if result == nil {
		result = &execution.Result{
			NodeID:  n.id,
			Success: execErr == nil,
			Data:    execution.NewToolResultPayload(map[string]any{}),
		}
	}
	if result.Data == nil {
		result.Data = execution.NewToolResultPayload(map[string]any{})
	}
	if execErr != nil {
		result.Success = false
		result.Error = execErr.Error()
	}

	if err := n.writeDelegationCaptures(env, childEnv, result); err != nil {
		return result, err
	}
	n.writeStepMetadata(env)
	contextdata.SetTyped(env, "euclo.execution.delegate."+n.step.ID+".child_task_id", childEnv.TaskID)
	contextdata.SetTyped(env, "euclo.execution.delegate."+n.step.ID+".parent_task_id", env.TaskID)
	if len(n.step.Sources) > 0 {
		contextdata.SetTyped(env, "euclo.execution.delegate."+n.step.ID+".sources", append([]string(nil), n.step.Sources...))
	}
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".result", result.Data)
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".success", result.Success)
	if result.Error != "" {
		contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".error", result.Error)
	}

	if execErr != nil {
		return result, execErr
	}
	return result, nil
}

func (n *ThoughtRecipeStepNode) executeAsk(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n == nil {
		return nil, fmt.Errorf("thoughtrecipe step node is nil")
	}

	frame, created := n.ensureAskFrame(env)
	if frame == nil {
		return nil, fmt.Errorf("ask step %q failed to initialize frame", n.step.ID)
	}

	if frame.Response == nil || frame.RespondedAt == nil {
		if created {
			if err := interaction.EmitFrame(ctx, frame, env, telemetry.TelemetryFromContext(ctx)); err != nil {
				return nil, err
			}
			// envelope: intentional dynamic key — ask frames are keyed by step ID.
			contextdata.SetTyped(env, askFrameKey(n.step.ID), frame)
		}
		state.SetInteractionFrameRequested(env, true)
		state.SetInteractionResumeNodeID(env, n.id)
		return &execution.Result{
			NodeID:  n.id,
			Success: true,
			Data: execution.NewToolResultPayload(map[string]any{
				"paused":   true,
				"frame_id": frame.ID,
				"question": n.step.Question,
			}),
			Metadata: map[string]any{
				"euclo.interaction.pause": true,
				"frame_id":                frame.ID,
			},
		}, nil
	}

	answer, _ := interaction.ResponseValue(frame)
	result := &execution.Result{
		NodeID:  n.id,
		Success: true,
		Data: execution.NewToolResultPayload(map[string]any{
			"answer":        answer,
			"selected_slot": answer,
			"frame_id":      frame.ID,
		}),
	}
	if frame.Response != nil && len(frame.Response.ExtraData) > 0 {
		fields := execution.ResultFields(result.Data)
		fields["response"] = frame.Response.ExtraData
		result.Data = execution.NewToolResultPayload(fields)
	}
	if err := n.writeCaptures(env, result); err != nil {
		return result, err
	}
	n.writeStepMetadata(env)
	state.SetInteractionResumeNodeID(env, "")
	state.SetInteractionFrameRequested(env, false)
	contextdata.SetTyped(env, askFrameKey(n.step.ID), frame)
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".result", result.Data)
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".success", true)
	return result, nil
}

func (n *ThoughtRecipeStepNode) executeCapability(ctx context.Context, env *contextdata.Envelope) (*execution.Result, error) {
	if n == nil {
		return nil, fmt.Errorf("thoughtrecipe step node is nil")
	}
	n.writeStepMetadata(env)
	writeCapabilityMetadata(env, n.step.ID, n.step.CapabilityID)

	reg := n.deps.Registry
	if reg == nil {
		return nil, fmt.Errorf("thoughtrecipe step %q: capability_id requires a registry", n.id)
	}
	if scoped := n.scopedRegistry(); scoped != nil {
		reg = scoped
	}

	args := n.buildCapabilityArgs(env)
	toolResult, err := reg.InvokeCapability(ctx, env.State(), n.step.CapabilityID, args)

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

	if n.deps.IngestOutputs && n.deps.OutputIngester != nil {
		if payload, marshalErr := json.Marshal(data); marshalErr == nil {
			knowledge.IngestToolResultAsync(contextdata.WithEnvelope(ctx, env), n.deps.OutputIngester, n.step.CapabilityID, payload)
		}
	}

	result := &execution.Result{
		NodeID:  n.id,
		Success: success,
		Data:    execution.NewToolResultPayload(data),
	}
	if msg, ok := data["error"].(string); ok && strings.TrimSpace(msg) != "" {
		result.Error = msg
	}

	if err := n.writeCaptures(env, result); err != nil {
		return result, err
	}
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".result", data)
	contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".success", success)
	if result.Error != "" {
		contextdata.SetTyped(env, "euclo.execution.step."+n.step.ID+".error", result.Error)
	}

	return result, nil
}

func (n *ThoughtRecipeStepNode) buildTask(ctx context.Context, env *contextdata.Envelope) (*execution.Task, error) {
	data := thoughtrecipeTemplateData(env, n.step)
	n.writeStepMetadata(env)

	var instruction string
	if n.step.PromptID != "" {
		if n.deps.PromptRegistry == nil {
			return nil, fmt.Errorf("thoughtrecipe step %q: prompt_id requires a registry", n.step.ID)
		}
		var err error
		instruction, err = n.resolveFromRegistry(ctx, env)
		if err != nil {
			return nil, err
		}
	}

	if instruction == "" {
		if strings.TrimSpace(n.step.Question) != "" {
			instruction = n.renderTemplate(n.step.Question, data)
		}
	}
	if instruction == "" {
		if strings.TrimSpace(n.step.Goal) != "" {
			instruction = n.renderTemplate(n.step.Goal, data)
		}
	}
	if instruction == "" {
		instruction = n.renderTemplate(n.step.Prompt, data)
		if instruction == "" {
			instruction = n.step.Prompt
		}
	}

	task := &execution.Task{
		ID:          n.id,
		Type:        n.step.Paradigm,
		Instruction: instruction,
		Data:        make(map[string]any),
		Context:     data,
		Metadata:    n.stepMetadata(),
	}
	if len(n.step.Sources) > 0 {
		task.Context["euclo.run.sources"] = append([]string(nil), n.step.Sources...)
	}
	if len(n.step.Directives) > 0 {
		task.Context["euclo.run.directives"] = append([]string(nil), n.step.Directives...)
	}
	if strings.TrimSpace(n.step.Question) != "" {
		task.Context["euclo.ask.question"] = n.step.Question
	}
	if len(n.step.Choices) > 0 {
		task.Context["euclo.ask.choices"] = append([]string(nil), n.step.Choices...)
	}
	if strings.TrimSpace(n.step.ChoiceSource) != "" {
		task.Context["euclo.ask.choice_source"] = n.step.ChoiceSource
	}
	if strings.TrimSpace(n.step.Goal) != "" {
		task.Context["euclo.run.goal"] = n.step.Goal
	}
	if strings.EqualFold(strings.TrimSpace(n.step.Type), "delegate") {
		task.Context["euclo.delegate.source_keys"] = append([]string(nil), n.step.Sources...)
		if parentID, ok := contextdata.GetTyped[string](env, "euclo.delegate.parent_task_id"); ok {
			task.Context["euclo.delegate.parent_task_id"] = parentID
		}
		task.Context["euclo.delegate.child_task_id"] = env.TaskID
	}

	// Set prompt_id in task context for downstream agents (like react think node)
	if n.step.PromptID != "" {
		task.Context["prompt_id"] = n.step.PromptID
	}

	return task, nil
}

func (n *ThoughtRecipeStepNode) buildDelegationEnvelope(parent *contextdata.Envelope) *contextdata.Envelope {
	if parent == nil {
		return contextdata.NewEnvelope("", "")
	}
	policy := contextdata.HandoffPolicy{
		PreserveWorkingMemory: true,
		WorkingKeys:           append([]string(nil), n.step.Sources...),
		WorkingPrefixes: []string{
			intentcontext.ClarificationNamespace + ".",
			"euclo.execution.",
			"euclo.policy.",
			"euclo.clarification.",
		},
		PreserveStreamedContext:  true,
		PreserveRetrieval:        true,
		PreserveCheckpoints:      true,
		PreserveAssemblyMetadata: true,
		PreserveNodeID:           true,
	}
	child := parent.HandoffSnapshot(policy)
	if child == nil {
		child = contextdata.NewEnvelope(parent.TaskID, parent.SessionID)
	}
	child.TaskID = parent.TaskID + "::delegate::" + n.step.ID
	child.NodeID = n.id
	child.WorkingData["euclo.delegate.parent_task_id"] = parent.TaskID
	child.WorkingData["euclo.delegate.child_task_id"] = child.TaskID
	child.WorkingData["euclo.handoff.continuation"] = map[string]any{
		"shared_context":    true,
		"parent_task_id":    parent.TaskID,
		"child_task_id":     child.TaskID,
		"source_keys":       append([]string(nil), n.step.Sources...),
		"source_route_kind": mustRouteKind(parent),
	}
	if len(n.step.Sources) > 0 {
		child.WorkingData["euclo.delegate.source_keys"] = append([]string(nil), n.step.Sources...)
	}
	return child
}

func (n *ThoughtRecipeStepNode) ensureAskFrame(env *contextdata.Envelope) (*interaction.InteractionFrame, bool) {
	if n == nil {
		return nil, false
	}
	key := askFrameKey(n.step.ID)
	if frame, ok := contextdata.GetTyped[*interaction.InteractionFrame](env, key); ok && frame != nil {
		return frame, false
	}
	choices := n.askChoices(env)
	frame := interaction.NewAskUserFrame(env.TaskID, env.SessionID, n.step.Question, choices)
	frame.Payload["step_id"] = n.step.ID
	frame.Payload["step_type"] = n.step.Type
	frame.Payload["choice_source"] = n.step.ChoiceSource
	frame.Payload["frame_key"] = key
	// envelope: intentional dynamic key — ask frames are keyed by step ID.
	contextdata.SetTyped(env, key, frame)
	return frame, true
}

func (n *ThoughtRecipeStepNode) askChoices(env *contextdata.Envelope) []string {
	if n == nil {
		return nil
	}
	if len(n.step.Choices) > 0 {
		return append([]string(nil), n.step.Choices...)
	}
	if strings.TrimSpace(n.step.ChoiceSource) == "" {
		return nil
	}
	data := thoughtrecipeTemplateData(env, n.step)
	value, ok := lookupTemplateValue(data, n.step.ChoiceSource)
	if !ok {
		return nil
	}
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, entry := range v {
			if s := strings.TrimSpace(fmt.Sprint(entry)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if s := strings.TrimSpace(part); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func askFrameKey(stepID string) string {
	return "euclo.execution.ask." + sanitizeComponent(stepID) + ".frame"
}

func (n *ThoughtRecipeStepNode) writeDelegationCaptures(parent, child *contextdata.Envelope, result *execution.Result) error {
	if parent == nil || child == nil || result == nil {
		return nil
	}
	sourceData := child.Snapshot()
	if len(n.step.CaptureBindings) > 0 {
		_, err := ApplyCaptureBindingsFromSnapshot(parent, sourceData, n.step.CaptureBindings, execution.ResultFields(result.Data))
		return err
	}
	return n.writeCaptures(parent, result)
}

// resolveFromRegistry resolves the prompt from the registry using the PromptID.
func (n *ThoughtRecipeStepNode) resolveFromRegistry(ctx context.Context, env *contextdata.Envelope) (string, error) {
	if n.deps.PromptRegistry == nil {
		return "", fmt.Errorf("no prompt registry available")
	}

	// Build runtime context for prompt resolution
	rctx := n.buildRuntimeContext(ctx, env)

	// Resolve the prompt from registry
	return n.deps.PromptRegistry.Resolve(n.step.PromptID, rctx)
}

// buildRuntimeContext creates a prompt.RuntimeContext for thoughtrecipe step resolution.
func (n *ThoughtRecipeStepNode) buildRuntimeContext(ctx context.Context, env *contextdata.Envelope) prompt.RuntimeContext {
	data := thoughtrecipeTemplateData(env, n.step)
	scopedRegistry := n.scopedRegistry()
	runtime := prompt.NewRuntimeContext(env, n.step.Paradigm, "euclo").
		WithVariable("instruction", n.renderTemplate(n.step.Prompt, data)).
		WithVariable("question", func() string {
			if strings.TrimSpace(n.step.Question) != "" {
				return n.renderTemplate(n.step.Question, data)
			}
			return n.step.Prompt
		}()).
		WithVariable("prompt_id", n.step.PromptID).
		WithStateMap(clarificationRuntimeState(ctx, env)).
		WithStateMap(n.stepRuntimeState(data))

	runtime.Task = &execution.Task{
		ID:          n.id,
		Type:        n.step.Paradigm,
		Instruction: n.renderTemplate(n.step.Prompt, data),
		Context:     data,
	}
	if scopedRegistry != nil {
		runtime.Tools = scopedRegistry.ModelCallableTools(ctx)
		if snapshot := scopedRegistry.CaptureExecutionCatalogSnapshot(ctx); snapshot != nil {
			runtime.Capabilities = snapshot.InspectableCapabilities()
		}
	} else if n.deps.Registry != nil {
		runtime.Tools = n.deps.Registry.ModelCallableTools(ctx)
		runtime.Capabilities = n.deps.Registry.AllCapabilities()
	}
	runtime.AgentSpec = nil
	return runtime
}

func (n *ThoughtRecipeStepNode) stepMetadata() map[string]any {
	metadata := map[string]any{
		"execution_step_id":   n.step.ID,
		"execution_step_type": n.step.Type,
		"execution_paradigm":  n.step.Paradigm,
		"execution_goal":      n.step.Goal,
		"execution_question":  n.step.Question,
		"execution_mutation":  n.step.Mutation,
		"execution_hitl":      n.step.HITL,
	}
	if len(n.step.Sources) > 0 {
		metadata["execution_sources"] = append([]string(nil), n.step.Sources...)
	}
	if len(n.step.Choices) > 0 {
		metadata["execution_choices"] = append([]string(nil), n.step.Choices...)
	}
	if strings.TrimSpace(n.step.ChoiceSource) != "" {
		metadata["execution_choice_source"] = n.step.ChoiceSource
	}
	if len(n.step.Directives) > 0 {
		metadata["execution_directives"] = append([]string(nil), n.step.Directives...)
	}
	if strings.TrimSpace(n.step.CapabilityID) != "" {
		metadata[executionCapabilityIDKey] = n.step.CapabilityID
	}
	if cfg := cloneClarificationStepConfig(n.step.ClarificationConfig); cfg != nil {
		metadata["execution_clarification_type"] = n.step.Type
		metadata["execution_clarification_config"] = cfg
	}
	return metadata
}

func (n *ThoughtRecipeStepNode) stepRuntimeState(data map[string]any) map[string]any {
	state := map[string]any{
		"execution_step_id":   n.step.ID,
		"execution_step_type": n.step.Type,
		"execution_paradigm":  n.step.Paradigm,
		"execution_goal":      n.step.Goal,
		"execution_question":  n.step.Question,
		"execution_prompt_id": n.step.PromptID,
		"execution_instruction": func() string {
			if strings.TrimSpace(n.step.Goal) != "" {
				return n.renderTemplate(n.step.Goal, data)
			}
			return n.renderTemplate(n.step.Prompt, data)
		}(),
	}
	if len(n.step.Sources) > 0 {
		state["execution_sources"] = append([]string(nil), n.step.Sources...)
	}
	if len(n.step.Choices) > 0 {
		state["execution_choices"] = append([]string(nil), n.step.Choices...)
	}
	if strings.TrimSpace(n.step.ChoiceSource) != "" {
		state["execution_choice_source"] = n.step.ChoiceSource
	}
	if len(n.step.Directives) > 0 {
		state["execution_directives"] = append([]string(nil), n.step.Directives...)
	}
	if strings.TrimSpace(n.step.CapabilityID) != "" {
		state[executionCapabilityIDKey] = n.step.CapabilityID
	}
	if cfg := cloneClarificationStepConfig(n.step.ClarificationConfig); cfg != nil {
		state["execution_clarification_type"] = n.step.Type
		state["execution_clarification_config"] = cfg
		state["execution_clarification_schema_id"] = cfg.OutputSchemaID
		state["execution_clarification_validation_mode"] = cfg.ValidationMode
		state["execution_clarification_required_fields"] = append([]string(nil), cfg.RequiredFields...)
		state["execution_clarification_allowed_statuses"] = append([]intentcontext.ClarificationStepStatus(nil), cfg.AllowedStatuses...)
		state["execution_clarification_state_write_keys"] = append([]string(nil), cfg.StateWriteKeys...)
		state["execution_clarification_projection_policy"] = cfg.ProjectionPolicy
		state["execution_clarification_requery_on_success"] = cfg.RequeryOnSuccess
	}
	return state
}

func (n *ThoughtRecipeStepNode) writeStepMetadata(env *contextdata.Envelope) {
	base := "euclo.execution.step." + n.step.ID
	// envelope: intentional dynamic key — step metadata is namespaced by step ID.
	contextdata.SetTyped(env, base+".id", n.step.ID)
	contextdata.SetTyped(env, base+".type", n.step.Type)
	contextdata.SetTyped(env, base+".paradigm", n.step.Paradigm)
	contextdata.SetTyped(env, base+".goal", n.step.Goal)
	contextdata.SetTyped(env, base+".question", n.step.Question)
	contextdata.SetTyped(env, base+".prompt_id", n.step.PromptID)
	contextdata.SetTyped(env, base+".mutation", n.step.Mutation)
	contextdata.SetTyped(env, base+".hitl", n.step.HITL)
	if len(n.step.Sources) > 0 {
		contextdata.SetTyped(env, base+".sources", append([]string(nil), n.step.Sources...))
	}
	if len(n.step.Choices) > 0 {
		contextdata.SetTyped(env, base+".choices", append([]string(nil), n.step.Choices...))
	}
	if strings.TrimSpace(n.step.ChoiceSource) != "" {
		contextdata.SetTyped(env, base+".choice_source", n.step.ChoiceSource)
	}
	if len(n.step.Directives) > 0 {
		contextdata.SetTyped(env, base+".directives", append([]string(nil), n.step.Directives...))
	}
	if strings.TrimSpace(n.step.CapabilityID) != "" {
		contextdata.SetTyped(env, base+".capability_id", n.step.CapabilityID)
	}
	if cfg := cloneClarificationStepConfig(n.step.ClarificationConfig); cfg != nil {
		contextdata.SetTyped(env, base+".clarification_type", n.step.Type)
		contextdata.SetTyped(env, base+".clarification_config", cfg)
		contextdata.SetTyped(env, base+".clarification_schema_id", cfg.OutputSchemaID)
		contextdata.SetTyped(env, base+".clarification_validation_mode", cfg.ValidationMode)
		contextdata.SetTyped(env, base+".clarification_required_fields", append([]string(nil), cfg.RequiredFields...))
		contextdata.SetTyped(env, base+".clarification_allowed_statuses", append([]intentcontext.ClarificationStepStatus(nil), cfg.AllowedStatuses...))
		contextdata.SetTyped(env, base+".clarification_state_write_keys", append([]string(nil), cfg.StateWriteKeys...))
		contextdata.SetTyped(env, base+".clarification_projection_policy", cfg.ProjectionPolicy)
		contextdata.SetTyped(env, base+".clarification_requery_on_success", cfg.RequeryOnSuccess)
	}
}

func writeCapabilityMetadata(env *contextdata.Envelope, stepID, capabilityID string) {
	if strings.TrimSpace(capabilityID) == "" {
		return
	}
	state.SetExecutionCapabilityID(env, capabilityID)
	// envelope: intentional dynamic key — capability metadata is namespaced by step ID.
	contextdata.SetTyped(env, "euclo.execution.step."+stepID+".capability_id", capabilityID)
}

func clarificationRuntimeState(ctx context.Context, env *contextdata.Envelope) map[string]any {
	state := make(map[string]any)
	if current, err := intentcontext.NewStateStore().Read(ctx, env); err == nil && current != nil {
		state[intentcontext.ClarificationStateKey] = current.Clone()
		state["euclo.intent.clarification.state_version"] = current.StateVersion
		state["euclo.intent.clarification.current_turn_id"] = current.CurrentTurnID
		state["euclo.intent.clarification.active_thoughtrecipe_id"] = current.ActiveThoughtRecipeID
		state["euclo.intent.clarification.last_checkpoint_id"] = current.LastCheckpointID
		state["euclo.intent.clarification.last_checkpoint_seq"] = current.LastCheckpointSeq
		state["euclo.intent.clarification.confirmed_entity_ids"] = stableEntityIDs(current.ConfirmedEntities)
		state["euclo.intent.clarification.confirmed_scope_ids"] = stableScopeIDs(current.ConfirmedScopes)
		state["euclo.intent.clarification.pending_relation_intents"] = append([]intentcontext.RelationIntent(nil), current.PendingRelationIntents...)
		state["euclo.intent.clarification.pending_projection_ids"] = stableProjectionIDs(current.PendingProjection)
		state["euclo.intent.clarification.applied_mutations"] = append([]intentcontext.ProjectionRecord(nil), current.AppliedMutations...)
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

func mustRouteKind(env *contextdata.Envelope) string {
	v, _ := contextdata.GetTyped[string](env, "euclo.dispatch.route_kind")
	return strings.TrimSpace(v)
}

func (n *ThoughtRecipeStepNode) buildCapabilityArgs(env *contextdata.Envelope) map[string]any {
	data := thoughtrecipeTemplateData(env, n.step)
	args := make(map[string]any, len(data)+len(n.step.Step.Config))
	for key, value := range data {
		args[key] = value
	}
	for key, value := range n.step.Step.Config {
		if _, exists := args[key]; !exists {
			args[key] = value
		}
	}
	return args
}

func (n *ThoughtRecipeStepNode) buildAgent(task *execution.Task) (agentgraph.WorkflowExecutor, error) {
	deps := n.deps
	if scopedRegistry := n.scopedRegistry(); scopedRegistry != nil {
		deps = depsWithRegistry(deps, scopedRegistry)
	}

	switch strings.ToLower(strings.TrimSpace(n.step.Paradigm)) {
	case "react":
		return reactagent.New(deps, n.streamOptions()...), nil
	case "planner":
		return planneragent.New(deps), nil
	case "htn":
		primitive := reactagent.New(deps, n.streamOptions()...)
		return htnagent.New(deps, htnagent.NewMethodLibrary(), append([]htnagent.Option{
			htnagent.WithPrimitiveExec(primitive),
		}, n.streamOptionsHTN()...)...), nil
	case "reflection":
		delegate := reactagent.New(deps, n.streamOptions()...)
		return reflectionagent.New(deps, delegate), nil
	case "blackboard":
		return blackboardagent.New(deps, n.streamOptionsBlackboard()...), nil
	case "chainer":
		return chaineragent.New(deps, n.streamOptionsChainer()...), nil
	case "pipeline":
		return pipelineagent.New(deps, n.streamOptionsPipeline()...), nil
	case "rewoo":
		agent := rewooagent.New(deps)
		agent.Options = n.rewooOptions()
		return agent, nil
	case "goalcon":
		agent := goalconagent.New(deps, goalconagent.DefaultOperatorRegistry(), n.streamOptionsGoalCon()...)
		if agent != nil && agent.PlanExecutor == nil {
			agent.PlanExecutor = reactagent.New(deps, n.streamOptions()...)
		}
		return agent, nil
	default:
		return nil, fmt.Errorf("thoughtrecipe step %q has unsupported paradigm %q", n.step.ID, n.step.Paradigm)
	}
}

func (n *ThoughtRecipeStepNode) scopedRegistry() *registry.CapabilityRegistry {
	if n == nil || n.deps == nil || n.deps.Registry == nil {
		return nil
	}
	allowed := n.effectiveToolAllowlist()
	if len(allowed) == 0 {
		return nil
	}
	return n.deps.Registry.WithAllowlist(allowed)
}

func (n *ThoughtRecipeStepNode) effectiveToolAllowlist() []string {
	if n == nil || n.deps == nil || n.deps.Registry == nil {
		return nil
	}
	if len(n.step.EffectiveToolNames) == 0 {
		return nil
	}
	allowed := make([]string, 0, len(n.step.EffectiveToolNames))
	seen := make(map[string]struct{}, len(n.step.EffectiveToolNames))
	for _, toolName := range n.step.EffectiveToolNames {
		name := strings.TrimSpace(toolName)
		if name == "" {
			continue
		}
		desc, ok := n.deps.Registry.GetCapability(name)
		if !ok {
			continue
		}
		if desc.ID == "" {
			continue
		}
		if _, exists := seen[desc.ID]; exists {
			continue
		}
		seen[desc.ID] = struct{}{}
		allowed = append(allowed, desc.ID)
	}
	if len(allowed) == 0 {
		return []string{"__euclo__.deny_all__"}
	}
	return allowed
}

func (n *ThoughtRecipeStepNode) streamOptions() []reactagent.Option {
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

func (n *ThoughtRecipeStepNode) streamOptionsHTN() []htnagent.Option {
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

func (n *ThoughtRecipeStepNode) streamOptionsBlackboard() []blackboardagent.Option {
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

func (n *ThoughtRecipeStepNode) streamOptionsChainer() []chaineragent.Option {
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

func (n *ThoughtRecipeStepNode) streamOptionsPipeline() []pipelineagent.Option {
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

func (n *ThoughtRecipeStepNode) streamOptionsGoalCon() []goalconagent.Option {
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

func (n *ThoughtRecipeStepNode) rewooOptions() rewooagent.RewooOptions {
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

func (n *ThoughtRecipeStepNode) writeCaptures(env *contextdata.Envelope, result *execution.Result) error {
	if n == nil || result == nil {
		return nil
	}
	if len(n.step.CaptureBindings) > 0 {
		_, err := ApplyCaptureBindings(env, n.step.CaptureBindings, execution.ResultFields(result.Data))
		return err
	}
	return nil
}

func thoughtrecipeTemplateData(env *contextdata.Envelope, step ExecutionStep) map[string]any {
	data := map[string]any{
		"TaskID":    "",
		"SessionID": "",
		"StepID":    step.ID,
		"Paradigm":  step.Paradigm,
		"Prompt":    step.Prompt,
		"Goal":      step.Goal,
	}
	if len(step.Sources) > 0 {
		data["RunSources"] = append([]string(nil), step.Sources...)
	}
	if len(step.Directives) > 0 {
		data["RunDirectives"] = append([]string(nil), step.Directives...)
	}
	if env != nil {
		data["TaskID"] = env.TaskID
		data["SessionID"] = env.SessionID
		for key, value := range env.Snapshot() {
			data[key] = value
		}
	}
	return data
}

func (n *ThoughtRecipeStepNode) renderTemplate(src string, data map[string]any) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	tpl, err := template.New("thoughtrecipe-step").Option("missingkey=zero").Parse(src)
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
	value, ok := data[ref]
	return value, ok
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
